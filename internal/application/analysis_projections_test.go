package application

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestAssetChangeProjectionReconcilesSummaryAndWaterfall(t *testing.T) {
	accountID := domain.AccountID("00000000-0000-0000-0000-000000000001")
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-01", To: "2026-08-02", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment}
	result := domain.PeriodAnalysisResult{Query: query, Status: domain.CompletenessOK, Days: []domain.ComponentDay{
		analysisTestDay(t, "2026-08-01", accountID, "100", map[domain.AttributionBucket]string{domain.BucketIncome: "10"}),
		analysisTestDay(t, "2026-08-02", accountID, "110", map[domain.AttributionBucket]string{domain.BucketPriceChange: "5"}),
	}}
	projection := foldAssetChange(result, "")
	if got := projection.Summary.BeginningValue.Amount().String(); got != "100" {
		t.Fatalf("beginning = %s, want 100", got)
	}
	if got := projection.Summary.EndingValue.Amount().String(); got != "115" {
		t.Fatalf("ending = %s, want 115", got)
	}
	if got := projection.Summary.Change.Amount().String(); got != "15" {
		t.Fatalf("change = %s, want 15", got)
	}
	waterfall := make(map[domain.AttributionBucket]string)
	for _, row := range projection.Waterfall {
		waterfall[row.Bucket] = row.Amount.Amount().String()
	}
	if waterfall[domain.BucketIncome] != "10" || waterfall[domain.BucketPriceChange] != "5" {
		t.Fatalf("waterfall = %#v, want income 10 and price 5", waterfall)
	}
}

func TestAssetDriverDetailResidualKeepsComponentAndDay(t *testing.T) {
	accountID := domain.AccountID("00000000-0000-0000-0000-000000000001")
	holdingID := domain.HoldingID("00000000-0000-0000-0000-000000000011")
	instrumentID := domain.InstrumentID("00000000-0000-0000-0000-000000000021")
	component := domain.ComponentID{AccountID: accountID, HoldingID: &holdingID, InstrumentID: &instrumentID, Currency: "USD", AssetClass: "equity"}
	residual := analysisSignedTestMoney(t, "-5")
	result := domain.PeriodAnalysisResult{Query: domain.AnalysisQuery{From: "2026-08-01", To: "2026-08-02"}, Days: []domain.ComponentDay{{
		Date:         "2026-08-02",
		Component:    component,
		Status:       domain.CompletenessPartial,
		AssetBuckets: map[domain.AttributionBucket]domain.SignedMoney{domain.BucketResidual: residual},
	}}}

	detail := foldAssetDriverDetail(result, "", string(domain.BucketResidual))
	if len(detail.ResidualDetails) != 1 {
		t.Fatalf("residual details = %+v, want one component/day detail", detail.ResidualDetails)
	}
	got := detail.ResidualDetails[0]
	if got.Date != "2026-08-02" || got.ComponentKey != component.Key() || got.AccountID != accountID.String() || got.HoldingID != holdingID.String() || got.InstrumentID != instrumentID.String() || got.Amount == nil || got.Amount.Amount().String() != "-5" {
		t.Fatalf("residual detail = %+v, want component/day identity and -5", got)
	}
}

func TestAssetChangeGroupsKeepResidualAsItsOwnOtherRow(t *testing.T) {
	accountID := domain.AccountID("00000000-0000-0000-0000-000000000001")
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-01", To: "2026-08-01", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment}
	day := analysisTestDay(t, "2026-08-01", accountID, "100", map[domain.AttributionBucket]string{domain.BucketResidual: "5"})
	result := foldAssetChange(domain.PeriodAnalysisResult{Query: query, Status: domain.CompletenessPartial, Days: []domain.ComponentDay{day}}, "")
	if len(result.Groups) != 1 || result.Groups[0].Key != "other" || len(result.Groups[0].Rows) != 1 || result.Groups[0].Rows[0].Bucket != domain.BucketResidual {
		t.Fatalf("groups = %+v, want residual as one row in other", result.Groups)
	}
}

func TestAssetChangeGroupsSeparateCashFlowsFromMarketAndInvestment(t *testing.T) {
	values := map[domain.AttributionBucket]string{
		domain.BucketExternalFlow:     "1",
		domain.BucketIncome:           "2",
		domain.BucketSpending:         "-3",
		domain.BucketDividendInterest: "4",
		domain.BucketPriceChange:      "5",
		domain.BucketFXImpact:         "-2",
		domain.BucketFee:              "-7",
		domain.BucketLiabilityImpact:  "8",
		domain.BucketAdjustment:       "9",
		domain.BucketResidual:         "-17",
	}
	waterfall := make(map[domain.AttributionBucket]decimal.Decimal, len(values))
	for bucket, value := range values {
		waterfall[bucket] = decimal.RequireFromString(value)
	}
	groups := foldAssetGroups(waterfall, "USD")
	if len(groups) != 3 {
		t.Fatalf("groups = %+v, want cash, market, other", groups)
	}
	want := map[string][]domain.AttributionBucket{
		"cash":   {domain.BucketExternalFlow, domain.BucketIncome, domain.BucketSpending},
		"market": {domain.BucketDividendInterest, domain.BucketPriceChange, domain.BucketFXImpact, domain.BucketFee},
		"other":  {domain.BucketLiabilityImpact, domain.BucketAdjustment, domain.BucketResidual},
	}
	for _, group := range groups {
		got := make([]domain.AttributionBucket, 0, len(group.Rows))
		for _, row := range group.Rows {
			got = append(got, row.Bucket)
		}
		if fmt.Sprint(got) != fmt.Sprint(want[group.Key]) {
			t.Fatalf("%s rows = %v, want %v", group.Key, got, want[group.Key])
		}
		if !group.Amount.Amount().IsZero() {
			t.Fatalf("%s amount = %s, want zero after cancellation", group.Key, group.Amount.Amount())
		}
	}
}

