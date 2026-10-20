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
	PublishReviewProjectionWithClock(context.Context, domain.ReviewPublication, func() time.Time) error
}

// Record every clock observation made by review commands/read models, including
// preview and handling. A rollback observed outside reconcile must invalidate
// the same outstanding certificates, even while still above published as-of.
func (s *Service) reviewNow() time.Time {
	now := s.clock()
	if observer, ok := s.repository.(interface{ ObserveReviewClock(time.Time) }); ok {
		observer.ObserveReviewClock(now)
	}
	return now
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
		now := s.reviewNow()
		certified, certifiedMode := s.repository.(ReviewCertifiedRepository)
		certifiedMode = certifiedMode && certified.CertifiedReviewEnabled()
		selection := domain.ReviewProviderSelection{}
		if certifiedMode {
			selection = s.reviewProviderSnapshot()
		}
		if certifiedMode {
			writeCtx, unlock, err := s.beginLedgerWrite(ctx)
			if err != nil {
				return empty, err
			}
			var confirmed domain.ReviewNoChangeResult
			var unchanged bool
			err = s.withReviewProviderLease(selection, func() error {
				var e error
				confirmed, unchanged, e = certified.ConfirmCertifiedReviewNoChange(writeCtx, household.ID, now, s.reviewNow, selection)
				return e
			})
			unlock()
			if err != nil {
				var conflict *domain.Error
				if errors.As(err, &conflict) && conflict.Code == domain.ErrConflict {
					continue
				}
				return empty, err
			}
			if unchanged {
				return ReviewReconcileResult{RuleInstrumentCount: confirmed.RuleInstrumentCount, LatestClosedOn: confirmed.LatestClosedOn, SourceFingerprint: confirmed.SourceFingerprint}, nil
			}
		}
		capture, err := port.CaptureReviewProjection(ctx, household.ID, now)
		if err != nil {
			return empty, err
		}
		providerKey := s.FXProviderKey()
		if certifiedMode {
			providerKey = selection.Key
			if capture.Certified != nil {
				capture.Certified.Provider = selection
			} else {
				capture.TemporalFullRecompute = true
				capture.ReusableDailyEvidence = nil
			}
		}
		stage, err := PrepareReviewPublication(ctx, capture, now, providerKey)
		if err != nil {
			return empty, err
		}
		writeCtx, unlock, err := s.beginLedgerWrite(ctx)
		if err != nil {
			return empty, err
		}
		if certifiedMode {
			err = certified.PublishCertifiedReviewProjection(writeCtx, stage, s.reviewNow, func(fn func() error) error { return s.withReviewProviderLease(selection, fn) })
		} else {
			err = port.PublishReviewProjectionWithClock(writeCtx, stage, s.reviewNow)
		}
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
