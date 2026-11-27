package application

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestCompleteOnboardingCreatesEmptyHistoryAndRecordsAfterReopen(t *testing.T) {
	now := time.Date(2026, 11, 21, 17, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, timezone, day string
		start               time.Time
	}{
		{"default", "", "", time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)},
		{"cross local midnight", "Asia/Singapore", "", time.Date(2026, 11, 21, 16, 0, 0, 0, time.UTC)},
		{"earlier date", "Asia/Singapore", "2026-11-20", time.Date(2026, 11, 19, 16, 0, 0, 0, time.UTC)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			path := filepath.Join(t.TempDir(), "synthetic-ledger.db")
			db, err := sqlite.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()
			repo := sqlite.NewRepository(db)
			svc := NewService(repo)
			svc.SetClock(func() time.Time { return now })
			request := OnboardingInput{HouseholdName: "Synthetic QA", BaseCurrency: "USD", MemberNames: []string{"Synthetic owner"}, Timezone: tc.timezone, HistoryStartDate: tc.day}
			if err := svc.CompleteOnboarding(ctx, request); err != nil {
				t.Fatal(err)
			}
			origin, err := svc.HistoryOrigin(ctx)
			zone := tc.timezone
			if zone == "" {
				zone = "UTC"
			}
			if err != nil || origin == nil || origin.Timezone != zone || !origin.StartedAt.Equal(tc.start) || !origin.CreatedAt.Equal(now) {
				t.Fatalf("bad origin: %+v, %v", origin, err)
			}
			data, err := repo.HistoryOriginData(ctx, origin.ID)
			if err != nil || len(data.Components) != 0 || len(data.AccountStates) != 0 || len(data.Ownership) != 0 {
				t.Fatalf("not empty: %+v, %v", data, err)
			}
			rows, err := svc.ListActivities(ctx, 20)
			if err != nil || len(rows) != 0 {
				t.Fatalf("fabricated activities: %+v, %v", rows, err)
			}
			if err := svc.CompleteOnboarding(ctx, request); err == nil {
				t.Fatal("duplicate create accepted")
			}
			boot, err := svc.Bootstrap(ctx)
			if err != nil || boot.Household == nil || len(boot.Members) != 1 {
				t.Fatalf("bootstrap: %+v %v", boot, err)
			}
			makeAccount := func(name, typ, role, mode, amount string) domain.AccountRecord {
				t.Helper()
				a, err := svc.CreateAccount(ctx, AccountInput{Name: name, AccountType: typ, BalanceSheetRole: role, TrackingMode: mode, InitialAmount: amount, DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
				if err != nil {
					t.Fatal(err)
				}
				return a
			}
			// Open a separate account at the ledger-local start of today, then
			// record earlier than the creation instant. The default origin
			// must not block ordinary first-day backdated recording.
			location, _ := time.LoadLocation(zone)
			today := now.In(location).Format(time.DateOnly)
			early, err := svc.CreateAccount(ctx, AccountInput{Name: "First-day account", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "0", OwnerIDs: []domain.MemberID{boot.Members[0].ID}, OpenedOn: &today})
			if err != nil {
				t.Fatal(err)
			}
			morning := tc.start.Add(5 * time.Minute)
			if tc.day != "" {
				morning = now.Add(-30 * time.Minute)
			}
			deposited, _ := domain.ParseMoney("5", "USD")
			if _, err := svc.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: boot.Household.ID, AccountID: early.Account.ID, Amount: deposited, Reason: domain.ReasonContribution, EffectiveAt: morning}); err != nil {
				t.Fatalf("first-day recording before the creation instant: %v", err)
			}
			cash := makeAccount("Existing cash", "bank_account", "asset", "balance", "120.25")
			debt := makeAccount("Existing debt", "credit_card", "liability", "balance", "34.25")
			broker := makeAccount("Existing brokerage", "brokerage", "asset", "holdings", "")
			instrument, err := svc.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic EUR fund", Type: "etf", QuoteCurrency: "EUR", QuoteSource: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			q, _ := domain.ParseQuantity("2.5")
			cost, _ := domain.ParseUnitPrice("12.3456")
			preview, token, err := svc.PreviewChangeGuarded(ctx, domain.PositionImportInput{HouseholdID: boot.Household.ID, AccountID: broker.Account.ID, InstrumentID: instrument.ID, Quantity: q, UnitCost: &cost, Currency: "EUR", EffectiveAt: now})
			if err != nil {
				t.Fatal(err)
			}
			if preview.Activity.Kind != domain.ActivityPositionTransfer {
				t.Fatalf("existing position became %v", preview.Activity.Kind)
			}
			// Existing position uses the same guarded change and cost-currency rules as the GUI.
			if _, err := svc.RecordChangeGuarded(ctx, domain.PositionImportInput{HouseholdID: boot.Household.ID, AccountID: broker.Account.ID, InstrumentID: instrument.ID, Quantity: q, UnitCost: &cost, Currency: "EUR", EffectiveAt: now}, domain.NewMutationID().String(), strings.Repeat("ab", 32), token); err != nil {
				t.Fatal(err)
			}
			if cash.LatestValue == nil || cash.LatestValue.Amount.CanonicalAmount() != "120.25" || debt.LatestValue == nil || debt.LatestValue.Amount.CanonicalAmount() != "34.25" {
				t.Fatalf("opening balances incorrect: %+v %+v", cash, debt)
			}
			// The account-opening amount is a contribution, not income.
			rows, err = svc.ListActivities(ctx, 20)
			if err != nil {
				t.Fatal(err)
			}
			for _, a := range rows {
				if a.Reason == domain.ReasonIncome || a.Kind == domain.ActivityBuy {
					t.Fatalf("fabricated income or buy: %+v", a)
				}
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = sqlite.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			svc = NewService(sqlite.NewRepository(db))
			svc.SetClock(func() time.Time { return now.Add(time.Minute) })
			reopened, err := svc.HistoryOrigin(ctx)
			if err != nil || reopened == nil || reopened.ID != origin.ID || !reopened.StartedAt.Equal(origin.StartedAt) {
				t.Fatalf("origin changed on reopen: %+v %v", reopened, err)
			}
			money, _ := domain.ParseMoney("3.25", "USD")
			if _, err := svc.RecordChange(ctx, domain.MoneyRemovedInput{HouseholdID: boot.Household.ID, AccountID: cash.Account.ID, Amount: money, Reason: domain.ReasonExpense, EffectiveAt: now.Add(time.Minute)}); err != nil {
				t.Fatal(err)
			}
			holdings, err := svc.HoldingsByAccounts(ctx, []domain.AccountID{broker.Account.ID})
			if err != nil || len(holdings[broker.Account.ID]) != 1 || holdings[broker.Account.ID][0].Quantity.Canonical() != "2.5" {
				t.Fatalf("holding not retained: %+v %v", holdings, err)
			}
			gain, err := svc.HoldingGain(ctx, holdings[broker.Account.ID][0].ID)
			if err != nil || gain.TotalCost == nil || gain.TotalCost.Amount != "30.864" || gain.TotalCost.Currency != "EUR" {
				t.Fatalf("native cost changed: %+v %v", gain, err)
			}
		})
	}
}

