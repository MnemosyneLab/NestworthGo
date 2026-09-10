package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func migrateV9ToV10(ctx context.Context, database *sql.DB) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, statement := range v10MigrationDDL {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("schema 10 ddl: %w", err)
		}
	}
	if err := backfillV10Observations(ctx, tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := backfillV10Bindings(ctx, tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := invalidateV10HistoryGenerations(ctx, tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", CurrentSchemaVersion)); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := verifySchema(ctx, tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func backfillV10Observations(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE instrument_quotes
		SET observation_kind = CASE WHEN source_kind = 'manual' THEN 'manual' ELSE 'legacy' END,
		    effective_date = CASE WHEN effective_date IS NULL OR effective_date = '' THEN substr(quoted_at, 1, 10) ELSE effective_date END,
		    revision = CASE WHEN revision IS NULL OR revision = 0 THEN 1 ELSE revision END
		WHERE observation_kind = ''`); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE fx_quotes
		SET observation_kind = CASE WHEN source_kind = 'manual' THEN 'manual' ELSE 'legacy' END,
		    effective_date = CASE WHEN effective_date IS NULL OR effective_date = '' THEN substr(quoted_at, 1, 10) ELSE effective_date END,
		    revision = CASE WHEN revision IS NULL OR revision = 0 THEN 1 ELSE revision END
		WHERE observation_kind = ''`)
	return err
}

func backfillV10Bindings(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO instrument_provider_bindings(
			instrument_id, provider_key, provider_symbol, market, currency, enabled, created_at, updated_at, binding_revision, effective_from
		)
		SELECT id, provider_key, provider_symbol, market_code, quote_currency, 1, created_at, updated_at, 1, created_at
		FROM instruments
		WHERE provider_key IS NOT NULL AND trim(provider_key) != ''
		  AND provider_symbol IS NOT NULL AND trim(provider_symbol) != ''`); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO instrument_provider_binding_revisions(
			instrument_id, provider_key, binding_revision, provider_symbol, market, currency, enabled, effective_from, created_at
		)
		SELECT instrument_id, provider_key, binding_revision, provider_symbol, market, currency, enabled, effective_from, created_at
		FROM instrument_provider_bindings`)
	return err
}

