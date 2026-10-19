package application

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestHistoricalSnapshotInverseFXPrecisionAcrossStorageTrendAndHealth(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "synthetic-fx.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewService(sqlite.NewRepository(db))
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	s.setClock(func() time.Time { return now })
	ctx := context.Background()
	if err := s.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Synthetic FX regression", BaseCurrency: "CNY", MemberNames: []string{"Test owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAccount(ctx, AccountInput{Name: "Synthetic USD", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", IncludeInNetWorth: true, InitialAmount: "100", Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetFXPreference(ctx, "CNY", "USD", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendManualFXQuote(ctx, "CNY", "USD", "1.123456789012", "2026-08-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 2)
	const exact = "89.01098909904924535"
	const rounded = "89.011"
	snapshot, appended, err := s.BuildDailyValuationSnapshot(ctx, "2026-08-01")
	if err != nil || !appended {
		t.Fatalf("build: appended=%v err=%v", appended, err)
	}
	if len(snapshot.Items) != 1 || snapshot.Items[0].BaseAmountExact != exact || snapshot.Items[0].NativeAmount != "100" {
		t.Fatalf("snapshot items: %+v", snapshot.Items)
	}
	if snapshot.NetWorthAmount == nil || snapshot.NetWorthAmount.CanonicalAmount() != rounded {
		t.Fatalf("snapshot total: %+v", snapshot.NetWorthAmount)
	}
	stored, err := s.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, time.Time{}, time.Time{})
	if err != nil || len(stored) != 1 {
		t.Fatalf("reload: count=%d err=%v", len(stored), err)
	}
	if stored[0].Items[0].BaseAmountExact != exact {
		t.Fatalf("precision lost on reload: %+v", stored[0].Items)
	}
	trend, err := s.NetWorthTrend(ctx, domain.TrendAllTime)
	if err != nil {
		t.Fatal(err)
	}
	if len(trend.Points) < 2 || trend.Points[0].NetWorth == nil || trend.Points[0].NetWorth.CanonicalAmount() != rounded {
		t.Fatalf("trend: %+v", trend.Points)
	}
	// Simulate an older incomplete derived snapshot to exercise health's fresh
	// valuation path, rather than only its fast path for completed snapshots.
	if _, err := db.SQL.Exec(`UPDATE daily_valuation_snapshots SET complete = 0 WHERE id = ?`, snapshot.ID.String()); err != nil {
		t.Fatal(err)
	}
	report, err := s.ScanMarketDataHealth(ctx)
	if err != nil {
		t.Fatalf("health fresh valuation: %v", err)
	}
	found := false
	for _, issue := range report.Issues {
		if issue.Code == "snapshot_outdated" && issue.RangeStart == "2026-08-01" {
			found = true
		}
	}
	if !found {
		t.Fatalf("health should offer snapshot repair: %+v", report.Issues)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	// Opening the persisted database runs canonical numeric integrity checks.
	reopened, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	stored, err = sqlite.NewRepository(reopened).ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, time.Time{}, time.Time{})
	if err != nil || len(stored) == 0 {
		t.Fatalf("reopened snapshots: %d %v", len(stored), err)
	}
	for _, day := range stored {
		if len(day.Items) != 1 || day.Items[0].BaseAmountExact != exact {
			t.Fatalf("reopened precision: %+v", day.Items)
		}
	}
}
