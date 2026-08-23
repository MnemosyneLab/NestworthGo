package sqlite

import (
	"context"
	"path/filepath"
	"testing"
)

func TestVerifyRejectsSchemaDefinitionMutationsWithStableNames(t *testing.T) {
	cases := []struct {
		name   string
		mutate string
	}{
		{
			name: "column type",
			mutate: `DROP TABLE fx_preferences;
CREATE TABLE fx_preferences (
    household_id TEXT NOT NULL,
    currency_a TEXT NOT NULL,
    currency_b INTEGER NOT NULL,
    source_kind TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY(household_id, currency_a, currency_b),
    CHECK(currency_a < currency_b),
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE CASCADE
);`,
		},
		{
			name: "index uniqueness",
			mutate: `DROP INDEX ux_holdings_active_account_instrument;
CREATE INDEX ux_holdings_active_account_instrument ON holdings(account_id, instrument_id) WHERE archived_at IS NULL;`,
		},
		{
			name: "index columns",
			mutate: `DROP INDEX idx_holdings_account;
CREATE INDEX idx_holdings_account ON holdings(account_id, sort_order, archived_at, id);`,
		},
		{
			name: "partial predicate",
			mutate: `DROP INDEX ux_holdings_active_account_instrument;
CREATE UNIQUE INDEX ux_holdings_active_account_instrument ON holdings(account_id, instrument_id) WHERE archived_at IS NOT NULL;`,
		},
		{
			name: "foreign key action",
			mutate: `DROP TABLE fx_preferences;
CREATE TABLE fx_preferences (
    household_id TEXT NOT NULL,
    currency_a TEXT NOT NULL CHECK(currency_a GLOB '[A-Z][A-Z][A-Z]'),
    currency_b TEXT NOT NULL CHECK(currency_b GLOB '[A-Z][A-Z][A-Z]'),
    source_kind TEXT NOT NULL CHECK(source_kind IN ('manual','provider')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY(household_id, currency_a, currency_b),
    CHECK(currency_a < currency_b),
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE RESTRICT
);
CREATE INDEX idx_fx_preferences_household ON fx_preferences(household_id, currency_a, currency_b);`,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			database, err := Open(filepath.Join(t.TempDir(), "schema.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			if _, err := database.SQL.Exec(testCase.mutate); err != nil {
				t.Fatalf("apply schema mutation: %v", err)
			}
			if err := database.Verify(context.Background()); err == nil {
				t.Fatal("Verify accepted a schema definition mutation")
			}
		})
	}
}
