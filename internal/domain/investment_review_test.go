package domain

import (
	"github.com/shopspring/decimal"
	"reflect"
	"testing"
	"time"
)

func reviewFixture(t *testing.T) (InvestmentReviewSnapshot, Instrument) {
	t.Helper()
	household := &Household{ID: NewHouseholdID()}
	instrument := Instrument{ID: NewInstrumentID(), HouseholdID: household.ID, Name: "Synthetic fund", Type: InstrumentMutualFund, QuoteCurrency: "USD", QuoteSource: QuoteSourceAgent}
	account := Account{ID: NewAccountID(), TrackingMode: TrackingHoldings}
	quantity, _ := ParseQuantity("10")
	return InvestmentReviewSnapshot{Gain: GainSnapshot{Portfolio: PortfolioSnapshot{Household: household, Instruments: []Instrument{instrument}, Accounts: []AccountRecord{{Account: account}}, Holdings: []Holding{{ID: NewHoldingID(), AccountID: account.ID, InstrumentID: instrument.ID, Quantity: quantity}}}, CostEvents: map[HoldingID][]CostBasisEvent{}}}, instrument
}
func reviewQuote(t *testing.T, i Instrument, date, value string) InstrumentQuote {
	t.Helper()
	price, e := ParseUnitPrice(value)
	if e != nil {
		t.Fatal(e)
	}
	when, e := time.Parse(time.DateOnly, date)
	if e != nil {
		t.Fatal(e)
	}
	return InstrumentQuote{ID: NewInstrumentQuoteID(), InstrumentID: i.ID, UnitPrice: price, Currency: i.QuoteCurrency, SourceKind: QuoteSourceAgent, SourceKey: "agent", QuotedAt: when, CreatedAt: when, ValueEffectiveAt: when, EffectiveDate: date, ObservationKind: "close", PriceBasis: "agent_unit_nav_v1", TimestampBasis: "date_label"}
}
func onlyReview(t *testing.T, s InvestmentReviewSnapshot, p InvestmentReviewPolicy, today string) InvestmentReviewItem {
	t.Helper()
	rows := ProjectInvestmentReviews(s, p, today)
	if len(rows) != 1 {
		t.Fatalf("rows = %+v", rows)
	}
	return rows[0]
}
func TestInvestmentReviewActualDaysAndLatestObservation(t *testing.T) {
	for _, tc := range []struct {
		base, latest, today  string
		window, elapsed, age int
		end                  string
		annual               string
	}{
		{"2026-09-07", "2026-10-09", "2026-10-13", 30, 32, 4, "1.032", "36.5"},
		{"2026-03-01", "2026-03-31", "2026-04-03", 30, 30, 3, "1.003", "3.65"},
		{"2024-02-01", "2024-03-02", "2024-03-02", 30, 30, 0, "1.003", "3.65"},
		{"2026-09-30", "2026-10-09", "2026-10-13", 7, 9, 4, "1.009", "36.5"},
	} {
		t.Run(tc.base+tc.latest, func(t *testing.T) {
			s, i := reviewFixture(t)
			s.Gain.Portfolio.InstrumentQuotes = []InstrumentQuote{reviewQuote(t, i, tc.base, "1"), reviewQuote(t, i, tc.latest, tc.end)}
			p := DefaultInvestmentReviewPolicy()
			p.WindowDays = tc.window
			r := onlyReview(t, s, p, tc.today)
			if r.LatestOn != tc.latest || r.BaselineOn != tc.base || r.ElapsedDays != tc.elapsed || r.AgeDays != tc.age || r.AnnualizedPercent == nil || *r.AnnualizedPercent != tc.annual || r.ObservationCount != 2 || r.MissingDays != tc.elapsed-1 {
				t.Fatalf("actual dates/span = %+v", r)
			}
		})
	}
}
func TestInvestmentReviewBaselineAndAgeBounds(t *testing.T) {
	for _, tc := range []struct{ base, today, reason, status string }{
		{"2026-09-10", "2026-10-09", "missing_baseline", "unavailable"},
		{"2026-09-02", "2026-10-09", "", "confirming"},
		{"2026-09-01", "2026-10-09", "baseline_too_old", "unavailable"},
		{"2026-09-09", "2026-10-08", "missing_baseline", "unavailable"},
		{"2026-09-09", "2026-10-16", "", "confirming"},
		{"2026-09-09", "2026-10-17", "", "stale"},
	} {
		t.Run(tc.base+tc.today, func(t *testing.T) {
			s, i := reviewFixture(t)
			s.Gain.Portfolio.InstrumentQuotes = []InstrumentQuote{reviewQuote(t, i, tc.base, "1"), reviewQuote(t, i, "2026-10-09", "1")}
			r := onlyReview(t, s, DefaultInvestmentReviewPolicy(), tc.today)
			if r.Status != tc.status || r.Reason != tc.reason {
				t.Fatalf("got %+v want %s/%s", r, tc.status, tc.reason)
			}
			if r.Status == "unavailable" && r.PeriodPercent != nil {
				t.Fatalf("unsupported return %+v", r)
			}
		})
	}
}
func TestInvestmentReviewUnsafeWindows(t *testing.T) {
	cases := map[string]func(*InvestmentReviewSnapshot){
		"currency":  func(s *InvestmentReviewSnapshot) { s.Gain.Portfolio.InstrumentQuotes[1].Currency = "EUR" },
		"binding":   func(s *InvestmentReviewSnapshot) { s.Gain.Portfolio.InstrumentQuotes[1].BindingRevision = 1 },
		"basis":     func(s *InvestmentReviewSnapshot) { s.Gain.Portfolio.InstrumentQuotes[1].PriceBasis = "cumulative_nav" },
		"manual":    func(s *InvestmentReviewSnapshot) { s.Gain.Portfolio.InstrumentQuotes[1].SourceKind = QuoteSourceManual },
		"timestamp": func(s *InvestmentReviewSnapshot) { s.Gain.Portfolio.InstrumentQuotes[1].TimestampBasis = "unknown" },
		"zero": func(s *InvestmentReviewSnapshot) {
			s.Gain.Portfolio.InstrumentQuotes[1].UnitPrice, _ = ParseUnitPrice("0")
		},
		"split":             func(s *InvestmentReviewSnapshot) { s.Gain.Portfolio.InstrumentQuotes[1].SplitFactor = "2" },
		"dividend_metadata": func(s *InvestmentReviewSnapshot) { s.Gain.Portfolio.InstrumentQuotes[1].DividendCash = "0.01" },
		"dividend_ledger": func(s *InvestmentReviewSnapshot) {
			s.Gain.Dividends = []Activity{{EffectiveLocalDate: "2026-09-15", DividendDetail: &DividendDetail{InstrumentID: s.Gain.Portfolio.Instruments[0].ID}}}
		},
		"quantity_adjustment": func(s *InvestmentReviewSnapshot) {
			s.Gain.CostEvents[s.Gain.Portfolio.Holdings[0].ID] = []CostBasisEvent{{Kind: CostBasisAdjustmentIn, EffectiveAt: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)}}
		},
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			s, i := reviewFixture(t)
			s.Gain.Portfolio.InstrumentQuotes = []InstrumentQuote{reviewQuote(t, i, "2026-09-01", "1"), reviewQuote(t, i, "2026-10-01", "0.98")}
			edit(&s)
			r := onlyReview(t, s, DefaultInvestmentReviewPolicy(), "2026-10-01")
			if r.Status != "unavailable" || r.PeriodPercent != nil || r.EpisodeID != "" {
				t.Fatalf("unsafe return = %+v", r)
			}
		})
	}
}
func dailyReviewQuotes(t *testing.T, i Instrument, start string, days int, value string) []InstrumentQuote {
	d, _ := time.Parse(time.DateOnly, start)
	q := []InstrumentQuote{}
	for n := 0; n < days; n++ {
		q = append(q, reviewQuote(t, i, d.AddDate(0, 0, n).Format(time.DateOnly), value))
	}
	return q
}
func TestInvestmentReviewConfirmationEpisodesAndRecovery(t *testing.T) {
	s, i := reviewFixture(t)
	p := DefaultInvestmentReviewPolicy()
	s.Gain.Portfolio.InstrumentQuotes = dailyReviewQuotes(t, i, "2026-09-01", 37, "1")
	r := onlyReview(t, s, p, "2026-10-07")
	if r.Status != "confirming" || r.EpisodeID != "" {
		t.Fatalf("premature reminder %+v", r)
	}
	s.Gain.Portfolio.InstrumentQuotes = append(s.Gain.Portfolio.InstrumentQuotes, reviewQuote(t, i, "2026-10-08", "1"))
	first := onlyReview(t, s, p, "2026-10-08")
	if first.Status != "low_yield" || first.EpisodeID == "" {
		t.Fatalf("sustained reminder %+v", first)
	}
	// Revision and repeated evaluation are not new observation days or episodes.
	duplicate := reviewQuote(t, i, "2026-10-08", "1")
	duplicate.Revision = 2
	duplicate.CreatedAt = duplicate.CreatedAt.Add(time.Hour)
	s.Gain.Portfolio.InstrumentQuotes = append(s.Gain.Portfolio.InstrumentQuotes, duplicate, reviewQuote(t, i, "2026-10-09", "0.98"))
	loss := onlyReview(t, s, p, "2026-10-09")
	if loss.Status != "loss" || loss.EpisodeID != first.EpisodeID || loss.ObservationCount != 31 {
		t.Fatalf("refresh/loss escalation changed episode %+v", loss)
	}
	stale := onlyReview(t, s, p, "2026-10-17")
	if stale.Status != "stale" || stale.EpisodeID != "" || stale.AnnualizedPercent == nil {
		t.Fatalf("stale anchor %+v", stale)
	}
	disabled := p
	disabled.Enabled = false
	if got := onlyReview(t, s, disabled, "2026-10-09"); got.EpisodeID != "" {
		t.Fatalf("disabled reminder %+v", got)
	}
	// A valid healthy window closes the episode; a new sustained run renews it.
	s.Gain.Portfolio.InstrumentQuotes = append(s.Gain.Portfolio.InstrumentQuotes, reviewQuote(t, i, "2026-10-10", "1.01"))
	if got := onlyReview(t, s, p, "2026-10-10"); got.Status != "healthy" || got.EpisodeID != "" {
		t.Fatalf("no recovery %+v", got)
	}
	s.Gain.Portfolio.InstrumentQuotes = append(s.Gain.Portfolio.InstrumentQuotes, dailyReviewQuotes(t, i, "2026-10-11", 8, "0.98")...)
	next := onlyReview(t, s, p, "2026-10-18")
	if next.Status != "loss" || next.EpisodeID == "" || next.EpisodeID == first.EpisodeID {
		t.Fatalf("no new episode %+v", next)
	}
}
func TestInvestmentReviewCannotConfirmWithRevisionsOrWideGaps(t *testing.T) {
	s, i := reviewFixture(t)
	p := DefaultInvestmentReviewPolicy()
	s.Gain.Portfolio.InstrumentQuotes = []InstrumentQuote{reviewQuote(t, i, "2026-09-01", "1"), reviewQuote(t, i, "2026-10-01", "1")}
	for n := 1; n < 12; n++ {
		q := reviewQuote(t, i, "2026-10-01", "1")
		q.Revision = n
		s.Gain.Portfolio.InstrumentQuotes = append(s.Gain.Portfolio.InstrumentQuotes, q)
	}
	r := onlyReview(t, s, p, "2026-10-08")
	if r.Status != "confirming" || r.ObservationCount != 2 || r.EpisodeID != "" {
		t.Fatalf("same-day revisions confirmed %+v", r)
	}
	s.Gain.Portfolio.InstrumentQuotes = append(s.Gain.Portfolio.InstrumentQuotes, reviewQuote(t, i, "2026-09-10", "1"), reviewQuote(t, i, "2026-10-10", "1"))
	r = onlyReview(t, s, p, "2026-10-10")
	if r.EpisodeID != "" {
		t.Fatalf("wide-gap confirmation %+v", r)
	}
}
func TestInvestmentReviewFlowsFeesAndLargeDecimals(t *testing.T) {
	s, i := reviewFixture(t)
	price := "9007199254.74099301"
	s.Gain.Portfolio.InstrumentQuotes = []InstrumentQuote{reviewQuote(t, i, "2026-09-01", price), reviewQuote(t, i, "2026-10-01", price)}
	fee, _ := ParseMoney("USD", "100")
	s.Gain.CostEvents[s.Gain.Portfolio.Holdings[0].ID] = []CostBasisEvent{{Kind: CostBasisBuy, Fee: &fee}, {Kind: CostBasisSell, Fee: &fee}, {Kind: CostBasisTransferIn}}
	r := onlyReview(t, s, DefaultInvestmentReviewPolicy(), "2026-10-01")
	if r.PeriodPercent == nil || *r.PeriodPercent != "0" || *r.AnnualizedPercent != "0" {
		t.Fatalf("flows/fees/float corrupted NAV %+v", r)
	}
	if !decimal.RequireFromString(price).Equal(s.Gain.Portfolio.InstrumentQuotes[0].UnitPrice.Decimal()) {
		t.Fatal("original NAV changed")
	}
}
func TestInvestmentReviewPolicyAndScope(t *testing.T) {
	p := DefaultInvestmentReviewPolicy()
	if p.Validate() != nil {
		t.Fatal("bad defaults")
	}
	for _, edit := range []func(*InvestmentReviewPolicy){func(p *InvestmentReviewPolicy) { p.WindowDays = 8 }, func(p *InvestmentReviewPolicy) { p.SustainDays = 1 }, func(p *InvestmentReviewPolicy) { p.MaxAgeDays = 0 }, func(p *InvestmentReviewPolicy) { p.LowAnnualizedPercent = "NaN" }, func(p *InvestmentReviewPolicy) { p.LossPercent = "0" }, func(p *InvestmentReviewPolicy) { p.LossPercent = "-51" }} {
		bad := p
		edit(&bad)
		if bad.Validate() == nil {
			t.Fatalf("invalid policy accepted %+v", bad)
		}
	}
	s, _ := reviewFixture(t)
	archived := time.Now()
	s.Gain.Portfolio.Holdings[0].ArchivedAt = &archived
	if len(ProjectInvestmentReviews(s, p, "2026-10-01")) != 0 {
		t.Fatal("archived holding included")
	}
}

