package application

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestAnalyticsPhase1bModifiedDietzAndGeometricLinking(t *testing.T) {
	dayStart := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.AddDate(0, 0, 1)
	flowMoney, err := domain.ParseSignedMoney("90000", "CNY")
	if err != nil {
		t.Fatal(err)
	}
	rate, rated := ModifiedDietzRate(decimal.NewFromInt(10000), decimal.NewFromInt(1000), []domain.DietzCapitalFlow{{Amount: flowMoney, EffectiveAt: dayStart.Add(12 * time.Hour)}}, dayStart, dayEnd)
	if !rated || rate.Sub(decimal.RequireFromString("0.0181818181818181818")).Abs().GreaterThan(decimal.RequireFromString("0.000000000000001")) {
		t.Fatalf("noon Modified Dietz rate = %s, want 1/55", rate)
	}
	linked := GeometricLink([]decimal.Decimal{rate, decimal.RequireFromString("0.01")})
	want := decimal.NewFromInt(1).Add(rate).Mul(decimal.RequireFromString("1.01")).Sub(decimal.NewFromInt(1))
	if !linked.Equal(want) {
		t.Fatalf("geometric link = %s, want %s", linked, want)
	}
}

func TestAnalyticsPhase1bCase37InterestRaisesReturnIncomeIsCapital(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		reason     domain.ActivityReason
		withCredit bool
		wantAmount string
		wantRate   string
	}{
		{name: "baseline", wantAmount: "100", wantRate: "0.05"},
		{name: "interest", reason: domain.ReasonInterest, withCredit: true, wantAmount: "600", wantRate: "0.3"},
		{name: "income", reason: domain.ReasonIncome, withCredit: true, wantAmount: "100", wantRate: "0.0444444444444444444"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			householdID := domain.NewHouseholdID()
			account := phase1aAgentAccount("CNY", domain.TrackingHoldings, domain.RoleAsset)
			account.HouseholdID = householdID
			instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
			instrument := domain.Instrument{ID: instrumentID, HouseholdID: householdID, Name: "fund", Type: domain.InstrumentETF, QuoteCurrency: "CNY"}
			holding := domain.Holding{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}
			openQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "100"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
			closeQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "110"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
			previousCash, currentCash := "1000", "1000"
			activities := []domain.Activity(nil)
			if testCase.withCredit {
				currentCash = "1500"
				activityID := domain.NewActivityID()
				money := phase1aAgentMoney(t, "500", "CNY")
				activities = []domain.Activity{{ID: activityID, Kind: domain.ActivityCashIn, Reason: testCase.reason, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02", Effects: []domain.ActivityEffect{{ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 1, Role: domain.EffectRoleAmount, Direction: domain.EffectAdded, Target: domain.EffectTargetAccountCash, Classification: domain.ClassificationIncome, AccountID: &account.ID, Money: &money}}}}
			}
			previous := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "CNY", previousCash, previousCash, nil, nil, "", ""), analyticsPhase1aItem(t, account.ID, "CNY", "1000", "1000", &holdingID, &instrumentID, openQuote.ID.String(), ""))
			current := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "CNY", currentCash, currentCash, nil, nil, "", ""), analyticsPhase1aItem(t, account.ID, "CNY", "1100", "1100", &holdingID, &instrumentID, closeQuote.ID.String(), ""))
			input := AnalysisInputs{Origin: phase1aAgentOrigin(t, householdID), Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: "CNY"}, Accounts: []domain.AccountRecord{{Account: account}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{holding}}, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: activities, InstrumentQuotes: []domain.InstrumentQuote{openQuote, closeQuote}}
			query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-02", To: "2026-08-02", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true}
			result, err := ComputeAnalysis(input, query)
			if err != nil {
				t.Fatal(err)
			}
			wantAmount := decimal.RequireFromString(testCase.wantAmount)
			if result.ReturnAmount == nil || !result.ReturnAmount.Amount().Equal(wantAmount) {
				t.Fatalf("return amount = %+v, want %s; days=%+v", result.ReturnAmount, wantAmount, result.Days)
			}
			wantRate := decimal.RequireFromString(testCase.wantRate)
			if result.ReturnRate == nil || result.ReturnRate.Sub(wantRate).Abs().GreaterThan(decimal.RequireFromString("0.000000000000001")) {
				t.Fatalf("return rate = %+v, want %s", result.ReturnRate, testCase.wantRate)
			}
			if testCase.name == "interest" && !result.ReturnRate.GreaterThan(decimal.RequireFromString("0.05")) {
				t.Fatalf("interest did not raise the positive baseline return: %s", result.ReturnRate)
			}
			if testCase.name == "income" && !result.ReturnRate.LessThan(decimal.RequireFromString("0.05")) {
				t.Fatalf("income did not lower the positive baseline return: %s", result.ReturnRate)
			}
			day := result.DailyReturns[0]
			if testCase.name == "interest" && len(result.Days[0].DietzCapitalFlows) != 0 {
				t.Fatalf("interest became Dietz capital: %+v", result.Days[0].DietzCapitalFlows)
			}
			if testCase.name == "income" && len(result.Days[0].DietzCapitalFlows) != 1 {
				t.Fatalf("income did not become Dietz capital: %+v", result.Days[0].DietzCapitalFlows)
			}
			if day.Rate == nil {
				t.Fatal("daily rate is missing")
			}
		})
	}
}

