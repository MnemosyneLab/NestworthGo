package application

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"

	"github.com/shopspring/decimal"
	"time"
)

func TestAnalysisCanonicalSimpleAccountTaxonomyAndLiabilitySigns(t *testing.T) {
	household := domain.NewHouseholdID()
	bank := analysisAccount(household, "CNY", domain.TrackingBalance, domain.RoleAsset)
	property := analysisAccount(household, "CNY", domain.TrackingManualValue, domain.RoleAsset)
	property.AccountType = domain.TypeProperty
	debt := analysisAccount(household, "CNY", domain.TrackingBalance, domain.RoleLiability)
	debt.AccountType = domain.TypeOther
	input := AnalysisInputs{Origin: analysisOrigin(t, household, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: household, BaseCurrency: "CNY"}, Accounts: []domain.AccountRecord{{Account: bank}, {Account: property}, {Account: debt}}}, Snapshots: []domain.DailyValuationSnapshot{
		analysisSnapshot("2026-08-01", analysisItem(t, bank.ID, "CNY", "100", "100", nil, nil, "", ""), analysisItem(t, property.ID, "CNY", "1000", "1000", nil, nil, "", ""), analysisItem(t, debt.ID, "CNY", "50", "50", nil, nil, "", "")),
		analysisSnapshot("2026-08-02", analysisItem(t, bank.ID, "CNY", "110", "110", nil, nil, "", ""), analysisItem(t, property.ID, "CNY", "1200", "1200", nil, nil, "", ""), analysisItem(t, debt.ID, "CNY", "60", "60", nil, nil, "", "")),
	}}
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-02", To: "2026-08-02", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true}
	u, err := resolveAnalysisUniverse(input, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(u.context.InvestmentUniverse.Components) != 1 || u.context.InvestmentUniverse.Components[0].AccountID != bank.ID {
		t.Fatalf("non-investment property or debt diluted return: %+v", u.context.InvestmentUniverse)
	}
	for _, test := range []struct {
		class            string
		id               domain.AccountID
		opening, closing string
	}{{domain.BucketCash, bank.ID, "100", "110"}, {domain.BucketProperty, property.ID, "1000", "1200"}, {domain.BucketOtherLiability, debt.ID, "-50", "-60"}} {
		query.Filters.AssetClass = test.class
		result, err := ComputeAnalysis(input, query)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Days) != 1 || result.Days[0].Component.AccountID != test.id || result.Days[0].BeginningValue.CanonicalAmount() != test.opening || result.Days[0].EndingValue.CanonicalAmount() != test.closing {
			t.Fatalf("%s filter: %+v", test.class, result.Days)
		}
	}
}

func TestAnalysisOrdinaryUndoCancelsOriginalSemanticBucket(t *testing.T) {
	for _, reason := range []domain.ActivityReason{domain.ReasonIncome, domain.ReasonExpense, domain.ReasonInterest} {
		t.Run(string(reason), func(t *testing.T) {
			household := domain.NewHouseholdID()
			account := analysisAccount(household, "CNY", domain.TrackingBalance, domain.RoleAsset)
			start := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
			money := mustMoney(t, "100", "CNY")
			initial := mustMoney(t, "500", "CNY")
			state := domain.ChangeState{HouseholdID: household, OriginAt: start.Add(-24 * time.Hour), Timezone: "UTC", Now: start, Accounts: map[domain.AccountID]domain.ChangeAccountState{account.ID: {ID: account.ID, Name: "Synthetic bank", Currency: "CNY", Mode: domain.TrackingBalance, Current: initial}}}
			var command any = domain.MoneyAddedInput{HouseholdID: household, AccountID: account.ID, Amount: money, Reason: reason}
			after := "600"
			bucket := domain.BucketIncome
			if reason == domain.ReasonExpense {
				command = domain.MoneyRemovedInput{HouseholdID: household, AccountID: account.ID, Amount: money, Reason: reason}
				after = "400"
				bucket = domain.BucketSpending
			}
			if reason == domain.ReasonInterest {
				bucket = domain.BucketDividendInterest
			}
			original, err := domain.PreviewChange(state, command)
			if err != nil {
				t.Fatal(err)
			}
			state, _, err = domain.ApplyEffects(state, original.Effects)
			if err != nil {
				t.Fatal(err)
			}
			state.Now = start.Add(24 * time.Hour)
			undo, err := domain.InverseChange(state, original.Activity, original.Effects)
			if err != nil {
				t.Fatal(err)
			}
			input := AnalysisInputs{Origin: analysisOrigin(t, household, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: household, BaseCurrency: "CNY"}, Accounts: []domain.AccountRecord{{Account: account}}}, Activities: []domain.Activity{original.Activity, undo.Activity}, Snapshots: []domain.DailyValuationSnapshot{analysisSnapshot("2026-08-01", analysisItem(t, account.ID, "CNY", "500", "500", nil, nil, "", "")), analysisSnapshot("2026-08-02", analysisItem(t, account.ID, "CNY", after, after, nil, nil, "", "")), analysisSnapshot("2026-08-03", analysisItem(t, account.ID, "CNY", "500", "500", nil, nil, "", ""))}}
			result, err := ComputeAnalysis(input, domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-02", To: "2026-08-03", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Days) != 2 {
				t.Fatalf("missing days: %+v", result)
			}
			first, second := result.Days[0].AssetBuckets[bucket], result.Days[1].AssetBuckets[bucket]
			if first.Amount().IsZero() || !first.Amount().Add(second.Amount()).IsZero() {
				t.Fatalf("undo did not cancel %s: %+v", bucket, result.Days)
			}
			if original.Activity.Reason != reason || undo.Activity.Reason != domain.ReasonOther || undo.Activity.Kind != domain.ActivityReversal {
				t.Fatal("reporting rewrote immutable audit")
			}
		})
	}
}

