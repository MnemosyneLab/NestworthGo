package application

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestManualPortfolioUseCasesAreOfflineAndAtomicAtTheRepositoryBoundary(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/portfolio.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return now })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Portfolio", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owner := bootstrap.Members[0].ID
	foreign, err := service.CreateAccount(ctx, AccountInput{Name: "USD balance", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", IncludeInNetWorth: true, Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "100"})
	if err != nil {
		t.Fatalf("foreign-currency Balance account: %v", err)
	}
	if foreign.Account.DefaultCurrency != domain.CurrencyCode("USD") || foreign.LatestValue == nil || foreign.LatestValue.Amount.Currency() != domain.CurrencyCode("USD") {
		t.Fatalf("foreign account = %+v", foreign)
	}
	holdingsAccount, err := service.CreateAccount(ctx, AccountInput{Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true, Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}}})
	if err != nil {
		t.Fatalf("Holdings account: %v", err)
	}
	if holdingsAccount.LatestValue != nil {
		t.Fatal("Holdings account received a fake initial value")
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "QQQ", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatalf("instrument: %v", err)
	}
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: holdingsAccount.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "3"})
	if err != nil {
		t.Fatalf("holding: %v", err)
	}
	updatedHolding, err := service.UpdateHoldingQuantity(ctx, holding.ID, "3.5")
	if err != nil || updatedHolding.Quantity.Canonical() != "3.5" {
		t.Fatalf("updated holding = %+v, err = %v", updatedHolding, err)
	}
	if _, err := service.AppendAccountCashValue(ctx, holdingsAccount.Account.ID, "5000", "SGD", "2026-08-23"); err != nil {
		t.Fatalf("cash: %v", err)
	}
	quote, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "700", "2026-08-23", false)
	if err != nil {
		t.Fatalf("manual price: %v", err)
	}
	if quote.SourceKind != domain.QuoteSourceManual || quote.SourceKey != string(domain.QuoteSourceManual) {
		t.Fatalf("manual quote = %+v", quote)
	}
	quotes, err := service.repository.ListInstrumentQuotes(ctx, instrument.ID)
	if err != nil || len(quotes) != 1 {
		t.Fatalf("manual quote history = %d, err = %v", len(quotes), err)
	}
	quoteHistory, err := service.InstrumentQuoteHistory(ctx, instrument.ID)
	if err != nil || len(quoteHistory) != 1 || quoteHistory[0].ID != quote.ID {
		t.Fatalf("application quote history = %d, err = %v", len(quoteHistory), err)
	}
	if err := service.SetInstrumentQuoteSource(ctx, instrument.ID, "manual"); err != nil {
		t.Fatalf("standalone manual source change: %v", err)
	}
	quotes, err = service.repository.ListInstrumentQuotes(ctx, instrument.ID)
	if err != nil || len(quotes) != 1 {
		t.Fatalf("standalone source change appended quote: %d, err = %v", len(quotes), err)
	}
	fx, err := service.AppendManualFXQuote(ctx, "USD", "CNY", "6.9", "2026-08-23")
	if err != nil {
		t.Fatalf("manual FX: %v", err)
	}
	if fx.BaseCurrency != domain.CurrencyCode("USD") || fx.QuoteCurrency != domain.CurrencyCode("CNY") {
		t.Fatalf("manual FX orientation = %+v", fx)
	}
	fxHistory, err := service.FXQuoteHistory(ctx)
	if err != nil || len(fxHistory) != 1 || fxHistory[0].ID != fx.ID {
		t.Fatalf("application FX history = %d, err = %v", len(fxHistory), err)
	}
	currentFX, err := service.CurrentFXQuote(ctx, "USD", "CNY")
	if err != nil || currentFX == nil || currentFX.ID != fx.ID {
		t.Fatalf("application current FX = %+v, err = %v", currentFX, err)
	}
	pref, err := service.repository.FXPreference(ctx, bootstrap.Household.ID, domain.CurrencyCode("USD"), domain.CurrencyCode("CNY"))
	if err != nil || pref.SourceKind != domain.QuoteSourceManual {
		t.Fatalf("FX preference = %+v, err = %v", pref, err)
	}
	if err := service.ArchiveHolding(ctx, holding.ID, true); err != nil {
		t.Fatalf("archive holding: %v", err)
	}
	if err := service.ArchiveHolding(ctx, holding.ID, false); err != nil {
		t.Fatalf("restore holding: %v", err)
	}
	if err := service.ArchiveInstrument(ctx, instrument.ID, true); err != nil {
		t.Fatalf("archive instrument: %v", err)
	}
	if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: holdingsAccount.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "1"}); err == nil {
		t.Fatal("new holding selected an archived instrument")
	}
	if err := service.ArchiveInstrument(ctx, instrument.ID, false); err != nil {
		t.Fatalf("restore instrument: %v", err)
	}
	if _, err := service.UpdateAccount(ctx, holdingsAccount.Account.ID, AccountInput{Name: "Brokerage updated"}); err != nil {
		t.Fatalf("Holdings Account metadata update: %v", err)
	}
}

func TestProviderBindingCanBeSelectedWithoutNetwork(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/provider-binding.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Portfolio", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Bound", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual", ProviderKey: "yahoo_finance", ProviderSymbol: "QQQ"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SetInstrumentQuoteSource(ctx, instrument.ID, "provider"); err != nil {
		t.Fatalf("select provider: %v", err)
	}
	items, err := service.ListInstruments(ctx, false)
	if err != nil || len(items) != 1 || items[0].QuoteSource != domain.QuoteSourceProvider {
		t.Fatalf("provider selection = %+v, err = %v", items, err)
	}
}

func TestInstrumentReplacementClearsOptionalFieldsAndPartialUpdatesPreserveThem(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/instrument-edit.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return now })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Portfolio", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	note := "keep me"
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{
		Name: "Bound", Type: "stock", QuoteCurrency: "USD", Symbol: "QQQ", MarketCode: "nasdaq",
		CountryCode: "us", ISIN: "us0000000001", Note: &note, SortOrder: 7,
		QuoteSource: "provider", ProviderKey: "yahoo_finance", ProviderSymbol: "QQQ",
	})
	if err != nil {
		t.Fatal(err)
	}
	partial, err := service.UpdateInstrument(ctx, instrument.ID, InstrumentInput{Name: "Renamed"})
	if err != nil {
		t.Fatal(err)
	}
	if partial.Symbol == nil || *partial.Symbol != "QQQ" || partial.Note == nil || *partial.Note != note || partial.SortOrder != 7 || partial.QuoteSource != domain.QuoteSourceProvider {
		t.Fatalf("partial update did not preserve omitted fields: %+v", partial)
	}
	cleared, err := service.UpdateInstrument(ctx, instrument.ID, InstrumentInput{
		Replace: true, Name: "Manual", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual", SortOrder: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Symbol != nil || cleared.MarketCode != nil || cleared.CountryCode != nil || cleared.ISIN != nil || cleared.Note != nil || cleared.ProviderKey != nil || cleared.ProviderSymbol != nil || cleared.SortOrder != 0 || cleared.QuoteSource != domain.QuoteSourceManual {
		t.Fatalf("full replacement did not clear optional fields: %+v", cleared)
	}
}
