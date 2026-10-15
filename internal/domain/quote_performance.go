package domain

import (
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// QuotePerformance is one descriptive price/NAV change. WindowDays is the
// requested lookback in civil days; zero means the whole comparable history.
type QuotePerformance struct {
	WindowDays        int     `json:"windowDays"`
	BaselineOn        string  `json:"baselineOn"`
	LatestOn          string  `json:"latestOn"`
	ElapsedDays       int     `json:"elapsedDays"`
	PeriodPercent     string  `json:"periodPercent"`
	AnnualizedPercent *string `json:"annualizedPercent,omitempty"`
}

// QuotePerformanceReport keeps unavailable evidence distinct from a zero return.
type QuotePerformanceReport struct {
	Items             []QuotePerformance `json:"items"`
	UnavailableReason string             `json:"unavailableReason,omitempty"`
}

func ProjectQuotePerformanceReport(i Instrument, quotes []InstrumentQuote) QuotePerformanceReport {
	return ProjectQuotePerformanceReportWindows(i, quotes, QuotePerformanceWindows)
}

func ProjectQuotePerformanceReportWindows(i Instrument, quotes []InstrumentQuote, windows []int) QuotePerformanceReport {
	report := QuotePerformanceReport{Items: ProjectQuotePerformanceWindows(i, quotes, windows)}
	if len(report.Items) > 0 {
		return report
	}
	if _, supported := quotePerformanceMode(i.Type); !supported {
		report.UnavailableReason = "unsupported_instrument"
		return report
	}
	dates := []string{}
	for _, q := range quotes {
		if _, err := time.Parse(time.DateOnly, q.EffectiveDate); err == nil && q.ObservationKind == "close" {
			dates = append(dates, q.EffectiveDate)
		}
	}
	sort.Strings(dates)
	report.UnavailableReason = "insufficient_daily_history"
	if len(dates) >= 2 && reviewDays(dates[0], dates[len(dates)-1]) >= 7 {
		report.UnavailableReason = "incomparable_daily_prices"
	}
	return report
}

// priceWindowPercent is shared by descriptive history and investment alerts.
// Civil dates, canonical prices and simple 365-day annualization are independent
// of alert freshness and baseline-gap policies.
func priceWindowPercent(base, latest decimal.Decimal, elapsed int) (string, string) {
	period := latest.DivRound(base, 24).Sub(decimal.NewFromInt(1)).Mul(decimal.NewFromInt(100))
	annual := period.Mul(decimal.NewFromInt(365)).DivRound(decimal.NewFromInt(int64(elapsed)), 12)
	return period.String(), annual.String()
}

// QuotePerformanceWindows are the windows used by alerts and the legacy
// descriptive projection.
var QuotePerformanceWindows = []int{7, 30}

// QuotePeriodWindows are the history periods offered by the performance
// selector (1W, 1M, 3M, 6M, 1Y and All). Zero is the whole comparable history.
var QuotePeriodWindows = []int{7, 30, 90, 180, 365, 0}

// QuoteReturnWindows are the fixed lookbacks of the returns section. All
// history is a chart period only and is never listed as a return.
var QuoteReturnWindows = []int{7, 30, 90, 180, 365}

// ValidQuotePeriodWindow reports whether days is a selector period.
func ValidQuotePeriodWindow(days int) bool {
	for _, window := range QuotePeriodWindows {
		if window == days {
			return true
		}
	}
	return false
}

type quotePerformanceKind int

const (
	quotePerformanceNAV quotePerformanceKind = iota + 1
	quotePerformancePrice
)

// quotePerformanceMode reports how an instrument type's daily facts compare:
// funds and bank products by unit NAV, every other priced type by raw close.
// Unknown types are unsupported so new types opt in explicitly.
func quotePerformanceMode(t InstrumentType) (quotePerformanceKind, bool) {
	switch t {
	case InstrumentMutualFund, InstrumentBankInvestmentProduct:
		return quotePerformanceNAV, true
	case InstrumentStock, InstrumentETF, InstrumentCrypto, InstrumentPreciousMetal, InstrumentBond, InstrumentOther:
		return quotePerformancePrice, true
	default:
		return 0, false
	}
}

// performancePriceBasis lists the explicit raw-close bases whose units are
// comparable for a type. Adjusted or unknown bases are never inferred.
func performancePriceBasis(t InstrumentType, basis string) bool {
	if basis == "agent_raw_close_v1" {
		return true
	}
	switch t {
	case InstrumentStock, InstrumentETF:
		return basis == TiingoRawClosePriceBasis || basis == YahooRawClosePriceBasis
	case InstrumentPreciousMetal:
		// Futures daily reference closes converted to the quote currency and unit.
		return basis == YahooRawClosePriceBasis
	case InstrumentCrypto:
		return basis == CoinGeckoDailyPriceBasis || basis == YahooRawClosePriceBasis
	}
	return false
}

// ProjectQuotePerformance accepts instrument evidence only. Personal positions,
// cash dividends and ledger adjustments cannot establish issuer unit changes.
func ProjectQuotePerformance(i Instrument, quotes []InstrumentQuote) []QuotePerformance {
	return ProjectQuotePerformanceWindows(i, quotes, QuotePerformanceWindows)
}

// ProjectQuotePerformanceWindows projects the requested lookback windows. A
// window is omitted, never filled, when its baseline is missing or any
// observation from baseline to latest is not economically comparable.
func ProjectQuotePerformanceWindows(i Instrument, quotes []InstrumentQuote, windows []int) []QuotePerformance {
	result := []QuotePerformance{}
	mode, supported := quotePerformanceMode(i.Type)
	if !supported {
		return result
	}
	points := dailyPerformanceQuotes(quotes)
	if len(points) < 2 {
		return result
	}
	latest := points[len(points)-1]
	anchor, _ := time.Parse(time.DateOnly, latest.EffectiveDate)
	for _, window := range windows {
		base := 0
		if window > 0 {
			target := anchor.AddDate(0, 0, -window).Format(time.DateOnly)
			base = sort.Search(len(points), func(n int) bool { return points[n].EffectiveDate > target }) - 1
		}
		if base < 0 {
			continue
		}
		first := points[base]
		elapsed := reviewDays(first.EffectiveDate, latest.EffectiveDate)
		comparable := elapsed > 0 && first.UnitPrice.Decimal().IsPositive()
		for _, q := range points[base:] {
			comparable = comparable && q.Currency == i.QuoteCurrency && q.BindingRevision == i.ProviderBindingRevision && performanceSource(q.SourceKind) && performanceTimestamp(i.Type, q.TimestampBasis)
			if mode == quotePerformanceNAV {
				comparable = comparable && q.UnitPrice.Decimal().IsPositive() && q.PriceBasis == "agent_unit_nav_v1" && !reviewNonzero(q.DividendCash) && (!reviewNonzero(q.SplitFactor) || splitFactorOne(q.SplitFactor))
			} else {
				comparable = comparable && !q.UnitPrice.Decimal().IsNegative() && performancePriceBasis(i.Type, q.PriceBasis) && (!reviewNonzero(q.SplitFactor) || splitFactorOne(q.SplitFactor))
			}
		}
		if !comparable {
			continue
		}
		period, annual := priceWindowPercent(first.UnitPrice.Decimal(), latest.UnitPrice.Decimal(), elapsed)
		item := QuotePerformance{WindowDays: window, BaselineOn: first.EffectiveDate, LatestOn: latest.EffectiveDate, ElapsedDays: elapsed, PeriodPercent: period}
		if i.Type == InstrumentBankInvestmentProduct {
			item.AnnualizedPercent = &annual
		}
		result = append(result, item)
	}
	return result
}

// Sources identify provenance, not economic units. Explicit raw-close bases from
// different providers are comparable; adjusted/unknown bases are not inferred.
func performanceSource(source QuoteSourceKind) bool {
	return source == QuoteSourceAgent || source == QuoteSourceManual || source == QuoteSourceProvider
}

// performanceTimestamp accepts the dated encodings shared by every type. Metals
// and crypto daily bars are labelled by a documented policy (a futures
// reference day or a UTC bar) or by observed publication, never a fake close.
func performanceTimestamp(t InstrumentType, basis string) bool {
	if basis == "date_label" || basis == "source_timestamp" || basis == "session_close" {
		return true
	}
	switch t {
	case InstrumentPreciousMetal:
		return basis == "policy_derived"
	case InstrumentCrypto:
		return basis == "policy_derived" || basis == "observed_publication"
	}
	return false
}

func splitFactorOne(text string) bool {
	value, err := decimal.NewFromString(text)
	return err == nil && value.Equal(decimal.NewFromInt(1))
}

// dailyPerformanceQuotes is the single daily selection rule behind every
// performance figure and the performance chart: dated daily closes only, with
// one observation per civil date chosen by source rank. Intraday and realtime
// facts never enter, so they cannot be aggregated or downsampled into a day.
func dailyPerformanceQuotes(quotes []InstrumentQuote) []InstrumentQuote {
	valid := make([]InstrumentQuote, 0, len(quotes))
	for _, q := range quotes {
		if _, err := time.Parse(time.DateOnly, q.EffectiveDate); err != nil {
			continue
		}
		if q.ObservationKind == "close" {
			valid = append(valid, q)
		}
	}
	return reviewDailyQuotes(valid)
}

// QuotePerformancePoint is one daily observation of the performance chart.
// BreakBefore marks a genuine price-basis discontinuity: the line must not be
// drawn between this point and the previous one.
type QuotePerformancePoint struct {
	Currency        CurrencyCode
	ObservationKind string
	EffectiveDate   string
	PriceBasis      string
	TimestampBasis  string
	QuotedAt        time.Time
	Value           string
	SourceKind      QuoteSourceKind
	SourceKey       string
	Delayed         bool
	BreakBefore     bool
}

// QuotePerformanceSeries is the chart and headline of one selected period. From
// and To are the dates the headline is calculated between, so the chart always
// covers exactly the interval of the figure, whatever the data density.
type QuotePerformanceSeries struct {
	WindowDays        int
	From              string
	To                string
	Item              *QuotePerformance
	UnavailableReason string
	Points            []QuotePerformancePoint
}

// ProjectQuotePerformanceSeries selects daily points with the same rules as the
// performance figures. The interval starts at the nearest observation on or
// before the lookback target (or at the first observation inside it when none
// exists) and ends at the latest daily observation, never at today.
func ProjectQuotePerformanceSeries(i Instrument, quotes []InstrumentQuote, window int) QuotePerformanceSeries {
	result := QuotePerformanceSeries{WindowDays: window, Points: []QuotePerformancePoint{}}
	mode, supported := quotePerformanceMode(i.Type)
	if !supported {
		result.UnavailableReason = "unsupported_instrument"
		return result
	}
	points := dailyPerformanceQuotes(quotes)
	if len(points) == 0 {
		result.UnavailableReason = "insufficient_daily_history"
		return result
	}
	latest := points[len(points)-1]
	start, covered := 0, true
	if window > 0 {
		anchor, _ := time.Parse(time.DateOnly, latest.EffectiveDate)
		target := anchor.AddDate(0, 0, -window).Format(time.DateOnly)
		base := sort.Search(len(points), func(n int) bool { return points[n].EffectiveDate > target }) - 1
		start, covered = max(base, 0), base >= 0
	}
	if items := ProjectQuotePerformanceWindows(i, quotes, []int{window}); len(items) > 0 {
		result.Item = &items[0]
	}
	switch {
	case result.Item != nil:
	case len(points) < 2 || !covered:
		result.UnavailableReason = "insufficient_daily_history"
	default:
		result.UnavailableReason = "incomparable_daily_prices"
	}
	selected := points[start:]
	result.From, result.To = selected[0].EffectiveDate, latest.EffectiveDate
	for n, q := range selected {
		point := QuotePerformancePoint{
			Currency: q.Currency, ObservationKind: q.ObservationKind, EffectiveDate: q.EffectiveDate,
			PriceBasis: q.PriceBasis, TimestampBasis: q.TimestampBasis, QuotedAt: q.QuotedAt,
			Value: q.UnitPrice.Canonical(), SourceKind: q.SourceKind, SourceKey: q.SourceKey, Delayed: q.Delayed,
		}
		point.BreakBefore = n > 0 && !sameDailyPriceUnit(i.Type, mode, selected[n-1].PriceBasis, q.PriceBasis)
		result.Points = append(result.Points, point)
	}
	return result
}

// sameDailyPriceUnit reports whether two daily price bases measure one unit.
// Identical bases always do; different raw-close bases do when the performance
// rules accept both, while NAV only ever compares the Agent unit-NAV basis.
func sameDailyPriceUnit(t InstrumentType, mode quotePerformanceKind, previous, current string) bool {
	if previous == current {
		return true
	}
	return mode == quotePerformancePrice && performancePriceBasis(t, previous) && performancePriceBasis(t, current)
}
