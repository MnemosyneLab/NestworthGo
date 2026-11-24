package history

import (
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestTransferFXAmountMatchesWailsHalfEvenMinorUnits(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		base, quote, from, to domain.CurrencyCode
		rate, amount, want    string
	}{
		{"forward half even", "USD", "EUR", "USD", "EUR", "0.8245", "10", "8.24"},
		{"forward half odd", "USD", "EUR", "USD", "EUR", "0.8255", "10", "8.26"},
		{"reverse half even", "USD", "EUR", "EUR", "USD", "8", "9", "1.12"},
		{"reverse half odd", "USD", "EUR", "EUR", "USD", "8", "9.08", "1.14"},
		{"reverse repeating", "EUR", "USD", "USD", "EUR", "1.2", "125.625", "104.69"},
		{"zero-place destination", "USD", "JPY", "USD", "JPY", "150.5", "1", "150"},
		{"sub-unit would produce zero", "USD", "JPY", "USD", "JPY", "1", "0.5", ""},
		{"unrelated quote", "USD", "JPY", "USD", "EUR", "150", "1", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rate, err := domain.ParseFxRate(tc.rate)
			if err != nil {
				t.Fatal(err)
			}
			amount, err := domain.ParseMoney(tc.amount, tc.from)
			if err != nil {
				t.Fatal(err)
			}
			got, err := TransferFXAmount(amount, tc.to, domain.FXQuote{BaseCurrency: tc.base, QuoteCurrency: tc.quote, Rate: rate})
			if tc.want == "" {
				if err == nil {
					t.Fatalf("invalid suggestion produced %+v", got)
				}
			} else if err != nil || got.CanonicalAmount() != tc.want || got.Currency() != tc.to {
				t.Fatalf("got %s %s, want %s %s: %v", got.CanonicalAmount(), got.Currency(), tc.want, tc.to, err)
			}
		})
	}
}
