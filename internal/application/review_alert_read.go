package application

import (
	"context"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type ReviewAlertV2Repository interface {
	ListReviewAlertGroupsV2(context.Context, domain.HouseholdID, domain.ReviewAlertFilter, string, int, time.Time) (domain.ReviewAlertPage, error)
	GetReviewAlertV2(context.Context, domain.HouseholdID, domain.ReviewEpisodeID, time.Time) (domain.ReviewAlertGroup, error)
}

// ListReviewAlertGroupsV2 reads only the last complete persisted publication.
// A caller explicitly invokes reconciliation to refresh it.
func (s *Service) ListReviewAlertGroupsV2(ctx context.Context, filter domain.ReviewAlertFilter, cursor string, pageSize int) (domain.ReviewAlertPage, error) {
	var empty domain.ReviewAlertPage
	reader, ok := s.repository.(ReviewAlertV2Repository)
	if !ok {
		return empty, &domain.Error{Code: domain.ErrUnavailable, Message: "daily review report is unavailable"}
	}
	household, err := s.repository.Household(ctx)
	if err != nil {
		return empty, err
	}
	if household == nil {
		return domain.ReviewAlertPage{Items: []domain.ReviewAlertGroup{}, ProjectionStatus: "pending"}, nil
	}
	return reader.ListReviewAlertGroupsV2(ctx, household.ID, filter, cursor, pageSize, s.clock())
}

// GetReviewAlertV2 reads exactly one currently open episode and returns the
// containing instrument group, including other active rules on that holding.
func (s *Service) GetReviewAlertV2(ctx context.Context, episodeID string) (domain.ReviewAlertGroup, error) {
	var empty domain.ReviewAlertGroup
	reader, ok := s.repository.(ReviewAlertV2Repository)
	if !ok {
		return empty, &domain.Error{Code: domain.ErrUnavailable, Message: "daily review report is unavailable"}
	}
	id, err := domain.ParseReviewEpisodeID(episodeID)
	if err != nil {
		return empty, err
	}
	household, err := s.repository.Household(ctx)
	if err != nil {
		return empty, err
	}
	if household == nil {
		return empty, &domain.Error{Code: domain.ErrNotFound, Field: "episodeId", Message: "review episode not found"}
	}
	return reader.GetReviewAlertV2(ctx, household.ID, id, s.clock())
}
