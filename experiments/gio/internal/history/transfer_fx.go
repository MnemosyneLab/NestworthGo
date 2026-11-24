package history

import (
	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// TransferFXAmount follows the Wails draft calculator's quote orientation and
// minor-unit half-even rounding. It supplies a principal amount for review.
func TransferFXAmount(amount domain.Money, target domain.CurrencyCode, quote domain.FXQuote) (domain.Money, error) {
	source, rate := amount.Currency(), quote.Rate.Decimal()
	if source == target || !amount.Amount().IsPositive() || !rate.IsPositive() {
		return domain.Money{}, &domain.Error{Code: domain.ErrValidation}
	}

	places := int32(domain.CurrencyFractionDigits(target))
	var result decimal.Decimal
	switch {
	case quote.BaseCurrency == source && quote.QuoteCurrency == target:
		result = amount.Amount().Mul(rate).RoundBank(places)
	case quote.BaseCurrency == target && quote.QuoteCurrency == source:
		// Divide directly at the target scale. Rounding a reciprocal or
		// an intermediate decimal can change which side of a tie wins.
		units, remainder := amount.Amount().Shift(places).QuoRem(rate, 0)
		switch tie := remainder.Add(remainder).Cmp(rate); {
		case tie > 0, tie == 0 && units.Coefficient().Bit(0) == 1:
			units = units.Add(decimal.NewFromInt(1))
		}
		result = units.Shift(-places)
	default:
		return domain.Money{}, &domain.Error{Code: domain.ErrValidation}
	}
	if !result.IsPositive() {
		return domain.Money{}, &domain.Error{Code: domain.ErrNoChange}
	}
	return domain.NewMoney(result, target)
}
