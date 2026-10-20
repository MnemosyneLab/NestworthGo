package application

import (
	"context"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

type reconciliationPlanRepository interface {
	SaveReconciliationPlan(context.Context, string, string, string, []byte) error
	LoadReconciliationPlan(context.Context, string, string, string) ([]byte, error)
	ReconciliationReceiptByID(context.Context, string, string) (*domain.ReconciliationSession, error)
}

// Desktop recovery material is private configuration, never business export data.
// Callers must retain the returned identity and may only retry that reviewed plan.
func (s *Service) SaveReconciliationPlan(ctx context.Context, family, id string, raw []byte) error {
	if _, err := uuid.Parse(id); err != nil || !jsoncodec.Valid(raw) {
		return reconciliationError("planId", "invalid saved plan")
	}
	h, err := s.requireHousehold(ctx)
	if err != nil {
		return err
	}
	r, ok := s.repository.(reconciliationPlanRepository)
	if !ok {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "durable plan storage unavailable"}
	}
	return r.SaveReconciliationPlan(ctx, h.ID.String(), family, id, raw)
}
func (s *Service) LoadReconciliationPlan(ctx context.Context, family, id string) ([]byte, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, reconciliationError("planId", "invalid plan identity")
	}
	h, err := s.requireHousehold(ctx)
	if err != nil {
		return nil, err
	}
	r, ok := s.repository.(reconciliationPlanRepository)
	if !ok {
		return nil, &domain.Error{Code: domain.ErrUnavailable, Message: "durable plan storage unavailable"}
	}
	return r.LoadReconciliationPlan(ctx, h.ID.String(), family, id)
}
func (s *Service) ReconciliationSessionReceipt(ctx context.Context, id string) (*domain.ReconciliationSession, error) {
	h, err := s.requireHousehold(ctx)
	if err != nil {
		return nil, err
	}
	r, ok := s.repository.(reconciliationPlanRepository)
	if !ok {
		return nil, nil
	}
	// Read-only recovery cannot submit or invent a plan for an unknown identity.
	return r.ReconciliationReceiptByID(ctx, h.ID.String(), id)
}

// This is receipt inspection only. It never executes an ID or relaxes the
// payload-hash checks on actual submissions.
func (s *Service) ReconciliationAdjustmentReceipt(ctx context.Context, id string) ([]domain.ChangePreview, error) {
	mutation, err := domain.ParseMutationID(id)
	if err != nil {
		return nil, err
	}
	h, err := s.requireHousehold(ctx)
	if err != nil {
		return nil, err
	}
	r, ok := s.repository.(ChangeBatchRepository)
	if !ok {
		return nil, nil
	}
	receipt, err := r.LookupChangeBatch(ctx, h.ID, mutation)
	if err != nil || receipt == nil {
		return nil, err
	}
	result := make([]domain.ChangePreview, 0, len(receipt.ActivityIDs))
	for _, id := range receipt.ActivityIDs {
		a, e := s.repository.Activity(ctx, h.ID, id)
		if e != nil {
			return nil, e
		}
		result = append(result, domain.ChangePreview{Activity: a, Effects: a.Effects, Resulting: a.Resulting})
	}
	return result, nil
}

func (s *Service) ReconciliationProductReceipt(ctx context.Context, id string) (bool, error) {
	operation, err := domain.ParseProductOperationID(id)
	if err != nil {
		return false, err
	}
	h, err := s.requireHousehold(ctx)
	if err != nil {
		return false, err
	}
	terms, err := s.repository.LookupProductTermsMutation(ctx, h.ID, operation)
	if err != nil {
		return false, err
	}
	if terms != nil {
		return true, nil
	}
	receipt, err := s.repository.LookupProductOperation(ctx, h.ID, operation)
	return receipt != nil, err
}
