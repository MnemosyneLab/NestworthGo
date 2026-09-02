package domain

import (
	"regexp"

	"github.com/shopspring/decimal"
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

func NewFXRate(value decimal.Decimal) (FxRate, error) { return NewFxRate(value) }

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
	return checkedIntermediate(value.Div(rate.value))
}

func (r FxRate) Inverse() (FxRate, error) {
	return NewFxRate(decimal.NewFromInt(1).Div(r.value))
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
