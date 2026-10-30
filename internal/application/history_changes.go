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
	preview, commit, err := s.prepareUndoChangeAt(ctx, activityID, time.Time{})
	if err != nil {
		return domain.ChangePreview{}, err
	}
	if err := s.repository.CommitActivityBatch(ctx, []domain.ActivityCommit{commit}, s.clock()); err != nil {
		return domain.ChangePreview{}, err
	}
	s.invalidateAnalysis()
	return preview, nil
}

// undoChangePlan validates the target and computes a reversal against today's
// state. Earlier explicit reversal instants replay any subsequent activities.
func (s *Service) undoChangePlan(ctx context.Context, activityID domain.ActivityID) (domain.ChangePreview, error) {
	return s.undoChangePlanAt(ctx, activityID, time.Time{})
}

func (s *Service) undoChangePlanAt(ctx context.Context, activityID domain.ActivityID, effectiveAt time.Time) (domain.ChangePreview, error) {
	preview, _, err := s.prepareUndoChangeAt(ctx, activityID, effectiveAt)
	return preview, err
}

func (s *Service) prepareUndoChangeAt(ctx context.Context, activityID domain.ActivityID, effectiveAt time.Time) (domain.ChangePreview, domain.ActivityCommit, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.ChangePreview{}, domain.ActivityCommit{}, err
	}
	if bootstrap.Household == nil {
		return domain.ChangePreview{}, domain.ActivityCommit{}, onboardingRequired()
	}
	if err := s.rejectManagedActivity(ctx, activityID); err != nil {
		return domain.ChangePreview{}, domain.ActivityCommit{}, err
	}
	activity, err := s.repository.Activity(ctx, bootstrap.Household.ID, activityID)
	if err != nil {
		return domain.ChangePreview{}, domain.ActivityCommit{}, err
	}
	if activity.ReversesActivityID != nil {
		return domain.ChangePreview{}, domain.ActivityCommit{}, &domain.Error{Code: domain.ErrAlreadyUndone, Message: "a reversal cannot be undone again"}
	}
	hasReversal, err := s.repository.ActivityHasReversal(ctx, bootstrap.Household.ID, activityID)
	if err != nil {
		return domain.ChangePreview{}, domain.ActivityCommit{}, err
	}
	if hasReversal {
		return domain.ChangePreview{}, domain.ActivityCommit{}, &domain.Error{Code: domain.ErrAlreadyUndone, Message: "the change has already been undone"}
	}
	// Absolute observations retain their original audit delta. Backfills can
	// change its economic magnitude (and direction), so undo must use the same
	// normalized effects as historical replay rather than the immutable row.
	timeline, err := s.repository.ListActivitiesUntil(ctx, bootstrap.Household.ID, s.clock())
	if err != nil {
		return domain.ChangePreview{}, domain.ActivityCommit{}, err
	}
	var effects []domain.ActivityEffect
	found := false
	for _, effective := range timeline {
		if effective.ID == activityID {
			effects = effective.Effects
			found = true
			break
		}
	}
	if !found {
		return domain.ChangePreview{}, domain.ActivityCommit{}, &domain.Error{Code: domain.ErrInvalidChange, Field: "activityId", Message: "the change is no longer active in history"}
	}
	if len(effects) == 0 && activity.Kind == domain.ActivityValueUpdate {
		// A now-redundant observation still needs a linked reversal and a
		// current endpoint projection. Keep a zero remeasurement effect; replay
		// drops its economic no-op while the original audit evidence stays intact.
		for _, effect := range activity.Effects {
			zero, err := domain.ParseMoney("0", effect.Money.Currency())
			if err != nil {
				return domain.ChangePreview{}, domain.ActivityCommit{}, err
			}
			effect.Money = &zero
			effects = append(effects, effect)
		}
	}
	state, err := s.changeState(ctx)
	if err != nil {
		return domain.ChangePreview{}, domain.ActivityCommit{}, err
	}
	if !effectiveAt.IsZero() {
		effectiveAt = effectiveAt.UTC()
		if effectiveAt.Before(state.OriginAt.UTC()) || effectiveAt.After(state.Now.UTC()) {
			return domain.ChangePreview{}, domain.ActivityCommit{}, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "effectiveAt", Message: "reversal time must be within the history interval"}
		}
	}
	if effectiveAt.IsZero() {
		effectiveAt = state.Now
	}
	if effectiveAt.Before(activity.EffectiveAt) {
		return domain.ChangePreview{}, domain.ActivityCommit{}, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "effectiveAt", Message: "reversal time cannot precede the original change"}
	}
	if err := s.rejectDependentHoldingUndo(ctx, activity, effects, state.Now); err != nil {
		return domain.ChangePreview{}, domain.ActivityCommit{}, err
	}
	// A frozen preview instant normally follows the complete timeline. Explicit
	// earlier instants must preserve later absolute observations just like an
	// ordinary historical insertion, rather than subtracting from today's value.
	index := len(timeline)
	for i, event := range timeline {
		if event.EffectiveAt.After(effectiveAt) {
			index = i
			break
		}
	}
	var components []domain.HistoryOriginComponent
	if index < len(timeline) {
		origin, err := s.HistoryOrigin(ctx)
		if err != nil {
			return domain.ChangePreview{}, domain.ActivityCommit{}, err
		}
		components, err = s.repository.ListHistoryOriginComponents(ctx, origin.ID)
		if err != nil {
			return domain.ChangePreview{}, domain.ActivityCommit{}, err
		}
		state = correctionOriginState(state, components)
		for _, event := range timeline[:index] {
			state, _, err = domain.ApplyEffects(state, event.Effects)
			if err != nil {
				return domain.ChangePreview{}, domain.ActivityCommit{}, err
			}
		}
	}
	preview, err := domain.InverseChangeAt(state, activity, effects, effectiveAt)
	if err != nil {
		return domain.ChangePreview{}, domain.ActivityCommit{}, err
	}
	commit := domain.ActivityCommit{Activity: preview.Activity, Effects: preview.Effects, Resulting: preview.Resulting}
	if index == len(timeline) {
		return preview, commit, nil
	}
	inserted := append([]domain.Activity{}, timeline[:index]...)
	inserted = append(inserted, preview.Activity)
	inserted = append(inserted, timeline[index:]...)
	inserted, err = domain.ReplayMoneyEffects(inserted, components)
	if err != nil {
		return domain.ChangePreview{}, domain.ActivityCommit{}, err
	}
	for _, event := range inserted[index:] {
		var views []domain.EndpointView
		state, views, err = domain.ApplyEffects(state, event.Effects)
		if err != nil {
			return domain.ChangePreview{}, domain.ActivityCommit{}, &domain.Error{Code: domain.ErrInvalidChange, Message: "the reversal would invalidate a later balance or holding quantity"}
		}
		if len(views) == 0 {
			if event.Kind == domain.ActivityValueUpdate {
				views = event.Resulting
			} else {
				views = correctionEndpointViews(state, event.RecordedEffects)
			}
		}
		commit.Replay = append(commit.Replay, domain.ActivityProjection{Activity: event, Resulting: views})
	}
	commit.Resulting = correctionEndpointViews(state, preview.Effects)
	return preview, commit, nil
}

