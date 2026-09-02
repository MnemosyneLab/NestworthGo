package application

import (
	"context"
	"regexp"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
)

var payloadSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func parseActivityMutation(mutationID, payloadHash string) (*domain.ActivityMutation, error) {
	id := strings.TrimSpace(mutationID)
	if id == "" {
		return nil, nil
	}
	parsed, err := domain.ParseMutationID(id)
	if err != nil {
		return nil, err
	}
	hash := strings.ToLower(strings.TrimSpace(payloadHash))
	if !payloadSHA256Pattern.MatchString(hash) {
		return nil, &domain.Error{Code: domain.ErrValidation, Field: "payload", Message: "mutation payload hash must be a SHA-256 hex digest"}
	}
	return &domain.ActivityMutation{ID: parsed, PayloadSHA256: hash}, nil
}

func (s *Service) replayActivityMutation(ctx context.Context, key *domain.ActivityMutation) (*domain.ChangePreview, error) {
	if key == nil {
		return nil, nil
	}
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return nil, err
	}
	if bootstrap.Household == nil {
		return nil, onboardingRequired()
	}
	stored, err := s.repository.LookupActivityMutation(ctx, bootstrap.Household.ID, key.ID)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return nil, nil
	}
	if stored.PayloadSHA256 != key.PayloadSHA256 {
		return nil, &domain.Error{Code: domain.ErrConflict, Field: "mutationId", Message: "this mutation ID was already used with a different command"}
	}
	activity, err := s.repository.Activity(ctx, bootstrap.Household.ID, stored.ActivityID)
	if err != nil {
		return nil, err
	}
	return &domain.ChangePreview{Activity: activity, Effects: activity.Effects, Resulting: activity.Resulting}, nil
}
