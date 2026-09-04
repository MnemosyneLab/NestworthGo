package application

import (
	"context"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func (s *Service) ListActivities(ctx context.Context, limit int) ([]domain.Activity, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return nil, err
	}
	if bootstrap.Household == nil {
		return []domain.Activity{}, nil
	}
	return s.repository.ListActivities(ctx, bootstrap.Household.ID, limit)
}

// Activity loads one immutable history record by ID. This read path is used by
// presentation code that needs the original record for a reversal even when
// that record is outside the current filtered or paginated activity list.
func (s *Service) Activity(ctx context.Context, activityID domain.ActivityID) (domain.Activity, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.Activity{}, err
	}
	if bootstrap.Household == nil {
		return domain.Activity{}, onboardingRequired()
	}
	return s.repository.Activity(ctx, bootstrap.Household.ID, activityID)
}

func (s *Service) ListActivityPage(ctx context.Context, query domain.ActivityQuery) (domain.ActivityPage, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.ActivityPage{}, err
	}
	if bootstrap.Household == nil {
		return domain.ActivityPage{Activities: []domain.Activity{}}, nil
	}
	for field, value := range map[string]string{"fromDate": query.FromLocalDate, "toDate": query.ToLocalDate} {
		if value == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", value); err != nil {
			return domain.ActivityPage{}, &domain.Error{Code: domain.ErrValidation, Field: field, Message: "date must use YYYY-MM-DD"}
		}
	}
	if query.FromLocalDate != "" && query.ToLocalDate != "" && query.FromLocalDate > query.ToLocalDate {
		return domain.ActivityPage{}, &domain.Error{Code: domain.ErrValidation, Field: "dateRange", Message: "from date cannot be later than to date"}
	}
	return s.repository.ListActivityPage(ctx, bootstrap.Household.ID, query)
}

func (s *Service) UndoChange(ctx context.Context, activityID domain.ActivityID) (domain.ChangePreview, error) {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return domain.ChangePreview{}, err
	}
	defer unlock()
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.ChangePreview{}, err
	}
	if bootstrap.Household == nil {
		return domain.ChangePreview{}, onboardingRequired()
	}
	activity, err := s.repository.Activity(ctx, bootstrap.Household.ID, activityID)
	if err != nil {
		return domain.ChangePreview{}, err
	}
	if activity.ReversesActivityID != nil {
		return domain.ChangePreview{}, &domain.Error{Code: domain.ErrAlreadyUndone, Message: "a reversal cannot be undone again"}
	}
	hasReversal, err := s.repository.ActivityHasReversal(ctx, bootstrap.Household.ID, activityID)
	if err != nil {
		return domain.ChangePreview{}, err
	}
	if hasReversal {
		return domain.ChangePreview{}, &domain.Error{Code: domain.ErrAlreadyUndone, Message: "the change has already been undone"}
	}
	effects, err := s.repository.ActivityEffects(ctx, activityID)
	if err != nil {
		return domain.ChangePreview{}, err
	}
	state, err := s.changeState(ctx)
	if err != nil {
		return domain.ChangePreview{}, err
	}
	preview, err := domain.InverseChange(state, activity, effects)
	if err != nil {
		return domain.ChangePreview{}, err
	}
	if err := s.repository.CommitActivity(ctx, preview.Activity, preview.Effects, preview.Resulting, s.clock()); err != nil {
		return domain.ChangePreview{}, err
	}
	s.invalidateAnalysis()
	return preview, nil
}

// fixChangePreview computes the inverse-then-replace ChangePreview a Fix
// produces, without committing anything: it inverts the original
// Activity's effects against the *current* ChangeState (mirroring
// FixChange's own state derivation) and previews the replacement command
// against that inverted state. Both FixChange (which commits the result)
// and PreviewFixChange (which is read-only, for the Fix form's "preview
// before confirm" step) share this so the two never compute different
// numbers for the same input — the bug this fixes is that a plain
// PreviewChange call (ignoring the original Activity entirely) previews
// against the state *after* the original effect already applied, double
// counting it once Confirm actually inverts and replaces.
func (s *Service) fixChangePreview(ctx context.Context, activityID domain.ActivityID, replacementCommand any) (inverse, replacement domain.ChangePreview, err error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.ChangePreview{}, domain.ChangePreview{}, err
	}
	if bootstrap.Household == nil {
		return domain.ChangePreview{}, domain.ChangePreview{}, onboardingRequired()
	}
	activity, err := s.repository.Activity(ctx, bootstrap.Household.ID, activityID)
	if err != nil {
		return domain.ChangePreview{}, domain.ChangePreview{}, err
	}
	if activity.ReversesActivityID != nil {
		return domain.ChangePreview{}, domain.ChangePreview{}, &domain.Error{Code: domain.ErrCannotFixChange, Message: "a reversal cannot be fixed directly"}
	}
	hasReversal, err := s.repository.ActivityHasReversal(ctx, bootstrap.Household.ID, activityID)
	if err != nil {
		return domain.ChangePreview{}, domain.ChangePreview{}, err
	}
	if hasReversal {
		return domain.ChangePreview{}, domain.ChangePreview{}, &domain.Error{Code: domain.ErrCannotFixChange, Message: "an already corrected change cannot be fixed again"}
	}
	effects, err := s.repository.ActivityEffects(ctx, activityID)
	if err != nil {
		return domain.ChangePreview{}, domain.ChangePreview{}, err
	}
	state, err := s.changeState(ctx)
	if err != nil {
		return domain.ChangePreview{}, domain.ChangePreview{}, err
	}
	inverse, err = domain.InverseChange(state, activity, effects)
	if err != nil {
		return domain.ChangePreview{}, domain.ChangePreview{}, err
	}
	stateAfterInverse, _, err := domain.ApplyEffects(state, inverse.Effects)
	if err != nil {
		return domain.ChangePreview{}, domain.ChangePreview{}, err
	}
	replacement, err = domain.PreviewChange(stateAfterInverse, replacementCommand)
	if err != nil {
		return domain.ChangePreview{}, domain.ChangePreview{}, err
	}
	return inverse, replacement, nil
}

