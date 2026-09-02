package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestPortfolioDecimalsRejectNonCanonicalAndHonorBounds(t *testing.T) {
	for _, input := range []string{"", "01", "1.", "1e2", "-1", " 1", "1.000000000"} {
		if _, err := ParseQuantity(input); err == nil {
			t.Fatalf("ParseQuantity(%q) succeeded", input)
		}
	}
	if value, err := ParseQuantity("1.23000000"); err != nil || value.Canonical() != "1.23" {
		t.Fatalf("quantity canonical value = %q, err = %v", value.Canonical(), err)
	}
	if _, err := ParseQuantity("1000000000000000000"); err == nil {
		t.Fatal("quantity over the integer-digit maximum succeeded")
	}
	if _, err := ParseUnitPrice("1.000000001"); err == nil {
		t.Fatal("unit price over the scale maximum succeeded")
	}
	if _, err := ParseFxRate("0"); err == nil {
		t.Fatal("zero FX rate succeeded")
	}
	if rate, err := ParseFXRate("1.230000000000"); err != nil || rate.Canonical() != "1.23" {
		t.Fatalf("FX rate canonical value = %q, err = %v", rate.Canonical(), err)
	}
}

func TestPortfolioDecimalArithmeticIsCheckedAndUnrounded(t *testing.T) {
	quantity, err := ParseQuantity("3")
	if err != nil {
		t.Fatal(err)
	}
	price, err := ParseUnitPrice("700.12345678")
	if err != nil {
		t.Fatal(err)
	}
	value, err := quantity.Multiply(price)
	if err != nil {
		t.Fatal(err)
	}
	if value.String() != "2100.37037034" {
		t.Fatalf("unrounded holding value = %s", value)
	}
	rate, err := ParseFxRate("6.9")
	if err != nil {
		t.Fatal(err)
	}
	converted, err := DivideByFxRate(decimal.RequireFromString("14490"), rate)
	if err != nil {
		t.Fatal(err)
	}
	if !converted.Equal(decimal.RequireFromString("2100")) {
		t.Fatalf("division result = %s", converted)
	}
	if _, err := quantity.Multiply(UnitPrice{value: decimal.RequireFromString("999999999999.99999999")}); err == nil {
		t.Fatal("overflowing multiplication succeeded")
	}
}

func TestNativeAmountAllowsFullQuantityAndUnitPricePrecision(t *testing.T) {
	value, err := ParseNativeAmount("11568.99735")
	if err != nil || value != "11568.99735" {
		t.Fatalf("native amount = %q, err = %v", value, err)
	}
	for _, input := range []string{"", "01", "1.", "1e2", "-1", "1.00000000000000000"} {
		if _, err := ParseNativeAmount(input); err == nil {
			t.Fatalf("ParseNativeAmount(%q) succeeded", input)
		}
	}
}

func TestMoneyBoundaryUsesBankersRounding(t *testing.T) {
	even, err := NewMoney(decimal.RequireFromString("1.23445"), CurrencyCode("CNY"))
	if err != nil {
		t.Fatal(err)
	}
	if even.CanonicalAmount() != "1.2344" {
		t.Fatalf("even midpoint rounded to %s", even.CanonicalAmount())
	}
	odd, err := NewMoney(decimal.RequireFromString("1.23455"), CurrencyCode("CNY"))
	if err != nil {
		t.Fatal(err)
	}
	if odd.CanonicalAmount() != "1.2346" {
		t.Fatalf("odd midpoint rounded to %s", odd.CanonicalAmount())
	}
}

func TestUnitPriceFromExactRoundsOnlyWhenScaleExceedsEight(t *testing.T) {
	exact, err := UnitPriceFromExact(decimal.RequireFromString("1.23456789"))
	if err != nil || exact.Canonical() != "1.23456789" {
		t.Fatalf("exact eight-decimal price = %q err=%v", exact.Canonical(), err)
	}
	// 1400/15 = 93.3 repeating; ninth fractional digit is 3, so banker's
	// rounding at eight places is 93.33333333.
	repeating, err := UnitPriceFromExact(decimal.RequireFromString("1400").Div(decimal.RequireFromString("15")))
	if err != nil || repeating.Canonical() != "93.33333333" {
		t.Fatalf("repeating average = %q err=%v", repeating.Canonical(), err)
	}
	// DivisionPrecision pads exact quotients with trailing zeros; those extra
	// scale digits must still round to the supported unit-price scale.
	exactQuotient, err := UnitPriceFromExact(decimal.RequireFromString("840").Div(decimal.RequireFromString("10")))
	if err != nil || exactQuotient.Canonical() != "84" {
		t.Fatalf("exact quotient = %q err=%v", exactQuotient.Canonical(), err)
	}
}
