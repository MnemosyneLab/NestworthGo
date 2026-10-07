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
	defer tx.Rollback()
	batch, err := loadHistoricalSnapshotBatchQuery(ctx, tx, householdID, cutoff)
	if err != nil {
		return domain.HistoricalSnapshotBatch{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.HistoricalSnapshotBatch{}, err
	}
	return batch, nil
}

func loadHistoricalSnapshotBatchQuery(ctx context.Context, query queryer, householdID domain.HouseholdID, cutoff time.Time) (domain.HistoricalSnapshotBatch, error) {
	var origin domain.HistoryOrigin
	if _, err := scanHistoryOrigin(query.QueryRowContext(ctx, `SELECT id, household_id, timezone, started_at, created_at FROM history_origins WHERE household_id = ?`, householdID.String()), &origin); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.HistoricalSnapshotBatch{}, &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "history origin was not found"}
		}
		return domain.HistoricalSnapshotBatch{}, err
	}
	if cutoff.IsZero() {
		return domain.HistoricalSnapshotBatch{}, &domain.Error{Code: domain.ErrValidation, Field: "cutoff", Message: "historical batch cutoff is required"}
	}
	portfolio, err := readPortfolioSnapshotQuery(ctx, query, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return domain.HistoricalSnapshotBatch{}, err
	}
	originData, err := historyOriginDataQuery(ctx, query, origin.ID)
	if err != nil {
		return domain.HistoricalSnapshotBatch{}, err
	}
	zeroBaselines, err := listZeroAccountBaselines(ctx, query, householdID, origin.StartedAt)
	if err != nil {
		return domain.HistoricalSnapshotBatch{}, err
	}
	accountObservations, err := listAccountStateObservationsQuery(ctx, query, householdID)
	if err != nil {
		return domain.HistoricalSnapshotBatch{}, err
	}
	instrumentStateObservations, err := listInstrumentStateObservationsQuery(ctx, query, householdID)
	if err != nil {
		return domain.HistoricalSnapshotBatch{}, err
	}
	holdingStateObservations, err := listHoldingStateObservationsQuery(ctx, query, householdID)
	if err != nil {
		return domain.HistoricalSnapshotBatch{}, err
	}
	instrumentPreferences, err := listInstrumentPreferenceObservationsQuery(ctx, query, householdID)
	if err != nil {
		return domain.HistoricalSnapshotBatch{}, err
	}
	instrumentProviderBindings, err := listInstrumentProviderBindingRevisionsQuery(ctx, query, householdID)
	if err != nil {
		return domain.HistoricalSnapshotBatch{}, err
	}
	fxPreferences, err := listFXPreferenceObservationsQuery(ctx, query, householdID)
	if err != nil {
		return domain.HistoricalSnapshotBatch{}, err
	}
	currentFXPreferences, err := listFXPreferencesQuery(ctx, query, householdID)
	if err != nil {
		return domain.HistoricalSnapshotBatch{}, err
	}
	activities, err := listActivitiesUntilQuery(ctx, query, householdID, cutoff)
	if err != nil {
		return domain.HistoricalSnapshotBatch{}, err
	}
	instrumentQuotes, err := listAllInstrumentQuotesQuery(ctx, query, householdID, cutoff)
	if err != nil {
		return domain.HistoricalSnapshotBatch{}, err
	}
	fxQuotes, err := listAllFXQuotesQuery(ctx, query, householdID, cutoff)
	if err != nil {
		return domain.HistoricalSnapshotBatch{}, err
	}
	instrumentCoverage, err := listHistoricalInstrumentHistoryCoverageQuery(ctx, query, householdID)
	if err != nil {
		return domain.HistoricalSnapshotBatch{}, err
	}
	fxCoverage, err := listFXHistoryCoverageQuery(ctx, query, householdID)
	if err != nil {
		return domain.HistoricalSnapshotBatch{}, err
	}
	var generation int
	var resolverPolicy sql.NullString
	if err := query.QueryRowContext(ctx, `SELECT input_generation, resolver_policy_version FROM history_snapshot_state WHERE household_id = ?`, householdID.String()).Scan(&generation, &resolverPolicy); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return domain.HistoricalSnapshotBatch{}, err
		}
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
func listZeroAccountBaselines(ctx context.Context, query queryer, householdID domain.HouseholdID, origin time.Time) ([]domain.AccountValue, error) {
	rows, err := query.QueryContext(ctx, `SELECT av.account_id, av.currency, av.effective_at
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