// PreviewFixChange is FixChange's read-only counterpart: it returns the
// same replacement ChangePreview FixChange would commit, so the Fix
// form's "preview, then confirm" step (matching every other change
// kind's UX) shows the actually-correct resulting balance/quantity
// instead of one that ignores the original Activity being replaced.
func (s *Service) PreviewFixChange(ctx context.Context, activityID domain.ActivityID, replacementCommand any) (domain.ChangePreview, error) {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return domain.ChangePreview{}, err
	}
	defer unlock()
	_, replacement, err := s.fixChangePreview(ctx, activityID, replacementCommand)
	if err != nil {
		return domain.ChangePreview{}, err
	}
	return replacement, nil
}

func (s *Service) FixChange(ctx context.Context, activityID domain.ActivityID, replacementCommand any) (domain.ChangePreview, error) {
	return s.FixChangeWithMutation(ctx, activityID, replacementCommand, "", "")
}

// FixChangeWithMutation is FixChange with an optional client-generated
// idempotency key stored on the replacement Activity. An empty mutation ID
// keeps the unkeyed path.
func (s *Service) FixChangeWithMutation(ctx context.Context, activityID domain.ActivityID, replacementCommand any, mutationID, payloadHash string) (domain.ChangePreview, error) {
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
	inverse, replacement, err := s.fixChangePreview(ctx, activityID, replacementCommand)
	if err != nil {
		return domain.ChangePreview{}, err
	}
	groupID := domain.NewActivityCorrectionGroupID()
	inverse.Activity.CorrectionGroupID = &groupID
	replacement.Activity.CorrectionGroupID = &groupID
	asOf := s.clock()
	if err := s.repository.CommitActivityBatch(ctx, []domain.ActivityCommit{
		{Activity: inverse.Activity, Effects: inverse.Effects, Resulting: inverse.Resulting},
		{Activity: replacement.Activity, Effects: replacement.Effects, Resulting: replacement.Resulting, Mutation: key},
	}, asOf); err != nil {
		return domain.ChangePreview{}, err
	}
	s.invalidateAnalysis()
	return replacement, nil
}

func (s *Service) appendObservationTime(origin *domain.HistoryOrigin, effectiveAt, createdAt time.Time) (time.Time, time.Time, error) {
	if origin == nil {
		return time.Time{}, time.Time{}, &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "start history before recording an observation"}
	}
	if effectiveAt.IsZero() {
		effectiveAt = s.clock()
	}
	effectiveAt = effectiveAt.UTC()
	createdAt = createdAt.UTC()
	if createdAt.IsZero() {
		createdAt = s.clock().UTC()
	}
	if effectiveAt.Before(origin.StartedAt) || effectiveAt.After(s.clock().UTC()) {
		return time.Time{}, time.Time{}, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "effectiveAt", Message: "observation time must be within the history interval"}
	}
	return effectiveAt, createdAt, nil
}

func (s *Service) AppendAccountStateObservation(ctx context.Context, observation domain.AccountStateObservation) error {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	origin, err := s.HistoryOrigin(ctx)
	if err != nil {
		return err
	}
	observation.EffectiveAt, observation.CreatedAt, err = s.appendObservationTime(origin, observation.EffectiveAt, observation.CreatedAt)
	if err != nil {
		return err
	}
	if observation.ID == "" {
		observation.ID = domain.NewAccountStateObservationID()
	}
	if _, err := domain.ParseOwnership(observation.Ownership); err != nil {
		return err
	}
	err = s.repository.AppendAccountStateObservation(ctx, observation)
	if err == nil {
		s.invalidateAnalysis()
	}
	return err
}

