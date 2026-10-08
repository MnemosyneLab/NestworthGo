package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

const planLifetime = 10 * time.Minute

type changePlan struct {
	Type      string                       `json:"type,omitempty"`
	ID        string                       `json:"id"`
	Command   history.ChangeCommandRequest `json:"command"`
	Version   string                       `json:"version"`
	ExpiresAt time.Time                    `json:"expiresAt"`
}

type ChangePlanPreview struct {
	PlanID    string                       `json:"planId"`
	ExpiresAt string                       `json:"expiresAt"`
	Command   history.ChangeCommandRequest `json:"command"`
	Preview   wire.ChangePreviewDTO        `json:"preview"`
}

type CommitChangeInput struct {
	PlanID string `json:"planId" jsonschema:"The opaque ID returned by the matching preview tool. Reusing this plan cannot record it twice, even with another operationId."`
}

func (s *Service) ledgerTools(server *mcp.Server) {
	s.productOperationTools(server)
	s.batchTools(server)
	s.correctionTools(server)
	s.reconciliationTools(server)
	// Persisting a preview stores only a private plan; it never mutates money,
	// positions, or the activity ledger.
	readTool(server, "preview_change", "Prepare one ledger record without posting it. Requires ledger_write. kind: position_import (accountId, instrumentId, quantity, original per-unit unitCost, currency matching the instrument; NEW position only, no cash leg); position_transfer (fromHoldingId, quantity, toHoldingId for an existing destination or toAccountId to find/create a destination holding for the same instrument; carries cost without income or sale); money_added/money_removed (accountId, amount, currency, reason); trade (side buy/sell, settlementAccountId, instrumentId, optional holdingId, quantity, gross total, grossCurrency, optional fee/feeCurrency); cash_dividend (holdingId, amount, currency); cash_transfer (fromAccountId, toAccountId, sent/sentCurrency, received/receivedCurrency, optional fee/feeCurrency); fx_conversion (accountId, sold/soldCurrency, bought/boughtCurrency, optional fee/feeCurrency); debt_draw/debt_payment (debtAccountId, cashAccountId, principal/principalCurrency, payment optional interestOrFee/interestOrFeeCurrency). Supply effectiveAt RFC3339 OR effectiveLocalDate and effectiveLocalTime in the history timezone. Omitted time is frozen at preview. Read get_catalog for reasons. Inspect effects, then pass planId to commit_change within ten minutes. Preview activity and new holding IDs are provisional; use committed receipt IDs. A first buy or transfer to a new account creates a holding atomically; this records completed trades, never places orders. Historical entries use the same replay validation as the App.", s.previewChange)
	closed := false
	mcp.AddTool(server, &mcp.Tool{Name: "commit_change", Description: "Post exactly one previously previewed ledger record. Requires ledger_write. Input contains only planId; the server stores the exact command. Expired or stale plans must be previewed again. Retry with identical operationId and input after interruptions; the database mutation key prevents duplicate posting. Returns a durable operation receipt. Reusing the same plan with another operationId still returns the original activity. For an atomic group use preview_batch and commit_batch instead.", Annotations: &mcp.ToolAnnotations{IdempotentHint: true, OpenWorldHint: &closed}}, func(ctx context.Context, _ *mcp.CallToolRequest, in Mutation[CommitChangeInput]) (*mcp.CallToolResult, Response, error) {
		value, err := s.executeWithRecovery(ctx, in.OperationID, "commit_change", in.Input, true, func(ctx context.Context) (any, error) { return s.commitChange(ctx, in.Input) })
		if err != nil {
			return nil, Response{}, safeError(err)
		}
		return nil, Response{Data: value}, nil
	})
}

