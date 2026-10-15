package domain

import (
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

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

func ProjectQuotePerformanceReport(i Instrument, quotes []InstrumentQuote, unsafeDates []string) QuotePerformanceReport {
	report := QuotePerformanceReport{Items: ProjectQuotePerformance(i, quotes, unsafeDates)}
	if len(report.Items) > 0 {
		return report
	}
	if i.Type != InstrumentMutualFund && i.Type != InstrumentBankInvestmentProduct && i.Type != InstrumentStock && i.Type != InstrumentETF {
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

func ProjectQuotePerformance(i Instrument, quotes []InstrumentQuote, unsafeDates []string) []QuotePerformance {
	result := []QuotePerformance{}
	nav := i.Type == InstrumentMutualFund || i.Type == InstrumentBankInvestmentProduct
	stock := i.Type == InstrumentStock || i.Type == InstrumentETF
	if !nav && !stock {
		return result
	}
	valid := []InstrumentQuote{}
	for _, q := range quotes {
		if _, err := time.Parse(time.DateOnly, q.EffectiveDate); err != nil {
			continue
		}
		if q.ObservationKind == "close" {
			valid = append(valid, q)
		}
	}
	points := reviewDailyQuotes(valid)
	if len(points) < 2 {
		return result
	}
	latest := points[len(points)-1]
	anchor, _ := time.Parse(time.DateOnly, latest.EffectiveDate)
	for _, window := range []int{7, 30} {
		target := anchor.AddDate(0, 0, -window).Format(time.DateOnly)
		base := sort.Search(len(points), func(n int) bool { return points[n].EffectiveDate > target }) - 1
		if base < 0 {
			continue
		}
		first := points[base]
		elapsed := reviewDays(first.EffectiveDate, latest.EffectiveDate)
		comparable := elapsed > 0 && first.UnitPrice.Decimal().IsPositive()
		for _, q := range points[base:] {
			comparable = comparable && q.Currency == i.QuoteCurrency && q.BindingRevision == i.ProviderBindingRevision && performanceSource(q.SourceKind) && performanceTimestamp(q.TimestampBasis)
			if nav {
				comparable = comparable && q.UnitPrice.Decimal().IsPositive() && q.PriceBasis == "agent_unit_nav_v1" && !reviewNonzero(q.DividendCash) && (!reviewNonzero(q.SplitFactor) || splitFactorOne(q.SplitFactor))
			}
			if stock {
				comparable = comparable && !q.UnitPrice.Decimal().IsNegative() && (q.PriceBasis == TiingoRawClosePriceBasis || q.PriceBasis == YahooRawClosePriceBasis || q.PriceBasis == "agent_raw_close_v1") && (!reviewNonzero(q.SplitFactor) || splitFactorOne(q.SplitFactor))
			}
		}
		for _, date := range unsafeDates {
			if date >= first.EffectiveDate && date <= latest.EffectiveDate {
				comparable = false
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

func performanceTimestamp(basis string) bool {
	return basis == "date_label" || basis == "source_timestamp" || basis == "session_close"
}

func splitFactorOne(text string) bool {
	value, err := decimal.NewFromString(text)
	return err == nil && value.Equal(decimal.NewFromInt(1))
}
