package application

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestExactSubCentComponentsAgreeAcrossLiveSnapshotReloadAndTrends(t *testing.T) {
	service, ctx, bootstrap, setClock := newOnboardedService(t, "exact-subcent", []string{"Owner"})
	owner := bootstrap.Members[0].ID
	account, err := service.CreateAccount(ctx, AccountInput{
		Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}},
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	first, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Dust A", Type: "crypto", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatalf("first instrument: %v", err)
	}
	second, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Dust B", Type: "crypto", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatalf("second instrument: %v", err)
	}
	firstHolding, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: first.ID.String(), Quantity: "1"})
	if err != nil {
		t.Fatalf("first holding: %v", err)
	}
	secondHolding, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: second.ID.String(), Quantity: "1"})
	if err != nil {
		t.Fatalf("second holding: %v", err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, first.ID, "0.00006", "2026-08-01", false); err != nil {
		t.Fatalf("first quote: %v", err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, second.ID, "0.00006", "2026-08-01", false); err != nil {
		t.Fatalf("second quote: %v", err)
	}
	if _, err := service.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{firstHolding.ID: "0.00006", secondHolding.ID: "0.00006"}); err != nil {
		t.Fatalf("StartHistoryWithCosts: %v", err)
	}
	setClock(time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC))
	overview, err := service.Overview(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if overview.Assets.String() != "0.00012" || overview.NetWorth.String() != "0.00012" {
		t.Fatalf("live overview = assets=%s net=%s, want exact 0.00012", overview.Assets, overview.NetWorth)
	}
	snapshot, appended, err := service.BuildDailyValuationSnapshot(ctx, "2026-08-01")
	if err != nil || !appended {
		t.Fatalf("BuildDailyValuationSnapshot: appended=%v err=%v", appended, err)
	}
	if !strings.HasPrefix(snapshot.ContentHash, "v2:") {
		t.Fatalf("content hash = %q, want v2 prefix", snapshot.ContentHash)
	}
	if snapshot.NetWorthAmount == nil || snapshot.NetWorthAmount.CanonicalAmount() != "0.0001" || snapshot.AssetsAmount == nil || snapshot.AssetsAmount.CanonicalAmount() != "0.0001" {
		t.Fatalf("snapshot rounded totals = assets=%v net=%v, want 0.0001 from exact 0.00012", snapshot.AssetsAmount, snapshot.NetWorthAmount)
	}
	if len(snapshot.Items) != 2 {
		t.Fatalf("snapshot items = %d, want 2", len(snapshot.Items))
	}
	for _, item := range snapshot.Items {
		if item.BaseAmountExact != "0.00006" {
			t.Fatalf("stored exact base = %q, want 0.00006", item.BaseAmountExact)
		}
		if item.BaseAmount == nil || item.BaseAmount.CanonicalAmount() != "0.0001" {
			t.Fatalf("display base = %+v, want rounded 0.0001", item.BaseAmount)
		}
	}
	listed, err := service.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, time.Time{}, time.Time{})
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListDailyValuationSnapshots: count=%d err=%v", len(listed), err)
	}
	if listed[0].Items[0].BaseAmountExact != "0.00006" || listed[0].NetWorthAmount == nil || listed[0].NetWorthAmount.CanonicalAmount() != "0.0001" {
		t.Fatalf("reloaded snapshot = %+v", listed[0])
	}
	wealth, err := service.NetWorthTrend(ctx, domain.TrendAllTime)
	if err != nil {
		t.Fatalf("NetWorthTrend: %v", err)
	}
	if len(wealth.Points) < 2 || wealth.Points[0].NetWorth == nil || wealth.Points[0].NetWorth.CanonicalAmount() != "0.0001" {
		t.Fatalf("net-worth trend = %+v", wealth.Points)
	}
	portfolio, err := service.PortfolioTrend(ctx, domain.TrendAllTime)
	if err != nil {
		t.Fatalf("PortfolioTrend: %v", err)
	}
	if len(portfolio.Points) < 2 || portfolio.Points[0].ValuedSubtotal == nil || portfolio.Points[0].ValuedSubtotal.CanonicalAmount() != "0.0001" {
		t.Fatalf("portfolio trend = %+v, want rounded-once 0.0001 from exact 0.00012", portfolio.Points)
	}
}