// Reject fields silently ignored by the UI's tagged-union adapter. The agent
// boundary must not turn a misunderstood command into a different operation.
func validateChangeRequest(in history.ChangeCommandRequest) error {
	fields := map[history.ChangeCommandKind]string{
		"position_import":              "accountId instrumentId quantity unitCost currency",
		history.ChangeMoneyAdded:       "accountId amount currency reason",
		history.ChangeMoneyRemoved:     "accountId holdingId amount currency reason",
		history.ChangeCashDividend:     "holdingId amount currency",
		history.ChangeCashTransfer:     "fromAccountId toAccountId sent sentCurrency received receivedCurrency fee feeCurrency",
		history.ChangeFXConversion:     "accountId sold soldCurrency bought boughtCurrency fee feeCurrency",
		history.ChangePositionTransfer: "fromHoldingId toHoldingId toAccountId quantity",
		history.ChangeTrade:            "settlementAccountId holdingId instrumentId side quantity gross grossCurrency fee feeCurrency",
		history.ChangeDebtDraw:         "debtAccountId cashAccountId principal principalCurrency",
		history.ChangeDebtPayment:      "debtAccountId cashAccountId principal principalCurrency interestOrFee interestOrFeeCurrency",
	}
	allowed, ok := fields[in.Kind]
	if !ok {
		return fail("validation", "unsupported ledger kind; use preview_reconciliation for target balances or quantities")
	}
	set := map[string]bool{}
	for _, field := range strings.Fields("kind effectiveAt effectiveLocalDate effectiveLocalTime note " + allowed) {
		set[field] = true
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return err
	}
	for key := range values {
		if !set[key] {
			return fail("validation", "field is not valid for this ledger kind: "+key)
		}
	}
	if in.EffectiveAt != "" && (in.EffectiveLocalDate != "" || in.EffectiveLocalTime != "") {
		return fail("validation", "supply either effectiveAt or local date/time, not both")
	}
	if (in.Fee == "") != (in.FeeCurrency == "") || (in.InterestOrFee == "") != (in.InterestOrFeeCurrency == "") {
		return fail("validation", "optional charges require both amount and currency")
	}
	return nil
}

func (s *Service) previewChange(ctx context.Context, in history.ChangeCommandRequest) (any, error) {
	var err error
	in, err = normalizeChangeRequest(in, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	preview, version, err := s.app.PreviewChangeGuardedWith(ctx, func(ctx context.Context) (any, error) {
		origin, err := s.app.HistoryOrigin(ctx)
		if err != nil {
			return nil, err
		}
		if origin == nil {
			return nil, fail("history_not_started", "start history in the App before recording changes")
		}
		return ledgerCommand(in, origin)
	})
	if err != nil {
		return nil, err
	}
	plan := changePlan{Type: "change", ID: uuid.NewString(), Command: in, Version: version, ExpiresAt: time.Now().UTC().Add(planLifetime)}
	if err := s.writePrivate(filepath.Join(s.dir, "plans", plan.ID+".json"), plan); err != nil {
		return nil, err
	}
	return ChangePlanPreview{PlanID: plan.ID, ExpiresAt: plan.ExpiresAt.Format(time.RFC3339Nano), Command: in, Preview: wire.FromChangePreview(preview)}, nil
}

func (s *Service) commitChange(ctx context.Context, in CommitChangeInput) (any, error) {
	raw, err := s.loadPreviewPlan(in.PlanID, "change")
	if err != nil {
		return nil, err
	}
	var plan changePlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return nil, err
	}
	var result any
	err = s.app.WithWrite(ctx, func(ctx context.Context) error {
		var err error
		result, err = s.commitChangeLocked(ctx, plan)
		return err
	})
	return result, err
}

// Resolve history and check expiry inside the same application write permit as
// the commit. Waiting behind another preview must not extend an expired plan.
func (s *Service) commitChangeLocked(ctx context.Context, plan changePlan) (any, error) {
	origin, err := s.app.HistoryOrigin(ctx)
	if err != nil {
		return nil, err
	}
	if origin == nil {
		return nil, fail("history_not_started", "history is not started")
	}
	command, err := ledgerCommand(plan.Command, origin)
	if err != nil {
		return nil, err
	}
	// The plan ID, not the outer operation receipt, is the durable transaction
	// identity. Two callers cannot post the same preview twice.
	plan.Command.MutationID = plan.ID
	mutationID, hash, err := plan.Command.MutationEnvelope()
	if err != nil {
		return nil, err
	}
	version := plan.Version
	if !time.Now().Before(plan.ExpiresAt) {
		version = "expired"
	}
	// Replay is checked before the guard: a successful write is recoverable
	// after expiry/restart or a crash before its MCP receipt was saved.
	preview, err := s.app.RecordChangeGuarded(ctx, command, mutationID, hash, version)
	if err != nil {
		return nil, err
	}
	return wire.FromChangePreview(preview), nil
}

