package application

import (
	"reflect"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func displayTestSnapshot() domain.PortfolioSnapshot {
	return domain.PortfolioSnapshot{Household: &domain.Household{ID: "household", BaseCurrency: "CNY"}}
}

func displayTestQuote(t *testing.T, from, to domain.CurrencyCode, rate string) domain.FXQuote {
	t.Helper()
	parsed, err := domain.ParseFxRate(rate)
	if err != nil {
		t.Fatal(err)
	}
	return domain.FXQuote{ID: domain.FXQuoteID(from.String() + "-" + to.String()), HouseholdID: "household", BaseCurrency: from, QuoteCurrency: to, Rate: parsed, SourceKind: domain.QuoteSourceProvider, SourceKey: "test", QuotedAt: time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)}
}

func TestAccountDisplayValueFXPolicy(t *testing.T) {
	v := NewValuationService(nil)
	v.SetFXProviderKey(func() string { return "test" })
	for _, test := range []struct {
		name       string
		quotes     []domain.FXQuote
		preference *domain.FXPreference
		want       string
		missing    bool
		evidence   int
	}{
		{name: "direct", quotes: []domain.FXQuote{displayTestQuote(t, "SGD", "USD", "0.75")}, want: "7.5", evidence: 1},
		{name: "inverse", quotes: []domain.FXQuote{displayTestQuote(t, "USD", "SGD", "1.25")}, want: "8", evidence: 1},
		{name: "household bridge", quotes: []domain.FXQuote{displayTestQuote(t, "SGD", "CNY", "5"), displayTestQuote(t, "USD", "CNY", "8")}, want: "6.25", evidence: 2},
		{name: "direct wins over bridge", quotes: []domain.FXQuote{displayTestQuote(t, "SGD", "USD", "0.75"), displayTestQuote(t, "SGD", "CNY", "5"), displayTestQuote(t, "USD", "CNY", "8")}, want: "7.5", evidence: 1},
		{name: "missing second bridge leg", quotes: []domain.FXQuote{displayTestQuote(t, "SGD", "CNY", "5")}, missing: true},
		{name: "no arbitrary pivot", quotes: []domain.FXQuote{displayTestQuote(t, "SGD", "EUR", "1"), displayTestQuote(t, "EUR", "USD", "2")}, missing: true},
		{name: "explicit pair preference blocks fallback", quotes: []domain.FXQuote{displayTestQuote(t, "SGD", "USD", "0.75"), displayTestQuote(t, "SGD", "CNY", "5"), displayTestQuote(t, "USD", "CNY", "8")}, preference: &domain.FXPreference{HouseholdID: "household", CurrencyA: "SGD", CurrencyB: "USD", SourceKind: domain.QuoteSourceManual}, missing: true},
		{name: "bridge leg preference is honored", quotes: []domain.FXQuote{displayTestQuote(t, "SGD", "CNY", "5"), displayTestQuote(t, "USD", "CNY", "8")}, preference: &domain.FXPreference{HouseholdID: "household", CurrencyA: "CNY", CurrencyB: "USD", SourceKind: domain.QuoteSourceManual}, missing: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := displayTestSnapshot()
			snapshot.FXQuotes = test.quotes
			if test.preference != nil {
				snapshot.FXPreferences = []domain.FXPreference{*test.preference}
			}
			actual, evidence, missing, err := v.convertAccountDisplay(snapshot, "account", decimal.NewFromInt(10), "SGD", "USD")
			if err != nil {
				t.Fatal(err)
			}
			if (len(missing) > 0) != test.missing || len(evidence) != test.evidence || (!test.missing && actual.String() != test.want) {
				t.Fatalf("amount=%s evidence=%+v missing=%+v", actual, evidence, missing)
			}
		})
	}
	snapshot := displayTestSnapshot()
	quote := displayTestQuote(t, "SGD", "USD", "0.75")
	quote.SourceKey = "wrong-provider"
	snapshot.FXQuotes = []domain.FXQuote{quote}
	_, _, missing, err := v.convertAccountDisplay(snapshot, "account", decimal.NewFromInt(10), "SGD", "USD")
	if err != nil || len(missing) == 0 {
		t.Fatalf("provider filter bypassed: %v %v", missing, err)
	}
}

