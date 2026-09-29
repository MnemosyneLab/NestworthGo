package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

const maxChangeBatchSize = 100

// ChangeBatchRepository is the atomic persistence boundary used by ledger batches.
// Keeping it separate from Repository preserves small test repositories.
type ChangeBatchRepository interface {
	CommitChangeBatch(context.Context, []domain.Holding, []domain.ActivityCommit, domain.ActivityMutation, time.Time) error
	LookupChangeBatch(context.Context, domain.HouseholdID, domain.MutationID) (*domain.ChangeBatchMutationRecord, error)
}

func (s *Service) PreviewChangesGuardedWith(ctx context.Context, build func(context.Context) ([]any, error)) ([]domain.ChangePreview, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if build == nil {
		return nil, "", &domain.Error{Code: domain.ErrValidation, Field: "commands", Message: "command builder is required"}
	}
	unlock, err := s.beginPreviewRead(ctx)
	if err != nil {
		return nil, "", err
	}
	defer unlock()
	commands, err := build(ctx)
	if err != nil {
		return nil, "", err
	}
	previews, _, _, err := s.prepareChangeBatch(ctx, commands)
	if err != nil {
		return nil, "", err
	}
	return previews, s.writes.previewToken(), nil
}

// RecordChangesGuarded commits every change and its receipt in one transaction.
// An already committed mutation replays before checking the preview token.
func (s *Service) RecordChangesGuarded(ctx context.Context, commands []any, mutationID, payloadHash, expectedToken string) ([]domain.ChangePreview, error) {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	key, err := parseActivityMutation(mutationID, payloadHash)
	if err != nil {
		return nil, err
	}
	if key == nil {
		return nil, &domain.Error{Code: domain.ErrValidation, Field: "mutationId", Message: "is required"}
	}
	batchRepo, ok := s.repository.(ChangeBatchRepository)
	if !ok {
		return nil, &domain.Error{Code: domain.ErrIntegrity, Message: "repository does not support atomic ledger batches"}
	}
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return nil, err
	}
	stored, err := batchRepo.LookupChangeBatch(ctx, household.ID, key.ID)
	if err != nil {
		return nil, err
	}
	if stored != nil {
		if stored.PayloadSHA256 != key.PayloadSHA256 {
			return nil, &domain.Error{Code: domain.ErrConflict, Field: "mutationId", Message: "this mutation ID was already used with a different command"}
		}
		previews := make([]domain.ChangePreview, 0, len(stored.ActivityIDs))
		for _, id := range stored.ActivityIDs {
			activity, err := s.repository.Activity(ctx, household.ID, id)
			if err != nil {
				return nil, err
			}
			previews = append(previews, domain.ChangePreview{Activity: activity, Effects: activity.Effects, Resulting: activity.Resulting})
		}
		return previews, nil
	}
	if single, err := s.repository.LookupActivityMutation(ctx, household.ID, key.ID); err != nil {
		return nil, err
	} else if single != nil {
		return nil, &domain.Error{Code: domain.ErrConflict, Field: "mutationId", Message: "this mutation ID was already used for a single change"}
	}
	if strings.TrimSpace(expectedToken) == "" {
		return nil, &domain.Error{Code: domain.ErrValidation, Field: "previewToken", Message: "is required"}
	}
	if s.writes.previewToken() != expectedToken {
		return nil, &domain.Error{Code: domain.ErrStalePreview, Field: "previewToken", Message: "preview is stale; request a new preview"}
	}
	previews, holdings, commits, err := s.prepareChangeBatch(ctx, commands)
	if err != nil {
		return nil, err
	}
	if err := batchRepo.CommitChangeBatch(ctx, holdings, commits, *key, commits[0].Activity.CreatedAt); err != nil {
		return nil, err
	}
	s.invalidateAnalysis()
	return previews, nil
}