func TestAnalyticsPhase1bCases19And20UseWeightedCapitalOnlyWhenCashIncluded(t *testing.T) {
	for _, testCase := range []struct {
		name           string
		reason         domain.ActivityReason
		classification domain.ActivityClassification
		includeCash    bool
		wantRate       decimal.Decimal
		wantBucket     domain.AttributionBucket
	}{
		{name: "case19_contribution", reason: domain.ReasonContribution, classification: domain.ClassificationExternalInflow, includeCash: true, wantRate: decimal.NewFromInt(1).Div(decimal.NewFromInt(55)), wantBucket: domain.BucketExternalFlow},
		{name: "case20_salary_with_cash", reason: domain.ReasonIncome, classification: domain.ClassificationIncome, includeCash: true, wantRate: decimal.NewFromInt(1).Div(decimal.NewFromInt(55)), wantBucket: domain.BucketIncome},
		{name: "case20_salary_without_cash", reason: domain.ReasonIncome, classification: domain.ClassificationIncome, includeCash: false, wantRate: decimal.NewFromInt(1).Div(decimal.NewFromInt(10)), wantBucket: domain.BucketIncome},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			householdID := domain.NewHouseholdID()
			account := phase1aAgentAccount("USD", domain.TrackingHoldings, domain.RoleAsset)
			account.HouseholdID = householdID
			instrumentID := domain.NewInstrumentID()
			holdingID := domain.NewHoldingID()
			instrument := domain.Instrument{ID: instrumentID, HouseholdID: householdID, Name: "QQQ", Type: domain.InstrumentETF, QuoteCurrency: "USD"}
			holding := domain.Holding{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}
			previous := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "USD", "0", "0", nil, nil, "", ""), analyticsPhase1aItem(t, account.ID, "USD", "10000", "10000", &holdingID, &instrumentID, "", ""))
			current := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "USD", "90000", "90000", nil, nil, "", ""), analyticsPhase1aItem(t, account.ID, "USD", "11000", "11000", &holdingID, &instrumentID, "", ""))
			activityID := domain.NewActivityID()
			amount := phase1aAgentMoney(t, "90000", "USD")
			activity := domain.Activity{ID: activityID, Kind: domain.ActivityCashIn, Reason: testCase.reason, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02", Effects: []domain.ActivityEffect{{ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 1, Role: domain.EffectRoleAmount, Direction: domain.EffectAdded, Target: domain.EffectTargetAccountCash, Classification: testCase.classification, AccountID: &account.ID, Money: &amount}}}
			openingPrice, _ := domain.ParseUnitPrice("10000")
			closingPrice, _ := domain.ParseUnitPrice("11000")
			input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, householdID, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: "USD"}, Accounts: []domain.AccountRecord{{Account: account}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{holding}}, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{activity}, InstrumentQuotes: []domain.InstrumentQuote{{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: openingPrice, Currency: "USD", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}, {ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: closingPrice, Currency: "USD", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}}}
			query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-02", To: "2026-08-02", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: testCase.includeCash}
			result, err := ComputeAnalysis(input, query)
			if err != nil {
				t.Fatal(err)
			}
			if result.ReturnRate == nil || result.ReturnRate.Sub(testCase.wantRate).Abs().GreaterThan(decimal.RequireFromString("0.000000000000001")) {
				t.Fatalf("return rate = %v, want %s", result.ReturnRate, testCase.wantRate)
			}
			if result.DailyReturns[0].Rate == nil {
				t.Fatal("daily rate is missing")
			}
			var bucketAmount decimal.Decimal
			for _, day := range result.Days {
				if value, ok := day.AssetBuckets[testCase.wantBucket]; ok {
					bucketAmount = bucketAmount.Add(value.Amount())
				}
			}
			if !bucketAmount.Equal(decimal.NewFromInt(90000)) {
				t.Fatalf("%s bucket = %s, want 90000", testCase.wantBucket, bucketAmount)
			}
		})
	}
}

func TestAnalyticsPhase1bDSTWeightUsesOriginLocalDayLength(t *testing.T) {
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		date  string
		hours string
	}{
		{date: "2026-03-08", hours: "23"},
		{date: "2026-11-01", hours: "25"},
	} {
		localDate, err := time.ParseInLocation("2006-01-02", testCase.date, location)
		if err != nil {
			t.Fatal(err)
		}
		start, end := localDayBounds(localDate, location)
		if got := end.Sub(start).Hours(); got != decimal.RequireFromString(testCase.hours).InexactFloat64() {
			t.Fatalf("%s day length = %v, want %s", testCase.date, got, testCase.hours)
		}
		noon := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 12, 0, 0, 0, location)
		weight := analysisDayFlowWeight(testCase.date, location.String(), noon)
		want := decimal.NewFromInt(12).Div(decimal.RequireFromString(testCase.hours))
		if !weight.Equal(want) {
			t.Fatalf("%s noon weight = %s, want %s", testCase.date, weight, want)
		}
	}
}

func TestAnalyticsPhase1bReconciliationIsNotDietzCapital(t *testing.T) {
	householdID := domain.NewHouseholdID()
	account := phase1aAgentAccount("CNY", domain.TrackingHoldings, domain.RoleAsset)
	account.HouseholdID = householdID
	instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: householdID, Name: "fund", Type: domain.InstrumentETF, QuoteCurrency: "CNY"}
	holding := domain.Holding{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}
	openQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "100"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
	closeQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "110"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	activityID := domain.NewActivityID()
	money := phase1aAgentMoney(t, "500", "CNY")
	reconciliation := domain.Activity{ID: activityID, Kind: domain.ActivityCashIn, Reason: domain.ReasonReconciliation, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02", Effects: []domain.ActivityEffect{{ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 1, Role: domain.EffectRoleAmount, Direction: domain.EffectAdded, Target: domain.EffectTargetAccountCash, Classification: domain.ClassificationRemeasurement, AccountID: &account.ID, Money: &money}}}
	previous := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "CNY", "1000", "1000", nil, nil, "", ""), analyticsPhase1aItem(t, account.ID, "CNY", "1000", "1000", &holdingID, &instrumentID, openQuote.ID.String(), ""))
	current := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "CNY", "1500", "1500", nil, nil, "", ""), analyticsPhase1aItem(t, account.ID, "CNY", "1100", "1100", &holdingID, &instrumentID, closeQuote.ID.String(), ""))
	input := AnalysisInputs{Origin: phase1aAgentOrigin(t, householdID), Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: "CNY"}, Accounts: []domain.AccountRecord{{Account: account}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{holding}}, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{reconciliation}, InstrumentQuotes: []domain.InstrumentQuote{openQuote, closeQuote}}
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-02", To: "2026-08-02", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true}
	result, err := ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	var cashDay domain.ComponentDay
	for _, day := range result.Days {
		if day.Component.Cash {
			cashDay = day
			break
		}
	}
	if cashDay.Component.AccountID == "" {
		t.Fatal("cash component day was missing")
	}
	if got := analyticsPhase1aBucket(cashDay, domain.BucketAdjustment); !got.Equal(decimal.NewFromInt(500)) {
		t.Fatalf("reconciliation asset bucket = %s, want Adjustment 500", got)
	}
	if len(cashDay.DietzCapitalFlows) != 0 || !cashDay.DietzFlow.IsZero() {
		t.Fatalf("reconciliation entered Dietz capital: flows=%+v flow=%s", cashDay.DietzCapitalFlows, cashDay.DietzFlow.Amount())
	}
	if result.ReturnRate == nil || !result.ReturnRate.Equal(decimal.RequireFromString("0.05")) {
		t.Fatalf("reconciliation distorted the return rate: %+v, want 0.05", result.ReturnRate)
	}
}

