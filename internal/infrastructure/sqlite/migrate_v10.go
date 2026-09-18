package sqlite

import (
	"context"
	"database/sql"
)

func migrateV10ToV11(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range []string{
		`ALTER TABLE instruments ADD COLUMN metal_template TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE instruments ADD COLUMN quantity_unit TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE instrument_quotes ADD COLUMN conversion_json TEXT`,
		`DROP INDEX ux_instruments_active_provider_binding`,
		`CREATE UNIQUE INDEX ux_instruments_active_provider_binding ON instruments(household_id, provider_key, provider_symbol) WHERE archived_at IS NULL AND provider_key IS NOT NULL AND provider_symbol IS NOT NULL AND metal_template = ''`,
		`CREATE UNIQUE INDEX ux_instruments_active_metal_binding ON instruments(household_id, provider_key, provider_symbol, quote_currency, quantity_unit) WHERE archived_at IS NULL AND provider_key IS NOT NULL AND provider_symbol IS NOT NULL AND metal_template <> ''`,
		`PRAGMA user_version = 11`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if err := verifySchema(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}