// All missing times in a batch use the same instant. Local times remain tied
// to the history timezone resolved while holding the application preview gate.
func normalizeChangeRequest(in history.ChangeCommandRequest, now time.Time) (history.ChangeCommandRequest, error) {
	in.EffectiveAt = strings.TrimSpace(in.EffectiveAt)
	in.EffectiveLocalDate = strings.TrimSpace(in.EffectiveLocalDate)
	in.EffectiveLocalTime = strings.TrimSpace(in.EffectiveLocalTime)
	if err := validateChangeRequest(in); err != nil {
		return in, err
	}
	if in.EffectiveAt == "" && in.EffectiveLocalDate == "" && in.EffectiveLocalTime == "" {
		in.EffectiveAt = now.Format(time.RFC3339Nano)
	}
	return in, nil
}

func ledgerCommand(in history.ChangeCommandRequest, origin *domain.HistoryOrigin) (any, error) {
	if in.Kind != "position_import" {
		return in.ToCommand(origin.HouseholdID, origin.Timezone)
	}
	accountID, err := domain.ParseAccountID(in.AccountID)
	if err != nil {
		return nil, err
	}
	instrumentID, err := domain.ParseInstrumentID(in.InstrumentID)
	if err != nil {
		return nil, err
	}
	quantity, err := domain.ParseQuantity(in.Quantity)
	if err != nil {
		return nil, err
	}
	currency, err := domain.ParseSupportedCurrency(in.Currency)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.UnitCost) == "" {
		return nil, fail("cost_basis_required", "provide the original per-unit cost; current market price is not an acquisition cost")
	}
	unitCost, err := domain.ParseUnitPrice(in.UnitCost)
	if err != nil {
		return nil, err
	}
	var at time.Time
	if in.EffectiveLocalDate != "" || in.EffectiveLocalTime != "" {
		if in.EffectiveLocalDate == "" || in.EffectiveLocalTime == "" {
			return nil, fail("validation", "local date and time must be provided together")
		}
		at, err = domain.ResolveLocalDateTime(in.EffectiveLocalDate, in.EffectiveLocalTime, origin.Timezone)
	} else {
		at, err = time.Parse(time.RFC3339Nano, in.EffectiveAt)
		if err != nil {
			return nil, fail("validation", "effectiveAt must be an RFC3339 timestamp")
		}
	}
	if err != nil {
		return nil, err
	}
	return domain.PositionImportInput{HouseholdID: origin.HouseholdID, AccountID: accountID, InstrumentID: instrumentID, Quantity: quantity, UnitCost: &unitCost, Currency: currency, EffectiveAt: at, Note: in.Note}, nil
}

// Plan-shape mistakes are rejected before acquiring a ledger write permit, so
// choosing the wrong commit tool cannot invalidate an otherwise valid preview.
func (s *Service) loadPreviewPlan(planID, expectedType string) ([]byte, error) {
	id, err := uuid.Parse(planID)
	if err != nil {
		return nil, fail("validation", "planId must be a UUID returned by a preview tool")
	}
	raw, err := s.readPrivate(filepath.Join(s.dir, "plans", id.String()+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fail("not_found", "preview plan not found; create a new preview")
	}
	if err != nil {
		return nil, err
	}
	var header struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return nil, err
	}
	if header.Type == "" {
		header.Type = "change"
	} // persisted stage-2 single plans
	if header.Type != expectedType || header.ID != id.String() {
		return nil, fail("validation", "use the commit tool matching this preview plan")
	}
	return raw, nil
}
