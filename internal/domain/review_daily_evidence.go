package domain

import (
	"slices"
	"time"

	"github.com/shopspring/decimal"
)

// ReviewLatestClosedDay uses the rule's fixed timezone. Advancing the day does
// not depend on whether the market has published a quote for that day.
func ReviewLatestClosedDay(now time.Time, timezone string) (string, error) {
	if now.IsZero() {
		return "", validation("now", "an evaluation time is required")
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return "", validation("timezone", "must be an available IANA timezone")
	}
	local := now.In(location)
	label := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	return label.AddDate(0, 0, -1).Format(time.DateOnly), nil
}

// ReviewCivilCutoff returns the exclusive end of the local day. A ledger
// event exactly at cutoff belongs to the next day, including DST boundaries.
func ReviewCivilCutoff(on, timezone string) (time.Time, error) {
	day, err := time.Parse(time.DateOnly, on)
	if err != nil {
		return time.Time{}, validation("on", "must be a valid ISO civil date")
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, validation("timezone", "must be an available IANA timezone")
	}
	last, err := ResolveCivilDateCutoff(day, location)
	if err != nil {
		return time.Time{}, err
	}
	return last.Add(time.Millisecond), nil
}

// ResolveReviewDailyPerformance shares the descriptive performance selection
// and arithmetic. Alerts add an exact endpoint-day and baseline-gap admission
// rule; they never fill a missing close with an earlier observation.
func ResolveReviewDailyPerformance(instrument Instrument, quotes []InstrumentQuote, on string, lookbackDays, maxBaselineGapDays int) (*QuotePerformance, string) {
	if _, err := time.Parse(time.DateOnly, on); err != nil || !ValidQuotePeriodWindow(lookbackDays) || lookbackDays == 0 || maxBaselineGapDays < 0 {
		return nil, "invalid_request"
	}
	prior := make([]InstrumentQuote, 0, len(quotes))
	for _, q := range quotes {
		if q.InstrumentID == instrument.ID && q.EffectiveDate <= on {
			prior = append(prior, q)
		}
	}
	points := dailyPerformanceQuotes(prior)
	if len(points) == 0 || points[len(points)-1].EffectiveDate != on {
		return nil, "daily_close_missing"
	}
	items := ProjectQuotePerformanceWindows(instrument, prior, []int{lookbackDays})
	if len(items) == 0 {
		return nil, "baseline_missing_or_incomparable"
	}
	item := items[0]
	end, _ := time.Parse(time.DateOnly, on)
	target := end.AddDate(0, 0, -lookbackDays).Format(time.DateOnly)
	gap, err := CivilDaysBetween(item.BaselineOn, target)
	if err != nil || gap < 0 || gap > maxBaselineGapDays {
		return nil, "baseline_too_old"
	}
	return &item, ""
}

// ResolveReviewDailyClose chooses the same one-per-date price as the
// descriptive series. A balance threshold requires a real eligible close
// labelled D; an older market day is never carried into D.
func ResolveReviewDailyClose(instrument Instrument, quotes []InstrumentQuote, on string) (*InstrumentQuote, string) {
	if _, err := time.Parse(time.DateOnly, on); err != nil {
		return nil, "invalid_request"
	}
	filtered := make([]InstrumentQuote, 0, len(quotes))
	for _, quote := range quotes {
		if quote.InstrumentID == instrument.ID && quote.EffectiveDate <= on {
			filtered = append(filtered, quote)
		}
	}
	points := dailyPerformanceQuotes(filtered)
	if len(points) == 0 || points[len(points)-1].EffectiveDate != on {
		return nil, "daily_close_missing"
	}
	quote := points[len(points)-1]
	mode, supported := quotePerformanceMode(instrument.Type)
	if !supported || quote.Currency != instrument.QuoteCurrency || quote.BindingRevision != instrument.ProviderBindingRevision || !performanceSource(quote.SourceKind) || !performanceTimestamp(instrument.Type, quote.TimestampBasis) {
		return nil, "daily_close_ineligible"
	}
	if mode == quotePerformanceNAV {
		if !quote.UnitPrice.Decimal().IsPositive() || quote.PriceBasis != "agent_unit_nav_v1" || reviewNonzero(quote.DividendCash) || (reviewNonzero(quote.SplitFactor) && !splitFactorOne(quote.SplitFactor)) {
			return nil, "daily_close_ineligible"
		}
	} else if quote.UnitPrice.Decimal().IsNegative() || !performancePriceBasis(instrument.Type, quote.PriceBasis) || (reviewNonzero(quote.SplitFactor) && !splitFactorOne(quote.SplitFactor)) {
		return nil, "daily_close_ineligible"
	}
	return &quote, ""
}

