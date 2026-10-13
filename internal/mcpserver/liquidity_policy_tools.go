package mcpserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
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

func (s *Service) liquidityPolicyTools(server *mcp.Server) {
	readTool(server, "preview_save_liquidity_policy", "Preview creation or full replacement of an ordinary source's GUI access/withdrawal rules. Requires ledger_write. Copy sourceRef from get_liquidity_overview (account_value, account_cash with currency, or holding with holdingId); never use instrument identity. expectedRevision is 0 for assumed defaults, otherwise the explicit policy revision. Supply the complete policy: accessKind on_request/on_date/unknown/excluded; unlockOn, settlementDays (0-365), dayBasis calendar/weekdays, receiptOnOverride, accessibleAmountCap, normalExitFee, earlyKind allowed/not_allowed/unknown, earlySettlementDays, earlyDayBasis, earlyFee, earlyAmountMode current_value/fixed_gross, earlyGrossAmount and note. Nullable fields clear with null; omission never preserves a previous value. Money is exact decimal strings in source currency; zero fee is known, null fee is unknown. Civil dates use YYYY-MM-DD. Returns effective before/after policies; metadata is provisional. Only active asset sources; managed policies require preview_product_terms. No cash, ledger or reservation effects. Inspect then commit_save_liquidity_policy within ten minutes.", func(ctx context.Context, in liquidity.SavePolicyRequest) (any, error) {
		return s.previewLiquidityPolicy(ctx, in, false)
	})
	readTool(server, "preview_reset_liquidity_policy", "Preview removal of one explicit ordinary source policy and restoration of the GUI's assumed defaults. Requires ledger_write. Copy sourceRef and the explicit expectedRevision from get_liquidity_overview. Returns effective before/after policies and origins; after revision is zero. Managed policies, archived sources and liabilities are rejected. No ledger or reservation effects. Inspect then commit_reset_liquidity_policy within ten minutes; reset does not write a replacement default policy.", func(ctx context.Context, in liquidity.ResetPolicyRequest) (any, error) {
		return s.previewLiquidityPolicy(ctx, liquidity.SavePolicyRequest{Source: in.Source, ExpectedRevision: in.ExpectedRevision}, true)
	})
	for _, entry := range []struct {
		tool  mcp.Tool
		reset bool
	}{
		{tool: mcp.Tool{Name: "commit_save_liquidity_policy"}, reset: false},
		{tool: mcp.Tool{Name: "commit_reset_liquidity_policy"}, reset: true},
	} {
		name, reset := entry.tool.Name, entry.reset
		closed := false
		mcp.AddTool(server, &mcp.Tool{Name: name, Description: "Commit exactly the matching stored ordinary liquidity policy preview. Requires ledger_write; input is only operationId and input.planId. CAS and preview version are checked under the shared application write permit. Successful same-operation retries return the original receipt; changed input conflicts. Unlike managed product atomic receipts, pending/unknown policy receipts are never replayed automatically: inspect get_operation and get_liquidity_overview before a new intentional preview. A used plan with a new operationId is stale, never a second write. Expired, changed, restarted or restored uncommitted plans require a new preview. Reset result is a revision-zero acknowledgement; read the overview for effective defaults. No ledger or reservation mutation.", Annotations: &mcp.ToolAnnotations{IdempotentHint: true, OpenWorldHint: &closed}}, func(ctx context.Context, _ *mcp.CallToolRequest, in Mutation[CommitChangeInput]) (*mcp.CallToolResult, Response, error) {
			value, err := s.execute(ctx, in.OperationID, name, in.Input, func(ctx context.Context) (any, error) { return s.commitLiquidityPolicy(ctx, in.Input, reset) })
			if err != nil {
				return nil, Response{}, safeError(err)
			}
			return nil, Response{Data: value}, nil
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
	if err := json.Unmarshal(raw, &plan); err != nil {
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