func TestLegacySnapshotHashRebuildsAppendOnlyV2Revision(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/legacy-hash.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	clock := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Legacy", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateAccount(ctx, AccountInput{
		Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "CNY", IncludeInNetWorth: true,
		Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "100",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	first, appended, err := service.BuildDailyValuationSnapshot(ctx, "2026-08-01")
	if err != nil || !appended || !strings.HasPrefix(first.ContentHash, "v2:") {
		t.Fatalf("first snapshot hash=%q appended=%v err=%v", first.ContentHash, appended, err)
	}
	if _, err := database.SQL.Exec(`UPDATE daily_valuation_snapshots SET content_hash = 'legacy-unversioned' WHERE id = ?`, first.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.NetWorthTrend(ctx, domain.TrendAllTime); err != nil {
		t.Fatalf("NetWorthTrend rebuild: %v", err)
	}
	var revisions int
	var latestHash string
	if err := database.SQL.QueryRow(`SELECT COUNT(*), (SELECT content_hash FROM daily_valuation_snapshots WHERE household_id = ? AND local_date = '2026-08-01' ORDER BY revision DESC LIMIT 1) FROM daily_valuation_snapshots WHERE household_id = ? AND local_date = '2026-08-01'`, bootstrap.Household.ID.String(), bootstrap.Household.ID.String()).Scan(&revisions, &latestHash); err != nil {
		t.Fatal(err)
	}
	if revisions != 2 || !strings.HasPrefix(latestHash, "v2:") {
		t.Fatalf("revisions=%d latestHash=%q, want append-only v2 revision", revisions, latestHash)
	}
	var activities int
	if err := database.SQL.QueryRow(`SELECT COUNT(*) FROM activities`).Scan(&activities); err != nil {
		t.Fatal(err)
	}
	if activities != 0 {
		t.Fatalf("activities mutated during hash rebuild: %d", activities)
	}
}

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
	listed, err := service.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, time.Time{}, time.Time{})
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
	listed, err := service.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, time.Time{}, time.Time{})
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
	listed, err := service.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, time.Time{}, time.Time{})
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

	listed, err := service.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, time.Time{}, time.Time{})
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
	snapshots, err := service.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, time.Time{}, time.Time{})
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

