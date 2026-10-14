package domain

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// InvestmentReviewPolicy thresholds are exact percentages, not ratios.
type InvestmentReviewPolicy struct {
	Enabled              bool   `json:"enabled"`
	WindowDays           int    `json:"windowDays"`
	LowAnnualizedPercent string `json:"lowAnnualizedPercent"`
	LossPercent          string `json:"lossPercent"`
	SustainDays          int    `json:"sustainDays"`
	MaxAgeDays           int    `json:"maxAgeDays"`
}

func DefaultInvestmentReviewPolicy() InvestmentReviewPolicy {
	return InvestmentReviewPolicy{true, 30, "1", "-1", 7, 7}
}

var reviewThresholdSyntax = regexp.MustCompile(`^-?(0|[1-9][0-9]?)(\.[0-9]{1,8})?$`)

func (p InvestmentReviewPolicy) Validate() error {
	low, e1 := decimal.NewFromString(p.LowAnnualizedPercent)
	loss, e2 := decimal.NewFromString(p.LossPercent)
	if !reviewThresholdSyntax.MatchString(p.LowAnnualizedPercent) || !reviewThresholdSyntax.MatchString(p.LossPercent) || (p.WindowDays != 7 && p.WindowDays != 30) || p.SustainDays < 2 || p.SustainDays > 30 || p.MaxAgeDays < 1 || p.MaxAgeDays > 30 || e1 != nil || e2 != nil || low.IsNegative() || low.GreaterThan(decimal.NewFromInt(10)) || !loss.IsNegative() || loss.LessThan(decimal.NewFromInt(-50)) {
		return validation("investmentReviewPolicy", "use a 7/30-day window, 2–30 sustained days, 1–30 age days, low threshold 0–10%, and loss threshold -50% to below 0%")
	}
	return nil
}

type InvestmentReviewSnapshot struct {
	Gain GainSnapshot
}
type InvestmentReviewItem struct {
	// EpisodeAliases are internal identities whose anchors still belong to this
	// breach run. Earlier coverage may move its first breach without ending it.
	EpisodeAliases    []string `json:"-"`
	Snoozed           bool     `json:"snoozed"`
	SnoozedUntilOn    string   `json:"snoozedUntilOn"`
	Monitored         bool     `json:"monitored"`
	InstrumentID      string   `json:"instrumentId"`
	DisplayName       string   `json:"displayName"`
	Status            string   `json:"status"`
	BaselineOn        string   `json:"baselineOn"`
	LatestOn          string   `json:"latestOn"`
	TargetOn          string   `json:"targetOn"`
	ElapsedDays       int      `json:"elapsedDays"`
	AgeDays           int      `json:"ageDays"`
	MissingDays       int      `json:"missingDays"`
	ObservationCount  int      `json:"observationCount"`
	PeriodPercent     *string  `json:"periodPercent"`
	AnnualizedPercent *string  `json:"annualizedPercent"`
	EpisodeID         string   `json:"episodeId"`
	Reason            string   `json:"reason"`
	Acknowledged      bool     `json:"acknowledged"`
}