// A real inverse has no persisted TradeDetail. The read projection must invert
// its quantity direction without mutating the original immutable trade evidence.
func TestAnalysisOrdinaryTradeUndoUsesInverseSide(t *testing.T) {
	for _, side := range []domain.TradeSide{domain.TradeBuy, domain.TradeSell} {
		t.Run(string(side), func(t *testing.T) {
			household := domain.NewHouseholdID()
			account := analysisAccount(household, "USD", domain.TrackingHoldings, domain.RoleAsset)
			instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
			instrument := domain.Instrument{ID: instrumentID, HouseholdID: household, Type: domain.InstrumentETF, QuoteCurrency: "USD"}
			quantity := mustQuantity(t, "10")
			gross, fee := mustMoney(t, "100", "USD"), mustMoney(t, "1", "USD")
			price := mustUnitPrice(t, "10")
			id := domain.NewActivityID()
			kind, qDirection, cashDirection := domain.ActivityBuy, domain.EffectAdded, domain.EffectRemoved
			values, cash := []string{"0", "120", "0"}, []string{"200", "99", "200"}
			if side == domain.TradeSell {
				kind, qDirection, cashDirection = domain.ActivitySell, domain.EffectRemoved, domain.EffectAdded
				values, cash = []string{"100", "0", "120"}, []string{"200", "299", "200"}
			}
			original := domain.Activity{ID: id, Kind: kind, Reason: domain.ReasonPrincipal, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02", TradeDetail: &domain.TradeDetail{Side: side, InstrumentID: instrumentID, HoldingID: holdingID, Quantity: quantity, Gross: gross, Fee: &fee, UnitPrice: price}, Effects: []domain.ActivityEffect{
				{ID: domain.NewActivityEffectID(), ActivityID: id, Sequence: 1, Role: domain.EffectRoleQuantity, Direction: qDirection, Target: domain.EffectTargetHoldingQuantity, Classification: domain.ClassificationTradePrincipal, HoldingID: &holdingID, InstrumentID: &instrumentID, Quantity: &quantity},
				{ID: domain.NewActivityEffectID(), ActivityID: id, Sequence: 2, Role: domain.EffectRolePrincipal, Direction: cashDirection, Target: domain.EffectTargetAccountCash, Classification: domain.ClassificationTradePrincipal, AccountID: &account.ID, Money: &gross},
				{ID: domain.NewActivityEffectID(), ActivityID: id, Sequence: 3, Role: domain.EffectRoleFee, Direction: domain.EffectRemoved, Target: domain.EffectTargetAccountCash, Classification: domain.ClassificationFee, AccountID: &account.ID, Money: &fee},
			}}
			// Match InverseChange's immutable audit shape and exact reversed effects.
			undo := domain.Activity{ID: domain.NewActivityID(), Kind: domain.ActivityReversal, Reason: domain.ReasonOther, ReversesActivityID: &id, EffectiveAt: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-03"}
			for _, effect := range original.Effects {
				effect.ID, effect.ActivityID = domain.NewActivityEffectID(), undo.ID
				if effect.Direction == domain.EffectAdded {
					effect.Direction = domain.EffectRemoved
				} else {
					effect.Direction = domain.EffectAdded
				}
				undo.Effects = append(undo.Effects, effect)
			}
			input := AnalysisInputs{Origin: analysisOrigin(t, household, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: household, BaseCurrency: "USD"}, Accounts: []domain.AccountRecord{{Account: account}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}}}, Activities: []domain.Activity{original, undo}}
			for i, day := range []string{"2026-08-01", "2026-08-02", "2026-08-03"} {
				mark := "12"
				if i == 0 {
					mark = "10"
				}
				quote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, mark), Currency: "USD", QuotedAt: time.Date(2026, 8, 1+i, 23, 0, 0, 0, time.UTC)}
				input.InstrumentQuotes = append(input.InstrumentQuotes, quote)
				input.Snapshots = append(input.Snapshots, analysisSnapshot(day, analysisItem(t, account.ID, "USD", cash[i], cash[i], nil, nil, "", ""), analysisItem(t, account.ID, "USD", values[i], values[i], &holdingID, &instrumentID, quote.ID.String(), "")))
			}
			query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-02", To: "2026-08-03", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true}
			result, err := ComputeAnalysis(input, query)
			if err != nil {
				t.Fatal(err)
			}
			want := "0"
			if side == domain.TradeSell {
				want = "20"
			}
			if result.ReturnAmount == nil || !result.ReturnAmount.Amount().Equal(decimal.RequireFromString(want)) {
				t.Fatalf("return = %+v want %s; days=%+v", result.ReturnAmount, want, result.Days)
			}
			for _, day := range result.Days {
				if day.Residual != nil && !day.Residual.Amount.Amount().IsZero() {
					t.Fatalf("inverse generated residual: %+v", day)
				}
			}
			u, err := resolveAnalysisUniverse(input, query)
			if err != nil {
				t.Fatal(err)
			}
			projected, err := u.classifyActivity(undo, input.Snapshots[2], &input.Snapshots[1], input, query)
			if err != nil {
				t.Fatal(err)
			}
			inverseSide := domain.TradeSell
			if side == domain.TradeSell {
				inverseSide = domain.TradeBuy
			}
			for _, effect := range projected {
				if effect.activity.TradeDetail != nil && effect.activity.TradeDetail.Side != inverseSide {
					t.Fatal("wrong inverse side")
				}
			}
			if original.TradeDetail.Side != side || undo.Kind != domain.ActivityReversal || undo.TradeDetail != nil {
				t.Fatal("mutated immutable evidence")
			}
		})
	}
}

