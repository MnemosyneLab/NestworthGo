package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// SQLite cannot widen a CHECK in place. Rebuild only the eight tables whose
// quote-source constraint changes, preserving their other v14 definitions.
func migrateV14ToV15(ctx context.Context, db *sql.DB) error {
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
	for _, table := range []string{
		"instruments", "instrument_quotes", "fx_quotes", "fx_preferences",
		"history_origin_instrument_preferences", "history_origin_fx_preferences",
		"instrument_preference_observations", "fx_preference_observations",
	} {
		if err := widenQuoteSourceCheckTx(ctx, tx, table); err != nil {
			return fmt.Errorf("migrate %s: %w", table, err)
		}
	}
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return err
	}
	start := strings.Index(string(schema), "CREATE TABLE agent_quote_batches (")
	end := strings.Index(string(schema), "PRAGMA user_version = 15;")
	if start < 0 || end <= start {
		return errors.New("agent quote audit schema is missing")
	}
	if _, err := tx.ExecContext(ctx, string(schema)[start:end]); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `PRAGMA user_version = 15`); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	if rows.Next() {
		_ = rows.Close()
		return errors.New("schema 15 migration has foreign key violations")
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

func widenQuoteSourceCheckTx(ctx context.Context, tx *sql.Tx, table string) error {
	var definition string
	if err := tx.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&definition); err != nil {
		return err
	}
	oldCheck := "CHECK(source_kind IN ('manual','provider'))"
	newCheck := "CHECK(source_kind IN ('manual','provider','agent'))"
	if table == "instruments" {
		oldCheck = "CHECK(quote_source IN ('manual','provider'))"
		newCheck = "CHECK(quote_source IN ('manual','provider','agent'))"
	}
	if strings.Count(definition, oldCheck) != 1 {
		return fmt.Errorf("expected one v14 source constraint, got %d", strings.Count(definition, oldCheck))
	}
	definition = strings.Replace(definition, oldCheck, newCheck, 1)
	if table == "instruments" {
		oldBinding := "CHECK(quote_source = 'manual' OR (provider_key IS NOT NULL AND provider_symbol IS NOT NULL))"
		newBinding := "CHECK(quote_source <> 'provider' OR (provider_key IS NOT NULL AND provider_symbol IS NOT NULL))"
		if strings.Count(definition, oldBinding) != 1 {
			return errors.New("expected v14 provider binding constraint")
		}
		definition = strings.Replace(definition, oldBinding, newBinding, 1)
	}
	paren := strings.Index(definition, "(")
	if paren < 0 {
		return errors.New("table definition has no columns")
	}
	temporary := table + "_v15"
	if _, err := tx.ExecContext(ctx, `CREATE TABLE `+temporary+` `+definition[paren:]); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO `+temporary+` SELECT * FROM `+table); err != nil {
		return err
	}
	indexRows, err := tx.QueryContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND sql IS NOT NULL ORDER BY name`, table)
	if err != nil {
		return err
	}
	var indexes []string
	for indexRows.Next() {
		var statement string
		if err := indexRows.Scan(&statement); err != nil {
			_ = indexRows.Close()
			return err
		}
		indexes = append(indexes, statement)
	}
	if err := indexRows.Err(); err != nil {
		_ = indexRows.Close()
		return err
	}
	if err := indexRows.Close(); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE `+table); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE `+temporary+` RENAME TO `+table); err != nil {
		return err
	}
	for _, statement := range indexes {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}