func TestAssetChangeAvailabilityUsesAssetDaysForCashOnlyAccount(t *testing.T) {
	householdID := domain.NewHouseholdID()
	account := domain.Account{ID: domain.NewAccountID(), HouseholdID: householdID, Name: "Cash", AccountType: domain.TypeBankAccount, BalanceSheetRole: domain.RoleAsset, TrackingMode: domain.TrackingBalance, DefaultCurrency: "USD", IncludeInNetWorth: true}
	origin := &domain.HistoryOrigin{HouseholdID: householdID, Timezone: "UTC", StartedAt: time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC)}
	portfolio := domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: "USD"}, Origin: origin, Accounts: []domain.AccountRecord{{Account: account}}}
	income, err := domain.ParseMoney("10", "USD")
	if err != nil {
		t.Fatal(err)
	}
	activityID := domain.NewActivityID()
	activity := domain.Activity{ID: activityID, HouseholdID: householdID, Kind: domain.ActivityCashIn, Reason: domain.ReasonIncome, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02", Effects: []domain.ActivityEffect{{ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 1, Role: domain.EffectRoleAmount, Direction: domain.EffectAdded, Target: domain.EffectTargetAccountCash, Classification: domain.ClassificationIncome, AccountID: &account.ID, Money: &income}}}
	snapshot := func(date, amount string) domain.DailyValuationSnapshot {
		return domain.DailyValuationSnapshot{HouseholdID: householdID, LocalDate: date, CutoffAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC), Complete: true, Items: []domain.DailyValuationSnapshotItem{{AccountID: account.ID, NativeAmount: amount, NativeCurrency: "USD", BaseAmountExact: amount, Complete: true}}}
	}
	repository := &projectionRepository{portfolio: portfolio, snapshots: []domain.DailyValuationSnapshot{snapshot("2026-07-31", "100"), snapshot("2026-08-01", "100"), snapshot("2026-08-02", "110")}, activities: []domain.Activity{activity}}
	service := &Service{analysis: NewAnalysisService(repository, func() time.Time { return time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC) })}
	query := analysisMemoTestQuery("2026-08-01", "2026-08-02")
	query.IncludeCash = false
	projection, err := service.AssetChange(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if !projection.Available || projection.Status != domain.CompletenessOK || projection.MissingReason != "" {
		t.Fatalf("cash-only availability = %+v, want available ok", projection.AnalysisAvailability)
	}
	if projection.Summary.BeginningValue == nil || projection.Summary.BeginningValue.Amount().String() != "100" || projection.Summary.EndingValue == nil || projection.Summary.EndingValue.Amount().String() != "110" {
		t.Fatalf("cash-only summary = %+v, want 100 -> 110", projection.Summary)
	}
	if len(projection.Waterfall) != 1 || projection.Waterfall[0].Bucket != domain.BucketIncome || projection.Waterfall[0].Amount.Amount().String() != "10" {
		t.Fatalf("cash-only waterfall = %+v, want income 10", projection.Waterfall)
	}
}

func TestAssetChangeAvailabilityIgnoresInvestmentReturnCoverage(t *testing.T) {
	householdID := domain.NewHouseholdID()
	account := analysisAccount(householdID, "CNY", domain.TrackingHoldings, domain.RoleAsset)
	instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: householdID, Name: "QQQ", Type: domain.InstrumentETF, QuoteCurrency: "CNY"}
	state := reviewChangeState(t, householdID, reviewAccountState(t, account, "0"))
	state.Cash[account.ID] = map[domain.CurrencyCode]domain.Money{"CNY": mustMoney(t, "0", "CNY")}
	state.Holdings[holdingID] = domain.ChangeHoldingState{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID, InstrumentName: instrument.Name, Currency: "CNY", Current: mustQuantity(t, "1")}
	state.Instruments[instrumentID] = instrument
	sell := reviewApplyChange(t, &state, domain.TradeInput{HouseholdID: householdID, Side: domain.TradeSell, SettlementAccountID: account.ID, HoldingID: holdingID, InstrumentID: instrumentID, Quantity: mustQuantity(t, "1"), Gross: mustMoney(t, "100", "CNY"), EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)})
	openQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "100"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
	closeQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "100"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	holdingItem := func(amount, quoteID string) domain.DailyValuationSnapshotItem {
		return analysisItem(t, account.ID, "CNY", amount, amount, &holdingID, &instrumentID, quoteID, "")
	}
	cashItem := func(amount string) domain.DailyValuationSnapshotItem {
		return analysisItem(t, account.ID, "CNY", amount, amount, nil, nil, "", "")
	}
	input := AnalysisInputs{
		Origin: analysisOrigin(t, householdID, "UTC"),
		Portfolio: domain.PortfolioSnapshot{
			Household:   &domain.Household{ID: householdID, BaseCurrency: "CNY"},
			Accounts:    []domain.AccountRecord{{Account: account}},
			Instruments: []domain.Instrument{instrument},
			Holdings:    []domain.Holding{{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}},
		},
		Snapshots: []domain.DailyValuationSnapshot{
			analysisSnapshot("2026-08-01", cashItem("0"), holdingItem("100", openQuote.ID.String())),
			analysisSnapshot("2026-08-02", cashItem("100"), holdingItem("0", closeQuote.ID.String())),
			analysisSnapshot("2026-08-03", cashItem("100"), holdingItem("0", closeQuote.ID.String())),
		},
		Activities:       []domain.Activity{sell},
		InstrumentQuotes: []domain.InstrumentQuote{openQuote, closeQuote},
	}
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-02", To: "2026-08-03", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: false}
	result, err := ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status == domain.CompletenessOK {
		t.Fatalf("return status = %s, want partial or unavailable after a zero-capital day", result.Status)
	}
	for _, day := range result.Days {
		if day.Status != domain.CompletenessOK {
			t.Fatalf("component day %s/%s status = %s, want complete asset identity", day.Date, day.Component.Key(), day.Status)
		}
	}
	projection := foldAssetChange(result, "")
	if !projection.Available || projection.Status != domain.CompletenessOK || projection.MissingReason != "" {
		t.Fatalf("asset-day availability = %+v, want available ok despite %s return coverage", projection.AnalysisAvailability, result.Status)
	}
	if projection.Summary.BeginningValue == nil || projection.Summary.BeginningValue.Amount().String() != "100" || projection.Summary.EndingValue == nil || projection.Summary.EndingValue.Amount().String() != "100" {
		t.Fatalf("asset-day summary = %+v, want 100 -> 100 after a complete sell into cash", projection.Summary)
	}
}