func TestAnalyticsPhase1bDSTNoonFlowWeightUsesOriginLocalDay(t *testing.T) {
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		date  string
		hours int64
	}{
		{date: "2026-03-08", hours: 23},
		{date: "2026-11-01", hours: 25},
	} {
		householdID := domain.NewHouseholdID()
		account := phase1aAgentAccount("USD", domain.TrackingBalance, domain.RoleAsset)
		account.HouseholdID = householdID
		previousDate := time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC)
		if testCase.date == "2026-11-01" {
			previousDate = time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
		}
		previous := domain.DailyValuationSnapshot{LocalDate: previousDate.Format("2006-01-02"), CutoffAt: previousDate.Add(23*time.Hour + 59*time.Minute), Currency: "USD", Complete: true, Items: []domain.DailyValuationSnapshotItem{phase1aAgentItem(t, account.ID, "USD", "100", "100")}}
		current := domain.DailyValuationSnapshot{LocalDate: testCase.date, CutoffAt: previousDate.AddDate(0, 0, 1).Add(23*time.Hour + 59*time.Minute), Currency: "USD", Complete: true, Items: []domain.DailyValuationSnapshotItem{phase1aAgentItem(t, account.ID, "USD", "200", "200")}}
		noon := time.Date(previousDate.Year(), previousDate.Month(), previousDate.Day()+1, 12, 0, 0, 0, location)
		activityID := domain.NewActivityID()
		money := phase1aAgentMoney(t, "100", "USD")
		activity := domain.Activity{ID: activityID, Kind: domain.ActivityCashIn, Reason: domain.ReasonContribution, EffectiveAt: noon, EffectiveLocalDate: "1999-01-01", Effects: []domain.ActivityEffect{{ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 1, Role: domain.EffectRoleAmount, Direction: domain.EffectAdded, Target: domain.EffectTargetAccountValue, Classification: domain.ClassificationExternalInflow, AccountID: &account.ID, Money: &money}}}
		origin, originErr := domain.NewHistoryOrigin(householdID, location.String(), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		if originErr != nil {
			t.Fatal(originErr)
		}
		input := AnalysisInputs{Origin: origin, Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: "USD"}, Accounts: []domain.AccountRecord{{Account: account}}}, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{activity}}
		query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: domain.LocalDate(testCase.date), To: domain.LocalDate(testCase.date), Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true}
		result, computeErr := ComputeAnalysis(input, query)
		if computeErr != nil {
			t.Fatal(computeErr)
		}
		day := result.Days[0]
		want, signedErr := domain.NewSignedMoney(decimal.NewFromInt(100).Mul(decimal.NewFromInt(12).Div(decimal.NewFromInt(testCase.hours))), "USD")
		if signedErr != nil {
			t.Fatal(signedErr)
		}
		if !day.DietzFlow.Amount().Equal(want.Amount()) {
			t.Fatalf("%s DietzFlow = %s, want %s (12/%dh)", testCase.date, day.DietzFlow.Amount(), want.Amount(), testCase.hours)
		}
	}
}

