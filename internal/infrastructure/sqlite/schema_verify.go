package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type schemaQuery interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type schemaColumn struct {
	name       string
	typeName   string
	notNull    int
	defaultVal string
	primaryKey int
}

type expectedIndexColumn struct {
	name string
	desc int
}

type expectedIndex struct {
	table   string
	name    string
	unique  int
	partial int
	where   string
	columns []expectedIndexColumn
}

type expectedForeignKey struct {
	table    string
	refTable string
	from     string
	to       string
	onDelete string
}

func verifySchema(ctx context.Context, query schemaQuery) error {
	var version int
	if err := query.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version != CurrentSchemaVersion {
		return fmt.Errorf("database schema version is %d, want %d", version, CurrentSchemaVersion)
	}
	var foreignKeys int
	if err := query.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return err
	}
	if foreignKeys != 1 {
		return fmt.Errorf("foreign keys are disabled")
	}
	var integrity string
	if err := query.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return fmt.Errorf("integrity check returned %q", integrity)
	}

	tables := expectedSchemaTables()
	tableNames := make([]string, 0, len(tables))
	for name := range tables {
		tableNames = append(tableNames, name)
	}
	sort.Strings(tableNames)
	for _, table := range tableNames {
		if err := verifyTable(ctx, query, table, tables[table]); err != nil {
			return err
		}
	}

	for _, index := range expectedSchemaIndexes() {
		if err := verifyIndex(ctx, query, index); err != nil {
			return err
		}
	}
	for _, foreignKey := range expectedSchemaForeignKeys() {
		if err := verifyForeignKeys(ctx, query, foreignKey); err != nil {
			return err
		}
	}
	if err := verifyHistorySchema(ctx, query); err != nil {
		return err
	}
	if err := verifyCostBasisInvariant(ctx, query); err != nil {
		return err
	}
	if err := verifyAccountModelInvariants(ctx, query); err != nil {
		return err
	}
	rows, err := query.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("foreign key check returned violations")
	}
	return rows.Err()
}

func verifyCostBasisInvariant(ctx context.Context, query schemaQuery) error {
	var missing int
	err := query.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM holdings h
		JOIN accounts a ON a.id = h.account_id
		JOIN history_origins o ON o.household_id = a.household_id
		WHERE CAST(h.quantity AS REAL) > 0
		  AND NOT EXISTS (
			SELECT 1
			FROM history_origin_components c
			WHERE c.origin_id = o.id
			  AND c.holding_id = h.id
			  AND c.instrument_id = h.instrument_id
			  AND c.component_kind = 'holding_quantity'
			  AND CAST(c.quantity AS REAL) > 0
			  AND c.unit_cost IS NOT NULL
		  )
		  AND NOT EXISTS (
			SELECT 1
			FROM activity_trade_details td
			JOIN activities trade ON trade.id = td.activity_id
			WHERE td.holding_id = h.id
			  AND trade.reverses_activity_id IS NULL
			  AND NOT EXISTS (SELECT 1 FROM activities reversal WHERE reversal.reverses_activity_id = trade.id)
		  )
		  AND NOT EXISTS (
			SELECT 1
			FROM activity_effects effect
			JOIN activities transfer ON transfer.id = effect.activity_id
			WHERE effect.holding_id = h.id
			  AND effect.direction = 'added'
			  AND transfer.kind = 'position_transfer'
		  )
		  AND NOT EXISTS (
			SELECT 1
			FROM activity_effects effect
			WHERE effect.holding_id = h.id
			  AND effect.cost_unit_price IS NOT NULL
		  )`).Scan(&missing)
	if err != nil {
		return err
	}
	if missing != 0 {
		return fmt.Errorf("%d positive Holding rows have no resolvable cost basis", missing)
	}
	return nil
}

func verifyAccountModelInvariants(ctx context.Context, query schemaQuery) error {
	var missingOwnership int
	if err := query.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts a WHERE NOT EXISTS (SELECT 1 FROM account_ownership o WHERE o.account_id = a.id)`).Scan(&missingOwnership); err != nil {
		return err
	}
	if missingOwnership != 0 {
		return fmt.Errorf("%d accounts have no ownership rows", missingOwnership)
	}
	var badOwnership int
	if err := query.QueryRowContext(ctx, `SELECT COUNT(*) FROM (SELECT account_id FROM account_ownership GROUP BY account_id HAVING SUM(share_bps) != 10000)`).Scan(&badOwnership); err != nil {
		return err
	}
	if badOwnership != 0 {
		return fmt.Errorf("%d accounts have ownership shares that do not total 10000 basis points", badOwnership)
	}
	var badHoldings int
	if err := query.QueryRowContext(ctx, `SELECT COUNT(*) FROM holdings h JOIN accounts a ON a.id = h.account_id WHERE a.tracking_mode != 'holdings'`).Scan(&badHoldings); err != nil {
		return err
	}
	if badHoldings != 0 {
		return fmt.Errorf("%d holdings belong to a non-holdings account", badHoldings)
	}
	var badCash int
	if err := query.QueryRowContext(ctx, `SELECT COUNT(*) FROM account_cash_values c JOIN accounts a ON a.id = c.account_id WHERE a.tracking_mode != 'holdings'`).Scan(&badCash); err != nil {
		return err
	}
	if badCash != 0 {
		return fmt.Errorf("%d cash observations belong to a non-holdings account", badCash)
	}
	var badValues int
	if err := query.QueryRowContext(ctx, `SELECT COUNT(*) FROM account_values v JOIN accounts a ON a.id = v.account_id WHERE a.tracking_mode = 'holdings'`).Scan(&badValues); err != nil {
		return err
	}
	if badValues != 0 {
		return fmt.Errorf("%d account values belong to a holdings account", badValues)
	}
	rows, err := query.QueryContext(ctx, `SELECT account_type, balance_sheet_role, tracking_mode FROM accounts`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var accountType, role, tracking string
		if err := rows.Scan(&accountType, &role, &tracking); err != nil {
			return err
		}
		parsedType, err := domain.ParseAccountType(accountType)
		if err != nil {
			return fmt.Errorf("stored account type %q is invalid", accountType)
		}
		parsedRole, err := domain.ParseBalanceSheetRole(role)
		if err != nil {
			return fmt.Errorf("stored balance sheet role %q is invalid", role)
		}
		parsedTracking, err := domain.ParseTrackingMode(tracking)
		if err != nil {
			return fmt.Errorf("stored tracking mode %q is invalid", tracking)
		}
		if !domain.IsValidAccountCombination(parsedType, parsedRole, parsedTracking) {
			return fmt.Errorf("stored account combination %s/%s/%s is not legal", accountType, role, tracking)
		}
	}
	return rows.Err()
}

