package mcpserver

import (
	"context"
	"path/filepath"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type ProductValuationInput struct {
	ReconciliationLink *domain.ReconciliationLink `json:"reconciliationLink,omitempty"`
	ProductID          string                     `json:"productId"`
	Amount             string                     `json:"amount"`
	ObservedAt         string                     `json:"observedAt" jsonschema:"Actual RFC3339 observation timestamp; preview freezes UTC milliseconds, truncating submillisecond precision; empty uses App now"`
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

func (s *Service) productDataTools(server *toolSet) {
	readTool(server, "preview_product_terms", "Preview a complete terms and policy edit for one managed product. productId and expectedRevision must match get_products with id; supply the full terms and policy (unknowns as null, decimal strings in product currency, annualRate as a fraction or annualRatePercent: prefer one, a nonempty percent wins). Check the resolved annualRate in after. terms.shortName is the only optional field: omitted or null keeps the stored value and an empty string clears it. kind and startOn are immutable; closed contracts accept only name and note. Cash, quantity, cost, quotes, net worth and reservations are unchanged, and forecasts never record actual interest. Commit with commit_plan; generic directory tools cannot edit financial terms.", s.previewProductTerms)
	readTool(server, "preview_product_valuation", "Preview one actual total-value observation for an open locked_product (term_deposit is rejected): a positive original-currency decimal amount and observedAt as RFC3339 or empty for frozen now (submillisecond precision is truncated). No NAV units, projected interest, provider pulls or generic quote imports. The preview shows the frozen command and the current value before and after under the portfolio quote authority, so a historical observation may leave today's value unchanged. Cash, quantity, cost and reservations are unchanged. Commit with commit_plan.", s.previewProductValuation)
}

func (s *Service) previewProductTerms(ctx context.Context, in liquidity.UpdateProductTermsRequest) (any, error) {
	id, err := domain.ParseProductContractID(in.ProductID)
	if err != nil {
		return nil, err
	}
	input := application.UpdateProductTermsInput{ReconciliationLink: in.ReconciliationLink, ProductID: id, ExpectedRevision: in.ExpectedRevision, Terms: in.Terms, Policy: in.Policy}
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
	if err := jsoncodec.Unmarshal(raw, &plan); err != nil {
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
	preview, version, err := s.app.PreviewProductValuationGuarded(ctx, application.ProductValuationCommand{ReconciliationLink: in.ReconciliationLink, ProductID: id, Amount: in.Amount, ObservedAt: in.ObservedAt})
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
	if err := jsoncodec.Unmarshal(raw, &plan); err != nil {
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