func invalidateV10HistoryGenerations(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT s.household_id, s.dirty_from, o.timezone, o.started_at, o.created_at
		FROM history_snapshot_state s
		JOIN history_origins o ON o.household_id = s.household_id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type row struct {
		householdID string
		dirtyFrom   sql.NullString
		timezone    string
		startedAt   string
		createdAt   string
	}
	var states []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.householdID, &item.dirtyFrom, &item.timezone, &item.startedAt, &item.createdAt); err != nil {
			return err
		}
		states = append(states, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, state := range states {
		origin, err := time.Parse(time.RFC3339Nano, state.startedAt)
		if err != nil {
			origin, err = time.Parse(time.RFC3339, state.startedAt)
			if err != nil {
				return &domain.Error{Code: domain.ErrIntegrity, Message: "stored history Starting point is invalid"}
			}
		}
		location, err := time.LoadLocation(state.timezone)
		if err != nil {
			return &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Message: "stored Household timezone is invalid"}
		}
		originDate := origin.In(location).Format("2006-01-02")
		dirtyFrom := originDate
		if state.dirtyFrom.Valid && state.dirtyFrom.String != "" && state.dirtyFrom.String < dirtyFrom {
			dirtyFrom = state.dirtyFrom.String
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE history_snapshot_state
			SET dirty_from = ?,
			    input_generation = input_generation + 1,
			    resolver_policy_version = ?,
			    updated_at = ?
			WHERE household_id = ?`, dirtyFrom, domain.MarketDataResolverPolicy, formatTimestamp(now), state.householdID); err != nil {
			return err
		}
	}
	return nil
}

var v10MigrationDDL = []string{
	`ALTER TABLE instrument_quotes ADD COLUMN observation_kind TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE instrument_quotes ADD COLUMN effective_date TEXT`,
	`ALTER TABLE instrument_quotes ADD COLUMN provider_timestamp TEXT`,
	`ALTER TABLE instrument_quotes ADD COLUMN fetched_at TEXT`,
	`ALTER TABLE instrument_quotes ADD COLUMN value_effective_at TEXT`,
	`ALTER TABLE instrument_quotes ADD COLUMN binding_revision INTEGER`,
	`ALTER TABLE instrument_quotes ADD COLUMN source_policy_version TEXT`,
	`ALTER TABLE instrument_quotes ADD COLUMN price_basis TEXT`,
	`ALTER TABLE instrument_quotes ADD COLUMN timestamp_basis TEXT`,
	`ALTER TABLE instrument_quotes ADD COLUMN revision INTEGER NOT NULL DEFAULT 1`,
	`ALTER TABLE instrument_quotes ADD COLUMN supersedes_quote_id TEXT`,
	`ALTER TABLE instrument_quotes ADD COLUMN split_factor TEXT`,
	`ALTER TABLE instrument_quotes ADD COLUMN dividend_cash TEXT`,
	`ALTER TABLE fx_quotes ADD COLUMN observation_kind TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE fx_quotes ADD COLUMN effective_date TEXT`,
	`ALTER TABLE fx_quotes ADD COLUMN fetched_at TEXT`,
	`ALTER TABLE fx_quotes ADD COLUMN value_effective_at TEXT`,
	`ALTER TABLE fx_quotes ADD COLUMN source_policy_version TEXT`,
	`ALTER TABLE fx_quotes ADD COLUMN timestamp_basis TEXT`,
	`ALTER TABLE fx_quotes ADD COLUMN revision INTEGER NOT NULL DEFAULT 1`,
	`ALTER TABLE fx_quotes ADD COLUMN supersedes_quote_id TEXT`,
	`ALTER TABLE history_snapshot_state ADD COLUMN dirty_to TEXT`,
	`ALTER TABLE history_snapshot_state ADD COLUMN input_generation INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE history_snapshot_state ADD COLUMN resolver_policy_version TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE daily_valuation_snapshots ADD COLUMN input_generation INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE daily_valuation_snapshots ADD COLUMN resolver_policy_version TEXT NOT NULL DEFAULT ''`,
	`CREATE TABLE instrument_provider_bindings (
    instrument_id TEXT NOT NULL,
    provider_key TEXT NOT NULL,
    provider_symbol TEXT NOT NULL,
    market TEXT,
    currency TEXT,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    binding_revision INTEGER NOT NULL,
    effective_from TEXT NOT NULL,
    PRIMARY KEY(instrument_id, provider_key),
    FOREIGN KEY(instrument_id) REFERENCES instruments(id) ON DELETE CASCADE
)`,
	`CREATE INDEX idx_instrument_provider_bindings_instrument ON instrument_provider_bindings(instrument_id)`,
	`CREATE TABLE instrument_provider_binding_revisions (
    instrument_id TEXT NOT NULL,
    provider_key TEXT NOT NULL,
    binding_revision INTEGER NOT NULL,
    provider_symbol TEXT NOT NULL,
    market TEXT,
    currency TEXT,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)),
    effective_from TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY(instrument_id, provider_key, binding_revision),
    FOREIGN KEY(instrument_id) REFERENCES instruments(id) ON DELETE CASCADE
)`,
	`CREATE TABLE instrument_observation_slots (
    instrument_id TEXT NOT NULL,
    provider_key TEXT NOT NULL,
    binding_revision INTEGER NOT NULL,
    source_policy_version TEXT NOT NULL,
    market_date TEXT NOT NULL,
    observation_kind TEXT NOT NULL,
    quote_id TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY(instrument_id, provider_key, binding_revision, source_policy_version, market_date, observation_kind),
    FOREIGN KEY(instrument_id) REFERENCES instruments(id) ON DELETE CASCADE,
    FOREIGN KEY(quote_id) REFERENCES instrument_quotes(id) ON DELETE RESTRICT
)`,
	`CREATE INDEX idx_instrument_observation_slots_quote ON instrument_observation_slots(quote_id)`,
	`CREATE TABLE fx_observation_slots (
    household_id TEXT NOT NULL,
    base_currency TEXT NOT NULL CHECK(base_currency GLOB '[A-Z][A-Z][A-Z]'),
    quote_currency TEXT NOT NULL CHECK(quote_currency GLOB '[A-Z][A-Z][A-Z]'),
    provider_key TEXT NOT NULL,
    source_policy_version TEXT NOT NULL,
    market_date TEXT NOT NULL,
    observation_kind TEXT NOT NULL,
    quote_id TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY(household_id, base_currency, quote_currency, provider_key, source_policy_version, market_date, observation_kind),
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE CASCADE,
    FOREIGN KEY(quote_id) REFERENCES fx_quotes(id) ON DELETE RESTRICT
)`,
	`CREATE INDEX idx_fx_observation_slots_quote ON fx_observation_slots(quote_id)`,
	`CREATE TABLE market_data_day_status (
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    provider_key TEXT NOT NULL,
    household_id TEXT NOT NULL,
    binding_revision INTEGER NOT NULL,
    source_policy_version TEXT NOT NULL,
    effective_date TEXT NOT NULL,
    status TEXT NOT NULL,
    reason TEXT NOT NULL,
    checked_at TEXT NOT NULL,
    next_check_at TEXT,
    expires_at TEXT,
    PRIMARY KEY(target_type, target_id, provider_key, household_id, binding_revision, source_policy_version, effective_date),
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE CASCADE
)`,
	`CREATE INDEX idx_market_data_day_status_household ON market_data_day_status(household_id, target_type, target_id, effective_date)`,
}
