package application

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"testing"
	"time"
)

func TestWeekendCarryIsHealthyForDailyPricedInstruments(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"stock", "etf", "mutual_fund", "bank_investment_product", "bond", "precious_metal", "crypto", "other"} {
		t.Run(kind, func(t *testing.T) {
			ctx := t.Context()
			s, db, owner, now := overviewFixture(t)
			*now = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
			a := overviewAccount(t, s, owner, "Synthetic weekend", "brokerage", "asset", "holdings", "CNY", "")
			i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic weekend instrument", Type: kind, QuoteCurrency: "CNY", QuoteSource: "agent"})
			if err != nil {
				t.Fatal(err)
			}
			h, err := s.CreateHolding(ctx, HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "10"})
			if err != nil {
				t.Fatal(err)
			}
			quoteKind := "close"
			if kind == "mutual_fund" || kind == "bank_investment_product" {
				quoteKind = "nav"
			}
			if _, err = s.ImportAgentMarketData(ctx, domain.NewActivityID().String(), AgentMarketDataInput{Items: []AgentMarketDataItem{{InstrumentID: i.ID.String(), Currency: "CNY", Kind: quoteKind, Date: "2026-10-16", Timezone: "UTC", Value: "2", SourceTitle: "Synthetic Friday price"}}}); err != nil {
				t.Fatal(err)
			}
			if _, err = s.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{h.ID: "2"}); err != nil {
				t.Fatal(err)
			}
			*now = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
			live, err := s.NetWorthTrend(ctx, domain.TrendRange("2026-10-18:2026-10-18"))
			if err != nil {
				t.Fatal(err)
			}
			if kind != "crypto" && kind != "other" {
				q := live.Points[0].DisplayValuation.NetWorth.Quality
				if q.CarriedCount != 0 || q.ProvisionalCount != 0 || q.StaleCount != 0 || q.Coverage != domain.CoverageVerified {
					t.Fatalf("live weekend: %+v", live.Points[0])
				}
			}
			*now = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
			trend, err := s.NetWorthTrend(ctx, domain.TrendRange("2026-10-16:2026-10-18"))
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range trend.Points {
				if p.LocalDate == "2026-10-16" {
					continue
				}
				q := p.DisplayValuation.NetWorth.Quality
				if kind == "crypto" || kind == "other" {
					if p.Complete || q.CarriedCount != 1 {
						t.Fatalf("crypto lost weekend requirements: %+v", p)
					}
				} else if !p.Complete || q.CarriedCount != 0 || q.ProvisionalCount != 0 || q.StaleCount != 0 || q.Coverage != domain.CoverageVerified {
					t.Fatalf("weekend not healthy: %+v", p)
				}
			}

			if kind != "crypto" && kind != "other" {
				batch, err := sqlite.NewRepository(db).LoadReviewHistoricalBatch(ctx, a.Account.HouseholdID, *now)
				if err != nil {
					t.Fatal(err)
				}
				for _, on := range []string{"2026-10-17", "2026-10-18"} {
					spec := syntheticHistoricalReviewSpec()
					spec.Predicate = domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{{Leaf: &domain.ReviewRuleLeaf{Metric: "amount", Operator: ">", Value: "10", Currency: "CNY", FXMode: "native"}}}}
					input, _, err := ProjectReviewHistoricalDay(ctx, batch, "", spec, i.ID, on)
					if err != nil {
						t.Fatal(err)
					}
					if input.AmountExact == nil || *input.AmountExact != "20" {
						t.Fatalf("review disagrees with weekend valuation: %+v", input)
					}
					cache := newReviewProjectionCache(domain.ReviewProjectionCapture{Historical: batch}, "")
					cached, _, err := cache.dailyInput(ctx, spec, i.ID, on)
					if err != nil {
						t.Fatal(err)
					}
					if cached.AmountExact == nil || *cached.AmountExact != "20" {
						t.Fatalf("cached review disagrees: %+v", cached)
					}
				}
				query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-10-17", To: "2026-10-18", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true}
				change, err := s.AssetChange(ctx, query)
				if err != nil {
					t.Fatal(err)
				}
				if !change.Available || change.Status != domain.CompletenessOK || change.Summary.Change == nil || !change.Summary.Change.IsZero() || change.ResidualIssueCount != 0 {
					t.Fatalf("asset change disagrees with healthy trend: %+v", change)
				}
				calendar, err := s.ReturnCalendar(ctx, query, "", "day")
				if err != nil {
					t.Fatal(err)
				}
				if len(calendar.Days) != 2 || calendar.Summary.AmountStatus != ReturnAmountComplete || calendar.Summary.ReturnAmount == nil || !calendar.Summary.ReturnAmount.IsZero() {
					t.Fatalf("returns disagree with healthy trend: %+v", calendar)
				}
				for _, day := range calendar.Days {
					if day.AmountStatus != ReturnAmountComplete {
						t.Fatalf("weekend return incomplete: %+v", day)
					}
				}
			}
			health, err := s.ScanMarketDataHealth(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if kind != "crypto" && kind != "other" && (!health.Healthy || len(health.Issues) != 0) {
				t.Fatalf("weekend health: %+v", health)
			}
		})
	}
}
func TestAgentClosureRoundTripCorrectionWithdrawalAndNoPriceFabrication(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s, db, owner, now := overviewFixture(t)
	*now = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	a := overviewAccount(t, s, owner, "Synthetic holiday", "brokerage", "asset", "holdings", "CNY", "")
	i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic holiday fund", Type: "mutual_fund", QuoteCurrency: "CNY", QuoteSource: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.CreateHolding(ctx, HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "10"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ImportAgentMarketData(ctx, domain.NewActivityID().String(), AgentMarketDataInput{Items: []AgentMarketDataItem{{InstrumentID: i.ID.String(), Currency: "CNY", Kind: "nav", Date: "2026-10-16", Timezone: "UTC", Value: "2", SourceTitle: "Synthetic NAV"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{h.ID: "2"}); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	check := func(complete bool) {
		t.Helper()
		out, err := s.HistoricalOverview(ctx, "2026-10-19", "")
		if err != nil {
			t.Fatal(err)
		}
		q := out.Left.DisplayValuation.NetWorth.Quality
		if out.Left.Complete != complete || (q.CarriedCount == 0) != complete {
			t.Fatalf("coverage: %+v", out.Left)
		}
		if out.Left.DisplayValuation.NetWorth.Total.Amount != "20" {
			t.Fatal("closure changed price")
		}
		if complete {
			batch, err := sqlite.NewRepository(db).LoadReviewHistoricalBatch(ctx, a.Account.HouseholdID, *now)
			if err != nil {
				t.Fatal(err)
			}
			spec := syntheticHistoricalReviewSpec()
			spec.Predicate = domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{{Leaf: &domain.ReviewRuleLeaf{Metric: "amount", Operator: ">", Value: "10", Currency: "CNY", FXMode: "native"}}}}
			cache := newReviewProjectionCache(domain.ReviewProjectionCapture{Historical: batch}, "")
			input, _, err := cache.dailyInput(ctx, spec, i.ID, "2026-10-19")
			if err != nil || input.AmountExact == nil || *input.AmountExact != "20" {
				t.Fatalf("review missed Agent closure: %+v %v", input, err)
			}
			query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-10-17", To: "2026-10-19", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true}
			change, err := s.AssetChange(ctx, query)
			if err != nil || change.Status != domain.CompletenessOK || change.Summary.Change == nil || !change.Summary.Change.IsZero() {
				t.Fatalf("asset change missed Agent closure: %+v %v", change, err)
			}
			returns, err := s.ReturnCalendar(ctx, query, "", "day")
			if err != nil || returns.Summary.AmountStatus != ReturnAmountComplete || returns.Summary.ReturnAmount == nil || !returns.Summary.ReturnAmount.IsZero() {
				t.Fatalf("returns missed Agent closure: %+v %v", returns, err)
			}
		}

	}
	check(false)
	key := domain.NewActivityID().String()
	input := AgentMarketDataInput{Items: []AgentMarketDataItem{{InstrumentID: i.ID.String(), Kind: "closed", Date: "2026-10-19", Timezone: "Asia/Hong_Kong", SourceTitle: "Synthetic holiday announcement", SourceURL: "https://example.com/holiday"}}}
	result, err := s.ImportAgentMarketData(ctx, key, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.QuoteIDs) != 1 {
		t.Fatal(result)
	}
	check(true)
	replay, err := s.ImportAgentMarketData(ctx, key, input)
	if err != nil || !replay.Replayed || replay.QuoteIDs[0] != result.QuoteIDs[0] {
		t.Fatalf("retry: %+v %v", replay, err)
	}
	records, err := s.AgentMarketDataRecords(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[1].Closure == nil || records[1].InstrumentQuote != nil || records[1].Closure.Date != "2026-10-19" {
		t.Fatalf("audit: %+v", records)
	}
	var prices int
	if err = db.SQL.QueryRow(`SELECT count(*) FROM instrument_quotes`).Scan(&prices); err != nil || prices != 1 {
		t.Fatalf("fabricated price: %d %v", prices, err)
	}
	input.Items[0].Operation = "correct"
	input.Items[0].TargetQuoteID = result.QuoteIDs[0]
	input.Items[0].Date = "2026-10-20"
	corrected, err := s.ImportAgentMarketData(ctx, domain.NewActivityID().String(), input)
	if err != nil {
		t.Fatal(err)
	}
	check(false) // moving the closure must reopen the old date
	if _, err = s.ImportAgentMarketData(ctx, domain.NewActivityID().String(), AgentMarketDataInput{Items: []AgentMarketDataItem{{Operation: "retract", TargetQuoteID: corrected.QuoteIDs[0], SourceTitle: "Synthetic withdrawal"}}}); err != nil {
		t.Fatal(err)
	}
	closures, err := s.repository.(interface {
		ListInstrumentClosures(context.Context, domain.HouseholdID) ([]domain.InstrumentClosure, error)
	}).ListInstrumentClosures(ctx, i.HouseholdID)
	if err != nil || len(closures) != 0 {
		t.Fatalf("active closures: %+v %v", closures, err)
	}
}

