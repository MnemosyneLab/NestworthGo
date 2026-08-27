package analytics_test

import (
	"context"
	"testing"

	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analytics"
	"github.com/waltwang/nestworth-go/internal/wailsapi/holding"
	"github.com/waltwang/nestworth-go/internal/wailsapi/household"
	"github.com/waltwang/nestworth-go/internal/wailsapi/instrument"
	"github.com/waltwang/nestworth-go/internal/wailsapi/quote"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

func TestHoldingGainAvailableAfterQuote(t *testing.T) {
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
		Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset",
		TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInPortfolio: true,
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
	// UnitCost is deliberately not passed here: without history started,
	// application.Service.CreateHolding never records a cost-basis event
	// for it (that path only exists once a Starting Point origin exists),
	// so the average cost for this holding is 0 and the entire current
	// value is treated as unrealized gain. This matches
	// domain.ReplayCostBasis's behavior for a Holding with no cost-basis
	// events at all.
	holdingDTO, err := holding.NewService(app).CreateHolding(ctx, holding.CreateHoldingRequest{
		AccountID: accountRecord.Account.ID, InstrumentID: instrumentDTO.ID, Quantity: "10",
	})
	if err != nil {
		t.Fatalf("CreateHolding: %v", err)
	}
	if _, err := quote.NewService(app).SaveManualInstrumentQuote(ctx, instrumentDTO.ID, "150", "2026-01-15T00:00:00.000Z"); err != nil {
		t.Fatalf("SaveManualInstrumentQuote: %v", err)
	}

	service := analytics.NewService(app)
	gain, err := service.HoldingGain(ctx, holdingDTO.ID)
	if err != nil {
		t.Fatalf("HoldingGain: %v", err)
	}
	if !gain.Available {
		t.Fatalf("Available = false, want true: %+v", gain)
	}
	if gain.CurrentValue == nil || gain.CurrentValue.Amount != "1500" {
		t.Fatalf("CurrentValue = %+v, want 1500", gain.CurrentValue)
	}
	if gain.UnrealizedGain == nil || gain.UnrealizedGain.Amount != "1500" {
		t.Fatalf("UnrealizedGain = %+v, want 1500 (zero cost basis pre-history)", gain.UnrealizedGain)
	}

	accountGain, err := service.AccountGain(ctx, accountRecord.Account.ID)
	if err != nil {
		t.Fatalf("AccountGain: %v", err)
	}
	if len(accountGain.Holdings) != 1 {
		t.Fatalf("Holdings = %+v, want 1", accountGain.Holdings)
	}

	realized, err := service.RealizedGainInRange(ctx, analytics.GainScopeRequest{}, "2000-01-01", "2030-01-01")
	if err != nil {
		t.Fatalf("RealizedGainInRange: %v", err)
	}
	if !realized.Available {
		t.Fatalf("RealizedGainInRange.Available = false, want true: %+v", realized)
	}
}

func TestHoldingGainNotFound(t *testing.T) {
	app := wailstest.NewService(t)
	ctx := context.Background()
	if err := household.NewService(app).CompleteOnboarding(ctx, household.CompleteOnboardingRequest{
		HouseholdName: "H", BaseCurrency: "USD", MemberNames: []string{"Alice"},
	}); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	service := analytics.NewService(app)
	_, err := service.HoldingGain(ctx, "00000000-0000-7000-8000-000000000000")
	if err == nil {
		t.Fatal("want an error for an unknown holding")
	}
}

func TestRealizedGainInvalidDateRange(t *testing.T) {
	app := wailstest.NewService(t)
	ctx := context.Background()
	if err := household.NewService(app).CompleteOnboarding(ctx, household.CompleteOnboardingRequest{
		HouseholdName: "H", BaseCurrency: "USD", MemberNames: []string{"Alice"},
	}); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	service := analytics.NewService(app)
	_, err := service.RealizedGainInRange(ctx, analytics.GainScopeRequest{}, "2030-01-01", "2000-01-01")
	if err == nil {
		t.Fatal("want a validation error when from is after to")
	}
}
