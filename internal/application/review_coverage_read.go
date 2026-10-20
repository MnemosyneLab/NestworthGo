package application

import (
	"context"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type ReviewCoverageV2Repository interface {
	ListReviewCoverageV2(context.Context, domain.HouseholdID, domain.ReviewCoverageFilter, string, int, time.Time) (domain.ReviewCoveragePage, error)
}

// ListReviewCoverageV2 is a pure read of the last complete publication.
// Status is all, triggered, unknown, confirming, healthy, paused or uncomputed.
func (s *Service) ListReviewCoverageV2(ctx context.Context, filter domain.ReviewCoverageFilter, cursor string, pageSize int) (domain.ReviewCoveragePage, error) {
	var empty domain.ReviewCoveragePage
	reader, ok := s.repository.(ReviewCoverageV2Repository)
	if !ok {
		return empty, &domain.Error{Code: domain.ErrUnavailable, Message: "daily review coverage is unavailable"}
	}
	household, err := s.repository.Household(ctx)
	if err != nil {
		return empty, err
	}
	if household == nil {
		return domain.ReviewCoveragePage{Items: []domain.ReviewCoverageGroup{}, ProjectionStatus: "pending"}, nil
	}
	return reader.ListReviewCoverageV2(ctx, household.ID, filter, cursor, pageSize, s.reviewNow())
}
