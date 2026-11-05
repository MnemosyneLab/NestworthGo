package mcpserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// commitPlanRoute describes how one stored plan kind is committed. The receipt
// tool name is the historical per-kind commit name. It stays in the durable
// operation record, its hash and the host review binding so receipts written
// before the unified tool, and the replay and conflict rules that depend on
// them, keep their exact meaning.
type commitPlanRoute struct {
	receiptTool string
	// recoverable follows the original per-kind choice. Ledger and managed
	// product plans commit the business write and idempotency key together;
	// liquidity-policy writes do not, so they never replay a pending or unknown
	// receipt.
	recoverable bool
	run         func(*Service, context.Context, CommitChangeInput) (any, error)
	mapError    func(error) error
}

var commitPlanRoutes = map[string]commitPlanRoute{
	"change":                 {receiptTool: "commit_change", recoverable: true, run: (*Service).commitChange},
	"batch":                  {receiptTool: "commit_batch", recoverable: true, run: (*Service).commitBatch},
	"correction":             {receiptTool: "commit_correction", recoverable: true, run: (*Service).commitCorrection},
	"reconciliation":         {receiptTool: "commit_reconciliation", recoverable: true, run: (*Service).commitReconciliation},
	"product_operation":      {receiptTool: "commit_product_operation", recoverable: true, run: (*Service).commitProductOperation, mapError: productWriteError},
	"product_terms":          {receiptTool: "commit_product_terms", recoverable: true, run: (*Service).commitProductTerms},
	"product_valuation":      {receiptTool: "commit_product_valuation", recoverable: true, run: (*Service).commitProductValuation},
	"reconciliation_session": {receiptTool: "commit_reconciliation_session", recoverable: true, run: (*Service).commitReconciliationSession},
	"liquidity_policy_save": {receiptTool: "commit_save_liquidity_policy", run: func(s *Service, ctx context.Context, in CommitChangeInput) (any, error) {
		return s.commitLiquidityPolicy(ctx, in, false)
	}},
	"liquidity_policy_reset": {receiptTool: "commit_reset_liquidity_policy", run: func(s *Service, ctx context.Context, in CommitChangeInput) (any, error) {
		return s.commitLiquidityPolicy(ctx, in, true)
	}},
}

func (s *Service) commitPlanTool(server *toolSet) {
	closed := false
	registerTool(server, &agentcapability.Definition{Name: "commit_plan", Description: "Commit one stored preview plan of any kind: preview_change, _batch, _correction, _reconciliation, _product_operation, _product_terms, _product_valuation, _save_liquidity_policy, _reset_liquidity_policy or a reconciliation-session action. Input is operationId and input.planId only; the server reads the plan kind and the exact stored command, so the caller cannot change or mislabel it. A plan commits once: retrying the same operationId and planId returns the original receipt, and another operationId for a used or stale plan is rejected with stale_preview; request a new preview. The receipt tool names the plan kind and result is that kind's committed outcome. After an unknown outcome call get_operation, read current data, then retry the same operationId. Liquidity-policy receipts that are pending or unknown are never replayed; inspect them and preview again.", Effect: agentcapability.Write, Annotations: &agentcapability.Annotations{IdempotentHint: true, OpenWorldHint: &closed}}, func(ctx context.Context, in Mutation[CommitChangeInput]) (Response, error) {
		route, err := s.resolveCommitPlanRoute(ctx, in.OperationID, in.Input.PlanID)
		if err != nil {
			return Response{}, safeError(err)
		}
		value, err := s.executeWithRecovery(ctx, in.OperationID, route.receiptTool, in.Input, route.recoverable, func(ctx context.Context) (any, error) { return route.run(s, ctx, in.Input) })
		if err != nil {
			if route.mapError != nil {
				err = route.mapError(err)
			}
			return Response{}, safeError(err)
		}
		return Response{Data: value}, nil
	})
}

// resolveCommitPlanRoute derives the commit family from the stored plan. When
// the plan file is gone (expired and pruned) it falls back to the same durable
// evidence the per-kind tools used: the operation receipt, then the database
// receipts that make reconciliation commits recoverable.
func (s *Service) resolveCommitPlanRoute(ctx context.Context, operationID, planID string) (commitPlanRoute, error) {
	id, err := uuid.Parse(planID)
	if err != nil {
		return commitPlanRoute{}, fail("validation", "planId must be a UUID returned by a preview tool")
	}
	raw, err := s.readPrivate(filepath.Join(s.dir, "plans", id.String()+".json"))
	if err == nil {
		var header struct {
			Type string `json:"type"`
		}
		if err = jsoncodec.Unmarshal(raw, &header); err != nil {
			return commitPlanRoute{}, err
		}
		if header.Type == "" {
			header.Type = "change" // persisted stage-2 single plans
		}
		route, ok := commitPlanRoutes[header.Type]
		if !ok {
			return commitPlanRoute{}, fail("validation", "planId does not reference a committable preview plan")
		}
		return route, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return commitPlanRoute{}, err
	}
	if prior, readErr := s.GetOperation(operationID); readErr == nil {
		for _, route := range commitPlanRoutes {
			if route.receiptTool == prior.Tool {
				return route, nil
			}
		}
		return commitPlanRoute{}, fail("conflict", "operationId was already used with different arguments")
	}
	if receipt, lookupErr := s.app.ReconciliationAdjustmentReceipt(ctx, planID); lookupErr == nil && receipt != nil {
		return commitPlanRoutes["reconciliation"], nil
	}
	if receipt, lookupErr := s.app.ReconciliationSessionReceipt(ctx, planID); lookupErr == nil && receipt != nil {
		return commitPlanRoutes["reconciliation_session"], nil
	}
	return commitPlanRoute{}, fail("operation_outcome_unknown", "reviewed plan unavailable; inspect the operation receipt and current data; do not replace an unknown submission")
}

func (s *Service) commitReconciliationSession(ctx context.Context, in CommitChangeInput) (any, error) {
	raw, err := s.loadPreviewPlan(in.PlanID, "reconciliation_session")
	if err != nil {
		if safeError(err).Code != "operation_outcome_unknown" {
			return nil, err
		}
		receipt, lookupErr := s.app.ReconciliationSessionReceipt(ctx, in.PlanID)
		if lookupErr != nil {
			return nil, lookupErr
		}
		if receipt != nil {
			receipt.Accounts = nil
			return receipt, nil
		}
		return nil, err
	}
	var saved struct {
		Plan application.ReconciliationSessionPlan `json:"plan"`
	}
	if err = jsoncodec.Unmarshal(raw, &saved); err != nil {
		return nil, err
	}
	result, err := s.app.CommitReconciliationSession(ctx, saved.Plan)
	result.Accounts = nil
	return result, err
}
