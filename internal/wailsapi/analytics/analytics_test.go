package analytics_test

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/testfixture"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"

	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/household"
	"github.com/waltwang/nestworth-go/internal/wailsapi/instrument"
	"github.com/waltwang/nestworth-go/internal/wailsapi/quote"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestHoldingGainNotFound(t *testing.T) {
	app := wailstest.NewBaselineService(t)
	ctx := context.Background()
	if err := household.NewService(app).CompleteOnboarding(ctx, household.CompleteOnboardingRequest{
		HouseholdName: "H", BaseCurrency: "USD", MemberNames: []string{"Alice"},
	}); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	_, err := app.HoldingGain(ctx, "00000000-0000-7000-8000-000000000000")
	if err == nil {
		t.Fatal("want an error for an unknown holding")
	}
}

func TestDividendIncomeIndependentOfRealizedGain(t *testing.T) {
	app := wailstest.NewBaselineService(t)
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
	rawHolding, err := testfixture.CreateHolding(app, ctx, application.HoldingInput{
		AccountID: accountRecord.Account.ID, InstrumentID: instrumentDTO.ID, Quantity: "10",
	})
	if err != nil {
		t.Fatalf("CreateHolding: %v", err)
	}
	holdingDTO := wire.FromHolding(rawHolding)
	if _, err := quote.NewService(app).AppendManualInstrumentQuote(ctx, instrumentDTO.ID, "150", "2026-01-15T00:00:00.000Z", false); err != nil {
		t.Fatalf("AppendManualInstrumentQuote: %v", err)
	}
	historyService := history.NewService(app)
	if _, err := testfixture.CaptureSyntheticOrigin(app, ctx, "UTC"); err != nil {
		t.Fatalf("CaptureSyntheticOrigin: %v", err)
	}
	if _, err := historyService.RecordChange(ctx, history.ChangeCommandRequest{
		Kind: history.ChangeCashDividend, HoldingID: holdingDTO.ID, Amount: "25", Currency: "USD",
	}); err != nil {
		t.Fatalf("RecordChange: %v", err)
	}

	income, err := app.DividendIncomeInRange(ctx, domain.GainScope{}, "2000-01-01", "2030-01-01")
	if err != nil {
		t.Fatalf("DividendIncomeInRange: %v", err)
	}
	if !income.Available || len(income.ByInstrument) != 1 || income.ByInstrument[0].Gain.Amount != "25" {
		t.Fatalf("DividendIncomeInRange = %+v", income)
	}
	realized, err := app.RealizedGainInRange(ctx, domain.GainScope{}, "2000-01-01", "2030-01-01")
	if err != nil {
		t.Fatalf("RealizedGainInRange: %v", err)
	}
	if len(realized.ByInstrument) != 0 {
		t.Fatalf("dividend mixed into realized gain: %+v", realized)
	}
}

func TestRealizedGainInvalidDateRange(t *testing.T) {
	app := wailstest.NewBaselineService(t)
	ctx := context.Background()
	if err := household.NewService(app).CompleteOnboarding(ctx, household.CompleteOnboardingRequest{
		HouseholdName: "H", BaseCurrency: "USD", MemberNames: []string{"Alice"},
	}); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	_, err := app.RealizedGainInRange(ctx, domain.GainScope{}, "2030-01-01", "2000-01-01")
	if err == nil {
		t.Fatal("want a validation error when from is after to")
	}
}
