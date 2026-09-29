package application

import (
	"context"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// PreviewFixChangeGuardedWith builds the replacement and its historical replay
// while writes are excluded. The returned token covers both the command's
// application state and every later activity checked by the replay.
func (s *Service) PreviewFixChangeGuardedWith(ctx context.Context, activityID domain.ActivityID, build func(context.Context) (any, error)) (domain.ChangePreview, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if build == nil {
		return domain.ChangePreview{}, "", &domain.Error{Code: domain.ErrValidation, Field: "command", Message: "command builder is required"}
	}
	unlock, err := s.beginPreviewRead(ctx)
	if err != nil {
		return domain.ChangePreview{}, "", err
	}
	defer unlock()
	command, err := build(ctx)
	if err != nil {
		return domain.ChangePreview{}, "", err
	}
	plan, err := s.fixChangePlan(ctx, activityID, command)
	if err != nil {
		return domain.ChangePreview{}, "", err
	}
	return plan.replacement, s.writes.previewToken(), nil
}

// PreviewUndoChangeGuarded computes a current-time reversal under the same
// serialization gate used by guarded writes.
func (s *Service) PreviewUndoChangeGuarded(ctx context.Context, activityID domain.ActivityID) (domain.ChangePreview, string, error) {
	return s.PreviewUndoChangeGuardedAt(ctx, activityID, s.clock())
}

// PreviewUndoChangeGuardedAt freezes the effective time shown for the
// reversal. The caller passes this same instant to RecordUndoChangeGuardedAt.
func (s *Service) PreviewUndoChangeGuardedAt(ctx context.Context, activityID domain.ActivityID, effectiveAt time.Time) (domain.ChangePreview, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	unlock, err := s.beginPreviewRead(ctx)
	if err != nil {
		return domain.ChangePreview{}, "", err
	}
	defer unlock()
	preview, err := s.undoChangePlanAt(ctx, activityID, effectiveAt)
	if err != nil {
		return domain.ChangePreview{}, "", err
	}
	return preview, s.writes.previewToken(), nil
}

// RecordFixChangeGuarded records the inverse, replacement, mutation key, and
// later projections in one database transaction. A committed retry is returned
// before checking its expired process-local preview token.
func (s *Service) RecordFixChangeGuarded(ctx context.Context, activityID domain.ActivityID, replacement any, mutationID, payloadHash, expectedToken string) (domain.ChangePreview, error) {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return domain.ChangePreview{}, err
	}
	defer unlock()
	key, err := parseActivityMutation(mutationID, payloadHash)
	if err != nil {
		return domain.ChangePreview{}, err
	}
	if replay, err := s.replayActivityMutation(ctx, key); err != nil {
		return domain.ChangePreview{}, err
	} else if replay != nil {
		return *replay, s.rebuildCorrectionSnapshots(ctx, replay.Activity.EffectiveLocalDate)
	}
	if err := s.validateCorrectionToken(key, expectedToken); err != nil {
		return domain.ChangePreview{}, err
	}
	return s.fixChangeLocked(ctx, activityID, replacement, key)
}

// RecordUndoChangeGuarded records a current-time reversal and its durable
// mutation key atomically. It deliberately leaves historical replay to Fix.
func (s *Service) RecordUndoChangeGuarded(ctx context.Context, activityID domain.ActivityID, mutationID, payloadHash, expectedToken string) (domain.ChangePreview, error) {
	return s.RecordUndoChangeGuardedAt(ctx, activityID, s.clock(), mutationID, payloadHash, expectedToken)
}

// RecordUndoChangeGuardedAt uses the effective time accepted during preview;
// its audit creation time remains the actual commit time.
func (s *Service) RecordUndoChangeGuardedAt(ctx context.Context, activityID domain.ActivityID, effectiveAt time.Time, mutationID, payloadHash, expectedToken string) (domain.ChangePreview, error) {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return domain.ChangePreview{}, err
	}
	defer unlock()
	key, err := parseActivityMutation(mutationID, payloadHash)
	if err != nil {
		return domain.ChangePreview{}, err
	}
	if replay, err := s.replayActivityMutation(ctx, key); err != nil {
		return domain.ChangePreview{}, err
	} else if replay != nil {
		return *replay, nil
	}
	if err := s.validateCorrectionToken(key, expectedToken); err != nil {
		return domain.ChangePreview{}, err
	}
	preview, err := s.undoChangePlanAt(ctx, activityID, effectiveAt)
	if err != nil {
		return domain.ChangePreview{}, err
	}
	if err := s.repository.CommitActivityBatch(ctx, []domain.ActivityCommit{{Activity: preview.Activity, Effects: preview.Effects, Resulting: preview.Resulting, Mutation: key}}, s.clock()); err != nil {
		return domain.ChangePreview{}, err
	}
	s.invalidateAnalysis()
	return preview, nil
}

func (s *Service) validateCorrectionToken(key *domain.ActivityMutation, expectedToken string) error {
	if key == nil {
		return &domain.Error{Code: domain.ErrValidation, Field: "mutationId", Message: "is required"}
	}
	if strings.TrimSpace(expectedToken) == "" {
		return &domain.Error{Code: domain.ErrValidation, Field: "previewToken", Message: "is required"}
	}
	if s.writes.previewToken() != expectedToken {
		return &domain.Error{Code: domain.ErrStalePreview, Field: "previewToken", Message: "preview is stale; request a new preview"}
	}
	return nil
}
