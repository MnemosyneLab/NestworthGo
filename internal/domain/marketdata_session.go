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
	CNEquitySessionTimezone    = "Asia/Shanghai"
	CNEquityRegularCloseClock  = "15:00"
	CNEquityRegularClosePolicy = "cn_equity_regular_close_v1"
	HKEquitySessionTimezone    = "Asia/Hong_Kong"
	HKEquityRegularCloseClock  = "16:00"
	HKEquityRegularClosePolicy = "hk_equity_regular_close_v1"
	SGEquitySessionTimezone    = "Asia/Singapore"
	SGEquityRegularCloseClock  = "17:00"
	SGEquityRegularClosePolicy = "sg_equity_regular_close_v1"
	JPEquitySessionTimezone    = "Asia/Tokyo"
	JPEquityRegularCloseClock  = "15:30"
	JPEquityRegularClosePolicy = "jp_equity_regular_close_v1"
	TWEquitySessionTimezone    = "Asia/Taipei"
	TWEquityRegularCloseClock  = "13:30"
	TWEquityRegularClosePolicy = "tw_equity_regular_close_v1"
	AUEquitySessionTimezone    = "Australia/Sydney"
	AUEquityRegularCloseClock  = "16:00"
	AUEquityRegularClosePolicy = "au_equity_regular_close_v1"
	GBEquitySessionTimezone    = "Europe/London"
	GBEquityRegularCloseClock  = "16:30"
	GBEquityRegularClosePolicy = "gb_equity_regular_close_v1"
	DEEquitySessionTimezone    = "Europe/Berlin"
	DEEquityRegularCloseClock  = "17:30"
	DEEquityRegularClosePolicy = "de_equity_regular_close_v1"
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

// EquitySessionSchedule is the market-specific session contract needed to
// turn a provider market-date label into an economic close instant. A market
// is supported only when this schedule is known; silently applying the US
// session to another exchange would make historical prices economically wrong.
type EquitySessionSchedule struct {
	Timezone   string
	CloseClock string
	EarlyClock string
	Policy     string
}

