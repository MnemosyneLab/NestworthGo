package application

import (
	"slices"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestReviewHistoricalOriginBaselinePresenceAndLastEntry(t *testing.T) {
	for _, kind := range []string{"missing", "nil-quantity", "nil-cost", "duplicate-last-cost", "unrelated-kind", "nil-holding"} {
		t.Run(kind, func(t *testing.T) {
			f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "2")
			f.day(t, "2026-08-02")
			cutoff, err := domain.ReviewCivilCutoff("2026-08-01", "UTC")
			if err != nil {
				t.Fatal(err)
			}
			batch, err := sqlite.NewRepository(f.db).LoadReviewHistoricalBatch(t.Context(), f.account.Account.HouseholdID, cutoff)
			if err != nil {
				t.Fatal(err)
			}
			portfolio, err := (HistoricalReplay{batch: &batch.History}).Snapshot(t.Context(), &batch.History.Origin, cutoff.Add(-time.Nanosecond))
			if err != nil {
				t.Fatal(err)
			}
			// Isolate baseline presence from holdings legitimately created after
			// the origin: absence must make this preexisting quantity unknown.
			for i := range portfolio.Holdings {
				portfolio.Holdings[i].CreatedAt = batch.History.Origin.StartedAt
			}
			at := slices.IndexFunc(batch.History.OriginData.Components, func(c domain.HistoryOriginComponent) bool {
				return c.Kind == domain.HistoryOriginHoldingQuantity && c.HoldingID != nil && *c.HoldingID == f.holding.ID
			})
			if at < 0 {
				t.Fatal("missing synthetic origin baseline")
			}
			switch kind {
			case "missing":
				batch.History.OriginData.Components = slices.Delete(batch.History.OriginData.Components, at, at+1)
			case "nil-quantity":
				batch.History.OriginData.Components[at].Quantity = nil
			case "nil-cost":
				batch.History.OriginData.Components[at].UnitCost = nil
			default:
				extra := batch.History.OriginData.Components[at]
				cost, err := domain.ParseUnitPrice("150")
				if err != nil {
					t.Fatal(err)
				}
				extra.UnitCost = &cost
				if kind == "unrelated-kind" {
					extra.Kind = domain.HistoryOriginAccountCash
				}
				if kind == "nil-holding" {
					extra.HoldingID = nil
				}
				batch.History.OriginData.Components = append(batch.History.OriginData.Components, extra)
			}
			input, coverage, err := projectReviewHistoricalDayFromPortfolio(t.Context(), batch, "", syntheticHistoricalReviewSpec(), f.instrument.ID, "2026-08-01", cutoff, portfolio)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "missing" {
				if input.Quantity != nil || !slices.Contains(coverage.Missing, "historical_quantity") {
					t.Fatal("missing origin fabricated quantity", input, coverage)
				}
				return
			}
			if input.Quantity == nil || *input.Quantity != "2" {
				t.Fatal("present origin lost quantity coverage", input, coverage)
			}
			if kind == "nil-quantity" || kind == "nil-cost" {
				if input.CostExact != nil || !slices.Contains(coverage.Missing, "historical_cost") {
					t.Fatal("unknown baseline fabricated cost", input, coverage)
				}
				return
			}
			want := "200"
			if kind == "duplicate-last-cost" {
				want = "300"
			}
			if input.CostExact == nil || *input.CostExact != want {
				t.Fatal("baseline selection changed", want, input, coverage)
			}
		})
	}
}

