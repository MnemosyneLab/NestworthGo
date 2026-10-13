package application

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"reflect"
	"testing"
	"time"
)

func TestDisplayFractionalLeavesAgreeAcrossValuationSurfaces(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s, _, owner, now := overviewFixture(t)
	i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic fractional NAV", Type: "mutual_fund", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	costs := map[domain.HoldingID]string{}
	for n := 0; n < 2; n++ {
		a := overviewAccount(t, s, owner, "Synthetic fractional account", "brokerage", "asset", "holdings", "CNY", "")
		h, err := s.CreateHolding(ctx, HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "1"})
		if err != nil {
			t.Fatal(err)
		}
		costs[h.ID] = "0.00006"
	}
	if _, err = s.AppendManualInstrumentQuote(ctx, i.ID, "0.00006", "2026-08-01", false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartHistoryWithCosts(ctx, "UTC", costs); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	overview, err := s.Overview(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	trend, err := s.NetWorthTrend(ctx, domain.TrendRange("2026-08-02:2026-08-02"))
	if err != nil {
		t.Fatal(err)
	}
	panorama, err := s.HistoricalOverview(ctx, "2026-08-02", "")
	if err != nil {
		t.Fatal(err)
	}
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-02", To: "2026-08-02", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment}
	for _, granularity := range []AssetTrendGranularity{TrendDay, TrendWeek, TrendMonth} {
		levels, err := s.AssetTrend(ctx, query, granularity, TrendNetWorth)
		if err != nil {
			t.Fatal(err)
		}
		if len(levels.Points) != 1 || levels.Points[0].DisplayValuation.Total == nil || levels.Points[0].DisplayValuation.Total.Amount != "0.0001" {
			t.Fatalf("%s rounded leaf values before aggregation: %+v", granularity, levels)
		}
	}
	for _, v := range []domain.ValuationAmount{overview.DisplayValuation.NetWorth, trend.Points[0].DisplayValuation.NetWorth, panorama.Left.DisplayValuation.NetWorth} {
		if v.Total == nil || v.Total.Amount != "0.0001" {
			t.Fatalf("cross-surface aggregate %+v", v)
		}
	}
}
func TestNativeDisplayRetainsUnverifiedNAVWhenFXMissing(t *testing.T) {
	t.Parallel()
	account := analysisAgentAccount("USD", domain.TrackingHoldings, domain.RoleAsset)
	id := domain.NewInstrumentID()
	qid := domain.NewInstrumentQuoteID()
	qs := qid.String()
	reason := "historical market-data coverage is unverified"
	at := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	p := domain.PortfolioSnapshot{Household: &domain.Household{BaseCurrency: "CNY"}, Instruments: []domain.Instrument{{ID: id, Type: domain.InstrumentMutualFund}}, Accounts: []domain.AccountRecord{{Account: account}}}
	evidence := newSnapshotDisplayEvidence(domain.HistoricalSnapshotBatch{Portfolio: p, InstrumentQuoteFacts: []domain.InstrumentQuote{{ID: qid, InstrumentID: id, SourceKind: domain.QuoteSourceAgent, EffectiveDate: "2026-10-09", QuotedAt: at, ValueEffectiveAt: at}}}, at.AddDate(0, 0, 3), 12*time.Hour)
	item := domain.DailyValuationSnapshotItem{AccountID: account.ID, InstrumentID: &id, NativeCurrency: "USD", NativeAmount: "200", QuoteID: &qs, MissingReason: &reason}
	for _, basis := range []domain.Valuation{domain.ValuationBase, domain.ValuationNative} {
		v, exact, err := analysisDisplayEnding(item, true, account, AnalysisInputs{Portfolio: p}, domain.AnalysisQuery{Valuation: basis}, evidence, "2026-10-12")
		if err != nil {
			t.Fatal(err)
		}
		if v.Quality.ProvisionalCount != 1 {
			t.Fatalf("%s erased price coverage: %+v", basis, v)
		}
		if basis == domain.ValuationNative {
			if v.Total == nil || v.Total.Amount != "200" || v.Total.Currency != "USD" || v.Quality.Coverage != domain.CoverageProvisional || exact == nil {
				t.Fatalf("native %+v", v)
			}
		} else if v.Total != nil || v.KnownSubtotal != nil || v.Quality.UnpricedCount != 1 {
			t.Fatalf("base fabricated missing FX value: %+v", v)
		}
	}
}

type trendEvidenceSpy struct {
	*sqlite.Repository
	reads      int
	wholeReads int
	quotes     int
}

func (r *trendEvidenceSpy) ReadSnapshotQuoteEvidence(ctx context.Context, h domain.HouseholdID, s []domain.DailyValuationSnapshot) (domain.PortfolioSnapshot, error) {
	r.reads++
	p, err := r.Repository.ReadSnapshotQuoteEvidence(ctx, h, s)
	r.quotes = len(p.InstrumentQuotes) + len(p.FXQuotes)
	return p, err
}
func (r *trendEvidenceSpy) LoadHistoricalOverviewSeriesBatch(ctx context.Context, now time.Time, limits domain.HistoricalSeriesLimits, validate func(domain.HistoryOrigin) error) (domain.HistoricalSnapshotBatch, domain.HistoricalSeriesWork, error) {
	r.wholeReads++
	return r.Repository.LoadHistoricalOverviewSeriesBatch(ctx, now, limits, validate)
}
func TestReadyTrendsIgnoreOversizedUnrelatedArchivedHistory(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s, db, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Synthetic visible cash", "bank_account", "asset", "balance", "CNY", "100")
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	if _, err := s.NetWorthTrend(ctx, domain.TrendAllTime); err != nil {
		t.Fatal(err)
	}
	tx, err := db.SQL.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 1100; n++ {
		if _, err = tx.ExecContext(ctx, `INSERT INTO accounts(id,household_id,name,account_type,balance_sheet_role,tracking_mode,default_currency,created_at,updated_at,archived_at,icon_key) SELECT ?,household_id,'Synthetic unrelated archived','bank_account','asset','balance','CNY',created_at,updated_at,updated_at,icon_key FROM accounts WHERE id=?`, domain.NewAccountID().String(), a.Account.ID.String()); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	repo := s.repository.(*sqlite.Repository)
	if _, _, err = repo.LoadHistoricalOverviewSeriesBatch(ctx, *now, historicalSeriesLimits, nil); !hasDomainCode(err, domain.ErrHistoricalSeriesBudget) {
		t.Fatalf("fixture should exceed panorama profile limits: %v", err)
	}
	spy := &trendEvidenceSpy{Repository: repo}
	s.repository = spy
	for _, r := range []domain.TrendRange{"2026-08-02:2026-08-02", "2026-08-03:2026-08-03"} {
		out, err := s.NetWorthTrend(ctx, r)
		if err != nil || len(out.Points) != 1 || out.Points[0].DisplayValuation.NetWorth.Total == nil || out.Points[0].DisplayValuation.NetWorth.Total.Amount != "100" {
			t.Fatalf("ready trend %s: %+v %v", r, out, err)
		}
	}
	if spy.reads != 1 || spy.wholeReads != 0 || spy.quotes != 0 {
		t.Fatalf("history load count: %+v", spy)
	}
}
func TestEstimatedChangesRequireCompatibleScopeAndBasis(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s, _, owner, now := overviewFixture(t)
	a, err := s.CreateAccount(ctx, AccountInput{Name: "Synthetic scope", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "100", IncludeInNetWorth: false, OwnerIDs: []domain.MemberID{owner}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	if _, err = s.UpdateAccount(ctx, a.Account.ID, AccountInput{IncludeInNetWorth: true, IncludeInNetWorthSet: true}); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	for _, series := range []bool{false, true} {
		var out HistoricalOverviewResult
		if series {
			r, e := s.HistoricalOverviewSeries(ctx, "2026-08-01", "2026-08-03")
			out, err = r.Overview, e
		} else {
			out, err = s.HistoricalOverview(ctx, "2026-08-01", "2026-08-03")
		}
		if err != nil {
			t.Fatal(err)
		}
		if out.EstimatedChange != nil || out.Change == nil || out.Change.NetWorth == nil || *out.Change.NetWorth != "100" {
			t.Fatalf("scope estimate or legacy semantics changed %+v", out)
		}
	}
	trend, err := s.NetWorthTrend(ctx, domain.TrendRange("2026-08-01:2026-08-03"))
	if err != nil || trend.EstimatedChange != nil || trend.Change == nil || trend.Change.CanonicalAmount() != "100" {
		t.Fatalf("trend %+v %v", trend, err)
	}
	// Selected quote basis changes with stable scope and currency remain incompatible.
	cell := &HistoricalOverviewCell{Included: true, Currency: "CNY", AssetClass: "investment", Price: &HistoricalOverviewEvidence{ID: "old"}}
	row := HistoricalOverviewRow{Key: "same", Kind: "holding", Role: "asset", InstrumentID: "same", Left: cell}
	first := domain.PortfolioSnapshot{InstrumentQuotes: []domain.InstrumentQuote{{ID: "old", Currency: "CNY", SourceKind: domain.QuoteSourceAgent, PriceBasis: "unit_nav"}}}
	second := domain.PortfolioSnapshot{InstrumentQuotes: []domain.InstrumentQuote{{ID: "old", Currency: "CNY", SourceKind: domain.QuoteSourceAgent, PriceBasis: "split_adjusted"}}}
	scope1, basis1 := historicalValuationCompatibility([]HistoricalOverviewRow{row}, first)
	scope2, basis2 := historicalValuationCompatibility([]HistoricalOverviewRow{row}, second)
	var acc displayAccumulator
	if err = acc.add(domain.ValuationComponent{Available: true, BaseAmountExact: "100"}, nil, "", false); err != nil {
		t.Fatal(err)
	}
	display, err := householdDisplay(acc, displayAccumulator{}, "CNY")
	if err != nil {
		t.Fatal(err)
	}
	left := HistoricalOverviewState{Currency: "CNY", DisplayValuation: display, displayScope: scope1, displayBasis: basis1}
	right := left
	right.displayScope, right.displayBasis = scope2, basis2
	if scope1 != scope2 || basis1 == basis2 || estimatedHistoricalChange(left, right) != nil {
		t.Fatal("selected basis change admitted estimate")
	}
}

func TestEstimatedEndpointChangeRejectsSelectedSourceBasisChange(t *testing.T) {
	t.Parallel()
	s, _, _, _ := seriesAgentSelectionFixture(t, "nav", "2026-08-03")
	for _, series := range []bool{false, true} {
		var out HistoricalOverviewResult
		var err error
		if series {
			r, e := s.HistoricalOverviewSeries(t.Context(), "2026-08-01", "2026-08-03")
			out, err = r.Overview, e
		} else {
			out, err = s.HistoricalOverview(t.Context(), "2026-08-01", "2026-08-03")
		}
		if err != nil {
			t.Fatal(err)
		}
		if !out.Left.Complete || !out.Right.Complete || out.EstimatedChange != nil || out.Change == nil || out.Change.NetWorth == nil || *out.Change.NetWorth != "0" {
			t.Fatalf("source basis guard changed strict endpoints: %+v", out)
		}
	}
	trend, err := s.NetWorthTrend(t.Context(), domain.TrendRange("2026-08-01:2026-08-03"))
	if err != nil || trend.EstimatedChange != nil || trend.Change == nil || trend.Change.CanonicalAmount() != "0" {
		t.Fatalf("selected source trend basis: %+v %v", trend, err)
	}
}

func TestEstimatedBasisIgnoresEconomicConversionValuesAndDates(t *testing.T) {
	t.Parallel()
	a := domain.InstrumentQuote{Currency: "CNY", SourceKind: domain.QuoteSourceProvider, ConversionJSON: `{"policy":"metal_troy_oz_to_gram_v1","symbol":"GC=F","rawCurrency":"USD","rawUnit":"troy_oz","currency":"CNY","unit":"gram","rawPrice":"100","fxRate":"7","rawQuotedAt":"2026-10-09T00:00:00Z"}`}
	b := a
	b.ConversionJSON = `{"policy":"metal_troy_oz_to_gram_v1","symbol":"GC=F","rawCurrency":"USD","rawUnit":"troy_oz","currency":"CNY","unit":"gram","rawPrice":"110","fxRate":"7.1","rawQuotedAt":"2026-10-12T00:00:00Z"}`
	if !reflect.DeepEqual(priceCompatibility(a), priceCompatibility(b)) {
		t.Fatal("economic value/date changed valuation basis")
	}
	b.ConversionJSON = `{"policy":"new_policy","rawCurrency":"USD","rawUnit":"troy_oz","currency":"CNY","unit":"gram"}`
	if reflect.DeepEqual(priceCompatibility(a), priceCompatibility(b)) {
		t.Fatal("conversion policy change ignored")
	}
}

func TestEstimatedRowsAndAccountsRejectProvisionalSourceBasisChange(t *testing.T) {
	t.Parallel()
	s, _, _, _ := seriesAgentSelectionFixture(t, "nav", "2026-08-03")
	for _, series := range []bool{false, true} {
		var out HistoricalOverviewResult
		var err error
		if series {
			r, e := s.HistoricalOverviewSeries(t.Context(), "2026-08-01", "2026-08-04")
			out, err = r.Overview, e
		} else {
			out, err = s.HistoricalOverview(t.Context(), "2026-08-01", "2026-08-04")
		}
		if err != nil {
			t.Fatal(err)
		}
		sawHolding, sawAccount := false, false
		for _, r := range out.Rows {
			if r.Left == nil || r.Right == nil || !r.Left.Included || !r.Right.Included {
				continue
			}
			if r.Kind != "holding" && r.Kind != "account" {
				continue
			}
			if r.Left.DisplayValuation.Total == nil || r.Right.DisplayValuation.Total == nil || r.Right.DisplayValuation.Quality.Coverage != domain.CoverageProvisional {
				t.Fatalf("fixture lacks full provisional values %+v", r)
			}
			if r.EstimatedBaseChange != nil || r.BaseChange != nil {
				t.Fatalf("source/basis estimate escaped guard: %+v", r)
			}
			sawHolding = sawHolding || r.Kind == "holding"
			sawAccount = sawAccount || r.Kind == "account"
		}
		if !sawHolding || !sawAccount || out.EstimatedChange != nil {
			t.Fatalf("missing row/account guarded projection %+v", out)
		}
	}
	// Same selected source/basis still permits estimates for both leaf and parent.
	out, err := s.HistoricalOverview(t.Context(), "2026-08-03", "2026-08-04")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range out.Rows {
		if r.Kind == "holding" || r.Kind == "account" {
			if r.EstimatedBaseChange == nil || *r.EstimatedBaseChange != "0" || r.BaseChange != nil {
				t.Fatalf("compatible provisional estimate lost %+v", r)
			}
		}
	}
	// Verified source changes preserve strict differences without granting new estimates.
	verified, err := s.HistoricalOverview(t.Context(), "2026-08-01", "2026-08-03")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range verified.Rows {
		if r.Kind == "holding" || r.Kind == "account" {
			if r.EstimatedBaseChange != nil || r.BaseChange == nil || *r.BaseChange != "0" {
				t.Fatalf("strict BaseChange changed %+v", r)
			}
		}
	}
}

func TestAnalysisBalanceDisplayMemoRetainsExactAndIsolatesCopies(t *testing.T) {
	t.Parallel()
	a := analysisAgentAccount("CNY", domain.TrackingBalance, domain.RoleAsset)
	p := domain.PortfolioSnapshot{Household: &domain.Household{BaseCurrency: "CNY"}}
	e := snapshotDisplayEvidence{balances: map[balanceDisplayKey]balanceDisplayValue{}}
	item := domain.DailyValuationSnapshotItem{AccountID: a.ID, NativeCurrency: "CNY", NativeAmount: "0.00006", BaseAmountExact: "0.00006", Complete: true}
	first, exact, err := analysisDisplayEnding(item, true, a, AnalysisInputs{Portfolio: p}, domain.AnalysisQuery{Valuation: domain.ValuationBase}, e, "2026-10-12")
	if err != nil {
		t.Fatal(err)
	}
	first.Total = nil
	first.Quality.Coverage = domain.CoverageProvisional
	next, nextExact, err := analysisDisplayEnding(item, true, a, AnalysisInputs{Portfolio: p}, domain.AnalysisQuery{Valuation: domain.ValuationBase}, e, "2026-10-13")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.balances) != 1 || next.Total == nil || next.Quality.Coverage != domain.CoverageVerified || exact.String() != "0.00006" || nextExact.String() != "0.00006" {
		t.Fatalf("memo lost exact value or leaked mutation: %+v %v %v", next, exact, nextExact)
	}
	a.BalanceSheetRole = domain.RoleLiability
	liability, signed, err := analysisDisplayEnding(item, true, a, AnalysisInputs{Portfolio: p}, domain.AnalysisQuery{Valuation: domain.ValuationBase}, e, "2026-10-13")
	if err != nil || len(e.balances) != 2 || liability.Total.Amount != "-0.0001" || signed.String() != "-0.00006" {
		t.Fatalf("memo leaked sign: %+v %v", liability, err)
	}
}
