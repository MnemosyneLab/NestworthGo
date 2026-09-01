package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const (
	cashOnHandBalanceOnlyCheck       = "account_type = 'cash_on_hand' AND balance_sheet_role = 'asset' AND tracking_mode = 'balance'"
	cashOnHandBalanceOrHoldingsCheck = "account_type = 'cash_on_hand' AND balance_sheet_role = 'asset' AND tracking_mode IN ('balance','holdings')"
)

// repairV9CashOnHandHoldingsCheck is the one approved exception to schema v9's
// no-auto-migrate rule: it rebuilds the accounts table solely to widen a CHECK.
// Existing rows already satisfy the new constraint, so no data is converted.
// Databases that already have the corrected CHECK are left unchanged.
func repairV9CashOnHandHoldingsCheck(ctx context.Context, database *sql.DB) error {
	return rewriteAccountsCheckFragment(ctx, database, cashOnHandBalanceOnlyCheck, cashOnHandBalanceOrHoldingsCheck)
}

func rewriteAccountsCheckFragment(ctx context.Context, database *sql.DB, from, to string) error {
	if database == nil {
		return fmt.Errorf("database is not open")
	}
	var definition string
	if err := database.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'accounts'`).Scan(&definition); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	if schemaSQLContains(definition, to) {
		return nil
	}
	if !schemaSQLContains(definition, from) {
		return nil
	}
	replaced, ok := replaceSchemaSQLFragment(definition, from, to)
	if !ok {
		return fmt.Errorf("accounts table check could not be rewritten")
	}
	createNew, ok := replaceCreateTableName(replaced, "accounts", "accounts_new")
	if !ok {
		return fmt.Errorf("accounts table create statement could not be renamed")
	}

	indexRows, err := database.QueryContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'index' AND tbl_name = 'accounts' AND sql IS NOT NULL`)
	if err != nil {
		return err
	}
	indexSQL := make([]string, 0, 4)
	for indexRows.Next() {
		var statement string
		if err := indexRows.Scan(&statement); err != nil {
			_ = indexRows.Close()
			return err
		}
		indexSQL = append(indexSQL, statement)
	}
	if err := indexRows.Err(); err != nil {
		_ = indexRows.Close()
		return err
	}
	_ = indexRows.Close()

	if _, err := database.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		_, _ = database.ExecContext(ctx, `PRAGMA foreign_keys = ON`)
		return err
	}
	if _, err := tx.ExecContext(ctx, createNew); err != nil {
		_ = tx.Rollback()
		_, _ = database.ExecContext(ctx, `PRAGMA foreign_keys = ON`)
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO accounts_new SELECT * FROM accounts`); err != nil {
		_ = tx.Rollback()
		_, _ = database.ExecContext(ctx, `PRAGMA foreign_keys = ON`)
		return err
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE accounts`); err != nil {
		_ = tx.Rollback()
		_, _ = database.ExecContext(ctx, `PRAGMA foreign_keys = ON`)
		return err
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE accounts_new RENAME TO accounts`); err != nil {
		_ = tx.Rollback()
		_, _ = database.ExecContext(ctx, `PRAGMA foreign_keys = ON`)
		return err
	}
	for _, statement := range indexSQL {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			_ = tx.Rollback()
			_, _ = database.ExecContext(ctx, `PRAGMA foreign_keys = ON`)
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		_, _ = database.ExecContext(ctx, `PRAGMA foreign_keys = ON`)
		return err
	}
	if _, err := database.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return err
	}
	return nil
}

func replaceSchemaSQLFragment(definition, from, to string) (string, bool) {
	normalizedFrom := normalizeSchemaSQL(from)
	compact := normalizeSchemaSQL(definition)
	index := strings.Index(compact, normalizedFrom)
	if index < 0 {
		return definition, false
	}
	// Walk the original definition with the same whitespace-skipping rules as
	// normalizeSchemaSQL so the replacement keeps surrounding formatting.
	runes := []rune(definition)
	seen := 0
	start := -1
	for i := 0; i < len(runes); i++ {
		if runes[i] == ' ' || runes[i] == '\n' || runes[i] == '\r' || runes[i] == '\t' {
			continue
		}
		if seen == index {
			start = i
			break
		}
		seen++
	}
	if start < 0 {
		return definition, false
	}
	needed := len([]rune(normalizedFrom))
	matched := 0
	end := start
	for end < len(runes) && matched < needed {
		if runes[end] == ' ' || runes[end] == '\n' || runes[end] == '\r' || runes[end] == '\t' {
			end++
			continue
		}
		matched++
		end++
	}
	if matched != needed {
		return definition, false
	}
	return string(runes[:start]) + to + string(runes[end:]), true
}

func replaceCreateTableName(definition, from, to string) (string, bool) {
	lower := strings.ToLower(definition)
	keyword := strings.Index(lower, "create table")
	if keyword < 0 {
		return definition, false
	}
	index := keyword + len("create table")
	for index < len(definition) {
		switch definition[index] {
		case ' ', '\n', '\r', '\t':
			index++
			continue
		}
		break
	}
	if index >= len(definition) {
		return definition, false
	}
	quote := byte(0)
	if definition[index] == '"' || definition[index] == '\'' || definition[index] == '`' {
		quote = definition[index]
		index++
	}
	start := index
	if quote != 0 {
		for index < len(definition) && definition[index] != quote {
			index++
		}
		if index >= len(definition) || !strings.EqualFold(definition[start:index], from) {
			return definition, false
		}
		return definition[:start] + to + definition[index:], true
	}
	for index < len(definition) {
		switch definition[index] {
		case ' ', '\n', '\r', '\t', '(':
			name := definition[start:index]
			if !strings.EqualFold(name, from) {
				return definition, false
			}
			return definition[:start] + to + definition[index:], true
		}
		index++
	}
	return definition, false
}
