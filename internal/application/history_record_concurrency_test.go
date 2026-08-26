package application

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestConcurrentRecordChangesSerializeWithoutLostCurrentState(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/concurrent.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	clock := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Concurrent", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Cash", PrimaryCategory: "cash_equivalent", SecondaryCategory: "bank_account", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "1000", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	amount, _ := domain.ParseMoney("100", "CNY")
	commands := domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: amount, Reason: domain.ReasonIncome, EffectiveAt: clock}
	var group sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, recordErr := service.RecordChange(ctx, commands)
			errs <- recordErr
		}()
	}
	group.Wait()
	close(errs)
	for recordErr := range errs {
		if recordErr != nil {
			t.Fatal(recordErr)
		}
	}
	var amountValue string
	if err := database.SQL.QueryRow("SELECT amount FROM account_values WHERE account_id = ? ORDER BY effective_at DESC, created_at DESC, id DESC LIMIT 1", account.Account.ID.String()).Scan(&amountValue); err != nil {
		t.Fatal(err)
	}
	if amountValue != "1200" {
		t.Fatalf("serialized current amount=%q, want 1200", amountValue)
	}
}
