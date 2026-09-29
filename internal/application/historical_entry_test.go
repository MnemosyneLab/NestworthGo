package application

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestOnboardingHistoricalStartAndBackdatedAccount(t *testing.T) {
	db, err := sqlite.Open(t.TempDir() + "/historical-entry.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := sqlite.NewRepository(db)
	svc := NewService(repo)
	ctx := context.Background()
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	svc.setClock(func() time.Time { return now })
	if err := svc.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Past", BaseCurrency: "CNY", MemberNames: []string{"Owner"}, Timezone: "Asia/Singapore", HistoryStartDate: "2026-09-26"}); err != nil {
		t.Fatal(err)
	}
	origin, err := svc.HistoryOrigin(ctx)
	if err != nil || origin == nil {
		t.Fatalf("origin = %+v, err = %v", origin, err)
	}
	wantStart := time.Date(2026, 9, 25, 16, 0, 0, 0, time.UTC)
	if !origin.StartedAt.Equal(wantStart) || !origin.CreatedAt.Equal(now) {
		t.Fatalf("origin times = %s / %s, want %s / %s", origin.StartedAt, origin.CreatedAt, wantStart, now)
	}
	bootstrap, err := svc.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	opened := "2026-09-27"
	account, err := svc.CreateAccount(ctx, AccountInput{Name: "Backdated bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "100", OpenedOn: &opened, IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if !account.Account.CreatedAt.Equal(now) {
		t.Fatalf("audit creation time = %s, want %s", account.Account.CreatedAt, now)
	}
	activities, err := repo.ListActivities(ctx, bootstrap.Household.ID, 10)
	if err != nil || len(activities) != 1 {
		t.Fatalf("activities = %+v, err = %v", activities, err)
	}
	if activities[0].EffectiveLocalDate != opened || !activities[0].CreatedAt.Equal(now) {
		t.Fatalf("backdated activity = %+v", activities[0])
	}
	for _, tc := range []struct {
		day  string
		want string
	}{
		{day: "2026-09-26", want: "0"},
		{day: "2026-09-27", want: "100"},
	} {
		snapshot, built, err := svc.BuildDailyValuationSnapshot(ctx, tc.day)
		if err != nil || !built || snapshot.NetWorthAmount == nil {
			t.Fatalf("%s snapshot = %+v, built=%v, err=%v", tc.day, snapshot, built, err)
		}
		if got := snapshot.NetWorthAmount.CanonicalAmount(); got != tc.want {
			t.Fatalf("%s net worth = %s, want %s", tc.day, got, tc.want)
		}
	}
	oldSnapshot, built, err := svc.BuildDailyValuationSnapshot(ctx, "2026-09-28")
	if err != nil || !built {
		t.Fatalf("old snapshot = %+v, built=%v, err=%v", oldSnapshot, built, err)
	}
	if _, err := svc.RebuildHistoricalSnapshots(ctx, "2026-09-26", "2026-09-28"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CompleteDailySnapshotRange(ctx, bootstrap.Household.ID, "2026-09-28", now); err != nil {
		t.Fatal(err)
	}
	zeroOpened := "2026-09-28"
	zeroAccount, err := svc.CreateAccount(ctx, AccountInput{Name: "Empty account", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "0", OpenedOn: &zeroOpened, IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	state, err := repo.DailySnapshotState(ctx, bootstrap.Household.ID)
	if err != nil || state.DirtyFrom == nil || *state.DirtyFrom != zeroOpened {
		t.Fatalf("zero account dirty state = %+v, err=%v", state, err)
	}
	newSnapshot, built, err := svc.BuildDailyValuationSnapshot(ctx, zeroOpened)
	if err != nil || !built || newSnapshot.ComponentCount != oldSnapshot.ComponentCount+1 {
		t.Fatalf("recomputed zero account snapshot = %+v, prior=%+v, built=%v, err=%v", newSnapshot, oldSnapshot, built, err)
	}
	if _, err := svc.RebuildHistoricalSnapshots(ctx, zeroOpened, zeroOpened); err != nil {
		t.Fatal(err)
	}
	before, err := (HistoricalReplay{repository: repo}).Snapshot(ctx, origin, time.Date(2026, 9, 27, 15, 59, 59, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	after, err := (HistoricalReplay{repository: repo}).Snapshot(ctx, origin, time.Date(2026, 9, 28, 15, 59, 59, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	contains := func(records []domain.AccountRecord, id domain.AccountID) bool {
		for _, record := range records {
			if record.Account.ID == id {
				return true
			}
		}
		return false
	}
	if contains(before.Accounts, zeroAccount.Account.ID) || !contains(after.Accounts, zeroAccount.Account.ID) {
		t.Fatalf("zero account presence before=%v after=%v", contains(before.Accounts, zeroAccount.Account.ID), contains(after.Accounts, zeroAccount.Account.ID))
	}
}

func TestHistoricalStartRejectsInferredPastAssetsAndFuture(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	for _, date := range []string{"2026-09-30", "2026-02-30"} {
		if _, err := historyStartInstant(date, "Asia/Singapore", now); err == nil {
			t.Fatalf("start date %q was accepted", date)
		}
	}
	db, err := sqlite.Open(t.TempDir() + "/past-assets.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewService(sqlite.NewRepository(db))
	svc.setClock(func() time.Time { return now })
	if err := svc.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Current", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := svc.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateAccount(ctx, AccountInput{Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "50", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartHistoryOnDate(ctx, "Asia/Singapore", "2026-09-27", nil); err == nil {
		t.Fatal("past history start accepted today's existing balance")
	}
	if origin, err := svc.HistoryOrigin(ctx); err != nil || origin != nil {
		t.Fatalf("rejected start persisted origin = %+v, err = %v", origin, err)
	}
	origin, err := svc.StartHistoryOnDate(ctx, "Asia/Singapore", "2026-09-29", nil)
	if err != nil || !origin.StartedAt.Equal(now) {
		t.Fatalf("current-assets start = %+v, err = %v", origin, err)
	}
}