func TestAnalysisIncludeCashDoesNotDiluteETFReturnWithProperty(t *testing.T) {
	household := domain.NewHouseholdID()
	broker := analysisAccount(household, "USD", domain.TrackingHoldings, domain.RoleAsset)
	property := analysisAccount(household, "USD", domain.TrackingManualValue, domain.RoleAsset)
	property.AccountType = domain.TypeProperty
	instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
	input := AnalysisInputs{Origin: analysisOrigin(t, household, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: household, BaseCurrency: "USD"}, Accounts: []domain.AccountRecord{{Account: broker}, {Account: property}}, Instruments: []domain.Instrument{{ID: instrumentID, HouseholdID: household, Type: domain.InstrumentETF, QuoteCurrency: "USD"}}, Holdings: []domain.Holding{{ID: holdingID, AccountID: broker.ID, InstrumentID: instrumentID}}}}
	for i, value := range []string{"100", "110"} {
		day := []string{"2026-08-01", "2026-08-02"}[i]
		quote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, value), Currency: "USD", QuotedAt: time.Date(2026, 8, 1+i, 23, 0, 0, 0, time.UTC)}
		input.InstrumentQuotes = append(input.InstrumentQuotes, quote)
		input.Snapshots = append(input.Snapshots, analysisSnapshot(day, analysisItem(t, broker.ID, "USD", value, value, &holdingID, &instrumentID, quote.ID.String(), ""), analysisItem(t, property.ID, "USD", "1000", "1000", nil, nil, "", "")))
	}
	for _, cash := range []bool{false, true} {
		result, err := ComputeAnalysis(input, domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-02", To: "2026-08-02", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: cash})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.DailyReturns) != 1 || result.DailyReturns[0].Rate == nil || !result.DailyReturns[0].Rate.Equal(decimal.RequireFromString("0.1")) {
			t.Fatalf("IncludeCash=%v diluted ETF return: %+v", cash, result.DailyReturns)
		}
	}
}
