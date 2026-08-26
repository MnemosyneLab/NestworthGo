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
		Name: "Brokerage", PrimaryCategory: "investment", SecondaryCategory: "brokerage_account",
		TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInInvestment: true,
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
	if view.Quantity != "9" || view.AverageCost.Amount != "93.33" || view.AverageCost.Currency != "USD" {
		t.Fatalf("cost view = %+v", view)
	}
	if view.TotalCost.Amount != "839.97" || view.RealizedGain.Amount != "340.02" || view.UnrealizedGain == nil || view.UnrealizedGain.Amount != "960.03" {
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
	if !period.Available || len(period.ByInstrument) != 1 || period.ByInstrument[0].Gain.Amount != "2380.14" || len(period.ByAccount) != 1 || period.ByAccount[0].Gain.Amount != "2380.14" {
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

func TestGainServiceMissingCurrentQuoteKeepsCostAndRealizedGain(t *testing.T) {
	fixture := newGoldenValuationFixture(t, true)
	ctx := context.Background()
	if _, err := fixture.service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	if err := fixture.repository.SetInstrumentQuoteSource(ctx, fixture.qqq.HouseholdID, fixture.qqq.ID, domain.QuoteSourceProvider); err != nil {
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
	database := seedGainSchema6Fixture(t)
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
	if source.AverageCost.Amount != "316.67" || target.AverageCost.Amount != "720" {
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
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Brokerage", PrimaryCategory: "investment", SecondaryCategory: "brokerage_account", TrackingMode: "holdings", DefaultCurrency: "CNY", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
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

func seedGainSchema6Fixture(t *testing.T) *sqlite.DB {
	t.Helper()
	scriptPath := filepath.Join("..", "..", "testdata", "schema6", "schema6-fixture.sql")
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
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	database, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return database
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