// ReviewSelectedFX is the already-authoritatively-selected source result.
// The caller may not fall back to a different source merely because this
// selection is too old. EconomicKind is assigned by a trusted source policy;
// fetchedAt and quotedAt never create an effective date or time.
type ReviewSelectedFX struct {
	Quote        FXQuote
	EconomicKind string // daily_reference or timestamped
}

type ReviewFXAdmission struct {
	Known         bool
	Rate          string
	QuoteID       FXQuoteID
	Direction     FXMappingStatus
	EffectiveDate string
	EffectiveAt   time.Time
	AgeDays       int
	Reason        string
}

func AdmitReviewFX(native, comparison CurrencyCode, selected *ReviewSelectedFX, on, timezone string, policy ReviewRuleDataPolicy) (ReviewFXAdmission, error) {
	cutoff, err := ReviewCivilCutoff(on, timezone)
	if err != nil {
		return ReviewFXAdmission{}, err
	}
	if _, err := ParseSupportedCurrency(string(native)); err != nil {
		return ReviewFXAdmission{}, err
	}
	if _, err := ParseSupportedCurrency(string(comparison)); err != nil {
		return ReviewFXAdmission{}, err
	}
	if native == comparison {
		return ReviewFXAdmission{Known: true, Rate: "1", Direction: FXMappingIdentity}, nil
	}
	if selected == nil {
		return ReviewFXAdmission{Reason: "fx_missing"}, nil
	}
	q := selected.Quote
	result := ReviewFXAdmission{QuoteID: q.ID}
	switch selected.EconomicKind {
	case "daily_reference":
		if q.ObservationKind != "close" && q.ObservationKind != "daily_reference" {
			result.Reason = "fx_date_untrusted"
			return result, nil
		}
		if _, err := time.Parse(time.DateOnly, q.EffectiveDate); err != nil || q.EffectiveDate > on {
			result.Reason = "fx_date_untrusted"
			return result, nil
		}
		age, err := CivilDaysBetween(q.EffectiveDate, on)
		if err != nil || age < 0 || age > policy.FXReferenceMaxAgeDays {
			result.Reason = "fx_stale"
			return result, nil
		}
		result.EffectiveDate, result.AgeDays = q.EffectiveDate, age
	case "timestamped":
		if q.ValueEffectiveAt.IsZero() || !q.ValueEffectiveAt.Before(cutoff) {
			result.Reason = "fx_time_untrusted"
			return result, nil
		}
		if cutoff.Sub(q.ValueEffectiveAt) > time.Duration(policy.FXTimestampMaxAgeHours)*time.Hour {
			result.Reason = "fx_stale"
			return result, nil
		}
		result.EffectiveAt = q.ValueEffectiveAt
	default:
		result.Reason = "fx_date_untrusted"
		return result, nil
	}
	mapping, err := MapFXRate(native, comparison, q.BaseCurrency, q.QuoteCurrency, q.Rate.Canonical())
	if err != nil {
		return ReviewFXAdmission{}, err
	}
	if mapping.Status == FXMappingUnsupported {
		result.Reason = mapping.Reason
		return result, nil
	}
	result.Known, result.Rate, result.Direction = true, mapping.Rate.Canonical(), mapping.Status
	return result, nil
}

