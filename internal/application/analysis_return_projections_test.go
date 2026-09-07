package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReturnCalendarProjectsCompositionCoverageAndIssues(t *testing.T) {
	account := domain.NewAccountID()
	component := domain.ComponentID{AccountID: account, InstrumentID: func() *domain.InstrumentID { id := domain.NewInstrumentID(); return &id }(), Currency: "USD", AssetClass: "equity"}
	price := domain.ReturnPriceChange
	priceMoney := testReturnMoney(t, "5")
	beginning := testReturnMoney(t, "100")
	ending := testReturnMoney(t, "105")
	day1 := domain.ComponentDay{Date: "2026-08-01", Component: component, BeginningValue: beginning, EndingValue: ending, ReturnAmount: &priceMoney, ReturnComponents: map[domain.ReturnComponent]domain.SignedMoney{price: priceMoney}, Status: domain.CompletenessOK}
	day2 := domain.ComponentDay{Date: "2026-08-02", Component: component, BeginningValue: ending, EndingValue: ending, Status: domain.CompletenessOK}
	query := testReturnQuery("2026-08-01", "2026-08-02")
	result := domain.PeriodAnalysisResult{Query: query, Days: []domain.ComponentDay{day1, day2}, DailyReturns: []domain.DailyReturn{{Date: "2026-08-01", Amount: &priceMoney, Rate: func() *decimal.Decimal { value := decimal.NewFromInt(5).Div(decimal.NewFromInt(100)); return &value }(), Status: domain.CompletenessOK}, {Date: "2026-08-02", Status: domain.CompletenessPartial}}, InvestedCapital: &beginning, ReturnAmount: &priceMoney, Coverage: domain.RateCoverage{RatedDays: 1, TotalDays: 2}, Status: domain.CompletenessPartial}
	calendar, err := projectReturnCalendar(result, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !calendar.Available || calendar.Status != domain.CompletenessPartial || len(calendar.Days) != 2 || len(calendar.Issues) != 1 {
		t.Fatalf("calendar = %+v", calendar)
	}
	if calendar.Days[0].Composition[0].Component != domain.ReturnPriceChange || calendar.Days[0].Composition[0].Amount.Amount().String() != "5" {
		t.Fatalf("composition = %+v", calendar.Days[0].Composition)
	}
	if calendar.Summary.BeginningInvestedValue.Amount().String() != "100" || calendar.Summary.EndingInvestedValue.Amount().String() != "105" {
		t.Fatalf("summary = %+v", calendar.Summary)
	}
}

func TestReturnCalendarUsesDailyCoverageNotPeriodCoverage(t *testing.T) {
	amount := testReturnMoney(t, "5")
	rate := decimal.NewFromInt(5).Div(decimal.NewFromInt(100))
	result := domain.PeriodAnalysisResult{
		Query: testReturnQuery("2026-08-01", "2026-08-02"),
		DailyReturns: []domain.DailyReturn{
			{Date: "2026-08-01", Amount: &amount, Rate: &rate, Status: domain.CompletenessOK},
			{Date: "2026-08-02", Status: domain.CompletenessPartial},
		},
		ReturnAmount: &amount,
		Coverage:     domain.RateCoverage{RatedDays: 1, TotalDays: 2},
	}
	calendar, err := projectReturnCalendar(result, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if calendar.Summary.Coverage != (domain.RateCoverage{RatedDays: 1, TotalDays: 2}) {
		t.Fatalf("summary coverage = %+v", calendar.Summary.Coverage)
	}
	if calendar.Days[0].Coverage != (domain.RateCoverage{RatedDays: 1, TotalDays: 1}) {
		t.Fatalf("complete cell coverage = %+v", calendar.Days[0].Coverage)
	}
	if calendar.Days[1].Coverage != (domain.RateCoverage{RatedDays: 0, TotalDays: 1}) {
		t.Fatalf("partial cell coverage = %+v", calendar.Days[1].Coverage)
	}

	trend, err := projectReturnTrend(result, "", ReturnTrendLinkedRate)
	if err != nil {
		t.Fatal(err)
	}
	if trend.Points[0].Coverage != (domain.RateCoverage{RatedDays: 1, TotalDays: 1}) || trend.Points[1].Coverage != (domain.RateCoverage{RatedDays: 0, TotalDays: 1}) {
		t.Fatalf("trend point coverage = %+v", trend.Points)
	}
}

func TestReturnCalendarCursorFiltersCellsButKeepsPeriodSummary(t *testing.T) {
	amount := testReturnMoney(t, "5")
	result := domain.PeriodAnalysisResult{
		Query: testReturnQuery("2026-07-31", "2026-08-01"),
		DailyReturns: []domain.DailyReturn{
			{Date: "2026-07-31", Amount: &amount, Status: domain.CompletenessOK},
			{Date: "2026-08-01", Amount: &amount, Status: domain.CompletenessOK},
		},
		ReturnAmount: &amount,
		Coverage:     domain.RateCoverage{RatedDays: 2, TotalDays: 2},
	}
	calendar, err := projectReturnCalendar(result, "", "2026-08")
	if err != nil {
		t.Fatal(err)
	}
	if len(calendar.Days) != 1 || calendar.Days[0].Date != "2026-08-01" {
		t.Fatalf("cursor cells = %+v", calendar.Days)
	}
	if calendar.Summary.Coverage != result.Coverage || calendar.Summary.ReturnAmount == nil || !calendar.Summary.ReturnAmount.Amount().Equal(amount.Amount()) {
		t.Fatalf("cursor summary = %+v", calendar.Summary)
	}
	if _, err := projectReturnCalendar(result, "", "2026-8"); err == nil {
		t.Fatal("invalid cursor unexpectedly accepted")
	}
}

func TestReturnTrendKeepsDailyRatesAndGeometricPeriodRate(t *testing.T) {
	amount1, amount2 := testReturnMoney(t, "10"), testReturnMoney(t, "24")
	rate1, rate2 := decimal.NewFromInt(1).Div(decimal.NewFromInt(10)), decimal.NewFromInt(2).Div(decimal.NewFromInt(10))
	result := domain.PeriodAnalysisResult{DailyReturns: []domain.DailyReturn{{Date: "2026-08-01", Amount: &amount1, Rate: &rate1, Status: domain.CompletenessOK}, {Date: "2026-08-02", Amount: &amount2, Rate: &rate2, Status: domain.CompletenessOK}}, ReturnAmount: func() *domain.SignedMoney { money := testReturnMoney(t, "34"); return &money }(), ReturnRate: func() *decimal.Decimal { value := decimal.NewFromFloat(0.32); return &value }(), Coverage: domain.RateCoverage{RatedDays: 2, TotalDays: 2}}
	trend, err := projectReturnTrend(result, "", ReturnTrendLinkedRate)
	if err != nil {
		t.Fatal(err)
	}
	if len(trend.Points) != 2 || !trend.Points[0].Rate.Equal(rate1) || !trend.Points[1].Rate.Equal(rate2) {
		t.Fatalf("points = %+v", trend.Points)
	}
	if trend.Rate == nil || !trend.Rate.Equal(decimal.NewFromInt(32).Div(decimal.NewFromInt(100))) {
		t.Fatalf("linked rate = %v", trend.Rate)
	}
}

func TestReturnCalendarDoesNotTurnACompleteNoCapitalDayIntoAnIssue(t *testing.T) {
	zero := testReturnMoney(t, "0")
	result := domain.PeriodAnalysisResult{
		Query:           testReturnQuery("2026-08-01", "2026-08-01"),
		DailyReturns:    []domain.DailyReturn{{Date: "2026-08-01", Amount: &zero, Status: domain.CompletenessOK}},
		ReturnAmount:    &zero,
		Coverage:        domain.RateCoverage{TotalDays: 1},
		InvestedCapital: &zero,
	}
	calendar, err := projectReturnCalendar(result, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(calendar.Issues) != 0 || len(calendar.Days[0].Issues) != 0 || calendar.Days[0].Status != domain.CompletenessOK {
		t.Fatalf("complete no-capital day = %+v", calendar)
	}
}

func TestReturnTrendDisplayProjectsCumulativeAmount(t *testing.T) {
	first, second := testReturnMoney(t, "10"), testReturnMoney(t, "-3")
	result := domain.PeriodAnalysisResult{
		DailyReturns: []domain.DailyReturn{{Date: "2026-08-01", Amount: &first, Status: domain.CompletenessOK}, {Date: "2026-08-02", Amount: &second, Status: domain.CompletenessOK}},
		ReturnAmount: &second,
	}
	trend, err := projectReturnTrend(result, "", ReturnTrendCumulativeAmount)
	if err != nil {
		t.Fatal(err)
	}
	if trend.Points[0].Value == nil || trend.Points[0].Value.Amount().String() != "10" || trend.Points[1].Value.Amount().String() != "7" {
		t.Fatalf("cumulative points = %+v", trend.Points)
	}
}

func TestContributionTotalReturnUsesFoldAndRateSortFallsBackForDividend(t *testing.T) {
	firstID, secondID := domain.NewInstrumentID(), domain.NewInstrumentID()
	first := returnProjectionDay(t, firstID, "100", "10")
	second := returnProjectionDay(t, secondID, "200", "-5")
	query := testReturnQuery("2026-08-01", "2026-08-01")
	query.IncludeCash = false
	result := domain.PeriodAnalysisResult{Query: query, Days: []domain.ComponentDay{first, second}, Coverage: domain.RateCoverage{RatedDays: 1, TotalDays: 1}, DailyReturns: []domain.DailyReturn{{Date: "2026-08-01", Status: domain.CompletenessOK}}}
	groups, err := FoldReturnGroups(result, GroupByInstrument)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %+v", groups)
	}
	rows := []ContributionRow{{Key: "positive", Label: "positive", Amount: signedPointer(decimal.NewFromInt(10), "USD")}, {Key: "negative", Label: "negative", Amount: signedPointer(decimal.NewFromInt(-5), "USD")}}
	sortContributionRows(rows, ContributionSortRateDesc, false)
	if rows[0].Key != "positive" {
		t.Fatalf("rate sort fallback rows = %+v", rows)
	}
	if groups[0].ReturnRate == nil || groups[1].ReturnRate == nil {
		t.Fatalf("fold groups lack Dietz rates: %+v", groups)
	}
}

func TestContributionSortingHandlesUnavailableAmounts(t *testing.T) {
	rows := []ContributionRow{{Key: "missing", Label: "missing"}, {Key: "positive", Label: "positive", Amount: signedPointer(decimal.NewFromInt(10), "USD")}}
	sortContributionRows(rows, ContributionSortAmountDesc, true)
	if rows[0].Key != "positive" || rows[1].Key != "missing" {
		t.Fatalf("amount sort with nil = %+v", rows)
	}
	contributors := returnContributorsFromAmounts(map[string]decimal.Decimal{"missing": decimal.NewFromInt(1)}, "", domain.RateCoverage{TotalDays: 1})
	if len(contributors) != 1 || contributors[0].Amount != nil {
		t.Fatalf("nil contributor amount = %+v", contributors)
	}
}

func TestContributionItemReturnsNotFoundForUnknownGroup(t *testing.T) {
	input, query := phase2aSizedInputs(1, "2025-12-01", "2025-12-02")
	repository := &projectionRepository{portfolio: input.Portfolio, snapshots: input.Snapshots}
	service := &Service{analysis: NewAnalysisService(repository, func() time.Time { return time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) })}
	_, err := service.ContributionItem(context.Background(), query, ContributionTotalReturn, ContributionGroupAccount, "missing-group")
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != domain.ErrNotFound {
		t.Fatalf("unknown contribution group error = %v", err)
	}
}

func TestFoldReturnGroupsLinksRatesInDateOrder(t *testing.T) {
	instrumentID := domain.NewInstrumentID()
	first := returnProjectionDay(t, instrumentID, "100", "10")
	second := returnProjectionDay(t, instrumentID, "100", "20")
	second.Date = "2026-08-02"
	query := testReturnQuery("2026-08-01", "2026-08-02")
	query.IncludeCash = false
	result := domain.PeriodAnalysisResult{Query: query, Days: []domain.ComponentDay{first, second}, Coverage: domain.RateCoverage{RatedDays: 2, TotalDays: 2}, AnalysisDayTimezone: "UTC"}
	groups, err := FoldReturnGroups(result, GroupByInstrument)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].ReturnRate == nil || !groups[0].ReturnRate.Equal(decimal.NewFromInt(32).Div(decimal.NewFromInt(100))) {
		t.Fatalf("folded date-order rate = %+v", groups)
	}
}

