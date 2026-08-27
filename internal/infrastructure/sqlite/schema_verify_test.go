package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
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

func TestVerifyRejectsAccountWithoutOwnershipRows(t *testing.T) {
	database, _, _, account, _ := seedPortfolioRepository(t)
	if _, err := database.SQL.Exec(`DELETE FROM account_ownership WHERE account_id = ?`, account.ID.String()); err != nil {
		t.Fatalf("delete ownership: %v", err)
	}
	if err := database.Verify(context.Background()); err == nil || err.Error() != "1 accounts have no ownership rows" {
		t.Fatalf("Verify error = %v, want missing ownership", err)
	}
}

func TestVerifyRejectsArchivedHoldingOnNonHoldingsAccount(t *testing.T) {
	database, repository, household, _, instrument := seedPortfolioRepository(t)
	ctx := context.Background()
	members, err := repository.ListMembers(ctx, true)
	if err != nil || len(members) == 0 {
		t.Fatalf("ListMembers: %v", err)
	}
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	balance, ownership, initial, err := domain.NewAccount(domain.AccountInput{
		HouseholdID: household.ID, Name: "Cash", AccountType: domain.TypeCashOnHand, BalanceSheetRole: domain.RoleAsset,
		TrackingMode: domain.TrackingBalance, DefaultCurrency: domain.CurrencyCode("CNY"),
		Ownership: []domain.OwnershipShare{{MemberID: members[0].ID, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "1",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	value, err := domain.NewAccountValue(balance, *initial, now, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateAccount(ctx, balance, ownership, &value); err != nil {
		t.Fatal(err)
	}
	archivedAt := now.UTC().Format(time.RFC3339Nano)
	if _, err := database.SQL.Exec(
		`INSERT INTO holdings(id, account_id, instrument_id, quantity, sort_order, created_at, updated_at, archived_at) VALUES (?, ?, ?, '1', 0, ?, ?, ?)`,
		domain.NewHoldingID().String(), balance.ID.String(), instrument.ID.String(), archivedAt, archivedAt, archivedAt,
	); err != nil {
		t.Fatalf("insert archived holding: %v", err)
	}
	if err := database.Verify(ctx); err == nil || err.Error() != "1 holdings belong to a non-holdings account" {
		t.Fatalf("Verify error = %v, want archived holding on a non-holdings account", err)
	}
}
