package domain

import (
	"strings"
	"time"
)

const (
	HistoryCapabilityUnavailable = "history_capability_unavailable"
	InstrumentTypeUnsupported    = "instrument_type_unsupported"
	MarketSessionUnsupported     = "market_not_supported_for_session_policy"
)

// CryptoDailyBarMarket reports whether the listing code is a crypto venue
// rather than an equity exchange. Asset type remains authoritative: a US
// Bitcoin instrument is still crypto, and a US gold ETF is still an ETF.
func CryptoDailyBarMarket(market string) bool {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "CRYPTO", "CCC", "CC", "COIN", "CRYPTOCURRENCY":
		return true
	default:
		return false
	}
}

// InstrumentUsesCryptoDailyBar is true when identity is crypto. Display names
// such as Gold are ignored; a precious-metal instrument is not treated as
// crypto, and an ETF is not treated as a commodity bar.
func InstrumentUsesCryptoDailyBar(instrumentType, market string) bool {
	parsed, err := ParseInstrumentType(instrumentType)
	if err == nil {
		return parsed == InstrumentCrypto
	}
	return strings.TrimSpace(instrumentType) == "" && CryptoDailyBarMarket(market)
}

// ResolveInstrumentHistorySupport is the local, offline type/market overlay
// on top of ResolveInstrumentRoute. It does not contact providers.
//
// Stocks and ETFs on a known equity session may use Yahoo, and US-listed
// stocks/ETFs may also use Tiingo. Yahoo crypto uses a UTC daily-bar policy,
// not an equity exchange close. Other instrument types stay explicitly
// unsupported rather than inheriting a provider whitelist.
func ResolveInstrumentHistorySupport(instrumentType, market, providerKey string) InstrumentRoute {
	provider := strings.ToLower(strings.TrimSpace(providerKey))
	parsed, err := ParseInstrumentType(instrumentType)
	if err != nil {
		if strings.TrimSpace(instrumentType) == "" {
			if _, equity := EquitySessionScheduleForMarket(market); equity {
				parsed = InstrumentStock
			} else if CryptoDailyBarMarket(market) {
				parsed = InstrumentCrypto
			} else {
				return InstrumentRoute{ProviderKey: provider, Status: InstrumentRouteUnsupported, Reason: InstrumentTypeUnsupported}
			}
		} else {
			return InstrumentRoute{ProviderKey: provider, Status: InstrumentRouteUnsupported, Reason: InstrumentTypeUnsupported}
		}
	}
	switch parsed {
	case InstrumentStock, InstrumentETF:
		if _, ok := EquitySessionScheduleForMarket(market); !ok {
			return InstrumentRoute{ProviderKey: provider, Status: InstrumentRouteUnsupported, Reason: MarketSessionUnsupported}
		}
		if provider == TiingoProviderKey && !USListedEquityMarket(market) {
			return InstrumentRoute{Status: InstrumentRouteUnsupported, Reason: "tiingo_us_listed_only"}
		}
		return InstrumentRoute{ProviderKey: provider, Status: InstrumentRouteOK}
	case InstrumentCrypto:
		if provider == TiingoProviderKey {
			return InstrumentRoute{Status: InstrumentRouteUnsupported, Reason: "tiingo_us_listed_only"}
		}
		if provider == YahooFinanceProviderKey || provider == WorkerProviderKey || provider == "" {
			if provider == "" {
				provider = YahooFinanceProviderKey
			}
			return InstrumentRoute{ProviderKey: provider, Status: InstrumentRouteOK}
		}
		return InstrumentRoute{Status: InstrumentRouteUnsupported, Reason: "provider_not_selectable"}
	default:
		return InstrumentRoute{ProviderKey: provider, Status: InstrumentRouteUnsupported, Reason: InstrumentTypeUnsupported}
	}
}

// CryptoDailyBarEligibleAt is the conservative policy-derived eligibility
// boundary for a Yahoo UTC daily bar: the end of that UTC calendar day. It is
// not an observed exchange close and must not use equity session clocks.
func CryptoDailyBarEligibleAt(marketDate string) (time.Time, error) {
	return endOfUTCCalendarDay(marketDate, "marketDate")
}

// LastFinalizedCryptoMarketDate is the latest UTC daily-bar date whose
// eligibility boundary is strictly before now.
func LastFinalizedCryptoMarketDate(now time.Time) (string, error) {
	if now.IsZero() {
		return "", validation("now", "backend clock is required")
	}
	start := now.UTC()
	for i := 1; i <= 3; i++ {
		date := start.AddDate(0, 0, -i).Format("2006-01-02")
		eligible, err := CryptoDailyBarEligibleAt(date)
		if err != nil {
			return "", err
		}
		if eligible.Before(now) {
			return date, nil
		}
	}
	return "", validation("session", "no finalized crypto daily bar is available")
}

func endOfUTCCalendarDay(dateLabel, field string) (time.Time, error) {
	date, err := ParseMarketDate(dateLabel)
	if err != nil {
		return time.Time{}, err
	}
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		return time.Time{}, validation(field, "must use YYYY-MM-DD")
	}
	return parsed.AddDate(0, 0, 1).Add(-time.Millisecond).UTC(), nil
}
