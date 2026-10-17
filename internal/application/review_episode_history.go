package application

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type ReviewEpisodeHistoryRepository interface {
	ListReviewEpisodeHistoryV2(context.Context, domain.HouseholdID, domain.ReviewRuleID, string, int) (domain.ReviewEpisodeHistoryPage, error)
	ListReviewEpisodeEventsV2(context.Context, domain.HouseholdID, domain.ReviewEpisodeID, string, int) (domain.ReviewEpisodeEventPage, error)
}

func (s *Service) ListReviewEpisodeHistoryV2(ctx context.Context, ruleID, cursor string, pageSize int) (domain.ReviewEpisodeHistoryPage, error) {
	var empty domain.ReviewEpisodeHistoryPage
	reader, ok := s.repository.(ReviewEpisodeHistoryRepository)
	if !ok {
		return empty, &domain.Error{Code: domain.ErrUnavailable, Message: "review history is unavailable"}
	}
	id, err := domain.ParseReviewRuleID(ruleID)
	if err != nil {
		return empty, err
	}
	household, err := s.repository.Household(ctx)
	if err != nil {
		return empty, err
	}
	if household == nil {
		return empty, &domain.Error{Code: domain.ErrNotFound, Field: "ruleId", Message: "review rule not found"}
	}
	return reader.ListReviewEpisodeHistoryV2(ctx, household.ID, id, cursor, pageSize)
}

func (s *Service) ListReviewEpisodeEventsV2(ctx context.Context, episodeID, cursor string, pageSize int) (domain.ReviewEpisodeEventPage, error) {
	var empty domain.ReviewEpisodeEventPage
	reader, ok := s.repository.(ReviewEpisodeHistoryRepository)
	if !ok {
		return empty, &domain.Error{Code: domain.ErrUnavailable, Message: "review history is unavailable"}
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
	return reader.ListReviewEpisodeEventsV2(ctx, household.ID, id, cursor, pageSize)
}