// ProjectInvestmentReviews measures unit NAV price change, never a holding's
// balance or cash flows. Unsupported economic adjustments fail closed. Dates
// are civil source labels, so DST never changes the elapsed-day denominator.
func ProjectInvestmentReviews(input InvestmentReviewSnapshot, p InvestmentReviewPolicy, today string, priorPolicies ...InvestmentReviewPolicy) []InvestmentReviewItem {
	snapshot := input.Gain.Portfolio
	unsafeDates := reviewUnsafeDates(input.Gain)
	lowThreshold := decimal.RequireFromString(p.LowAnnualizedPercent)
	lossThreshold := decimal.RequireFromString(p.LossPercent)
	accounts := map[AccountID]bool{}
	for _, a := range snapshot.Accounts {
		accounts[a.Account.ID] = a.Account.ArchivedAt == nil && a.Account.TrackingMode == TrackingHoldings
	}
	held := map[InstrumentID]bool{}
	for _, h := range snapshot.Holdings {
		if h.ArchivedAt == nil && !h.Quantity.IsZero() && accounts[h.AccountID] {
			held[h.InstrumentID] = true
		}
	}
	quotes := map[InstrumentID][]InstrumentQuote{}
	for _, q := range snapshot.InstrumentQuotes {
		if q.EffectiveDate != "" && q.ObservationKind == "close" && q.EffectiveDate <= today {
			quotes[q.InstrumentID] = append(quotes[q.InstrumentID], q)
		}
	}
	result := []InvestmentReviewItem{}
	for _, i := range snapshot.Instruments {
		if !held[i.ID] || i.ArchivedAt != nil || (i.Type != InstrumentMutualFund && i.Type != InstrumentBankInvestmentProduct) {
			continue
		}
		points := reviewDailyQuotes(quotes[i.ID])
		profiles := append([]InvestmentReviewPolicy{p}, priorPolicies...)
		item := InvestmentReviewItem{InstrumentID: i.ID.String(), DisplayName: i.DisplayLabel(), Status: "unavailable", Reason: "unit_nav_required"}
		if len(points) > 0 {
			var firstBreach, episodeStart string
			var previousBreach string
			var episodeAnchors []string
			for index := range points {
				current := reviewWindow(unsafeDates[i.ID], i, points, index, p, today)
				if current.Status == "unavailable" {
					firstBreach, previousBreach = "", ""
					if episodeStart == "" {
						episodeAnchors = nil
					}
				} else {
					period := decimal.RequireFromString(*current.PeriodPercent)
					annual := decimal.RequireFromString(*current.AnnualizedPercent)
					breach := period.LessThanOrEqual(lossThreshold) || annual.LessThan(lowThreshold)
					if !breach {
						firstBreach, previousBreach, episodeStart = "", "", ""
						episodeAnchors = nil
					} else {
						if previousBreach != "" && reviewDays(previousBreach, current.LatestOn) > 7 {
							firstBreach, previousBreach = "", ""
							if episodeStart == "" {
								episodeAnchors = nil
							}
						}
						if firstBreach == "" {
							firstBreach = current.LatestOn
						}
						// Two distinct endpoints, separated by the configured confirmation time.
						if episodeStart == "" && previousBreach != "" && reviewDays(firstBreach, current.LatestOn) >= p.SustainDays {
							episodeStart = firstBreach
						}
						previousBreach = current.LatestOn
						episodeAnchors = append(episodeAnchors, current.LatestOn)
						if episodeStart != "" && current.AgeDays <= p.MaxAgeDays && p.Enabled {
							current.Status = "low_yield"
							if period.LessThanOrEqual(lossThreshold) {
								current.Status = "loss"
							}
							// Identity follows the observed run, so preferences and
							// refresh timestamps do not independently create an episode.
							current.EpisodeID = reviewEpisodeID(snapshot.Household.ID, i.ID, episodeStart)
						} else if breach {
							current.Status = "confirming"
						}
					}
					if current.AgeDays > p.MaxAgeDays {
						current.Status = "stale"
					}
				}
				item = current
			}
			if item.EpisodeID != "" {
				for _, anchor := range episodeAnchors {
					item.EpisodeAliases = append(item.EpisodeAliases, reviewEpisodeID(snapshot.Household.ID, i.ID, anchor))
					// Read initial-MVP acknowledgements without rewriting configuration.
					for _, profile := range profiles {
						item.EpisodeAliases = append(item.EpisodeAliases, legacyReviewEpisodeID(snapshot.Household.ID, i.ID, profile, anchor))
					}
				}
			}
		}
		result = append(result, item)
	}
	sort.Slice(result, func(a, b int) bool {
		if result[a].DisplayName != result[b].DisplayName {
			return result[a].DisplayName < result[b].DisplayName
		}
		return result[a].InstrumentID < result[b].InstrumentID
	})
	return result
}
func reviewDailyQuotes(quotes []InstrumentQuote) []InstrumentQuote {
	byDate := map[string]InstrumentQuote{}
	for _, q := range quotes {
		old, exists := byDate[q.EffectiveDate]
		if !exists || reviewSourceRank(q.SourceKind) > reviewSourceRank(old.SourceKind) || (q.SourceKind == old.SourceKind && (q.Revision > old.Revision || (q.Revision == old.Revision && (q.CreatedAt.After(old.CreatedAt) || (q.CreatedAt.Equal(old.CreatedAt) && q.ID > old.ID))))) {
			byDate[q.EffectiveDate] = q
		}
	}
	points := make([]InstrumentQuote, 0, len(byDate))
	for _, q := range byDate {
		points = append(points, q)
	}
	sort.Slice(points, func(a, b int) bool { return points[a].EffectiveDate < points[b].EffectiveDate })
	return points
}
func reviewDays(from, to string) int {
	a, e1 := time.Parse(time.DateOnly, from)
	b, e2 := time.Parse(time.DateOnly, to)
	if e1 != nil || e2 != nil {
		return -1
	}
	return int(b.Sub(a) / (24 * time.Hour))
}
func reviewWindow(unsafeDates []string, i Instrument, points []InstrumentQuote, end int, p InvestmentReviewPolicy, today string) InvestmentReviewItem {
	latest := points[end]
	r := InvestmentReviewItem{InstrumentID: i.ID.String(), DisplayName: i.DisplayLabel(), Status: "unavailable", Reason: "missing_baseline", LatestOn: latest.EffectiveDate, AgeDays: reviewDays(latest.EffectiveDate, today)}
	anchor, err := time.Parse(time.DateOnly, latest.EffectiveDate)
	if err != nil {
		r.Reason = "unit_nav_required"
		return r
	}
	r.TargetOn = anchor.AddDate(0, 0, -p.WindowDays).Format(time.DateOnly)
	// Choose the closest real observation on or before the target; never fill.
	base := sort.Search(end+1, func(n int) bool { return points[n].EffectiveDate > r.TargetOn }) - 1
	if base < 0 {
		return r
	}
	r.BaselineOn = points[base].EffectiveDate
	r.ElapsedDays = reviewDays(r.BaselineOn, r.LatestOn)
	r.ObservationCount = end - base + 1
	r.MissingDays = r.ElapsedDays + 1 - r.ObservationCount
	if reviewDays(r.BaselineOn, r.TargetOn) > 7 {
		r.Reason = "baseline_too_old"
		return r
	}
	for _, q := range points[base : end+1] {
		if q.SourceKind != QuoteSourceAgent || q.PriceBasis != "agent_unit_nav_v1" || (q.TimestampBasis != "date_label" && q.TimestampBasis != "source_timestamp") || q.Currency != i.QuoteCurrency || q.BindingRevision != i.ProviderBindingRevision || !q.UnitPrice.Decimal().IsPositive() || reviewDays(q.EffectiveDate, q.EffectiveDate) < 0 {
			r.Reason = "unit_nav_required"
			return r
		}
		if reviewNonzero(q.SplitFactor) || reviewNonzero(q.DividendCash) {
			r.Reason = "distribution_or_unit_change"
			return r
		}
	}
	blocked := sort.SearchStrings(unsafeDates, r.BaselineOn)
	if blocked < len(unsafeDates) && unsafeDates[blocked] <= r.LatestOn {
		r.Reason = "distribution_or_unit_change"
		return r
	}

	if r.ElapsedDays <= 0 || r.AgeDays < 0 || r.MissingDays < 0 {
		r.Reason = "unit_nav_required"
		return r
	}
	// Simple annualization, not compounding or a forward yield projection.
	periodText, annualText := priceWindowPercent(points[base].UnitPrice.Decimal(), latest.UnitPrice.Decimal(), r.ElapsedDays)
	r.PeriodPercent, r.AnnualizedPercent = &periodText, &annualText
	r.Status, r.Reason = "healthy", ""
	return r
}
func reviewNonzero(text string) bool {
	if text == "" {
		return false
	}
	v, e := decimal.NewFromString(text)
	return e != nil || !v.IsZero()
}

