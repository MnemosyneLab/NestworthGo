package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestBaseAmountExactPreservesFXPrecisionAndBounds(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"999999999999.9999"} {
		got, err := ParseBaseAmountExact(value)
		if err != nil || got != value {
			t.Fatalf("ParseBaseAmountExact(%q) = %q, %v", value, got, err)
		}
	}
	for _, value := range []string{"1e2", "999999999999.99990000000000001"} {
		if _, err := ParseBaseAmountExact(value); err == nil {
			t.Fatalf("accepted invalid base amount %q", value)
		}
	}
	if got, err := ParseBaseAmountExact("1.230000000000000000"); err != nil || got != "1.23" {
		t.Fatalf("canonical = %q, %v", got, err)
	}
	if _, err := ParseNativeAmount("89.01098909904924535"); err == nil {
		t.Fatal("native precision was widened")
	}
	// Both multiplication of a 16-place native amount and inverse conversion
	// may exceed 16 places, and must survive the snapshot storage boundary.
	native := decimal.RequireFromString("1.1234567890123456")
	rate, err := ParseFxRate("1.123456789012")
	if err != nil {
		t.Fatal(err)
	}
	for _, inverse := range []bool{false, true} {
		var converted decimal.Decimal
		if inverse {
			converted, err = DivideByFxRate(native, rate)
		} else {
			converted, err = MultiplyByFxRate(native, rate)
		}
		if err != nil {
			t.Fatal(err)
		}
		if -converted.Exponent() <= 16 {
			t.Fatal("fixture does not exercise extended precision")
		}
		got, err := ParseBaseAmountExact(converted.String())
		if err != nil || got != converted.String() {
			t.Fatalf("converted amount changed: %q %v", got, err)
		}
	}
}

func TestSnapshotBaseAmountSupportsMaximumSingleFXScale(t *testing.T) {
	t.Parallel()
	native := decimal.RequireFromString("1.1234567890123456")
	rate, err := ParseFxRate("99999999.999999999999")
	if err != nil {
		t.Fatal(err)
	}
	converted, err := DivideByFxRate(native, rate)
	if err != nil {
		t.Fatal(err)
	}
	if -converted.Exponent() != maxSnapshotBaseAmountFractionDigits {
		t.Fatalf("fixture scale = %d", -converted.Exponent())
	}
	got, err := ParseBaseAmountExact(converted.String())
	if err != nil || got != converted.String() {
		t.Fatalf("maximum inverse precision = %q, %v", got, err)
	}
}
