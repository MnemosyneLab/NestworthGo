package mcpserver

import (
	"context"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

type liquidityPolicyPlan struct {
	Type      string                      `json:"type"`
	ID        string                      `json:"id"`
	Input     liquidity.SavePolicyRequest `json:"input"`
	Version   string                      `json:"version"`
	ExpiresAt time.Time                   `json:"expiresAt"`
}

type LiquidityPolicyPlanPreview struct {
	PlanID       string              `json:"planId"`
	ExpiresAt    string              `json:"expiresAt"`
	Before       liquidity.PolicyDTO `json:"before"`
	After        liquidity.PolicyDTO `json:"after"`
	BeforeOrigin string              `json:"beforeOrigin"`
	AfterOrigin  string              `json:"afterOrigin"`
}

func (s *Service) liquidityPolicyTools(server *toolSet) {
	readTool(server, "preview_save_liquidity_policy", "Preview creating or fully replacing an ordinary source's access and withdrawal rule. Copy sourceRef from get_liquidity_overview (account_value, account_cash with currency, or holding with holdingId); expectedRevision is 0 for assumed defaults, otherwise the explicit revision. Supply the complete policy: accessKind (on_request, on_date, unknown, excluded), unlockOn, settlementDays (0-365), dayBasis, receiptOnOverride, accessibleAmountCap, normalExitFee, earlyKind, earlySettlementDays, earlyDayBasis, earlyFee, earlyAmountMode, earlyGrossAmount and note. Null clears a field; omission never keeps the old value. Money is an exact decimal string in source currency (zero fee is known, null is unknown); dates are YYYY-MM-DD. Active asset sources only; managed policies use preview_product_terms. Commit with commit_save_liquidity_policy.", func(ctx context.Context, in liquidity.SavePolicyRequest) (any, error) {
		return s.previewLiquidityPolicy(ctx, in, false)
	})
	readTool(server, "preview_reset_liquidity_policy", "Preview removing one explicit ordinary source policy to restore the GUI's assumed defaults. Copy sourceRef and the explicit expectedRevision from get_liquidity_overview; the after revision is 0. Managed policies, archived sources and liabilities are rejected. Commit with commit_reset_liquidity_policy.", func(ctx context.Context, in liquidity.ResetPolicyRequest) (any, error) {
		return s.previewLiquidityPolicy(ctx, liquidity.SavePolicyRequest{Source: in.Source, ExpectedRevision: in.ExpectedRevision}, true)
	})
	for _, entry := range []struct {
		tool  agentcapability.Definition
		reset bool
	}{
		{tool: agentcapability.Definition{Name: "commit_save_liquidity_policy"}, reset: false},
		{tool: agentcapability.Definition{Name: "commit_reset_liquidity_policy"}, reset: true},
	} {
		name, reset := entry.tool.Name, entry.reset
		closed := false
		registerTool(server, &agentcapability.Definition{Name: name, Description: "Commit the stored liquidity-policy preview of the matching kind (operationId and input.planId only). The revision and plan version are checked under the shared write permit; a used plan with a new operationId is stale, never a second write. Unlike managed-product receipts, a pending or unknown receipt is never replayed: inspect get_operation and get_liquidity_overview, then preview again. Reset returns a revision-zero acknowledgement; read the overview for effective defaults.", Effect: agentcapability.Write, Annotations: &agentcapability.Annotations{IdempotentHint: true, OpenWorldHint: &closed}}, func(ctx context.Context, in Mutation[CommitChangeInput]) (Response, error) {
			value, err := s.execute(ctx, in.OperationID, name, in.Input, func(ctx context.Context) (any, error) { return s.commitLiquidityPolicy(ctx, in.Input, reset) })
			if err != nil {
				return Response{}, safeError(err)
			}
			return Response{Data: value}, nil
		})
	}
}

func policyInput(in liquidity.SavePolicyRequest) (application.SavePolicyInput, error) {
	ref, err := in.Source.ToDomain()
	return application.SavePolicyInput{Source: ref, ExpectedRevision: in.ExpectedRevision, Policy: in.Policy}, err
}

func (s *Service) previewLiquidityPolicy(ctx context.Context, in liquidity.SavePolicyRequest, reset bool) (any, error) {
	input, err := policyInput(in)
	if err != nil {
		return nil, err
	}
	preview, version, err := s.app.PreviewLiquidityPolicyGuarded(ctx, input, reset)
	if err != nil {
		return nil, err
	}
	kind := "liquidity_policy_save"
	if reset {
		kind = "liquidity_policy_reset"
	}
	plan := liquidityPolicyPlan{Type: kind, ID: uuid.NewString(), Input: in, Version: version, ExpiresAt: time.Now().UTC().Add(planLifetime)}
	if err := s.writePrivate(filepath.Join(s.dir, "plans", plan.ID+".json"), plan); err != nil {
		return nil, err
	}
	return LiquidityPolicyPlanPreview{PlanID: plan.ID, ExpiresAt: plan.ExpiresAt.Format(time.RFC3339Nano), Before: liquidity.FromPolicy(preview.Before), After: liquidity.FromPolicy(preview.After), BeforeOrigin: string(preview.BeforeOrigin), AfterOrigin: string(preview.AfterOrigin)}, nil
}

func (s *Service) commitLiquidityPolicy(ctx context.Context, in CommitChangeInput, reset bool) (any, error) {
	kind := "liquidity_policy_save"
	if reset {
		kind = "liquidity_policy_reset"
	}
	raw, err := s.loadPreviewPlan(in.PlanID, kind)
	if err != nil {
		return nil, err
	}
	var plan liquidityPolicyPlan
	if err := jsoncodec.Unmarshal(raw, &plan); err != nil {
		return nil, err
	}
	input, err := policyInput(plan.Input)
	if err != nil {
		return nil, err
	}
	var result liquidity.PolicyDTO
	err = s.app.WithWrite(ctx, func(ctx context.Context) error {
		version := plan.Version
		if !time.Now().Before(plan.ExpiresAt) {
			version = "expired"
		}
		policy, err := s.app.CommitLiquidityPolicyGuarded(ctx, input, reset, version)
		if err == nil {
			result = liquidity.FromPolicy(policy)
		}
		return err
	})
	return result, err
}