func TestAssetChangeProjectionUsesRecordedEndingValueForInternalTransfers(t *testing.T) {
	first := domain.AccountID("00000000-0000-0000-0000-000000000001")
	second := domain.AccountID("00000000-0000-0000-0000-000000000002")
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-01", To: "2026-08-01", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment}
	left := analysisTestDay(t, "2026-08-01", first, "100", nil)
	right := analysisTestDay(t, "2026-08-01", second, "0", nil)
	left.EndingValue = analysisSignedTestMoney(t, "90")
	right.EndingValue = analysisSignedTestMoney(t, "10")
	result := foldAssetChange(domain.PeriodAnalysisResult{Query: query, Status: domain.CompletenessOK, Days: []domain.ComponentDay{left, right}}, "")
	if got := result.Summary.BeginningValue.Amount().String(); got != "100" {
		t.Fatalf("beginning = %s, want 100", got)
	}
	if got := result.Summary.EndingValue.Amount().String(); got != "100" {
		t.Fatalf("ending = %s, want 100 after internal transfer", got)
	}
	if result.Summary.Change.Amount().Sign() != 0 || len(result.Waterfall) != 0 {
		t.Fatalf("internal transfer changed asset projection: %+v", result)
	}
}

func TestAssetTrendUsesPeriodEndForLevelsAndSumsFlows(t *testing.T) {
	firstAccountID := domain.AccountID("00000000-0000-0000-0000-000000000001")
	secondAccountID := domain.AccountID("00000000-0000-0000-0000-000000000002")
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-01", To: "2026-08-02", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment}
	result := domain.PeriodAnalysisResult{Query: query, AnalysisDayTimezone: "UTC", Status: domain.CompletenessOK, Days: []domain.ComponentDay{
		analysisTestDay(t, "2026-08-01", firstAccountID, "100", map[domain.AttributionBucket]string{domain.BucketIncome: "10"}),
		analysisTestDay(t, "2026-08-01", secondAccountID, "50", map[domain.AttributionBucket]string{domain.BucketIncome: "5"}),
		analysisTestDay(t, "2026-08-02", firstAccountID, "110", map[domain.AttributionBucket]string{domain.BucketIncome: "7"}),
		analysisTestDay(t, "2026-08-02", secondAccountID, "55", map[domain.AttributionBucket]string{domain.BucketIncome: "5"}),
	}}
	levels, err := foldAssetTrend(result, "", TrendMonth, TrendNetWorth)
	if err != nil {
		t.Fatal(err)
	}
	if len(levels.Points) != 1 || levels.Points[0].Value.Amount().String() != "177" {
		t.Fatalf("monthly level = %#v, want period-end 177 across both components", levels.Points)
	}
	flows, err := foldAssetTrend(result, "", TrendMonth, TrendIncome)
	if err != nil {
		t.Fatal(err)
	}
	if len(flows.Points) != 1 || flows.Points[0].Value.Amount().String() != "27" {
		t.Fatalf("monthly flow = %#v, want sum 27 across both components", flows.Points)
	}
	dailyFlows, err := foldAssetTrend(result, "", TrendDay, TrendIncome)
	if err != nil {
		t.Fatal(err)
	}
	if dailyFlows.Summary == nil || dailyFlows.Summary.Amount().String() != "27" {
		t.Fatalf("multi-period flow summary = %+v, want 27", dailyFlows.Summary)
	}
}

func TestAssetTrendReturnRateGeometricallyLinksDailyRates(t *testing.T) {
	accountID := domain.AccountID("00000000-0000-0000-0000-000000000001")
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-01", To: "2026-08-02", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment}
	rateOne, _ := decimal.NewFromString("0.10")
	rateTwo, _ := decimal.NewFromString("0.20")
	linked, _ := decimal.NewFromString("0.32")
	result := domain.PeriodAnalysisResult{
		Query:               query,
		AnalysisDayTimezone: "UTC",
		Status:              domain.CompletenessOK,
		Days: []domain.ComponentDay{
			analysisTestDay(t, "2026-08-01", accountID, "100", nil),
			analysisTestDay(t, "2026-08-02", accountID, "100", nil),
		},
		DailyReturns: []domain.DailyReturn{
			{Date: "2026-08-01", Rate: &rateOne, Status: domain.CompletenessOK},
			{Date: "2026-08-02", Rate: &rateTwo, Status: domain.CompletenessOK},
		},
		Coverage:   domain.RateCoverage{RatedDays: 2, TotalDays: 2},
		ReturnRate: &linked,
	}
	trend, err := foldAssetTrend(result, "", TrendMonth, TrendReturnRate)
	if err != nil {
		t.Fatal(err)
	}
	if len(trend.Points) != 1 || trend.Points[0].Rate == nil || !trend.Points[0].Rate.Equal(linked) {
		t.Fatalf("monthly return rate = %#v, want geometric 0.32", trend.Points)
	}
	if trend.Points[0].Coverage != (domain.RateCoverage{RatedDays: 2, TotalDays: 2}) {
		t.Fatalf("monthly coverage = %+v, want 2/2", trend.Points[0].Coverage)
	}
}