func TestInvestmentReviewThresholdEquality(t *testing.T) {
	s, i := reviewFixture(t)
	p := DefaultInvestmentReviewPolicy()
	s.Gain.Portfolio.InstrumentQuotes = []InstrumentQuote{reviewQuote(t, i, "2026-09-01", "1"), reviewQuote(t, i, "2026-10-01", "1.003")}
	p.LowAnnualizedPercent = "3.65"
	if r := onlyReview(t, s, p, "2026-10-01"); r.Status != "healthy" {
		t.Fatalf("low threshold equality %+v", r)
	}
	p.LowAnnualizedPercent = "3.65000001"
	if r := onlyReview(t, s, p, "2026-10-01"); r.Status != "confirming" {
		t.Fatalf("strict low threshold %+v", r)
	}
	p.LowAnnualizedPercent = "0"
	s.Gain.Portfolio.InstrumentQuotes[1] = reviewQuote(t, i, "2026-10-01", "0.99")
	if r := onlyReview(t, s, p, "2026-10-01"); r.Status != "confirming" || *r.PeriodPercent != "-1" {
		t.Fatalf("loss equality %+v", r)
	}
	p.LowAnnualizedPercent = "1.000000000"
	if p.Validate() == nil {
		t.Fatal("unbounded precision accepted")
	}
}

