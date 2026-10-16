package application

import (
	"context"
	"errors"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type ReviewProjectionRepository interface {
	CaptureReviewProjection(context.Context, domain.HouseholdID, time.Time) (domain.ReviewProjectionCapture, error)
	PublishReviewProjection(context.Context, domain.ReviewPublication, time.Time) error
}

type ReviewReconcileResult struct {
	RuleInstrumentCount int    `json:"ruleInstrumentCount"`
	DayCount            int    `json:"dayCount"`
	LatestClosedOn      string `json:"latestClosedOn"`
	SourceFingerprint   string `json:"sourceFingerprint"`
}

// ReconcileInvestmentReviewsV2 is an explicit foreground refresh. Readers
// never call it implicitly. Conflicts from a ledger write, rule edit, handling
// command or local midnight discard the entire stage and retry from scratch.
func (s *Service) ReconcileInvestmentReviewsV2(ctx context.Context) (ReviewReconcileResult, error) {
	var empty ReviewReconcileResult
	port, ok := s.repository.(ReviewProjectionRepository)
	if !ok {
		return empty, &domain.Error{Code: domain.ErrUnavailable, Message: "daily review publication is unavailable"}
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		household, err := s.repository.Household(ctx)
		if err != nil {
			return empty, err
		}
		if household == nil {
			return empty, &domain.Error{Code: domain.ErrUnavailable, Message: "household is unavailable"}
		}
		now := s.clock()
		capture, err := port.CaptureReviewProjection(ctx, household.ID, now)
		if err != nil {
			return empty, err
		}
		stage, err := PrepareReviewPublication(ctx, capture, now, s.FXProviderKey())
		if err != nil {
			return empty, err
		}
		writeCtx, unlock, err := s.beginLedgerWrite(ctx)
		if err != nil {
			return empty, err
		}
		err = port.PublishReviewProjection(writeCtx, stage, s.clock())
		unlock()
		if err == nil {
			result := ReviewReconcileResult{RuleInstrumentCount: len(stage.Rules), SourceFingerprint: capture.HistoricalFingerprint}
			for _, rule := range stage.Rules {
				result.DayCount += len(rule.Days)
				if rule.LatestClosedOn > result.LatestClosedOn {
					result.LatestClosedOn = rule.LatestClosedOn
				}
			}
			return result, nil
		}
		var domainError *domain.Error
		if !errors.As(err, &domainError) || domainError.Code != domain.ErrConflict {
			return empty, err
		}
	}
	return empty, &domain.Error{Code: domain.ErrConflict, Message: "review evidence kept changing; try refreshing again"}
}