func TestAssetTrendReturnRateOmitsMinorityCoverage(t *testing.T) {
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-01", To: "2026-08-03", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment}
	rate := decimal.NewFromInt(1).Div(decimal.NewFromInt(10))
	result := domain.PeriodAnalysisResult{
		Query: query, AnalysisDayTimezone: "UTC", Status: domain.CompletenessOK,
		DailyReturns: []domain.DailyReturn{
			{Date: "2026-08-01", Rate: &rate, Status: domain.CompletenessOK},
			{Date: "2026-08-02", Status: domain.CompletenessOK},
			{Date: "2026-08-03", Status: domain.CompletenessOK},
		},
		Coverage: domain.RateCoverage{RatedDays: 1, TotalDays: 3},
	}
	trend, err := foldAssetTrend(result, "", TrendMonth, TrendReturnRate)
	if err != nil {
		t.Fatal(err)
	}
	if len(trend.Points) != 1 || trend.Points[0].Rate != nil || trend.Points[0].Status != domain.CompletenessPartial || trend.Points[0].Coverage != (domain.RateCoverage{RatedDays: 1, TotalDays: 3}) {
		t.Fatalf("minority rate coverage = %+v, want partial 1/3 with nil rate", trend.Points)
	}
}

func TestAnalysisMemoIsCanonicalAndBounded(t *testing.T) {
	repository := &emptyAnalysisRepository{}
	service := NewAnalysisService(repository, func() time.Time { return time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC) })
	query := analysisMemoTestQuery("2026-08-01", "2026-08-02")
	if _, err := service.Compute(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Compute(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	if repository.reads != 1 {
		t.Fatalf("repository reads = %d, want one cached compute", repository.reads)
	}
	if _, err := service.Compute(context.Background(), analysisMemoTestQuery("2026-08-02", "2026-08-02")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Compute(context.Background(), analysisMemoTestQuery("2026-08-03", "2026-08-03")); err != nil {
		t.Fatal(err)
	}
	if service.MemoEntryCount() != 2 {
		t.Fatalf("memo entries = %d, want bounded capacity 2", service.MemoEntryCount())
	}
	if _, err := service.Compute(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	if repository.reads != 4 {
		t.Fatalf("LRU eviction reads = %d, want 4 after the oldest query is evicted", repository.reads)
	}
	service.Invalidate()
	if service.AnalysisDataGeneration() != 1 || service.MemoEntryCount() != 0 {
		t.Fatalf("invalidation did not advance generation and clear memo")
	}
}

func TestAnalysisQueryHashCanonicalizesAssetClass(t *testing.T) {
	upper := analysisMemoTestQuery("2026-08-01", "2026-08-02")
	upper.Filters.AssetClass = " Cash "
	lower := upper
	lower.Filters.AssetClass = "cash"
	if analysisQueryHash(upper) != analysisQueryHash(lower) {
		t.Fatal("equivalent asset-class filters produced different memo keys")
	}
}

func TestDividendCategoriesUseAssociatedReturnEffects(t *testing.T) {
	accountID := domain.AccountID("00000000-0000-0000-0000-000000000001")
	holdingID := domain.HoldingID("00000000-0000-0000-0000-000000000011")
	instrumentID := domain.InstrumentID("00000000-0000-0000-0000-000000000021")
	returnComponent := domain.ReturnDividendInterest
	dividendActivity := domain.ActivityID("dividend-activity")
	dividend := analysisSignedTestMoney(t, "70")
	holding := domain.ComponentID{AccountID: accountID, HoldingID: &holdingID, InstrumentID: &instrumentID, Currency: "USD", AssetClass: "equity"}
	holdingDay := domain.ComponentDay{
		Date:         "2026-08-02",
		Component:    holding,
		Status:       domain.CompletenessOK,
		AssetBuckets: map[domain.AttributionBucket]domain.SignedMoney{},
		AttributedEffects: []domain.AttributedEffect{{
			ReturnComponent:     &returnComponent,
			Amount:              dividend,
			SourceEffect:        domain.ActivityEffect{ActivityID: dividendActivity},
			Component:           holding,
			RelatedHoldingID:    &holdingID,
			RelatedInstrumentID: &instrumentID,
		}},
	}
	interestActivity := domain.ActivityID("interest-activity")
	interest := analysisSignedTestMoney(t, "5")
	dividendBucket := domain.BucketDividendInterest
	cash := domain.ComponentID{AccountID: accountID, Currency: "USD", Cash: true}
	cashDay := domain.ComponentDay{
		Date:         "2026-08-02",
		Component:    cash,
		Status:       domain.CompletenessOK,
		AssetBuckets: map[domain.AttributionBucket]domain.SignedMoney{dividendBucket: interest},
		AttributedEffects: []domain.AttributedEffect{{
			AssetBucket:     &dividendBucket,
			ReturnComponent: &returnComponent,
			Amount:          interest,
			SourceEffect:    domain.ActivityEffect{ActivityID: interestActivity},
			Component:       cash,
		}},
	}
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-02", To: "2026-08-02", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment}
	result := domain.PeriodAnalysisResult{Query: query, Status: domain.CompletenessOK, Days: []domain.ComponentDay{holdingDay, cashDay}}
	categories := foldCategories(result, "", CategoryDividendInterest)
	if categories.Total == nil || !categories.Total.Amount().Equal(decimal.NewFromInt(75)) {
		t.Fatalf("dividend category total = %+v, want 75", categories.Total)
	}
	if len(categories.Rows) != 2 || categories.Rows[0].Key != instrumentID.String() || categories.Rows[1].Key != domain.BucketCash {
		t.Fatalf("dividend category rows = %+v", categories.Rows)
	}
	detail := foldCategoryDetail(result, "", CategoryDividendInterest, instrumentID.String())
	if len(detail.ActivityRefs) != 1 || detail.ActivityRefs[0].InstrumentID != instrumentID.String() || detail.ActivityRefs[0].HoldingID != holdingID.String() {
		t.Fatalf("associated dividend detail refs = %+v", detail.ActivityRefs)
	}
	if len(detail.Children) != 1 || !detail.Children[0].Amount.Amount().Equal(decimal.NewFromInt(70)) {
		t.Fatalf("associated dividend detail children = %+v", detail.Children)
	}
}

func TestCategoriesRanksRowsBySignedAmount(t *testing.T) {
	smallAccount := domain.AccountID("00000000-0000-0000-0000-000000000001")
	largeAccount := domain.AccountID("00000000-0000-0000-0000-000000000002")
	spending := domain.BucketSpending
	small := analysisSignedTestMoney(t, "5")
	large := analysisSignedTestMoney(t, "50")
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-02", To: "2026-08-02", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment}
	result := domain.PeriodAnalysisResult{Query: query, Status: domain.CompletenessOK, Days: []domain.ComponentDay{
		{Date: "2026-08-02", Component: domain.ComponentID{AccountID: smallAccount, Currency: "USD"}, Status: domain.CompletenessOK, AssetBuckets: map[domain.AttributionBucket]domain.SignedMoney{spending: small}},
		{Date: "2026-08-02", Component: domain.ComponentID{AccountID: largeAccount, Currency: "USD"}, Status: domain.CompletenessOK, AssetBuckets: map[domain.AttributionBucket]domain.SignedMoney{spending: large}},
	}}
	categories := foldCategories(result, "", CategorySpending)
	if len(categories.Rows) != 2 || categories.Rows[0].Key != largeAccount.String() || categories.Rows[1].Key != smallAccount.String() {
		t.Fatalf("category ranking = %+v, want amount desc not key order", categories.Rows)
	}
	holdingSmall, holdingLarge := domain.NewHoldingID(), domain.NewHoldingID()
	instrumentID := domain.NewInstrumentID()
	price := domain.BucketPriceChange
	smallHolding := domain.ComponentID{AccountID: largeAccount, HoldingID: &holdingSmall, InstrumentID: &instrumentID, Currency: "USD", AssetClass: "equity"}
	largeHolding := domain.ComponentID{AccountID: largeAccount, HoldingID: &holdingLarge, InstrumentID: &instrumentID, Currency: "USD", AssetClass: "equity"}
	detailResult := domain.PeriodAnalysisResult{Query: query, Status: domain.CompletenessOK, Days: []domain.ComponentDay{
		{Date: "2026-08-02", Component: smallHolding, Status: domain.CompletenessOK, AssetBuckets: map[domain.AttributionBucket]domain.SignedMoney{price: small}},
		{Date: "2026-08-02", Component: largeHolding, Status: domain.CompletenessOK, AssetBuckets: map[domain.AttributionBucket]domain.SignedMoney{price: large}},
	}}
	detail := foldCategoryDetail(detailResult, "", CategoryInvestmentReturn, instrumentID.String())
	if len(detail.Children) != 2 || !detail.Children[0].Amount.Amount().Equal(decimal.NewFromInt(50)) || !detail.Children[1].Amount.Amount().Equal(decimal.NewFromInt(5)) {
		t.Fatalf("category children ranking = %+v, want amount desc", detail.Children)
	}
}

func TestAssetChangeForcesBaseForMultiCurrencyNativeQuery(t *testing.T) {
	householdID := domain.NewHouseholdID()
	usdAccount := domain.Account{ID: domain.NewAccountID(), HouseholdID: householdID, Name: "USD bank", AccountType: domain.TypeBankAccount, BalanceSheetRole: domain.RoleAsset, TrackingMode: domain.TrackingBalance, DefaultCurrency: "USD", IncludeInNetWorth: true}
	cnyAccount := domain.Account{ID: domain.NewAccountID(), HouseholdID: householdID, Name: "CNY bank", AccountType: domain.TypeBankAccount, BalanceSheetRole: domain.RoleAsset, TrackingMode: domain.TrackingBalance, DefaultCurrency: "CNY", IncludeInNetWorth: true}
	origin := &domain.HistoryOrigin{HouseholdID: householdID, Timezone: "UTC", StartedAt: time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC)}
	portfolio := domain.PortfolioSnapshot{
		Household: &domain.Household{ID: householdID, BaseCurrency: "USD"},
		Origin:    origin,
		Accounts:  []domain.AccountRecord{{Account: usdAccount}, {Account: cnyAccount}},
	}
	fxRate, err := domain.ParseFxRate("7")
	if err != nil {
		t.Fatal(err)
	}
	fxQuote, err := domain.NewFXQuote(domain.FXQuoteInput{HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: fxRate, SourceKind: domain.QuoteSourceManual, QuotedAt: time.Date(2026, 7, 31, 23, 0, 0, 0, time.UTC)}, time.Date(2026, 7, 31, 23, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func(date string) domain.DailyValuationSnapshot {
		return domain.DailyValuationSnapshot{HouseholdID: householdID, LocalDate: date, CutoffAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC), Complete: true, Items: []domain.DailyValuationSnapshotItem{
			{AccountID: usdAccount.ID, NativeAmount: "100", NativeCurrency: "USD", BaseAmountExact: "100", Complete: true},
			{AccountID: cnyAccount.ID, NativeAmount: "100", NativeCurrency: "CNY", BaseAmountExact: "14.2857142857142857", Complete: true},
		}}
	}
	repository := &projectionRepository{portfolio: portfolio, snapshots: []domain.DailyValuationSnapshot{snapshot("2026-07-31"), snapshot("2026-08-01"), snapshot("2026-08-02")}, fxQuotes: []domain.FXQuote{fxQuote}}
	service := &Service{analysis: NewAnalysisService(repository, func() time.Time { return time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC) })}
	query := analysisMemoTestQuery("2026-08-01", "2026-08-02")
	query.Valuation = domain.ValuationNative
	query.IncludeCash = true
	projection, err := service.AssetChange(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if projection.ValuationForced == nil || *projection.ValuationForced != string(domain.ValuationBase) {
		t.Fatalf("valuationForced = %v, want base", projection.ValuationForced)
	}
	if projection.Summary.BeginningValue == nil || projection.Summary.BeginningValue.Currency() != "USD" {
		t.Fatalf("forced summary beginning = %+v, want USD", projection.Summary.BeginningValue)
	}
	firstReads := repository.reads
	if firstReads < 1 || firstReads > 2 {
		t.Fatalf("native fallback repository reads = %d, want one or two initial reads", firstReads)
	}
	repeated, err := service.AssetChange(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if repeated.ValuationForced == nil || *repeated.ValuationForced != string(domain.ValuationBase) {
		t.Fatalf("repeated valuationForced = %v, want base", repeated.ValuationForced)
	}
	if repository.reads != firstReads {
		t.Fatalf("repeated native fallback repository reads = %d, want unchanged at %d", repository.reads, firstReads)
	}
}

func TestAnalysisCase44MemoInvalidatesAfterActivityAndSnapshotBuild(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/analysis-memo.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	service := NewService(sqlite.NewRepository(database))
	clock := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Memo", BaseCurrency: "USD", MemberNames: []string{"Owner"}, Timezone: "UTC"}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "100", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-02", To: "2026-08-03", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true}
	before, err := service.AssetChange(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if before.Summary.EndingValue == nil || !before.Summary.EndingValue.Amount().Equal(decimal.NewFromInt(100)) {
		t.Fatalf("before activity ending = %+v, want 100", before.Summary.EndingValue)
	}
	beforeGeneration := service.analysis.AnalysisDataGeneration()
	amount, err := domain.ParseMoney("10", "USD")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: amount, Reason: domain.ReasonIncome, EffectiveAt: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	if service.analysis.AnalysisDataGeneration() <= beforeGeneration || service.analysis.MemoEntryCount() != 0 {
		t.Fatalf("activity mutation did not invalidate memo: generation %d -> %d, entries=%d", beforeGeneration, service.analysis.AnalysisDataGeneration(), service.analysis.MemoEntryCount())
	}
	state, err := service.DailySnapshotState(ctx, bootstrap.Household.ID)
	if err != nil || state.DirtyFrom == nil || *state.DirtyFrom != "2026-08-03" {
		t.Fatalf("dirty snapshot marker after activity = %+v, err=%v, want 2026-08-03", state, err)
	}
	after, err := service.AssetChange(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if after.Summary.EndingValue == nil || !after.Summary.EndingValue.Amount().Equal(decimal.NewFromInt(110)) {
		t.Fatalf("after activity ending = %+v, want 110", after.Summary.EndingValue)
	}
	state, err = service.DailySnapshotState(ctx, bootstrap.Household.ID)
	if err != nil || state.DirtyFrom == nil || *state.DirtyFrom != "2026-08-04" {
		t.Fatalf("dirty snapshot marker after analysis rebuild = %+v, err=%v, want 2026-08-04", state, err)
	}
	if _, err := service.AssetChange(ctx, query); err != nil {
		t.Fatal(err)
	}
	if service.analysis.MemoEntryCount() != 1 {
		t.Fatalf("memo entries after recompute = %d, want 1", service.analysis.MemoEntryCount())
	}
	rebuildGeneration := service.analysis.AnalysisDataGeneration()
	if _, appended, err := service.BuildDailyValuationSnapshot(ctx, "2026-08-04"); err != nil || !appended {
		t.Fatalf("snapshot rebuild append = %v, err=%v", appended, err)
	}
	state, err = service.DailySnapshotState(ctx, bootstrap.Household.ID)
	if err != nil || state.DirtyFrom == nil || *state.DirtyFrom != "2026-08-05" {
		t.Fatalf("dirty snapshot marker after snapshot build = %+v, err=%v, want 2026-08-05", state, err)
	}
	if service.analysis.AnalysisDataGeneration() <= rebuildGeneration || service.analysis.MemoEntryCount() != 0 {
		t.Fatalf("snapshot mutation did not invalidate memo: generation %d -> %d, entries=%d", rebuildGeneration, service.analysis.AnalysisDataGeneration(), service.analysis.MemoEntryCount())
	}
	rebuilt, err := service.AssetChange(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.Summary.EndingValue == nil || !rebuilt.Summary.EndingValue.Amount().Equal(decimal.NewFromInt(110)) {
		t.Fatalf("after snapshot rebuild ending = %+v, want 110", rebuilt.Summary.EndingValue)
	}
}

func TestAnalysisCase44SnapshotRebuildChangesWindowResult(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/analysis-snapshot-rebuild.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	service := NewService(sqlite.NewRepository(database))
	clock := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Memo", BaseCurrency: "USD", MemberNames: []string{"Owner"}, Timezone: "UTC"}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "QQQ", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "100", "2026-08-01T00:00:00Z", false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "1"}); err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-02", To: "2026-08-03", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true}
	before, err := service.AssetChange(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if before.Summary.EndingValue == nil || !before.Summary.EndingValue.Amount().Equal(decimal.NewFromInt(100)) {
		t.Fatalf("before quote rebuild ending = %+v, want 100", before.Summary.EndingValue)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "150", "2026-08-03T12:00:00Z", false); err != nil {
		t.Fatal(err)
	}
	if _, appended, err := service.BuildDailyValuationSnapshot(ctx, "2026-08-03"); err != nil || !appended {
		t.Fatalf("in-window snapshot rebuild append = %v, err=%v", appended, err)
	}
	after, err := service.AssetChange(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if after.Summary.EndingValue == nil || !after.Summary.EndingValue.Amount().Equal(decimal.NewFromInt(150)) {
		t.Fatalf("after snapshot rebuild ending = %+v, want 150", after.Summary.EndingValue)
	}
}

// TestAnalysisPerformanceBudgets enforces architecture §18.1. The 3Y/500-component
// cold path is skipped under the race detector because that instrumentation
// changes wall-clock cost by more than the budget.
func TestAnalysisPerformanceBudgets(t *testing.T) {
	t.Run("warm-memo", func(t *testing.T) {
		input, query := analysisSizedInputs(50, "2025-12-01", "2025-12-31")
		repository := &projectionRepository{portfolio: input.Portfolio, snapshots: input.Snapshots}
		service := NewAnalysisService(repository, func() time.Time { return time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) })
		ctx := context.Background()
		if _, err := service.Compute(ctx, query); err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		if _, err := service.Compute(ctx, query); err != nil {
			t.Fatal(err)
		}
		if elapsed := time.Since(started); elapsed >= 100*time.Millisecond {
			t.Fatalf("warm memo took %s, want < 100ms", elapsed)
		}
	})
	t.Run("cold-month", func(t *testing.T) {
		input, query := analysisSizedInputs(200, "2025-12-01", "2025-12-31")
		budget := 300 * time.Millisecond
		if analysisRaceDetector {
			budget = 2 * time.Second
		}
		started := time.Now()
		if _, err := ComputeAnalysis(input, query); err != nil {
			t.Fatal(err)
		}
		if elapsed := time.Since(started); elapsed >= budget {
			t.Fatalf("cold 1-month/200-component took %s, want < %s", elapsed, budget)
		}
	})
	t.Run("cold-3y", func(t *testing.T) {
		if analysisRaceDetector {
			t.Skip("§18.1 production 3s / Linux CI 7s budget is a production-runtime number; -race inflates wall-clock cost")
		}
		input, query := analysisUpperBoundInputs()
		started := time.Now()
		if _, err := ComputeAnalysis(input, query); err != nil {
			t.Fatal(err)
		}
		if elapsed := time.Since(started); elapsed >= 7*time.Second {
			t.Fatalf("cold 3Y/500-component took %s, want < 7s", elapsed)
		}
	})
}

// BenchmarkAnalysisColdComputeUpperBound is intentionally a benchmark rather
// than a wall-clock assertion in the regular test suite. It keeps the analysis
// performance budget reproducible without making CI depend on host load.
func BenchmarkAnalysisColdComputeUpperBound(b *testing.B) {
	input, query := analysisUpperBoundInputs()
	b.ReportMetric(float64(len(input.Snapshots)*len(input.Portfolio.Accounts)), "snapshot-components")
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := ComputeAnalysis(input, query); err != nil {
			b.Fatal(err)
		}
	}
}

func analysisUpperBoundInputs() (AnalysisInputs, domain.AnalysisQuery) {
	return analysisSizedInputs(500, "2023-01-01", "2025-12-31")
}

func analysisSizedInputs(componentCount int, startDate, endDate string) (AnalysisInputs, domain.AnalysisQuery) {
	householdID := domain.HouseholdID("00000000-0000-0000-0000-000000000100")
	accounts := make([]domain.AccountRecord, 0, componentCount)
	accountIDs := make([]domain.AccountID, 0, componentCount)
	for index := 0; index < componentCount; index++ {
		accountID := domain.AccountID(fmt.Sprintf("00000000-0000-0000-0000-%012d", index+1))
		accountIDs = append(accountIDs, accountID)
		accounts = append(accounts, domain.AccountRecord{Account: domain.Account{ID: accountID, HouseholdID: householdID, Name: fmt.Sprintf("Bank %d", index+1), AccountType: domain.TypeBankAccount, BalanceSheetRole: domain.RoleAsset, TrackingMode: domain.TrackingBalance, DefaultCurrency: "USD", IncludeInNetWorth: true}})
	}
	origin := &domain.HistoryOrigin{HouseholdID: householdID, Timezone: "UTC", StartedAt: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)}
	portfolio := domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: "USD"}, Origin: origin, Accounts: accounts}
	start, _ := time.Parse("2006-01-02", startDate)
	end, _ := time.Parse("2006-01-02", endDate)
	snapshots := make([]domain.DailyValuationSnapshot, 0)
	for date := start.AddDate(0, 0, -1); !date.After(end); date = date.AddDate(0, 0, 1) {
		localDate := date.Format("2006-01-02")
		items := make([]domain.DailyValuationSnapshotItem, 0, componentCount)
		for _, accountID := range accountIDs {
			items = append(items, domain.DailyValuationSnapshotItem{AccountID: accountID, NativeAmount: "100", NativeCurrency: "USD", BaseAmountExact: "100", Complete: true})
		}
		snapshots = append(snapshots, domain.DailyValuationSnapshot{HouseholdID: householdID, LocalDate: localDate, CutoffAt: date.Add(24*time.Hour - time.Millisecond), Complete: true, Items: items})
	}
	return AnalysisInputs{Origin: *origin, Portfolio: portfolio, Snapshots: snapshots}, domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: domain.LocalDate(startDate), To: domain.LocalDate(endDate), Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true}
}

func analysisTestDay(t *testing.T, date string, accountID domain.AccountID, beginning string, buckets map[domain.AttributionBucket]string) domain.ComponentDay {
	t.Helper()
	value, err := decimal.NewFromString(beginning)
	if err != nil {
		t.Fatal(err)
	}
	money := analysisSignedTestMoney(t, value.String())
	day := domain.ComponentDay{Date: date, Component: domain.ComponentID{AccountID: accountID, Currency: domain.CurrencyCode("USD"), Cash: true}, BeginningValue: money, Status: domain.CompletenessOK, AssetBuckets: map[domain.AttributionBucket]domain.SignedMoney{}}
	for bucket, amount := range buckets {
		parsed, parseErr := decimal.NewFromString(amount)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		signed, signedErr := domain.NewSignedMoney(parsed, domain.CurrencyCode("USD"))
		if signedErr != nil {
			t.Fatal(signedErr)
		}
		day.AssetBuckets[bucket] = signed
	}
	ending := value
	for _, amount := range day.AssetBuckets {
		ending = ending.Add(amount.Amount())
	}
	day.EndingValue = analysisSignedTestMoney(t, ending.String())
	return day
}

func analysisSignedTestMoney(t *testing.T, amount string) domain.SignedMoney {
	t.Helper()
	value, err := decimal.NewFromString(amount)
	if err != nil {
		t.Fatal(err)
	}
	money, err := domain.NewSignedMoney(value, domain.CurrencyCode("USD"))
	if err != nil {
		t.Fatal(err)
	}
	return money
}

func analysisMemoTestQuery(from, to string) domain.AnalysisQuery {
	return domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: from, To: to, Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment}
}