func verifyTable(ctx context.Context, query schemaQuery, table string, expected []schemaColumn) error {
	var objectType string
	if err := query.QueryRowContext(ctx, `SELECT type FROM sqlite_master WHERE name = ?`, table).Scan(&objectType); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("required table %s is missing", table)
		}
		return err
	}
	if objectType != "table" {
		return fmt.Errorf("schema object %s is %s, want table", table, objectType)
	}
	rows, err := query.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info("%s")`, table))
	if err != nil {
		return err
	}
	actual := make([]schemaColumn, 0, len(expected))
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, typeName string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &typeName, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return err
		}
		value := ""
		if defaultValue.Valid {
			value = defaultValue.String
		}
		actual = append(actual, schemaColumn{name: name, typeName: strings.ToUpper(strings.TrimSpace(typeName)), notNull: notNull, defaultVal: value, primaryKey: primaryKey})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	if len(actual) != len(expected) {
		return fmt.Errorf("table %s has %d columns, want %d", table, len(actual), len(expected))
	}
	for index, want := range expected {
		got := actual[index]
		want.typeName = strings.ToUpper(strings.TrimSpace(want.typeName))
		if got != want {
			return fmt.Errorf("table %s column %d definition is %+v, want %+v", table, index, got, want)
		}
	}
	for _, check := range expectedSchemaChecks()[table] {
		var definition string
		if err := query.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&definition); err != nil {
			return err
		}
		if !schemaSQLContains(definition, check) {
			return fmt.Errorf("table %s is missing required check %q", table, check)
		}
	}
	return nil
}

func verifyIndex(ctx context.Context, query schemaQuery, expected expectedIndex) error {
	rows, err := query.QueryContext(ctx, fmt.Sprintf(`PRAGMA index_list("%s")`, expected.table))
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var sequence, unique, partial int
		var name, origin string
		if err := rows.Scan(&sequence, &name, &unique, &origin, &partial); err != nil {
			_ = rows.Close()
			return err
		}
		if name != expected.name {
			continue
		}
		found = true
		if unique != expected.unique || partial != expected.partial {
			_ = rows.Close()
			return fmt.Errorf("index %s definition flags are unique=%d partial=%d, want unique=%d partial=%d", expected.name, unique, partial, expected.unique, expected.partial)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	if !found {
		return fmt.Errorf("required index %s is missing from %s", expected.name, expected.table)
	}

	indexRows, err := query.QueryContext(ctx, fmt.Sprintf(`PRAGMA index_xinfo("%s")`, expected.name))
	if err != nil {
		return err
	}
	actualColumns := make([]expectedIndexColumn, 0, len(expected.columns))
	for indexRows.Next() {
		var sequence, cid, descending, key int
		var name sql.NullString
		var collation string
		if err := indexRows.Scan(&sequence, &cid, &name, &descending, &collation, &key); err != nil {
			_ = indexRows.Close()
			return err
		}
		if key == 0 {
			continue
		}
		if !name.Valid {
			_ = indexRows.Close()
			return fmt.Errorf("index %s contains an unnamed key column", expected.name)
		}
		actualColumns = append(actualColumns, expectedIndexColumn{name: name.String, desc: descending})
	}
	if err := indexRows.Err(); err != nil {
		_ = indexRows.Close()
		return err
	}
	_ = indexRows.Close()
	if len(actualColumns) != len(expected.columns) {
		return fmt.Errorf("index %s has %d key columns, want %d", expected.name, len(actualColumns), len(expected.columns))
	}
	for index, want := range expected.columns {
		if actualColumns[index] != want {
			return fmt.Errorf("index %s column %d is %+v, want %+v", expected.name, index, actualColumns[index], want)
		}
	}
	if expected.where != "" {
		var definition sql.NullString
		if err := query.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?`, expected.name).Scan(&definition); err != nil {
			return err
		}
		if !definition.Valid || normalizedIndexPredicate(definition.String) != normalizeSchemaSQL(expected.where) {
			return fmt.Errorf("index %s is missing required predicate", expected.name)
		}
	}
	return nil
}

