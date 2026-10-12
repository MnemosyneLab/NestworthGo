package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// This optional port is specific to the trend. P1 and snapshot loaders keep
// their existing behavior. All preflight and hydration use the same read tx.
func (r *Repository) LoadHistoricalOverviewSeriesBatch(ctx context.Context, cutoff time.Time, limits domain.HistoricalSeriesLimits) (domain.HistoricalSnapshotBatch, domain.HistoricalSeriesWork, error) {
	var work domain.HistoricalSeriesWork
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.HistoricalSnapshotBatch{}, work, err
	}
	defer tx.Rollback()
	q := &seriesQueryCounter{queryer: tx, work: &work}
	if limits.MaxFacts <= 0 || limits.MaxLeaves <= 0 || limits.MaxBytes <= 0 {
		return domain.HistoricalSnapshotBatch{}, work, &domain.Error{Code: domain.ErrHistoricalSeriesBudget, Message: "invalid series input budget"}
	}
	// Fixed schema tables only; never interpolate a user supplied identifier.
	// Snapshots and app configuration are neither counted nor read.
	tables := []string{"households", "members", "institutions", "account_groups", "accounts", "account_ownership", "account_values", "instruments", "holdings", "account_cash_values", "instrument_quotes", "fx_quotes", "fx_preferences", "history_origins", "history_origin_components", "history_origin_account_states", "history_origin_ownership", "history_origin_instrument_preferences", "history_origin_fx_preferences", "activities", "activity_effects", "activity_trade_details", "activity_dividend_details", "activity_correction_groups", "holding_quantity_values", "account_state_observations", "account_state_ownership", "instrument_preference_observations", "fx_preference_observations", "history_snapshot_state", "instrument_state_observations", "holding_state_observations", "instrument_provider_bindings", "instrument_provider_binding_revisions", "instrument_observation_slots", "fx_observation_slots", "market_data_day_status", "product_operations", "product_contracts", "liquidity_policies", "liquidity_reservations", "product_operation_products", "product_operation_activities", "product_operation_reservations", "agent_quote_batches", "agent_quote_records"}
	for _, table := range tables {
		if err := ctx.Err(); err != nil {
			return domain.HistoricalSnapshotBatch{}, work, err
		}
		info, err := q.QueryContext(ctx, "PRAGMA table_info("+table+")")
		if err != nil {
			return domain.HistoricalSnapshotBatch{}, work, err
		}
		var columns []string
		for info.Next() {
			var cid, notnull, pk int
			var name, kind string
			var def sql.NullString
			if err := info.Scan(&cid, &name, &kind, &notnull, &def, &pk); err != nil {
				info.Close()
				return domain.HistoricalSnapshotBatch{}, work, err
			}
			columns = append(columns, `COALESCE(length(CAST("`+strings.ReplaceAll(name, `"`, `""`)+`" AS BLOB)),0)`)
		}
		err = info.Err()
		info.Close()
		if err != nil {
			return domain.HistoricalSnapshotBatch{}, work, err
		}
		rows, err := q.QueryContext(ctx, "SELECT "+strings.Join(columns, "+")+" FROM "+table+" LIMIT ?", limits.MaxFacts-work.CandidateFacts+1)
		if err != nil {
			return domain.HistoricalSnapshotBatch{}, work, err
		}
		for rows.Next() {
			var bytes int64
			if err := rows.Scan(&bytes); err != nil {
				rows.Close()
				return domain.HistoricalSnapshotBatch{}, work, err
			}
			work.CandidateFacts++
			work.CandidateBytes += bytes
			if work.CandidateFacts > limits.MaxFacts || work.CandidateBytes > limits.MaxBytes {
				rows.Close()
				return domain.HistoricalSnapshotBatch{}, work, &domain.Error{Code: domain.ErrHistoricalSeriesBudget, Message: "retained facts exceed the initial local trend input budget"}
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return domain.HistoricalSnapshotBatch{}, work, err
		}
	}
	leaves, err := q.QueryContext(ctx, `SELECT 1 FROM (
 SELECT id FROM holdings UNION ALL SELECT id FROM accounts WHERE tracking_mode != 'holdings'
 UNION ALL SELECT account_id FROM account_cash_values GROUP BY account_id,currency) LIMIT ?`, limits.MaxLeaves+1)
	if err != nil {
		return domain.HistoricalSnapshotBatch{}, work, err
	}
	for leaves.Next() {
		work.Leaves++
	}
	err = leaves.Err()
	leaves.Close()
	if err != nil {
		return domain.HistoricalSnapshotBatch{}, work, err
	}
	if work.Leaves > limits.MaxLeaves {
		return domain.HistoricalSnapshotBatch{}, work, &domain.Error{Code: domain.ErrHistoricalSeriesBudget, Message: "retained component count exceeds the initial local trend input budget"}
	}
	var householdID string
	if err := q.QueryRowContext(ctx, `SELECT id FROM households WHERE singleton_key=1`).Scan(&householdID); err != nil {
		return domain.HistoricalSnapshotBatch{}, work, err
	}
	batch, err := loadHistoricalSeriesBatchQuery(ctx, q, domain.HouseholdID(householdID), cutoff)
	if err != nil {
		return domain.HistoricalSnapshotBatch{}, work, err
	}
	if err := ctx.Err(); err != nil {
		return domain.HistoricalSnapshotBatch{}, work, err
	}
	if err := tx.Commit(); err != nil {
		return domain.HistoricalSnapshotBatch{}, work, err
	}
	return batch, work, nil
}

