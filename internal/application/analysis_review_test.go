package application

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func reviewAnalysisClock() time.Time {
	return time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
}

func reviewOrigin(householdID domain.HouseholdID) *domain.HistoryOrigin {
	return &domain.HistoryOrigin{HouseholdID: householdID, Timezone: "UTC", StartedAt: time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC)}
}

func reviewCashItem(accountID domain.AccountID, currency, native, base string, complete bool) domain.DailyValuationSnapshotItem {
	item := domain.DailyValuationSnapshotItem{AccountID: accountID, NativeAmount: native, NativeCurrency: domain.CurrencyCode(currency), BaseAmountExact: base, Complete: complete}
	if base != "" {
		money, err := domain.ParseMoney(base, "USD")
		if err == nil {
			item.BaseAmount = &money
		}
	}
	return item
}

func reviewSnapshot(householdID domain.HouseholdID, date string, items ...domain.DailyValuationSnapshotItem) domain.DailyValuationSnapshot {
	cutoff, _ := time.Parse("2006-01-02", date)
	return domain.DailyValuationSnapshot{HouseholdID: householdID, LocalDate: date, CutoffAt: cutoff.Add(23 * time.Hour), Complete: true, Items: items}
}

func TestReviewF01ReturnFallbackDoesNotPolluteAssetUniverseCache(t *testing.T) {
	householdID := domain.NewHouseholdID()
	usdCash := domain.Account{ID: domain.NewAccountID(), HouseholdID: householdID, Name: "USD cash", AccountType: domain.TypeBankAccount, BalanceSheetRole: domain.RoleAsset, TrackingMode: domain.TrackingBalance, DefaultCurrency: "USD", IncludeInNetWorth: true}
	cnyCash := domain.Account{ID: domain.NewAccountID(), HouseholdID: householdID, Name: "CNY cash", AccountType: domain.TypeBankAccount, BalanceSheetRole: domain.RoleAsset, TrackingMode: domain.TrackingBalance, DefaultCurrency: "CNY", IncludeInNetWorth: true}
	debt := domain.Account{ID: domain.NewAccountID(), HouseholdID: householdID, Name: "Card", AccountType: domain.TypeCreditCard, BalanceSheetRole: domain.RoleLiability, TrackingMode: domain.TrackingBalance, DefaultCurrency: "USD", IncludeInNetWorth: true}
	origin := reviewOrigin(householdID)
	portfolio := domain.PortfolioSnapshot{
		Household: &domain.Household{ID: householdID, BaseCurrency: "USD"},
		Origin:    origin,
		Accounts:  []domain.AccountRecord{{Account: usdCash}, {Account: cnyCash}, {Account: debt}},
	}
	snapshot := func(date string) domain.DailyValuationSnapshot {
		return reviewSnapshot(householdID, date,
			reviewCashItem(usdCash.ID, "USD", "100", "100", true),
			reviewCashItem(cnyCash.ID, "CNY", "700", "100", true),
			reviewCashItem(debt.ID, "USD", "100", "100", true),
		)
	}
	newService := func() *Service {
		repository := &projectionRepository{portfolio: portfolio, snapshots: []domain.DailyValuationSnapshot{snapshot("2026-07-31"), snapshot("2026-08-01"), snapshot("2026-08-02")}}
		return &Service{analysis: NewAnalysisService(repository, reviewAnalysisClock)}
	}
	query := analysisMemoTestQuery("2026-08-01", "2026-08-02")
	query.Valuation = domain.ValuationNative
	query.IncludeCash = true

	assertAssetBeginning := func(t *testing.T, service *Service, label string) {
		t.Helper()
		assets, err := service.AssetChange(context.Background(), query)
		if err != nil {
			t.Fatal(err)
		}
		if assets.Summary.BeginningValue == nil || assets.Summary.BeginningValue.Currency() != "USD" || !assets.Summary.BeginningValue.Amount().Equal(decimal.NewFromInt(100)) {
			t.Fatalf("%s asset beginning = %+v, want 100 USD with liability retained", label, assets.Summary.BeginningValue)
		}
	}

	t.Run("assets-first", func(t *testing.T) {
		assertAssetBeginning(t, newService(), "assets-first")
	})
	t.Run("return-native-fallback-then-assets", func(t *testing.T) {
		service := newService()
		calendar, err := service.ReturnCalendar(context.Background(), query, "", "day")
		if err != nil {
			t.Fatal(err)
		}
		if calendar.ValuationForced == nil || *calendar.ValuationForced != string(domain.ValuationBase) {
			t.Fatalf("return fallback valuationForced = %v, want base", calendar.ValuationForced)
		}
		assertAssetBeginning(t, service, "return-then-assets")
	})
}

