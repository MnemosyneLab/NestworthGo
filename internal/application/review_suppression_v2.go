package application

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type ReviewSuppressionV2Repository interface {
	ListReviewSuppressionsV2(context.Context, domain.HouseholdID, string, int) ([]domain.ReviewSuppression, error)
	MutateReviewSuppressionV2(context.Context, domain.ReviewSuppressionMutation) (*domain.ReviewSuppression, error)
}

type SetReviewSuppressionV2Input struct {
	SuppressionID    string `json:"suppressionId"`
	MutationID       string `json:"mutationId"`
	InstrumentID     string `json:"instrumentId"`
	RuleID           string `json:"ruleId,omitempty"`
	Mode             string `json:"mode"`
	Days             int    `json:"days,omitempty"`
	UntilOn          string `json:"untilOn,omitempty"`
	ExpectedRevision int    `json:"expectedRevision"`
}

func (s *Service) ListReviewSuppressionsV2(ctx context.Context, after string, limit int) ([]domain.ReviewSuppression, error) {
	port, ok := s.repository.(ReviewSuppressionV2Repository)
	if !ok {
		return nil, &domain.Error{Code: domain.ErrUnavailable, Message: "review suppressions are unavailable"}
	}
	if limit < 1 || limit > 100 {
		return nil, &domain.Error{Code: domain.ErrValidation, Field: "pageSize", Message: "use 1–100 suppressions per page"}
	}
	if after != "" {
		if _, err := domain.ParseReviewSuppressionID(after); err != nil {
			return nil, err
		}
	}
	household, err := s.repository.Household(ctx)
	if err != nil {
		return nil, err
	}
	if household == nil {
		return []domain.ReviewSuppression{}, nil
	}
	return port.ListReviewSuppressionsV2(ctx, household.ID, after, limit)
}

func (s *Service) SetReviewSuppressionV2(ctx context.Context, in SetReviewSuppressionV2Input) (domain.ReviewSuppression, error) {
	var empty domain.ReviewSuppression
	port, ok := s.repository.(ReviewSuppressionV2Repository)
	if !ok {
		return empty, &domain.Error{Code: domain.ErrUnavailable, Message: "review suppressions are unavailable"}
	}
	id, err := domain.ParseReviewSuppressionID(in.SuppressionID)
	if err != nil {
		return empty, err
	}
	mutation, err := domain.ParseMutationID(in.MutationID)
	if err != nil {
		return empty, err
	}
	instrument, err := domain.ParseInstrumentID(in.InstrumentID)
	if err != nil {
		return empty, err
	}
	var rule domain.ReviewRuleID
	if in.RuleID != "" {
		rule, err = domain.ParseReviewRuleID(in.RuleID)
		if err != nil {
			return empty, err
		}
	}
	if in.ExpectedRevision < 0 {
		return empty, &domain.Error{Code: domain.ErrValidation, Field: "expectedRevision", Message: "invalid suppression revision"}
	}
	if in.Mode != "permanent" && in.Mode != "until_date" {
		return empty, &domain.Error{Code: domain.ErrValidation, Field: "mode", Message: "choose permanent or a resume date"}
	}
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return empty, err
	}
	defer unlock()
	household, err := s.repository.Household(ctx)
	if err != nil {
		return empty, err
	}
	if household == nil {
		return empty, &domain.Error{Code: domain.ErrUnavailable, Message: "household is unavailable"}
	}
	origin, err := s.repository.HistoryOrigin(ctx, household.ID)
	if err != nil {
		return empty, err
	}
	timezone := "UTC"
	if origin != nil {
		timezone = origin.Timezone
	}
	if in.Mode == "permanent" {
		if in.Days != 0 || in.UntilOn != "" {
			return empty, &domain.Error{Code: domain.ErrValidation, Field: "untilOn", Message: "permanent ignore has no resume date"}
		}
	} else {
		if in.Days == 0 && in.UntilOn == "" {
			return empty, &domain.Error{Code: domain.ErrValidation, Field: "untilOn", Message: "resume date is required"}
		}
	}
	result, err := port.MutateReviewSuppressionV2(ctx, domain.ReviewSuppressionMutation{Operation: "set", HouseholdID: household.ID, SuppressionID: id, MutationID: mutation, InstrumentID: instrument, RuleID: rule, Mode: in.Mode, Timezone: timezone, RelativeDays: in.Days, CustomUntil: in.UntilOn, ExpectedRevision: in.ExpectedRevision, At: s.reviewNow()})
	if err != nil {
		return empty, err
	}
	return *result, nil
}

func (s *Service) ClearReviewSuppressionV2(ctx context.Context, suppressionID, mutationID string, expectedRevision int) error {
	port, ok := s.repository.(ReviewSuppressionV2Repository)
	if !ok {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "review suppressions are unavailable"}
	}
	id, err := domain.ParseReviewSuppressionID(suppressionID)
	if err != nil {
		return err
	}
	mutation, err := domain.ParseMutationID(mutationID)
	if err != nil {
		return err
	}
	if expectedRevision < 1 {
		return &domain.Error{Code: domain.ErrValidation, Field: "expectedRevision", Message: "invalid suppression revision"}
	}
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	household, err := s.repository.Household(ctx)
	if err != nil {
		return err
	}
	if household == nil {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "household is unavailable"}
	}
	_, err = port.MutateReviewSuppressionV2(ctx, domain.ReviewSuppressionMutation{Operation: "clear", HouseholdID: household.ID, SuppressionID: id, MutationID: mutation, ExpectedRevision: expectedRevision, At: s.reviewNow()})
	return err
}
