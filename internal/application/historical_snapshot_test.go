package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestNegativeNetWorthSavesReloadsAndTrends(t *testing.T) {
	service, ctx, bootstrap, setClock := newOnboardedService(t, "negative-net-worth", []string{"Owner"})
	owner := bootstrap.Members[0].ID
	if _, err := service.CreateAccount(ctx, AccountInput{
		Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "CNY", IncludeInNetWorth: true,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "50",
	}); err != nil {
		t.Fatalf("asset: %v", err)
	}
	if _, err := service.CreateAccount(ctx, AccountInput{
		Name: "Card", AccountType: "credit_card", BalanceSheetRole: "liability", TrackingMode: "balance",
		DefaultCurrency: "CNY", IncludeInNetWorth: true,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "80",
	}); err != nil {
		t.Fatalf("liability: %v", err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatalf("StartHistory: %v", err)
	}
	setClock(time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC))
	overview, err := service.Overview(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if overview.Assets.String() != "50" || overview.Liabilities.String() != "80" || overview.NetWorth.String() != "-30" {
		t.Fatalf("live overview = assets=%s liabilities=%s net=%s, want 50/80/-30", overview.Assets, overview.Liabilities, overview.NetWorth)
	}
	snapshot, appended, err := service.BuildDailyValuationSnapshot(ctx, "2026-08-01")
	if err != nil || !appended {
		t.Fatalf("BuildDailyValuationSnapshot: appended=%v err=%v", appended, err)
	}
	if snapshot.NetWorthAmount == nil || snapshot.NetWorthAmount.CanonicalAmount() != "-30" {
		t.Fatalf("snapshot net worth = %+v, want -30", snapshot.NetWorthAmount)
	}
	listed, err := service.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, time.Time{})
	if err != nil || len(listed) != 1 || listed[0].NetWorthAmount == nil || listed[0].NetWorthAmount.CanonicalAmount() != "-30" {
		t.Fatalf("reloaded snapshot = %+v err=%v", listed, err)
	}
	trend, err := service.NetWorthTrend(ctx, domain.TrendAllTime)
	if err != nil {
		t.Fatalf("NetWorthTrend: %v", err)
	}
	if len(trend.Points) < 2 {
		t.Fatalf("trend points = %d, want closed day plus today", len(trend.Points))
	}
	if trend.Points[0].NetWorth == nil || trend.Points[0].NetWorth.CanonicalAmount() != "-30" {
		t.Fatalf("closed-day trend point = %+v, want -30", trend.Points[0].NetWorth)
	}
	last := trend.Points[len(trend.Points)-1]
	if last.NetWorth == nil || last.NetWorth.CanonicalAmount() != "-30" {
		t.Fatalf("today trend point = %+v, want -30", last.NetWorth)
	}
	if trend.Start == nil || trend.Start.CanonicalAmount() != "-30" || trend.End == nil || trend.End.CanonicalAmount() != "-30" {
		t.Fatalf("trend start/end = %+v / %+v, want -30", trend.Start, trend.End)
	}
	if trend.Change == nil || trend.Change.CanonicalAmount() != "0" {
		t.Fatalf("trend change = %+v, want 0", trend.Change)
	}
}

func TestHistoricalSnapshotOmitsExcludedAssetAndLiabilityAccounts(t *testing.T) {
	service, ctx, bootstrap, setClock := newOnboardedService(t, "excluded-snapshot", []string{"Owner"})
	owner := bootstrap.Members[0].ID
	included, err := service.CreateAccount(ctx, AccountInput{
		Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "CNY", IncludeInNetWorth: true,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "100",
	})
	if err != nil {
		t.Fatalf("included asset: %v", err)
	}
	excludedAsset, err := service.CreateAccount(ctx, AccountInput{
		Name: "Hidden cash", AccountType: "cash_on_hand", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "CNY", IncludeInNetWorth: false,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "9000",
	})
	if err != nil {
		t.Fatalf("excluded asset: %v", err)
	}
	excludedLiability, err := service.CreateAccount(ctx, AccountInput{
		Name: "Hidden card", AccountType: "credit_card", BalanceSheetRole: "liability", TrackingMode: "balance",
		DefaultCurrency: "CNY", IncludeInNetWorth: false,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "50",
	})
	if err != nil {
		t.Fatalf("excluded liability: %v", err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatalf("StartHistory: %v", err)
	}
	setClock(time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC))
	overview, err := service.Overview(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if overview.NetWorth.String() != "100" || overview.Assets.String() != "100" || overview.Liabilities.String() != "0" {
		t.Fatalf("live overview = assets=%s liabilities=%s net=%s, want 100/0/100", overview.Assets, overview.Liabilities, overview.NetWorth)
	}
	snapshot, appended, err := service.BuildDailyValuationSnapshot(ctx, "2026-08-01")
	if err != nil || !appended {
		t.Fatalf("BuildDailyValuationSnapshot: appended=%v err=%v", appended, err)
	}
	if snapshot.NetWorthAmount == nil || snapshot.NetWorthAmount.CanonicalAmount() != "100" || snapshot.AssetsAmount == nil || snapshot.AssetsAmount.CanonicalAmount() != "100" || snapshot.LiabilitiesAmount == nil || snapshot.LiabilitiesAmount.CanonicalAmount() != "0" {
		t.Fatalf("snapshot totals = %+v", snapshot)
	}
	for _, item := range snapshot.Items {
		if item.AccountID == excludedAsset.Account.ID || item.AccountID == excludedLiability.Account.ID {
			t.Fatalf("excluded account %s entered snapshot items: %+v", item.AccountID, item)
		}
		if item.AccountID != included.Account.ID {
			t.Fatalf("unexpected snapshot account %s", item.AccountID)
		}
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("snapshot items = %d, want 1 included account", len(snapshot.Items))
	}
	listed, err := service.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, time.Time{})
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListDailyValuationSnapshots: count=%d err=%v", len(listed), err)
	}
	if listed[0].NetWorthAmount == nil || listed[0].NetWorthAmount.CanonicalAmount() != "100" {
		t.Fatalf("reloaded snapshot net worth = %+v", listed[0].NetWorthAmount)
	}
}