func TestContributionUnrealizedIsUnavailableWithoutCostBasis(t *testing.T) {
	availability := unavailableReturn("unrealized range-end cost basis is not available from the analysis result")
	if availability.Available || availability.Status != domain.CompletenessUnavailable || availability.MissingReason == "" {
		t.Fatalf("availability = %+v", availability)
	}
}

func TestContributionIndependentViewsDoNotRequireReturnEngine(t *testing.T) {
	query := testReturnQuery("2026-08-01", "2026-08-02")
	service := &Service{}

	unrealized, err := service.Contribution(context.Background(), query, ContributionUnrealized, ContributionGroupInstrument, ContributionSortAmountDesc)
	if err != nil {
		t.Fatal(err)
	}
	if unrealized.Available || unrealized.Status != domain.CompletenessUnavailable {
		t.Fatalf("unrealized = %+v", unrealized)
	}

	realized, err := service.Contribution(context.Background(), query, ContributionRealized, ContributionGroupInstrument, ContributionSortAmountDesc)
	if err != nil {
		t.Fatal(err)
	}
	if realized.Available || realized.Status != domain.CompletenessUnavailable || realized.MissingReason == "" {
		t.Fatalf("realized = %+v", realized)
	}
}

func TestReturnProjectionCarriesForcedBaseForCase43(t *testing.T) {
	amount := testReturnMoney(t, "7")
	result := domain.PeriodAnalysisResult{DailyReturns: []domain.DailyReturn{{Date: "2026-08-01", Amount: &amount, Status: domain.CompletenessOK}}, ReturnAmount: &amount, Coverage: domain.RateCoverage{RatedDays: 1, TotalDays: 1}}
	calendar, err := projectReturnCalendar(result, "base", "")
	if err != nil {
		t.Fatal(err)
	}
	if calendar.ValuationForced == nil || *calendar.ValuationForced != "base" || calendar.Days[0].ValuationForced == nil {
		t.Fatalf("forced valuation = %+v / %+v", calendar.ValuationForced, calendar.Days)
	}
}