func (s *Service) prepareChangeBatch(ctx context.Context, commands []any) ([]domain.ChangePreview, []domain.Holding, []domain.ActivityCommit, error) {
	if len(commands) == 0 || len(commands) > maxChangeBatchSize {
		return nil, nil, nil, &domain.Error{Code: domain.ErrValidation, Field: "commands", Message: "provide 1 to 100 changes"}
	}
	origin, snapshot, err := s.loadChangeContext(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	base, err := s.changeStateFrom(origin, snapshot)
	if err != nil {
		return nil, nil, nil, err
	}
	components, err := s.repository.ListHistoryOriginComponents(ctx, origin.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	timeline, err := s.repository.ListActivitiesUntil(ctx, base.HouseholdID, base.Now)
	if err != nil {
		return nil, nil, nil, err
	}
	previews := make([]domain.ChangePreview, 0, len(commands))
	holdings := make([]domain.Holding, 0)
	var previous time.Time
	for index, original := range commands {
		at := commandEffectiveTime(original)
		if at.IsZero() {
			at = base.Now
		}
		if index > 0 && at.Before(previous) {
			return nil, nil, nil, &domain.Error{Code: domain.ErrValidation, Field: fmt.Sprintf("commands[%d].effectiveAt", index), Message: "changes must be ordered by effectiveAt"}
		}
		previous = at
		if err := s.rejectManagedCommand(ctx, original); err != nil {
			return nil, nil, nil, batchCommandError(index, err)
		}
		// SQLite stores timestamps at millisecond precision. Activity IDs are
		// monotonic UUIDv7 values, so ties preserve input order and a later
		// single command at the same app clock follows this batch.
		createdAt := base.Now.Truncate(time.Millisecond)
		insertAt := len(timeline)
		for i, activity := range timeline {
			if activity.EffectiveAt.After(at) || (activity.EffectiveAt.Equal(at) && activity.CreatedAt.After(createdAt)) {
				insertAt = i
				break
			}
		}
		state := correctionOriginState(base, components)
		for _, activity := range timeline[:insertAt] {
			state, _, err = domain.ApplyEffects(state, activity.Effects)
			if err != nil {
				return nil, nil, nil, &domain.Error{Code: domain.ErrInvalidChange, Message: "the batch would invalidate an earlier balance or holding quantity"}
			}
		}
		state, command, newHolding, err := s.prepareChangeHolding(snapshot, state, original)
		if err != nil {
			return nil, nil, nil, batchCommandError(index, err)
		}
		if newHolding != nil {
			base.Holdings[newHolding.ID] = state.Holdings[newHolding.ID]
			snapshot.Holdings = append(snapshot.Holdings, *newHolding)
			holdings = append(holdings, *newHolding)
		}
		preview, err := domain.PreviewChange(state, command)
		if err != nil {
			return nil, nil, nil, batchCommandError(index, err)
		}
		preview.Activity.CreatedAt = createdAt
		preview.Activity.Resulting = preview.Resulting
		previews = append(previews, preview)
		timeline = append(timeline, domain.Activity{})
		copy(timeline[insertAt+1:], timeline[insertAt:])
		timeline[insertAt] = preview.Activity
		timeline, err = domain.ReplayMoneyEffects(timeline, components)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	state := correctionOriginState(base, components)
	projections := make([]domain.ActivityProjection, 0, len(timeline))
	for _, activity := range timeline {
		var views []domain.EndpointView
		state, views, err = domain.ApplyEffects(state, activity.Effects)
		if err != nil {
			return nil, nil, nil, &domain.Error{Code: domain.ErrInvalidChange, Message: "the batch would invalidate a later balance or holding quantity"}
		}
		if len(views) == 0 && activity.Kind == domain.ActivityValueUpdate {
			views = activity.Resulting
		}
		projections = append(projections, domain.ActivityProjection{Activity: activity, Resulting: views})
	}
	byID := make(map[domain.ActivityID]domain.Activity, len(previews))
	for _, activity := range timeline {
		byID[activity.ID] = activity
	}
	commits := make([]domain.ActivityCommit, 0, len(previews))
	for _, preview := range previews {
		activity := byID[preview.Activity.ID]
		commits = append(commits, domain.ActivityCommit{Activity: activity, Effects: activity.Effects, Resulting: correctionEndpointViews(state, activity.Effects)})
	}
	// Rebuild event projections after every insert, including later existing
	// facts whose balances changed because this batch was historical.
	commits[len(commits)-1].Replay = projections
	return previews, holdings, commits, nil
}

func batchCommandError(index int, err error) error {
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) {
		return err
	}
	field := fmt.Sprintf("commands[%d]", index)
	if domainErr.Field != "" {
		field += "." + domainErr.Field
	}
	return &domain.Error{Code: domainErr.Code, Field: field, Message: domainErr.Message}
}
