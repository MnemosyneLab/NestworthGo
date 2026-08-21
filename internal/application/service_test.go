package application

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestHouseholdAccountAndOverviewFlow(t *testing.T) {
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "nestworth.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	repository := sqlite.NewRepository(database)
	service := NewService(repository)
	clock := time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()

	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Wang Household", BaseCurrency: "CNY", MemberNames: []string{"Alice", "Bob"}}); err != nil {
		t.Fatalf("complete onboarding: %v", err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if bootstrap.Household == nil || len(bootstrap.Members) != 2 {
		t.Fatalf("bootstrap = %#v", bootstrap)
	}
	ownership, err := domain.ParseOwnership([]domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: 6000}, {MemberID: bootstrap.Members[1].ID, ShareBPS: 4000}})
	if err != nil {
		t.Fatalf("ownership: %v", err)
	}
	if _, err := service.CreateAccount(ctx, AccountInput{Name: "Bank", PrimaryCategory: "cash_equivalent", SecondaryCategory: "bank_account", TrackingMode: "balance", DefaultCurrency: "CNY", IncludeInNetWorth: true, Ownership: ownership.Shares(), InitialAmount: "1000"}); err != nil {
		t.Fatalf("create asset account: %v", err)
	}
	if _, err := service.CreateAccount(ctx, AccountInput{Name: "Card", PrimaryCategory: "liability", SecondaryCategory: "credit_card", TrackingMode: "balance", DefaultCurrency: "CNY", IncludeInNetWorth: true, Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "250"}); err != nil {
		t.Fatalf("create liability account: %v", err)
	}
	if _, err := service.CreateAccount(ctx, AccountInput{Name: "Excluded", PrimaryCategory: "cash_equivalent", SecondaryCategory: "cash", TrackingMode: "balance", DefaultCurrency: "CNY", IncludeInNetWorth: false, Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "9000"}); err != nil {
		t.Fatalf("create excluded account: %v", err)
	}
	accounts, err := service.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatalf("list accounts for update: %v", err)
	}
	if len(accounts) < 1 {
		t.Fatal("expected account for update")
	}
	if _, err := service.UpdateAccount(ctx, accounts[0].Account.ID, AccountInput{PrimaryCategory: "investment", SecondaryCategory: "manual_investment", TrackingMode: "manual_value"}); err == nil {
		t.Fatal("tracking mode change was accepted")
	}
	result, err := service.Overview(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if !result.Assets.Equal(decimal.RequireFromString("1000")) || !result.Liabilities.Equal(decimal.RequireFromString("250")) || !result.NetWorth.Equal(decimal.RequireFromString("750")) {
		t.Fatalf("overview totals = assets %s liabilities %s net worth %s", result.Assets, result.Liabilities, result.NetWorth)
	}
	if len(result.ByMember) != 2 {
		t.Fatalf("member breakdown length = %d", len(result.ByMember))
	}
}