func verifyForeignKeys(ctx context.Context, query schemaQuery, expected expectedForeignKey) error {
	rows, err := query.QueryContext(ctx, fmt.Sprintf(`PRAGMA foreign_key_list("%s")`, expected.table))
	if err != nil {
		return err
	}
	actual := make(map[string]bool)
	for rows.Next() {
		var id, sequence int
		var table, from, to, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &sequence, &table, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			_ = rows.Close()
			return err
		}
		actual[foreignKeyKey(table, from, to, onDelete)] = true
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	want := foreignKeyKey(expected.refTable, expected.from, expected.to, expected.onDelete)
	if !actual[want] {
		return fmt.Errorf("table %s is missing foreign key %s.%s -> %s.%s ON DELETE %s", expected.table, expected.table, expected.from, expected.refTable, expected.to, expected.onDelete)
	}
	return nil
}

func foreignKeyKey(table, from, to, onDelete string) string {
	return strings.Join([]string{table, from, to, strings.ToUpper(strings.TrimSpace(onDelete))}, "|")
}

func schemaSQLContains(definition, fragment string) bool {
	return strings.Contains(normalizeSchemaSQL(definition), normalizeSchemaSQL(fragment))
}

func normalizeSchemaSQL(value string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, strings.ToLower(value))
}

func normalizedIndexPredicate(definition string) string {
	compact := strings.TrimSuffix(normalizeSchemaSQL(definition), ";")
	where := strings.Index(compact, "where")
	if where < 0 {
		return ""
	}
	return compact[where:]
}

func expectedColumn(name, typeName string, notNull, primaryKey int, defaultVal ...string) schemaColumn {
	value := ""
	if len(defaultVal) > 0 {
		value = defaultVal[0]
	}
	return schemaColumn{name: name, typeName: typeName, notNull: notNull, primaryKey: primaryKey, defaultVal: value}
}

