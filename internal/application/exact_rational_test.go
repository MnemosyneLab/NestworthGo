package application

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestRationalMoneyViewChecksUnsignedSignBeforeRounding(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, amount, wantView string
		wantError              bool
	}{
		{name: "zero", amount: "0", wantView: "0"},
		{name: "maximum", amount: "999999999999.9999", wantView: "999999999999.9999"},
		{name: "tiny negative rounded to zero", amount: "-0.00001", wantError: true},
		{name: "negative", amount: "-1", wantError: true},
		{name: "above maximum", amount: "1000000000000", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			view, err := rationalMoneyView(rationalFromDecimal(decimal.RequireFromString(test.amount)), "CNY")
			if test.wantError {
				var problem *domain.Error
				if !errors.As(err, &problem) || problem.Code != domain.ErrDecimalOverflow {
					t.Fatalf("rationalMoneyView(%s) = %+v, %v; want ErrDecimalOverflow", test.amount, view, err)
				}
				if test.amount == "-0.00001" {
					if _, moneyErr := domain.NewMoney(decimal.RequireFromString(test.amount), "CNY"); moneyErr == nil {
						t.Fatal("domain.NewMoney unexpectedly accepted a negative fraction that rounds to zero")
					}
				}
				return
			}
			if err != nil || view.Amount != test.wantView {
				t.Fatalf("rationalMoneyView(%s) = %+v, %v", test.amount, view, err)
			}
		})
	}
}