func TestAnalyticsPhase1bDividendAndTradeFeeFollowInvestmentAssociation(t *testing.T) {
	householdID := domain.NewHouseholdID()
	account := phase1aAgentAccount("USD", domain.TrackingHoldings, domain.RoleAsset)
	account.HouseholdID = householdID
	instrumentID := domain.NewInstrumentID()
	holdingID := domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: householdID, Name: "QQQ", Type: domain.InstrumentETF, QuoteCurrency: "USD"}
	holding := domain.Holding{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}
	previous := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "USD", "0", "0", nil, nil, "", ""), analyticsPhase1aItem(t, account.ID, "USD", "100", "100", &holdingID, &instrumentID, "", ""))
	current := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "USD", "70", "70", nil, nil, "", ""), analyticsPhase1aItem(t, account.ID, "USD", "99", "99", &holdingID, &instrumentID, "", ""))
	openingPrice, _ := domain.ParseUnitPrice("100")
	closingPrice, _ := domain.ParseUnitPrice("99")
	dividendID := domain.NewActivityID()
	dividendMoney := phase1aAgentMoney(t, "70", "USD")
	dividend := domain.Activity{ID: dividendID, Kind: domain.ActivityCashDividend, Reason: domain.ReasonIncome, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02", DividendDetail: &domain.DividendDetail{HoldingID: holdingID, InstrumentID: instrumentID, Amount: dividendMoney}, Effects: []domain.ActivityEffect{{ID: domain.NewActivityEffectID(), ActivityID: dividendID, Sequence: 1, Role: domain.EffectRoleAmount, Direction: domain.EffectAdded, Target: domain.EffectTargetAccountCash, Classification: domain.ClassificationIncome, AccountID: &account.ID, Money: &dividendMoney}}}
	openingQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: openingPrice, Currency: "USD", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
	closingQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: closingPrice, Currency: "USD", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, householdID, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: "USD"}, Accounts: []domain.AccountRecord{{Account: account}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{holding}}, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{dividend}, InstrumentQuotes: []domain.InstrumentQuote{openingQuote, closingQuote}}
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeInstrument, ID: instrumentID.String()}, From: "2026-08-02", To: "2026-08-02", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment}
	result, err := ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	day := result.Days[0]
	if got := day.ReturnComponents[domain.ReturnPriceChange].Amount(); !got.Equal(decimal.NewFromInt(-1)) {
		t.Fatalf("price return = %s, want -1", got)
	}
	if got := day.ReturnComponents[domain.ReturnDividendInterest].Amount(); !got.Equal(decimal.NewFromInt(70)) {
		t.Fatalf("dividend return = %s, want 70", got)
	}
	if got := day.AssetBuckets[domain.BucketDividendInterest]; !got.Amount().IsZero() {
		t.Fatalf("instrument scope incorrectly exposed cash dividend: %+v", day.AssetBuckets)
	}
	if result.ReturnAmount == nil || !result.ReturnAmount.Amount().Equal(decimal.NewFromInt(69)) {
		t.Fatalf("instrument return = %+v, want 69", result.ReturnAmount)
	}

	fee := phase1aAgentMoney(t, "2", "USD")
	feeID := domain.NewActivityID()
	unitPrice, _ := domain.ParseUnitPrice("100")
	quantity, _ := domain.ParseQuantity("1")
	trade := domain.Activity{ID: feeID, Kind: domain.ActivityBuy, Reason: domain.ReasonPrincipal, EffectiveAt: time.Date(2026, 8, 2, 13, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02", TradeDetail: &domain.TradeDetail{Side: domain.TradeBuy, InstrumentID: instrumentID, HoldingID: holdingID, Quantity: quantity, Gross: phase1aAgentMoney(t, "100", "USD"), UnitPrice: unitPrice, Fee: &fee}, Effects: []domain.ActivityEffect{{ID: domain.NewActivityEffectID(), ActivityID: feeID, Sequence: 1, Role: domain.EffectRoleFee, Direction: domain.EffectRemoved, Target: domain.EffectTargetAccountCash, Classification: domain.ClassificationFee, AccountID: &account.ID, Money: &fee}}}
	feeInput := input
	feeInput.Activities = []domain.Activity{trade}
	feeResult, err := ComputeAnalysis(feeInput, query)
	if err != nil {
		t.Fatal(err)
	}
	feeDay := feeResult.Days[0]
	if got := feeDay.ReturnComponents[domain.ReturnInvestmentFee].Amount(); !got.Equal(decimal.NewFromInt(-2)) {
		t.Fatalf("associated trade fee return = %s, want -2", got)
	}
	if _, ok := feeDay.ReturnComponents[domain.ReturnDividendInterest]; ok {
		t.Fatal("trade fee was classified as dividend")
	}
}

func TestAnalyticsPhase1bCases04To07And18TradePath(t *testing.T) {
	type tradeSpec struct {
		name       string
		side       domain.TradeSide
		quantity   string
		unitPrice  string
		wantReturn string
	}
	for _, testCase := range []struct {
		name         string
		openingQty   string
		openingPrice string
		closingQty   string
		closingPrice string
		wantEnding   string
		trades       []tradeSpec
	}{
		{name: "case04_buy_rises", openingQty: "0", openingPrice: "100", closingQty: "1", closingPrice: "110", wantEnding: "110", trades: []tradeSpec{{name: "buy", side: domain.TradeBuy, quantity: "1", unitPrice: "100", wantReturn: "10"}}},
		{name: "case05_buy_falls", openingQty: "0", openingPrice: "100", closingQty: "1", closingPrice: "90", wantEnding: "90", trades: []tradeSpec{{name: "buy", side: domain.TradeBuy, quantity: "1", unitPrice: "100", wantReturn: "-10"}}},
		{name: "case06_partial_sell", openingQty: "2", openingPrice: "100", closingQty: "1", closingPrice: "90", wantEnding: "90", trades: []tradeSpec{{name: "sell", side: domain.TradeSell, quantity: "1", unitPrice: "100", wantReturn: "-10"}}},
		{name: "case07_multiple_buys_sells", openingQty: "2", openingPrice: "100", closingQty: "2", closingPrice: "120", wantEnding: "240", trades: []tradeSpec{{name: "buy", side: domain.TradeBuy, quantity: "1", unitPrice: "110"}, {name: "sell", side: domain.TradeSell, quantity: "1", unitPrice: "130", wantReturn: "60"}}},
		{name: "case18_zero_beginning", openingQty: "0", openingPrice: "100", closingQty: "1", closingPrice: "110", wantEnding: "110", trades: []tradeSpec{{name: "buy", side: domain.TradeBuy, quantity: "1", unitPrice: "100", wantReturn: "10"}}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			householdID := domain.NewHouseholdID()
			account := phase1aAgentAccount("USD", domain.TrackingHoldings, domain.RoleAsset)
			account.HouseholdID = householdID
			instrumentID := domain.NewInstrumentID()
			holdingID := domain.NewHoldingID()
			instrument := domain.Instrument{ID: instrumentID, HouseholdID: householdID, Name: "QQQ", Type: domain.InstrumentETF, QuoteCurrency: "USD"}
			holding := domain.Holding{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}
			openingQuantity, _ := domain.ParseQuantity(testCase.openingQty)
			closingQuantity, _ := domain.ParseQuantity(testCase.closingQty)
			openingUnitPrice, _ := domain.ParseUnitPrice(testCase.openingPrice)
			closingUnitPrice, _ := domain.ParseUnitPrice(testCase.closingPrice)
			openingNative := openingQuantity.Decimal().Mul(openingUnitPrice.Decimal()).String()
			closingNative := closingQuantity.Decimal().Mul(closingUnitPrice.Decimal()).String()
			previous := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "USD", openingNative, openingNative, &holdingID, &instrumentID, "", ""))
			current := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "USD", closingNative, closingNative, &holdingID, &instrumentID, "", ""))
			activities := make([]domain.Activity, 0, len(testCase.trades))
			quotes := []domain.InstrumentQuote{{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: openingUnitPrice, Currency: "USD", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}, {ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: closingUnitPrice, Currency: "USD", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}}
			for index, trade := range testCase.trades {
				quantity, _ := domain.ParseQuantity(trade.quantity)
				unitPrice, _ := domain.ParseUnitPrice(trade.unitPrice)
				grossValue := quantity.Decimal().Mul(unitPrice.Decimal())
				gross, _ := domain.NewMoney(grossValue, "USD")
				activityID := domain.NewActivityID()
				when := time.Date(2026, 8, 2, 12+index, 0, 0, 0, time.UTC)
				quantityDirection, cashDirection := domain.EffectAdded, domain.EffectRemoved
				if trade.side == domain.TradeSell {
					quantityDirection, cashDirection = domain.EffectRemoved, domain.EffectAdded
				}
				activities = append(activities, domain.Activity{ID: activityID, Kind: map[domain.TradeSide]domain.ActivityKind{domain.TradeBuy: domain.ActivityBuy, domain.TradeSell: domain.ActivitySell}[trade.side], Reason: domain.ReasonPrincipal, EffectiveAt: when, EffectiveLocalDate: "2026-08-02", TradeDetail: &domain.TradeDetail{Side: trade.side, InstrumentID: instrumentID, HoldingID: holdingID, Quantity: quantity, Gross: gross, UnitPrice: unitPrice}, Effects: []domain.ActivityEffect{{ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 1, Role: domain.EffectRoleQuantity, Direction: quantityDirection, Target: domain.EffectTargetHoldingQuantity, Classification: domain.ClassificationTradePrincipal, HoldingID: &holdingID, InstrumentID: &instrumentID, Quantity: &quantity}, {ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 2, Role: domain.EffectRolePrincipal, Direction: cashDirection, Target: domain.EffectTargetAccountCash, Classification: domain.ClassificationTradePrincipal, AccountID: &account.ID, Money: &gross}}})
			}
			input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, householdID, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: "USD"}, Accounts: []domain.AccountRecord{{Account: account}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{holding}}, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: activities, InstrumentQuotes: quotes}
			query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeInstrument, ID: instrumentID.String()}, From: "2026-08-02", To: "2026-08-02", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment}
			result, err := ComputeAnalysis(input, query)
			if err != nil {
				t.Fatal(err)
			}
			want := testCase.trades[len(testCase.trades)-1].wantReturn
			if want == "" {
				want = "40"
			}
			if result.ReturnAmount == nil || !result.ReturnAmount.Amount().Equal(decimal.RequireFromString(want)) {
				actual := "<nil>"
				if result.ReturnAmount != nil {
					actual = result.ReturnAmount.Amount().String()
				}
				t.Fatalf("return amount = %s, want %s; day=%+v", actual, want, result.Days[0])
			}
			if result.Days[0].ReturnComponents[domain.ReturnFXImpact].Amount().Sign() != 0 {
				t.Fatalf("trade-path return incorrectly classified as FX: %+v", result.Days[0].ReturnComponents)
			}
			if len(result.DailyReturns) != 1 || result.DailyReturns[0].Rate == nil || result.DailyReturns[0].Status != domain.CompletenessOK {
				t.Fatalf("trade-path daily rate is unavailable: %+v", result.DailyReturns)
			}
			if testCase.name == "case18_zero_beginning" && !result.DailyReturns[0].Rate.Equal(decimal.NewFromInt(1).Div(decimal.NewFromInt(5))) {
				t.Fatalf("case 18 daily rate = %s, want 20%%", *result.DailyReturns[0].Rate)
			}
			reviewAssertPeriodIdentity(t, result, testCase.wantEnding)
		})
	}
}