func expectedSchemaTables() map[string][]schemaColumn {
	return map[string][]schemaColumn{
		"households": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("singleton_key", "INTEGER", 1, 0, "1"), expectedColumn("name", "TEXT", 1, 0),
			expectedColumn("base_currency", "TEXT", 1, 0), expectedColumn("created_at", "TEXT", 1, 0), expectedColumn("updated_at", "TEXT", 1, 0),
		},
		"media_assets": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("household_id", "TEXT", 1, 0), expectedColumn("mime_type", "TEXT", 1, 0),
			expectedColumn("data", "BLOB", 1, 0), expectedColumn("created_at", "TEXT", 1, 0),
		},
		"members": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("household_id", "TEXT", 1, 0), expectedColumn("name", "TEXT", 1, 0),
			expectedColumn("avatar_asset_id", "TEXT", 0, 0), expectedColumn("note", "TEXT", 0, 0), expectedColumn("sort_order", "INTEGER", 1, 0, "0"),
			expectedColumn("created_at", "TEXT", 1, 0), expectedColumn("updated_at", "TEXT", 1, 0), expectedColumn("archived_at", "TEXT", 0, 0),
		},
		"institutions": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("household_id", "TEXT", 1, 0), expectedColumn("name", "TEXT", 1, 0),
			expectedColumn("institution_type", "TEXT", 0, 0), expectedColumn("country_code", "TEXT", 0, 0), expectedColumn("website", "TEXT", 0, 0),
			expectedColumn("note", "TEXT", 0, 0), expectedColumn("logo_asset_id", "TEXT", 0, 0), expectedColumn("sort_order", "INTEGER", 1, 0, "0"),
			expectedColumn("created_at", "TEXT", 1, 0), expectedColumn("updated_at", "TEXT", 1, 0), expectedColumn("archived_at", "TEXT", 0, 0), expectedColumn("icon_key", "TEXT", 0, 0),
		},
		"account_groups": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("household_id", "TEXT", 1, 0), expectedColumn("name", "TEXT", 1, 0), expectedColumn("icon_key", "TEXT", 0, 0),
			expectedColumn("color", "TEXT", 0, 0), expectedColumn("logo_asset_id", "TEXT", 0, 0), expectedColumn("description", "TEXT", 0, 0), expectedColumn("sort_order", "INTEGER", 1, 0, "0"),
			expectedColumn("created_at", "TEXT", 1, 0), expectedColumn("updated_at", "TEXT", 1, 0), expectedColumn("archived_at", "TEXT", 0, 0),
		},
		"accounts": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("household_id", "TEXT", 1, 0), expectedColumn("institution_id", "TEXT", 0, 0), expectedColumn("group_id", "TEXT", 0, 0),
			expectedColumn("name", "TEXT", 1, 0), expectedColumn("account_type", "TEXT", 1, 0), expectedColumn("balance_sheet_role", "TEXT", 1, 0), expectedColumn("tracking_mode", "TEXT", 1, 0),
			expectedColumn("default_currency", "TEXT", 1, 0), expectedColumn("note", "TEXT", 0, 0), expectedColumn("logo_asset_id", "TEXT", 0, 0),
			expectedColumn("include_in_net_worth", "INTEGER", 1, 0, "1"), expectedColumn("include_in_portfolio", "INTEGER", 1, 0, "0"), expectedColumn("include_in_liquid_assets", "INTEGER", 1, 0, "0"),
			expectedColumn("opened_on", "TEXT", 0, 0), expectedColumn("closed_on", "TEXT", 0, 0), expectedColumn("sort_order", "INTEGER", 1, 0, "0"),
			expectedColumn("created_at", "TEXT", 1, 0), expectedColumn("updated_at", "TEXT", 1, 0), expectedColumn("archived_at", "TEXT", 0, 0), expectedColumn("icon_key", "TEXT", 0, 0),
		},
		"account_ownership": {
			expectedColumn("account_id", "TEXT", 1, 1), expectedColumn("member_id", "TEXT", 1, 2), expectedColumn("share_bps", "INTEGER", 1, 0),
		},
		"account_values": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("account_id", "TEXT", 1, 0), expectedColumn("value_kind", "TEXT", 1, 0), expectedColumn("amount", "TEXT", 1, 0),
			expectedColumn("currency", "TEXT", 1, 0), expectedColumn("effective_at", "TEXT", 1, 0), expectedColumn("created_at", "TEXT", 1, 0), expectedColumn("activity_effect_id", "TEXT", 0, 0), expectedColumn("projection_kind", "TEXT", 1, 0, "'baseline'"),
		},
		"instruments": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("household_id", "TEXT", 1, 0), expectedColumn("name", "TEXT", 1, 0), expectedColumn("instrument_type", "TEXT", 1, 0),
			expectedColumn("quote_currency", "TEXT", 1, 0), expectedColumn("symbol", "TEXT", 0, 0), expectedColumn("market_code", "TEXT", 0, 0), expectedColumn("country_code", "TEXT", 0, 0),
			expectedColumn("isin", "TEXT", 0, 0), expectedColumn("note", "TEXT", 0, 0), expectedColumn("logo_asset_id", "TEXT", 0, 0), expectedColumn("sort_order", "INTEGER", 1, 0, "0"),
			expectedColumn("quote_source", "TEXT", 1, 0, "'manual'"), expectedColumn("provider_key", "TEXT", 0, 0), expectedColumn("provider_symbol", "TEXT", 0, 0),
			expectedColumn("created_at", "TEXT", 1, 0), expectedColumn("updated_at", "TEXT", 1, 0), expectedColumn("archived_at", "TEXT", 0, 0),
		},
		"holdings": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("account_id", "TEXT", 1, 0), expectedColumn("instrument_id", "TEXT", 1, 0), expectedColumn("quantity", "TEXT", 1, 0),
			expectedColumn("note", "TEXT", 0, 0), expectedColumn("sort_order", "INTEGER", 1, 0, "0"), expectedColumn("created_at", "TEXT", 1, 0), expectedColumn("updated_at", "TEXT", 1, 0), expectedColumn("archived_at", "TEXT", 0, 0),
		},
		"account_cash_values": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("account_id", "TEXT", 1, 0), expectedColumn("amount", "TEXT", 1, 0), expectedColumn("currency", "TEXT", 1, 0), expectedColumn("effective_at", "TEXT", 1, 0), expectedColumn("created_at", "TEXT", 1, 0), expectedColumn("activity_effect_id", "TEXT", 0, 0), expectedColumn("projection_kind", "TEXT", 1, 0, "'baseline'"),
		},
		"instrument_quotes": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("instrument_id", "TEXT", 1, 0), expectedColumn("unit_price", "TEXT", 1, 0), expectedColumn("currency", "TEXT", 1, 0), expectedColumn("source_kind", "TEXT", 1, 0), expectedColumn("source_key", "TEXT", 1, 0), expectedColumn("quoted_at", "TEXT", 1, 0), expectedColumn("created_at", "TEXT", 1, 0), expectedColumn("delayed", "INTEGER", 1, 0, "0"),
		},
		"fx_quotes": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("household_id", "TEXT", 1, 0), expectedColumn("base_currency", "TEXT", 1, 0), expectedColumn("quote_currency", "TEXT", 1, 0), expectedColumn("rate", "TEXT", 1, 0), expectedColumn("source_kind", "TEXT", 1, 0), expectedColumn("source_key", "TEXT", 1, 0), expectedColumn("quoted_at", "TEXT", 1, 0), expectedColumn("created_at", "TEXT", 1, 0), expectedColumn("delayed", "INTEGER", 1, 0, "0"),
		},
		"fx_preferences": {
			expectedColumn("household_id", "TEXT", 1, 1), expectedColumn("currency_a", "TEXT", 1, 2), expectedColumn("currency_b", "TEXT", 1, 3), expectedColumn("source_kind", "TEXT", 1, 0), expectedColumn("created_at", "TEXT", 1, 0), expectedColumn("updated_at", "TEXT", 1, 0),
		},
	}
}