func TestInvestmentReviewEpisodeAliasesStopAtSupportedRecovery(t *testing.T) {
	s, i := reviewFixture(t)
	p := DefaultInvestmentReviewPolicy()
	s.Gain.Portfolio.InstrumentQuotes = dailyReviewQuotes(t, i, "2026-09-01", 39, "1")
	original := onlyReview(t, s, p, "2026-10-09")
	s.Gain.Portfolio.InstrumentQuotes = append(s.Gain.Portfolio.InstrumentQuotes, reviewQuote(t, i, "2026-08-31", "1"))
	enriched := onlyReview(t, s, p, "2026-10-09")
	contains := func(aliases []string, id string) bool {
		for _, alias := range aliases {
			if alias == id {
				return true
			}
		}
		return false
	}
	if original.EpisodeID == enriched.EpisodeID || !contains(enriched.EpisodeAliases, original.EpisodeID) {
		t.Fatalf("acknowledgement lineage lost: %+v", enriched)
	}
	s.Gain.Portfolio.InstrumentQuotes = append(s.Gain.Portfolio.InstrumentQuotes, reviewQuote(t, i, "2026-10-10", "1.01"))
	s.Gain.Portfolio.InstrumentQuotes = append(s.Gain.Portfolio.InstrumentQuotes, dailyReviewQuotes(t, i, "2026-10-11", 8, "0.98")...)
	next := onlyReview(t, s, p, "2026-10-18")
	if next.EpisodeID == "" || contains(next.EpisodeAliases, original.EpisodeID) {
		t.Fatalf("recovered episode still aliases old acknowledgement: %+v", next)
	}
}