func EquitySessionScheduleForMarket(market string) (EquitySessionSchedule, bool) {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "US", "US_EQUITY", "XNYS", "XNAS", "NYSE", "NASDAQ":
		return EquitySessionSchedule{Timezone: USEquitySessionTimezone, CloseClock: USEquityRegularCloseClock, EarlyClock: USEquityEarlyCloseClock, Policy: USEquityRegularClosePolicy}, true
	case "AMEX", "XASE":
		return EquitySessionSchedule{Timezone: USEquitySessionTimezone, CloseClock: USEquityRegularCloseClock, EarlyClock: USEquityEarlyCloseClock, Policy: USEquityRegularClosePolicy}, true
	case "CN", "SSE", "SZSE", "BSE", "XSHG", "XSHE":
		return EquitySessionSchedule{Timezone: CNEquitySessionTimezone, CloseClock: CNEquityRegularCloseClock, Policy: CNEquityRegularClosePolicy}, true
	case "HK", "HKEX", "XHKG":
		return EquitySessionSchedule{Timezone: HKEquitySessionTimezone, CloseClock: HKEquityRegularCloseClock, Policy: HKEquityRegularClosePolicy}, true
	case "SG", "SGX", "XSES":
		return EquitySessionSchedule{Timezone: SGEquitySessionTimezone, CloseClock: SGEquityRegularCloseClock, Policy: SGEquityRegularClosePolicy}, true
	case "JP", "TSE", "XTKS":
		return EquitySessionSchedule{Timezone: JPEquitySessionTimezone, CloseClock: JPEquityRegularCloseClock, Policy: JPEquityRegularClosePolicy}, true
	case "TW", "TWSE", "XTAI":
		return EquitySessionSchedule{Timezone: TWEquitySessionTimezone, CloseClock: TWEquityRegularCloseClock, Policy: TWEquityRegularClosePolicy}, true
	case "KR", "KRX", "XKRX":
		return EquitySessionSchedule{Timezone: "Asia/Seoul", CloseClock: "15:30", Policy: "kr_equity_regular_close_v1"}, true
	case "AU", "ASX", "XASX":
		return EquitySessionSchedule{Timezone: AUEquitySessionTimezone, CloseClock: AUEquityRegularCloseClock, Policy: AUEquityRegularClosePolicy}, true
	case "GB", "UK", "LSE", "XLON":
		return EquitySessionSchedule{Timezone: GBEquitySessionTimezone, CloseClock: GBEquityRegularCloseClock, Policy: GBEquityRegularClosePolicy}, true
	case "DE", "XETRA", "XFRA":
		return EquitySessionSchedule{Timezone: DEEquitySessionTimezone, CloseClock: DEEquityRegularCloseClock, Policy: DEEquityRegularClosePolicy}, true
	case "EURONEXT", "XPAR":
		return EquitySessionSchedule{Timezone: "Europe/Paris", CloseClock: "17:30", Policy: "eu_equity_regular_close_v1"}, true
	case "CH", "SIX", "XSWX":
		return EquitySessionSchedule{Timezone: "Europe/Zurich", CloseClock: "17:30", Policy: "ch_equity_regular_close_v1"}, true
	default:
		return EquitySessionSchedule{}, false
	}
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
	schedule, supported := EquitySessionScheduleForMarket(market)
	if !supported {
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
		timezone = schedule.Timezone
	} else if timezone != schedule.Timezone {
		return SessionResolution{Status: "uncertain", Reason: "session_timezone_mismatch", TimestampBasis: "unknown"}, nil
	}
	clock := strings.TrimSpace(evidence.CloseClock)
	if clock == "" {
		if kind == SessionKindEarlyClose {
			clock = schedule.EarlyClock
			if clock == "" {
				return SessionResolution{Status: "uncertain", Reason: "early_close_clock_unverified", TimestampBasis: "unknown"}, nil
			}
		} else {
			clock = schedule.CloseClock
		}
	}
	if kind == SessionKindRegular && clock != schedule.CloseClock {
		return SessionResolution{Status: "uncertain", Reason: "session_close_clock_unverified", TimestampBasis: "unknown"}, nil
	}
	if kind == SessionKindEarlyClose && (schedule.EarlyClock == "" || clock == schedule.CloseClock || clock != schedule.EarlyClock) {
		return SessionResolution{Status: "uncertain", Reason: "early_close_clock_unverified", TimestampBasis: "unknown"}, nil
	}
	closeInstant, err := ResolveLocalDateTime(date, clock, timezone)
	if err != nil {
		return SessionResolution{Status: "uncertain", Reason: "session_clock_not_resolvable", TimestampBasis: "unknown"}, nil
	}
	policy := strings.TrimSpace(evidence.Policy)
	if policy == "" {
		policy = schedule.Policy
	} else if policy != schedule.Policy {
		return SessionResolution{Status: "uncertain", Reason: "session_policy_unverified", TimestampBasis: "unknown"}, nil
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
	return LastFinalizedEquityMarketDate(now, "US")
}

// LastFinalizedEquityMarketDate is the latest regular-session date for the
// requested supported market whose close is strictly before now.
func LastFinalizedEquityMarketDate(now time.Time, market string) (string, error) {
	if now.IsZero() {
		return "", validation("now", "backend clock is required")
	}
	schedule, supported := EquitySessionScheduleForMarket(market)
	if !supported {
		return "", validation("market", "market is not supported for session policy")
	}
	location, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		return "", err
	}
	evidence := SessionEvidence{Kind: SessionKindRegular, Timezone: schedule.Timezone, CloseClock: schedule.CloseClock, Policy: schedule.Policy}
	start := now.In(location)
	for i := 0; i < 14; i++ {
		date := start.AddDate(0, 0, -i).Format("2006-01-02")
		session, resolveErr := ResolveEquitySessionClose(date, market, evidence)
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
	return "", validation("session", "no finalized equity close is available")
}
