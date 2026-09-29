package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

// A target describes a current balance or quantity, never a historical entry.
type ReconciliationTargetInput struct {
	AccountID      string  `json:"accountId,omitempty"`
	HoldingID      string  `json:"holdingId,omitempty"`
	TargetBalance  string  `json:"targetBalance,omitempty"`
	Currency       string  `json:"currency,omitempty"`
	TargetQuantity string  `json:"targetQuantity,omitempty"`
	UnitCost       string  `json:"unitCost,omitempty"`
	TotalCost      string  `json:"totalCost,omitempty"`
	Note           *string `json:"note,omitempty"`
}

type ReconciliationInput struct {
	Targets []ReconciliationTargetInput `json:"targets" jsonschema:"One to 100 current targets. For an account use accountId, targetBalance and currency; for a holding use holdingId and targetQuantity. An increase requires unitCost, or totalCost plus currency for the entire target position. Omit targetQuantity to adjust cost only. Do not combine unitCost and totalCost. Expanded plans have at most 100 commands."`
}

// Only the resolved reconciliation command kinds may be saved here.
// Keeping an explicit tagged DTO prevents an arbitrary ledger request from
// being smuggled into a private reconciliation plan.
type reconciliationCommand struct {
	Kind             history.ChangeCommandKind `json:"kind"`
	AccountID        string                    `json:"accountId,omitempty"`
	HoldingID        string                    `json:"holdingId,omitempty"`
	Amount           string                    `json:"amount,omitempty"`
	Currency         string                    `json:"currency,omitempty"`
	NewValue         string                    `json:"newValue,omitempty"`
	NewValueCurrency string                    `json:"newValueCurrency,omitempty"`
	Quantity         string                    `json:"quantity,omitempty"`
	Added            bool                      `json:"added,omitempty"`
	UnitCost         string                    `json:"unitCost,omitempty"`
	Reason           string                    `json:"reason,omitempty"`
	EffectiveAt      string                    `json:"effectiveAt"`
	Note             *string                   `json:"note,omitempty"`
}

type reconciliationPlan struct {
	Type      string                      `json:"type"`
	ID        string                      `json:"id"`
	Targets   []ReconciliationTargetInput `json:"targets"`
	Commands  []reconciliationCommand     `json:"commands"`
	Version   string                      `json:"version"`
	ExpiresAt time.Time                   `json:"expiresAt"`
}

type ReconciliationPlanPreview struct {
	PlanID    string                      `json:"planId"`
	ExpiresAt string                      `json:"expiresAt"`
	Targets   []ReconciliationTargetInput `json:"targets"`
	Commands  []reconciliationCommand     `json:"commands"`
	Previews  []wire.ChangePreviewDTO     `json:"previews"`
}

func (s *Service) reconciliationTools(server *mcp.Server) {
	readTool(server, "preview_reconciliation", "Preview 1 to 100 current account balances or holding quantities as exact ledger adjustments. Requires ledger_write. Account targets need accountId, targetBalance and currency. Holding targets accept targetQuantity and/or totalCost. totalCost requires the instrument currency and sets the entire remaining position cost; omit targetQuantity for cost-only correction. Without totalCost, quantity increases require unitCost for the added units. Never combine unitCost and totalCost. Quantity and cost changes can produce two commands and the plan is limited to 100 commands. Current values must differ. The preview posts nothing. Inspect every resolved command and effect before commit_reconciliation. Plans expire in ten minutes and become stale on other writes.", s.previewReconciliation)
	closed := false
	mcp.AddTool(server, &mcp.Tool{Name: "commit_reconciliation", Description: "Commit exactly the resolved preview_reconciliation plan in one database transaction. Requires ledger_write. Pass operationId and input.planId only. An identical retry or reuse of the plan returns the original activities, even after an interruption. An uncommitted expired or stale plan requires a new preview.", Annotations: &mcp.ToolAnnotations{IdempotentHint: true, OpenWorldHint: &closed}}, func(ctx context.Context, _ *mcp.CallToolRequest, in Mutation[CommitChangeInput]) (*mcp.CallToolResult, Response, error) {
		value, err := s.executeWithRecovery(ctx, in.OperationID, "commit_reconciliation", in.Input, true, func(ctx context.Context) (any, error) { return s.commitReconciliation(ctx, in.Input) })
		if err != nil {
			return nil, Response{}, safeError(err)
		}
		return nil, Response{Data: value}, nil
	})
}

func targetInputError(index int, err error) error {
	value := *safeError(err)
	field := fmt.Sprintf("targets[%d]", index)
	if value.Field != "" {
		field += "." + value.Field
	}
	value.Field = field
	return &value
}

