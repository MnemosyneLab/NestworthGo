package domain

import (
	"strings"
	"time"
)

const (
	RecentNoObservationWindow     = 7
	RecentCorrectionWindow        = 3
	RecentNoObservationTTL        = 24 * time.Hour
	OlderNoObservationTTL         = 30 * 24 * time.Hour
	RecentCorrectionTTL           = 24 * time.Hour
	InstrumentRouteOK             = "ok"
	InstrumentRouteUnsupported    = "unsupported"
	InstrumentRouteBindingMissing = "binding_missing"
)

type InstrumentRoute struct {
	ProviderKey string
	Status      string
	Reason      string
}

// ResolveInstrumentRoute is the historical/latest instrument provider route
// for a listing market. US equities may use Tiingo or Yahoo; China and other
// markets are Yahoo-only. There is no silent cross-provider fallback.
func ResolveInstrumentRoute(market, preferredProvider, providerSymbol string) InstrumentRoute {
	preferredProvider = strings.ToLower(strings.TrimSpace(preferredProvider))
	providerSymbol = strings.TrimSpace(providerSymbol)
	if preferredProvider == FrankfurterProviderKey {
		return InstrumentRoute{Status: InstrumentRouteUnsupported, Reason: "frankfurter_is_fx_only"}
	}
	if USListedEquityMarket(market) {
		switch preferredProvider {
		case "":
			if providerSymbol == "" {
				return InstrumentRoute{Status: InstrumentRouteBindingMissing, Reason: "binding_missing"}
			}
			return InstrumentRoute{ProviderKey: YahooFinanceProviderKey, Status: InstrumentRouteOK}
		case YahooFinanceProviderKey, TiingoProviderKey:
			if providerSymbol == "" {
				return InstrumentRoute{ProviderKey: preferredProvider, Status: InstrumentRouteBindingMissing, Reason: "binding_missing"}
			}
			return InstrumentRoute{ProviderKey: preferredProvider, Status: InstrumentRouteOK}
		default:
			return InstrumentRoute{Status: InstrumentRouteUnsupported, Reason: "provider_not_selectable"}
		}
	}
	if preferredProvider == TiingoProviderKey {
		return InstrumentRoute{Status: InstrumentRouteUnsupported, Reason: "tiingo_us_listed_only"}
	}
	if preferredProvider != "" && preferredProvider != YahooFinanceProviderKey {
		return InstrumentRoute{Status: InstrumentRouteUnsupported, Reason: "provider_not_selectable"}
	}
	if providerSymbol == "" || preferredProvider == "" {
		return InstrumentRoute{ProviderKey: YahooFinanceProviderKey, Status: InstrumentRouteBindingMissing, Reason: "binding_missing"}
	}
	return InstrumentRoute{ProviderKey: YahooFinanceProviderKey, Status: InstrumentRouteOK}
}

func LastNInclusiveDates(end string, n int) ([]string, error) {
	if n <= 0 {
		return nil, validation("count", "must be positive")
	}
	endDate, err := ParseMarketDate(end)
	if err != nil {
		return nil, err
	}
	parsed, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, validation("marketDate", "must use YYYY-MM-DD")
	}
	start := parsed.AddDate(0, 0, -(n - 1)).Format("2006-01-02")
	return InclusiveMarketDates(start, endDate)
}

func DateInLastN(date, lastFinalized string, n int) (bool, error) {
	window, err := LastNInclusiveDates(lastFinalized, n)
	if err != nil {
		return false, err
	}
	date, err = ParseMarketDate(date)
	if err != nil {
		return false, err
	}
	for _, item := range window {
		if item == date {
			return true, nil
		}
	}
	return false, nil
}

// NoObservationExpiresAt is the negative-cache expiry for a successful
// omission. Recent omissions (last 7 finalized dates) expire after 24 hours;
// older omissions expire after 30 days. checkedAt is the successful check
// time, not an economic timestamp.
func NoObservationExpiresAt(marketDate string, checkedAt time.Time) (time.Time, error) {
	if checkedAt.IsZero() {
		return time.Time{}, validation("checkedAt", "checked time is required")
	}
	lastFinalized, err := LastFinalizedUSEquityMarketDate(checkedAt)
	if err != nil {
		return checkedAt.Add(RecentNoObservationTTL), nil
	}
	recent, err := DateInLastN(marketDate, lastFinalized, RecentNoObservationWindow)
	if err != nil {
		return time.Time{}, err
	}
	if recent {
		return checkedAt.Add(RecentNoObservationTTL), nil
	}
	return checkedAt.Add(OlderNoObservationTTL), nil
}

func LatestRequestDue(lastSuccessfulCheck, now time.Time, ttl time.Duration, force bool) bool {
	if force {
		return true
	}
	if ttl <= 0 || lastSuccessfulCheck.IsZero() {
		return true
	}
	return !lastSuccessfulCheck.Add(ttl).After(now)
}

type HistoryCoverageKind string

const (
	HistoryCoverageNone          HistoryCoverageKind = ""
	HistoryCoverageClose         HistoryCoverageKind = "close"
	HistoryCoverageNoObservation HistoryCoverageKind = "no_observation"
)

type HistoryFetchAction string

const (
	HistoryFetch             HistoryFetchAction = "fetch"
	HistorySkipExistingClose HistoryFetchAction = "skip_close"
	HistorySkipNoObservation HistoryFetchAction = "skip_no_observation"
)

type HistoryFetchDecision struct {
	Action HistoryFetchAction
	Reason string
}

type HistoryFetchInput struct {
	Date                   string
	LastFinalized          string
	Now                    time.Time
	ForceRecheck           bool
	HasClose               bool
	CloseFetchedAt         time.Time
	HasNoObservation       bool
	NoObservationExpiresAt time.Time
	NoObservationHasExpiry bool
	NoObservationCheckedAt time.Time
}

// DecideHistoryFetch applies section 12.6 / 17 cache policy for one
// finalized date. Force Recheck bypasses both positive and negative caches.
// Ordinary sync also rechecks the last 3 finalized dates once per 24 hours.
func DecideHistoryFetch(input HistoryFetchInput) HistoryFetchDecision {
	if input.ForceRecheck {
		return HistoryFetchDecision{Action: HistoryFetch, Reason: "force_recheck"}
	}
	inCorrection, err := DateInLastN(input.Date, input.LastFinalized, RecentCorrectionWindow)
	if err != nil {
		return HistoryFetchDecision{Action: HistoryFetch, Reason: "invalid_date"}
	}
	correctionDue := func(lastChecked time.Time) bool {
		if !inCorrection {
			return false
		}
		return LatestRequestDue(lastChecked, input.Now, RecentCorrectionTTL, false)
	}
	if input.HasClose {
		if correctionDue(input.CloseFetchedAt) {
			return HistoryFetchDecision{Action: HistoryFetch, Reason: "recent_correction"}
		}
		return HistoryFetchDecision{Action: HistorySkipExistingClose, Reason: "existing_close"}
	}
	if input.HasNoObservation {
		if input.NoObservationHasExpiry && !input.NoObservationExpiresAt.IsZero() && !input.Now.Before(input.NoObservationExpiresAt) {
			return HistoryFetchDecision{Action: HistoryFetch, Reason: "no_observation_expired"}
		}
		if correctionDue(input.NoObservationCheckedAt) {
			return HistoryFetchDecision{Action: HistoryFetch, Reason: "recent_correction"}
		}
		return HistoryFetchDecision{Action: HistorySkipNoObservation, Reason: "unexpired_no_observation"}
	}
	return HistoryFetchDecision{Action: HistoryFetch, Reason: "coverage_gap"}
}
