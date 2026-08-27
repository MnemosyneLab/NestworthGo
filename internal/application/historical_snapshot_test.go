package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

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
