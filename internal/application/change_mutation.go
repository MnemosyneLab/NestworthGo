package application

import (
	"context"
	"regexp"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
)

var payloadSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// InspectChangeReceipt looks up a transaction-backed receipt without attempting
// a write. Absence is not permission to replay an interrupted host submission.
func (s *Service) InspectChangeReceipt(ctx context.Context, mutationID, payloadHash string) (*domain.ChangePreview, error) {
	key, err := parseActivityMutation(mutationID, payloadHash)
	if err != nil {
		return nil, err
	}
	if key == nil {
		return nil, &domain.Error{Code: domain.ErrValidation, Field: "mutationId", Message: "is required"}
	}
	unlock, err := s.beginPreviewRead(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	return s.replayActivityMutation(ctx, key)
}

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
	if batchRepo, ok := s.repository.(ChangeBatchRepository); ok {
		batch, err := batchRepo.LookupChangeBatch(ctx, bootstrap.Household.ID, key.ID)
		if err != nil {
			return nil, err
		}
		if batch != nil {
			return nil, &domain.Error{Code: domain.ErrConflict, Field: "mutationId", Message: "this mutation ID was already used for a batch"}
		}
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