func TestHighPrecisionHoldingValueSurvivesSnapshotReloadAndNetWorthTrend(t *testing.T) {
	service, ctx, bootstrap, setClock := newOnboardedService(t, "high-precision-snapshot", []string{"Owner"})
	owner := bootstrap.Members[0].ID
	account, err := service.CreateAccount(ctx, AccountInput{
		Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}},
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	stock, err := service.CreateInstrument(ctx, InstrumentInput{Name: "High precision ETF", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatalf("CreateInstrument: %v", err)
	}
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: stock.ID.String(), Quantity: "3.14159"})
	if err != nil {
		t.Fatalf("CreateHolding: %v", err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, stock.ID, "3682", "2026-08-01", false); err != nil {
		t.Fatalf("AppendManualInstrumentQuote: %v", err)
	}
	if _, err := service.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{holding.ID: "3682"}); err != nil {
		t.Fatalf("StartHistoryWithCosts: %v", err)
	}
	setClock(time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC))
	snapshot, appended, err := service.BuildDailyValuationSnapshot(ctx, "2026-08-01")
	if err != nil || !appended {
		t.Fatalf("BuildDailyValuationSnapshot: appended=%v err=%v", appended, err)
	}
	if len(snapshot.Items) != 1 || snapshot.Items[0].NativeAmount != "11567.33438" || snapshot.Items[0].BaseAmount == nil || snapshot.Items[0].BaseAmount.CanonicalAmount() != "11567.3344" || snapshot.NetWorthAmount == nil || snapshot.NetWorthAmount.CanonicalAmount() != "11567.3344" {
		t.Fatalf("snapshot native amount = %+v", snapshot.Items)
	}
	listed, err := service.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, time.Time{})
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListDailyValuationSnapshots: count=%d err=%v", len(listed), err)
	}
	if listed[0].Items[0].NativeAmount != "11567.33438" {
		t.Fatalf("reloaded native amount = %q", listed[0].Items[0].NativeAmount)
	}
	trend, err := service.NetWorthTrend(ctx, domain.TrendAllTime)
	if err != nil {
		t.Fatalf("NetWorthTrend: %v", err)
	}
	if len(trend.Points) < 2 || trend.Points[0].NetWorth == nil || trend.Points[0].NetWorth.CanonicalAmount() != "11567.3344" {
		t.Fatalf("trend = %+v", trend)
	}
	if trend.Points[0].Assets == nil || trend.Points[0].Assets.CanonicalAmount() != "11567.3344" {
		t.Fatalf("trend assets = %+v", trend.Points[0].Assets)
	}
}

