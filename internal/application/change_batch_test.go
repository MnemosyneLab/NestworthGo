package application

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestGuardedChangeBatchReplaysHistoricalTimelineAndReceipt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "batch.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db)
	service := NewService(repo)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := start
	service.SetClock(func() time.Time { return now })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Batch", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "1000", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	now = start.Add(96 * time.Hour)
	withdrawal, _ := domain.ParseMoney("500", "USD")
	if _, err := service.RecordChange(ctx, domain.MoneyRemovedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: withdrawal, Reason: domain.ReasonExpense, EffectiveAt: start.Add(72 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	remove, _ := domain.ParseMoney("600", "USD")
	add, _ := domain.ParseMoney("500", "USD")
	commands := []any{
		domain.MoneyRemovedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: remove, Reason: domain.ReasonExpense, EffectiveAt: start.Add(24 * time.Hour)},
		domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: add, Reason: domain.ReasonIncome, EffectiveAt: start.Add(48 * time.Hour)},
	}
	previews, token, err := service.PreviewChangesGuardedWith(ctx, func(context.Context) ([]any, error) { return commands, nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(previews) != 2 || previews[0].Resulting[0].Amount != "400" || previews[1].Resulting[0].Amount != "900" {
		t.Fatalf("batch preview = %+v", previews)
	}
	id := domain.NewMutationID().String()
	hash := strings.Repeat("ab", 32)
	committed, err := service.RecordChangesGuarded(ctx, commands, id, hash, token)
	if err != nil {
		t.Fatal(err)
	}
	if len(committed) != 2 || committed[0].Activity.ID == committed[1].Activity.ID {
		t.Fatalf("committed batch = %+v", committed)
	}
	var amount string
	if err := db.SQL.QueryRowContext(ctx, `SELECT amount FROM account_values WHERE account_id = ? ORDER BY effective_at DESC, created_at DESC, id DESC LIMIT 1`, account.Account.ID.String()).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if amount != "400" {
		t.Fatalf("current amount = %s, want 400", amount)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	restarted := NewService(sqlite.NewRepository(db))
	replayed, err := restarted.RecordChangesGuarded(ctx, commands, id, hash, token)
	if err != nil || len(replayed) != 2 || replayed[0].Activity.ID != committed[0].Activity.ID || replayed[1].Activity.ID != committed[1].Activity.ID {
		t.Fatalf("replayed batch = %+v, %v", replayed, err)
	}
	if replayed[0].Resulting[0].Amount != committed[0].Resulting[0].Amount || replayed[1].Resulting[0].Amount != committed[1].Resulting[0].Amount {
		t.Fatalf("replayed results = %+v, want %+v", replayed, committed)
	}
	if _, err := restarted.RecordChangesGuarded(ctx, commands, id, strings.Repeat("cd", 32), token); !hasDomainCode(err, domain.ErrConflict) {
		t.Fatalf("changed payload = %v, want conflict", err)
	}
}

func TestGuardedChangeBatchRollsBackAllFacts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "rollback.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := NewService(sqlite.NewRepository(db))
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := start
	service.SetClock(func() time.Time { return now })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Batch", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, _ := service.Bootstrap(ctx)
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "100", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	broker, err := service.CreateAccount(ctx, AccountInput{Name: "Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, broker.Account.ID, "100", "USD", "2026-01-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	now = start.Add(time.Hour)
	amount, _ := domain.ParseMoney("70", "USD")
	commands := []any{
		domain.TradeInput{HouseholdID: bootstrap.Household.ID, Side: domain.TradeBuy, SettlementAccountID: broker.Account.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, "1"), Gross: mustMoney(t, "50", "USD"), EffectiveAt: now},
		domain.MoneyRemovedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: amount, Reason: domain.ReasonExpense, EffectiveAt: now},
	}
	tooMuch := mustMoney(t, "1000", "USD")
	invalid := []any{commands[0], domain.MoneyRemovedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: tooMuch, Reason: domain.ReasonExpense, EffectiveAt: now}}
	if _, _, err := service.PreviewChangesGuardedWith(ctx, func(context.Context) ([]any, error) { return invalid, nil }); !hasDomainCode(err, domain.ErrInsufficientBalance) || !strings.Contains(err.Error(), "commands[1].amount") {
		t.Fatalf("invalid second command = %v", err)
	}
	_, token, err := service.PreviewChangesGuardedWith(ctx, func(context.Context) ([]any, error) { return commands, nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.ExecContext(ctx, `CREATE TRIGGER reject_batch_second_activity BEFORE INSERT ON activities WHEN NEW.kind = 'cash_out' BEGIN SELECT RAISE(ABORT, 'blocked second activity'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordChangesGuarded(ctx, commands, domain.NewMutationID().String(), strings.Repeat("ab", 32), token); err == nil {
		t.Fatal("second insert should fail and roll back first insert")
	}
	var activities, receipts, holdings int
	if err := db.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM activities`).Scan(&activities); err != nil {
		t.Fatal(err)
	}
	if err := db.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM change_batch_mutation_keys`).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if err := db.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM holdings`).Scan(&holdings); err != nil {
		t.Fatal(err)
	}
	if activities != 0 || receipts != 0 || holdings != 0 {
		t.Fatalf("partial batch activities=%d receipts=%d holdings=%d", activities, receipts, holdings)
	}
	var bankCash, brokerCash string
	if err := db.SQL.QueryRowContext(ctx, `SELECT amount FROM account_values WHERE account_id=? ORDER BY effective_at DESC, created_at DESC, id DESC LIMIT 1`, account.Account.ID.String()).Scan(&bankCash); err != nil {
		t.Fatal(err)
	}
	if err := db.SQL.QueryRowContext(ctx, `SELECT amount FROM account_cash_values WHERE account_id=? ORDER BY effective_at DESC, created_at DESC, id DESC LIMIT 1`, broker.Account.ID.String()).Scan(&brokerCash); err != nil {
		t.Fatal(err)
	}
	if bankCash != "100" || brokerCash != "100" {
		t.Fatalf("after rollback bank cash=%s broker cash=%s, want 100 and 100", bankCash, brokerCash)
	}
}

func TestChangeBatchSameTimeOrderSurvivesFollowingSingleChange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "same-time.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db)
	service := NewService(repo)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	service.SetClock(func() time.Time { return now })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Trading", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	commands := []any{
		domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: mustMoney(t, "1000", "USD"), Reason: domain.ReasonContribution, EffectiveAt: now},
		domain.TradeInput{HouseholdID: bootstrap.Household.ID, Side: domain.TradeBuy, SettlementAccountID: account.Account.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, "2"), Gross: mustMoney(t, "200", "USD"), EffectiveAt: now},
		domain.TradeInput{HouseholdID: bootstrap.Household.ID, Side: domain.TradeSell, SettlementAccountID: account.Account.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, "1"), Gross: mustMoney(t, "120", "USD"), EffectiveAt: now},
	}
	_, token, err := service.PreviewChangesGuardedWith(ctx, func(context.Context) ([]any, error) { return commands, nil })
	if err != nil {
		t.Fatal(err)
	}
	committed, err := service.RecordChangesGuarded(ctx, commands, domain.NewMutationID().String(), strings.Repeat("ab", 32), token)
	if err != nil || len(committed) != 3 {
		t.Fatalf("commit = %+v, %v", committed, err)
	}
	if _, err := service.RecordChange(ctx, domain.MoneyRemovedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: mustMoney(t, "20", "USD"), Reason: domain.ReasonExpense, EffectiveAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	activities, err := sqlite.NewRepository(db).ListActivitiesUntil(ctx, bootstrap.Household.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.ActivityKind{domain.ActivityCashIn, domain.ActivityBuy, domain.ActivitySell, domain.ActivityCashOut}
	if len(activities) != len(want) {
		t.Fatalf("timeline has %d activities, want %d", len(activities), len(want))
	}
	for i, activity := range activities {
		if activity.Kind != want[i] {
			t.Fatalf("timeline[%d] = %s, want %s", i, activity.Kind, want[i])
		}
	}
	var cash, quantity string
	if err := db.SQL.QueryRowContext(ctx, `SELECT amount FROM account_cash_values WHERE account_id=? ORDER BY effective_at DESC, created_at DESC, id DESC LIMIT 1`, account.Account.ID.String()).Scan(&cash); err != nil {
		t.Fatal(err)
	}
	if err := db.SQL.QueryRowContext(ctx, `SELECT quantity FROM holdings WHERE account_id=? AND instrument_id=?`, account.Account.ID.String(), instrument.ID.String()).Scan(&quantity); err != nil {
		t.Fatal(err)
	}
	if cash != "900" || quantity != "1" {
		t.Fatalf("cash=%s quantity=%s, want 900 and 1", cash, quantity)
	}
}
