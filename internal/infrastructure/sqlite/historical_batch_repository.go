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
// filters the returned immutable portfolio/activity input by each day's
// household cutoff while the historical resolver applies each requested
// market-date label to its market data facts, instead of reopening a read
// transaction for every day.
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
	zeroBaselines, err := listZeroAccountBaselines(ctx, tx, householdID, origin.StartedAt)
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
	instrumentProviderBindings, err := listInstrumentProviderBindingRevisionsQuery(ctx, tx, householdID)
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
	instrumentCoverage, err := listHistoricalInstrumentHistoryCoverageQuery(ctx, tx, householdID)
	if err != nil {
		return fail(err)
	}
	fxCoverage, err := listFXHistoryCoverageQuery(ctx, tx, householdID)
	if err != nil {
		return fail(err)
	}
	var generation int
	var resolverPolicy sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT input_generation, resolver_policy_version FROM history_snapshot_state WHERE household_id = ?`, householdID.String()).Scan(&generation, &resolverPolicy); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return fail(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.HistoricalSnapshotBatch{}, err
	}
	return domain.HistoricalSnapshotBatch{
		ZeroAccountBaselines:           zeroBaselines,
		Origin:                         origin,
		OriginData:                     originData,
		Portfolio:                      portfolio,
		AccountStateObservations:       accountObservations,
		InstrumentStateObservations:    instrumentStateObservations,
		HoldingStateObservations:       holdingStateObservations,
		InstrumentPreferenceFacts:      instrumentPreferences,
		InstrumentProviderBindingFacts: instrumentProviderBindings,
		FXPreferenceFacts:              fxPreferences,
		FXPreferences:                  currentFXPreferences,
		Activities:                     activities,
		InstrumentQuoteFacts:           instrumentQuotes,
		FXQuoteFacts:                   fxQuotes,
		InstrumentHistoryCoverage:      instrumentCoverage,
		FXHistoryCoverage:              fxCoverage,
		InputGeneration:                generation,
		ResolverPolicyVersion:          resolverPolicy.String,
	}, nil
}

// Only explicit zero baselines can seed post-origin accounts. Nonzero initial
// funding is already represented by activities; replay projections are not facts.
func listZeroAccountBaselines(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID, origin time.Time) ([]domain.AccountValue, error) {
	rows, err := tx.QueryContext(ctx, `SELECT av.account_id, av.currency, av.effective_at
 FROM account_values av JOIN accounts a ON a.id = av.account_id
 WHERE a.household_id = ? AND a.created_at >= ?
 AND av.projection_kind = 'baseline' AND av.activity_effect_id IS NULL AND av.amount = '0'
 AND NOT EXISTS (SELECT 1 FROM account_values earlier WHERE earlier.account_id = av.account_id
 AND (earlier.created_at < av.created_at OR (earlier.created_at = av.created_at AND earlier.id < av.id)))`, householdID.String(), origin.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.AccountValue
	for rows.Next() {
		var accountID, currency, effectiveAt string
		if err := rows.Scan(&accountID, &currency, &effectiveAt); err != nil {
			return nil, err
		}
		id, err := domain.ParseAccountID(accountID)
		if err != nil {
			return nil, err
		}
		code, err := domain.ParseCurrency(currency)
		if err != nil {
			return nil, err
		}
		amount, err := domain.ParseMoney("0", code)
		if err != nil {
			return nil, err
		}
		effective, err := time.Parse(time.RFC3339Nano, effectiveAt)
		if err != nil {
			return nil, err
		}
		result = append(result, domain.AccountValue{AccountID: id, Amount: amount, EffectiveAt: effective})
	}
	return result, rows.Err()
}
