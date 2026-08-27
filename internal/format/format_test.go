package format

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestMoneyUsesExactDecimalRoundingAndSeparators(t *testing.T) {
	pref := settings.Default()
	if got := Money("1234567.895", "CNY", pref); got != "¥ 1,234,567.90" {
		t.Fatalf("Money() = %q, want ¥ 1,234,567.90", got)
	}
	if got := Money("1234567.885", "CNY", pref); got != "¥ 1,234,567.88" {
		t.Fatalf("Money() = %q, want ¥ 1,234,567.88", got)
	}

	pref.DecimalSeparator = settings.DecimalComma
	pref.GroupingSeparator = settings.GroupingDot
	pref.Currency = "EUR"
	if got := Money("-1234567.895", pref.Currency, pref); got != "-€ 1.234.567,90" {
		t.Fatalf("Money() localized = %q, want -€ 1.234.567,90", got)
	}
}

func TestMoneyRoundsHalfEvenAtZeroPlaces(t *testing.T) {
	pref := settings.Default()
	if got := Money("8.5", "JPY", pref); got != "¥ 8" {
		t.Fatalf("Money(8.5 JPY) = %q, want ¥ 8", got)
	}
	if got := Money("9.5", "JPY", pref); got != "¥ 10" {
		t.Fatalf("Money(9.5 JPY) = %q, want ¥ 10", got)
	}
}

func TestMoneyUsesCurrencyFractionDigits(t *testing.T) {
	pref := settings.Default()
	if got := Money("1234", "CNY", pref); got != "¥ 1,234.00" {
		t.Fatalf("Money(CNY) = %q, want ¥ 1,234.00", got)
	}
	if got := Money("1234.5", "SGD", pref); got != "S$ 1,234.50" {
		t.Fatalf("Money(SGD) = %q, want S$ 1,234.50", got)
	}
	if got := Money("1234.56", "JPY", pref); got != "¥ 1,235" {
		t.Fatalf("Money(JPY) = %q, want ¥ 1,235", got)
	}
	if got := Money("1234.00", "KRW", pref); got != "₩ 1,234" {
		t.Fatalf("Money(KRW) = %q, want ₩ 1,234", got)
	}
}

func TestCurrencySymbolIncludesKRWAndCHF(t *testing.T) {
	if got := CurrencySymbol("KRW"); got != "₩" {
		t.Fatalf("CurrencySymbol(KRW) = %q, want ₩", got)
	}
	if got := CurrencySymbol("CHF"); got != "CHF" {
		t.Fatalf("CurrencySymbol(CHF) = %q, want CHF", got)
	}
}

func TestShareBPSFormatsWithoutBinaryFloat(t *testing.T) {
	for value, want := range map[int]string{0: "0.00%", 1250: "12.50%", 10000: "100.00%", -25: "-0.25%"} {
		if got := ShareBPS(value); got != want {
			t.Fatalf("ShareBPS(%d) = %q, want %q", value, got, want)
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
