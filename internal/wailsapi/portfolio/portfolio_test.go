package portfolio_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
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
		Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset",
		TrackingMode: "balance", DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInPortfolio: true,
		OwnerIDs: []string{memberID}, InitialAmount: "1000",
	}); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if _, err := accountService.CreateAccount(ctx, account.CreateAccountRequest{
		Name: "Card", AccountType: "credit_card", BalanceSheetRole: "liability",
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
	if len(result.AssetsByType) == 0 {
		t.Fatalf("AssetsByType is empty, want at least one breakdown entry")
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

// Unknown trend ranges fail at ParseTrendRange on the Wails boundary, even
// before history has started. The frontend catalog is the only source of
// range buttons, so an unknown value is a client bug rather than a
// pass-through.
func TestNetWorthTrendRejectsUnknownRangeBeforeHistoryStarts(t *testing.T) {
	app, _ := setup(t)
	service := portfolio.NewService(app)
	_, err := service.NetWorthTrend(context.Background(), "not-a-range")
	if err == nil {
		t.Fatal("NetWorthTrend accepted an unknown range")
	}
	wireErr, ok := apierror.Parse(err.Error())
	if !ok {
		t.Fatalf("error is not a parseable WireError: %v", err)
	}
	if wireErr.Code != "validation" || wireErr.Field != "range" {
		t.Fatalf("error = %+v, want validation/range", wireErr)
	}
}

// TestOverviewMultiOwnerFixtureMatchesFrontendGolden is the shared fixture
// that verifies Overview's displayed net worth, assets, and liabilities exactly match
// the same fixture's Go-computed OverviewResult, for at least one ...
// multi-owner fixture"). The exact literal values asserted here
// (1000/40%/60% ownership split, 300 liability, 700 net worth) are
// duplicated verbatim as the mocked binding response in
// frontend/src/features/overview/OverviewPage.test.tsx's
// "matches the Go-computed multi-owner fixture" test; if either side
// changes, the other must change with it.
func TestOverviewMultiOwnerFixtureMatchesFrontendGolden(t *testing.T) {
	app := wailstest.NewService(t)
	ctx := context.Background()
	if err := household.NewService(app).CompleteOnboarding(ctx, household.CompleteOnboardingRequest{
		HouseholdName: "The Tans", BaseCurrency: "USD", MemberNames: []string{"Alice", "Bob"},
	}); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	bootstrap, err := household.NewService(app).Bootstrap(ctx)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	alice, bob := bootstrap.Members[0].ID, bootstrap.Members[1].ID
	accountService := account.NewService(app)
	if _, err := accountService.CreateAccount(ctx, account.CreateAccountRequest{
		Name: "Joint Savings", AccountType: "bank_account", BalanceSheetRole: "asset",
		TrackingMode: "balance", DefaultCurrency: "USD", IncludeInNetWorth: true,
		OwnerIDs: []string{alice, bob}, OwnershipPercentages: []string{"60", "40"}, InitialAmount: "1000",
	}); err != nil {
		t.Fatalf("CreateAccount (joint savings): %v", err)
	}
	if _, err := accountService.CreateAccount(ctx, account.CreateAccountRequest{
		Name: "Credit Card", AccountType: "credit_card", BalanceSheetRole: "liability",
		TrackingMode: "balance", DefaultCurrency: "USD", IncludeInNetWorth: true,
		OwnerIDs: []string{alice}, InitialAmount: "300",
	}); err != nil {
		t.Fatalf("CreateAccount (credit card): %v", err)
	}

	service := portfolio.NewService(app)
	result, err := service.Overview(ctx, account.AccountFilterRequest{})
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if result.Assets != "1000" || result.Liabilities != "300" || result.NetWorth != "700" {
		t.Fatalf("Overview = %+v, want assets=1000 liabilities=300 netWorth=700", result)
	}
	if !result.Complete {
		t.Fatalf("Complete = false, want true")
	}
	byMember := map[string]string{}
	for _, item := range result.ByMember {
		byMember[item.Label] = item.Amount
	}
	if byMember["Alice"] != "600" || byMember["Bob"] != "400" {
		t.Fatalf("ByMember = %+v, want Alice=600 Bob=400 (60/40 split of the 1000 asset)", result.ByMember)
	}
	if len(result.AssetsByType) != 1 || result.AssetsByType[0].Key != "cash" || result.AssetsByType[0].Amount != "1000" {
		t.Fatalf("AssetsByType = %+v, want cash=1000", result.AssetsByType)
	}
	if len(result.LiabilitiesByType) != 1 || result.LiabilitiesByType[0].Key != "credit_card" || result.LiabilitiesByType[0].Amount != "300" {
		t.Fatalf("LiabilitiesByType = %+v, want credit_card=300", result.LiabilitiesByType)
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
