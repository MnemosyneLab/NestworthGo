package domain

import (
	"github.com/shopspring/decimal"
	"strings"
	"time"
)

const TroyOunceGrams = "31.1034768"
const MetalConversionPolicy = "metal_usd_troy_ounce_v1"
const MetalFuturesMarket = "COMEX"

func MetalProviderSymbol(template string) string {
	switch template {
	case "gold":
		return "GC=F"
	case "silver":
		return "SI=F"
	}
	return ""
}

func applyMetalTemplateDefaults(input *InstrumentInput) {
	if MetalProviderSymbol(input.MetalTemplate) == "" {
		return
	}
	provider, symbol, market := YahooFinanceProviderKey, MetalProviderSymbol(input.MetalTemplate), MetalFuturesMarket
	if input.ProviderKey == nil || strings.TrimSpace(*input.ProviderKey) == "" {
		input.ProviderKey = &provider
	}
	if input.ProviderSymbol == nil || strings.TrimSpace(*input.ProviderSymbol) == "" {
		input.ProviderSymbol = &symbol
	}
	if input.MarketCode == nil || strings.TrimSpace(*input.MarketCode) == "" {
		input.MarketCode = &market
	}
}

func ValidateMetalConfiguration(input InstrumentInput) error {
	if input.MetalTemplate == "" {
		if input.QuantityUnit != "" {
			return validation("quantityUnit", "a metal template is required")
		}
		return nil
	}
	if input.Type != InstrumentPreciousMetal || MetalProviderSymbol(input.MetalTemplate) == "" {
		return validation("metalTemplate", "unsupported precious metal template")
	}
	if input.QuantityUnit != "g" && input.QuantityUnit != "troy_oz" {
		return validation("quantityUnit", "select grams or troy ounces")
	}
	if input.MarketCode == nil || *input.MarketCode != MetalFuturesMarket {
		return validation("marketCode", "metal templates use COMEX reference prices")
	}
	if input.ProviderKey == nil || *input.ProviderKey != YahooFinanceProviderKey || input.ProviderSymbol == nil || *input.ProviderSymbol != MetalProviderSymbol(input.MetalTemplate) {
		return validation("providerSymbol", "metal templates require their Yahoo futures symbol")
	}
	return nil
}

func (i Instrument) UsesMetalConversion() bool {
	return i.Type == InstrumentPreciousMetal && MetalProviderSymbol(i.MetalTemplate) != ""
}

// ConvertMetalPrice rounds only at the existing eight-decimal UnitPrice boundary.
func ConvertMetalPrice(raw UnitPrice, unit string, usdRate decimal.Decimal) (UnitPrice, error) {
	if !usdRate.IsPositive() {
		return UnitPrice{}, validation("fxRate", "exchange rate must be positive")
	}
	value := raw.Decimal().Mul(usdRate)
	switch unit {
	case "g":
		value = value.DivRound(decimal.RequireFromString(TroyOunceGrams), 24)
	case "troy_oz":
	default:
		return UnitPrice{}, validation("quantityUnit", "unsupported weight unit")
	}
	return UnitPriceFromExact(value)
}

func UsesMetalFuturesHistory(instrumentType, market string) bool {
	return instrumentType == string(InstrumentPreciousMetal) && strings.EqualFold(market, MetalFuturesMarket)
}

// Futures bars use the provider's exchange date. Conservatively expose a daily
// reference only after that whole New York calendar day has ended, not at an
// invented equity close or as an exchange settlement price.
func MetalDailyBarEligibleAt(date string) (time.Time, error) {
	if _, err := ParseMarketDate(date); err != nil {
		return time.Time{}, err
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.Time{}, err
	}
	start, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return time.Time{}, err
	}
	return start.AddDate(0, 0, 1).Add(-time.Millisecond).UTC(), nil
}

func LastFinalizedMetalMarketDate(now time.Time) (string, error) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return "", err
	}
	return now.In(loc).AddDate(0, 0, -1).Format("2006-01-02"), nil
}

// MetalDailyReferenceClosedDate describes GC/SI trade-date labels, not wall-clock
// trading hours. Sunday evening trading belongs to Monday's trade date. Do not
// generalize this rule to other COMEX contracts or infer weekday holidays.
func MetalDailyReferenceClosedDate(instrumentType, market, symbol, date string) bool {
	if !UsesMetalFuturesHistory(instrumentType, market) || (symbol != "GC=F" && symbol != "SI=F") {
		return false
	}
	day, err := time.Parse("2006-01-02", date)
	return err == nil && (day.Weekday() == time.Saturday || day.Weekday() == time.Sunday)
}