func TestAnalyticsPhase1bCase39RateCoverageSkipsUnavailableDays(t *testing.T) {
	householdID := domain.NewHouseholdID()
	account := phase1aAgentAccount("CNY", domain.TrackingBalance, domain.RoleAsset)
	account.HouseholdID = householdID
	item := func(date, amount string) domain.DailyValuationSnapshot {
		return analyticsPhase1aSnapshot(date, analyticsPhase1aItem(t, account.ID, "CNY", amount, amount, nil, nil, "", ""))
	}
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, householdID, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: "CNY"}, Accounts: []domain.AccountRecord{{Account: account}}}, Snapshots: []domain.DailyValuationSnapshot{item("2026-08-01", "100"), item("2026-08-02", "100"), item("2026-08-03", "100")}}
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-02", To: "2026-08-04", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true}
	result, err := ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	if result.Coverage != (domain.RateCoverage{RatedDays: 2, TotalDays: 3}) || result.Status != domain.CompletenessPartial || result.ReturnRate == nil {
		t.Fatalf("coverage with one unavailable day = %+v, status=%s, rate=%v", result.Coverage, result.Status, result.ReturnRate)
	}
	query.To = "2026-08-06"
	result, err = ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	if result.Coverage.RatedDays != 2 || result.Coverage.TotalDays != 5 || result.ReturnRate != nil {
		t.Fatalf("minority rate coverage was linked: coverage=%+v rate=%v", result.Coverage, result.ReturnRate)
	}
}

func TestAnalyticsPhase1bPeriodInvestedCapitalUsesFirstDayBeginning(t *testing.T) {
	h := domain.NewHouseholdID()
	account := analyticsPhase1aAccount(h, "CNY", domain.TrackingBalance, domain.RoleAsset)
	household := &domain.Household{ID: h, BaseCurrency: "CNY"}
	item := func(date string) domain.DailyValuationSnapshot {
		return analyticsPhase1aSnapshot(date, analyticsPhase1aItem(t, account.ID, "CNY", "100", "100", nil, nil, "", ""))
	}
	input := AnalysisInputs{
		Origin:    analyticsPhase1aOrigin(t, h, "UTC"),
		Portfolio: domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: account}}},
		Snapshots: []domain.DailyValuationSnapshot{item("2026-08-01"), item("2026-08-02"), item("2026-08-03")},
	}
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-02", To: "2026-08-03", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true}
	result, err := ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	if result.InvestedCapital == nil || !result.InvestedCapital.Amount().Equal(decimal.NewFromInt(100)) {
		t.Fatalf("period invested capital = %v, want 100", result.InvestedCapital)
	}
	if len(result.DailyReturns) != 2 {
		t.Fatalf("daily returns = %d, want 2", len(result.DailyReturns))
	}
	for _, daily := range result.DailyReturns {
		if daily.InvestedCapital == nil || !daily.InvestedCapital.Amount().Equal(decimal.NewFromInt(100)) {
			t.Fatalf("daily invested capital for %s = %v, want 100", daily.Date, daily.InvestedCapital)
		}
	}
}

func TestAnalyticsPhase1bFoldReturnGroupsUsesAdditiveDailyCapital(t *testing.T) {
	first := domain.ComponentID{AccountID: domain.NewAccountID(), InstrumentID: func() *domain.InstrumentID { id := domain.NewInstrumentID(); return &id }(), Currency: "USD", AssetClass: string(domain.InstrumentETF)}
	second := first
	second.AccountID = domain.NewAccountID()
	firstReturn, _ := domain.NewSignedMoney(decimal.NewFromInt(10), "USD")
	secondReturn, _ := domain.NewSignedMoney(decimal.NewFromInt(5), "USD")
	firstCapital, _ := domain.NewSignedMoney(decimal.NewFromInt(120), "USD")
	secondCapital, _ := domain.NewSignedMoney(decimal.NewFromInt(80), "USD")
	cash := domain.ComponentID{AccountID: domain.NewAccountID(), Currency: "USD", Cash: true}
	cashReturn, _ := domain.NewSignedMoney(decimal.NewFromInt(1000), "USD")
	cashCapital, _ := domain.NewSignedMoney(decimal.NewFromInt(1000), "USD")
	result := domain.PeriodAnalysisResult{Query: domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-02", To: "2026-08-02", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: false}, Coverage: domain.RateCoverage{TotalDays: 1}, Days: []domain.ComponentDay{{Date: "2026-08-02", Component: first, ReturnAmount: &firstReturn, InvestedCapital: &firstCapital, Status: domain.CompletenessOK}, {Date: "2026-08-02", Component: second, ReturnAmount: &secondReturn, InvestedCapital: &secondCapital, Status: domain.CompletenessOK}, {Date: "2026-08-02", Component: cash, ReturnAmount: &cashReturn, InvestedCapital: &cashCapital, Status: domain.CompletenessOK}}}
	groups, err := FoldReturnGroups(result, GroupByInstrument)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].ReturnRate == nil || !groups[0].ReturnRate.Equal(decimal.NewFromInt(15).Div(decimal.NewFromInt(200))) || !groups[0].ReturnAmount.Equal(decimal.NewFromInt(15)) {
		t.Fatalf("folded group = %+v, want amount 15 and rate 7.5%%", groups)
	}
}

