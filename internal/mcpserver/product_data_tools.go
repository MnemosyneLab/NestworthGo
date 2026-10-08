package mcpserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type ProductValuationInput struct {
	ProductID  string `json:"productId"`
	Amount     string `json:"amount"`
	ObservedAt string `json:"observedAt" jsonschema:"Actual RFC3339 observation timestamp; empty freezes App now at preview"`
}

type productTermsPlan struct {
	Type              string                              `json:"type"`
	ID                string                              `json:"id"`
	Input             application.UpdateProductTermsInput `json:"input"`
	ReviewedStateHash string                              `json:"reviewedStateHash"`
	Version           string                              `json:"version"`
	ExpiresAt         time.Time                           `json:"expiresAt"`
}

type productValuationPlan struct {
	Type              string                              `json:"type"`
	ID                string                              `json:"id"`
	Command           application.ProductValuationCommand `json:"command"`
	ReviewedStateHash string                              `json:"reviewedStateHash"`
	Version           string                              `json:"version"`
	ExpiresAt         time.Time                           `json:"expiresAt"`
}

type ProductTermsPlanPreview struct {
	PlanID            string               `json:"planId"`
	ExpiresAt         string               `json:"expiresAt"`
	Before            liquidity.ProductDTO `json:"before"`
	After             liquidity.ProductDTO `json:"after"`
	NetWorthDelta     wire.SignedMoneyView `json:"netWorthDelta"`
	ReviewedStateHash string               `json:"reviewedStateHash"`
}

type ProductValuationPlanPreview struct {
	PlanID    string                               `json:"planId"`
	ExpiresAt string                               `json:"expiresAt"`
	Preview   liquidity.ProductValuationPreviewDTO `json:"preview"`
}

func (s *Service) productDataTools(server *mcp.Server) {
	readTool(server, "preview_product_terms", "Preview a complete GUI terms/policy edit for one managed product. Requires ledger_write. productId and expectedRevision must match get_product; supply full terms/policy, including nullable unknowns, with decimal strings in product currency and fractional annualRate or annualRatePercent. Kind/startOn are immutable; closed contracts only accept name/note while all financial fields/policy stay unchanged. Shows exact before/after revisions, terms, forecast policy and unchanged current value; cash/quantity/cost/quotes/net worth are unchanged. Preview timestamps are provisional metadata. Reservations remain unchanged; forecasts may change and never record actual interest. Inspect the plan then commit_product_terms within ten minutes. Generic directory writes cannot edit financial terms.", s.previewProductTerms)
	readTool(server, "preview_product_valuation", "Preview one actual total-value observation for an open locked_product. Requires ledger_write. Original-currency amount is a positive decimal string; observedAt is the actual RFC3339 timestamp or empty for frozen App now. No NAV units, projected interest, provider pulls or generic managed quote imports. term_deposit is rejected. Shows the normalized frozen command and current value before/after using the portfolio quote authority: a historical observation may leave today's value unchanged. Missing current value remains unknown. No cash/quantity/cost or reservation changes. Historical observation may dirty derived analysis. Inspect then commit_product_valuation within ten minutes; draft effects are provisional.", s.previewProductValuation)
	closed := false
	mcp.AddTool(server, &mcp.Tool{Name: "commit_product_terms", Description: "Commit exactly a stored preview_product_terms plan. Requires ledger_write; only operationId and input.planId. Contract/policy CAS revisions and immutable business receipt share one SQLite transaction. Retry identical operation/input after interruption; same plan/new operationId returns original result even after expiry/restart. Uncommitted stale/expired/restarted/restored plans need a fresh preview. Returned product is the recorded-time result; read get_product separately for current value and complete active reservations. Never releases/restores reserves.", Annotations: &mcp.ToolAnnotations{IdempotentHint: true, OpenWorldHint: &closed}}, func(ctx context.Context, _ *mcp.CallToolRequest, in Mutation[CommitChangeInput]) (*mcp.CallToolResult, Response, error) {
		value, err := s.executeWithRecovery(ctx, in.OperationID, "commit_product_terms", in.Input, true, func(ctx context.Context) (any, error) { return s.commitProductTerms(ctx, in.Input) })
		if err != nil {
			return nil, Response{}, safeError(err)
		}
		return nil, Response{Data: value}, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "commit_product_valuation", Description: "Commit exactly a stored preview_product_valuation plan. Requires ledger_write; only operationId and input.planId. Quote, value_observation operation and immutable receipt commit atomically; no cash/quantity/cost or reservation changes. Matching retries and same plan/new operationId return original quote/operation/value/time, including after expiry/restart, never today's live product. Read get_product separately; derived analysis may remain dirty. Uncommitted stale/expired/restarted/restored plans need a fresh preview.", Annotations: &mcp.ToolAnnotations{IdempotentHint: true, OpenWorldHint: &closed}}, func(ctx context.Context, _ *mcp.CallToolRequest, in Mutation[CommitChangeInput]) (*mcp.CallToolResult, Response, error) {
		value, err := s.executeWithRecovery(ctx, in.OperationID, "commit_product_valuation", in.Input, true, func(ctx context.Context) (any, error) { return s.commitProductValuation(ctx, in.Input) })
		if err != nil {
			return nil, Response{}, safeError(err)
		}
		return nil, Response{Data: value}, nil
	})
}