type emptyAnalysisRepository struct {
	Repository
	reads int
}

type projectionRepository struct {
	Repository
	portfolio  domain.PortfolioSnapshot
	snapshots  []domain.DailyValuationSnapshot
	activities []domain.Activity
	fxQuotes   []domain.FXQuote
	reads      int
}

func (r *projectionRepository) ReadPortfolioSnapshot(context.Context, domain.AccountFilter) (domain.PortfolioSnapshot, error) {
	r.reads++
	return r.portfolio, nil
}

func (r *projectionRepository) ListDailyValuationSnapshots(_ context.Context, _ domain.HouseholdID, since, until time.Time) ([]domain.DailyValuationSnapshot, error) {
	if since.IsZero() && until.IsZero() {
		return r.snapshots, nil
	}
	start, end := "", ""
	if !since.IsZero() {
		start = since.Format("2006-01-02")
	}
	if !until.IsZero() {
		end = until.Format("2006-01-02")
	}
	filtered := make([]domain.DailyValuationSnapshot, 0, len(r.snapshots))
	for _, snapshot := range r.snapshots {
		if start != "" && snapshot.LocalDate < start {
			continue
		}
		if end != "" && snapshot.LocalDate > end {
			continue
		}
		filtered = append(filtered, snapshot)
	}
	return filtered, nil
}