type seriesQueryCounter struct {
	queryer
	work *domain.HistoricalSeriesWork
}

func (q *seriesQueryCounter) QueryContext(ctx context.Context, sql string, args ...any) (*sql.Rows, error) {
	q.work.Queries++
	return q.queryer.QueryContext(ctx, sql, args...)
}
func (q *seriesQueryCounter) QueryRowContext(ctx context.Context, sql string, args ...any) *sql.Row {
	q.work.Queries++
	return q.queryer.QueryRowContext(ctx, sql, args...)
}

// Historical replay does not use current quote selection. Avoid its potentially
// quadratic latest-FX query entirely, while retaining the same entity readers.
func readHistoricalSeriesPortfolio(ctx context.Context, q queryer, id domain.HouseholdID) (domain.PortfolioSnapshot, error) {
	var p domain.PortfolioSnapshot
	var err error
	p.Household, err = scanHousehold(q.QueryRowContext(ctx, `SELECT id,name,base_currency,created_at,updated_at FROM households WHERE singleton_key=1`))
	if err != nil {
		return p, err
	}
	p.Origin, err = historyOriginQuery(ctx, q, id)
	if err != nil {
		return p, err
	}
	p.Members, err = listMembersQuery(ctx, q, true)
	if err != nil {
		return p, err
	}
	p.Institutions, err = listInstitutionsQuery(ctx, q, true)
	if err != nil {
		return p, err
	}
	p.Groups, err = listGroupsQuery(ctx, q, true)
	if err != nil {
		return p, err
	}
	p.Accounts, err = listAccountRecords(ctx, q, id, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return p, err
	}
	p.Instruments, err = listInstrumentsQuery(ctx, q, id, true)
	if err != nil {
		return p, err
	}
	p.Holdings, err = listHoldingsQuery(ctx, q, id, true)
	if err != nil {
		return p, err
	}
	p.CashValues, err = listCashValuesQuery(ctx, q, id)
	if err != nil {
		return p, err
	}
	p.FXPreferences, err = listFXPreferencesQuery(ctx, q, id)
	return p, err
}

func loadHistoricalSeriesBatchQuery(ctx context.Context, query queryer, householdID domain.HouseholdID, cutoff time.Time) (domain.HistoricalSnapshotBatch, error) {
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
	portfolio, err := readHistoricalSeriesPortfolio(ctx, query, householdID)
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
	activities, err := listHistoricalSeriesActivities(ctx, query, householdID, cutoff)
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

// Bound SQLite variables without partitioning economic replay. Hydrate source
// rows in small batches, then apply absolute observations over the full order.
func listHistoricalSeriesActivities(ctx context.Context, q queryer, id domain.HouseholdID, cutoff time.Time) ([]domain.Activity, error) {
	rows, err := q.QueryContext(ctx, correctedActivityCTE+`SELECT id,household_id,kind,reason,effective_at,effective_local_date,created_at,note,reverses_activity_id,correction_group_id,transaction_fx_rate FROM economic_activities WHERE household_id=? AND effective_at<=? ORDER BY effective_at ASC,created_at ASC,ordering_id ASC`, id.String(), formatTimestamp(cutoff))
	if err != nil {
		return nil, err
	}
	activities := []domain.Activity{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		a, err := activityFromScanner(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		activities = append(activities, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for start := 0; start < len(activities); start += 256 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := start + 256
		if end > len(activities) {
			end = len(activities)
		}
		if err := hydrateActivities(ctx, q, activities[start:end]); err != nil {
			return nil, err
		}
	}
	var originID string
	if err := q.QueryRowContext(ctx, `SELECT id FROM history_origins WHERE household_id=?`, id.String()).Scan(&originID); err != nil {
		return nil, err
	}
	components, err := listHistoryOriginComponentsQuery(ctx, q, domain.HistoryOriginID(originID))
	if err != nil {
		return nil, err
	}
	return domain.ReplayMoneyEffectsContext(ctx, activities, components)
}