func syntheticHistoricalReviewSpec() domain.ReviewRuleSpec {
	return domain.ReviewRuleSpec{
		SpecVersion: domain.ReviewSpecVersion, MetricPolicyVersion: domain.ReviewMetricPolicyVersion, Evaluator: domain.ReviewEvaluator,
		Target: domain.ReviewRuleTarget{Kind: "all_current"},
		Predicate: domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{
			{Leaf: &domain.ReviewRuleLeaf{Metric: "type", Operator: "in", Types: []domain.InstrumentType{domain.InstrumentStock}}},
			{Leaf: &domain.ReviewRuleLeaf{Metric: "cost", Operator: "<", Value: "250", Currency: "CNY", FXMode: "native"}},
		}},
		Schedule:      domain.ReviewRuleSchedule{Period: "calendar_day", Timezone: "UTC", CutoffVersion: domain.ReviewCutoffVersion},
		TriggerPolicy: domain.ReviewTriggerPolicy{Kind: "single"},
		DataPolicy:    domain.DefaultReviewRuleDataPolicy(),
	}
}

func TestReviewHistoricalBatchDoesNotBackfillCurrentHolding(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "2")
	f.day(t, "2026-08-02")
	f.quantity(t, false)
	f.day(t, "2026-08-03")
	f.quantity(t, false) // current position is zero, but prior days were positive
	repo := sqlite.NewRepository(f.db)
	var before, after int
	if err := f.db.SQL.QueryRow(`SELECT total_changes()`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	batch, err := repo.LoadReviewHistoricalBatch(t.Context(), f.account.Account.HouseholdID, f.now.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		on, quantity, cost string
		truth              domain.ReviewTruth
	}{
		{"2026-08-01", "2", "200", domain.ReviewTrue},
		{"2026-08-02", "1", "100", domain.ReviewTrue},
		{"2026-08-03", "0", "", domain.ReviewFalse},
	} {
		input, _, err := ProjectReviewHistoricalDay(t.Context(), batch, "", syntheticHistoricalReviewSpec(), f.instrument.ID, tc.on)
		if err != nil {
			t.Fatal(err)
		}
		if input.Quantity == nil || *input.Quantity != tc.quantity || (tc.cost != "" && (input.CostExact == nil || *input.CostExact != tc.cost)) {
			t.Fatalf("%s: quantity=%v cost=%v", tc.on, input.Quantity, input.CostExact)
		}
		result, err := domain.EvaluateReviewDaily(syntheticHistoricalReviewSpec(), input)
		if err != nil || result.Truth != tc.truth {
			t.Fatalf("%s: result=%+v err=%v", tc.on, result, err)
		}
	}
	// Missing cost events are unknown, never a fabricated zero or an inherited
	// current-day cost. The separately proven quantity remains usable.
	delete(batch.CostEvents, f.holding.ID)
	input, coverage, err := ProjectReviewHistoricalDay(t.Context(), batch, "", syntheticHistoricalReviewSpec(), f.instrument.ID, "2026-08-01")
	if err != nil || input.CostExact != nil || input.Quantity == nil || *input.Quantity != "2" || len(coverage.Missing) == 0 {
		t.Fatalf("missing cost must remain unknown: %+v %+v %v", input, coverage, err)
	}
	result, err := domain.EvaluateReviewDaily(syntheticHistoricalReviewSpec(), input)
	if err != nil || result.Truth != domain.ReviewUnknown {
		t.Fatalf("unknown cost became hit: %+v %v", result, err)
	}
	if err := f.db.SQL.QueryRow(`SELECT total_changes()`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("read-only projection wrote %d changes", after-before)
	}
}

func TestReviewHistoricalDayBeforeOriginIsUnknown(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "2")
	repo := sqlite.NewRepository(f.db)
	batch, err := repo.LoadReviewHistoricalBatch(t.Context(), f.account.Account.HouseholdID, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	input, coverage, err := ProjectReviewHistoricalDay(t.Context(), batch, "", syntheticHistoricalReviewSpec(), f.instrument.ID, "2026-07-31")
	if err != nil || input.Quantity != nil || len(coverage.Missing) != 1 || coverage.Missing[0] != "history_origin" {
		t.Fatalf("day before authoritative origin: %+v %+v %v", input, coverage, err)
	}
}

