package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestNewSignedMoneyPreservesBankRounding(t *testing.T) {
	// Include exact four-place values, positive exponents, trailing zeros,
	// both signs of half-even ties, and amounts next to the range boundary.
	values := []string{"0", "0.0000", "0.000000", "1e3", "100", "1.2", "1.2340", "1.2344", "1.23445", "1.23455", "1.23445001", "1.23444999", "0.00005", "0.00015", "999999999999.9999", "999999999999.99989"}
	for _, text := range values {
		for _, sign := range []int64{1, -1} {
			value := decimal.RequireFromString(text).Mul(decimal.NewFromInt(sign))
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
	for exponent := int32(-16); exponent <= 8; exponent++ {
		for coefficient := int64(-101); coefficient <= 101; coefficient++ {
			value := decimal.New(coefficient, exponent)
			got, err := NewSignedMoney(value, "JPY")
			want := value.RoundBank(4)
			if err != nil || !got.Amount().Equal(want) || got.Amount().Exponent() != want.Exponent() {
				t.Fatalf("coefficient=%d exponent=%d: got %+v, %v; want %s (exp %d)", coefficient, exponent, got, err, want, want.Exponent())
			}
		}
	}
}

func TestNewSignedMoneyStillChecksRangeBeforeRounding(t *testing.T) {
	for _, text := range []string{"999999999999.99991", "-999999999999.99991", "1000000000000", "-1000000000000"} {
		_, err := NewSignedMoney(decimal.RequireFromString(text), "USD")
		if domainErr, ok := err.(*Error); !ok || domainErr.Code != ErrDecimalOverflow {
			t.Fatalf("%s: got %v, want decimal overflow", text, err)
		}
	}
	if _, err := NewSignedMoney(decimal.Zero, "US"); err == nil {
		t.Fatal("invalid currency accepted")
	}
}