func TestReviewF02UnknownAmountsStayNilAndKnownZeroIsKept(t *testing.T) {
	householdID := domain.NewHouseholdID()
	account := analysisAccount(householdID, "USD", domain.TrackingBalance, domain.RoleAsset)
	household := &domain.Household{ID: householdID, BaseCurrency: "USD"}
	complete := func(date, amount string) domain.DailyValuationSnapshot {
		return analysisSnapshot(date, analysisItem(t, account.ID, "USD", amount, amount, nil, nil, "", ""))
	}
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-01", To: "2026-08-02", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true}

	t.Run("fully-missing-day-is-unavailable", func(t *testing.T) {
		input := AnalysisInputs{
			Origin:    analysisOrigin(t, householdID, "UTC"),
			Portfolio: domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: account}}},
			Snapshots: []domain.DailyValuationSnapshot{complete("2026-07-31", "100"), complete("2026-08-01", "100")},
		}
		result, err := ComputeAnalysis(input, query)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.DailyReturns) != 2 {
			t.Fatalf("daily returns = %+v", result.DailyReturns)
		}
		if result.DailyReturns[0].Amount == nil || !result.DailyReturns[0].Amount.Amount().IsZero() || result.DailyReturns[0].Status != domain.CompletenessOK {
			t.Fatalf("complete day = %+v, want known zero", result.DailyReturns[0])
		}
		missing := result.DailyReturns[1]
		if missing.Amount != nil || missing.InvestedCapital != nil || missing.Status != domain.CompletenessUnavailable {
			t.Fatalf("missing day = %+v, want unavailable nil amounts", missing)
		}
		calendar, err := projectReturnCalendar(result, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if len(calendar.Days) != 2 || calendar.Days[1].Available || calendar.Days[1].ReturnAmount != nil || calendar.Days[1].BeginningInvestedValue != nil || calendar.Days[1].EndingInvestedValue != nil {
			t.Fatalf("calendar missing day = %+v, want unavailable nil amounts", calendar.Days[1])
		}
	})

	t.Run("complete-zero-return", func(t *testing.T) {
		zeroQuery := query
		zeroQuery.To = "2026-08-01"
		input := AnalysisInputs{
			Origin:    analysisOrigin(t, householdID, "UTC"),
			Portfolio: domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: account}}},
			Snapshots: []domain.DailyValuationSnapshot{complete("2026-07-31", "100"), complete("2026-08-01", "100")},
		}
		result, err := ComputeAnalysis(input, zeroQuery)
		if err != nil {
			t.Fatal(err)
		}
		if result.DailyReturns[0].Amount == nil || !result.DailyReturns[0].Amount.Amount().IsZero() || result.DailyReturns[0].Status != domain.CompletenessOK {
			t.Fatalf("complete zero = %+v", result.DailyReturns[0])
		}
	})

	t.Run("complete-non-positive-capital", func(t *testing.T) {
		zeroQuery := query
		zeroQuery.To = "2026-08-01"
		input := AnalysisInputs{
			Origin:    analysisOrigin(t, householdID, "UTC"),
			Portfolio: domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: account}}},
			Snapshots: []domain.DailyValuationSnapshot{complete("2026-07-31", "0"), complete("2026-08-01", "0")},
		}
		result, err := ComputeAnalysis(input, zeroQuery)
		if err != nil {
			t.Fatal(err)
		}
		daily := result.DailyReturns[0]
		if daily.Amount == nil || !daily.Amount.Amount().IsZero() || daily.Rate != nil || daily.Status != domain.CompletenessOK {
			t.Fatalf("non-positive capital = %+v, want known zero amount and no rate", daily)
		}
	})
}