func historySchemaColumns() map[string][]schemaColumn {
	return map[string][]schemaColumn{
		"history_origins": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("household_id", "TEXT", 1, 0), expectedColumn("timezone", "TEXT", 1, 0), expectedColumn("started_at", "TEXT", 1, 0), expectedColumn("created_at", "TEXT", 1, 0),
		},
		"history_origin_components": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("origin_id", "TEXT", 1, 0), expectedColumn("component_kind", "TEXT", 1, 0), expectedColumn("account_id", "TEXT", 0, 0), expectedColumn("holding_id", "TEXT", 0, 0), expectedColumn("instrument_id", "TEXT", 0, 0), expectedColumn("amount", "TEXT", 0, 0), expectedColumn("currency", "TEXT", 0, 0), expectedColumn("quantity", "TEXT", 0, 0), expectedColumn("created_at", "TEXT", 1, 0), expectedColumn("unit_cost", "TEXT", 0, 0),
		},
		"history_origin_account_states": {
			expectedColumn("origin_id", "TEXT", 1, 1), expectedColumn("account_id", "TEXT", 1, 2), expectedColumn("archived_at", "TEXT", 0, 0), expectedColumn("include_in_net_worth", "INTEGER", 1, 0), expectedColumn("include_in_portfolio", "INTEGER", 1, 0), expectedColumn("include_in_liquid_assets", "INTEGER", 1, 0), expectedColumn("created_at", "TEXT", 1, 0),
		},
		"history_origin_ownership": {
			expectedColumn("origin_id", "TEXT", 1, 1), expectedColumn("account_id", "TEXT", 1, 2), expectedColumn("member_id", "TEXT", 1, 3), expectedColumn("share_bps", "INTEGER", 1, 0),
		},
		"history_origin_instrument_preferences": {
			expectedColumn("origin_id", "TEXT", 1, 1), expectedColumn("instrument_id", "TEXT", 1, 2), expectedColumn("source_kind", "TEXT", 1, 0), expectedColumn("created_at", "TEXT", 1, 0),
		},
		"history_origin_fx_preferences": {
			expectedColumn("origin_id", "TEXT", 1, 1), expectedColumn("currency_a", "TEXT", 1, 2), expectedColumn("currency_b", "TEXT", 1, 3), expectedColumn("source_kind", "TEXT", 1, 0), expectedColumn("created_at", "TEXT", 1, 0),
		},
		"activities": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("household_id", "TEXT", 1, 0), expectedColumn("kind", "TEXT", 1, 0), expectedColumn("reason", "TEXT", 1, 0), expectedColumn("effective_at", "TEXT", 1, 0), expectedColumn("effective_local_date", "TEXT", 1, 0), expectedColumn("created_at", "TEXT", 1, 0), expectedColumn("note", "TEXT", 0, 0), expectedColumn("reverses_activity_id", "TEXT", 0, 0), expectedColumn("correction_group_id", "TEXT", 0, 0), expectedColumn("transaction_fx_rate", "TEXT", 0, 0),
		},
		"activity_effects": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("activity_id", "TEXT", 1, 0), expectedColumn("sequence", "INTEGER", 1, 0), expectedColumn("role", "TEXT", 1, 0), expectedColumn("direction", "TEXT", 1, 0), expectedColumn("target", "TEXT", 1, 0), expectedColumn("classification", "TEXT", 1, 0), expectedColumn("account_id", "TEXT", 0, 0), expectedColumn("holding_id", "TEXT", 0, 0), expectedColumn("instrument_id", "TEXT", 0, 0), expectedColumn("amount", "TEXT", 0, 0), expectedColumn("currency", "TEXT", 0, 0), expectedColumn("quantity", "TEXT", 0, 0), expectedColumn("cost_unit_price", "TEXT", 0, 0),
		},
		"activity_trade_details": {
			expectedColumn("activity_id", "TEXT", 1, 1), expectedColumn("side", "TEXT", 1, 0), expectedColumn("instrument_id", "TEXT", 1, 0), expectedColumn("holding_id", "TEXT", 1, 0), expectedColumn("quantity", "TEXT", 1, 0), expectedColumn("gross_amount", "TEXT", 1, 0), expectedColumn("gross_currency", "TEXT", 1, 0), expectedColumn("unit_price", "TEXT", 1, 0), expectedColumn("fee_amount", "TEXT", 0, 0), expectedColumn("fee_currency", "TEXT", 0, 0),
		},
		"activity_correction_groups": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("household_id", "TEXT", 1, 0), expectedColumn("original_activity_id", "TEXT", 1, 0), expectedColumn("replacement_activity_id", "TEXT", 0, 0), expectedColumn("created_at", "TEXT", 1, 0),
		},
		"holding_quantity_values": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("holding_id", "TEXT", 1, 0), expectedColumn("quantity", "TEXT", 1, 0), expectedColumn("effective_at", "TEXT", 1, 0), expectedColumn("created_at", "TEXT", 1, 0), expectedColumn("activity_effect_id", "TEXT", 0, 0), expectedColumn("projection_kind", "TEXT", 1, 0, "'baseline'"),
		},
		"account_state_observations": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("account_id", "TEXT", 1, 0), expectedColumn("effective_at", "TEXT", 1, 0), expectedColumn("archived_at", "TEXT", 0, 0), expectedColumn("include_in_net_worth", "INTEGER", 1, 0), expectedColumn("include_in_portfolio", "INTEGER", 1, 0), expectedColumn("include_in_liquid_assets", "INTEGER", 1, 0), expectedColumn("activity_id", "TEXT", 0, 0), expectedColumn("created_at", "TEXT", 1, 0),
		},
		"account_state_ownership": {
			expectedColumn("observation_id", "TEXT", 1, 1), expectedColumn("member_id", "TEXT", 1, 2), expectedColumn("share_bps", "INTEGER", 1, 0),
		},
		"instrument_preference_observations": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("instrument_id", "TEXT", 1, 0), expectedColumn("source_kind", "TEXT", 1, 0), expectedColumn("effective_at", "TEXT", 1, 0), expectedColumn("activity_id", "TEXT", 0, 0), expectedColumn("created_at", "TEXT", 1, 0),
		},
		"fx_preference_observations": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("household_id", "TEXT", 1, 0), expectedColumn("currency_a", "TEXT", 1, 0), expectedColumn("currency_b", "TEXT", 1, 0), expectedColumn("source_kind", "TEXT", 1, 0), expectedColumn("effective_at", "TEXT", 1, 0), expectedColumn("activity_id", "TEXT", 0, 0), expectedColumn("created_at", "TEXT", 1, 0),
		},
		"instrument_state_observations": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("instrument_id", "TEXT", 1, 0), expectedColumn("effective_at", "TEXT", 1, 0), expectedColumn("archived_at", "TEXT", 0, 0), expectedColumn("activity_id", "TEXT", 0, 0), expectedColumn("created_at", "TEXT", 1, 0),
		},
		"holding_state_observations": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("holding_id", "TEXT", 1, 0), expectedColumn("effective_at", "TEXT", 1, 0), expectedColumn("archived_at", "TEXT", 0, 0), expectedColumn("activity_id", "TEXT", 0, 0), expectedColumn("created_at", "TEXT", 1, 0),
		},
		"daily_valuation_snapshots": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("household_id", "TEXT", 1, 0), expectedColumn("local_date", "TEXT", 1, 0), expectedColumn("cutoff_at", "TEXT", 1, 0), expectedColumn("revision", "INTEGER", 1, 0), expectedColumn("supersedes_id", "TEXT", 0, 0), expectedColumn("content_hash", "TEXT", 1, 0), expectedColumn("assets_amount", "TEXT", 0, 0), expectedColumn("liabilities_amount", "TEXT", 0, 0), expectedColumn("net_worth_amount", "TEXT", 0, 0), expectedColumn("currency", "TEXT", 1, 0), expectedColumn("complete", "INTEGER", 1, 0), expectedColumn("component_count", "INTEGER", 1, 0), expectedColumn("missing_count", "INTEGER", 1, 0), expectedColumn("generation_reason", "TEXT", 1, 0), expectedColumn("created_at", "TEXT", 1, 0),
		},
		"daily_valuation_snapshot_items": {
			expectedColumn("id", "TEXT", 1, 1), expectedColumn("snapshot_id", "TEXT", 1, 0), expectedColumn("account_id", "TEXT", 1, 0), expectedColumn("holding_id", "TEXT", 0, 0), expectedColumn("instrument_id", "TEXT", 0, 0), expectedColumn("native_amount", "TEXT", 0, 0), expectedColumn("native_currency", "TEXT", 0, 0), expectedColumn("base_amount", "TEXT", 0, 0), expectedColumn("base_currency", "TEXT", 1, 0), expectedColumn("quote_id", "TEXT", 0, 0), expectedColumn("fx_quote_id", "TEXT", 0, 0), expectedColumn("state_observation_id", "TEXT", 0, 0), expectedColumn("preference_observation_id", "TEXT", 0, 0), expectedColumn("complete", "INTEGER", 1, 0), expectedColumn("missing_reason", "TEXT", 0, 0), expectedColumn("fx_preference_observation_id", "TEXT", 0, 0),
		},
		"history_snapshot_state": {
			expectedColumn("household_id", "TEXT", 1, 1), expectedColumn("dirty_from", "TEXT", 0, 0), expectedColumn("last_completed_closed_on", "TEXT", 0, 0), expectedColumn("updated_at", "TEXT", 1, 0),
		},
	}
}

