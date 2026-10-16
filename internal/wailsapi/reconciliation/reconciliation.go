// Package reconciliation is the desktop adapter for the shared session workflow.
package reconciliation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/preview"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type Service struct {
	products    preview.Store[productPlan]
	app         *application.Service
	sessions    preview.Store[application.ReconciliationSessionPlan]
	adjustments preview.Store[adjustmentPlan]
}

func NewService(app *application.Service) *Service { return &Service{app: app} }

type SessionPreview struct {
	PlanID string                       `json:"planId"`
	Result domain.ReconciliationSession `json:"result"`
}

func (s *Service) List(ctx context.Context) ([]domain.ReconciliationSession, error) {
	v, e := s.app.ListReconciliationSessions(ctx)
	return v, apierror.Wrap(e)
}
func (s *Service) Get(ctx context.Context, id string) (domain.ReconciliationSession, error) {
	v, e := s.app.GetReconciliationSession(ctx, id)
	return v, apierror.Wrap(e)
}
func (s *Service) Links(ctx context.Context, id string) ([]domain.ReconciliationLinkedChange, error) {
	v, e := s.app.ReconciliationLinkedChanges(ctx, id)
	return v, apierror.Wrap(e)
}
func (s *Service) LastSuccess(ctx context.Context, account string) (*domain.ReconciliationSession, error) {
	v, e := s.app.ReconciliationLastSuccess(ctx, account)
	return v, apierror.Wrap(e)
}
func (s *Service) Preview(ctx context.Context, in application.ReconciliationAction) (SessionPreview, error) {
	v, e := s.app.PreviewReconciliationSession(ctx, in)
	if e != nil {
		return SessionPreview{}, apierror.Wrap(e)
	}
	raw, e := json.Marshal(v)
	if e == nil {
		e = s.app.SaveReconciliationPlan(ctx, "session", v.ID, raw)
	}
	if e != nil {
		return SessionPreview{}, apierror.Wrap(e)
	}
	return SessionPreview{PlanID: v.ID, Result: v.Result}, nil
}
func (s *Service) Commit(ctx context.Context, id string) (domain.ReconciliationSession, error) {
	var plan application.ReconciliationSessionPlan
	raw, e := s.app.LoadReconciliationPlan(ctx, "session", id)
	if e != nil {
		receipt, lookupErr := s.app.ReconciliationSessionReceipt(ctx, id)
		if lookupErr != nil {
			return domain.ReconciliationSession{}, apierror.Wrap(lookupErr)
		}
		if receipt != nil {
			return *receipt, nil
		}
		return domain.ReconciliationSession{}, apierror.Wrap(e)
	}
	if e = json.Unmarshal(raw, &plan); e != nil {
		return domain.ReconciliationSession{}, apierror.Wrap(e)
	}
	if plan.ID != id {
		return domain.ReconciliationSession{}, apierror.Wrap(&domain.Error{Code: domain.ErrConflict, Field: "planId", Message: "saved identity differs"})
	}
	v, e := s.app.CommitReconciliationSession(ctx, plan)
	return v, apierror.Wrap(e)
}

type AdjustmentRequest struct {
	Link      domain.ReconciliationLink `json:"link"`
	AccountID string                    `json:"accountId"`
	HoldingID string                    `json:"holdingId"`
	Balance   string                    `json:"balance"`
	Currency  string                    `json:"currency"`
	Quantity  string                    `json:"quantity"`
	UnitCost  string                    `json:"unitCost"`
	TotalCost string                    `json:"totalCost"`
	Note      string                    `json:"note"`
}
type adjustmentPlan struct {
	commands        []any
	id, hash, token string
	link            domain.ReconciliationLink
	expires         time.Time
}
type AdjustmentPreview struct {
	PlanID  string                  `json:"planId"`
	Changes []wire.ChangePreviewDTO `json:"changes"`
}