func TestReviewF03InKindTransferIsDietzCapitalAtScopeBoundary(t *testing.T) {
	householdID := domain.NewHouseholdID()
	household := &domain.Household{ID: householdID, BaseCurrency: "CNY"}
	source := analysisAccount(householdID, "CNY", domain.TrackingHoldings, domain.RoleAsset)
	receiver := analysisAccount(householdID, "CNY", domain.TrackingHoldings, domain.RoleAsset)
	instrumentID := domain.NewInstrumentID()
	fromHolding, toHolding := domain.NewHoldingID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: householdID, Type: domain.InstrumentStock, QuoteCurrency: "CNY"}
	openingQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "100"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
	quote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "100"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	previous := analysisSnapshot("2026-08-01", analysisItem(t, source.ID, "CNY", "100", "100", &fromHolding, &instrumentID, openingQuote.ID.String(), ""), analysisItem(t, receiver.ID, "CNY", "0", "0", &toHolding, &instrumentID, "", ""))
	current := analysisSnapshot("2026-08-02", analysisItem(t, source.ID, "CNY", "0", "0", &fromHolding, &instrumentID, quote.ID.String(), ""), analysisItem(t, receiver.ID, "CNY", "100", "100", &toHolding, &instrumentID, quote.ID.String(), ""))
	activityID := domain.NewActivityID()
	quantity := mustQuantity(t, "1")
	activity := domain.Activity{ID: activityID, Kind: domain.ActivityPositionTransfer, Reason: domain.ReasonOther, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02", Effects: []domain.ActivityEffect{
		{ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 1, Direction: domain.EffectRemoved, Target: domain.EffectTargetHoldingQuantity, Classification: domain.ClassificationInternalTransfer, HoldingID: &fromHolding, InstrumentID: &instrumentID, Quantity: &quantity},
		{ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 2, Direction: domain.EffectAdded, Target: domain.EffectTargetHoldingQuantity, Classification: domain.ClassificationInternalTransfer, HoldingID: &toHolding, InstrumentID: &instrumentID, Quantity: &quantity},
	}}
	input := AnalysisInputs{
		Origin:           analysisOrigin(t, householdID, "UTC"),
		Portfolio:        domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: source}, {Account: receiver}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{{ID: fromHolding, AccountID: source.ID, InstrumentID: instrumentID}, {ID: toHolding, AccountID: receiver.ID, InstrumentID: instrumentID}}},
		Snapshots:        []domain.DailyValuationSnapshot{previous, current},
		Activities:       []domain.Activity{activity},
		InstrumentQuotes: []domain.InstrumentQuote{openingQuote, quote},
	}

	householdResult, err := ComputeAnalysis(input, analysisBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	if len(householdResult.DailyReturns) != 1 || householdResult.DailyReturns[0].InvestedCapital == nil || !householdResult.DailyReturns[0].InvestedCapital.Amount().Equal(decimal.NewFromInt(100)) {
		t.Fatalf("household capital = %+v, want internal-neutral 100", householdResult.DailyReturns)
	}

	accountQuery := analysisBaseQuery(domain.ValuationBase)
	accountQuery.Scope = domain.AnalysisScope{Kind: domain.ScopeAccount, ID: receiver.ID.String()}
	accountResult, err := ComputeAnalysis(input, accountQuery)
	if err != nil {
		t.Fatal(err)
	}
	if len(accountResult.DailyReturns) != 1 {
		t.Fatalf("receiving daily returns = %+v", accountResult.DailyReturns)
	}
	daily := accountResult.DailyReturns[0]
	if daily.InvestedCapital == nil || !daily.InvestedCapital.Amount().Equal(decimal.NewFromInt(50)) {
		t.Fatalf("receiving invested capital = %+v, want 50", daily.InvestedCapital)
	}
	if daily.Rate == nil || !daily.Rate.IsZero() || daily.Status != domain.CompletenessOK {
		t.Fatalf("receiving rate = %+v, want defined 0%%", daily)
	}
}