func verifyHistorySchema(ctx context.Context, query schemaQuery) error {
	for table, columns := range historySchemaColumns() {
		if err := verifyTable(ctx, query, table, columns); err != nil {
			return err
		}
	}
	for _, name := range []string{
		"idx_history_origins_household", "idx_history_origin_components_origin", "idx_history_origin_components_account",
		"idx_activities_timeline", "idx_activities_local_date", "idx_activity_effects_activity", "idx_activity_effects_account", "idx_activity_effects_holding",
		"idx_holding_quantity_latest", "idx_account_state_observations_effective", "idx_instrument_preference_observations_effective", "idx_fx_preference_observations_effective", "idx_instrument_state_observations_effective", "idx_holding_state_observations_effective",
		"idx_daily_valuation_latest", "idx_daily_valuation_items_snapshot", "idx_daily_valuation_items_fx_preference",
	} {
		var count int
		if err := query.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("required history index %s is missing", name)
		}
	}
	return nil
}

func expectedSchemaChecks() map[string][]string {
	return map[string][]string{
		"households":          {"CHECK(singleton_key = 1)", "CHECK(base_currency GLOB '[A-Z][A-Z][A-Z]')"},
		"accounts":            {"CHECK(account_type IN ('cash_on_hand','bank_account','brokerage','investment_account','crypto_exchange','digital_wallet','pension','insurance_policy','property','vehicle','collectible','receivable','credit_card','loan','other'))", "CHECK(balance_sheet_role IN ('asset','liability'))", "CHECK(tracking_mode IN ('balance','manual_value','holdings'))", "CHECK((account_type = 'cash_on_hand' AND balance_sheet_role = 'asset' AND tracking_mode = 'balance')", "CHECK(default_currency GLOB '[A-Z][A-Z][A-Z]')", "CHECK(include_in_net_worth IN (0,1))", "CHECK(include_in_portfolio IN (0,1))", "CHECK(include_in_liquid_assets IN (0,1))"},
		"account_ownership":   {"CHECK(share_bps > 0 AND share_bps <= 10000)"},
		"account_values":      {"CHECK(value_kind IN ('balance','manual_value'))", "CHECK(currency GLOB '[A-Z][A-Z][A-Z]')"},
		"instruments":         {"CHECK(instrument_type IN ('stock','etf','mutual_fund','crypto','bond','precious_metal','bank_investment_product','other'))", "CHECK(quote_currency GLOB '[A-Z][A-Z][A-Z]')", "CHECK(country_code IS NULL OR country_code GLOB '[A-Z][A-Z]')", "CHECK(quote_source IN ('manual','provider'))", "CHECK(quote_source = 'manual' OR (provider_key IS NOT NULL AND provider_symbol IS NOT NULL))"},
		"account_cash_values": {"CHECK(currency GLOB '[A-Z][A-Z][A-Z]')"},
		"instrument_quotes":   {"CHECK(currency GLOB '[A-Z][A-Z][A-Z]')", "CHECK(source_kind IN ('manual','provider'))", "CHECK(delayed IN (0,1))"},
		"fx_quotes":           {"CHECK(base_currency GLOB '[A-Z][A-Z][A-Z]')", "CHECK(quote_currency GLOB '[A-Z][A-Z][A-Z]')", "CHECK(source_kind IN ('manual','provider'))", "CHECK(delayed IN (0,1))", "CHECK(base_currency <> quote_currency)"},
		"fx_preferences":      {"CHECK(currency_a GLOB '[A-Z][A-Z][A-Z]')", "CHECK(currency_b GLOB '[A-Z][A-Z][A-Z]')", "CHECK(source_kind IN ('manual','provider'))", "CHECK(currency_a < currency_b)"},
	}
}