func TestReturnCalendarCase43UsesValuationFallback(t *testing.T) {
	householdID := domain.NewHouseholdID()
	usd := domain.Account{ID: domain.AccountID("00000000-0000-0000-0000-000000000002"), HouseholdID: householdID, Name: "USD investments", AccountType: domain.TypeBrokerage, BalanceSheetRole: domain.RoleAsset, TrackingMode: domain.TrackingHoldings, DefaultCurrency: "USD", IncludeInNetWorth: true}
	cny := domain.Account{ID: domain.AccountID("00000000-0000-0000-0000-000000000001"), HouseholdID: householdID, Name: "CNY", AccountType: domain.TypeBankAccount, BalanceSheetRole: domain.RoleAsset, TrackingMode: domain.TrackingBalance, DefaultCurrency: "CNY", IncludeInNetWorth: true}
	instrumentID := domain.NewInstrumentID()
	holdingID := domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: householdID, Name: "USD ETF", Type: domain.InstrumentETF, QuoteCurrency: "USD", QuoteSource: domain.QuoteSourceManual}
	quantity, err := domain.ParseQuantity("1")
	if err != nil {
		t.Fatal(err)
	}
	holding := domain.Holding{ID: holdingID, AccountID: usd.ID, InstrumentID: instrumentID, Quantity: quantity}
	origin := &domain.HistoryOrigin{HouseholdID: householdID, Timezone: "UTC", StartedAt: time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC)}
	portfolio := domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: "USD"}, Origin: origin, Accounts: []domain.AccountRecord{{Account: usd}, {Account: cny}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{holding}}
	snapshot := func(date string) domain.DailyValuationSnapshot {
		return domain.DailyValuationSnapshot{HouseholdID: householdID, LocalDate: date, CutoffAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC), Complete: true, Items: []domain.DailyValuationSnapshotItem{{AccountID: usd.ID, HoldingID: &holdingID, InstrumentID: &instrumentID, NativeAmount: "100", NativeCurrency: "USD", BaseAmountExact: "100", Complete: true}, {AccountID: cny.ID, NativeAmount: "100", NativeCurrency: "CNY", BaseAmountExact: "14.2857", Complete: true}}}
	}
	repository := &projectionRepository{portfolio: portfolio, snapshots: []domain.DailyValuationSnapshot{snapshot("2026-07-31"), snapshot("2026-08-01"), snapshot("2026-08-02")}}
	service := &Service{analysis: NewAnalysisService(repository, func() time.Time { return time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC) })}
	query := testReturnQuery("2026-08-01", "2026-08-02")
	query.Valuation = domain.ValuationNative
	query.IncludeCash = false
	calendar, err := service.ReturnCalendar(context.Background(), query, "", "day")
	if err != nil {
		t.Fatal(err)
	}
	if calendar.ValuationForced != nil {
		t.Fatalf("return calendar valuationForced = %v, want nil", calendar.ValuationForced)
	}
	if calendar.Summary.BeginningInvestedValue == nil || calendar.Summary.BeginningInvestedValue.Currency() != "USD" || !calendar.Summary.BeginningInvestedValue.Amount().Equal(decimal.NewFromInt(100)) {
		t.Fatalf("return beginning = %+v, summary=%+v, days=%+v, want 100 USD", calendar.Summary.BeginningInvestedValue, calendar.Summary, calendar.Days)
	}
	if calendar.Summary.ReturnAmount == nil || calendar.Summary.ReturnAmount.Currency() != "USD" || !calendar.Summary.ReturnAmount.Amount().IsZero() {
		t.Fatalf("return amount = %+v, want 0 USD", calendar.Summary.ReturnAmount)
	}
	if calendar.Summary.EndingInvestedValue == nil || calendar.Summary.EndingInvestedValue.Currency() != "USD" || !calendar.Summary.EndingInvestedValue.Amount().Equal(decimal.NewFromInt(100)) {
		t.Fatalf("return ending = %+v, summary=%+v, days=%+v, want 100 USD", calendar.Summary.EndingInvestedValue, calendar.Summary, calendar.Days)
	}
	if len(calendar.Days) == 0 {
		t.Fatalf("return calendar has no days")
	}
	firstDay := calendar.Days[0]
	if firstDay.BeginningInvestedValue == nil || firstDay.BeginningInvestedValue.Currency() != "USD" || !firstDay.BeginningInvestedValue.Amount().Equal(decimal.NewFromInt(100)) {
		t.Fatalf("return cell beginning = %+v, want 100 USD", firstDay.BeginningInvestedValue)
	}
	if firstDay.EndingInvestedValue == nil || firstDay.EndingInvestedValue.Currency() != "USD" || !firstDay.EndingInvestedValue.Amount().Equal(decimal.NewFromInt(100)) {
		t.Fatalf("return cell ending = %+v, want 100 USD", firstDay.EndingInvestedValue)
	}
	if firstDay.ReturnAmount == nil || firstDay.ReturnAmount.Currency() != "USD" || !firstDay.ReturnAmount.Amount().IsZero() {
		t.Fatalf("return cell amount = %+v, want 0 USD", firstDay.ReturnAmount)
	}
	assetChanges, err := service.AssetChange(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if assetChanges.ValuationForced == nil || *assetChanges.ValuationForced != string(domain.ValuationBase) {
		t.Fatalf("asset changes valuationForced = %v, want base", assetChanges.ValuationForced)
	}
}

