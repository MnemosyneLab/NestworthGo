package format

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestQuotePriceAndRateRetainPrecision(t *testing.T) {
	pref := settings.Default()
	pref.DecimalSeparator, pref.GroupingSeparator = settings.DecimalComma, settings.GroupingDot
	for _, item := range []struct{ amount, currency, want string }{
		{"1234.56789123", "USD", "US$1.234,56789123"},
		{"750", "JPY", "JP¥750,00"},
		{"0.00000001", "CNY", "CN¥0,00000001"},
		{"0", "USD", "US$0,00"},
		{"-1234.5678", "JPY", "-JP¥1.234,5678"},
		{"—", "USD", "—"},
	} {
		if got := QuotePrice(item.amount, item.currency, pref); got != item.want {
			t.Errorf("QuotePrice(%q, %q) = %q, want %q", item.amount, item.currency, got, item.want)
		}
	}
	if got := ExactDecimal("1321.005600", pref); got != "1.321,005600" {
		t.Errorf("exact rate = %q, want 1.321,005600", got)
	}
}

func TestMoneyUsesExactDecimalRoundingAndSeparators(t *testing.T) {
	pref := settings.Default()
	if got := Money("1234567.895", "CNY", pref); got != "CN¥1,234,567.90" {
		t.Fatalf("Money() = %q, want CN¥1,234,567.90", got)
	}
	if got := Money("1234567.885", "CNY", pref); got != "CN¥1,234,567.88" {
		t.Fatalf("Money() = %q, want CN¥1,234,567.88", got)
	}

	pref.DecimalSeparator = settings.DecimalComma
	pref.GroupingSeparator = settings.GroupingDot
	pref.Currency = "EUR"
	if got := Money("-1234567.895", pref.Currency, pref); got != "-€1.234.567,90" {
		t.Fatalf("Money() localized = %q, want -€1.234.567,90", got)
	}
}

func TestMoneyRoundsHalfEvenAtZeroPlaces(t *testing.T) {
	pref := settings.Default()
	if got := Money("8.5", "JPY", pref); got != "JP¥8" {
		t.Fatalf("Money(8.5 JPY) = %q, want JP¥8", got)
	}
	if got := Money("9.5", "JPY", pref); got != "JP¥10" {
		t.Fatalf("Money(9.5 JPY) = %q, want JP¥10", got)
	}
}

func TestMoneyUsesCurrencyFractionDigits(t *testing.T) {
	pref := settings.Default()
	if got := Money("1234", "CNY", pref); got != "CN¥1,234.00" {
		t.Fatalf("Money(CNY) = %q, want CN¥1,234.00", got)
	}
	if got := Money("1234.5", "SGD", pref); got != "S$1,234.50" {
		t.Fatalf("Money(SGD) = %q, want S$1,234.50", got)
	}
	if got := Money("1234.56", "JPY", pref); got != "JP¥1,235" {
		t.Fatalf("Money(JPY) = %q, want JP¥1,235", got)
	}
	if got := Money("1234.00", "KRW", pref); got != "₩1,234" {
		t.Fatalf("Money(KRW) = %q, want ₩1,234", got)
	}
}

func TestCurrencySymbolIncludesKRWAndCHF(t *testing.T) {
	if got := CurrencySymbol("KRW"); got != "₩" {
		t.Fatalf("CurrencySymbol(KRW) = %q, want ₩", got)
	}
}

func TestShareBPSFormatsWithoutBinaryFloat(t *testing.T) {
	for value, want := range map[int]string{0: "0.00%", 1250: "12.50%", 10000: "100.00%", -25: "-0.25%"} {
		if got := ShareBPS(value); got != want {
			t.Fatalf("ShareBPS(%d) = %q, want %q", value, got, want)
		}
	}
}

func TestShareBPSOneDecimalMatchesWailsHalfEven(t *testing.T) {
	for value, want := range map[int]string{0: "0.0%", 15: "0.2%", 25: "0.2%", 35: "0.4%", 7408: "74.1%", 2592: "25.9%", 10000: "100.0%", -15: "-0.2%"} {
		if got := ShareBPSOneDecimal(value); got != want {
			t.Fatalf("ShareBPSOneDecimal(%d) = %q, want %q", value, got, want)
		}
	}
}

func TestDateTimeConvertsTimezoneAndUsesSelectedLayouts(t *testing.T) {
	pref := settings.Default()
	pref.Timezone = "Asia/Shanghai"
	value := time.Date(2026, time.August, 21, 14, 35, 0, 0, time.UTC)
	got, err := DateTime(value, pref, settings.LanguageEnglish)
	if err != nil {
		t.Fatalf("DateTime() error = %v", err)
	}
	if got != "2026-08-21 22:35" {
		t.Fatalf("DateTime() = %q, want 2026-08-21 22:35", got)
	}

	pref.DateFormat = settings.DateFormatLocalized
	pref.TimeFormat = settings.TimeFormat12
	got, err = DateTime(value, pref, settings.LanguageZhTW)
	if err != nil {
		t.Fatalf("DateTime() localized error = %v", err)
	}
	if got != "2026年8月21日 10:35 PM" {
		t.Fatalf("DateTime() localized = %q, want 2026年8月21日 10:35 PM", got)
	}
}

func TestValidateTimezone(t *testing.T) {
	if err := ValidateTimezone(settings.TimezoneSystem); err != nil {
		t.Fatalf("ValidateTimezone(system) error = %v", err)
	}
	if err := ValidateTimezone("Not/AZone"); err == nil {
		t.Fatal("ValidateTimezone(invalid) error = nil")
	}
}

func TestMoneyPrefixesMatchTheWailsApp(t *testing.T) {
	pref := settings.Default()
	for _, test := range []struct{ currency, want string }{
		{"CNY", "-CN¥1.50"}, {"USD", "-US$1.50"}, {"HKD", "-HK$1.50"}, {"CHF", "-CHF 1.50"}, {"XXX", "-XXX 1.50"},
	} {
		if got := Money("-1.5", test.currency, pref); got != test.want {
			t.Errorf("Money(-1.5 %s) = %q, want %q", test.currency, got, test.want)
		}
	}
}