func TestAccountDisplayValuePrecisionAndIndependentCompleteness(t *testing.T) {
	v := NewValuationService(nil)
	snapshot := displayTestSnapshot()
	snapshot.FXQuotes = []domain.FXQuote{displayTestQuote(t, "SGD", "USD", "0.75")}
	account := domain.AccountValuation{Account: domain.Account{ID: "account", DefaultCurrency: "USD", TrackingMode: domain.TrackingHoldings}, Complete: false,
		Components: []domain.ValuationComponent{
			{NativeAmount: "0.00004", NativeCurrency: "USD", Available: false},
			{NativeAmount: "0.00004", NativeCurrency: "USD", Available: false},
			{NativeAmount: "0.00004", NativeCurrency: "USD", Available: false},
			{NativeAmount: "10", NativeCurrency: "SGD", Available: false},
		}, MissingInputs: []domain.MissingInputView{{Kind: domain.MissingFXRate, BaseCurrency: "CNY", QuoteCurrency: "USD"}}}
	before := account
	actual, err := v.projectAccountDisplayValue(snapshot, account)
	if err != nil {
		t.Fatal(err)
	}
	if actual.TotalExact != "7.50012" || actual.Value.Amount != "7.5001" || !actual.Complete || actual.ValuedCount != 4 || actual.MissingCount != 0 {
		t.Fatalf("sum before rounding/native independence: %+v", actual)
	}
	if !reflect.DeepEqual(account, before) {
		t.Fatal("projection changed household valuation")
	}
	account.Components = append(account.Components, domain.ValuationComponent{NativeCurrency: "EUR"})
	actual, err = v.projectAccountDisplayValue(snapshot, account)
	if err != nil || actual.Complete || actual.MissingCount != 1 || actual.Value == nil || actual.Value.Amount != "7.5001" {
		t.Fatalf("partial native input: %+v %v", actual, err)
	}
	account.Components = []domain.ValuationComponent{{NativeCurrency: "EUR"}}
	actual, err = v.projectAccountDisplayValue(snapshot, account)
	if err != nil || actual.Complete || actual.Value != nil || actual.TotalExact != "" {
		t.Fatalf("unknown became zero: %+v %v", actual, err)
	}
	account.Components = []domain.ValuationComponent{{NativeAmount: "0", NativeCurrency: "EUR"}}
	actual, err = v.projectAccountDisplayValue(snapshot, account)
	if err != nil || !actual.Complete || actual.Value == nil || actual.Value.Amount != "0" || len(actual.FXEvidence) != 0 {
		t.Fatalf("known zero required FX: %+v %v", actual, err)
	}
	account.Components = nil
	actual, err = v.projectAccountDisplayValue(snapshot, account)
	if err != nil || !actual.Complete || actual.Value == nil || actual.Value.Amount != "0" {
		t.Fatalf("empty: %+v %v", actual, err)
	}
}

func TestAccountDisplayValueSixteenDecimalNativeAndMissingAccountFX(t *testing.T) {
	snapshot := displayTestSnapshot()
	v := NewValuationService(nil)
	account := domain.AccountValuation{Account: domain.Account{ID: "account", DefaultCurrency: "USD"}, Complete: true, BaseValue: &domain.MoneyView{Amount: "80", Currency: "CNY"}, Components: []domain.ValuationComponent{{NativeAmount: "10.0000000000000001", NativeCurrency: "SGD", Available: true, BaseAmountExact: "80.0000000000000008"}}}
	actual, err := v.projectAccountDisplayValue(snapshot, account)
	if err != nil || actual.Complete || actual.Value != nil || actual.MissingCount != 1 || !account.Complete || account.BaseValue.Amount != "80" {
		t.Fatalf("missing display FX corrupted household: %+v %v", actual, err)
	}
	account.Account.DefaultCurrency = "SGD"
	actual, err = v.projectAccountDisplayValue(snapshot, account)
	if err != nil || actual.TotalExact != "10.0000000000000001" {
		t.Fatalf("lost native precision: %+v %v", actual, err)
	}
}

func TestAccountDisplayValueSimpleAccountsConvertLatestCurrency(t *testing.T) {
	for _, mode := range []domain.TrackingMode{domain.TrackingBalance, domain.TrackingManualValue} {
		snapshot := displayTestSnapshot()
		value, err := domain.ParseMoney("10", "SGD")
		if err != nil {
			t.Fatal(err)
		}
		snapshot.Accounts = []domain.AccountRecord{{Account: domain.Account{ID: "account", DefaultCurrency: "USD", TrackingMode: mode}, LatestValue: &domain.AccountValue{Amount: value}}}
		snapshot.FXQuotes = []domain.FXQuote{displayTestQuote(t, "SGD", "USD", "0.75")}
		values, _, err := NewValuationService(nil).ValueAccountCompositions(snapshot)
		if err != nil || len(values) != 1 {
			t.Fatalf("mode=%s values=%+v err=%v", mode, values, err)
		}
		if values[0].DisplayValue.Value.Amount != "7.5" || values[0].DisplayValue.Currency != "USD" || !values[0].DisplayValue.Complete || values[0].Complete {
			t.Fatalf("mode=%s value=%+v", mode, values[0])
		}
	}
}

func TestAccountDisplayValueListDetailAndOtherConsumers(t *testing.T) {
	fixture := newGoldenValuationFixture(t, true)
	detail, err := fixture.service.AccountValuation(t.Context(), fixture.account.Account.ID)
	if err != nil {
		t.Fatal(err)
	}
	list, err := fixture.service.AccountValuations(t.Context(), domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, account := range list {
		if account.Account.ID == detail.Account.ID {
			found = true
			if !reflect.DeepEqual(account.DisplayValue, detail.DisplayValue) {
				t.Fatalf("list/detail mismatch: %+v %+v", account.DisplayValue, detail.DisplayValue)
			}
		}
	}
	if !found || detail.DisplayValue == nil || detail.DisplayValue.TotalExact != "62190" || detail.Composition.Currency != "CNY" {
		t.Fatalf("projection not wired: %+v", detail)
	}
	snapshot, err := fixture.repository.ReadPortfolioSnapshot(t.Context(), domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	plain, _, err := NewValuationService(fixture.repository).ValueAccounts(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range plain {
		if account.DisplayValue != nil {
			t.Fatal("non-account reader opted into display projection")
		}
	}
}