func expectedSchemaIndexes() []expectedIndex {
	asc := func(names ...string) []expectedIndexColumn {
		columns := make([]expectedIndexColumn, 0, len(names))
		for _, name := range names {
			columns = append(columns, expectedIndexColumn{name: name})
		}
		return columns
	}
	return []expectedIndex{
		{table: "members", name: "idx_members_household", columns: asc("household_id")},
		{table: "institutions", name: "idx_institutions_household", columns: asc("household_id")},
		{table: "account_groups", name: "idx_groups_household", columns: asc("household_id")},
		{table: "accounts", name: "idx_accounts_household", columns: asc("household_id")},
		{table: "accounts", name: "idx_accounts_institution", columns: asc("institution_id")},
		{table: "accounts", name: "idx_accounts_group", columns: asc("group_id")},
		{table: "accounts", name: "idx_accounts_type", columns: asc("account_type")},
		{table: "account_ownership", name: "idx_ownership_member", columns: asc("member_id")},
		{table: "account_values", name: "idx_account_values_latest", columns: []expectedIndexColumn{{name: "account_id"}, {name: "effective_at", desc: 1}, {name: "created_at", desc: 1}, {name: "id", desc: 1}}},
		{table: "instruments", name: "idx_instruments_household", columns: asc("household_id")},
		{table: "instruments", name: "idx_instruments_quote_source", columns: asc("household_id", "quote_source", "archived_at")},
		{table: "instruments", name: "ux_instruments_active_provider_binding", unique: 1, partial: 1, where: "WHERE archived_at IS NULL AND provider_key IS NOT NULL AND provider_symbol IS NOT NULL", columns: asc("household_id", "provider_key", "provider_symbol")},
		{table: "holdings", name: "idx_holdings_account", columns: asc("account_id", "archived_at", "sort_order", "id")},
		{table: "holdings", name: "idx_holdings_instrument", columns: asc("instrument_id", "archived_at")},
		{table: "holdings", name: "ux_holdings_active_account_instrument", unique: 1, partial: 1, where: "WHERE archived_at IS NULL", columns: asc("account_id", "instrument_id")},
		{table: "account_cash_values", name: "idx_account_cash_latest", columns: []expectedIndexColumn{{name: "account_id"}, {name: "currency"}, {name: "effective_at", desc: 1}, {name: "created_at", desc: 1}, {name: "id", desc: 1}}},
		{table: "instrument_quotes", name: "idx_instrument_quotes_latest", columns: []expectedIndexColumn{{name: "instrument_id"}, {name: "source_kind"}, {name: "currency"}, {name: "quoted_at", desc: 1}, {name: "created_at", desc: 1}, {name: "id", desc: 1}}},
		{table: "fx_quotes", name: "idx_fx_quotes_latest", columns: []expectedIndexColumn{{name: "household_id"}, {name: "base_currency"}, {name: "quote_currency"}, {name: "source_kind"}, {name: "quoted_at", desc: 1}, {name: "created_at", desc: 1}, {name: "id", desc: 1}}},
		{table: "fx_preferences", name: "idx_fx_preferences_household", columns: asc("household_id", "currency_a", "currency_b")},
	}
}

