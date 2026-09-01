package sqlite

import (
	"strings"
	"testing"
)

func TestReplaceSchemaSQLFragmentPreservesSurroundingSQL(t *testing.T) {
	definition := `CREATE TABLE accounts (
    id TEXT PRIMARY KEY NOT NULL,
    CHECK(
        (account_type = 'cash_on_hand' AND balance_sheet_role = 'asset' AND tracking_mode = 'balance') OR
        (account_type = 'bank_account' AND balance_sheet_role = 'asset' AND tracking_mode IN ('balance','holdings'))
    )
);`
	replaced, ok := replaceSchemaSQLFragment(definition, cashOnHandBalanceOnlyCheck, cashOnHandBalanceOrHoldingsCheck)
	if !ok {
		t.Fatal("replaceSchemaSQLFragment returned false")
	}
	if !schemaSQLContains(replaced, cashOnHandBalanceOrHoldingsCheck) {
		t.Fatalf("replaced SQL missing new check: %s", replaced)
	}
	if schemaSQLContains(replaced, cashOnHandBalanceOnlyCheck) {
		t.Fatalf("replaced SQL still has legacy check: %s", replaced)
	}
	if !schemaSQLContains(replaced, "account_type = 'bank_account' AND balance_sheet_role = 'asset' AND tracking_mode IN ('balance','holdings')") {
		t.Fatalf("replaced SQL lost neighboring check: %s", replaced)
	}
}

func TestReplaceCreateTableNameHandlesQuotedIdentifiers(t *testing.T) {
	cases := []string{
		`CREATE TABLE accounts (id TEXT)`,
		`CREATE TABLE "accounts" (id TEXT)`,
		"CREATE TABLE `accounts` (id TEXT)",
	}
	for _, definition := range cases {
		got, ok := replaceCreateTableName(definition, "accounts", "accounts_new")
		if !ok {
			t.Fatalf("replaceCreateTableName(%q) returned false", definition)
		}
		if strings.Contains(strings.ToLower(got), "create table accounts ") || strings.Contains(strings.ToLower(got), `create table "accounts"`) {
			t.Fatalf("old table name remains: %s", got)
		}
		renamed, ok := replaceCreateTableName(got, "accounts_new", "accounts")
		if !ok {
			t.Fatalf("round-trip rename failed: %s", got)
		}
		if !strings.Contains(strings.ToLower(renamed), "accounts") {
			t.Fatalf("restored name missing: %s", renamed)
		}
	}
}
