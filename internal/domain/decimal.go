package domain

import (
	"math"
	"math/big"
	"regexp"

	"github.com/shopspring/decimal"
)

const (
	moneyAmountFractionDigits     = 4
	minimumFXDivisionPrecision    = 16 // Preserve the existing decimal intermediate precision for later aggregation.
	maxNativeAmountFractionDigits = 16
	maxFXRateCoefficientDigits    = 8 + 12 // FX rates allow eight integer and twelve fractional digits.
	// One bridge conversion can produce at most this many decimal places from a
	// 16-place native amount and a maximum-size rate; a second conversion can
	// consume that result before the final Money projection.
	maxFXConversionInputFractionDigits = maxNativeAmountFractionDigits + maxFXRateCoefficientDigits + moneyAmountFractionDigits
)

var (
	quantitySyntax     = regexp.MustCompile(`^(0|[1-9][0-9]{0,17})(\.[0-9]{1,8})?$`)
	unitPriceSyntax    = regexp.MustCompile(`^(0|[1-9][0-9]{0,11})(\.[0-9]{1,8})?$`)
	fxRateSyntax       = regexp.MustCompile(`^(0|[1-9][0-9]{0,7})(\.[0-9]{1,12})?$`)
	nativeAmountSyntax = regexp.MustCompile(`^(0|[1-9][0-9]{0,11})(\.[0-9]{1,16})?$`)
	signedMoneySyntax  = regexp.MustCompile(`^-?(0|[1-9][0-9]{0,11})(\.[0-9]{1,4})?$`)

	maxQuantity  = decimal.RequireFromString("999999999999999999.99999999")
	maxUnitPrice = decimal.RequireFromString("999999999999.99999999")
	maxFxRate    = decimal.RequireFromString("99999999.999999999999")
	maxMoney     = decimal.RequireFromString("999999999999.9999")
)

// Quantity is a non-negative exact position quantity with up to 18 integer
// and 8 fractional digits. It is deliberately string-parsed at boundaries so
// a binary float cannot enter the financial model.
type Quantity struct{ value decimal.Decimal }

func ParseQuantity(value string) (Quantity, error) {
	if !quantitySyntax.MatchString(value) {
		return Quantity{}, validation("quantity", "must be a canonical non-negative decimal with up to eight fractional digits")
	}
	parsed, err := decimal.NewFromString(value)
	if err != nil {
		return Quantity{}, validation("quantity", "is not a valid decimal")
	}
	return NewQuantity(parsed)
}

func NewQuantity(value decimal.Decimal) (Quantity, error) {
	if err := validateBoundedDecimal("quantity", value, maxQuantity, 18, 8, false); err != nil {
		return Quantity{}, err
	}
	return Quantity{value: value}, nil
}

func (q Quantity) Decimal() decimal.Decimal { return q.value }
func (q Quantity) Amount() decimal.Decimal  { return q.value }
func (q Quantity) Canonical() string        { return canonicalDecimal(q.value) }
func (q Quantity) CanonicalString() string  { return q.Canonical() }
func (q Quantity) String() string           { return q.Canonical() }
func (q Quantity) IsZero() bool             { return q.value.IsZero() }

// UnitPrice is a non-negative exact price with up to 12 integer and 8
// fractional digits. Zero is valid and is different from an unavailable
// observation.
type UnitPrice struct{ value decimal.Decimal }

func ParseUnitPrice(value string) (UnitPrice, error) {
	if !unitPriceSyntax.MatchString(value) {
		return UnitPrice{}, validation("unitPrice", "must be a canonical non-negative decimal with up to eight fractional digits")
	}
	parsed, err := decimal.NewFromString(value)
	if err != nil {
		return UnitPrice{}, validation("unitPrice", "is not a valid decimal")
	}
	return NewUnitPrice(parsed)
}

func NewUnitPrice(value decimal.Decimal) (UnitPrice, error) {
	if err := validateBoundedDecimal("unitPrice", value, maxUnitPrice, 12, 8, false); err != nil {
		return UnitPrice{}, err
	}
	return UnitPrice{value: value}, nil
}

func (p UnitPrice) Decimal() decimal.Decimal { return p.value }
func (p UnitPrice) Amount() decimal.Decimal  { return p.value }
func (p UnitPrice) Canonical() string        { return canonicalDecimal(p.value) }
func (p UnitPrice) CanonicalString() string  { return p.Canonical() }
func (p UnitPrice) String() string           { return p.Canonical() }
func (p UnitPrice) IsZero() bool             { return p.value.IsZero() }

