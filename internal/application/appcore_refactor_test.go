package application

import (
	"context"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func newOnboardedService(t *testing.T, name string, members []string) (*Service, context.Context, Bootstrap, func(time.Time)) {
	t.Helper()
	if len(members) == 0 {
		members = []string{"Owner"}
	}
	database, err := sqlite.Open(filepath.Join(t.TempDir(), name+".db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	repository := sqlite.NewRepository(database)
	service := NewService(repository)
	clock := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Test", BaseCurrency: "CNY", MemberNames: members}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return service, ctx, bootstrap, func(next time.Time) { clock = next }
}

func newRefactorTestService(t *testing.T, name string) (*Service, context.Context, domain.Household, func(time.Time)) {
	service, ctx, bootstrap, setClock := newOnboardedService(t, name, []string{"Owner"})
	return service, ctx, *bootstrap.Household, setClock
}

func createHoldingsAccount(t *testing.T, service *Service, ctx context.Context, owner domain.MemberID, name string) domain.AccountRecord {
	t.Helper()
	account, err := service.CreateAccount(ctx, AccountInput{Name: name, AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true, Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}}})
	if err != nil {
		t.Fatalf("%s account: %v", name, err)
	}
	return account
}

func TestHoldingsByAccountsGroupsHoldingsByAccount(t *testing.T) {
	service, ctx, _, setClock := newRefactorTestService(t, "holdings-by-accounts")
	setClock(time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC))
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owner := bootstrap.Members[0].ID
	first := createHoldingsAccount(t, service, ctx, owner, "Brokerage One")
	second := createHoldingsAccount(t, service, ctx, owner, "Brokerage Two")
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "QQQ", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.CreateInstrument(ctx, InstrumentInput{Name: "SPY", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	firstHolding, err := service.CreateHolding(ctx, HoldingInput{AccountID: first.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "3"})
	if err != nil {
		t.Fatal(err)
	}
	secondHolding, err := service.CreateHolding(ctx, HoldingInput{AccountID: second.Account.ID.String(), InstrumentID: other.ID.String(), Quantity: "1"})
	if err != nil {
		t.Fatal(err)
	}

	grouped, err := service.HoldingsByAccounts(ctx, []domain.AccountID{first.Account.ID, second.Account.ID, domain.AccountID("00000000-0000-4000-8000-0000000000ff")})
	if err != nil {
		t.Fatal(err)
	}
	if len(grouped) != 2 {
		t.Fatalf("grouped accounts = %d, want 2", len(grouped))
	}
	if got := grouped[first.Account.ID]; len(got) != 1 || got[0].ID != firstHolding.ID {
		t.Fatalf("first account holdings = %+v", got)
	}
	if got := grouped[second.Account.ID]; len(got) != 1 || got[0].ID != secondHolding.ID {
		t.Fatalf("second account holdings = %+v", got)
	}

	empty, err := service.HoldingsByAccounts(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty batch = %+v, err=%v", empty, err)
	}
}

// The append path reads the current cash value and commits the delta in one
// changeMu critical section. Distinct reconciliation targets always produce a
// non-zero delta no matter the commit order, so every concurrent append must
// succeed and serialize.
func TestConcurrentAppendAccountCashValueSerializes(t *testing.T) {
	service, ctx, _, setClock := newRefactorTestService(t, "concurrent-cash")
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	setClock(base)
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account := createHoldingsAccount(t, service, ctx, bootstrap.Members[0].ID, "Brokerage Cash")
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	setClock(base.Add(24 * time.Hour))

	const appends = 8
	var wg sync.WaitGroup
	errs := make(chan error, appends)
	for i := range appends {
		wg.Add(1)
		go func(target string) {
			defer wg.Done()
			_, err := service.AppendAccountCashValue(ctx, account.Account.ID, target, "CNY", "")
			errs <- err
		}(strconv.Itoa((i + 1) * 10))
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("append cash: %v", err)
		}
	}

	// A final reconciliation pins the committed result deterministically.
	final, err := service.AppendAccountCashValue(ctx, account.Account.ID, "100", "CNY", "")
	if err != nil {
		t.Fatalf("final reconcile: %v", err)
	}
	if final.Amount.Amount().String() != "100" || final.Amount.Currency().String() != "CNY" {
		t.Fatalf("final reconciled value = %s %s, want 100 CNY", final.Amount.Amount(), final.Amount.Currency())
	}
}
