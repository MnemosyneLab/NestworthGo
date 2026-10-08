package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
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

type ProductOperationInput struct {
	Kind            string                              `json:"kind" jsonschema:"open, record_existing, receive_interest, settle or undo; exactly one matching payload"`
	Open            *application.OpenProductCommand     `json:"open,omitempty"`
	RecordExisting  *RecordExistingProductInput         `json:"recordExisting,omitempty"`
	ReceiveInterest *application.ReceiveInterestCommand `json:"receiveInterest,omitempty"`
	Settle          *SettleProductInput                 `json:"settle,omitempty"`
	Undo            *application.UndoProductCommand     `json:"undo,omitempty"`
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
	r := liquidity.ProductCommandRequest{Kind: in.Kind, Open: in.Open, ReceiveInterest: in.ReceiveInterest, Undo: in.Undo}
	if in.RecordExisting != nil {
		v := in.RecordExisting
		r.RecordExisting = &application.RecordExistingProductCommand{AccountID: v.AccountID, Currency: v.Currency, Principal: v.Principal, TotalCostBasis: v.TotalCostBasis, CurrentValue: v.CurrentValue, CashExcludesProduct: v.CashExcludesProduct, Terms: v.Terms, Policy: v.Policy}
	}
	if in.Settle != nil {
		v := in.Settle
		r.Settle = &application.SettleProductCommand{ProductID: v.ProductID, ReturnedPrincipal: v.ReturnedPrincipal, Interest: v.Interest, GrossProceeds: v.GrossProceeds, Fee: v.Fee, EffectiveAt: v.EffectiveAt, EffectiveLocalDate: v.EffectiveLocalDate, EffectiveLocalTime: v.EffectiveLocalTime}
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

func (s *Service) productOperationTools(server *mcp.Server) {
	readTool(server, "preview_product_operation", "Preview exactly one managed product lifecycle fact: open, record_existing, receive_interest, whole-contract settle, or latest safe operation-group undo. Requires ledger_write. This records facts already completed, never bank orders or automatic maturity cash. Exactly one matching payload. Money is original-currency decimal strings. record_existing requires original totalCostBasis/currentValue and cashExcludesProduct=true (account cash already excludes this product); recording time is server-frozen, startOn is descriptive. For open/interest/settle, effectiveAt may be empty for frozen now or use one timestamp OR local date/time in history timezone. Settle closes the whole contract; actual early receipts are allowed despite forecast locks. No renewal or reservation release/restore; use the GUI if reservations constrain the operation. Inspect normalizedJSON/effects, then commit only the planId within ten minutes. Draft IDs are provisional.", s.previewProductOperation)
	closed := false
	mcp.AddTool(server, &mcp.Tool{Name: "commit_product_operation", Description: "Commit exactly a stored preview_product_operation plan. Requires ledger_write; only operationId and input.planId. Financial effects, lifecycle operation and immutable business receipt commit atomically. Retry identical operationId/input after interruption; same plan with another operationId returns original committed IDs without posting again, including after expiry/restart. Uncommitted expired/stale/restarted/restored plans require a new preview. Never releases/restores reservations. Read get_operation for outer receipt; list_product_operations for business operation IDs.", Annotations: &mcp.ToolAnnotations{IdempotentHint: true, OpenWorldHint: &closed}}, func(ctx context.Context, _ *mcp.CallToolRequest, in Mutation[CommitChangeInput]) (*mcp.CallToolResult, Response, error) {
		value, err := s.executeWithRecovery(ctx, in.OperationID, "commit_product_operation", in.Input, true, func(ctx context.Context) (any, error) { return s.commitProductOperation(ctx, in.Input) })
		if err != nil {
			return nil, Response{}, safeError(productWriteError(err))
		}
		return nil, Response{Data: value}, nil
	})
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
	if err := json.Unmarshal([]byte(preview.NormalizedJSON), &normalized); err != nil {
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
	if err := json.Unmarshal(raw, &plan); err != nil {
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
