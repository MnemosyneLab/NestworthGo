package application

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type ReviewAcknowledgeV2Repository interface {
	AcknowledgeReviewEpisodeV2(context.Context, domain.ReviewAcknowledgeMutation) (domain.ReviewAcknowledgement, error)
}

func (s *Service) AcknowledgeReviewEpisodeV2(ctx context.Context, episodeID, mutationID string, expectedRevision int) (domain.ReviewAcknowledgement, error) {
	var empty domain.ReviewAcknowledgement
	port, ok := s.repository.(ReviewAcknowledgeV2Repository)
	if !ok {
		return empty, &domain.Error{Code: domain.ErrUnavailable, Message: "review acknowledgements are unavailable"}
	}
	episode, err := domain.ParseReviewEpisodeID(episodeID)
	if err != nil {
		return empty, err
	}
	mutation, err := domain.ParseMutationID(mutationID)
	if err != nil {
		return empty, err
	}
	if expectedRevision < 1 {
		return empty, &domain.Error{Code: domain.ErrValidation, Field: "expectedRevision", Message: "invalid episode revision"}
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
	return port.AcknowledgeReviewEpisodeV2(ctx, domain.ReviewAcknowledgeMutation{HouseholdID: household.ID, EpisodeID: episode, MutationID: mutation, ExpectedRevision: expectedRevision, At: s.clock()})
}