func TestReviewF03SplitAndReconciliationAreNotDietzCapital(t *testing.T) {
	householdID := domain.NewHouseholdID()
	household := &domain.Household{ID: householdID, BaseCurrency: "CNY"}
	account := analysisAccount(householdID, "CNY", domain.TrackingHoldings, domain.RoleAsset)
	instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: householdID, Type: domain.InstrumentStock, QuoteCurrency: "CNY"}
	openQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "400"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
	closeQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "100"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	previous := analysisSnapshot("2026-08-01", analysisItem(t, account.ID, "CNY", "40000", "40000", &holdingID, &instrumentID, openQuote.ID.String(), ""))
	current := analysisSnapshot("2026-08-02", analysisItem(t, account.ID, "CNY", "40000", "40000", &holdingID, &instrumentID, closeQuote.ID.String(), ""))
	activityID := domain.NewActivityID()
	quantity := mustQuantity(t, "300")
	activity := domain.Activity{ID: activityID, Kind: domain.ActivityPositionTransfer, Reason: domain.ReasonReconciliation, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02", Effects: []domain.ActivityEffect{{ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 1, Direction: domain.EffectAdded, Target: domain.EffectTargetHoldingQuantity, Classification: domain.ClassificationRemeasurement, HoldingID: &holdingID, InstrumentID: &instrumentID, Quantity: &quantity}}}
	input := AnalysisInputs{Origin: analysisOrigin(t, householdID, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: account}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}}}, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{activity}, InstrumentQuotes: []domain.InstrumentQuote{openQuote, closeQuote}}
	result, err := ComputeAnalysis(input, analysisBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	day := result.Days[0]
	if len(day.DietzCapitalFlows) != 0 {
		t.Fatalf("split produced Dietz capital = %+v", day.DietzCapitalFlows)
	}
	if day.InvestedCapital == nil || !day.InvestedCapital.Amount().Equal(decimal.NewFromInt(40000)) {
		t.Fatalf("split invested capital = %+v, want opening 40000", day.InvestedCapital)
	}
}

func TestReviewF07OffsettingResidualKeepsIssueCountWithoutZeroBar(t *testing.T) {
	first := domain.AccountID("00000000-0000-0000-0000-000000000001")
	second := domain.AccountID("00000000-0000-0000-0000-000000000002")
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-01", To: "2026-08-02", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment}
	plus := analysisTestDay(t, "2026-08-01", first, "100", map[domain.AttributionBucket]string{domain.BucketResidual: "5"})
	minus := analysisTestDay(t, "2026-08-02", second, "100", map[domain.AttributionBucket]string{domain.BucketResidual: "-5"})
	projection := foldAssetChange(domain.PeriodAnalysisResult{Query: query, Status: domain.CompletenessPartial, Days: []domain.ComponentDay{plus, minus}}, "")
	if projection.ResidualIssueCount != 2 {
		t.Fatalf("residual issue count = %d, want 2", projection.ResidualIssueCount)
	}
	for _, row := range projection.Waterfall {
		if row.Bucket == domain.BucketResidual {
			t.Fatalf("net-zero residual was drawn as a waterfall bar: %+v", row)
		}
	}
}

func TestReviewD5AssetChangeAggregatesExactAttributionBeforeRounding(t *testing.T) {
	accountID := domain.NewAccountID()
	beginning := analysisSignedTestMoney(t, "100")
	ending := analysisSignedTestMoney(t, "100.0001")
	zero := analysisSignedTestMoney(t, "0")
	day := domain.ComponentDay{
		Date:         "2026-08-01",
		Component:    domain.ComponentID{AccountID: accountID, Currency: "USD", Cash: true},
		AssetBuckets: map[domain.AttributionBucket]domain.SignedMoney{domain.BucketAdjustment: zero},
		AssetBucketExact: map[domain.AttributionBucket]decimal.Decimal{
			domain.BucketAdjustment: decimal.RequireFromString("0.0001"),
		},
		BeginningValue: beginning,
		EndingValue:    ending,
		Status:         domain.CompletenessOK,
	}
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-01", To: "2026-08-01", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment}
	projection := foldAssetChange(domain.PeriodAnalysisResult{Query: query, Days: []domain.ComponentDay{day}}, "")
	if projection.Summary.Change == nil || !projection.Summary.Change.Amount().Equal(decimal.RequireFromString("0.0001")) {
		t.Fatalf("summary change = %+v, want 0.0001 USD", projection.Summary.Change)
	}
	if len(projection.Waterfall) != 1 || projection.Waterfall[0].Amount == nil || !projection.Waterfall[0].Amount.Amount().Equal(decimal.RequireFromString("0.0001")) {
		t.Fatalf("waterfall = %+v, want exact adjustment", projection.Waterfall)
	}
}