func (r *projectionRepository) ListActivitiesUntil(context.Context, domain.HouseholdID, time.Time) ([]domain.Activity, error) {
	return r.activities, nil
}

func (r *projectionRepository) ListFXQuotes(context.Context, domain.HouseholdID) ([]domain.FXQuote, error) {
	return r.fxQuotes, nil
}

func (r *emptyAnalysisRepository) ReadPortfolioSnapshot(context.Context, domain.AccountFilter) (domain.PortfolioSnapshot, error) {
	r.reads++
	started := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	origin := &domain.HistoryOrigin{HouseholdID: "00000000-0000-0000-0000-000000000010", Timezone: "UTC", StartedAt: started}
	household := &domain.Household{ID: origin.HouseholdID, BaseCurrency: domain.CurrencyCode("USD")}
	return domain.PortfolioSnapshot{Household: household, Origin: origin}, nil
}

func (r *emptyAnalysisRepository) ListDailyValuationSnapshots(context.Context, domain.HouseholdID, time.Time, time.Time) ([]domain.DailyValuationSnapshot, error) {
	return []domain.DailyValuationSnapshot{}, nil
}

func (r *emptyAnalysisRepository) ListActivitiesUntil(context.Context, domain.HouseholdID, time.Time) ([]domain.Activity, error) {
	return []domain.Activity{}, nil
}

func (r *emptyAnalysisRepository) ListFXQuotes(context.Context, domain.HouseholdID) ([]domain.FXQuote, error) {
	return []domain.FXQuote{}, nil
}