func TestAnalyticsPhase1bCase19NoonContributionUsesWeightedDietzCapital(t *testing.T) {
	h := domain.NewHouseholdID()
	household := &domain.Household{ID: h, BaseCurrency: "CNY"}
	account := analyticsPhase1aAccount(h, "CNY", domain.TrackingHoldings, domain.RoleAsset)
	instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: h, Name: "fund", Type: domain.InstrumentETF, QuoteCurrency: "CNY"}
	state := reviewChangeState(t, h, reviewAccountState(t, account, "0"))
	state.Cash[account.ID] = map[domain.CurrencyCode]domain.Money{"CNY": mustMoney(t, "0", "CNY")}
	state.Holdings[holdingID] = domain.ChangeHoldingState{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID, InstrumentName: instrument.Name, Currency: "CNY", Current: mustQuantity(t, "1")}
	state.Instruments[instrumentID] = instrument
	noon := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	contribution := reviewApplyChange(t, &state, domain.MoneyAddedInput{HouseholdID: h, AccountID: account.ID, Amount: mustMoney(t, "90000", "CNY"), Reason: domain.ReasonContribution, EffectiveAt: noon})
	trade := reviewApplyChange(t, &state, domain.TradeInput{HouseholdID: h, Side: domain.TradeBuy, SettlementAccountID: account.ID, HoldingID: holdingID, InstrumentID: instrumentID, Quantity: mustQuantity(t, "9"), Gross: mustMoney(t, "90000", "CNY"), EffectiveAt: noon})
	openQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "10000"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
	closeQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "10100"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	previous := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "CNY", "0", "0", nil, nil, "", ""), analyticsPhase1aItem(t, account.ID, "CNY", "10000", "10000", &holdingID, &instrumentID, openQuote.ID.String(), ""))
	current := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "CNY", "0", "0", nil, nil, "", ""), analyticsPhase1aItem(t, account.ID, "CNY", "101000", "101000", &holdingID, &instrumentID, closeQuote.ID.String(), ""))
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, h, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: account}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}}}, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{contribution, trade}, InstrumentQuotes: []domain.InstrumentQuote{openQuote, closeQuote}}
	query := analyticsPhase1aBaseQuery(domain.ValuationBase)
	result, err := ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	if result.ReturnAmount == nil || !result.ReturnAmount.Amount().Equal(decimal.NewFromInt(1000)) {
		t.Fatalf("case 19 return amount = %+v, want 1000", result.ReturnAmount)
	}
	wantRate := decimal.NewFromInt(1).Div(decimal.NewFromInt(55))
	if result.ReturnRate == nil || !result.ReturnRate.Equal(wantRate) {
		t.Fatalf("case 19 return rate = %v, want %s", result.ReturnRate, wantRate)
	}
	if result.Coverage != (domain.RateCoverage{RatedDays: 1, TotalDays: 1}) || result.Status != domain.CompletenessOK {
		t.Fatalf("case 19 coverage/status = %+v/%s", result.Coverage, result.Status)
	}
	cashDay := analyticsPhase1aFindDay(t, result, domain.ComponentID{AccountID: account.ID, Currency: "CNY", Cash: true})
	if len(cashDay.DietzCapitalFlows) != 1 || !cashDay.DietzCapitalFlows[0].Amount.Amount().Equal(decimal.NewFromInt(90000)) {
		t.Fatalf("case 19 cash Dietz flow = %+v", cashDay.DietzCapitalFlows)
	}
	reviewAssertPeriodIdentity(t, result, "101000")
}

func TestAnalyticsPhase1bCase20SalaryIsIncomeAndCashInclusionControlsDietz(t *testing.T) {
	h := domain.NewHouseholdID()
	household := &domain.Household{ID: h, BaseCurrency: "CNY"}
	account := analyticsPhase1aAccount(h, "CNY", domain.TrackingBalance, domain.RoleAsset)
	state := reviewChangeState(t, h, reviewAccountState(t, account, "100"))
	salary := reviewApplyChange(t, &state, domain.MoneyAddedInput{HouseholdID: h, AccountID: account.ID, Amount: mustMoney(t, "100", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)})
	previous := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "CNY", "100", "100", nil, nil, "", ""))
	current := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "CNY", "200", "200", nil, nil, "", ""))
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, h, "UTC"), Portfolio: reviewPortfolio(household, account), Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{salary}}
	for _, includeCash := range []bool{true, false} {
		t.Run(map[bool]string{true: "included_cash", false: "excluded_cash"}[includeCash], func(t *testing.T) {
			query := analyticsPhase1aBaseQuery(domain.ValuationBase)
			query.IncludeCash = includeCash
			result, err := ComputeAnalysis(input, query)
			if err != nil {
				t.Fatal(err)
			}
			day := result.Days[0]
			if got := analyticsPhase1aBucket(day, domain.BucketIncome); !got.Equal(decimal.NewFromInt(100)) || day.Residual != nil {
				t.Fatalf("salary asset projection = %+v", day)
			}
			reviewAssertIdentity(t, day, "200")
			if includeCash {
				if len(day.DietzCapitalFlows) != 1 || !day.DietzCapitalFlows[0].Amount.Amount().Equal(decimal.NewFromInt(100)) || result.ReturnRate == nil || !result.ReturnRate.IsZero() {
					t.Fatalf("included salary was not weighted Dietz capital: day=%+v result=%+v", day, result)
				}
			} else if len(day.DietzCapitalFlows) != 0 || result.ReturnRate != nil || result.Coverage.RatedDays != 0 {
				t.Fatalf("excluded salary entered return denominator: day=%+v result=%+v", day, result)
			}
		})
	}
}

func TestAnalyticsPhase1bCases11And22TradeFeeIsInvestmentReturn(t *testing.T) {
	h := domain.NewHouseholdID()
	household := &domain.Household{ID: h, BaseCurrency: "USD"}
	account := analyticsPhase1aAccount(h, "USD", domain.TrackingHoldings, domain.RoleAsset)
	instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: h, Name: "QQQ", Type: domain.InstrumentETF, QuoteCurrency: "USD"}
	state := reviewChangeState(t, h, reviewAccountState(t, account, "0"))
	state.Cash[account.ID] = map[domain.CurrencyCode]domain.Money{"USD": mustMoney(t, "102", "USD")}
	state.Holdings[holdingID] = domain.ChangeHoldingState{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID, InstrumentName: instrument.Name, Currency: "USD", Current: mustQuantity(t, "0")}
	state.Instruments[instrumentID] = instrument
	fee := mustMoney(t, "2", "USD")
	trade := reviewApplyChange(t, &state, domain.TradeInput{HouseholdID: h, Side: domain.TradeBuy, SettlementAccountID: account.ID, HoldingID: holdingID, InstrumentID: instrumentID, Quantity: mustQuantity(t, "1"), Gross: mustMoney(t, "100", "USD"), Fee: &fee, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)})
	openQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "100"), Currency: "USD", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
	closeQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "100"), Currency: "USD", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	previous := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "USD", "102", "102", nil, nil, "", ""), analyticsPhase1aItem(t, account.ID, "USD", "0", "0", &holdingID, &instrumentID, openQuote.ID.String(), ""))
	current := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "USD", "0", "0", nil, nil, "", ""), analyticsPhase1aItem(t, account.ID, "USD", "100", "100", &holdingID, &instrumentID, closeQuote.ID.String(), ""))
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, h, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: account}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}}}, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{trade}, InstrumentQuotes: []domain.InstrumentQuote{openQuote, closeQuote}}
	householdQuery := analyticsPhase1aBaseQuery(domain.ValuationBase)
	householdResult, err := ComputeAnalysis(input, householdQuery)
	if err != nil {
		t.Fatal(err)
	}
	cashDay := analyticsPhase1aFindDay(t, householdResult, domain.ComponentID{AccountID: account.ID, Currency: "USD", Cash: true})
	if got := analyticsPhase1aBucket(cashDay, domain.BucketFee); !got.Equal(decimal.NewFromInt(-2)) || cashDay.Residual != nil {
		t.Fatalf("case 11 cash fee projection = %+v", cashDay)
	}
	holdingDay := analyticsPhase1aFindDay(t, householdResult, domain.ComponentID{AccountID: account.ID, HoldingID: &holdingID, InstrumentID: &instrumentID, Currency: "USD"})
	if got := holdingDay.ReturnComponents[domain.ReturnInvestmentFee].Amount(); !got.Equal(decimal.NewFromInt(-2)) || len(holdingDay.DietzCapitalFlows) != 0 || !analyticsPhase1aBucket(holdingDay, domain.BucketPriceChange).IsZero() {
		t.Fatalf("case 11 associated fee return/flow = %+v/%+v", holdingDay.ReturnComponents, holdingDay.DietzCapitalFlows)
	}
	reviewAssertPeriodIdentity(t, householdResult, "100")

	instrumentQuery := householdQuery
	instrumentQuery.Scope = domain.AnalysisScope{Kind: domain.ScopeInstrument, ID: instrumentID.String()}
	instrumentResult, err := ComputeAnalysis(input, instrumentQuery)
	if err != nil {
		t.Fatal(err)
	}
	instrumentDay := instrumentResult.Days[0]
	if !analyticsPhase1aBucket(instrumentDay, domain.BucketExternalFlow).Equal(decimal.NewFromInt(100)) || !analyticsPhase1aBucket(instrumentDay, domain.BucketFee).IsZero() || instrumentDay.Residual != nil || !instrumentDay.ReturnComponents[domain.ReturnInvestmentFee].Amount().Equal(decimal.NewFromInt(-2)) {
		t.Fatalf("case 22 QQQ fee leaked to Asset Changes = %+v", instrumentDay)
	}
}