func TestCompleteOnboardingOriginFailureRollsBack(t *testing.T) {
	ctx := t.Context()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "synthetic-failure.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewService(sqlite.NewRepository(db))
	svc.SetClock(func() time.Time { return time.Date(2026, 11, 21, 17, 0, 0, 0, time.UTC) })
	req := OnboardingInput{HouseholdName: "Synthetic QA", BaseCurrency: "USD", MemberNames: []string{"Owner"}, Timezone: "Asia/Singapore", HistoryStartDate: "2026-11-23"}
	if err := svc.CompleteOnboarding(ctx, req); err == nil {
		t.Fatal("future date accepted")
	}
	req.HistoryStartDate = ""
	if _, err := db.SQL.Exec("CREATE TRIGGER fail_origin BEFORE INSERT ON history_origins BEGIN SELECT RAISE(ABORT, 'synthetic fault'); END"); err != nil {
		t.Fatal(err)
	}
	if err := svc.CompleteOnboarding(ctx, req); err == nil {
		t.Fatal("origin fault accepted")
	}
	for _, table := range []string{"households", "members", "history_origins", "history_snapshot_state"} {
		var count int
		if err := db.SQL.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s: %d %v", table, count, err)
		}
	}
	if _, err := db.SQL.Exec("DROP TRIGGER fail_origin"); err != nil {
		t.Fatal(err)
	}
	if err := svc.CompleteOnboarding(ctx, req); err != nil {
		t.Fatal(err)
	}
}