func (s *Service) previewProductTerms(ctx context.Context, in liquidity.UpdateProductTermsRequest) (any, error) {
	id, err := domain.ParseProductContractID(in.ProductID)
	if err != nil {
		return nil, err
	}
	input := application.UpdateProductTermsInput{ProductID: id, ExpectedRevision: in.ExpectedRevision, Terms: in.Terms, Policy: in.Policy}
	preview, version, err := s.app.PreviewProductTermsGuarded(ctx, input)
	if err != nil {
		return nil, err
	}
	plan := productTermsPlan{Type: "product_terms", ID: domain.NewProductOperationID().String(), Input: input, ReviewedStateHash: preview.ReviewedStateHash, Version: version, ExpiresAt: time.Now().UTC().Add(planLifetime)}
	if err := s.writePrivate(filepath.Join(s.dir, "plans", plan.ID+".json"), plan); err != nil {
		return nil, err
	}
	zero, err := domain.ParseSignedMoney("0", preview.After.Contract.Currency)
	if err != nil {
		return nil, err
	}
	return ProductTermsPlanPreview{PlanID: plan.ID, ExpiresAt: plan.ExpiresAt.Format(time.RFC3339Nano), Before: liquidity.FromProductDetail(preview.Before).Product, After: liquidity.FromProductDetail(preview.After).Product, NetWorthDelta: wire.FromSignedMoney(zero), ReviewedStateHash: preview.ReviewedStateHash}, nil
}

func (s *Service) commitProductTerms(ctx context.Context, in CommitChangeInput) (any, error) {
	raw, err := s.loadPreviewPlan(in.PlanID, "product_terms")
	if err != nil {
		return nil, err
	}
	var plan productTermsPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return nil, err
	}
	var receipt domain.ProductTermsReceipt
	err = s.app.WithWrite(ctx, func(ctx context.Context) error {
		version := plan.Version
		if !time.Now().Before(plan.ExpiresAt) {
			version = "expired"
		}
		var err error
		receipt, err = s.app.CommitProductTermsGuarded(ctx, plan.Input, plan.ID, plan.ReviewedStateHash, version)
		return err
	})
	if err != nil {
		return nil, err
	}
	return liquidity.FromProductTermsReceipt(receipt), nil
}

func (s *Service) previewProductValuation(ctx context.Context, in ProductValuationInput) (any, error) {
	id, err := domain.ParseProductContractID(in.ProductID)
	if err != nil {
		return nil, err
	}
	preview, version, err := s.app.PreviewProductValuationGuarded(ctx, application.ProductValuationCommand{ProductID: id, Amount: in.Amount, ObservedAt: in.ObservedAt})
	if err != nil {
		return nil, err
	}
	plan := productValuationPlan{Type: "product_valuation", ID: domain.NewProductOperationID().String(), Command: preview.Command, ReviewedStateHash: preview.ReviewedStateHash, Version: version, ExpiresAt: time.Now().UTC().Add(planLifetime)}
	if err := s.writePrivate(filepath.Join(s.dir, "plans", plan.ID+".json"), plan); err != nil {
		return nil, err
	}
	return ProductValuationPlanPreview{PlanID: plan.ID, ExpiresAt: plan.ExpiresAt.Format(time.RFC3339Nano), Preview: liquidity.FromProductValuationPreview(preview)}, nil
}

func (s *Service) commitProductValuation(ctx context.Context, in CommitChangeInput) (any, error) {
	raw, err := s.loadPreviewPlan(in.PlanID, "product_valuation")
	if err != nil {
		return nil, err
	}
	var plan productValuationPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return nil, err
	}
	var receipt application.ProductValuationReceipt
	err = s.app.WithWrite(ctx, func(ctx context.Context) error {
		version := plan.Version
		if !time.Now().Before(plan.ExpiresAt) {
			version = "expired"
		}
		var err error
		receipt, err = s.app.CommitProductValuationGuarded(ctx, plan.Command, plan.ID, plan.ReviewedStateHash, version)
		return err
	})
	if err != nil {
		return nil, err
	}
	return liquidity.FromProductValuationReceipt(receipt), nil
}
