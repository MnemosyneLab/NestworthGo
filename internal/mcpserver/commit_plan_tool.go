package mcpserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// CommitPlanRequest is the commit_plan envelope. operationId is optional only
// here; every other write still uses Mutation, which requires it. planId stays
// the atomic idempotency key. An omitted or blank operationId is replaced by
// derivedCommitPlanOperationID before any receipt is read or written. A
// non-blank value is explicit and follows the existing UUID, retry, conflict
// and liquidity rules unchanged.
type CommitPlanRequest struct {
	OperationID string            `json:"operationId,omitempty" jsonschema:"Optional. Omit to derive a UUID v5 from the canonical planId. An explicit UUID is stored and retried unchanged."`
	Input       CommitChangeInput `json:"input"`
}

// commitPlanOperationNamespace is the UUID v5 namespace for operation IDs
// derived when commit_plan omits operationId. It is itself UUID v5 of the name
// "nestworth:mcp:commit_plan:operation" in the RFC 4122 URL namespace. The
// derived id hashes the canonical lowercase planId under this namespace.
// Changing either value would mint a new receipt identity for the same omitted
// retry, so both are part of the durable contract.
var commitPlanOperationNamespace = uuid.NewSHA1(uuid.NameSpaceURL, []byte("nestworth:mcp:commit_plan:operation"))

func derivedCommitPlanOperationID(planID string) (string, error) {
	id, err := uuid.Parse(planID)
	if err != nil {
		return "", fail("validation", "planId must be a UUID returned by a preview tool")
	}
	return uuid.NewSHA1(commitPlanOperationNamespace, []byte(id.String())).String(), nil
}

func commitPlanOperationID(operationID, planID string) (string, error) {
	if strings.TrimSpace(operationID) != "" {
		return operationID, nil
	}
	return derivedCommitPlanOperationID(planID)
}

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
	registerTool(server, &agentcapability.Definition{
		Name: "commit_plan",
		Description: "Commit a stored preview with input.planId. operationId is optional only on this tool: omit it and the server derives UUID v5 from the canonical planId under namespace f1f65af0-bbb8-5d57-af9c-b624de715dcb (lowercase hyphenated plan UUID as name). An explicit operationId is stored and retried unchanged. planId is the atomic idempotency key. " +
			"The same operationId and input return the saved receipt; a different input conflicts and does not post, including when a derived id already belongs to another explicit call or tool. The receipt tool names the plan kind. " +
			"Ledger, batch, correction and managed-product plans deduplicate by planId and can recover with another operationId after expiry. Reconciliation can recover from its database receipt if the plan file is gone. " +
			"Uncommitted stale or expired previews are rejected. Liquidity save/reset do not replay: a different explicit operationId on a used plan returns stale_preview, and pending or unknown receipts are not replayed. Missing plans without evidence return operation_outcome_unknown. " +
			"If unclear, locally derive the omitted ID without resubmitting, call get_operation with that ID and read current data before a fresh preview. Unknown outcomes do not authorize blind retries.",
		Effect: agentcapability.Write, Annotations: &agentcapability.Annotations{IdempotentHint: true, OpenWorldHint: &closed},
	}, func(ctx context.Context, in CommitPlanRequest) (Response, error) {
		operationID, err := commitPlanOperationID(in.OperationID, in.Input.PlanID)
		if err != nil {
			return Response{}, safeError(err)
		}
		route, err := s.resolveCommitPlanRoute(ctx, operationID, in.Input.PlanID)
		if err != nil {
			return Response{}, safeError(err)
		}
		value, err := s.executeWithRecovery(ctx, operationID, route.receiptTool, in.Input, route.recoverable, func(ctx context.Context) (any, error) { return route.run(s, ctx, in.Input) })
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