func testReturnQuery(from, to domain.LocalDate) domain.AnalysisQuery {
	return domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: from, To: to, Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment}
}

func testReturnMoney(t *testing.T, value string) domain.SignedMoney {
	t.Helper()
	money, err := domain.ParseSignedMoney(value, "USD")
	if err != nil {
		t.Fatal(err)
	}
	return money
}

func returnProjectionDay(t *testing.T, instrumentID domain.InstrumentID, beginning, amount string) domain.ComponentDay {
	t.Helper()
	component := domain.ComponentID{AccountID: domain.NewAccountID(), InstrumentID: &instrumentID, Currency: "USD", AssetClass: "equity"}
	returnMoney := testReturnMoney(t, amount)
	beginningMoney := testReturnMoney(t, beginning)
	return domain.ComponentDay{Date: "2026-08-01", Component: component, BeginningValue: beginningMoney, EndingValue: beginningMoney, ReturnAmount: &returnMoney, InvestedCapital: &beginningMoney, ReturnComponents: map[domain.ReturnComponent]domain.SignedMoney{domain.ReturnPriceChange: returnMoney}, Status: domain.CompletenessOK}
}

func (r *projectionRepository) ListInstrumentQuotes(context.Context, domain.InstrumentID) ([]domain.InstrumentQuote, error) {
	return nil, nil
}