func TestHistoricalSnapshotMarksSimpleItemsNotCompositeOrTotals(t *testing.T) {
	service, ctx, bootstrap, setClock := newOnboardedService(t, "snapshot-classification", []string{"Owner"})
	owner := bootstrap.Members[0].ID
	bank, err := service.CreateAccount(ctx, AccountInput{
		Name: "招行", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "CNY", IncludeInNetWorth: true,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "800",
	})
	if err != nil {
		t.Fatalf("bank: %v", err)
	}
	brokerage, err := service.CreateAccount(ctx, AccountInput{
		Name: "MooMoo", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}},
	})
	if err != nil {
		t.Fatalf("brokerage: %v", err)
	}
	stock, err := service.CreateInstrument(ctx, InstrumentInput{Name: "AAPL", Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatalf("instrument: %v", err)
	}
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: brokerage.Account.ID.String(), InstrumentID: stock.ID.String(), Quantity: "2"})
	if err != nil {
		t.Fatalf("holding: %v", err)
	}
	if _, err := service.AppendAccountCashValue(ctx, brokerage.Account.ID, "200", "CNY", "2026-08-01"); err != nil {
		t.Fatalf("cash: %v", err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, stock.ID, "50", "2026-08-01", false); err != nil {
		t.Fatalf("quote: %v", err)
	}
	if _, err := service.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{holding.ID: "50"}); err != nil {
		t.Fatalf("StartHistory: %v", err)
	}
	setClock(time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC))
	snapshot, appended, err := service.BuildDailyValuationSnapshot(ctx, "2026-08-01")
	if err != nil || !appended {
		t.Fatalf("BuildDailyValuationSnapshot: appended=%v err=%v", appended, err)
	}
	assertSnapshotClassification(t, snapshot, bank.Account.ID, brokerage.Account.ID)

	listed, err := service.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, time.Time{})
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListDailyValuationSnapshots = %d err=%v", len(listed), err)
	}
	assertSnapshotClassification(t, listed[0], bank.Account.ID, brokerage.Account.ID)
}

func assertSnapshotClassification(t *testing.T, snapshot domain.DailyValuationSnapshot, simpleAccount, holdingsAccount domain.AccountID) {
	t.Helper()
	if len(snapshot.Items) == 0 {
		t.Fatal("snapshot items are empty")
	}
	var simple, holdingsCash, holdingsStock int
	for _, item := range snapshot.Items {
		switch item.AccountID {
		case simpleAccount:
			simple++
			if item.ClassificationBasis != domain.ClassificationCurrentMetadataDerived {
				t.Fatalf("simple item %+v missing current-metadata-derived basis", item)
			}
			if item.HoldingID != nil || item.InstrumentID != nil {
				t.Fatalf("simple item should not be a holding/instrument result: %+v", item)
			}
		case holdingsAccount:
			if item.ClassificationBasis != "" {
				t.Fatalf("composite item %+v has classificationBasis %q", item, item.ClassificationBasis)
			}
			if item.HoldingID == nil && item.InstrumentID == nil {
				holdingsCash++
			} else {
				holdingsStock++
			}
		default:
			t.Fatalf("unexpected snapshot account %s", item.AccountID)
		}
	}
	if simple != 1 || holdingsCash != 1 || holdingsStock != 1 {
		t.Fatalf("item counts simple=%d cash=%d stock=%d, want 1/1/1", simple, holdingsCash, holdingsStock)
	}
}

func TestBackdatedCashDividendAppearsInHistoricalSnapshots(t *testing.T) {
	service, ctx, bootstrap, setClock := newOnboardedService(t, "backdated-dividend", []string{"Owner"})
	owner := bootstrap.Members[0].ID
	brokerage, err := service.CreateAccount(ctx, AccountInput{
		Name: "MooMoo", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInPortfolio: true,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}},
	})
	if err != nil {
		t.Fatalf("brokerage: %v", err)
	}
	stock, err := service.CreateInstrument(ctx, InstrumentInput{Name: "AAPL", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatalf("instrument: %v", err)
	}
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: brokerage.Account.ID.String(), InstrumentID: stock.ID.String(), Quantity: "2"})
	if err != nil {
		t.Fatalf("holding: %v", err)
	}
	if _, err := service.AppendAccountCashValue(ctx, brokerage.Account.ID, "50", "USD", "2026-08-01"); err != nil {
		t.Fatalf("cash: %v", err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, stock.ID, "50", "2026-08-01", false); err != nil {
		t.Fatalf("quote: %v", err)
	}
	setClock(time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC))
	if _, err := service.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{holding.ID: "50"}); err != nil {
		t.Fatalf("StartHistory: %v", err)
	}
	setClock(time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC))
	amount, _ := domain.ParseMoney("25", "USD")
	if _, err := service.RecordChange(ctx, domain.CashDividendInput{
		HouseholdID: bootstrap.Household.ID, HoldingID: holding.ID, Amount: amount,
		EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("RecordChange: %v", err)
	}
	count, err := service.RebuildHistoricalSnapshots(ctx, "2026-08-01", "2026-08-03")
	if err != nil || count != 3 {
		t.Fatalf("RebuildHistoricalSnapshots count=%d err=%v", count, err)
	}
	snapshots, err := service.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	byDate := map[string]string{}
	for _, snapshot := range snapshots {
		for _, item := range snapshot.Items {
			if item.AccountID == brokerage.Account.ID && item.HoldingID == nil && item.InstrumentID == nil {
				byDate[snapshot.LocalDate] = item.NativeAmount
			}
		}
	}
	if byDate["2026-08-01"] != "50" || byDate["2026-08-02"] != "75" || byDate["2026-08-03"] != "75" {
		t.Fatalf("backdated dividend cash by date = %+v", byDate)
	}
}
