package mcpserver

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

// These two payloads deliberately omit reservation releases and the
// server-owned recording time. SDK additionalProperties rejects either field.
type RecordExistingProductInput struct {
	AccountID           string                         `json:"accountId"`
	Currency            string                         `json:"currency"`
	Principal           string                         `json:"principal"`
	TotalCostBasis      string                         `json:"totalCostBasis"`
	CurrentValue        string                         `json:"currentValue"`
	CashExcludesProduct bool                           `json:"cashExcludesProduct"`
	Terms               application.ProductTermsInput  `json:"terms"`
	Policy              application.ProductPolicyInput `json:"policy"`
}

type SettleProductInput struct {
	EffectiveLocalDate string  `json:"effectiveLocalDate,omitempty"`
	EffectiveLocalTime string  `json:"effectiveLocalTime,omitempty"`
	ProductID          string  `json:"productId"`
	ReturnedPrincipal  *string `json:"returnedPrincipal"`
	Interest           *string `json:"interest"`
	GrossProceeds      *string `json:"grossProceeds"`
	Fee                *string `json:"fee"`
	EffectiveAt        string  `json:"effectiveAt"`
}

type RenewProductInput struct {
	Settle     SettleProductInput             `json:"settle"`
	Principal  string                         `json:"principal"`
	OpeningFee *string                        `json:"openingFee"`
	Terms      application.ProductTermsInput  `json:"terms"`
	Policy     application.ProductPolicyInput `json:"policy"`
}

type ProductOperationInput struct {
	Renew              *RenewProductInput                  `json:"renew,omitempty"`
	ReconciliationLink *domain.ReconciliationLink          `json:"reconciliationLink,omitempty"`
	Kind               string                              `json:"kind"`
	Open               *application.OpenProductCommand     `json:"open,omitempty"`
	RecordExisting     *RecordExistingProductInput         `json:"recordExisting,omitempty"`
	ReceiveInterest    *application.ReceiveInterestCommand `json:"receiveInterest,omitempty"`
	Settle             *SettleProductInput                 `json:"settle,omitempty"`
	Undo               *application.UndoProductCommand     `json:"undo,omitempty"`
}

type productOperationPlan struct {
	Type              string                     `json:"type"`
	ID                string                     `json:"id"`
	Command           application.ProductCommand `json:"command"`
	ReviewedStateHash string                     `json:"reviewedStateHash"`
	Version           string                     `json:"version"`
	ExpiresAt         time.Time                  `json:"expiresAt"`
}

type ProductOperationPlanPreview struct {
	PlanID    string                               `json:"planId"`
	ExpiresAt string                               `json:"expiresAt"`
	Preview   liquidity.ProductOperationPreviewDTO `json:"preview"`
}

func (in ProductOperationInput) command() (application.ProductCommand, error) {
	r := liquidity.ProductCommandRequest{ReconciliationLink: in.ReconciliationLink, Kind: in.Kind, Open: in.Open, ReceiveInterest: in.ReceiveInterest, Undo: in.Undo}
	if in.RecordExisting != nil {
		v := in.RecordExisting
		r.RecordExisting = &application.RecordExistingProductCommand{AccountID: v.AccountID, Currency: v.Currency, Principal: v.Principal, TotalCostBasis: v.TotalCostBasis, CurrentValue: v.CurrentValue, CashExcludesProduct: v.CashExcludesProduct, Terms: v.Terms, Policy: v.Policy}
	}
	if in.Settle != nil {
		v := in.Settle
		r.Settle = &application.SettleProductCommand{ProductID: v.ProductID, ReturnedPrincipal: v.ReturnedPrincipal, Interest: v.Interest, GrossProceeds: v.GrossProceeds, Fee: v.Fee, EffectiveAt: v.EffectiveAt, EffectiveLocalDate: v.EffectiveLocalDate, EffectiveLocalTime: v.EffectiveLocalTime}
	}
	if in.Renew != nil {
		v := in.Renew
		s := v.Settle
		r.Renew = &application.RenewProductCommand{Settle: application.SettleProductCommand{ProductID: s.ProductID, ReturnedPrincipal: s.ReturnedPrincipal, Interest: s.Interest, GrossProceeds: s.GrossProceeds, Fee: s.Fee, EffectiveAt: s.EffectiveAt, EffectiveLocalDate: s.EffectiveLocalDate, EffectiveLocalTime: s.EffectiveLocalTime}, Principal: v.Principal, OpeningFee: v.OpeningFee, Terms: v.Terms, Policy: v.Policy}
	}
	return r.ToApplication()
}

func productWriteError(err error) error {
	var de *domain.Error
	if errors.As(err, &de) && de.Code == domain.ErrUnresolvedReservationRelease {
		return fail(string(domain.ErrUnresolvedReservationRelease), "reservations require GUI handling; this tool never releases or restores them")
	}
	return err
}

func (s *Service) productOperationTools(server *toolSet) {
	readTool(server, "preview_product_operation", "Preview one managed-product lifecycle fact: open, record_existing, receive_interest, whole-contract settle, atomic renewal (no reservation changes) or the latest safe operation-group undo. It records completed facts, never bank orders or automatic maturity cash. Supply exactly one matching payload; money is decimal strings in the original currency. record_existing needs the original totalCostBasis and currentValue and cashExcludesProduct=true. effectiveAt may be empty (frozen now), one timestamp, or a local date and time in the history timezone; startOn is descriptive. Settle closes the whole contract, and actual early receipts are allowed. Reservations are never released or restored; use the GUI if they constrain the operation. Inspect normalizedJSON and effects, then commit_plan.", s.previewProductOperation, productOperationInputSchema())
}

func (s *Service) previewProductOperation(ctx context.Context, in ProductOperationInput) (any, error) {
	command, err := in.command()
	if err != nil {
		return nil, err
	}
	preview, version, err := s.app.PreviewProductOperationGuarded(ctx, command)
	if err != nil {
		return nil, productWriteError(err)
	}
	var normalized application.ProductCommand
	if err := jsoncodec.Unmarshal([]byte(preview.NormalizedJSON), &normalized); err != nil {
		return nil, err
	}
	plan := productOperationPlan{Type: "product_operation", ID: domain.NewProductOperationID().String(), Command: normalized, ReviewedStateHash: preview.ReviewedStateHash, Version: version, ExpiresAt: time.Now().UTC().Add(planLifetime)}
	if err := s.writePrivate(filepath.Join(s.dir, "plans", plan.ID+".json"), plan); err != nil {
		return nil, err
	}
	return ProductOperationPlanPreview{PlanID: plan.ID, ExpiresAt: plan.ExpiresAt.Format(time.RFC3339Nano), Preview: liquidity.FromProductOperationPreview(preview)}, nil
}

func (s *Service) commitProductOperation(ctx context.Context, in CommitChangeInput) (any, error) {
	raw, err := s.loadPreviewPlan(in.PlanID, "product_operation")
	if err != nil {
		return nil, err
	}
	var plan productOperationPlan
	if err := jsoncodec.Unmarshal(raw, &plan); err != nil {
		return nil, err
	}
	var receipt application.ProductOperationReceipt
	err = s.app.WithWrite(ctx, func(ctx context.Context) error {
		version := plan.Version
		if !time.Now().Before(plan.ExpiresAt) {
			version = "expired"
		}
		var err error
		receipt, err = s.app.RecordProductOperationGuarded(ctx, plan.Command, plan.ID, plan.ReviewedStateHash, version)
		return productWriteError(err)
	})
	if err != nil {
		return nil, err
	}
	return liquidity.FromProductOperationReceipt(receipt), nil
}
