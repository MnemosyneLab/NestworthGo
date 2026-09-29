package application

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestFullRebuildReplacesCorruptDerivedResultsAndPreservesFacts(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(t.TempDir() + "/rebuild.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := sqlite.NewRepository(db)
	s := NewService(repo)
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	s.setClock(func() time.Time { return now })
	if err := s.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Family", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	boot, _ := s.Bootstrap(ctx)
	account, err := s.CreateAccount(ctx, AccountInput{Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID}, InitialAmount: "1000"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistory(ctx, "Asia/Singapore"); err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 1)
	change, err := s.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: boot.Household.ID, AccountID: account.Account.ID, Amount: mustMoney(t, "100", "CNY"), EffectiveAt: now})
	if err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 3)
	if _, err := s.RebuildHistoricalSnapshots(ctx, "2026-09-25", "2026-09-28"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.ExecContext(ctx, `UPDATE account_values SET amount='9999' WHERE projection_kind <> 'baseline'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.ExecContext(ctx, `UPDATE daily_valuation_snapshots SET net_worth_amount='9999'`); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		result, err := s.RebuildDerivedData(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if result.From != "2026-09-25" || result.To != "2026-09-29" || result.SnapshotDays != 4 || result.IncompleteDays != 0 {
			t.Fatalf("unexpected result: %+v", result)
		}
		current, err := repo.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
		if err != nil {
			t.Fatal(err)
		}
		if got := current.Accounts[0].LatestValue.Amount.CanonicalAmount(); got != "1100" {
			t.Fatalf("current balance %s", got)
		}
		snapshots, err := repo.ListDailyValuationSnapshots(ctx, boot.Household.ID, time.Time{}, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshots) != 4 {
			t.Fatalf("snapshot count %d", len(snapshots))
		}
		for _, snapshot := range snapshots {
			want := "1100"
			if snapshot.LocalDate == "2026-09-25" {
				want = "1000"
			}
			if snapshot.NetWorthAmount == nil || snapshot.NetWorthAmount.CanonicalAmount() != want {
				t.Fatalf("incorrect daily value %+v", snapshot)
			}
		}
		raw, err := repo.Activity(ctx, boot.Household.ID, change.Activity.ID)
		if err != nil {
			t.Fatal(err)
		}
		if raw.Effects[0].Money.CanonicalAmount() != "100" || !raw.EffectiveAt.Equal(change.Activity.EffectiveAt) {
			t.Fatal("source activity changed")
		}
		var count int
		if err := db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM activities`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("rebuild changed source ledger count %d", count)
		}
	}
	// A rebuild cannot supply a missing exchange rate. Keep the native input
	// and report that closed day as incomplete instead of inventing a total.
	_, err = s.CreateAccount(ctx, AccountInput{Name: "Foreign", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID}, InitialAmount: "5"})
	if err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 1)
	result, err := s.RebuildDerivedData(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.SnapshotDays != 5 || result.IncompleteDays != 1 {
		t.Fatalf("missing FX was hidden: %+v", result)
	}

}