// ReviewDailyInput is a single economic-day projection, built only after
// authoritative replay proves the quantity, type, amount, and cost through
// that day's cutoff. Nil fields mean the corresponding historical fact is
// unknown; a known zero is represented by a non-nil "0". FX must already be
// selected by the existing source preference before admission above.
type ReviewDailyInput struct {
	On             string
	InstrumentID   InstrumentID
	Quantity       *string
	InstrumentType *InstrumentType
	NativeCurrency *CurrencyCode
	AmountExact    *string
	CostExact      *string
	Performance    map[int]QuotePerformance
	SelectedFX     map[CurrencyCode]ReviewSelectedFX
}

type ReviewDailyLeafEvidence struct {
	Metric      string             `json:"metric"`
	Truth       ReviewTruth        `json:"truth"`
	Actual      string             `json:"actual,omitempty"`
	Threshold   string             `json:"threshold,omitempty"`
	Currency    CurrencyCode       `json:"currency,omitempty"`
	Reason      string             `json:"reason,omitempty"`
	EffectiveOn string             `json:"effectiveOn,omitempty"`
	BaselineOn  string             `json:"baselineOn,omitempty"`
	FX          *ReviewFXAdmission `json:"fx,omitempty"`
}

type ReviewDailyResult struct {
	On     string                    `json:"on"`
	Truth  ReviewTruth               `json:"truth"`
	Leaves []ReviewDailyLeafEvidence `json:"leaves"`
}

func EvaluateReviewDaily(spec ReviewRuleSpec, input ReviewDailyInput) (ReviewDailyResult, error) {
	spec, err := spec.Normalize()
	if err != nil {
		return ReviewDailyResult{}, err
	}
	if _, err := time.Parse(time.DateOnly, input.On); err != nil {
		return ReviewDailyResult{}, validation("on", "must be a valid ISO civil date")
	}
	result := ReviewDailyResult{On: input.On, Truth: ReviewUnknown, Leaves: []ReviewDailyLeafEvidence{}}
	if spec.Target.Kind == "instruments" && !slices.Contains(spec.Target.InstrumentIDs, input.InstrumentID) {
		result.Truth = ReviewFalse
		return result, nil
	}
	quantity := ReviewUnknown
	if input.Quantity != nil {
		q, err := ParseQuantity(*input.Quantity)
		if err != nil {
			return ReviewDailyResult{}, err
		}
		quantity = ReviewFalse
		if q.Decimal().IsPositive() {
			quantity = ReviewTrue
		}
	}
	// The outer AND gate cannot be bypassed by a true branch in an OR group.
	if quantity == ReviewFalse {
		result.Truth = ReviewFalse
		return result, nil
	}
	predicate, err := evaluateReviewNode(spec.Predicate, input, spec, &result.Leaves)
	if err != nil {
		return ReviewDailyResult{}, err
	}
	result.Truth = ReviewAll(quantity, predicate)
	return result, nil
}

func evaluateReviewNode(node ReviewPredicateNode, input ReviewDailyInput, spec ReviewRuleSpec, evidence *[]ReviewDailyLeafEvidence) (ReviewTruth, error) {
	if node.Leaf != nil {
		item, err := evaluateReviewLeaf(*node.Leaf, input, spec)
		if err != nil {
			return ReviewUnknown, err
		}
		*evidence = append(*evidence, item)
		return item.Truth, nil
	}
	children, all := node.Any, false
	if node.All != nil {
		children, all = node.All, true
	}
	values := make([]ReviewTruth, 0, len(children))
	for _, child := range children {
		truth, err := evaluateReviewNode(child, input, spec, evidence)
		if err != nil {
			return ReviewUnknown, err
		}
		values = append(values, truth)
	}
	if all {
		return ReviewAll(values...), nil
	}
	return ReviewAny(values...), nil
}