func expectedSchemaForeignKeys() []expectedForeignKey {
	return []expectedForeignKey{
		{table: "media_assets", refTable: "households", from: "household_id", to: "id", onDelete: "CASCADE"},
		{table: "members", refTable: "households", from: "household_id", to: "id", onDelete: "CASCADE"},
		{table: "members", refTable: "media_assets", from: "avatar_asset_id", to: "id", onDelete: "SET NULL"},
		{table: "institutions", refTable: "households", from: "household_id", to: "id", onDelete: "CASCADE"},
		{table: "institutions", refTable: "media_assets", from: "logo_asset_id", to: "id", onDelete: "SET NULL"},
		{table: "account_groups", refTable: "households", from: "household_id", to: "id", onDelete: "CASCADE"},
		{table: "account_groups", refTable: "media_assets", from: "logo_asset_id", to: "id", onDelete: "SET NULL"},
		{table: "accounts", refTable: "households", from: "household_id", to: "id", onDelete: "CASCADE"},
		{table: "accounts", refTable: "institutions", from: "institution_id", to: "id", onDelete: "SET NULL"},
		{table: "accounts", refTable: "account_groups", from: "group_id", to: "id", onDelete: "SET NULL"},
		{table: "accounts", refTable: "media_assets", from: "logo_asset_id", to: "id", onDelete: "SET NULL"},
		{table: "account_ownership", refTable: "accounts", from: "account_id", to: "id", onDelete: "CASCADE"},
		{table: "account_ownership", refTable: "members", from: "member_id", to: "id", onDelete: "RESTRICT"},
		{table: "account_values", refTable: "accounts", from: "account_id", to: "id", onDelete: "CASCADE"},
		{table: "instruments", refTable: "households", from: "household_id", to: "id", onDelete: "CASCADE"},
		{table: "instruments", refTable: "media_assets", from: "logo_asset_id", to: "id", onDelete: "SET NULL"},
		{table: "instrument_state_observations", refTable: "instruments", from: "instrument_id", to: "id", onDelete: "RESTRICT"},
		{table: "instrument_state_observations", refTable: "activities", from: "activity_id", to: "id", onDelete: "RESTRICT"},
		{table: "holdings", refTable: "accounts", from: "account_id", to: "id", onDelete: "CASCADE"},
		{table: "holdings", refTable: "instruments", from: "instrument_id", to: "id", onDelete: "RESTRICT"},
		{table: "holding_state_observations", refTable: "holdings", from: "holding_id", to: "id", onDelete: "RESTRICT"},
		{table: "holding_state_observations", refTable: "activities", from: "activity_id", to: "id", onDelete: "RESTRICT"},
		{table: "account_cash_values", refTable: "accounts", from: "account_id", to: "id", onDelete: "CASCADE"},
		{table: "instrument_quotes", refTable: "instruments", from: "instrument_id", to: "id", onDelete: "CASCADE"},
		{table: "fx_quotes", refTable: "households", from: "household_id", to: "id", onDelete: "CASCADE"},
		{table: "fx_preferences", refTable: "households", from: "household_id", to: "id", onDelete: "CASCADE"},
		{table: "daily_valuation_snapshot_items", refTable: "fx_preference_observations", from: "fx_preference_observation_id", to: "id", onDelete: "RESTRICT"},
	}
}
