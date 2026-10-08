package application

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func historicalInsertionFixture(t *testing.T) (context.Context, *Service, *sqlite.Repository, *sqlite.DB, Bootstrap, func(time.Time)) {
	t.Helper()
	ctx := context.Background()
	db, err := sqlite.Open(t.TempDir() + "/insertion.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewRepository(db)
	s := NewService(repo)
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	s.setClock(func() time.Time { return now })
	if err := s.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Family", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	boot, err := s.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, s, repo, db, boot, func(next time.Time) { now = next }
}

func historicalInsertionTime(day int) time.Time {
	return time.Date(2026, 9, day, 12, 0, 0, 0, time.UTC)
}

func TestBackdatedSellUsesQuantityAtItsDateThenChecksLaterSales(t *testing.T) {
	t.Parallel()
	ctx, s, repo, db, boot, setNow := historicalInsertionFixture(t)
	account, err := s.CreateAccount(ctx, AccountInput{Name: "Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendAccountCashValue(ctx, account.Account.ID, "1000", "CNY", "2026-09-25"); err != nil {
		t.Fatal(err)
	}
	instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Fund", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := s.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "0"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	setNow(historicalInsertionTime(29))
	trade := domain.TradeInput{HouseholdID: boot.Household.ID, Side: domain.TradeBuy, SettlementAccountID: account.Account.ID, HoldingID: holding.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, "10"), Gross: mustMoney(t, "100", "CNY"), EffectiveAt: historicalInsertionTime(26)}
	if _, err := s.RecordChange(ctx, trade); err != nil {
		t.Fatal(err)
	}
	trade.Side, trade.Gross, trade.EffectiveAt = domain.TradeSell, mustMoney(t, "120", "CNY"), historicalInsertionTime(28)
	if _, err := s.RecordChange(ctx, trade); err != nil {
		t.Fatal(err)
	}
	trade.Quantity, trade.Gross, trade.EffectiveAt = mustQuantity(t, "4"), mustMoney(t, "48", "CNY"), historicalInsertionTime(27)

	// The holding is empty today but had ten units at the proposed sale date.
	// The insertion must reach replay and reject the later negative end state.
	if _, err := s.PreviewChange(ctx, trade); err == nil {
		t.Fatal("backdated sell invalidating the later full sale was previewed")
	} else if domainErr, ok := err.(*domain.Error); !ok || domainErr.Code != domain.ErrInvalidChange {
		t.Fatalf("preview rejected before checking later replay: %v", err)
	}
	var before, after int
	if err := db.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordChange(ctx, trade); err == nil {
		t.Fatal("backdated sell invalidating the later full sale was written")
	} else if domainErr, ok := err.(*domain.Error); !ok || domainErr.Code != domain.ErrInvalidChange {
		t.Fatalf("record returned unexpected error: %v", err)
	}
	if err := db.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("failed insertion wrote an activity: before=%d after=%d", before, after)
	}
	snapshot, err := repo.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Holdings) != 1 || snapshot.Holdings[0].Quantity.String() != "0" {
		t.Fatalf("failed insertion changed current holding: %+v", snapshot.Holdings)
	}
}

func TestBackdatedEntryPreservesLaterAbsoluteValue(t *testing.T) {
	t.Parallel()
	ctx, s, repo, _, boot, setNow := historicalInsertionFixture(t)
	account, err := s.CreateAccount(ctx, AccountInput{Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "1000", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	setNow(historicalInsertionTime(29))
	if _, err := s.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: boot.Household.ID, AccountID: account.Account.ID, Amount: mustMoney(t, "100", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: historicalInsertionTime(26)}); err != nil {
		t.Fatal(err)
	}
	observation, err := s.RecordChange(ctx, domain.ValueUpdateInput{HouseholdID: boot.Household.ID, AccountID: account.Account.ID, NewValue: mustMoney(t, "1200", "CNY"), Reason: domain.ReasonReconciliation, EffectiveAt: historicalInsertionTime(28)})
	if err != nil {
		t.Fatal(err)
	}
	inserted, err := s.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: boot.Household.ID, AccountID: account.Account.ID, Amount: mustMoney(t, "80", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: historicalInsertionTime(27)})
	if err != nil || len(inserted.Resulting) != 1 || inserted.Resulting[0].Amount != "1180" {
		t.Fatalf("inserted entry has wrong at-date result: %+v %v", inserted.Resulting, err)
	}
	live, err := s.Overview(ctx, domain.AccountFilter{})
	if err != nil || live.NetWorth.String() != "1200" {
		t.Fatalf("later absolute value moved: %+v %v", live.NetWorth, err)
	}
	events, err := repo.ListActivitiesUntil(ctx, boot.Household.ID, historicalInsertionTime(29))
	if err != nil || len(events) != 3 || events[2].ID != observation.Activity.ID || events[2].Effects[0].Money.CanonicalAmount() != "20" {
		t.Fatalf("later observation projection did not adjust: %+v %v", events, err)
	}
	raw, err := repo.Activity(ctx, boot.Household.ID, observation.Activity.ID)
	if err != nil || raw.Effects[0].Money.CanonicalAmount() != "100" {
		t.Fatalf("original observation was changed: %+v %v", raw.Effects, err)
	}
}

func TestBackdatedExpenseUsesEarlierBalanceAndKeepsLaterObservation(t *testing.T) {
	t.Parallel()
	ctx, s, repo, _, boot, setNow := historicalInsertionFixture(t)
	account, err := s.CreateAccount(ctx, AccountInput{Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "100", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	setNow(historicalInsertionTime(29))
	observation, err := s.RecordChange(ctx, domain.ValueUpdateInput{HouseholdID: boot.Household.ID, AccountID: account.Account.ID, NewValue: mustMoney(t, "200", "CNY"), Reason: domain.ReasonReconciliation, EffectiveAt: historicalInsertionTime(27)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordChange(ctx, domain.MoneyRemovedInput{HouseholdID: boot.Household.ID, AccountID: account.Account.ID, Amount: mustMoney(t, "190", "CNY"), Reason: domain.ReasonExpense, EffectiveAt: historicalInsertionTime(28)}); err != nil {
		t.Fatal(err)
	}
	backdated := domain.MoneyRemovedInput{HouseholdID: boot.Household.ID, AccountID: account.Account.ID, Amount: mustMoney(t, "50", "CNY"), Reason: domain.ReasonExpense, EffectiveAt: historicalInsertionTime(26)}
	preview, err := s.PreviewChange(ctx, backdated)
	if err != nil || len(preview.Resulting) != 1 || preview.Resulting[0].Amount != "50" {
		t.Fatalf("backdated expense preview should use earlier 100 balance: %+v %v", preview.Resulting, err)
	}
	inserted, err := s.RecordChange(ctx, backdated)
	if err != nil || len(inserted.Resulting) != 1 || inserted.Resulting[0].Amount != "50" {
		t.Fatalf("backdated expense was not inserted: %+v %v", inserted.Resulting, err)
	}
	live, err := s.Overview(ctx, domain.AccountFilter{})
	if err != nil || live.NetWorth.String() != "10" {
		t.Fatalf("later observed balance and expense changed: %+v %v", live.NetWorth, err)
	}
	events, err := repo.ListActivitiesUntil(ctx, boot.Household.ID, historicalInsertionTime(29))
	if err != nil || len(events) != 3 || events[1].ID != observation.Activity.ID || events[1].Effects[0].Money.CanonicalAmount() != "150" {
		t.Fatalf("later absolute observation did not absorb backdated expense: %+v %v", events, err)
	}
}