func reviewSourceRank(source QuoteSourceKind) int {
	switch source {
	case QuoteSourceAgent:
		return 3
	case QuoteSourceManual:
		return 2
	default:
		return 1
	}
}

// Canonical normalizes numerically identical preferences to one episode context.
// Call only after Validate succeeds.
func (p InvestmentReviewPolicy) Canonical() InvestmentReviewPolicy {
	p.LowAnnualizedPercent = decimal.RequireFromString(p.LowAnnualizedPercent).String()
	p.LossPercent = decimal.RequireFromString(p.LossPercent).String()
	return p
}

// Index once per capture so repeated rolling windows do not replay or scan the
// household's entire transaction history per NAV endpoint.
func reviewUnsafeDates(gain GainSnapshot) map[InstrumentID][]string {
	dates := map[InstrumentID][]string{}
	location := time.UTC
	if gain.Portfolio.Origin != nil {
		if l, e := time.LoadLocation(gain.Portfolio.Origin.Timezone); e == nil {
			location = l
		}
	}
	for _, d := range gain.Dividends {
		if d.DividendDetail == nil {
			continue
		}
		date := d.EffectiveLocalDate
		if date == "" {
			date = d.EffectiveAt.In(location).Format(time.DateOnly)
		}
		dates[d.DividendDetail.InstrumentID] = append(dates[d.DividendDetail.InstrumentID], date)
	}
	// Trades and transfers change positions, not the unit NAV series; trade fees
	// are excluded. Quantity adjustments can represent unsupported unit changes.
	for _, h := range gain.Portfolio.Holdings {
		for _, e := range gain.CostEvents[h.ID] {
			if e.Kind == CostBasisAdjustmentIn || e.Kind == CostBasisAdjustmentOut {
				dates[h.InstrumentID] = append(dates[h.InstrumentID], e.EffectiveAt.In(location).Format(time.DateOnly))
			}
		}
	}
	for id := range dates {
		sort.Strings(dates[id])
	}
	return dates
}

func legacyReviewEpisodeID(household HouseholdID, instrument InstrumentID, p InvestmentReviewPolicy, anchor string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("investment-review/1|%s|%s|%+v|%s", household, instrument, p, anchor))))
}

// Current episode identity follows evidence runs, independently of preferences.
func reviewEpisodeID(household HouseholdID, instrument InstrumentID, anchor string) string {
	return "ir2/" + fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("investment-review/2|%s|%s|%s", household, instrument, anchor))))
}