func evaluateReviewLeaf(leaf ReviewRuleLeaf, input ReviewDailyInput, spec ReviewRuleSpec) (ReviewDailyLeafEvidence, error) {
	item := ReviewDailyLeafEvidence{Metric: leaf.Metric, Truth: ReviewUnknown, Threshold: leaf.Value, Currency: leaf.Currency, EffectiveOn: input.On}
	switch leaf.Metric {
	case "instrument":
		contains := slices.Contains(leaf.InstrumentIDs, input.InstrumentID)
		if leaf.Operator == "not_in" {
			contains = !contains
		}
		item.Truth = reviewBool(contains)
	case "type":
		if input.InstrumentType == nil {
			item.Reason = "historical_type_unknown"
			break
		}
		item.Actual = string(*input.InstrumentType)
		item.Truth = reviewBool(slices.Contains(leaf.Types, *input.InstrumentType))
	case "amount", "cost":
		value := input.AmountExact
		if leaf.Metric == "cost" {
			value = input.CostExact
		}
		if value == nil || input.NativeCurrency == nil {
			item.Reason = "historical_amount_unknown"
			break
		}
		exact, err := decimal.NewFromString(*value)
		if err != nil || exact.IsNegative() {
			return item, validation(leaf.Metric, "historical monetary input is invalid")
		}
		if leaf.FXMode == "native" {
			if *input.NativeCurrency != leaf.Currency {
				item.Reason = "currency_mismatch"
				break
			}
		} else {
			var selected *ReviewSelectedFX
			if q, ok := input.SelectedFX[leaf.Currency]; ok {
				selected = &q
			}
			fx, err := AdmitReviewFX(*input.NativeCurrency, leaf.Currency, selected, input.On, spec.Schedule.Timezone, spec.DataPolicy)
			if err != nil {
				return item, err
			}
			item.FX = &fx
			if !fx.Known {
				item.Reason = fx.Reason
				break
			}
			rate, _ := decimal.NewFromString(fx.Rate)
			exact = exact.Mul(rate)
		}
		money, err := NewMoney(exact, leaf.Currency)
		if err != nil {
			return item, err
		}
		comparison := money.Amount()
		item.Actual = comparison.StringFixed(4)
		threshold, _ := decimal.NewFromString(leaf.Value)
		item.Truth = reviewBool(reviewCompare(comparison, threshold, leaf.Operator))
	case "price_change", "annualized":
		if leaf.Metric == "annualized" && (input.InstrumentType == nil || *input.InstrumentType != InstrumentBankInvestmentProduct) {
			item.Reason = "unsupported_instrument"
			break
		}
		performance, ok := input.Performance[leaf.LookbackDays]
		if !ok || performance.LatestOn != input.On || performance.WindowDays != leaf.LookbackDays || performance.ElapsedDays <= 0 {
			item.Reason = "daily_performance_unknown"
			break
		}
		endpoint, err := time.Parse(time.DateOnly, input.On)
		if err != nil {
			return item, err
		}
		target := endpoint.AddDate(0, 0, -leaf.LookbackDays).Format(time.DateOnly)
		gap, err := CivilDaysBetween(performance.BaselineOn, target)
		if err != nil || gap < 0 || gap > spec.DataPolicy.BaselineMaxGapDays {
			item.Reason = "baseline_too_old"
			break
		}
		item.BaselineOn = performance.BaselineOn
		value := performance.PeriodPercent
		if leaf.Metric == "annualized" {
			if performance.AnnualizedPercent == nil {
				item.Reason = "annualized_unavailable"
				break
			}
			value = *performance.AnnualizedPercent
		}
		actual, err := decimal.NewFromString(value)
		if err != nil {
			return item, validation(leaf.Metric, "invalid performance evidence")
		}
		item.Actual = actual.String()
		threshold, _ := decimal.NewFromString(leaf.Value)
		item.Truth = reviewBool(reviewCompare(actual, threshold, leaf.Operator))
	}
	return item, nil
}

func reviewBool(value bool) ReviewTruth {
	if value {
		return ReviewTrue
	}
	return ReviewFalse
}
func reviewCompare(actual, threshold decimal.Decimal, op string) bool {
	switch op {
	case ">":
		return actual.GreaterThan(threshold)
	case ">=":
		return actual.GreaterThanOrEqual(threshold)
	case "<":
		return actual.LessThan(threshold)
	default:
		return actual.LessThanOrEqual(threshold)
	}
}
