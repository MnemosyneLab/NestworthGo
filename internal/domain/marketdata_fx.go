package domain

import (
	"strings"

	"github.com/shopspring/decimal"
)

const fxInversionIntermediateScale int32 = 24

type FXMappingStatus string

const (
	FXMappingIdentity    FXMappingStatus = "identity"
	FXMappingDirect      FXMappingStatus = "direct"
	FXMappingReciprocal  FXMappingStatus = "reciprocal"
	FXMappingUnsupported FXMappingStatus = "unsupported"
)

type FXMapping struct {
	Status        FXMappingStatus
	Rate          FxRate
	BaseCurrency  CurrencyCode
	QuoteCurrency CurrencyCode
	Derived       bool
	Reason        string
}

// MapFXRate applies the vNext FX mapping contract. Same-currency conversion
// is identity 1 and needs neither a fetch nor a stored quote. A reciprocal
// is a derived value, not an extra raw observation. Unsupported pairs stay
// explicit.
func MapFXRate(base, quote CurrencyCode, storedBase, storedQuote CurrencyCode, storedRate string) (FXMapping, error) {
	base, err := ParseCurrency(base.String())
	if err != nil {
		return FXMapping{}, err
	}
	quote, err = ParseCurrency(quote.String())
	if err != nil {
		return FXMapping{}, err
	}
	if base == quote {
		identity, err := ParseFxRate("1")
		if err != nil {
			return FXMapping{}, err
		}
		return FXMapping{Status: FXMappingIdentity, Rate: identity, BaseCurrency: base, QuoteCurrency: quote, Derived: false}, nil
	}
	if !isSupportedFXCurrency(base) || !isSupportedFXCurrency(quote) {
		return FXMapping{Status: FXMappingUnsupported, Reason: "unsupported_currency_pair", BaseCurrency: base, QuoteCurrency: quote}, nil
	}
	storedRate = strings.TrimSpace(storedRate)
	if storedRate == "" {
		return FXMapping{Status: FXMappingUnsupported, Reason: "rate_missing", BaseCurrency: base, QuoteCurrency: quote}, nil
	}
	rate, err := ParseFxRate(storedRate)
	if err != nil {
		return FXMapping{Status: FXMappingUnsupported, Reason: "malformed_rate", BaseCurrency: base, QuoteCurrency: quote}, nil
	}
	storedBase, err = ParseCurrency(storedBase.String())
	if err != nil {
		return FXMapping{}, err
	}
	storedQuote, err = ParseCurrency(storedQuote.String())
	if err != nil {
		return FXMapping{}, err
	}
	if storedBase == base && storedQuote == quote {
		return FXMapping{Status: FXMappingDirect, Rate: rate, BaseCurrency: base, QuoteCurrency: quote, Derived: false}, nil
	}
	if storedBase == quote && storedQuote == base {
		inverted, err := InvertFxRateHalfEven(rate)
		if err != nil {
			return FXMapping{}, err
		}
		return FXMapping{Status: FXMappingReciprocal, Rate: inverted, BaseCurrency: base, QuoteCurrency: quote, Derived: true}, nil
	}
	return FXMapping{Status: FXMappingUnsupported, Reason: "no_application_triangulation", BaseCurrency: base, QuoteCurrency: quote}, nil
}

// InvertFxRateHalfEven divides 1 by rate to 24 fractional digits with
// half-even rounding, then applies domain FxRate rounding. The reciprocal
// is derived and must not be persisted as a raw observation.
func InvertFxRateHalfEven(rate FxRate) (FxRate, error) {
	if rate.value.IsZero() {
		return FxRate{}, validation("fxRate", "must be greater than zero")
	}
	one := decimal.NewFromInt(1)
	shifted := one.Shift(fxInversionIntermediateScale).Div(rate.value).RoundBank(0)
	intermediate := shifted.Shift(-fxInversionIntermediateScale)
	rounded := intermediate.RoundBank(12)
	return NewFxRate(rounded)
}

func isSupportedFXCurrency(code CurrencyCode) bool {
	for _, supported := range SupportedCurrencies() {
		if supported == code {
			return true
		}
	}
	return false
}