// ParseNativeAmount validates the unrounded native value stored in a daily
// valuation snapshot item. A holding's quantity and unit price each allow
// eight fractional digits, so their product may require up to sixteen. The
// value is still bounded by the existing intermediate monetary range, but it
// must not be sent through ParseMoney because ParseMoney intentionally allows
// only four fractional digits.
func ParseNativeAmount(value string) (string, error) {
	if !nativeAmountSyntax.MatchString(value) {
		return "", validation("nativeAmount", "must be a canonical non-negative decimal with up to sixteen fractional digits")
	}
	parsed, err := decimal.NewFromString(value)
	if err != nil {
		return "", validation("nativeAmount", "is not a valid decimal")
	}
	if err := validateBoundedDecimal("nativeAmount", parsed, maxMoney, 12, 16, false); err != nil {
		return "", err
	}
	return canonicalDecimal(parsed), nil
}

// FxRate is a strictly positive exact rate with up to 8 integer and 12
// fractional digits. Its orientation is documented by the FXQuote contract:
// one base currency equals rate quote currency.
type FxRate struct{ value decimal.Decimal }

func ParseFxRate(value string) (FxRate, error) {
	if !fxRateSyntax.MatchString(value) {
		return FxRate{}, validation("fxRate", "must be a canonical positive decimal with up to twelve fractional digits")
	}
	parsed, err := decimal.NewFromString(value)
	if err != nil {
		return FxRate{}, validation("fxRate", "is not a valid decimal")
	}
	return NewFxRate(parsed)
}

// ParseFXRate is kept as an acronym-friendly alias for callers using the
// contract's FX terminology.
func ParseFXRate(value string) (FxRate, error) { return ParseFxRate(value) }

func NewFxRate(value decimal.Decimal) (FxRate, error) {
	if err := validateBoundedDecimal("fxRate", value, maxFxRate, 8, 12, true); err != nil {
		return FxRate{}, err
	}
	return FxRate{value: value}, nil
}

func (r FxRate) Decimal() decimal.Decimal { return r.value }
func (r FxRate) Amount() decimal.Decimal  { return r.value }
func (r FxRate) Canonical() string        { return canonicalDecimal(r.value) }
func (r FxRate) CanonicalString() string  { return r.Canonical() }
func (r FxRate) String() string           { return r.Canonical() }

func (q Quantity) Multiply(price UnitPrice) (decimal.Decimal, error) {
	return MultiplyQuantityAndUnitPrice(q, price)
}

func (p UnitPrice) Multiply(quantity Quantity) (decimal.Decimal, error) {
	return MultiplyQuantityAndUnitPrice(quantity, p)
}

// UnitPriceFromExact constructs a UnitPrice at the supported eight-decimal
// scale. Banker's rounding is applied once only when division or another
// exact calculation produces more than eight fractional digits.
func UnitPriceFromExact(value decimal.Decimal) (UnitPrice, error) {
	scale, _ := decimalShape(value)
	if scale > 8 {
		value = value.RoundBank(8)
	}
	return NewUnitPrice(value)
}

// MultiplyQuantityAndUnitPrice preserves precision until the Money boundary.
func MultiplyQuantityAndUnitPrice(quantity Quantity, price UnitPrice) (decimal.Decimal, error) {
	return checkedIntermediate(quantity.value.Mul(price.value))
}

func MultiplyByFxRate(value decimal.Decimal, rate FxRate) (decimal.Decimal, error) {
	return checkedIntermediate(value.Mul(rate.value))
}

func DivideByFxRate(value decimal.Decimal, rate FxRate) (decimal.Decimal, error) {
	if rate.value.IsZero() {
		return decimal.Zero, validation("fxRate", "must be greater than zero")
	}
	converted, err := divideFXAmountByRate(value, rate.value)
	if err != nil {
		return decimal.Zero, err
	}
	return checkedIntermediate(converted)
}

// ConvertFXAmount converts one signed whole amount using the stored quote
// orientation. It keeps the quote rate un-inverted for division paths so the
// amount is rounded only at its eventual Money projection boundary.
func ConvertFXAmount(value decimal.Decimal, from, to CurrencyCode, quote FXQuote) (decimal.Decimal, error) {
	if value.Abs().GreaterThan(maxMoney) {
		return decimal.Zero, &Error{Code: ErrDecimalOverflow, Field: "amount", Message: "calculation is outside the supported range"}
	}
	if from == to {
		return value, nil
	}
	multiplier, divisor, err := fxConversionFactors(from, to, quote)
	if err != nil {
		return decimal.Zero, err
	}
	converted := value.Mul(multiplier)
	if !divisor.Equal(decimal.NewFromInt(1)) {
		converted, err = divideFXAmountByRate(converted, divisor)
		if err != nil {
			return decimal.Zero, err
		}
	}
	if converted.Abs().GreaterThan(maxMoney) {
		return decimal.Zero, &Error{Code: ErrDecimalOverflow, Field: "amount", Message: "calculation is outside the supported range"}
	}
	return converted, nil
}