func TestReviewF11ContributionHistoryHintUsesWholeGroup(t *testing.T) {
	firstAccount, secondAccount := domain.NewAccountID(), domain.NewAccountID()
	dividendTen := testReturnMoney(t, "10")
	dividendTwenty := testReturnMoney(t, "20")
	first := domain.ComponentDay{
		Date:             "2026-08-01",
		Component:        domain.ComponentID{AccountID: firstAccount, Currency: "USD", Cash: true, AssetClass: domain.BucketCash},
		ReturnAmount:     &dividendTen,
		ReturnComponents: map[domain.ReturnComponent]domain.SignedMoney{domain.ReturnDividendInterest: dividendTen},
		Status:           domain.CompletenessOK,
	}
	second := domain.ComponentDay{
		Date:             "2026-08-01",
		Component:        domain.ComponentID{AccountID: secondAccount, Currency: "USD", Cash: true, AssetClass: domain.BucketCash},
		ReturnAmount:     &dividendTwenty,
		ReturnComponents: map[domain.ReturnComponent]domain.SignedMoney{domain.ReturnDividendInterest: dividendTwenty},
		Status:           domain.CompletenessOK,
	}
	query := testReturnQuery("2026-08-01", "2026-08-01")
	query.IncludeCash = true
	assertCurrencyHint := func(t *testing.T, days []domain.ComponentDay) {
		t.Helper()
		result := domain.PeriodAnalysisResult{Query: query, Days: days, Coverage: domain.RateCoverage{RatedDays: 1, TotalDays: 1}}
		item, err := projectContributionItem(result, "", query, ContributionDividendInterest, ContributionGroupCurrency, "USD")
		if err != nil {
			t.Fatal(err)
		}
		if item.Amount == nil || !item.Amount.Amount().Equal(decimal.NewFromInt(30)) {
			t.Fatalf("currency group amount = %+v, want 30", item.Amount)
		}
		if len(item.ByAccount) != 2 {
			t.Fatalf("byAccount = %+v, want two accounts", item.ByAccount)
		}
		if item.HistoryHint.AccountID != "" || item.HistoryHint.InstrumentID != "" {
			t.Fatalf("history hint = %+v, want no single account or instrument", item.HistoryHint)
		}
	}
	assertCurrencyHint(t, []domain.ComponentDay{first, second})
	assertCurrencyHint(t, []domain.ComponentDay{second, first})

	result := domain.PeriodAnalysisResult{Query: query, Days: []domain.ComponentDay{first, second}, Coverage: domain.RateCoverage{RatedDays: 1, TotalDays: 1}}
	accountItem, err := projectContributionItem(result, "", query, ContributionDividendInterest, ContributionGroupAccount, firstAccount.String())
	if err != nil {
		t.Fatal(err)
	}
	if accountItem.HistoryHint.AccountID != firstAccount.String() {
		t.Fatalf("account hint = %+v, want the selected account", accountItem.HistoryHint)
	}
	if accountItem.HistoryHint.InstrumentID != "" {
		t.Fatalf("cash-only account hint = %+v, want no instrument filter", accountItem.HistoryHint)
	}

	instrumentID := domain.NewInstrumentID()
	holdingReturn := testReturnMoney(t, "20")
	cashInterest := domain.ComponentDay{
		Date:             "2026-08-01",
		Component:        domain.ComponentID{AccountID: firstAccount, Currency: "USD", Cash: true, AssetClass: domain.BucketCash},
		ReturnAmount:     &dividendTen,
		ReturnComponents: map[domain.ReturnComponent]domain.SignedMoney{domain.ReturnDividendInterest: dividendTen},
		Status:           domain.CompletenessOK,
	}
	holding := domain.ComponentDay{
		Date:             "2026-08-01",
		Component:        domain.ComponentID{AccountID: firstAccount, InstrumentID: &instrumentID, Currency: "USD", AssetClass: "equity"},
		ReturnAmount:     &holdingReturn,
		ReturnComponents: map[domain.ReturnComponent]domain.SignedMoney{domain.ReturnPriceChange: holdingReturn},
		Status:           domain.CompletenessOK,
	}
	assertAccountKeepsCash := func(t *testing.T, days []domain.ComponentDay) {
		t.Helper()
		mixed := domain.PeriodAnalysisResult{Query: query, Days: days, Coverage: domain.RateCoverage{RatedDays: 1, TotalDays: 1}}
		item, err := projectContributionItem(mixed, "", query, ContributionTotalReturn, ContributionGroupAccount, firstAccount.String())
		if err != nil {
			t.Fatal(err)
		}
		if item.Amount == nil || !item.Amount.Amount().Equal(decimal.NewFromInt(30)) {
			t.Fatalf("account group amount = %+v, want 30", item.Amount)
		}
		if item.HistoryHint.AccountID != firstAccount.String() {
			t.Fatalf("mixed account hint = %+v, want the account", item.HistoryHint)
		}
		if item.HistoryHint.InstrumentID != "" {
			t.Fatalf("mixed account hint = %+v, want no instrument filter so cash interest stays in History", item.HistoryHint)
		}
	}
	assertAccountKeepsCash(t, []domain.ComponentDay{cashInterest, holding})
	assertAccountKeepsCash(t, []domain.ComponentDay{holding, cashInterest})

	holdingOnly := domain.PeriodAnalysisResult{Query: query, Days: []domain.ComponentDay{holding}, Coverage: domain.RateCoverage{RatedDays: 1, TotalDays: 1}}
	holdingItem, err := projectContributionItem(holdingOnly, "", query, ContributionTotalReturn, ContributionGroupAccount, firstAccount.String())
	if err != nil {
		t.Fatal(err)
	}
	if holdingItem.HistoryHint.AccountID != firstAccount.String() || holdingItem.HistoryHint.InstrumentID != instrumentID.String() {
		t.Fatalf("holding-only hint = %+v, want account and instrument", holdingItem.HistoryHint)
	}
}

