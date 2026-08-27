package application

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestStartHistoryCapturesExistingStateAtomicallyAndRetriesIdempotently(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/history.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repository := sqlite.NewRepository(database)
	service := NewService(repository)
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return now })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "History", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "10000", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	origin, err := service.StartHistory(ctx, "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	if origin.ID == "" || origin.Timezone != "Asia/Shanghai" {
		t.Fatalf("origin = %+v", origin)
	}
	components, err := repository.ListHistoryOriginComponents(ctx, origin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(components) != 1 || components[0].Amount == nil || components[0].Amount.CanonicalAmount() != "10000" || *components[0].AccountID != account.Account.ID {
		t.Fatalf("origin components = %+v", components)
	}
	var activities int
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&activities); err != nil {
		t.Fatal(err)
	}
	if activities != 0 {
		t.Fatalf("starting point fabricated %d activities", activities)
	}
	retried, err := service.StartHistory(ctx, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if retried.ID != origin.ID || retried.Timezone != origin.Timezone {
		t.Fatalf("retry origin = %+v, first = %+v", retried, origin)
	}
}

func TestStartHistoryRequiresConfirmedIANAZone(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/history-timezone.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "History", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	_, err = service.StartHistory(ctx, "system")
	if err == nil || err.(*domain.Error).Code != domain.ErrHistoryTimezoneRequired {
		t.Fatalf("timezone error = %v", err)
	}
	var origins int
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM history_origins").Scan(&origins); err != nil {
		t.Fatal(err)
	}
	if origins != 0 {
		t.Fatalf("invalid timezone created %d origins", origins)
	}
}
