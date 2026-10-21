package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func scanAgentClosure(row interface{ Scan(...any) error }) (*domain.InstrumentClosure, error) {
	var c domain.InstrumentClosure
	var at, created string
	if err := row.Scan(&c.ID, &c.InstrumentID, &c.Date, &at, &c.Timezone, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var err error
	c.EffectiveAt, err = parsePortfolioTime(at)
	if err != nil {
		return nil, err
	}
	c.CreatedAt, err = parsePortfolioTime(created)
	return &c, err
}
func agentClosureByID(ctx context.Context, q queryer, id string) (*domain.InstrumentClosure, error) {
	return scanAgentClosure(q.QueryRowContext(ctx, `SELECT id,instrument_id,effective_date,effective_at,timezone,created_at FROM agent_instrument_closures WHERE id=?`, id))
}
func listAgentClosuresQuery(ctx context.Context, q queryer, h domain.HouseholdID) ([]domain.InstrumentClosure, error) {
	if domain.IsBoundedRead(ctx) {
		if err := checkReadTableBudget(ctx, q, "agent_instrument_closures", "instrument_id IN (SELECT id FROM instruments WHERE household_id=?)", h.String()); err != nil {
			if schema21ClosureTableAbsent(ctx, q, err) {
				return nil, nil
			}
			return nil, err
		}
	}
	rows, err := q.QueryContext(ctx, `SELECT c.id,c.instrument_id,c.effective_date,c.effective_at,c.timezone,c.created_at FROM agent_instrument_closures c JOIN instruments i ON i.id=c.instrument_id JOIN agent_quote_records r ON r.quote_id=c.id AND r.target_type='instrument' WHERE i.household_id=? AND NOT EXISTS(SELECT 1 FROM agent_quote_records x WHERE x.target_type='instrument' AND x.target_quote_id=c.id) ORDER BY c.instrument_id,c.effective_date,c.id`, h.String())
	if err != nil {
		if schema21ClosureTableAbsent(ctx, q, err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []domain.InstrumentClosure
	for rows.Next() {
		c, err := scanAgentClosure(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}
func insertAgentClosureTx(ctx context.Context, tx *sql.Tx, h domain.HouseholdID, c domain.InstrumentClosure) error {
	if _, err := uuid.Parse(c.ID); err != nil {
		return err
	}
	if err := validateAgentEffectiveDate(c.Date); err != nil {
		return err
	}
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return err
	}
	if c.EffectiveAt.IsZero() || c.EffectiveAt.In(loc).Format("2006-01-02") != c.Date || c.CreatedAt.IsZero() || c.EffectiveAt.After(c.CreatedAt) {
		return agentQuoteError(domain.ErrValidation, "date", "invalid closure date")
	}
	var owner string
	var archived sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT household_id,archived_at FROM instruments WHERE id=?`, c.InstrumentID.String()).Scan(&owner, &archived); err != nil {
		return err
	}
	if owner != h.String() || archived.Valid {
		return agentQuoteError(domain.ErrValidation, "instrumentId", "instrument is outside the active household")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO agent_instrument_closures(id,instrument_id,effective_date,effective_at,timezone,created_at) VALUES(?,?,?,?,?,?)`, c.ID, c.InstrumentID.String(), c.Date, formatTimestamp(c.EffectiveAt), c.Timezone, formatTimestamp(c.CreatedAt))
	return err
}

func (r *Repository) ListInstrumentClosures(ctx context.Context, h domain.HouseholdID) ([]domain.InstrumentClosure, error) {
	return listAgentClosuresQuery(ctx, r.database.SQL, h)
}

// Verified schema21 backups remain readable without creating new tables or
// claiming facts that their schema could not store. Current schemas fail closed.
func schema21ClosureTableAbsent(ctx context.Context, q queryer, err error) bool {
	if err == nil || !strings.Contains(err.Error(), "no such table: agent_instrument_closures") {
		return false
	}
	var version int
	return q.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version) == nil && version == 21
}