func TestReviewF11RealizedContributionHistoryHintKeepsOnlyExplicitDimensions(t *testing.T) {
	accountID, instrumentID := domain.NewAccountID(), domain.NewInstrumentID()
	query := testReturnQuery("2026-08-01", "2026-08-31")

	instrumentHint := realizedContributionHistoryHint(query, ContributionGroupInstrument, instrumentID.String())
	if instrumentHint.AccountID != "" || instrumentHint.InstrumentID != instrumentID.String() {
		t.Fatalf("household instrument hint = %+v, want instrument only", instrumentHint)
	}

	query.Scope = domain.AnalysisScope{Kind: domain.ScopeAccount, ID: accountID.String()}
	accountScopedInstrumentHint := realizedContributionHistoryHint(query, ContributionGroupInstrument, instrumentID.String())
	if accountScopedInstrumentHint.AccountID != accountID.String() || accountScopedInstrumentHint.InstrumentID != instrumentID.String() {
		t.Fatalf("account-scoped instrument hint = %+v, want both dimensions", accountScopedInstrumentHint)
	}

	query.Scope = domain.AnalysisScope{Kind: domain.ScopeHousehold}
	query.Filters.InstrumentID = &instrumentID
	accountHint := realizedContributionHistoryHint(query, ContributionGroupAccount, accountID.String())
	if accountHint.AccountID != accountID.String() || accountHint.InstrumentID != instrumentID.String() {
		t.Fatalf("instrument-filtered account hint = %+v, want both dimensions", accountHint)
	}
}
