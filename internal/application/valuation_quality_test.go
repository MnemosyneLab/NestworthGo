package application

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
	"time"
)

func TestDisplayQualityDistinctLeavesAndEmptyUnknown(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                  string
		components            []domain.ValuationComponent
		missing               [][]domain.MissingInputView
		availability          domain.ValuationAvailability
		subtotal              string
		unpriced, provisional int
	}{
		{name: "empty", availability: domain.ValuationFull, subtotal: "0"},
		{name: "zero quantity", components: []domain.ValuationComponent{{Available: true, BaseAmountExact: "0"}}, availability: domain.ValuationFull, subtotal: "0"},
		{name: "none", components: []domain.ValuationComponent{{}}, missing: [][]domain.MissingInputView{{{Kind: domain.MissingInstrumentPrice}, {Kind: domain.MissingFXRate}}}, availability: domain.ValuationNone, unpriced: 1},
		{name: "partial", components: []domain.ValuationComponent{{Available: true, BaseAmountExact: "12.34567"}, {}}, missing: [][]domain.MissingInputView{nil, {{Kind: domain.MissingAccountValue}, {Kind: domain.MissingFXRate}}}, availability: domain.ValuationPartial, subtotal: "12.3457", unpriced: 1},
		{name: "coverage only", components: []domain.ValuationComponent{{Available: true, BaseAmountExact: "200"}}, missing: [][]domain.MissingInputView{{{Kind: domain.MissingHistoryCoverage}, {Kind: domain.MissingHistoryCoverage}}}, availability: domain.ValuationFull, subtotal: "200", provisional: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var a displayAccumulator
			for i, c := range tc.components {
				var missing []domain.MissingInputView
				if i < len(tc.missing) {
					missing = tc.missing[i]
				}
				if err := a.add(c, missing, "2026-10-12", false); err != nil {
					t.Fatal(err)
				}
			}
			v, err := a.amount("CNY", false)
			if err != nil {
				t.Fatal(err)
			}
			if v.Quality.Availability != tc.availability || v.Quality.UnpricedCount != tc.unpriced || v.Quality.ProvisionalCount != tc.provisional {
				t.Fatalf("quality %+v", v.Quality)
			}
			if tc.subtotal == "" {
				if v.KnownSubtotal != nil || v.Total != nil {
					t.Fatalf("unknown became zero: %+v", v)
				}
			} else if v.KnownSubtotal == nil || v.KnownSubtotal.Amount != tc.subtotal {
				t.Fatalf("subtotal %+v", v)
			}
			if (v.Total != nil) != (tc.availability == domain.ValuationFull) {
				t.Fatal("full total contract")
			}
		})
	}
}
func TestDisplayQualityCarriedPriceFXAndMissingLiability(t *testing.T) {
	t.Parallel()
	var a, l displayAccumulator
	price := &domain.QuoteEvidenceView{Source: domain.QuoteSourceAgent, EffectiveDate: "2026-10-09", FetchedAt: time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC), Freshness: domain.FreshnessStale}
	fx := &domain.QuoteEvidenceView{Source: domain.QuoteSourceProvider, EffectiveDate: "2026-10-08"}
	if err := a.add(domain.ValuationComponent{Available: true, BaseAmountExact: "200", PriceEvidence: price, FXEvidence: fx}, []domain.MissingInputView{{Kind: domain.MissingHistoryCoverage}}, "2026-10-12", false); err != nil {
		t.Fatal(err)
	}
	if err := l.add(domain.ValuationComponent{}, []domain.MissingInputView{{Kind: domain.MissingAccountValue}}, "2026-10-12", false); err != nil {
		t.Fatal(err)
	}
	v, err := householdDisplay(a, l, "CNY")
	if err != nil {
		t.Fatal(err)
	}
	if v.Assets.Total == nil || v.NetWorth.Total != nil || v.NetWorth.KnownSubtotal == nil || v.NetWorth.KnownSubtotal.Amount != "200" {
		t.Fatalf("missing liability hid known contribution or faked total: %+v", v)
	}
	q := v.Assets.Quality
	if q.CarriedCount != 1 || q.StaleCount != 1 || q.OldestMarketDate == nil || *q.OldestMarketDate != "2026-10-08" || *q.NewestMarketDate != "2026-10-09" || q.Coverage != domain.CoverageProvisional {
		t.Fatalf("economic evidence %+v", q)
	}
}
func TestDelayedNAVDisplayRolloverLateRevisionAndReadOnlyCapture(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s, db, owner, now := overviewFixture(t)
	*now = time.Date(2026, 10, 9, 4, 0, 0, 0, time.UTC)
	account := overviewAccount(t, s, owner, "Synthetic NAV account", "brokerage", "asset", "holdings", "CNY", "")
	instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic NAV", Type: "mutual_fund", QuoteCurrency: "CNY", QuoteSource: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := s.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "100"})
	if err != nil {
		t.Fatal(err)
	}

	item := AgentMarketDataItem{InstrumentID: instrument.ID.String(), Currency: "CNY", Kind: "nav", Date: "2026-10-09", Timezone: "Asia/Shanghai", Value: "2", SourceTitle: "Synthetic source"}
	if _, err = s.ImportAgentMarketData(ctx, domain.NewActivityID().String(), AgentMarketDataInput{Items: []AgentMarketDataItem{item}}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartHistoryWithCosts(ctx, "Asia/Shanghai", map[domain.HoldingID]string{holding.ID: "2"}); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 10, 12, 4, 0, 0, 0, time.UTC)
	live, err := s.NetWorthTrend(ctx, domain.TrendRange("2026-10-12:2026-10-12"))
	if err != nil {
		t.Fatal(err)
	}
	if len(live.Points) != 1 || live.Points[0].DisplayValuation.NetWorth.Total == nil || live.Points[0].DisplayValuation.NetWorth.Total.Amount != "200" {
		t.Fatalf("current %+v", live)
	}
	*now = time.Date(2026, 10, 13, 4, 0, 0, 0, time.UTC)
	closed, err := s.NetWorthTrend(ctx, domain.TrendRange("2026-10-12:2026-10-12"))
	if err != nil {
		t.Fatal(err)
	}
	p := closed.Points[0]
	if p.Complete || p.NetWorth != nil || p.DisplayValuation.NetWorth.Total == nil || p.DisplayValuation.NetWorth.Total.Amount != "200" || p.DisplayValuation.NetWorth.Quality.Coverage != domain.CoverageProvisional {
		t.Fatalf("rolled day %+v", p)
	}
	var before, after int
	if err = db.SQL.QueryRow(`SELECT total_changes()`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	panorama, err := s.HistoricalOverview(ctx, "2026-10-12", "")
	if err != nil {
		t.Fatal(err)
	}
	series, err := s.HistoricalOverviewSeries(ctx, "2026-10-11", "2026-10-12")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.SQL.QueryRow(`SELECT total_changes()`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after || panorama.Left.Complete || panorama.Left.DisplayValuation.NetWorth.Total.Amount != "200" || series.Points[1].DisplayValuation.NetWorth.Total.Amount != "200" || !series.Points[1].ConnectNetWorth {
		t.Fatalf("read-only agreement: changes %d/%d panorama %+v series %+v", before, after, panorama.Left, series.Points)
	}
	item.Date = "2026-10-12"
	item.Value = "2.1"
	if _, err = s.ImportAgentMarketData(ctx, domain.NewActivityID().String(), AgentMarketDataInput{Items: []AgentMarketDataItem{item}}); err != nil {
		t.Fatal(err)
	}
	revised, err := s.NetWorthTrend(ctx, domain.TrendRange("2026-10-12:2026-10-13"))
	if err != nil {
		t.Fatal(err)
	}
	if revised.Points[0].DisplayValuation.NetWorth.Total.Amount != "210" || revised.Points[1].DisplayValuation.NetWorth.Total.Amount != "210" {
		t.Fatalf("economic revision %+v", revised.Points)
	}
	records, err := s.AgentMarketDataRecords(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("display created fake observations: %d", len(records))
	}
}
func TestDelayedNAVQuantityAndNoLookahead(t *testing.T) {
	t.Parallel()
	id := domain.NewInstrumentID()
	household := &domain.Household{ID: domain.NewHouseholdID(), BaseCurrency: "CNY"}
	instrument := domain.Instrument{ID: id, Type: domain.InstrumentMutualFund, QuoteCurrency: "CNY", QuoteSource: domain.QuoteSourceAgent}
	price, _ := domain.ParseUnitPrice("2")
	at := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	quote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: id, Currency: "CNY", UnitPrice: price, SourceKind: domain.QuoteSourceAgent, SourceKey: "agent", ObservationKind: "close", EffectiveDate: "2026-10-09", ValueEffectiveAt: at, QuotedAt: at, PriceBasis: "agent_unit_nav_v1", SourcePolicyVersion: "agent_supplied_v1", TimestampBasis: "date_label"}
	p := domain.PortfolioSnapshot{Household: household, InstrumentQuotes: []domain.InstrumentQuote{quote}}
	v := NewValuationService(nil, func() time.Time { return time.Date(2026, 10, 12, 16, 0, 0, 0, time.UTC) })
	v.SetHistorical(true)
	v.SetHistoricalMarketDate("2026-10-12")
	for _, tc := range []struct{ quantity, value string }{{"100", "200"}, {"150", "300"}, {"0", "0"}} {
		q, _ := domain.ParseQuantity(tc.quantity)
		c, m, err := v.valueHolding(p, domain.NewAccountID(), domain.Holding{ID: domain.NewHoldingID(), InstrumentID: id, Quantity: q}, instrument)
		if err != nil {
			t.Fatal(err)
		}
		var a displayAccumulator
		if err = a.add(c, m, "2026-10-12", false); err != nil {
			t.Fatal(err)
		}
		display, err := a.amount("CNY", false)
		if err != nil || display.Total == nil || display.Total.Amount != tc.value {
			t.Fatalf("quantity %s display %+v %v", tc.quantity, display, err)
		}
	}
	quote.EffectiveDate = "2026-10-13"
	quote.ValueEffectiveAt = at.AddDate(0, 0, 4)
	quote.QuotedAt = quote.ValueEffectiveAt
	p.InstrumentQuotes = []domain.InstrumentQuote{quote}
	q, _ := domain.ParseQuantity("100")
	c, _, err := v.valueHolding(p, domain.NewAccountID(), domain.Holding{InstrumentID: id, Quantity: q}, instrument)
	if err != nil || c.Available {
		t.Fatalf("future quote admitted %+v %v", c, err)
	}
}
func TestLegacyDisplayEvidenceConservative(t *testing.T) {
	t.Parallel()
	reason := "historical market-data coverage is unverified"
	item := domain.DailyValuationSnapshotItem{AccountID: domain.NewAccountID(), BaseAmountExact: "200", MissingReason: &reason}
	snapshot := domain.DailyValuationSnapshot{ID: domain.NewDailyValuationSnapshotID(), Currency: "CNY", Items: []domain.DailyValuationSnapshotItem{item}, ComponentCount: 1}
	h, _, err := snapshotDisplay(snapshot, snapshotDisplayEvidence{})
	if err != nil || h.NetWorth.Total != nil || h.NetWorth.KnownSubtotal == nil || h.NetWorth.KnownSubtotal.Amount != "200" {
		t.Fatalf("legacy %+v %v", h, err)
	}
}

func TestDelayedNAVAssetLevelsNeverGrantReturnsOrSearchInward(t *testing.T) {
	t.Parallel()
	householdID := domain.NewHouseholdID()
	account := analysisAgentAccount("CNY", domain.TrackingHoldings, domain.RoleAsset)
	account.HouseholdID = householdID
	account.AccountType = domain.TypeBrokerage
	instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
	quoteID := domain.NewInstrumentQuoteID()
	quoteIDString := quoteID.String()
	at := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	reason := "historical market-data coverage is unverified"
	item := domain.DailyValuationSnapshotItem{AccountID: account.ID, HoldingID: &holdingID, InstrumentID: &instrumentID, NativeCurrency: "CNY", NativeAmount: "200", BaseAmountExact: "200", QuoteID: &quoteIDString, MissingReason: &reason}
	price, _ := domain.ParseUnitPrice("2")
	quote := domain.InstrumentQuote{ID: quoteID, InstrumentID: instrumentID, SourceKind: domain.QuoteSourceAgent, EffectiveDate: "2026-10-09", QuotedAt: at, ValueEffectiveAt: at, UnitPrice: price, Currency: "CNY"}
	input := AnalysisInputs{Origin: analysisAgentOrigin(t, householdID), Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: "CNY"}, Accounts: []domain.AccountRecord{{Account: account}}, Instruments: []domain.Instrument{{ID: instrumentID, Type: domain.InstrumentMutualFund, QuoteCurrency: "CNY"}}, Holdings: []domain.Holding{{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}}}, Snapshots: []domain.DailyValuationSnapshot{analysisAgentSnapshot("2026-10-11", item), analysisAgentSnapshot("2026-10-12", item)}, InstrumentQuotes: []domain.InstrumentQuote{quote}}
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-10-12", To: "2026-10-12", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment}
	result, err := ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	for _, day := range result.Days {
		if day.ReturnAmount != nil || day.ReturnRate != nil || day.BeginningValue.Currency() != "" || day.EndingValue.Currency() != "" || len(day.AssetBuckets) > 0 || len(day.ReturnComponents) > 0 || day.DisplayEnding == nil || day.DisplayEnding.Total == nil {
			t.Fatalf("display leaked into return kernel: %+v", day)
		}
	}
	levels, err := foldAssetTrend(result, "", TrendDay, TrendNetWorth)
	if err != nil {
		t.Fatal(err)
	}
	if !levels.Available || len(levels.Points) != 1 || levels.Points[0].Value != nil || levels.Points[0].DisplayValuation.Total.Amount != "200" {
		t.Fatalf("levels %+v", levels)
	}
	for _, metric := range []AssetTrendMetric{TrendReturnRate, TrendInvestmentReturn, TrendPriceChange, TrendFXImpact, TrendResidual, TrendNetChange} {
		view, err := foldAssetTrend(result, "", TrendDay, metric)
		if err != nil {
			t.Fatal(err)
		}
		for _, point := range view.Points {
			if point.Value != nil || point.Rate != nil || point.DisplayValuation != nil {
				t.Fatalf("strict %s fabricated data: %+v", metric, point)
			}
		}
	}
	missing := item
	missing.NativeAmount = ""
	missing.BaseAmountExact = ""
	missing.QuoteID = nil
	input.Snapshots = append(input.Snapshots, analysisAgentSnapshot("2026-10-13", missing))
	query.To = "2026-10-13"
	result, err = ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	for _, granularity := range []AssetTrendGranularity{TrendWeek, TrendMonth} {
		view, err := foldAssetTrend(result, "", granularity, TrendNetWorth)
		if err != nil {
			t.Fatal(err)
		}
		if len(view.Points) != 1 || view.Points[0].DisplayValuation.Total != nil || view.Points[0].DisplayValuation.KnownSubtotal != nil {
			t.Fatalf("%s searched inward %+v", granularity, view)
		}
	}
}

func TestDisplayMissingSnapshotDoesNotProveEmptyLiabilities(t *testing.T) {
	t.Parallel()
	h, p, err := snapshotDisplay(domain.DailyValuationSnapshot{Currency: "CNY"}, snapshotDisplayEvidence{})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []domain.ValuationAmount{h.Assets, h.Liabilities, h.NetWorth, p} {
		if v.Total != nil || v.KnownSubtotal != nil || v.Quality.Availability != domain.ValuationNone {
			t.Fatalf("missing day became known zero: %+v", v)
		}
	}
}