func TestAnalyticsPhase1bCase12DividendNetCashAndAssociatedTax(t *testing.T) {
	h := domain.NewHouseholdID()
	household := &domain.Household{ID: h, BaseCurrency: "USD"}
	account := analyticsPhase1aAccount(h, "USD", domain.TrackingHoldings, domain.RoleAsset)
	instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: h, Name: "QQQ", Type: domain.InstrumentETF, QuoteCurrency: "USD"}
	state := reviewChangeState(t, h, reviewAccountState(t, account, "0"))
	state.Cash[account.ID] = map[domain.CurrencyCode]domain.Money{"USD": mustMoney(t, "0", "USD")}
	state.Holdings[holdingID] = domain.ChangeHoldingState{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID, InstrumentName: instrument.Name, Currency: "USD", Current: mustQuantity(t, "1")}
	state.Instruments[instrumentID] = instrument
	noon := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	dividend := reviewApplyChange(t, &state, domain.CashDividendInput{HouseholdID: h, HoldingID: holdingID, Amount: mustMoney(t, "70", "USD"), EffectiveAt: noon})
	associatedTax := reviewApplyChange(t, &state, domain.MoneyRemovedInput{HouseholdID: h, AccountID: account.ID, HoldingID: &holdingID, Amount: mustMoney(t, "30", "USD"), Reason: domain.ReasonTax, EffectiveAt: noon})
	unassociatedTax := reviewApplyChange(t, &state, domain.MoneyRemovedInput{HouseholdID: h, AccountID: account.ID, Amount: mustMoney(t, "5", "USD"), Reason: domain.ReasonTax, EffectiveAt: noon})
	openQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "100"), Currency: "USD", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
	closeQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "100"), Currency: "USD", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	previous := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "USD", "0", "0", nil, nil, "", ""), analyticsPhase1aItem(t, account.ID, "USD", "100", "100", &holdingID, &instrumentID, openQuote.ID.String(), ""))
	current := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "USD", "35", "35", nil, nil, "", ""), analyticsPhase1aItem(t, account.ID, "USD", "100", "100", &holdingID, &instrumentID, closeQuote.ID.String(), ""))
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, h, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: account}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}}}, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{dividend, associatedTax, unassociatedTax}, InstrumentQuotes: []domain.InstrumentQuote{openQuote, closeQuote}}
	result, err := ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	cashDay := analyticsPhase1aFindDay(t, result, domain.ComponentID{AccountID: account.ID, Currency: "USD", Cash: true})
	if got := analyticsPhase1aBucket(cashDay, domain.BucketDividendInterest); !got.Equal(decimal.NewFromInt(70)) || !analyticsPhase1aBucket(cashDay, domain.BucketFee).Equal(decimal.NewFromInt(-35)) || cashDay.Residual != nil {
		t.Fatalf("case 12 cash identity = %+v", cashDay)
	}
	holdingDay := analyticsPhase1aFindDay(t, result, domain.ComponentID{AccountID: account.ID, HoldingID: &holdingID, InstrumentID: &instrumentID, Currency: "USD"})
	if !holdingDay.ReturnComponents[domain.ReturnDividendInterest].Amount().Equal(decimal.NewFromInt(70)) || !holdingDay.ReturnComponents[domain.ReturnInvestmentFee].Amount().Equal(decimal.NewFromInt(-30)) {
		t.Fatalf("case 12 return association = %+v", holdingDay.ReturnComponents)
	}
	if result.ReturnAmount == nil || !result.ReturnAmount.Amount().Equal(decimal.NewFromInt(40)) {
		t.Fatalf("case 12 net return = %+v, want 40", result.ReturnAmount)
	}
	if len(holdingDay.DietzCapitalFlows) != 0 {
		t.Fatalf("associated tax became capital flow: %+v", holdingDay.DietzCapitalFlows)
	}
	if len(cashDay.DietzCapitalFlows) != 1 || !cashDay.DietzCapitalFlows[0].Amount.Amount().Equal(decimal.NewFromInt(-5)) {
		t.Fatalf("unassociated tax did not remain a cash Dietz flow: %+v", cashDay.DietzCapitalFlows)
	}
	reviewAssertPeriodIdentity(t, result, "135")
}

func TestAnalyticsPhase1bCase28BankMaintenanceFeeIsCapitalNotInvestmentReturn(t *testing.T) {
	h := domain.NewHouseholdID()
	household := &domain.Household{ID: h, BaseCurrency: "USD"}
	account := analyticsPhase1aAccount(h, "USD", domain.TrackingBalance, domain.RoleAsset)
	state := reviewChangeState(t, h, reviewAccountState(t, account, "100"))
	fee := reviewApplyChange(t, &state, domain.MoneyRemovedInput{HouseholdID: h, AccountID: account.ID, Amount: mustMoney(t, "5", "USD"), Reason: domain.ReasonFee, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)})
	previous := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "USD", "100", "100", nil, nil, "", ""))
	current := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "USD", "95", "95", nil, nil, "", ""))
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, h, "UTC"), Portfolio: reviewPortfolio(household, account), Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{fee}}
	query := analyticsPhase1aBaseQuery(domain.ValuationBase)
	query.IncludeCash = true
	result, err := ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	day := analyticsPhase1aFindDay(t, result, domain.ComponentID{AccountID: account.ID, Currency: "USD", Cash: true})
	if got := analyticsPhase1aBucket(day, domain.BucketFee); !got.Equal(decimal.NewFromInt(-5)) || day.Residual != nil {
		t.Fatalf("case 28 Asset Changes fee = %+v", day)
	}
	if len(day.DietzCapitalFlows) != 1 || !day.DietzCapitalFlows[0].Amount.Amount().Equal(decimal.NewFromInt(-5)) {
		t.Fatalf("case 28 Dietz flow = %+v, want -5", day.DietzCapitalFlows)
	}
	if _, ok := day.ReturnComponents[domain.ReturnInvestmentFee]; ok || result.ReturnAmount == nil || !result.ReturnAmount.Amount().IsZero() || result.ReturnRate == nil || !result.ReturnRate.IsZero() {
		t.Fatalf("case 28 bank fee leaked into investment return: day=%+v result=%+v", day, result)
	}
}