func TestReviewHistoricalCostKeepsActivityUntilEffectiveReversalDate(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
	f.day(t, "2026-08-02")
	importReviewDailyClose(t, f, "2026-08-02", "100")
	cost, err := domain.ParseUnitPrice("100")
	if err != nil {
		t.Fatal(err)
	}
	added, err := f.s.RecordChange(t.Context(), domain.PositionAdjustmentInput{
		HouseholdID: f.account.Account.HouseholdID, HoldingID: f.holding.ID,
		Quantity: mustQuantity(t, "1"), Added: true, UnitCost: &cost, EffectiveAt: *f.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.day(t, "2026-08-04")
	if _, err := f.s.UndoChange(t.Context(), added.Activity.ID); err != nil {
		t.Fatal(err)
	}

	batch, err := sqlite.NewRepository(f.db).LoadReviewHistoricalBatch(t.Context(), f.account.Account.HouseholdID, f.now.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		date, quantity, cost string
	}{
		{date: "2026-08-02", quantity: "2", cost: "200"},
		{date: "2026-08-04", quantity: "1", cost: "100"},
	} {
		input, coverage, err := ProjectReviewHistoricalDay(t.Context(), batch, "", syntheticHistoricalReviewSpec(), f.instrument.ID, tc.date)
		if err != nil || input.Quantity == nil || *input.Quantity != tc.quantity || input.CostExact == nil || *input.CostExact != tc.cost {
			t.Fatalf("historical cost on %s: input=%+v coverage=%+v events=%+v err=%v", tc.date, input, coverage, batch.CostEvents[f.holding.ID], err)
		}
	}
}

func TestReviewHistoricalDayDoesNotBorrowAnotherStocksDailyPrices(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-09-10", "1")
	requireDailyReviewPrices(t, &f)
	f.day(t, "2026-10-10")
	batch, err := sqlite.NewRepository(f.db).LoadReviewHistoricalBatch(t.Context(), f.account.Account.HouseholdID, f.now.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	other := f.instrument
	other.ID = domain.NewInstrumentID()
	other.QuoteSource = domain.QuoteSourceAgent
	batch.History.Portfolio.Instruments = append(batch.History.Portfolio.Instruments, other)
	quote := func(on, value string) domain.InstrumentQuote {
		t.Helper()
		price, err := domain.ParseUnitPrice(value)
		if err != nil {
			t.Fatal(err)
		}
		at, err := time.Parse(time.DateOnly, on)
		if err != nil {
			t.Fatal(err)
		}
		return domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: other.ID, UnitPrice: price, Currency: "CNY", SourceKind: domain.QuoteSourceAgent, SourceKey: "agent", QuotedAt: at, CreatedAt: at, ValueEffectiveAt: at, EffectiveDate: on, ObservationKind: "close", PriceBasis: "agent_raw_close_v1", TimestampBasis: "date_label", Revision: 1}
	}
	batch.History.InstrumentQuoteFacts = append(batch.History.InstrumentQuoteFacts, quote("2026-09-10", "1000"), quote("2026-10-10", "2000"))
	spec := syntheticHistoricalReviewSpec()
	allowDailyReviewCrypto(&spec.Predicate)
	spec.Predicate = domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{
		{Leaf: &domain.ReviewRuleLeaf{Metric: "amount", Operator: ">", Value: "1000", Currency: "CNY", FXMode: "native"}},
		{Leaf: &domain.ReviewRuleLeaf{Metric: "price_change", Operator: ">", Value: "50", LookbackDays: 30}},
	}}
	input, coverage, err := ProjectReviewHistoricalDay(t.Context(), batch, "", spec, f.instrument.ID, "2026-10-10")
	if err != nil {
		t.Fatal(err)
	}
	if input.Quantity == nil || *input.Quantity != "1" || input.AmountExact != nil || len(input.Performance) != 0 {
		t.Fatalf("other stock's prices leaked into target input: %+v coverage=%+v", input, coverage)
	}
	result, err := domain.EvaluateReviewDaily(spec, input)
	if err != nil || result.Truth != domain.ReviewUnknown {
		t.Fatalf("missing target close should leave amount and performance unknown: %+v %v", result, err)
	}
}