// fixChangePlan validates the original and reconstructs the corrected timeline.
// Preview and commit share this calculation, including subsequent events.
func (s *Service) fixChangePlan(ctx context.Context, activityID domain.ActivityID, replacementCommand any) (correctionPlan, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return correctionPlan{}, err
	}
	if bootstrap.Household == nil {
		return correctionPlan{}, onboardingRequired()
	}
	if err := s.rejectManagedActivity(ctx, activityID); err != nil {
		return correctionPlan{}, err
	}
	if err := s.rejectManagedCommand(ctx, replacementCommand); err != nil {
		return correctionPlan{}, err
	}
	// Historical replay cannot introduce a new Holding. Unlike an ordinary
	// change, the correction batch has no Holding creation step.
	switch input := replacementCommand.(type) {
	case domain.PositionImportInput:
		return correctionPlan{}, &domain.Error{Code: domain.ErrCannotFixChange, Field: "command", Message: "a position import cannot replace a change"}
	case domain.PositionTransferInput:
		if input.ToHoldingID == "" {
			return correctionPlan{}, &domain.Error{Code: domain.ErrCannotFixChange, Field: "toHoldingId", Message: "a historical transfer correction requires an existing destination Holding"}
		}
	case domain.TradeInput:
		if input.HoldingID == "" {
			return correctionPlan{}, &domain.Error{Code: domain.ErrCannotFixChange, Field: "holdingId", Message: "a historical trade correction requires an existing Holding"}
		}
	}
	activity, err := s.repository.Activity(ctx, bootstrap.Household.ID, activityID)
	if err != nil {
		return correctionPlan{}, err
	}
	if activity.Kind == domain.ActivityCostAdjustment {
		return correctionPlan{}, &domain.Error{Code: domain.ErrCannotFixChange, Field: "activityId", Message: "reconcile total cost again to correct a cost adjustment"}
	}
	if activity.ReversesActivityID != nil {
		return correctionPlan{}, &domain.Error{Code: domain.ErrCannotFixChange, Message: "a reversal cannot be fixed directly"}
	}
	hasReversal, err := s.repository.ActivityHasReversal(ctx, bootstrap.Household.ID, activityID)
	if err != nil {
		return correctionPlan{}, err
	}
	if hasReversal {
		return correctionPlan{}, &domain.Error{Code: domain.ErrCannotFixChange, Message: "an already corrected change cannot be fixed again"}
	}
	return s.replayCorrection(ctx, activity, replacementCommand)
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
	plan, err := s.fixChangePlan(ctx, activityID, replacementCommand)
	if err != nil {
		return domain.ChangePreview{}, err
	}
	return plan.replacement, nil
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
		return *replay, s.rebuildCorrectionSnapshots(ctx, replay.Activity.EffectiveLocalDate)
	}
	return s.fixChangeLocked(ctx, activityID, replacementCommand, key)
}

