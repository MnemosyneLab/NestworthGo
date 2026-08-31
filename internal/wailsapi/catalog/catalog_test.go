package catalog_test

import (
	"reflect"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/catalog"
)

func TestCatalogMatchesDomainAndSettings(t *testing.T) {
	got := catalog.NewService().Catalog()

	if !reflect.DeepEqual(got.Currencies, settings.SupportedCurrencies()) {
		t.Fatalf("currencies = %v, want %v", got.Currencies, settings.SupportedCurrencies())
	}
	if !reflect.DeepEqual(got.Currencies, stringSlice(domain.SupportedCurrencies())) {
		t.Fatalf("currencies drifted from domain.SupportedCurrencies: %v", got.Currencies)
	}
	if !reflect.DeepEqual(got.InstrumentTypes, stringSlice(domain.AllInstrumentTypes())) {
		t.Fatalf("instrumentTypes = %v", got.InstrumentTypes)
	}
	if !reflect.DeepEqual(got.QuoteSources, stringSlice(domain.AllQuoteSourceKinds())) {
		t.Fatalf("quoteSources = %v", got.QuoteSources)
	}
	if !reflect.DeepEqual(got.InstrumentProviders, application.InstrumentProviderKeys()) {
		t.Fatalf("instrumentProviders = %v, want %v", got.InstrumentProviders, application.InstrumentProviderKeys())
	}
	if !reflect.DeepEqual(got.InstrumentCountryCodes, domain.SupportedInstrumentCountryCodes()) {
		t.Fatalf("instrumentCountryCodes = %v", got.InstrumentCountryCodes)
	}
	if !reflect.DeepEqual(got.InstrumentMarketCodes, domain.SupportedInstrumentMarketCodes()) {
		t.Fatalf("instrumentMarketCodes = %v", got.InstrumentMarketCodes)
	}
	if !reflect.DeepEqual(got.AccountTypes, stringSlice(domain.AllAccountTypes())) {
		t.Fatalf("accountTypes = %v", got.AccountTypes)
	}
	if len(got.AccountCombinations) != len(domain.LegalAccountCombinations()) {
		t.Fatalf("accountCombinations = %d, want %d", len(got.AccountCombinations), len(domain.LegalAccountCombinations()))
	}
	if got.AccountCombinations[0].AccountType != domain.TypeCashOnHand.String() {
		t.Fatalf("first combination = %+v", got.AccountCombinations[0])
	}
	if got.AccountCombinations[1].AccountType != domain.TypeCashOnHand.String() || got.AccountCombinations[1].TrackingMode != string(domain.TrackingHoldings) || got.AccountCombinations[1].WholeAccountWarning {
		t.Fatalf("multi-currency cash combination = %+v", got.AccountCombinations[1])
	}
	if !reflect.DeepEqual(got.BalanceSheetRoles, stringSlice(domain.AllBalanceSheetRoles())) {
		t.Fatalf("balanceSheetRoles = %v", got.BalanceSheetRoles)
	}
	if !reflect.DeepEqual(got.TrendRanges, stringSlice(domain.AllTrendRanges())) {
		t.Fatalf("trendRanges = %v", got.TrendRanges)
	}
	if !reflect.DeepEqual(got.Appearances, stringSlice(settings.AllAppearances())) {
		t.Fatalf("appearances = %v", got.Appearances)
	}
	if !reflect.DeepEqual(got.Languages, stringSlice(settings.AllLanguages())) {
		t.Fatalf("languages = %v", got.Languages)
	}
	if !reflect.DeepEqual(got.Accents, stringSlice(settings.AllAccents())) {
		t.Fatalf("accents = %v", got.Accents)
	}
	if !reflect.DeepEqual(got.MoneyInReasons, stringSlice(domain.MoneyInReasons())) {
		t.Fatalf("moneyInReasons = %v", got.MoneyInReasons)
	}
	if !reflect.DeepEqual(got.MoneyOutReasons, stringSlice(domain.MoneyOutReasons())) {
		t.Fatalf("moneyOutReasons = %v", got.MoneyOutReasons)
	}
	if !reflect.DeepEqual(got.ValueUpdateReasons, stringSlice(domain.ValueUpdateReasons())) {
		t.Fatalf("valueUpdateReasons = %v", got.ValueUpdateReasons)
	}
	if !reflect.DeepEqual(got.TradeSides, stringSlice(domain.AllTradeSides())) {
		t.Fatalf("tradeSides = %v", got.TradeSides)
	}
}

func TestCatalogIncludesKRWAndCHF(t *testing.T) {
	got := catalog.NewService().Catalog()
	want := map[string]bool{"KRW": false, "CHF": false}
	for _, currency := range got.Currencies {
		if _, ok := want[currency]; ok {
			want[currency] = true
		}
	}
	for code, found := range want {
		if !found {
			t.Fatalf("catalog missing %s", code)
		}
	}
}

func stringSlice[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = string(value)
	}
	return out
}
