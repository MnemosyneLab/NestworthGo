package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// ensureCurrentHistoricalResolverPolicy is a data migration for changes to
// historical valuation semantics. The schema is unchanged, but snapshots
// calculated under an older resolver must not remain publishable as current
// results. Marking the state dirty and invalidating completeness, while
// preserving content_hash, lets the normal resumable rebuild replace them
// with the new market-date summary. Same-hash reuse in
// saveDailyValuationSnapshotTx must persist the rebuilt completeness and
// related state because the economic payload can be unchanged.
func ensureCurrentHistoricalResolverPolicy(ctx context.Context, database *sql.DB) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		SELECT s.household_id, s.dirty_from, s.dirty_to, s.input_generation,
		       s.resolver_policy_version, o.timezone, o.started_at
		FROM history_snapshot_state s
		JOIN history_origins o ON o.household_id = s.household_id
		WHERE COALESCE(s.resolver_policy_version, '') <> ?`, domain.MarketDataResolverPolicy)
	if err != nil {
		return err
	}
	type staleState struct {
		householdID    string
		dirtyFrom      sql.NullString
		dirtyTo        sql.NullString
		generation     int
		resolverPolicy string
		timezone       string
		startedAt      string
	}
	var stale []staleState
	for rows.Next() {
		var state staleState
		if err := rows.Scan(&state.householdID, &state.dirtyFrom, &state.dirtyTo, &state.generation, &state.resolverPolicy, &state.timezone, &state.startedAt); err != nil {
			_ = rows.Close()
			return err
		}
		stale = append(stale, state)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	now := time.Now().UTC()
	for _, state := range stale {
		location, err := time.LoadLocation(state.timezone)
		if err != nil {
			return err
		}
		startedAt, err := parseStoredTime(state.startedAt)
		if err != nil {
			return err
		}
		originDate := startedAt.In(location).Format("2006-01-02")
		dirtyTo := ""
		if state.dirtyTo.Valid {
			dirtyTo = state.dirtyTo.String
		}
		lastClosed := lastClosedLocalDate(now, location)
		if dirtyTo == "" || lastClosed > dirtyTo {
			dirtyTo = lastClosed
		}
		if dirtyTo == "" || dirtyTo < originDate {
			dirtyTo = originDate
		}

		generation := state.generation + 1
		if _, err := tx.ExecContext(ctx, `
			UPDATE history_snapshot_state
			SET dirty_from = ?, dirty_to = ?, input_generation = ?,
			    resolver_policy_version = ?, updated_at = ?
			WHERE household_id = ?`,
			originDate, dirtyTo, generation, domain.MarketDataResolverPolicy,
			formatTimestamp(now), state.householdID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE daily_valuation_snapshots
			SET complete = 0, input_generation = ?
			WHERE household_id = ?`, generation, state.householdID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func parseStoredTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, value)
	}
	return parsed, err
}