func TestAnalyticsPhase1bCase30GroupRateUsesOwnDietzDenominator(t *testing.T) {
	account := domain.NewAccountID()
	instrumentID := domain.NewInstrumentID()
	component := domain.ComponentID{AccountID: account, InstrumentID: &instrumentID, Currency: "USD", AssetClass: string(domain.InstrumentETF)}
	returnAmount, _ := domain.NewSignedMoney(decimal.NewFromInt(100), "USD")
	capital, _ := domain.NewSignedMoney(decimal.NewFromInt(550), "USD")
	result := domain.PeriodAnalysisResult{Query: domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-02", To: "2026-08-02", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment}, Coverage: domain.RateCoverage{TotalDays: 1}, Days: []domain.ComponentDay{{Date: "2026-08-02", Component: component, ReturnAmount: &returnAmount, InvestedCapital: &capital, Status: domain.CompletenessOK}}}
	groups, err := FoldReturnGroups(result, GroupByInstrument)
	if err != nil {
		t.Fatal(err)
	}
	want := decimal.NewFromInt(100).Div(decimal.NewFromInt(550))
	if len(groups) != 1 || groups[0].ReturnRate == nil || !groups[0].ReturnRate.Equal(want) || groups[0].ReturnRate.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("case 30 group = %+v, want own Dietz rate %s", groups, want)
	}
}

func TestAnalyticsPhase1bCase42FoldMatchesInstrumentScopeEngine(t *testing.T) {
	h := domain.NewHouseholdID()
	household := &domain.Household{ID: h, BaseCurrency: "USD"}
	firstAccount := analyticsPhase1aAccount(h, "USD", domain.TrackingHoldings, domain.RoleAsset)
	secondAccount := analyticsPhase1aAccount(h, "USD", domain.TrackingHoldings, domain.RoleAsset)
	instrumentID := domain.NewInstrumentID()
	firstHoldingID, secondHoldingID := domain.NewHoldingID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: h, Name: "QQQ", Type: domain.InstrumentETF, QuoteCurrency: "USD"}
	openQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "100"), Currency: "USD", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
	closeQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "101"), Currency: "USD", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	state := reviewChangeState(t, h, reviewAccountState(t, firstAccount, "90000"), reviewAccountState(t, secondAccount, "0"))
	state.Cash[firstAccount.ID] = map[domain.CurrencyCode]domain.Money{"USD": mustMoney(t, "90000", "USD")}
	state.Holdings[firstHoldingID] = domain.ChangeHoldingState{ID: firstHoldingID, AccountID: firstAccount.ID, InstrumentID: instrumentID, InstrumentName: instrument.Name, Currency: "USD", Current: mustQuantity(t, "0")}
	state.Holdings[secondHoldingID] = domain.ChangeHoldingState{ID: secondHoldingID, AccountID: secondAccount.ID, InstrumentID: instrumentID, InstrumentName: instrument.Name, Currency: "USD", Current: mustQuantity(t, "100")}
	state.Instruments[instrumentID] = instrument
	trade := reviewApplyChange(t, &state, domain.TradeInput{HouseholdID: h, Side: domain.TradeBuy, SettlementAccountID: firstAccount.ID, HoldingID: firstHoldingID, InstrumentID: instrumentID, Quantity: mustQuantity(t, "900"), Gross: mustMoney(t, "90000", "USD"), EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)})
	previous := analyticsPhase1aSnapshot("2026-08-01",
		analyticsPhase1aItem(t, firstAccount.ID, "USD", "90000", "90000", nil, nil, "", ""),
		analyticsPhase1aItem(t, firstAccount.ID, "USD", "0", "0", &firstHoldingID, &instrumentID, openQuote.ID.String(), ""),
		analyticsPhase1aItem(t, secondAccount.ID, "USD", "10000", "10000", &secondHoldingID, &instrumentID, openQuote.ID.String(), ""),
	)
	current := analyticsPhase1aSnapshot("2026-08-02",
		analyticsPhase1aItem(t, firstAccount.ID, "USD", "0", "0", nil, nil, "", ""),
		analyticsPhase1aItem(t, firstAccount.ID, "USD", "90900", "90900", &firstHoldingID, &instrumentID, closeQuote.ID.String(), ""),
		analyticsPhase1aItem(t, secondAccount.ID, "USD", "10100", "10100", &secondHoldingID, &instrumentID, closeQuote.ID.String(), ""),
	)
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, h, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: firstAccount}, {Account: secondAccount}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{{ID: firstHoldingID, AccountID: firstAccount.ID, InstrumentID: instrumentID}, {ID: secondHoldingID, AccountID: secondAccount.ID, InstrumentID: instrumentID}}}, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{trade}, InstrumentQuotes: []domain.InstrumentQuote{openQuote, closeQuote}}
	query := analyticsPhase1aBaseQuery(domain.ValuationBase)
	query.IncludeCash = true
	portfolioResult, err := ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	groups, err := FoldReturnGroups(portfolioResult, GroupByInstrument)
	if err != nil {
		t.Fatal(err)
	}
	instrumentQuery := query
	instrumentQuery.Scope = domain.AnalysisScope{Kind: domain.ScopeInstrument, ID: instrumentID.String()}
	instrumentResult, err := ComputeAnalysis(input, instrumentQuery)
	if err != nil {
		t.Fatal(err)
	}
	var folded *ReturnGroup
	for index := range groups {
		if groups[index].Key == instrumentID.String() {
			folded = &groups[index]
			break
		}
	}
	if folded == nil || !folded.ReturnAmount.Equal(decimal.NewFromInt(1000)) || folded.ReturnRate == nil || !folded.ReturnRate.Equal(decimal.NewFromInt(1).Div(decimal.NewFromInt(55))) {
		t.Fatalf("case 42 folded group = %+v, want 1000 and 1/55", folded)
	}
	if instrumentResult.ReturnAmount == nil || instrumentResult.ReturnRate == nil || !folded.ReturnAmount.Equal(instrumentResult.ReturnAmount.Amount()) || !folded.ReturnRate.Equal(*instrumentResult.ReturnRate) || folded.Coverage != instrumentResult.Coverage || folded.Status != instrumentResult.Status {
		t.Fatalf("case 42 folded=%+v engine=%+v", folded, instrumentResult)
	}
}
