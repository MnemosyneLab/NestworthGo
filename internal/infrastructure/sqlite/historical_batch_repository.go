package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// LoadHistoricalSnapshotBatch reads every candidate needed by a bounded
// rebuild under one SQLite read transaction. The application deliberately
// filters the returned immutable input by each day's cutoff instead of
// reopening a read transaction for every day.
func (r *Repository) LoadHistoricalSnapshotBatch(ctx context.Context, householdID domain.HouseholdID, cutoff time.Time) (domain.HistoricalSnapshotBatch, error) {
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.HistoricalSnapshotBatch{}, err
	}
	fail := func(cause error) (domain.HistoricalSnapshotBatch, error) {
		_ = tx.Rollback()
		return domain.HistoricalSnapshotBatch{}, cause
	}
	var origin domain.HistoryOrigin
	if _, err := scanHistoryOrigin(tx.QueryRowContext(ctx, `SELECT id, household_id, timezone, started_at, created_at FROM history_origins WHERE household_id = ?`, householdID.String()), &origin); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fail(&domain.Error{Code: domain.ErrHistoryNotStarted, Message: "history origin was not found"})
		}
		return fail(err)
	}
	if cutoff.IsZero() {
		return fail(&domain.Error{Code: domain.ErrValidation, Field: "cutoff", Message: "historical batch cutoff is required"})
	}
	portfolio, err := readPortfolioSnapshotQuery(ctx, tx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return fail(err)
	}
	originData, err := historyOriginDataQuery(ctx, tx, origin.ID)
	if err != nil {
		return fail(err)
	}
	accountObservations, err := listAccountStateObservationsQuery(ctx, tx, householdID)
	if err != nil {
		return fail(err)
	}
	instrumentStateObservations, err := listInstrumentStateObservationsQuery(ctx, tx, householdID)
	if err != nil {
		return fail(err)
	}
	holdingStateObservations, err := listHoldingStateObservationsQuery(ctx, tx, householdID)
	if err != nil {
		return fail(err)
	}
	instrumentPreferences, err := listInstrumentPreferenceObservationsQuery(ctx, tx, householdID)
	if err != nil {
		return fail(err)
	}
	fxPreferences, err := listFXPreferenceObservationsQuery(ctx, tx, householdID)
	if err != nil {
		return fail(err)
	}
	currentFXPreferences, err := listFXPreferencesQuery(ctx, tx, householdID)
	if err != nil {
		return fail(err)
	}
	activities, err := listActivitiesUntilQuery(ctx, tx, householdID, cutoff)
	if err != nil {
		return fail(err)
	}
	instrumentQuotes, err := listAllInstrumentQuotesQuery(ctx, tx, householdID, cutoff)
	if err != nil {
		return fail(err)
	}
	fxQuotes, err := listAllFXQuotesQuery(ctx, tx, householdID, cutoff)
	if err != nil {
		return fail(err)
	}
	if err := tx.Commit(); err != nil {
		return domain.HistoricalSnapshotBatch{}, err
	}
	return domain.HistoricalSnapshotBatch{
		Origin:                      origin,
		OriginData:                  originData,
		Portfolio:                   portfolio,
		AccountStateObservations:    accountObservations,
		InstrumentStateObservations: instrumentStateObservations,
		HoldingStateObservations:    holdingStateObservations,
		InstrumentPreferenceFacts:   instrumentPreferences,
		FXPreferenceFacts:           fxPreferences,
		FXPreferences:               currentFXPreferences,
		Activities:                  activities,
		InstrumentQuoteFacts:        instrumentQuotes,
		FXQuoteFacts:                fxQuotes,
	}, nil
}
