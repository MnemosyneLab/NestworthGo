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
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
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

func (s *Service) ledgerTools(server *toolSet) {
	s.productOperationTools(server)
	s.productDataTools(server)
	s.liquidityPolicyTools(server)
	s.batchTools(server)
	s.correctionTools(server)
	s.reconciliationTools(server)
	s.commitPlanTool(server)

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
	for _, field := range strings.Fields("kind effectiveAt effectiveLocalDate effectiveLocalTime note reconciliationLink " + allowed) {
		set[field] = true
	}
	raw, err := jsoncodec.Marshal(in)
	if err != nil {
		return err
	}
	var values map[string]json.RawMessage
	if err := jsoncodec.Unmarshal(raw, &values); err != nil {
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

// previewHostLedgerChange keeps the host-owned review and receipt format private.
func (s *Service) previewHostLedgerChange(ctx context.Context, in history.ChangeCommandRequest) (ChangePlanPreview, error) {
	var err error
	in, err = normalizeChangeRequest(in, time.Now().UTC())
	if err != nil {
		return ChangePlanPreview{}, err
	}
	var resolved any
	preview, version, err := s.app.PreviewChangeGuardedWith(ctx, func(ctx context.Context) (any, error) {
		origin, err := s.app.HistoryOrigin(ctx)
		if err != nil {
			return nil, err
		}
		if origin == nil {
			return nil, fail("history_not_started", "start history in the App before recording changes")
		}
		resolved, err = ledgerCommand(in, origin)
		return resolved, err
	})
	if err != nil {
		return ChangePlanPreview{}, err
	}
	if err := s.app.ValidateReconciliationCommandLink(ctx, in.ReconciliationLink, resolved, preview); err != nil {
		return ChangePlanPreview{}, err
	}
	plan := changePlan{Type: "change", ID: uuid.NewString(), Command: in, Version: version, ExpiresAt: time.Now().UTC().Add(planLifetime)}
	if err := s.writePrivate(filepath.Join(s.dir, "plans", plan.ID+".json"), plan); err != nil {
		return ChangePlanPreview{}, err
	}
	return ChangePlanPreview{PlanID: plan.ID, ExpiresAt: plan.ExpiresAt.Format(time.RFC3339Nano), Command: in, Preview: wire.FromChangePreview(preview)}, nil
}

func (s *Service) commitChange(ctx context.Context, in CommitChangeInput) (any, error) {
	raw, err := s.loadPreviewPlan(in.PlanID, "change")
	if err != nil {
		return nil, err
	}
	var plan changePlan
	if err := jsoncodec.Unmarshal(raw, &plan); err != nil {
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
	preview, err := s.app.RecordChangeGuarded(domain.WithReconciliationLink(ctx, plan.Command.ReconciliationLink), command, mutationID, hash, version)
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
		return nil, fail("operation_outcome_unknown", "reviewed plan unavailable; inspect the operation receipt and current data; do not replace an unknown submission")
	}
	if err != nil {
		return nil, err
	}
	var header struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if err := jsoncodec.Unmarshal(raw, &header); err != nil {
		return nil, err
	}
	if header.Type != expectedType || header.ID != id.String() {
		return nil, fail("validation", "use the commit tool matching this preview plan")
	}
	return raw, nil
}