func parseReconciliationTargets(input []ReconciliationTargetInput) ([]application.ReconciliationTarget, error) {
	if len(input) < 1 || len(input) > 100 {
		return nil, fail("validation", "a reconciliation requires 1 to 100 targets")
	}
	targets := make([]application.ReconciliationTarget, len(input))
	for i, in := range input {
		target := application.ReconciliationTarget{Note: in.Note}
		switch {
		case in.AccountID != "" && in.HoldingID == "" && in.TargetBalance != "" && in.Currency != "" && in.TargetQuantity == "" && in.UnitCost == "" && in.TotalCost == "":
			id, err := domain.ParseAccountID(in.AccountID)
			if err != nil {
				return nil, targetInputError(i, err)
			}
			currency, err := domain.ParseSupportedCurrency(in.Currency)
			if err != nil {
				return nil, targetInputError(i, err)
			}
			balance, err := domain.ParseMoney(in.TargetBalance, currency)
			if err != nil {
				return nil, targetInputError(i, err)
			}
			target.AccountID, target.TargetBalance = &id, &balance
		case in.HoldingID != "" && in.AccountID == "" && in.TargetBalance == "" && (in.TargetQuantity != "" || in.TotalCost != ""):
			if (in.TotalCost == "" && in.Currency != "") || (in.TotalCost != "" && (in.Currency == "" || in.UnitCost != "")) {
				return nil, targetInputError(i, fail("validation", "totalCost requires currency and cannot be combined with unitCost"))
			}
			id, err := domain.ParseHoldingID(in.HoldingID)
			if err != nil {
				return nil, targetInputError(i, err)
			}
			target.HoldingID = &id
			if in.TargetQuantity != "" {
				quantity, err := domain.ParseQuantity(in.TargetQuantity)
				if err != nil {
					return nil, targetInputError(i, err)
				}
				target.TargetQuantity = &quantity
			}
			if in.TotalCost != "" {
				currency, err := domain.ParseSupportedCurrency(in.Currency)
				if err != nil {
					return nil, targetInputError(i, err)
				}
				cost, err := domain.ParseMoney(in.TotalCost, currency)
				if err != nil {
					return nil, targetInputError(i, err)
				}
				target.TargetTotalCost = &cost
			}
			if in.UnitCost != "" {
				cost, err := domain.ParseUnitPrice(in.UnitCost)
				if err != nil {
					return nil, targetInputError(i, err)
				}
				target.UnitCost = &cost
			}
		default:
			return nil, targetInputError(i, &domain.Error{Code: domain.ErrValidation, Message: "provide either an account balance or a holding quantity and/or total cost"})
		}
		targets[i] = target
	}
	return targets, nil
}

func resolvedReconciliationCommand(command any) (reconciliationCommand, error) {
	switch value := command.(type) {
	case domain.PositionCostAdjustmentInput:
		return reconciliationCommand{Kind: "cost_adjustment", HoldingID: value.HoldingID.String(), UnitCost: value.UnitCost.Canonical(), EffectiveAt: value.EffectiveAt.Format(time.RFC3339Nano), Note: value.Note}, nil
	case domain.ValueUpdateInput:
		return reconciliationCommand{Kind: history.ChangeValueUpdate, AccountID: value.AccountID.String(), NewValue: value.NewValue.CanonicalAmount(), NewValueCurrency: value.NewValue.Currency().String(), Reason: string(value.Reason), EffectiveAt: value.EffectiveAt.Format(time.RFC3339Nano), Note: value.Note}, nil
	case domain.MoneyAddedInput:
		return reconciliationCommand{Kind: history.ChangeMoneyAdded, AccountID: value.AccountID.String(), Amount: value.Amount.CanonicalAmount(), Currency: value.Amount.Currency().String(), Reason: string(value.Reason), EffectiveAt: value.EffectiveAt.Format(time.RFC3339Nano), Note: value.Note}, nil
	case domain.MoneyRemovedInput:
		return reconciliationCommand{Kind: history.ChangeMoneyRemoved, AccountID: value.AccountID.String(), Amount: value.Amount.CanonicalAmount(), Currency: value.Amount.Currency().String(), Reason: string(value.Reason), EffectiveAt: value.EffectiveAt.Format(time.RFC3339Nano), Note: value.Note}, nil
	case domain.PositionAdjustmentInput:
		result := reconciliationCommand{Kind: history.ChangePositionAdjustment, HoldingID: value.HoldingID.String(), Quantity: value.Quantity.Canonical(), Added: value.Added, EffectiveAt: value.EffectiveAt.Format(time.RFC3339Nano), Note: value.Note}
		if value.UnitCost != nil {
			result.UnitCost = value.UnitCost.Canonical()
		}
		return result, nil
	default:
		return reconciliationCommand{}, fail("internal", "unexpected reconciliation command")
	}
}