func BenchmarkPhase2bContributionUpperBound(b *testing.B) {
	input, query := phase2aUpperBoundInputs()
	repository := &projectionRepository{portfolio: input.Portfolio, snapshots: input.Snapshots}
	service := &Service{analysis: NewAnalysisService(repository, func() time.Time { return time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) })}
	ctx := context.Background()
	b.ReportMetric(float64(len(input.Snapshots)*len(input.Portfolio.Accounts)), "snapshot-components")
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		service.analysis.Invalidate()
		if _, err := service.Contribution(ctx, query, ContributionTotalReturn, ContributionGroupAccount, ContributionSortAmountDesc); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPhase2bFoldUpperBound(b *testing.B) {
	input, query := phase2aUpperBoundInputs()
	result, err := ComputeAnalysis(input, query)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(float64(len(result.Days)), "component-days")
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := FoldReturnGroups(result, GroupByAccount); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPhase2bServiceComputeUpperBound(b *testing.B) {
	input, query := phase2aUpperBoundInputs()
	repository := &projectionRepository{portfolio: input.Portfolio, snapshots: input.Snapshots}
	service := &Service{analysis: NewAnalysisService(repository, func() time.Time { return time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) })}
	ctx := context.Background()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		service.analysis.Invalidate()
		if _, err := service.Analyze(ctx, query); err != nil {
			b.Fatal(err)
		}
	}
}
