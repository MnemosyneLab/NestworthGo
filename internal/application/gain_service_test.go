package application

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestGainServiceHoldingGainReplaysStartingPointBuySellThroughRepository(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/gain.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repository := sqlite.NewRepository(database)
	service := NewService(repository)
	now := time.Date(2026, time.August, 24, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return now })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Gain Household", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{
		Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset",
		TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true,
		OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, account.Account.ID, "1000", "USD", "2026-08-24"); err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Fixture ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "10"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "80", "2026-08-24T12:00:00Z", false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualFXQuote(ctx, "USD", "CNY", "7", "2026-08-24T12:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	service.setClock(func() time.Time { return now.Add(time.Hour) })
	if _, err := service.RecordChange(ctx, domain.TradeInput{
		HouseholdID: bootstrap.Household.ID, Side: domain.TradeBuy, SettlementAccountID: account.Account.ID,
		HoldingID: holding.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, "5"),
		Gross: mustMoney(t, "600", "USD"), EffectiveAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	service.setClock(func() time.Time { return now.Add(2 * time.Hour) })
	if _, err := service.RecordChange(ctx, domain.TradeInput{
		HouseholdID: bootstrap.Household.ID, Side: domain.TradeSell, SettlementAccountID: account.Account.ID,
		HoldingID: holding.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, "6"),
		Gross: mustMoney(t, "900", "USD"), EffectiveAt: now.Add(2 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "200", "2026-08-24T15:00:00Z", false); err != nil {
		t.Fatal(err)
	}

	view, err := NewGainService(repository, func() time.Time { return now.Add(3 * time.Hour) }).HoldingGain(ctx, holding.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Quantity != "9" || view.AverageCost.Amount != "93.3333" || view.AverageCost.Currency != "USD" {
		t.Fatalf("cost view = %+v", view)
	}
	if view.TotalCost.Amount != "840" || view.RealizedGain.Amount != "340" || view.UnrealizedGain == nil || view.UnrealizedGain.Amount != "960" {
		t.Fatalf("gain view = %+v", view)
	}
	if view.CurrentValue == nil || view.CurrentValue.Amount != "1800" || !view.Available {
		t.Fatalf("current view = %+v", view)
	}
	scope := domain.GainScope{AccountID: &account.Account.ID}
	period, err := service.RealizedGainInRange(ctx, scope, "2026-08-24", "2026-08-24")
	if err != nil {
		t.Fatal(err)
	}
	if !period.Available || len(period.ByInstrument) != 1 || period.ByInstrument[0].Gain.Amount != "2380" || len(period.ByAccount) != 1 || period.ByAccount[0].Gain.Amount != "2380" {
		t.Fatalf("realized period = %+v", period)
	}
	excluded, err := service.RealizedGainInRange(ctx, scope, "2026-08-25", "2026-08-25")
	if err != nil {
		t.Fatal(err)
	}
	if len(excluded.ByInstrument) != 0 || len(excluded.ByAccount) != 0 {
		t.Fatalf("sell outside range was included = %+v", excluded)
	}
}

func TestAccountGainIncludesActiveHoldingWithArchivedInstrument(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/gain-archived-instrument.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	now := time.Date(2026, time.August, 24, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return now })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Archived instrument gain", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{
		Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset",
		TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInPortfolio: true,
		OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "QQQ", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "10"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "12", "2026-08-24", false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	if err := service.ArchiveInstrument(ctx, instrument.ID, true); err != nil {
		t.Fatal(err)
	}

	accountGain, err := service.AccountGain(ctx, account.Account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(accountGain.Holdings) != 1 || accountGain.Holdings[0].HoldingID != holding.ID {
		t.Fatalf("archived instrument dropped from current account gain: %+v", accountGain)
	}
	if accountGain.Holdings[0].Quantity != "10" || accountGain.Holdings[0].CurrentValue == nil || accountGain.Holdings[0].CurrentValue.Amount != "120" {
		t.Fatalf("archived instrument current gain = %+v", accountGain.Holdings[0])
	}
	holdingGain, err := service.HoldingGain(ctx, holding.ID)
	if err != nil {
		t.Fatal(err)
	}
	if holdingGain.InstrumentName != "QQQ" || holdingGain.CurrentValue == nil || holdingGain.CurrentValue.Amount != "120" {
		t.Fatalf("holding gain after instrument archive = %+v", holdingGain)
	}
}

func TestRealizedGainIncludesSoldThenArchivedHolding(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/gain-archived-holding.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	now := time.Date(2026, time.August, 24, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return now })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Archived holding gain", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{
		Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset",
		TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInPortfolio: true,
		OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, account.Account.ID, "1000", "USD", "2026-08-24"); err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "QQQ", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "10"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "10", "2026-08-24", false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	service.setClock(func() time.Time { return now.Add(time.Hour) })
	if _, err := service.RecordChange(ctx, domain.TradeInput{
		HouseholdID: bootstrap.Household.ID, Side: domain.TradeSell, SettlementAccountID: account.Account.ID,
		HoldingID: holding.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, "10"),
		Gross: mustMoney(t, "150", "USD"), EffectiveAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	before, err := service.RealizedGainInRange(ctx, domain.GainScope{AccountID: &account.Account.ID}, "2026-08-24", "2026-08-24")
	if err != nil {
		t.Fatal(err)
	}
	if !before.Available || len(before.ByInstrument) != 1 || before.ByInstrument[0].Gain.Amount != "50" {
		t.Fatalf("realized gain before archive = %+v", before)
	}
	if err := service.ArchiveHolding(ctx, holding.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := service.ArchiveInstrument(ctx, instrument.ID, true); err != nil {
		t.Fatal(err)
	}

	after, err := service.RealizedGainInRange(ctx, domain.GainScope{AccountID: &account.Account.ID}, "2026-08-24", "2026-08-24")
	if err != nil {
		t.Fatal(err)
	}
	if !after.Available || len(after.ByInstrument) != 1 || after.ByInstrument[0].Gain.Amount != "50" || after.ByInstrument[0].Label != "QQQ" {
		t.Fatalf("sold then archived holding dropped from realized gain: %+v", after)
	}
	accountGain, err := service.AccountGain(ctx, account.Account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(accountGain.Holdings) != 0 {
		t.Fatalf("archived holding remained in current account gain: %+v", accountGain)
	}
}

func TestHoldingGainIncludesBuyFeeInAverageCost(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/gain-buy-fee.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	now := time.Date(2026, time.August, 24, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return now })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Buy fee gain", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{
		Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset",
		TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInPortfolio: true,
		OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, account.Account.ID, "1000", "USD", "2026-08-24"); err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "QQQ", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "10"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "10", "2026-08-24", false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	fee := mustMoney(t, "10", "USD")
	service.setClock(func() time.Time { return now.Add(time.Hour) })
	if _, err := service.RecordChange(ctx, domain.TradeInput{
		HouseholdID: bootstrap.Household.ID, Side: domain.TradeBuy, SettlementAccountID: account.Account.ID,
		HoldingID: holding.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, "10"),
		Gross: mustMoney(t, "100", "USD"), Fee: &fee, EffectiveAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "12", "2026-08-24T13:00:00Z", false); err != nil {
		t.Fatal(err)
	}

	view, err := service.HoldingGain(ctx, holding.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Starting 10 at 10 plus buy 10 at (100+10)/10 = 11 => average 10.5, total 210.
	// Quote 12 * 20 = 240, unrealized 30.
	if view.Quantity != "20" || view.AverageCost.Amount != "10.5" || view.TotalCost.Amount != "210" {
		t.Fatalf("fee-adjusted cost view = %+v", view)
	}
	if view.CurrentValue == nil || view.CurrentValue.Amount != "240" || view.UnrealizedGain == nil || view.UnrealizedGain.Amount != "30" {
		t.Fatalf("fee-adjusted current view = %+v", view)
	}
}

func TestGainServiceMissingCurrentQuoteKeepsCostAndRealizedGain(t *testing.T) {
	fixture := newGoldenValuationFixture(t, true)
	ctx := context.Background()
	if _, err := fixture.service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	if err := fixture.repository.SetInstrumentQuoteSource(ctx, fixture.qqq.HouseholdID, fixture.qqq.ID, domain.QuoteSourceProvider, time.Now()); err != nil {
		t.Fatal(err)
	}
	view, err := fixture.service.HoldingGain(ctx, findHoldingForInstrument(t, fixture.repository, fixture.account.Account.ID, fixture.qqq.ID))
	if err != nil {
		t.Fatal(err)
	}
	if view.Available || view.CurrentValue != nil || view.UnrealizedGain != nil {
		t.Fatalf("missing current quote was not explained: %+v", view)
	}
	if view.TotalCost.Amount != "2100" || view.RealizedGain.Amount != "0" {
		t.Fatalf("cost/realized disappeared with quote: %+v", view)
	}
}

func TestGainServiceTransferUsesSendingCostAtTransferTime(t *testing.T) {
	database := seedGainSchema7Fixture(t)
	defer database.Close()
	repository := sqlite.NewRepository(database)
	service := NewGainService(repository, func() time.Time { return time.Date(2026, time.January, 8, 0, 0, 0, 0, time.UTC) })
	ctx := context.Background()

	source, err := service.HoldingGain(ctx, domain.HoldingID("00000000-0000-4000-8000-000000000050"))
	if err != nil {
		t.Fatal(err)
	}
	target, err := service.HoldingGain(ctx, domain.HoldingID("00000000-0000-4000-8000-000000000053"))
	if err != nil {
		t.Fatal(err)
	}
	if source.AverageCost.Amount != "316.6667" || target.AverageCost.Amount != "720" {
		t.Fatalf("transfer cost resolution used today's source cost: source=%+v target=%+v", source, target)
	}
}

func TestGainServiceCurrencyDecompositionUsesAcquisitionAndCurrentFX(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/decomposition.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repository := sqlite.NewRepository(database)
	service := NewService(repository)
	now := time.Date(2026, time.August, 24, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return now })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Decomposition", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Fixture ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "10"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "80", "2026-08-24", false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualFXQuote(ctx, "USD", "CNY", "6.8", "2026-08-24"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	service.setClock(func() time.Time { return now.Add(24 * time.Hour) })
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "100", "2026-08-25", false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualFXQuote(ctx, "USD", "CNY", "7.1", "2026-08-25"); err != nil {
		t.Fatal(err)
	}

	view, err := service.HoldingGain(ctx, holding.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.TotalCostBase == nil || view.TotalCostBase.Amount != "5440" || view.UnrealizedGainBase == nil || view.UnrealizedGainBase.Amount != "1660" {
		t.Fatalf("base gain = %+v", view)
	}
	if view.InstrumentMovement == nil || view.InstrumentMovement.Amount != "1360" || view.CurrencyMovement == nil || view.CurrencyMovement.Amount != "300" {
		t.Fatalf("currency decomposition = %+v", view)
	}
}

func seedGainSchema7Fixture(t *testing.T) *sqlite.DB {
	t.Helper()
	scriptPath := filepath.Join("..", "..", "testdata", "schema7", "schema7-fixture.sql")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "gain-fixture.db")
	seed, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(string(script)); err != nil {
		_ = seed.Close()
		t.Fatal(err)
	}
	if _, err := seed.Exec(`ALTER TABLE members ADD COLUMN icon_key TEXT NOT NULL DEFAULT 'user'; ALTER TABLE instruments ADD COLUMN icon_key TEXT NOT NULL DEFAULT 'investment';`); err != nil {
		_ = seed.Close()
		t.Fatal(err)
	}
	return &sqlite.DB{SQL: seed, Path: path, Status: sqlite.StatusReady}
}

func TestDividendIncomeAggregatesIndependentlyOfRealizedGain(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/dividend-income.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repository := sqlite.NewRepository(database)
	service := NewService(repository)
	now := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return now })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Dividend income", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	firstAccount, err := service.CreateAccount(ctx, AccountInput{
		Name: "Brokerage A", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "USD", IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondAccount, err := service.CreateAccount(ctx, AccountInput{
		Name: "Brokerage B", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "USD", IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	qqq, err := service.CreateInstrument(ctx, InstrumentInput{Name: "QQQ", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	aapl, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Apple", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, qqq.ID, "100", "2026-08-24", false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, aapl.ID, "50", "2026-08-24", false); err != nil {
		t.Fatal(err)
	}
	qqqHolding, err := service.CreateHolding(ctx, HoldingInput{AccountID: firstAccount.Account.ID.String(), InstrumentID: qqq.ID.String(), Quantity: "10"})
	if err != nil {
		t.Fatal(err)
	}
	aaplHolding, err := service.CreateHolding(ctx, HoldingInput{AccountID: secondAccount.Account.ID.String(), InstrumentID: aapl.ID.String(), Quantity: "4"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, firstAccount.Account.ID, "50", "USD", "2026-08-24"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, secondAccount.Account.ID, "20", "USD", "2026-08-24"); err != nil {
		t.Fatal(err)
	}
	service.setClock(func() time.Time { return time.Date(2026, time.August, 24, 12, 0, 0, 0, time.UTC) })
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	service.setClock(func() time.Time { return now })
	qqqAmount, _ := domain.ParseMoney("10", "USD")
	aaplAmount, _ := domain.ParseMoney("20", "USD")
	laterAmount, _ := domain.ParseMoney("5", "USD")
	if _, err := service.RecordChange(ctx, domain.CashDividendInput{HouseholdID: bootstrap.Household.ID, HoldingID: qqqHolding.ID, Amount: qqqAmount, EffectiveAt: time.Date(2026, time.August, 24, 15, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordChange(ctx, domain.CashDividendInput{HouseholdID: bootstrap.Household.ID, HoldingID: aaplHolding.ID, Amount: aaplAmount, EffectiveAt: time.Date(2026, time.August, 24, 16, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	later, err := service.RecordChange(ctx, domain.CashDividendInput{HouseholdID: bootstrap.Household.ID, HoldingID: qqqHolding.ID, Amount: laterAmount, EffectiveAt: time.Date(2026, time.August, 26, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}

	day, err := service.DividendIncomeInRange(ctx, domain.GainScope{}, "2026-08-24", "2026-08-24")
	if err != nil {
		t.Fatal(err)
	}
	if !day.Available || day.Currency != "USD" {
		t.Fatalf("same-currency dividend income unavailable: %+v", day)
	}
	qqqGroup := gainGroupByKey(t, day.ByInstrument, qqq.ID.String())
	aaplGroup := gainGroupByKey(t, day.ByInstrument, aapl.ID.String())
	if qqqGroup.Gain.Amount != "10" || qqqGroup.Label != "QQQ" || aaplGroup.Gain.Amount != "20" || aaplGroup.Label != "Apple" {
		t.Fatalf("by instrument = %+v", day.ByInstrument)
	}
	if len(day.ByInstrument) != 2 || day.ByInstrument[0].Label != "Apple" || day.ByInstrument[1].Label != "QQQ" {
		t.Fatalf("instrument groups were not largest-first: %+v", day.ByInstrument)
	}
	if gainGroupByKey(t, day.ByAccount, firstAccount.Account.ID.String()).Gain.Amount != "10" || gainGroupByKey(t, day.ByAccount, secondAccount.Account.ID.String()).Gain.Amount != "20" {
		t.Fatalf("by account = %+v", day.ByAccount)
	}
	if len(day.ByAccount) != 2 || day.ByAccount[0].Label != "Brokerage B" || day.ByAccount[1].Label != "Brokerage A" {
		t.Fatalf("account groups were not largest-first: %+v", day.ByAccount)
	}

	realized, err := service.RealizedGainInRange(ctx, domain.GainScope{}, "2026-08-24", "2026-08-26")
	if err != nil {
		t.Fatal(err)
	}
	if len(realized.ByInstrument) != 0 || len(realized.ByAccount) != 0 {
		t.Fatalf("dividends were classified as realized sell gain: %+v", realized)
	}

	scoped, err := service.DividendIncomeInRange(ctx, domain.GainScope{AccountID: &firstAccount.Account.ID, InstrumentID: &qqq.ID}, "2026-08-24", "2026-08-26")
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped.ByInstrument) != 1 || scoped.ByInstrument[0].Gain.Amount != "15" || len(scoped.ByAccount) != 1 {
		t.Fatalf("scoped dividend income = %+v", scoped)
	}

	if err := service.ArchiveInstrument(ctx, qqq.ID, true); err != nil {
		t.Fatal(err)
	}
	archived, err := service.DividendIncomeInRange(ctx, domain.GainScope{}, "2026-08-24", "2026-08-24")
	if err != nil {
		t.Fatal(err)
	}
	if gainGroupByKey(t, archived.ByInstrument, qqq.ID.String()).Label != "QQQ" {
		t.Fatalf("archived instrument dropped from dividend income: %+v", archived.ByInstrument)
	}

	if _, err := service.UndoChange(ctx, later.Activity.ID); err != nil {
		t.Fatal(err)
	}
	afterUndo, err := service.DividendIncomeInRange(ctx, domain.GainScope{}, "2026-08-26", "2026-08-26")
	if err != nil {
		t.Fatal(err)
	}
	if len(afterUndo.ByInstrument) != 0 {
		t.Fatalf("reversed dividend remained in income: %+v", afterUndo)
	}
}

func TestDividendIncomeMissingFXMarksGroupUnavailable(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/dividend-fx.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	now := time.Date(2026, time.August, 24, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return now })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Dividend FX", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{
		Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "USD", IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	localAccount, err := service.CreateAccount(ctx, AccountInput{
		Name: "Local Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "CNY", IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	usdInstrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "QQQ", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	cnyInstrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "510300", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, usdInstrument.ID, "100", "2026-08-24", false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, cnyInstrument.ID, "4", "2026-08-24", false); err != nil {
		t.Fatal(err)
	}
	usdHolding, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: usdInstrument.ID.String(), Quantity: "10"})
	if err != nil {
		t.Fatal(err)
	}
	cnyHolding, err := service.CreateHolding(ctx, HoldingInput{AccountID: localAccount.Account.ID.String(), InstrumentID: cnyInstrument.ID.String(), Quantity: "100"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, account.Account.ID, "50", "USD", "2026-08-24"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, localAccount.Account.ID, "200", "CNY", "2026-08-24"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	usdAmount, _ := domain.ParseMoney("10", "USD")
	cnyAmount, _ := domain.ParseMoney("30", "CNY")
	if _, err := service.RecordChange(ctx, domain.CashDividendInput{HouseholdID: bootstrap.Household.ID, HoldingID: usdHolding.ID, Amount: usdAmount, EffectiveAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordChange(ctx, domain.CashDividendInput{HouseholdID: bootstrap.Household.ID, HoldingID: cnyHolding.ID, Amount: cnyAmount, EffectiveAt: now}); err != nil {
		t.Fatal(err)
	}

	missing, err := service.DividendIncomeInRange(ctx, domain.GainScope{}, "2026-08-24", "2026-08-24")
	if err != nil {
		t.Fatal(err)
	}
	if missing.Available || missing.MissingReason == "" {
		t.Fatalf("missing FX was not explained: %+v", missing)
	}
	usdGroup := gainGroupByKey(t, missing.ByInstrument, usdInstrument.ID.String())
	cnyGroup := gainGroupByKey(t, missing.ByInstrument, cnyInstrument.ID.String())
	if usdGroup.Available || usdGroup.MissingReason == "" {
		t.Fatalf("USD dividend group should be unavailable: %+v", usdGroup)
	}
	if !cnyGroup.Available || cnyGroup.Gain.Amount != "30" {
		t.Fatalf("CNY dividend should still convert: %+v", cnyGroup)
	}

	if _, err := service.AppendManualFXQuote(ctx, "USD", "CNY", "7", "2026-08-24"); err != nil {
		t.Fatal(err)
	}
	converted, err := service.DividendIncomeInRange(ctx, domain.GainScope{}, "2026-08-24", "2026-08-24")
	if err != nil {
		t.Fatal(err)
	}
	if !converted.Available || gainGroupByKey(t, converted.ByInstrument, usdInstrument.ID.String()).Gain.Amount != "70" {
		t.Fatalf("converted dividend income = %+v", converted)
	}
}

func TestFXQuoteWithoutPreferenceAgreesAcrossOverviewGainAndDividend(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/fx-no-preference.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repository := sqlite.NewRepository(database)
	service := NewService(repository)
	now := time.Date(2026, time.August, 24, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return now })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "FX agreement", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{
		Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInPortfolio: true,
		OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, account.Account.ID, "1000", "USD", "2026-08-24"); err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "QQQ", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "10"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "10", "2026-08-24", false); err != nil {
		t.Fatal(err)
	}
	rate, err := domain.ParseFxRate("7")
	if err != nil {
		t.Fatal(err)
	}
	quote, err := domain.NewFXQuote(domain.FXQuoteInput{
		HouseholdID: bootstrap.Household.ID, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: rate,
		SourceKind: domain.QuoteSourceProvider, SourceKey: "frankfurter", QuotedAt: now,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.AppendFXQuote(ctx, quote); err != nil {
		t.Fatal(err)
	}
	preferences, err := service.ListFXPreferences(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(preferences) != 0 {
		t.Fatalf("explicit FX preference was stored: %+v", preferences)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}

	overview, err := service.Overview(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if !overview.Complete || overview.Assets.String() != "7700" {
		t.Fatalf("overview without explicit FX preference = %+v", overview)
	}

	amount, _ := domain.ParseMoney("10", "USD")
	if _, err := service.RecordChange(ctx, domain.CashDividendInput{HouseholdID: bootstrap.Household.ID, HoldingID: holding.ID, Amount: amount, EffectiveAt: now}); err != nil {
		t.Fatal(err)
	}
	dividends, err := service.DividendIncomeInRange(ctx, domain.GainScope{}, "2026-08-24", "2026-08-24")
	if err != nil {
		t.Fatal(err)
	}
	if !dividends.Available || len(dividends.ByInstrument) != 1 || dividends.ByInstrument[0].Gain.Amount != "70" {
		t.Fatalf("dividend without explicit FX preference = %+v", dividends)
	}

	service.setClock(func() time.Time { return now.Add(time.Hour) })
	if _, err := service.RecordChange(ctx, domain.TradeInput{
		HouseholdID: bootstrap.Household.ID, Side: domain.TradeSell, SettlementAccountID: account.Account.ID,
		HoldingID: holding.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, "10"),
		Gross: mustMoney(t, "150", "USD"), EffectiveAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	realized, err := service.RealizedGainInRange(ctx, domain.GainScope{}, "2026-08-24", "2026-08-24")
	if err != nil {
		t.Fatal(err)
	}
	if !realized.Available || len(realized.ByInstrument) != 1 || realized.ByInstrument[0].Gain.Amount != "350" {
		t.Fatalf("realized gain without explicit FX preference = %+v", realized)
	}
}

func gainGroupByKey(t *testing.T, groups []domain.GainGroupView, key string) domain.GainGroupView {
	t.Helper()
	for _, group := range groups {
		if group.Key == key {
			return group
		}
	}
	t.Fatalf("gain group %s was not found in %+v", key, groups)
	return domain.GainGroupView{}
}

func mustMoney(t *testing.T, amount, currency string) domain.Money {
	t.Helper()
	money, err := domain.ParseMoney(amount, domain.CurrencyCode(currency))
	if err != nil {
		t.Fatal(err)
	}
	return money
}

func findHoldingForInstrument(t *testing.T, repository Repository, accountID domain.AccountID, instrumentID domain.InstrumentID) domain.HoldingID {
	t.Helper()
	holdings, err := repository.ListHoldings(context.Background(), accountID, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, holding := range holdings {
		if holding.InstrumentID == instrumentID {
			return holding.ID
		}
	}
	t.Fatalf("holding for instrument %s was not found", instrumentID)
	return ""
}