func TestRebuildAfterResolverPolicyMigrationRestoresUnchangedSnapshotCompleteness(t *testing.T) {
	t.Run("complete unchanged result", func(t *testing.T) {
		path := t.TempDir() + "/migration-complete.db"
		database, err := sqlite.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		service := NewService(sqlite.NewRepository(database))
		clock := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
		service.setClock(func() time.Time { return clock })
		ctx := context.Background()
		if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Migration complete", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
			t.Fatal(err)
		}
		bootstrap, err := service.Bootstrap(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.CreateAccount(ctx, AccountInput{
			Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
			DefaultCurrency: "CNY", IncludeInNetWorth: true,
			Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "100",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := service.StartHistory(ctx, "UTC"); err != nil {
			t.Fatal(err)
		}
		clock = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
		first, appended, err := service.BuildDailyValuationSnapshot(ctx, "2026-08-01")
		if err != nil || !appended || !first.Complete {
			t.Fatalf("first snapshot complete=%v appended=%v err=%v", first.Complete, appended, err)
		}
		if _, err := database.SQL.ExecContext(ctx, `UPDATE history_snapshot_state SET resolver_policy_version = 'household-cutoff-close-v1' WHERE household_id = ?`, bootstrap.Household.ID.String()); err != nil {
			t.Fatal(err)
		}
		if err := database.Close(); err != nil {
			t.Fatal(err)
		}

		reopened, err := sqlite.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		var storedComplete int
		var storedHash string
		if err := reopened.SQL.QueryRowContext(ctx, `SELECT complete, content_hash FROM daily_valuation_snapshots WHERE household_id = ? AND local_date = '2026-08-01' ORDER BY revision DESC LIMIT 1`, bootstrap.Household.ID.String()).Scan(&storedComplete, &storedHash); err != nil {
			t.Fatal(err)
		}
		if storedComplete != 0 || storedHash != first.ContentHash {
			t.Fatalf("migrated snapshot complete=%d hash=%s, want incomplete %s", storedComplete, storedHash, first.ContentHash)
		}

		service = NewService(sqlite.NewRepository(reopened))
		service.setClock(func() time.Time { return clock })
		rebuilt, appended, err := service.BuildDailyValuationSnapshot(ctx, "2026-08-01")
		if err != nil || !rebuilt.Complete {
			t.Fatalf("rebuilt calculation complete=%v appended=%v err=%v", rebuilt.Complete, appended, err)
		}
		if rebuilt.ContentHash != first.ContentHash {
			t.Fatalf("rebuilt hash=%s, want unchanged %s", rebuilt.ContentHash, first.ContentHash)
		}
		listed, err := service.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, time.Time{}, time.Time{})
		if err != nil || len(listed) != 1 {
			t.Fatalf("readback count=%d err=%v", len(listed), err)
		}
		if !listed[0].Complete || listed[0].ContentHash != first.ContentHash {
			t.Fatalf("database readback = complete=%v hash=%s", listed[0].Complete, listed[0].ContentHash)
		}
		if appended {
			t.Fatal("unchanged economic rebuild appended a new revision")
		}
	})

	t.Run("incomplete rebuilt result remains incomplete", func(t *testing.T) {
		path := t.TempDir() + "/migration-incomplete.db"
		database, err := sqlite.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		service := NewService(sqlite.NewRepository(database))
		clock := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
		service.setClock(func() time.Time { return clock })
		ctx := context.Background()
		if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Migration incomplete", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
			t.Fatal(err)
		}
		bootstrap, err := service.Bootstrap(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.CreateAccount(ctx, AccountInput{
			Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
			DefaultCurrency: "CNY", IncludeInNetWorth: true,
			Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "100",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := service.StartHistory(ctx, "UTC"); err != nil {
			t.Fatal(err)
		}
		clock = time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
		account, err := service.CreateAccount(ctx, AccountInput{
			Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
			DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true,
			Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}},
		})
		if err != nil {
			t.Fatal(err)
		}
		instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Unquoted", Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "1", UnitCost: "10"}); err != nil {
			t.Fatal(err)
		}
		clock = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
		first, appended, err := service.BuildDailyValuationSnapshot(ctx, "2026-08-02")
		if err != nil || !appended || first.Complete {
			t.Fatalf("first snapshot complete=%v appended=%v err=%v", first.Complete, appended, err)
		}
		if _, err := database.SQL.ExecContext(ctx, `UPDATE history_snapshot_state SET resolver_policy_version = 'household-cutoff-close-v1' WHERE household_id = ?`, bootstrap.Household.ID.String()); err != nil {
			t.Fatal(err)
		}
		if err := database.Close(); err != nil {
			t.Fatal(err)
		}

		reopened, err := sqlite.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		service = NewService(sqlite.NewRepository(reopened))
		service.setClock(func() time.Time { return clock })
		rebuilt, _, err := service.BuildDailyValuationSnapshot(ctx, "2026-08-02")
		if err != nil || rebuilt.Complete {
			t.Fatalf("rebuilt calculation complete=%v err=%v", rebuilt.Complete, err)
		}
		listed, err := service.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, time.Time{}, time.Time{})
		if err != nil || len(listed) != 1 {
			t.Fatalf("readback count=%d err=%v", len(listed), err)
		}
		if listed[0].Complete || listed[0].ContentHash != rebuilt.ContentHash {
			t.Fatalf("incomplete readback = complete=%v hash=%s want %s", listed[0].Complete, listed[0].ContentHash, rebuilt.ContentHash)
		}
	})
}