// Renaming affects only presentation, across both unavailable and evaluated rows.
func TestInvestmentReviewDisplayLabelsPreserveProjection(t *testing.T) {
	for _, state := range []string{"no_quotes", "missing_baseline", "baseline_too_old", "unsafe_nav", "distribution", "confirming", "healthy", "low_yield", "loss", "stale", "disabled"} {
		t.Run(state, func(t *testing.T) {
			s, i := reviewFixture(t)
			p := DefaultInvestmentReviewPolicy()
			today := "2026-10-09"
			s.Gain.Portfolio.InstrumentQuotes = dailyReviewQuotes(t, i, "2026-09-01", 39, "1")
			status, reason := "low_yield", ""
			switch state {
			case "no_quotes":
				s.Gain.Portfolio.InstrumentQuotes = nil
				status, reason = "unavailable", "unit_nav_required"
			case "missing_baseline":
				s.Gain.Portfolio.InstrumentQuotes = []InstrumentQuote{reviewQuote(t, i, today, "1")}
				status, reason = "unavailable", "missing_baseline"
			case "baseline_too_old":
				s.Gain.Portfolio.InstrumentQuotes = []InstrumentQuote{reviewQuote(t, i, "2026-09-01", "1"), reviewQuote(t, i, today, "1")}
				status, reason = "unavailable", "baseline_too_old"
			case "unsafe_nav":
				s.Gain.Portfolio.InstrumentQuotes[len(s.Gain.Portfolio.InstrumentQuotes)-1].PriceBasis = "cumulative_nav"
				status, reason = "unavailable", "unit_nav_required"
			case "distribution":
				s.Gain.Portfolio.InstrumentQuotes[len(s.Gain.Portfolio.InstrumentQuotes)-1].DividendCash = "0.01"
				status, reason = "unavailable", "distribution_or_unit_change"
			case "confirming":
				s.Gain.Portfolio.InstrumentQuotes = dailyReviewQuotes(t, i, "2026-09-01", 31, "1")
				today = "2026-10-01"
				status = "confirming"
			case "healthy":
				s.Gain.Portfolio.InstrumentQuotes[len(s.Gain.Portfolio.InstrumentQuotes)-1] = reviewQuote(t, i, today, "1.01")
				status = "healthy"
			case "loss":
				s.Gain.Portfolio.InstrumentQuotes[len(s.Gain.Portfolio.InstrumentQuotes)-1] = reviewQuote(t, i, today, "0.98")
				status = "loss"
			case "stale":
				today = "2026-10-17"
				status = "stale"
			case "disabled":
				p.Enabled = false
				status = "confirming"
			}
			original := onlyReview(t, s, p, today)
			if original.Status != status || original.Reason != reason {
				t.Fatalf("fixture: %+v", original)
			}
			for _, label := range []struct{ alias, code, want string }{
				{"Family fund", "000001", "Family fund"},
				{"", "000001", "000001"},
				{"\u0085\u2003\ufeff", "\u00a0000001\u2003", "000001"},
				{"\ufeff\u2003家用基金\u0085", "000001", "家用基金"},
				{"", "\u0085\ufeff", "Synthetic fund"},
			} {
				s.Gain.Portfolio.Instruments[0].ShortName = label.alias
				s.Gain.Portfolio.Instruments[0].Symbol = &label.code
				got := onlyReview(t, s, p, today)
				if got.DisplayName != label.want {
					t.Errorf("label = %q, want %q", got.DisplayName, label.want)
				}
				got.DisplayName = original.DisplayName
				if !reflect.DeepEqual(got, original) {
					t.Fatalf("label changed financial projection: %+v / %+v", got, original)
				}
			}
		})
	}
}

func TestInvestmentReviewDuplicateDisplayAliasesKeepDistinctIDs(t *testing.T) {
	s, i := reviewFixture(t)
	s.Gain.Portfolio.Instruments[0].ShortName = "Family fund"
	other := i
	other.ID = NewInstrumentID()
	other.ShortName = "Family fund"
	s.Gain.Portfolio.Instruments = append(s.Gain.Portfolio.Instruments, other)
	holding := s.Gain.Portfolio.Holdings[0]
	holding.ID, holding.InstrumentID = NewHoldingID(), other.ID
	s.Gain.Portfolio.Holdings = append(s.Gain.Portfolio.Holdings, holding)
	rows := ProjectInvestmentReviews(s, DefaultInvestmentReviewPolicy(), "2026-10-09")
	if len(rows) != 2 || rows[0].DisplayName != "Family fund" || rows[1].DisplayName != "Family fund" || rows[0].InstrumentID == rows[1].InstrumentID {
		t.Fatalf("duplicate aliases: %+v", rows)
	}
}
