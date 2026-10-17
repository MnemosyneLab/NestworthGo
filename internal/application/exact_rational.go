package application

import (
	"math/big"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

const exactMoneyScale = 4

// rationalFromDecimal preserves a decimal's exact coefficient and exponent.
// It is used only in acquisition-FX weighting, where a rounded average rate can
// later be multiplied by a much larger position amount.
func rationalFromDecimal(value decimal.Decimal) *big.Rat {
	numerator := new(big.Int).Set(value.Coefficient())
	exponent := value.Exponent()
	if exponent >= 0 {
		numerator.Mul(numerator, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil))
		return new(big.Rat).SetInt(numerator)
	}
	denominator := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-exponent)), nil)
	return new(big.Rat).SetFrac(numerator, denominator)
}

// decimalAtMoneyBoundary rounds an exact ratio directly to the supported
// four-place Money scale using half-even. No recurring weighted rate is first
// truncated to a decimal, so later multiplication or aggregation cannot
// amplify a rounded rate.
func decimalAtMoneyBoundary(value *big.Rat) decimal.Decimal {
	if value == nil || value.Sign() == 0 {
		return decimal.Zero
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(exactMoneyScale), nil)
	scaledNumerator := new(big.Int).Mul(value.Num(), scale)
	quotient := new(big.Int)
	remainder := new(big.Int)
	quotient.QuoRem(scaledNumerator, value.Denom(), remainder)
	comparison := new(big.Int).Lsh(new(big.Int).Abs(remainder), 1).Cmp(value.Denom())
	if comparison > 0 || (comparison == 0 && quotient.Bit(0) == 1) {
		if value.Sign() < 0 {
			quotient.Sub(quotient, big.NewInt(1))
		} else {
			quotient.Add(quotient, big.NewInt(1))
		}
	}
	return decimal.NewFromBigInt(quotient, -exactMoneyScale)
}

func rationalMoneyView(value *big.Rat, currency domain.CurrencyCode) (domain.MoneyView, error) {
	if err := domain.CheckExactMoneyRange(value); err != nil {
		return domain.MoneyView{}, err
	}
	return moneyView(decimalAtMoneyBoundary(value), currency)
}

func rationalSignedMoneyView(value *big.Rat, currency domain.CurrencyCode) (domain.SignedMoneyView, error) {
	if err := domain.CheckExactMoneyRange(value); err != nil {
		return domain.SignedMoneyView{}, err
	}
	return signedMoneyView(decimalAtMoneyBoundary(value), currency)
}
