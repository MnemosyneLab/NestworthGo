package application

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestResidualToleranceExactConstantMatchesPreviousCalculation(t *testing.T) {
	for _, currency := range []domain.CurrencyCode{"USD", "JPY", "KRW", "XYZ"} {
		for _, amount := range []string{"0", "100", "-100", "19999999.9999", "20000000", "20000000.0001", "2000000000", "-999999999999.9999"} {
			beginning := decimal.RequireFromString(amount)
			floor := decimal.New(1, int32(-domain.CurrencyFractionDigits(currency))).Mul(decimal.NewFromInt(2))
			relative := beginning.Abs().Mul(decimal.NewFromFloat(1e-9))
			want := floor
			if relative.GreaterThan(floor) {
				want = relative
			}
			got := residualTolerance(currency, beginning)
			if !got.Equal(want) || got.Exponent() != want.Exponent() {
				t.Fatalf("%s %s: got %s (exp %d), want %s (exp %d)", currency, amount, got, got.Exponent(), want, want.Exponent())
			}
		}
	}
}