func TestClosedCarryRequiresAnEligibleAnchorAndNoInterveningGap(t *testing.T) {
	t.Parallel()
	id := domain.NewInstrumentID()
	provider := domain.TiingoProviderKey
	i := domain.Instrument{ID: id, Type: domain.InstrumentStock, QuoteCurrency: "USD", QuoteSource: domain.QuoteSourceProvider, ProviderKey: &provider, ProviderBindingRevision: 1}
	price, _ := domain.ParseUnitPrice("10")
	q := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: id, UnitPrice: price, Currency: "USD", SourceKind: domain.QuoteSourceProvider, SourceKey: provider, ObservationKind: "close", EffectiveDate: "2026-10-16", ValueEffectiveAt: time.Date(2026, 10, 16, 20, 0, 0, 0, time.UTC), QuotedAt: time.Date(2026, 10, 16, 20, 0, 0, 0, time.UTC), BindingRevision: 1, PriceBasis: string(PriceBasisTiingoRawClose), SourcePolicyVersion: string(PriceBasisTiingoRawClose), TimestampBasis: string(TimestampBasisSessionClose)}
	coverage := []domain.InstrumentHistoryCoverage{{InstrumentID: id, ClosedDates: []string{"2026-10-19"}}}
	cutoff := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		date             string
		quotes           []domain.InstrumentQuote
		complete, exists bool
	}{
		{"2026-10-18", []domain.InstrumentQuote{q}, true, true},
		{"2026-10-19", []domain.InstrumentQuote{q}, true, true},
		{"2026-10-20", []domain.InstrumentQuote{q}, false, true},
		{"2026-10-19", nil, true, false},
	} {
		selected := selectHistoricalInstrumentQuoteWithCoverageAtMarketDate(i, tc.quotes, coverage, tc.date, cutoff)
		if selected.coverageComplete != tc.complete || (selected.quote != nil) != tc.exists {
			t.Fatalf("%s: %+v", tc.date, selected)
		}
	}
	q.BindingRevision = 2
	if selected := selectHistoricalInstrumentQuoteWithCoverageAtMarketDate(i, []domain.InstrumentQuote{q}, coverage, "2026-10-19", cutoff); selected.quote != nil {
		t.Fatal("closure accepted incompatible price")
	}
	planCoverage := domain.InstrumentHistoryCoverage{InstrumentID: id, InstrumentType: "stock", Market: "HKEX", ProviderKey: provider, CloseMarketDates: []string{"2026-10-16"}, ClosedDates: []string{"2026-10-19"}}
	need, err := planInstrumentRepairNeed(planCoverage, "2026-10-17", "2026-10-19")
	if err != nil || need.OpeningAnchorMissing {
		t.Fatalf("closure requested price: %+v %v", need, err)
	}
	for _, gap := range need.MissingRanges {
		if gap.End >= "2026-10-17" {
			t.Fatalf("closed day backfill: %+v", need)
		}
	}
	planCoverage.CloseMarketDates = nil
	need, err = planInstrumentRepairNeed(planCoverage, "2026-10-19", "2026-10-19")
	if err != nil || !need.OpeningAnchorMissing {
		t.Fatalf("closure became opening price: %+v %v", need, err)
	}
}