func (s *Service) AppendInstrumentPreferenceObservation(ctx context.Context, observation domain.InstrumentPreferenceObservation) error {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	origin, err := s.HistoryOrigin(ctx)
	if err != nil {
		return err
	}
	observation.EffectiveAt, observation.CreatedAt, err = s.appendObservationTime(origin, observation.EffectiveAt, observation.CreatedAt)
	if err != nil {
		return err
	}
	if observation.ID == "" {
		observation.ID = domain.NewInstrumentPreferenceObservationID()
	}
	if _, err := domain.ParseQuoteSourceKind(string(observation.SourceKind)); err != nil {
		return err
	}
	err = s.repository.AppendInstrumentPreferenceObservation(ctx, observation)
	if err == nil {
		s.invalidateAnalysis()
	}
	return err
}

func (s *Service) AppendFXPreferenceObservation(ctx context.Context, observation domain.FXPreferenceObservation) error {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	origin, err := s.HistoryOrigin(ctx)
	if err != nil {
		return err
	}
	observation.EffectiveAt, observation.CreatedAt, err = s.appendObservationTime(origin, observation.EffectiveAt, observation.CreatedAt)
	if err != nil {
		return err
	}
	if observation.ID == "" {
		observation.ID = domain.NewFXPreferenceObservationID()
	}
	if observation.CurrencyA == observation.CurrencyB {
		return &domain.Error{Code: domain.ErrValidation, Field: "currency", Message: "FX preference currencies must differ"}
	}
	if observation.CurrencyA > observation.CurrencyB {
		observation.CurrencyA, observation.CurrencyB = observation.CurrencyB, observation.CurrencyA
	}
	if _, err := domain.ParseQuoteSourceKind(string(observation.SourceKind)); err != nil {
		return err
	}
	err = s.repository.AppendFXPreferenceObservation(ctx, observation)
	if err == nil {
		s.invalidateAnalysis()
	}
	return err
}

func (s *Service) accountStateObservation(ctx context.Context, account domain.Account, ownership domain.Ownership) (domain.AccountStateObservation, error) {
	origin, err := s.HistoryOrigin(ctx)
	if err != nil {
		return domain.AccountStateObservation{}, err
	}
	if origin == nil {
		return domain.AccountStateObservation{}, nil
	}
	now := s.clock().UTC()
	observation := domain.AccountStateObservation{ID: domain.NewAccountStateObservationID(), AccountID: account.ID, EffectiveAt: now, ArchivedAt: account.ArchivedAt, IncludeInNetWorth: account.IncludeInNetWorth, IncludeInPortfolio: account.IncludeInPortfolio, IncludeInLiquidAssets: account.IncludeInLiquidAssets, CreatedAt: now, Ownership: ownership.Shares()}
	observation.EffectiveAt, observation.CreatedAt, err = s.appendObservationTime(origin, observation.EffectiveAt, observation.CreatedAt)
	if err != nil {
		return domain.AccountStateObservation{}, err
	}
	if _, err := domain.ParseOwnership(observation.Ownership); err != nil {
		return domain.AccountStateObservation{}, err
	}
	return observation, nil
}

func (s *Service) instrumentPreferenceObservation(ctx context.Context, instrument domain.Instrument) (domain.InstrumentPreferenceObservation, error) {
	origin, err := s.HistoryOrigin(ctx)
	if err != nil {
		return domain.InstrumentPreferenceObservation{}, err
	}
	if origin == nil {
		return domain.InstrumentPreferenceObservation{}, nil
	}
	now := s.clock().UTC()
	observation := domain.InstrumentPreferenceObservation{ID: domain.NewInstrumentPreferenceObservationID(), InstrumentID: instrument.ID, SourceKind: instrument.QuoteSource, EffectiveAt: now, CreatedAt: now}
	observation.EffectiveAt, observation.CreatedAt, err = s.appendObservationTime(origin, observation.EffectiveAt, observation.CreatedAt)
	return observation, err
}

func (s *Service) fxPreferenceObservation(ctx context.Context, preference domain.FXPreference) (domain.FXPreferenceObservation, error) {
	origin, err := s.HistoryOrigin(ctx)
	if err != nil {
		return domain.FXPreferenceObservation{}, err
	}
	if origin == nil {
		return domain.FXPreferenceObservation{}, nil
	}
	now := s.clock().UTC()
	observation := domain.FXPreferenceObservation{ID: domain.NewFXPreferenceObservationID(), HouseholdID: preference.HouseholdID, CurrencyA: preference.CurrencyA, CurrencyB: preference.CurrencyB, SourceKind: preference.SourceKind, EffectiveAt: now, CreatedAt: now}
	observation.EffectiveAt, observation.CreatedAt, err = s.appendObservationTime(origin, observation.EffectiveAt, observation.CreatedAt)
	return observation, err
}