// ConvertFXAmountExact applies the same oriented quote and per-event magnitude
// guards as ConvertFXAmount, but retains the exact rational amount until its
// caller reaches the final Money boundary.
func ConvertFXAmountExact(value decimal.Decimal, from, to CurrencyCode, quote FXQuote) (*big.Rat, error) {
	if value.Abs().GreaterThan(maxMoney) {
		return nil, &Error{Code: ErrDecimalOverflow, Field: "amount", Message: "calculation is outside the supported range"}
	}
	converted := RationalFromDecimal(value)
	if from == to {
		if err := CheckExactMoneyRange(converted); err != nil {
			return nil, err
		}
		return converted, nil
	}
	multiplier, divisor, err := fxConversionFactors(from, to, quote)
	if err != nil {
		return nil, err
	}
	converted.Mul(converted, RationalFromDecimal(multiplier))
	if !divisor.Equal(decimal.NewFromInt(1)) {
		converted.Quo(converted, RationalFromDecimal(divisor))
	}
	if err := CheckExactMoneyRange(converted); err != nil {
		return nil, &Error{Code: ErrDecimalOverflow, Field: "amount", Message: "calculation is outside the supported range"}
	}
	return converted, nil
}

// CheckExactMoneyRange applies the shared Money magnitude limit to an exact
// rational result before rounding. The denominator is never approximated, so
// a value just outside the supported range cannot round back into it.
func CheckExactMoneyRange(value *big.Rat) error {
	if value == nil {
		return nil
	}
	limit, ok := new(big.Rat).SetString(maxMoney.String())
	if !ok {
		return &Error{Code: ErrIntegrity, Field: "amount", Message: "money range could not be resolved"}
	}
	if new(big.Rat).Abs(value).Cmp(limit) > 0 {
		return &Error{Code: ErrDecimalOverflow, Field: "amount", Message: "amount is outside the supported range"}
	}
	return nil
}

// CheckExactUnsignedMoneyRange applies the non-negative Money contract before
// any rounding can erase a negative fractional amount.
func CheckExactUnsignedMoneyRange(value *big.Rat) error {
	if value != nil && value.Sign() < 0 {
		return &Error{Code: ErrDecimalOverflow, Field: "amount", Message: "amount is outside the supported range"}
	}
	return CheckExactMoneyRange(value)
}

// ExactFXIntermediateDecimal projects an exact FX bridge ratio into the
// existing bounded decimal intermediate precision. Callers must retain the
// rational value through their sums and differences and apply Money rounding
// only at the actual amount boundary.
func ExactFXIntermediateDecimal(value *big.Rat) (decimal.Decimal, error) {
	if value == nil || value.Sign() == 0 {
		return decimal.Zero, nil
	}
	if err := CheckExactMoneyRange(value); err != nil {
		return decimal.Zero, err
	}
	numerator := decimal.NewFromBigInt(value.Num(), 0)
	denominator := decimal.NewFromBigInt(value.Denom(), 0)
	return numerator.DivRound(denominator, maxFXConversionInputFractionDigits), nil
}

// RationalFromDecimal preserves the exact coefficient and exponent of a
// decimal amount for calculations that need to defer rounding.
func RationalFromDecimal(value decimal.Decimal) *big.Rat {
	numerator := new(big.Int).Set(value.Coefficient())
	exponent := value.Exponent()
	if exponent >= 0 {
		numerator.Mul(numerator, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil))
		return new(big.Rat).SetInt(numerator)
	}
	denominator := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-exponent)), nil)
	return new(big.Rat).SetFrac(numerator, denominator)
}

// ConvertFXAmountViaQuotes composes two quote legs before dividing. Keeping
// both exact rates in the same expression avoids rounding the intermediate
// currency amount before the final projection.
func ConvertFXAmountViaQuotes(value decimal.Decimal, from, via, to CurrencyCode, first, second FXQuote) (decimal.Decimal, error) {
	if value.Abs().GreaterThan(maxMoney) {
		return decimal.Zero, &Error{Code: ErrDecimalOverflow, Field: "amount", Message: "calculation is outside the supported range"}
	}
	if from == via || via == to {
		return decimal.Zero, validation("fxRate", "quote currencies do not match the conversion")
	}
	firstMultiplier, firstDivisor, err := fxConversionFactors(from, via, first)
	if err != nil {
		return decimal.Zero, err
	}
	secondMultiplier, secondDivisor, err := fxConversionFactors(via, to, second)
	if err != nil {
		return decimal.Zero, err
	}
	multiplier := firstMultiplier.Mul(secondMultiplier)
	divisor := firstDivisor.Mul(secondDivisor)
	converted := value.Mul(multiplier)
	if !divisor.Equal(decimal.NewFromInt(1)) {
		converted, err = divideFXAmountByRate(converted, divisor)
		if err != nil {
			return decimal.Zero, err
		}
	}
	if converted.Abs().GreaterThan(maxMoney) {
		return decimal.Zero, &Error{Code: ErrDecimalOverflow, Field: "amount", Message: "calculation is outside the supported range"}
	}
	return converted, nil
}

