package application

import (
	"context"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestStartingPointCostOverridePersistsAndZeroHoldingNeedsNoCost(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/starting-point-cost.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Capture", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	priced, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Priced ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	unpriced, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Empty ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	positive, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: priced.ID.String(), Quantity: "2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: unpriced.ID.String(), Quantity: "0"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, priced.ID, "100", "2026-08-24", false); err != nil {
		t.Fatal(err)
	}
	draft, err := service.StartingPointDraft(ctx)
	if err != nil || len(draft) != 1 || draft[0].HoldingID != positive.ID || draft[0].UnitCost != "100" {
		t.Fatalf("Starting Point draft = %+v, err=%v", draft, err)
	}
	if _, err := service.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{positive.ID: "123.45"}); err != nil {
		t.Fatal(err)
	}
	origin, err := service.HistoryOrigin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	components, err := service.repository.ListHistoryOriginComponents(ctx, origin.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, component := range components {
		if component.HoldingID != nil && *component.HoldingID == positive.ID {
			if component.UnitCost == nil || component.UnitCost.Canonical() != "123.45" {
				t.Fatalf("Starting Point override = %v, want 123.45", component.UnitCost)
			}
			return
		}
	}
	t.Fatal("positive holding Starting Point component was not found")
}

func TestCreateHoldingCostOverridePersistsAlreadyExistedAdjustment(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/already-existed-cost.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Capture", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Initial ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: initial.ID.String(), Quantity: "0"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Already Existed ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, created.ID, "100", "2026-08-24", false); err != nil {
		t.Fatal(err)
	}
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: created.ID.String(), Quantity: "3", UnitCost: "77.25"})
	if err != nil {
		t.Fatal(err)
	}
	events, err := service.repository.ListCostBasisEvents(ctx, holding.ID, domain.CostBasisReadFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Kind != domain.CostBasisAdjustmentIn || events[0].UnitCost == nil || events[0].UnitCost.Canonical() != "77.25" {
		t.Fatalf("already existed cost event = %+v", events)
	}
}
