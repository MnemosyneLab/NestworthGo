package reconciliation

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
	"time"
)

type productPlan struct {
	kind, id, token, state string
	terms                  application.UpdateProductTermsInput
	valuation              application.ProductValuationCommand
	operation              application.ProductCommand
	expires                time.Time
}
type ProductPreview struct {
	PlanID    string                                `json:"planId"`
	Before    *liquidity.ProductDetailDTO           `json:"before,omitempty"`
	After     *liquidity.ProductDetailDTO           `json:"after,omitempty"`
	Valuation *liquidity.ProductValuationPreviewDTO `json:"valuation,omitempty"`
	Operation *liquidity.ProductOperationPreviewDTO `json:"operation,omitempty"`
}

func (s *Service) PreviewProductTerms(ctx context.Context, in liquidity.UpdateProductTermsRequest) (ProductPreview, error) {
	id, e := domain.ParseProductContractID(in.ProductID)
	if e != nil {
		return ProductPreview{}, apierror.Wrap(e)
	}
	input := application.UpdateProductTermsInput{ReconciliationLink: in.ReconciliationLink, ProductID: id, ExpectedRevision: in.ExpectedRevision, Terms: in.Terms, Policy: in.Policy}
	p, token, e := s.app.PreviewProductTermsGuarded(ctx, input)
	if e != nil {
		return ProductPreview{}, apierror.Wrap(e)
	}
	before, after := liquidity.FromProductDetail(p.Before), liquidity.FromProductDetail(p.After)
	plan := productPlan{kind: "terms", id: uuid.NewString(), terms: input, token: token, state: p.ReviewedStateHash, expires: time.Now().Add(10 * time.Minute)}
	return ProductPreview{PlanID: s.products.Put(plan), Before: &before, After: &after}, nil
}
func (s *Service) PreviewProductValuation(ctx context.Context, link domain.ReconciliationLink, productID, amount, observedAt string) (ProductPreview, error) {
	id, e := domain.ParseProductContractID(productID)
	if e != nil {
		return ProductPreview{}, apierror.Wrap(e)
	}
	p, token, e := s.app.PreviewProductValuationGuarded(ctx, application.ProductValuationCommand{ReconciliationLink: &link, ProductID: id, Amount: amount, ObservedAt: observedAt})
	if e != nil {
		return ProductPreview{}, apierror.Wrap(e)
	}
	value := liquidity.FromProductValuationPreview(p)
	plan := productPlan{kind: "valuation", id: uuid.NewString(), valuation: p.Command, token: token, state: p.ReviewedStateHash, expires: time.Now().Add(10 * time.Minute)}
	return ProductPreview{PlanID: s.products.Put(plan), Valuation: &value}, nil
}
func (s *Service) PreviewProductOperation(ctx context.Context, in liquidity.ProductCommandRequest) (ProductPreview, error) {
	command, e := in.ToApplication()
	if e != nil {
		return ProductPreview{}, apierror.Wrap(e)
	}
	p, token, e := s.app.PreviewProductOperationGuarded(ctx, command)
	if e != nil {
		return ProductPreview{}, apierror.Wrap(e)
	}
	// NormalizedJSON contains the frozen resolved effective time and exact command.
	var frozen application.ProductCommand
	e = json.Unmarshal([]byte(p.NormalizedJSON), &frozen)
	if e != nil {
		return ProductPreview{}, apierror.Wrap(e)
	}
	value := liquidity.FromProductOperationPreview(p)
	plan := productPlan{kind: "operation", id: uuid.NewString(), operation: frozen, token: token, state: p.ReviewedStateHash, expires: time.Now().Add(10 * time.Minute)}
	return ProductPreview{PlanID: s.products.Put(plan), Operation: &value}, nil
}
func (s *Service) CommitProduct(ctx context.Context, id string) error {
	p, e := s.products.Get(id)
	if e != nil {
		return apierror.Wrap(e)
	}
	token := p.token
	if time.Now().After(p.expires) {
		token = "expired"
	}
	switch p.kind {
	case "terms":
		_, e = s.app.CommitProductTermsGuarded(ctx, p.terms, p.id, p.state, token)
	case "valuation":
		_, e = s.app.CommitProductValuationGuarded(ctx, p.valuation, p.id, p.state, token)
	case "operation":
		_, e = s.app.RecordProductOperationGuarded(ctx, p.operation, p.id, p.state, token)
	}
	return apierror.Wrap(e)
}
