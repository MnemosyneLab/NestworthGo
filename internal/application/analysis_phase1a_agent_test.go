package application

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func phase1aAgentAccount(currency domain.CurrencyCode, tracking domain.TrackingMode, role domain.BalanceSheetRole) domain.Account {
	return domain.Account{ID: domain.NewAccountID(), HouseholdID: domain.NewHouseholdID(), Name: "test", AccountType: domain.TypeBankAccount, BalanceSheetRole: role, TrackingMode: tracking, DefaultCurrency: currency, IncludeInNetWorth: true}
}
func phase1aAgentMoney(t *testing.T, amount, currency string) domain.Money {
	t.Helper()
	m, err := domain.ParseMoney(amount, domain.CurrencyCode(currency))
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func phase1aAgentOrigin(t *testing.T, id domain.HouseholdID) domain.HistoryOrigin {
	t.Helper()
	o, err := domain.NewHistoryOrigin(id, "UTC", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return o
}
func phase1aAgentItem(t *testing.T, account domain.AccountID, currency, native, base string) domain.DailyValuationSnapshotItem {
	m := phase1aAgentMoney(t, base, "CNY")
	return domain.DailyValuationSnapshotItem{AccountID: account, NativeAmount: native, NativeCurrency: domain.CurrencyCode(currency), BaseAmount: &m, BaseAmountExact: base, Complete: true}
}
func phase1aAgentSnapshot(date string, items ...domain.DailyValuationSnapshotItem) domain.DailyValuationSnapshot {
	d, _ := time.Parse("2006-01-02", date)
	return domain.DailyValuationSnapshot{LocalDate: date, CutoffAt: d.Add(23*time.Hour + 59*time.Minute), Currency: "CNY", Complete: true, Items: items}
}
func phase1aAgentEffect(t *testing.T, id domain.ActivityID, seq int, account domain.AccountID, amount string, direction domain.EffectDirection, class domain.ActivityClassification) domain.ActivityEffect {
	m := phase1aAgentMoney(t, amount, "CNY")
	return domain.ActivityEffect{ID: domain.NewActivityEffectID(), ActivityID: id, Sequence: seq, AccountID: &account, Direction: direction, Target: domain.EffectTargetAccountValue, Classification: class, Money: &m}
}
func phase1aAgentAmount(day domain.ComponentDay, bucket domain.AttributionBucket) decimal.Decimal {
	if v, ok := day.AssetBuckets[bucket]; ok {
		return v.Amount()
	}
	return decimal.Zero
}

func TestPhase1aAgentDebtDrawIsNetWorthNeutral(t *testing.T) {
	hID := domain.NewHouseholdID()
	h := &domain.Household{ID: hID, BaseCurrency: "CNY"}
	asset := phase1aAgentAccount("CNY", domain.TrackingBalance, domain.RoleAsset)
	asset.HouseholdID = hID
	debt := phase1aAgentAccount("CNY", domain.TrackingBalance, domain.RoleLiability)
	debt.HouseholdID = hID
	prev := phase1aAgentSnapshot("2026-08-01", phase1aAgentItem(t, asset.ID, "CNY", "1000", "1000"), phase1aAgentItem(t, debt.ID, "CNY", "0", "0"))
	curr := phase1aAgentSnapshot("2026-08-02", phase1aAgentItem(t, asset.ID, "CNY", "1500", "1500"), phase1aAgentItem(t, debt.ID, "CNY", "500", "500"))
	aID := domain.NewActivityID()
	activity := domain.Activity{ID: aID, Kind: domain.ActivityDebtDraw, Reason: domain.ReasonPrincipal, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02", Effects: []domain.ActivityEffect{
		phase1aAgentEffect(t, aID, 1, asset.ID, "500", domain.EffectAdded, domain.ClassificationDebtPrincipal),
		phase1aAgentEffect(t, aID, 2, debt.ID, "500", domain.EffectAdded, domain.ClassificationDebtPrincipal),
	}}
	input := AnalysisInputs{Origin: phase1aAgentOrigin(t, hID), Portfolio: domain.PortfolioSnapshot{Household: h, Accounts: []domain.AccountRecord{{Account: asset}, {Account: debt}}}, Snapshots: []domain.DailyValuationSnapshot{prev, curr}, Activities: []domain.Activity{activity}}
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-02", To: "2026-08-02", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true}
	result, err := ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	for _, day := range result.Days {
		if !phase1aAgentAmount(day, domain.BucketExternalFlow).IsZero() || day.Residual != nil {
			t.Fatalf("debt draw not neutral: %+v", day)
		}
	}
}

func TestPhase1aAgentInterestUsesDividendInterestBucket(t *testing.T) {
	hID := domain.NewHouseholdID()
	h := &domain.Household{ID: hID, BaseCurrency: "CNY"}
	account := phase1aAgentAccount("CNY", domain.TrackingBalance, domain.RoleAsset)
	account.HouseholdID = hID
	prev := phase1aAgentSnapshot("2026-08-01", phase1aAgentItem(t, account.ID, "CNY", "1000", "1000"))
	curr := phase1aAgentSnapshot("2026-08-02", phase1aAgentItem(t, account.ID, "CNY", "1500", "1500"))
	aID := domain.NewActivityID()
	activity := domain.Activity{ID: aID, Kind: domain.ActivityCashIn, Reason: domain.ReasonInterest, EffectiveAt: time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02", Effects: []domain.ActivityEffect{phase1aAgentEffect(t, aID, 1, account.ID, "500", domain.EffectAdded, domain.ClassificationIncome)}}
	input := AnalysisInputs{Origin: phase1aAgentOrigin(t, hID), Portfolio: domain.PortfolioSnapshot{Household: h, Accounts: []domain.AccountRecord{{Account: account}}}, Snapshots: []domain.DailyValuationSnapshot{prev, curr}, Activities: []domain.Activity{activity}}
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-02", To: "2026-08-02", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true}
	result, err := ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	day := result.Days[0]
	if got := phase1aAgentAmount(day, domain.BucketDividendInterest); got.String() != "500" {
		t.Fatalf("interest = %s", got)
	}
	if !phase1aAgentAmount(day, domain.BucketIncome).IsZero() || !phase1aAgentAmount(day, domain.BucketExternalFlow).IsZero() {
		t.Fatalf("interest leaked: %+v", day.AssetBuckets)
	}
}
