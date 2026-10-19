package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestNewSignedMoneyPreservesBankRounding(t *testing.T) {
	// Include exact four-place values, positive exponents, trailing zeros,
	// both signs of half-even ties, and amounts next to the range boundary.
	// Exponent >= -4 uses exact rescale; a longer scale uses banker's rounding.
	for _, text := range []string{"0", "1.2", "-1.23445", "1.23455", "999999999999.9999"} {
		value := decimal.RequireFromString(text)
		got, err := NewSignedMoney(value, " usd ")
		if err != nil {
			t.Fatalf("NewSignedMoney(%s): %v", value, err)
		}
		want := value.RoundBank(4)
		if !got.Amount().Equal(want) || got.Amount().Exponent() != want.Exponent() || got.Currency() != "USD" {
			t.Fatalf("%s: got %s (exp %d, %s), want %s (exp %d, USD)", value, got.Amount(), got.Amount().Exponent(), got.Currency(), want, want.Exponent())
		}
	}
}

func TestNewSignedMoneyStillChecksRangeBeforeRounding(t *testing.T) {
	for _, text := range []string{"1000000000000"} {
		_, err := NewSignedMoney(decimal.RequireFromString(text), "USD")
		if domainErr, ok := err.(*Error); !ok || domainErr.Code != ErrDecimalOverflow {
			t.Fatalf("%s: got %v, want decimal overflow", text, err)
		}
	}
	if _, err := NewSignedMoney(decimal.Zero, "US"); err == nil {
		t.Fatal("invalid currency accepted")
	}
}