func (command reconciliationCommand) toDomain(householdID domain.HouseholdID, timezone string) (any, error) {
	if command.Kind == "cost_adjustment" {
		id, err := domain.ParseHoldingID(command.HoldingID)
		if err != nil {
			return nil, err
		}
		cost, err := domain.ParseUnitPrice(command.UnitCost)
		if err != nil {
			return nil, err
		}
		at, err := time.Parse(time.RFC3339Nano, command.EffectiveAt)
		if err != nil {
			return nil, fail("validation", "invalid cost adjustment timestamp")
		}
		return domain.PositionCostAdjustmentInput{HouseholdID: householdID, HoldingID: id, UnitCost: cost, EffectiveAt: at, Note: command.Note}, nil
	}
	switch command.Kind {
	case history.ChangeValueUpdate, history.ChangeMoneyAdded, history.ChangeMoneyRemoved, history.ChangePositionAdjustment:
	default:
		return nil, fail("validation", "reconciliation plan contains an unsupported command")
	}
	if command.Kind == history.ChangeValueUpdate || command.Kind == history.ChangeMoneyAdded || command.Kind == history.ChangeMoneyRemoved {
		if command.Reason != string(domain.ReasonReconciliation) {
			return nil, fail("validation", "reconciliation plan contains an invalid reason")
		}
	}
	return (history.ChangeCommandRequest{Kind: command.Kind, AccountID: command.AccountID, HoldingID: command.HoldingID, Amount: command.Amount, Currency: command.Currency, NewValue: command.NewValue, NewValueCurrency: command.NewValueCurrency, Quantity: command.Quantity, Added: command.Added, UnitCost: command.UnitCost, Reason: command.Reason, EffectiveAt: command.EffectiveAt, Note: command.Note}).ToCommand(householdID, timezone)
}

func (s *Service) previewReconciliation(ctx context.Context, in ReconciliationInput) (any, error) {
	targets, err := parseReconciliationTargets(in.Targets)
	if err != nil {
		return nil, err
	}
	var saved []reconciliationCommand
	previews, version, err := s.app.PreviewChangesGuardedWith(ctx, func(ctx context.Context) ([]any, error) {
		commands, err := s.app.BuildReconciliationCommands(ctx, targets)
		if err != nil {
			return nil, err
		}
		saved = make([]reconciliationCommand, len(commands))
		for i, command := range commands {
			saved[i], err = resolvedReconciliationCommand(command)
			if err != nil {
				return nil, err
			}
		}
		return commands, nil
	})
	if err != nil {
		return nil, err
	}
	plan := reconciliationPlan{Type: "reconciliation", ID: uuid.NewString(), Targets: in.Targets, Commands: saved, Version: version, ExpiresAt: time.Now().UTC().Add(planLifetime)}
	if err := s.writePrivate(filepath.Join(s.dir, "plans", plan.ID+".json"), plan); err != nil {
		return nil, err
	}
	result := ReconciliationPlanPreview{PlanID: plan.ID, ExpiresAt: plan.ExpiresAt.Format(time.RFC3339Nano), Targets: in.Targets, Commands: saved, Previews: make([]wire.ChangePreviewDTO, len(previews))}
	for i, preview := range previews {
		result.Previews[i] = wire.FromChangePreview(preview)
	}
	return result, nil
}

func (s *Service) commitReconciliation(ctx context.Context, in CommitChangeInput) (any, error) {
	raw, err := s.loadPreviewPlan(in.PlanID, "reconciliation")
	if err != nil {
		return nil, err
	}
	var plan reconciliationPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return nil, err
	}
	var result BatchChangeResult
	err = s.app.WithWrite(ctx, func(ctx context.Context) error {
		origin, err := s.app.HistoryOrigin(ctx)
		if err != nil {
			return err
		}
		if origin == nil {
			return fail("history_not_started", "start history in the App before recording changes")
		}
		commands := make([]any, len(plan.Commands))
		for i, request := range plan.Commands {
			commands[i], err = request.toDomain(origin.HouseholdID, origin.Timezone)
			if err != nil {
				return batchInputError(i, err)
			}
		}
		payload, err := json.Marshal(plan.Commands)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(append([]byte("ledger_reconciliation\n"), payload...))
		version := plan.Version
		if !time.Now().Before(plan.ExpiresAt) {
			version = "expired"
		}
		previews, err := s.app.RecordChangesGuarded(ctx, commands, plan.ID, hex.EncodeToString(digest[:]), version)
		if err != nil {
			return err
		}
		result.Changes = make([]wire.ChangePreviewDTO, len(previews))
		for i, preview := range previews {
			result.Changes[i] = wire.FromChangePreview(preview)
		}
		return nil
	})
	return result, err
}
