package sqlite

import (
	"context"
	"database/sql"
	"errors"
)

// SQLite cannot alter a CHECK constraint in place. Rebuild the activities and
// effects tables while foreign-key enforcement is paused on this one
// connection; the transaction and final verification preserve existing facts.
func migrateV13ToV14(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), `PRAGMA foreign_keys = ON`)
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range []string{
		`CREATE TABLE activities_v14 (
    id TEXT PRIMARY KEY NOT NULL,
    household_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK(kind IN ('cash_in','cash_out','cash_dividend','cash_transfer','fx_conversion','position_transfer','cost_adjustment','buy','sell','value_update','debt_draw','debt_payment','reversal')),
    reason TEXT NOT NULL,
    effective_at TEXT NOT NULL,
    effective_local_date TEXT NOT NULL,
    created_at TEXT NOT NULL,
    note TEXT,
    reverses_activity_id TEXT UNIQUE,
    correction_group_id TEXT,
    transaction_fx_rate TEXT,
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE RESTRICT,
    FOREIGN KEY(reverses_activity_id) REFERENCES activities(id) ON DELETE RESTRICT
)`,
		`INSERT INTO activities_v14(id, household_id, kind, reason, effective_at, effective_local_date, created_at, note, reverses_activity_id, correction_group_id, transaction_fx_rate)
 SELECT id, household_id, kind, reason, effective_at, effective_local_date, created_at, note, reverses_activity_id, correction_group_id, transaction_fx_rate FROM activities`,
		`DROP TABLE activities`,
		`ALTER TABLE activities_v14 RENAME TO activities`,
		`CREATE INDEX idx_activities_timeline ON activities(household_id, effective_at DESC, created_at DESC, id DESC)`,
		`CREATE INDEX idx_activities_local_date ON activities(household_id, effective_local_date, id)`,
		`CREATE TABLE activity_effects_v14 (
    id TEXT PRIMARY KEY NOT NULL,
    activity_id TEXT NOT NULL,
    sequence INTEGER NOT NULL CHECK(sequence > 0),
    role TEXT NOT NULL,
    direction TEXT NOT NULL CHECK(direction IN ('added','removed')),
    target TEXT NOT NULL CHECK(target IN ('account_value','account_cash','holding_quantity','holding_cost')),
    classification TEXT NOT NULL,
    account_id TEXT,
    holding_id TEXT,
    instrument_id TEXT,
    amount TEXT,
    currency TEXT,
    quantity TEXT, cost_unit_price TEXT,
    FOREIGN KEY(activity_id) REFERENCES activities(id) ON DELETE RESTRICT,
    FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE RESTRICT,
    FOREIGN KEY(holding_id) REFERENCES holdings(id) ON DELETE RESTRICT,
    FOREIGN KEY(instrument_id) REFERENCES instruments(id) ON DELETE RESTRICT,
    UNIQUE(activity_id, sequence),
    CHECK((target IN ('account_value','account_cash') AND account_id IS NOT NULL AND amount IS NOT NULL AND currency IS NOT NULL AND quantity IS NULL) OR
          (target = 'holding_quantity' AND holding_id IS NOT NULL AND instrument_id IS NOT NULL AND quantity IS NOT NULL AND amount IS NULL AND currency IS NULL) OR
          (target = 'holding_cost' AND account_id IS NULL AND holding_id IS NOT NULL AND instrument_id IS NOT NULL AND quantity IS NULL AND amount IS NULL AND currency IS NULL AND cost_unit_price IS NOT NULL))
)`,
		`INSERT INTO activity_effects_v14(id, activity_id, sequence, role, direction, target, classification, account_id, holding_id, instrument_id, amount, currency, quantity, cost_unit_price)
 SELECT id, activity_id, sequence, role, direction, target, classification, account_id, holding_id, instrument_id, amount, currency, quantity, cost_unit_price FROM activity_effects`,
		`DROP TABLE activity_effects`,
		`ALTER TABLE activity_effects_v14 RENAME TO activity_effects`,
		`CREATE INDEX idx_activity_effects_activity ON activity_effects(activity_id, sequence)`,
		`CREATE INDEX idx_activity_effects_account ON activity_effects(account_id, activity_id, sequence)`,
		`CREATE INDEX idx_activity_effects_holding ON activity_effects(holding_id, activity_id, sequence)`,
		`PRAGMA user_version = 14`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	if rows.Next() {
		_ = rows.Close()
		return errors.New("schema 14 migration has foreign key violations")
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	return tx.Commit()
}