func (s *Service) fixChangeLocked(ctx context.Context, activityID domain.ActivityID, replacementCommand any, key *domain.ActivityMutation) (domain.ChangePreview, error) {
	plan, err := s.fixChangePlan(ctx, activityID, replacementCommand)
	if err != nil {
		return domain.ChangePreview{}, err
	}
	inverse, replacement := plan.inverse, plan.replacement
	groupID := domain.NewActivityCorrectionGroupID()
	inverse.Activity.CorrectionGroupID = &groupID
	replacement.Activity.CorrectionGroupID = &groupID
	asOf := s.clock()
	if err := s.repository.CommitActivityBatch(ctx, []domain.ActivityCommit{
		{Activity: inverse.Activity, Effects: inverse.Effects, Resulting: inverse.Resulting},
		{Activity: replacement.Activity, Effects: replacement.Effects, Resulting: plan.current, Mutation: key, Replay: plan.projections},
	}, asOf); err != nil {
		return domain.ChangePreview{}, err
	}
	s.invalidateAnalysis()
	return replacement, s.rebuildCorrectionSnapshots(ctx, replacement.Activity.EffectiveLocalDate)
}

func (s *Service) rebuildCorrectionSnapshots(ctx context.Context, from string) error {
	origin, err := s.HistoryOrigin(ctx)
	if err != nil {
		return err
	}
	location, err := time.LoadLocation(origin.Timezone)
	if err != nil {
		return err
	}
	yesterday := s.clock().In(location).AddDate(0, 0, -1).Format("2006-01-02")
	if err := s.ensureClosedDaySnapshots(ctx, from, yesterday); err != nil {
		return err
	}
	return nil
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
