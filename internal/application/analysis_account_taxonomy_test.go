package application

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
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