func fxConversionFactors(from, to CurrencyCode, quote FXQuote) (multiplier, divisor decimal.Decimal, err error) {
	one := decimal.NewFromInt(1)
	if quote.Rate.value.IsZero() {
		return decimal.Zero, decimal.Zero, validation("fxRate", "must be greater than zero")
	}
	switch {
	case quote.BaseCurrency == from && quote.QuoteCurrency == to:
		return quote.Rate.value, one, nil
	case quote.BaseCurrency == to && quote.QuoteCurrency == from:
		return one, quote.Rate.value, nil
	default:
		return decimal.Zero, decimal.Zero, validation("fxRate", "quote currencies do not match the conversion")
	}
}

// divideFXAmountByRate keeps reciprocal whole-amount conversion precise
// enough for the eventual four-place Money half-even boundary. If value is
// A*10^-s and rate is C*10^e with d coefficient digits, the exact quotient's
// denominator is less than 10^(d+max(0,s+e)). Any non-tie distance from a
// four-place Money midpoint is therefore greater than 0.5*10^-(d+max(0,s+e)+4).
// DivRound's error at that many places is below the bound, so it cannot cross
// a Money midpoint. The 16-place minimum retains the existing intermediate
// precision when converted components are summed before the Money boundary.
// Exact Money ties terminate by five places and are preserved by RoundBank.
func divideFXAmountByRate(value, rate decimal.Decimal) (decimal.Decimal, error) {
	if rate.IsZero() {
		return decimal.Zero, validation("fxRate", "must be greater than zero")
	}
	scale, _ := decimalShape(value)
	if scale > maxFXConversionInputFractionDigits {
		return decimal.Zero, &Error{Code: ErrDecimalOverflow, Field: "amount", Message: "calculation is outside the supported range"}
	}
	denominatorScale := int64(scale) + int64(rate.Exponent())
	if denominatorScale < 0 {
		denominatorScale = 0
	}
	precision := int64(len(rate.Coefficient().String())) + denominatorScale + moneyAmountFractionDigits
	if precision < minimumFXDivisionPrecision {
		precision = minimumFXDivisionPrecision
	}
	if precision > math.MaxInt32 {
		return decimal.Zero, &Error{Code: ErrDecimalOverflow, Field: "amount", Message: "calculation is outside the supported range"}
	}
	return value.DivRound(rate, int32(precision)), nil
}

func validateBoundedDecimal(field string, value, maximum decimal.Decimal, maxIntegerDigits, maxScale int, positive bool) error {
	if value.IsNegative() {
		return validation(field, "must not be negative")
	}
	if positive && value.IsZero() {
		return validation(field, "must be greater than zero")
	}
	if value.GreaterThan(maximum) {
		return &Error{Code: ErrDecimalOverflow, Field: field, Message: "value is outside the supported range"}
	}
	scale, integerDigits := decimalShape(value)
	if scale > maxScale || integerDigits > maxIntegerDigits {
		return &Error{Code: ErrDecimalOverflow, Field: field, Message: "value is outside the supported range"}
	}
	return nil
}

func decimalShape(value decimal.Decimal) (scale, integerDigits int) {
	if value.IsZero() {
		return 0, 1
	}
	coefficientDigits := len(value.Abs().Coefficient().String())
	exponent := int(value.Exponent())
	if exponent >= 0 {
		return 0, coefficientDigits + exponent
	}
	scale = -exponent
	integerDigits = coefficientDigits - scale
	if integerDigits < 1 {
		integerDigits = 1
	}
	return scale, integerDigits
}

func canonicalDecimal(value decimal.Decimal) string {
	if value.IsZero() {
		return "0"
	}
	return value.String()
}

func checkedIntermediate(value decimal.Decimal) (decimal.Decimal, error) {
	if value.IsNegative() || value.GreaterThan(maxMoney) {
		return decimal.Zero, &Error{Code: ErrDecimalOverflow, Field: "amount", Message: "calculation is outside the supported range"}
	}
	return value, nil
}