func (s *Service) PreviewAdjustment(ctx context.Context, in AdjustmentRequest) (AdjustmentPreview, error) {
	target := application.ReconciliationTarget{Note: &in.Note}
	var err error
	if in.HoldingID != "" {
		id, e := domain.ParseHoldingID(in.HoldingID)
		if e != nil {
			return AdjustmentPreview{}, apierror.Wrap(e)
		}
		target.HoldingID = &id
		if in.Quantity != "" {
			q, e := domain.ParseQuantity(in.Quantity)
			if e != nil {
				return AdjustmentPreview{}, apierror.Wrap(e)
			}
			target.TargetQuantity = &q
		}
		if in.UnitCost != "" {
			q, e := domain.ParseUnitPrice(in.UnitCost)
			if e != nil {
				return AdjustmentPreview{}, apierror.Wrap(e)
			}
			target.UnitCost = &q
		}
		if in.TotalCost != "" {
			currency, e := domain.ParseSupportedCurrency(in.Currency)
			if e != nil {
				return AdjustmentPreview{}, apierror.Wrap(e)
			}
			m, e := domain.ParseMoney(in.TotalCost, currency)
			if e != nil {
				return AdjustmentPreview{}, apierror.Wrap(e)
			}
			target.TargetTotalCost = &m
		}
	} else {
		id, e := domain.ParseAccountID(in.AccountID)
		if e != nil {
			return AdjustmentPreview{}, apierror.Wrap(e)
		}
		target.AccountID = &id
		currency, e := domain.ParseSupportedCurrency(in.Currency)
		if e != nil {
			return AdjustmentPreview{}, apierror.Wrap(e)
		}
		m, e := domain.ParseMoney(in.Balance, currency)
		if e != nil {
			return AdjustmentPreview{}, apierror.Wrap(e)
		}
		target.TargetBalance = &m
	}
	plan := adjustmentPlan{id: uuid.NewString(), link: in.Link, expires: time.Now().Add(10 * time.Minute)}
	values, token, err := s.app.PreviewChangesGuardedWith(ctx, func(ctx context.Context) ([]any, error) {
		plan.commands, err = s.app.BuildReconciliationCommands(ctx, []application.ReconciliationTarget{target})
		return plan.commands, err
	})
	if err != nil {
		return AdjustmentPreview{}, apierror.Wrap(err)
	}
	if err = s.app.ValidateReconciliationLink(ctx, &in.Link, values); err != nil {
		return AdjustmentPreview{}, apierror.Wrap(err)
	}
	raw, _ := json.Marshal(in)
	sum := sha256.Sum256(raw)
	plan.hash = hex.EncodeToString(sum[:])
	plan.token = token
	raw, e := json.Marshal(plan)
	if e == nil {
		e = s.app.SaveReconciliationPlan(ctx, "adjustment", plan.id, raw)
	}
	if e != nil {
		return AdjustmentPreview{}, apierror.Wrap(e)
	}
	result := AdjustmentPreview{PlanID: plan.id}
	for _, v := range values {
		result.Changes = append(result.Changes, wire.FromChangePreview(v))
	}
	return result, nil
}
func (s *Service) CommitAdjustment(ctx context.Context, id string) ([]wire.ChangePreviewDTO, error) {
	p, e := s.adjustments.Get(id)
	if e != nil {
		raw, err := s.app.LoadReconciliationPlan(ctx, "adjustment", id)
		if err != nil {
			receipt, lookupErr := s.app.ReconciliationAdjustmentReceipt(ctx, id)
			if lookupErr != nil {
				return nil, apierror.Wrap(lookupErr)
			}
			if receipt != nil {
				result := []wire.ChangePreviewDTO{}
				for _, v := range receipt {
					result = append(result, wire.FromChangePreview(v))
				}
				return result, nil
			}
			return nil, apierror.Wrap(err)
		}
		if e = json.Unmarshal(raw, &p); e != nil {
			return nil, apierror.Wrap(e)
		}
	}
	token := p.token
	if time.Now().After(p.expires) {
		token = "expired"
	}
	values, e := s.app.RecordChangesGuarded(domain.WithReconciliationLink(ctx, &p.link), p.commands, p.id, p.hash, token)
	if e != nil {
		return nil, apierror.Wrap(e)
	}
	result := []wire.ChangePreviewDTO{}
	for _, v := range values {
		result = append(result, wire.FromChangePreview(v))
	}
	return result, nil
}

func (s *Service) Changes(ctx context.Context, id, account string) ([]domain.ReconciliationFinancialChange, error) {
	v, e := s.app.ReconciliationChanges(ctx, id, account)
	return v, apierror.Wrap(e)
}
