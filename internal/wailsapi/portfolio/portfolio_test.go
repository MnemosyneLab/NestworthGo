package portfolio_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/household"
	"github.com/waltwang/nestworth-go/internal/wailsapi/portfolio"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

func setup(t *testing.T) (*application.Service, string) {
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
	memberID := bootstrap.Members[0].ID
	accountService := account.NewService(app)
	if _, err := accountService.CreateAccount(ctx, account.CreateAccountRequest{
		Name: "Bank", PrimaryCategory: "cash_equivalent", SecondaryCategory: "bank_account",
		TrackingMode: "balance", DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInInvestment: true,
		OwnerIDs: []string{memberID}, InitialAmount: "1000",
	}); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if _, err := accountService.CreateAccount(ctx, account.CreateAccountRequest{
		Name: "Card", PrimaryCategory: "liability", SecondaryCategory: "credit_card",
		TrackingMode: "balance", DefaultCurrency: "USD", IncludeInNetWorth: true,
		OwnerIDs: []string{memberID}, InitialAmount: "200",
	}); err != nil {
		t.Fatalf("CreateAccount (liability): %v", err)
	}
	return app, memberID
}

func TestOverviewComputesNetWorth(t *testing.T) {
	app, _ := setup(t)
	service := portfolio.NewService(app)
	result, err := service.Overview(context.Background(), account.AccountFilterRequest{})
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if result.Assets != "1000" || result.Liabilities != "200" || result.NetWorth != "800" {
		t.Fatalf("Overview = %+v, want assets=1000 liabilities=200 netWorth=800", result)
	}
	if !result.Complete {
		t.Fatalf("Complete = false, want true")
	}
	if len(result.ByCategory) == 0 {
		t.Fatalf("ByCategory is empty, want at least one breakdown entry")
	}
}

func TestOverviewBeforeOnboarding(t *testing.T) {
	app := wailstest.NewService(t)
	service := portfolio.NewService(app)
	result, err := service.Overview(context.Background(), account.AccountFilterRequest{})
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if result.Currency != "" || result.AccountCount != 0 {
		t.Fatalf("Overview before onboarding = %+v, want zero value", result)
	}
}

func TestPortfolioValuation(t *testing.T) {
	app, _ := setup(t)
	service := portfolio.NewService(app)
	result, err := service.Portfolio(context.Background(), account.AccountFilterRequest{})
	if err != nil {
		t.Fatalf("Portfolio: %v", err)
	}
	if len(result.Accounts) != 1 {
		t.Fatalf("Accounts = %+v, want 1 (liabilities are excluded from Portfolio)", result.Accounts)
	}
}

func TestNetWorthTrendWithoutHistory(t *testing.T) {
	app, _ := setup(t)
	service := portfolio.NewService(app)
	result, err := service.NetWorthTrend(context.Background(), "30d")
	if err != nil {
		t.Fatalf("NetWorthTrend: %v", err)
	}
	if result.Currency != "USD" {
		t.Fatalf("Currency = %q, want USD", result.Currency)
	}
	if len(result.Points) != 0 {
		t.Fatalf("Points = %+v, want empty before history starts", result.Points)
	}
}

// Before history starts, NetWorthTrend intentionally short-circuits without
// validating the range (application.Service.NetWorthTrend); an unsupported
// range only becomes an error once daily snapshots are actually queried.
// This test documents that pass-through behavior rather than asserting an
// error that would not actually occur pre-history.
func TestNetWorthTrendPassesThroughRangeBeforeHistoryStarts(t *testing.T) {
	app, _ := setup(t)
	service := portfolio.NewService(app)
	result, err := service.NetWorthTrend(context.Background(), "not-a-range")
	if err != nil {
		t.Fatalf("NetWorthTrend before history starts should not validate the range yet: %v", err)
	}
	if result.Range != "not-a-range" {
		t.Fatalf("Range = %q, want the input echoed back before history starts", result.Range)
	}
}

func TestOverviewDTORoundTripsAsJSON(t *testing.T) {
	app, _ := setup(t)
	service := portfolio.NewService(app)
	result, err := service.Overview(context.Background(), account.AccountFilterRequest{})
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var decoded portfolio.OverviewDTO
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded.NetWorth != "800" {
		t.Fatalf("round-tripped NetWorth = %q, want 800", decoded.NetWorth)
	}
}
