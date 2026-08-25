package holding_test

import (
	"context"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/holding"
	"github.com/waltwang/nestworth-go/internal/wailsapi/household"
	"github.com/waltwang/nestworth-go/internal/wailsapi/instrument"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

type fixture struct {
	app          *application.Service
	accountID    string
	instrumentID string
}

func onboardedFixture(t *testing.T) fixture {
	t.Helper()
	app := wailstest.NewService(t)
	ctx := context.Background()
	if err := household.NewService(app).CompleteOnboarding(ctx, household.CompleteOnboardingRequest{
		HouseholdName: "H", BaseCurrency: "USD", MemberNames: []string{"Alice"},
	}); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	bootstrap, err := household.NewService(app).Bootstrap(ctx)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	accountRecord, err := account.NewService(app).CreateAccount(ctx, account.CreateAccountRequest{
		Name: "Brokerage", PrimaryCategory: "investment", SecondaryCategory: "brokerage_account",
		TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInInvestment: true,
		OwnerIDs: []string{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	instrumentDTO, err := instrument.NewService(app).CreateInstrument(ctx, instrument.InstrumentRequest{
		Name: "NVIDIA", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatalf("CreateInstrument: %v", err)
	}
	return fixture{app: app, accountID: accountRecord.Account.ID, instrumentID: instrumentDTO.ID}
}

func TestCreateAndListHolding(t *testing.T) {
	fx := onboardedFixture(t)
	service := holding.NewService(fx.app)
	ctx := context.Background()
	created, err := service.CreateHolding(ctx, holding.CreateHoldingRequest{
		AccountID: fx.accountID, InstrumentID: fx.instrumentID, Quantity: "10",
	})
	if err != nil {
		t.Fatalf("CreateHolding: %v", err)
	}
	if created.Quantity != "10" {
		t.Fatalf("Quantity = %q, want 10", created.Quantity)
	}
	list, err := service.ListHoldings(ctx, fx.accountID, false)
	if err != nil {
		t.Fatalf("ListHoldings: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v", list)
	}
}

func TestUpdateHoldingQuantityAndNote(t *testing.T) {
	fx := onboardedFixture(t)
	service := holding.NewService(fx.app)
	ctx := context.Background()
	created, err := service.CreateHolding(ctx, holding.CreateHoldingRequest{
		AccountID: fx.accountID, InstrumentID: fx.instrumentID, Quantity: "10",
	})
	if err != nil {
		t.Fatalf("CreateHolding: %v", err)
	}
	note := "core position"
	updated, err := service.UpdateHolding(ctx, created.ID, holding.UpdateHoldingRequest{
		Quantity: "15", QuantitySet: true, Note: &note, NoteSet: true,
	})
	if err != nil {
		t.Fatalf("UpdateHolding: %v", err)
	}
	if updated.Quantity != "15" || updated.Note == nil || *updated.Note != "core position" {
		t.Fatalf("updated = %+v", updated)
	}
}

func TestHoldingsByAccounts(t *testing.T) {
	fx := onboardedFixture(t)
	service := holding.NewService(fx.app)
	ctx := context.Background()
	if _, err := service.CreateHolding(ctx, holding.CreateHoldingRequest{
		AccountID: fx.accountID, InstrumentID: fx.instrumentID, Quantity: "10",
	}); err != nil {
		t.Fatalf("CreateHolding: %v", err)
	}
	grouped, err := service.HoldingsByAccounts(ctx, []string{fx.accountID})
	if err != nil {
		t.Fatalf("HoldingsByAccounts: %v", err)
	}
	if len(grouped[fx.accountID]) != 1 {
		t.Fatalf("grouped = %+v, want one holding for account", grouped)
	}
}

func TestArchiveHoldingRequiresZeroQuantityAfterHistoryStarts(t *testing.T) {
	fx := onboardedFixture(t)
	service := holding.NewService(fx.app)
	ctx := context.Background()
	created, err := service.CreateHolding(ctx, holding.CreateHoldingRequest{
		AccountID: fx.accountID, InstrumentID: fx.instrumentID, Quantity: "0",
	})
	if err != nil {
		t.Fatalf("CreateHolding: %v", err)
	}
	if err := service.ArchiveHolding(ctx, created.ID, true); err != nil {
		t.Fatalf("ArchiveHolding: %v", err)
	}
}

func TestAppendAndListAccountCashValue(t *testing.T) {
	fx := onboardedFixture(t)
	service := holding.NewService(fx.app)
	ctx := context.Background()
	value, err := service.AppendAccountCashValue(ctx, fx.accountID, "500", "USD", "")
	if err != nil {
		t.Fatalf("AppendAccountCashValue: %v", err)
	}
	if value.Amount.Amount != "500" || value.Amount.Currency != "USD" {
		t.Fatalf("value = %+v", value)
	}
	list, err := service.ListAccountCashValues(ctx, fx.accountID)
	if err != nil {
		t.Fatalf("ListAccountCashValues: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list = %+v, want one cash value", list)
	}
}
