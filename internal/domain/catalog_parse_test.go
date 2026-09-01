package domain

import "testing"

func TestParseSupportedCurrencyRejectsUnknownCodes(t *testing.T) {
	if _, err := ParseSupportedCurrency("CAD"); err == nil {
		t.Fatal("ParseSupportedCurrency accepted CAD")
	}
	if _, err := ParseSupportedCurrency("XXX"); err == nil {
		t.Fatal("ParseSupportedCurrency accepted XXX")
	}
	code, err := ParseSupportedCurrency(" krw ")
	if err != nil || code != CurrencyCode("KRW") {
		t.Fatalf("ParseSupportedCurrency(KRW) = %q, %v", code, err)
	}
	if _, err := ParseCurrency("CAD"); err != nil {
		t.Fatalf("ParseCurrency should still accept syntax-only CAD: %v", err)
	}
}

func TestSupportedCurrenciesIncludesKRWAndCHF(t *testing.T) {
	found := map[CurrencyCode]bool{"KRW": false, "CHF": false, "USD": false, "CNY": false}
	for _, code := range SupportedCurrencies() {
		if _, ok := found[code]; ok {
			found[code] = true
		}
	}
	for code, ok := range found {
		if !ok {
			t.Fatalf("SupportedCurrencies missing %s", code)
		}
	}
}

func TestCurrencyFractionDigitsCoversSupportedCurrencies(t *testing.T) {
	for _, code := range SupportedCurrencies() {
		if _, ok := currencyFractionDigits[code]; !ok {
			t.Fatalf("currencyFractionDigits missing %s", code)
		}
	}
	if got := CurrencyFractionDigits("JPY"); got != 0 {
		t.Fatalf("CurrencyFractionDigits(JPY) = %d, want 0", got)
	}
	if got := CurrencyFractionDigits("KRW"); got != 0 {
		t.Fatalf("CurrencyFractionDigits(KRW) = %d, want 0", got)
	}
	if got := CurrencyFractionDigits("CNY"); got != 2 {
		t.Fatalf("CurrencyFractionDigits(CNY) = %d, want 2", got)
	}
	if got := CurrencyFractionDigits("SGD"); got != 2 {
		t.Fatalf("CurrencyFractionDigits(SGD) = %d, want 2", got)
	}
	if got := CurrencyFractionDigits("CAD"); got != 2 {
		t.Fatalf("CurrencyFractionDigits(unknown) = %d, want 2", got)
	}
}

func TestParseTrendRange(t *testing.T) {
	if _, err := ParseTrendRange("week"); err == nil {
		t.Fatal("ParseTrendRange accepted week")
	}
	got, err := ParseTrendRange("1y")
	if err != nil || got != TrendOneYear {
		t.Fatalf("ParseTrendRange(1y) = %q, %v", got, err)
	}
}

func TestParseQuoteSourceFilter(t *testing.T) {
	got, err := ParseQuoteSourceFilter("")
	if err != nil || got != QuoteSourceFilterAll {
		t.Fatalf("empty filter = %q, %v", got, err)
	}
	got, err = ParseQuoteSourceFilter("manual")
	if err != nil || got != QuoteSourceFilterManual {
		t.Fatalf("manual filter = %q, %v", got, err)
	}
	if _, err := ParseQuoteSourceFilter("yahoo"); err == nil {
		t.Fatal("ParseQuoteSourceFilter accepted yahoo")
	}
}

func TestParseActivityReasonAndTradeSide(t *testing.T) {
	if _, err := ParseActivityReason("not-a-reason"); err == nil {
		t.Fatal("ParseActivityReason accepted an unknown reason")
	}
	reason, err := ParseActivityReason("income")
	if err != nil || reason != ReasonIncome {
		t.Fatalf("ParseActivityReason(income) = %q, %v", reason, err)
	}
	if got, err := ParseActivityReason(""); err != nil || got != "" {
		t.Fatalf("empty reason should pass through, got %q %v", got, err)
	}
	if _, err := ParseTradeSide("hold"); err == nil {
		t.Fatal("ParseTradeSide accepted hold")
	}
	side, err := ParseTradeSide("sell")
	if err != nil || side != TradeSell {
		t.Fatalf("ParseTradeSide(sell) = %q, %v", side, err)
	}
}

func TestParseActivityKind(t *testing.T) {
	if _, err := ParseActivityKind("deposit"); err == nil {
		t.Fatal("ParseActivityKind accepted deposit")
	}
	got, err := ParseActivityKind("cash_in")
	if err != nil || got != ActivityCashIn {
		t.Fatalf("ParseActivityKind(cash_in) = %q, %v", got, err)
	}
	dividend, err := ParseActivityKind("cash_dividend")
	if err != nil || dividend != ActivityCashDividend {
		t.Fatalf("ParseActivityKind(cash_dividend) = %q, %v", dividend, err)
	}
}

func TestParseOwnershipScope(t *testing.T) {
	if got, err := ParseOwnershipScope(""); err != nil || got != OwnershipAny {
		t.Fatalf("empty scope = %q, %v", got, err)
	}
	if _, err := ParseOwnershipScope("joint"); err == nil {
		t.Fatal("ParseOwnershipScope accepted joint")
	}
	got, err := ParseOwnershipScope("shared")
	if err != nil || got != OwnershipShared {
		t.Fatalf("ParseOwnershipScope(shared) = %q, %v", got, err)
	}
}
