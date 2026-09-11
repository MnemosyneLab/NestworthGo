package domain

import (
	"strings"
	"time"
)

const (
	USEquitySessionTimezone    = "America/New_York"
	USEquityRegularCloseClock  = "16:00"
	USEquityEarlyCloseClock    = "13:00"
	USEquityRegularClosePolicy = "us_equity_regular_close_v1"
	FrankfurterV2BlendedPolicy = "frankfurter-v2-blended-v1"
	TiingoRawClosePriceBasis   = "tiingo_raw_close_v1"
	YahooRawClosePriceBasis    = "yahoo_close_v1"
	TiingoProviderKey          = "tiingo"
	YahooFinanceProviderKey    = "yahoo_finance"
	FrankfurterProviderKey     = "frankfurter"
)

type SessionKind string

const (
	SessionKindRegular    SessionKind = "regular"
	SessionKindEarlyClose SessionKind = "early_close"
	SessionKindUnknown    SessionKind = "unknown"
)

type SessionEvidence struct {
	Kind       SessionKind
	Timezone   string
	CloseClock string
	Policy     string
}

type SessionResolution struct {
	Status         string // mapped, uncertain, unsupported
	Reason         string
	CloseInstant   time.Time
	TimestampBasis string
	Policy         string
}

// ResolveEquitySessionClose maps a market-date label to an economic close
// instant using recorded session evidence. UTC midnight on the market date is
// never treated as the close. Unknown timing stays uncertain; the adapter
// must not invent a timestamp or mark coverage verified.
func ResolveEquitySessionClose(marketDate, market string, evidence SessionEvidence) (SessionResolution, error) {
	date, err := ParseMarketDate(marketDate)
	if err != nil {
		return SessionResolution{}, err
	}
	market = strings.TrimSpace(market)
	if !USListedEquityMarket(market) {
		return SessionResolution{Status: "unsupported", Reason: "market_not_supported_for_session_policy"}, nil
	}
	kind := evidence.Kind
	if kind == "" {
		kind = SessionKindUnknown
	}
	switch kind {
	case SessionKindUnknown:
		return SessionResolution{Status: "uncertain", Reason: "session_timing_unknown", TimestampBasis: "unknown"}, nil
	case SessionKindRegular, SessionKindEarlyClose:
	default:
		return SessionResolution{Status: "unsupported", Reason: "session_kind_unsupported"}, nil
	}
	timezone := strings.TrimSpace(evidence.Timezone)
	if timezone == "" {
		timezone = USEquitySessionTimezone
	}
	clock := strings.TrimSpace(evidence.CloseClock)
	if clock == "" {
		if kind == SessionKindEarlyClose {
			clock = USEquityEarlyCloseClock
		} else {
			clock = USEquityRegularCloseClock
		}
	}
	if kind == SessionKindEarlyClose && clock == USEquityRegularCloseClock {
		return SessionResolution{Status: "uncertain", Reason: "early_close_clock_unverified", TimestampBasis: "unknown"}, nil
	}
	closeInstant, err := ResolveLocalDateTime(date, clock, timezone)
	if err != nil {
		return SessionResolution{Status: "uncertain", Reason: "session_clock_not_resolvable", TimestampBasis: "unknown"}, nil
	}
	policy := strings.TrimSpace(evidence.Policy)
	if policy == "" {
		policy = USEquityRegularClosePolicy
	}
	return SessionResolution{
		Status:         "mapped",
		CloseInstant:   closeInstant,
		TimestampBasis: "session_close",
		Policy:         policy,
	}, nil
}

func USListedEquityMarket(market string) bool {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "US", "US_EQUITY", "XNYS", "XNAS", "NYSE", "NASDAQ":
		return true
	default:
		return false
	}
}

// FrankfurterReferenceEligibleAt is the conservative policy-derived
// eligibility boundary for a daily FX reference that has no observed
// publication instant: the end of the source reference day in UTC. It is
// not an observed publication time.
func FrankfurterReferenceEligibleAt(referenceDate string) (time.Time, error) {
	date, err := ParseMarketDate(referenceDate)
	if err != nil {
		return time.Time{}, err
	}
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		return time.Time{}, validation("referenceDate", "must use YYYY-MM-DD")
	}
	return parsed.AddDate(0, 0, 1).Add(-time.Millisecond).UTC(), nil
}

func OpeningAnchorLookbackWindows() []int {
	return []int{7, 30, 365}
}

// LastFinalizedUSEquityMarketDate is the latest US regular-session market
// date whose close instant is strictly before now. It is a session label,
// not a household local date.
func LastFinalizedUSEquityMarketDate(now time.Time) (string, error) {
	if now.IsZero() {
		return "", validation("now", "backend clock is required")
	}
	location, err := time.LoadLocation(USEquitySessionTimezone)
	if err != nil {
		return "", err
	}
	evidence := SessionEvidence{Kind: SessionKindRegular, Timezone: USEquitySessionTimezone, CloseClock: USEquityRegularCloseClock, Policy: USEquityRegularClosePolicy}
	start := now.In(location)
	for i := 0; i < 14; i++ {
		date := start.AddDate(0, 0, -i).Format("2006-01-02")
		session, resolveErr := ResolveEquitySessionClose(date, "US", evidence)
		if resolveErr != nil {
			return "", resolveErr
		}
		if session.Status != "mapped" {
			continue
		}
		if session.CloseInstant.Before(now) {
			return date, nil
		}
	}
	return "", validation("session", "no finalized US equity close is available")
}
