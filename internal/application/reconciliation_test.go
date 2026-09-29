package application

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestReconciliationBuildsAndCommitsCurrentTargetsWithoutCashTrade(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "reconciliation.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := NewService(sqlite.NewRepository(db))
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	service.SetClock(func() time.Time { return now })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Home", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owner := bootstrap.Members[0].ID
	bank, err := service.CreateAccount(ctx, AccountInput{Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "100", OwnerIDs: []domain.MemberID{owner}})
	if err != nil {
		t.Fatal(err)
	}
	broker, err := service.CreateAccount(ctx, AccountInput{Name: "Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{owner}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Fund", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, broker.Account.ID, "100", "USD", "2026-01-02"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: broker.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "10", UnitCost: "25"})
	if err != nil {
		t.Fatal(err)
	}
	bankTarget := reconciliationMoney(t, "125", "USD")
	cashTarget := reconciliationMoney(t, "80", "USD")
	quantityTarget := reconciliationQuantity(t, "12")
	unitCost := reconciliationUnitCost(t, "30")
	targets := []ReconciliationTarget{
		{AccountID: &bank.Account.ID, TargetBalance: &bankTarget},
		{AccountID: &broker.Account.ID, TargetBalance: &cashTarget},
		{HoldingID: &holding.ID, TargetQuantity: &quantityTarget, UnitCost: &unitCost},
	}
	var commands []any
	previews, token, err := service.PreviewChangesGuardedWith(ctx, func(ctx context.Context) ([]any, error) {
		commands, err = service.BuildReconciliationCommands(ctx, targets)
		return commands, err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(previews) != 3 || len(commands) != 3 {
		t.Fatalf("previews = %+v, commands = %+v", previews, commands)
	}
	if previews[0].Resulting[0].Amount != "125" || previews[1].Resulting[0].Amount != "80" || previews[2].Resulting[0].Quantity != "12" {
		t.Fatalf("unexpected results: %+v", previews)
	}
	if len(previews[2].Effects) != 1 || previews[2].Effects[0].Target != domain.EffectTargetHoldingQuantity || previews[2].Effects[0].CostUnitPrice == nil {
		t.Fatalf("holding correction must contain only a priced quantity effect: %+v", previews[2].Effects)
	}
	committed, err := service.RecordChangesGuarded(ctx, commands, domain.NewMutationID().String(), strings.Repeat("a", 64), token)
	if err != nil {
		t.Fatal(err)
	}
	if len(committed) != 3 {
		t.Fatalf("committed = %+v", committed)
	}
	removeTarget := reconciliationQuantity(t, "9")
	removal, err := service.BuildReconciliationCommands(ctx, []ReconciliationTarget{{HoldingID: &holding.ID, TargetQuantity: &removeTarget}})
	if err != nil {
		t.Fatal(err)
	}
	adjustment, ok := removal[0].(domain.PositionAdjustmentInput)
	if !ok || adjustment.Added || adjustment.UnitCost != nil || adjustment.Quantity.Canonical() != "3" {
		t.Fatalf("removal = %+v", removal[0])
	}
}

func TestReconciliationRejectsMissingCostAndDuplicateTargets(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "reconciliation-errors.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := NewService(sqlite.NewRepository(db))
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	service.SetClock(func() time.Time { return now })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Home", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, _ := service.Bootstrap(ctx)
	bank, err := service.CreateAccount(ctx, AccountInput{Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "100", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	broker, err := service.CreateAccount(ctx, AccountInput{Name: "Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Fund", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: broker.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "2", UnitCost: "10"})
	if err != nil {
		t.Fatal(err)
	}
	targetQuantity := reconciliationQuantity(t, "3")
	if _, err := service.BuildReconciliationCommands(ctx, []ReconciliationTarget{{HoldingID: &holding.ID, TargetQuantity: &targetQuantity}}); !hasDomainCode(err, domain.ErrCostBasisRequired) {
		t.Fatalf("missing unit cost = %v", err)
	}
	unitCost := reconciliationUnitCost(t, "12")
	currentQuantity := reconciliationQuantity(t, "2")
	if _, err := service.BuildReconciliationCommands(ctx, []ReconciliationTarget{{HoldingID: &holding.ID, TargetQuantity: &currentQuantity, UnitCost: &unitCost}}); !hasDomainCode(err, domain.ErrInvalidChange) || !strings.Contains(err.Error(), "cost-only") {
		t.Fatalf("cost-only correction = %v", err)
	}
	entries := []ReconciliationTarget{{HoldingID: &holding.ID, TargetQuantity: &targetQuantity, UnitCost: &unitCost}, {HoldingID: &holding.ID, TargetQuantity: &targetQuantity, UnitCost: &unitCost}}
	if _, err := service.BuildReconciliationCommands(ctx, entries); !hasDomainCode(err, domain.ErrValidation) || !strings.Contains(err.Error(), "targets[1]") {
		t.Fatalf("duplicate endpoint = %v", err)
	}
	missingValue := NewService(reconciliationMissingRepository{Repository: service.repository, noAccountValue: bank.Account.ID})
	bankTarget := reconciliationMoney(t, "110", "USD")
	before, err := service.ListActivities(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := missingValue.PreviewChangesGuardedWith(ctx, func(ctx context.Context) ([]any, error) {
		return missingValue.BuildReconciliationCommands(ctx, []ReconciliationTarget{{AccountID: &bank.Account.ID, TargetBalance: &bankTarget}})
	}); !hasDomainCode(err, domain.ErrUnavailable) {
		t.Fatalf("unknown account balance = %v", err)
	}
	after, err := service.ListActivities(ctx, 100)
	if err != nil || len(after) != len(before) {
		t.Fatalf("unknown balance preview changed activities: before=%d after=%d err=%v", len(before), len(after), err)
	}
	missingCost := NewService(reconciliationMissingRepository{Repository: service.repository, noCost: holding.ID})
	if _, err := missingCost.BuildReconciliationCommands(ctx, []ReconciliationTarget{{HoldingID: &holding.ID, TargetQuantity: &targetQuantity, UnitCost: &unitCost}}); !hasDomainCode(err, domain.ErrCostBasisRequired) {
		t.Fatalf("unknown existing cost = %v", err)
	}
}

type reconciliationMissingRepository struct {
	Repository
	noAccountValue domain.AccountID
	noCost         domain.HoldingID
}

func (r reconciliationMissingRepository) ReadPortfolioSnapshot(ctx context.Context, filter domain.AccountFilter) (domain.PortfolioSnapshot, error) {
	snapshot, err := r.Repository.ReadPortfolioSnapshot(ctx, filter)
	if err != nil {
		return snapshot, err
	}
	for i := range snapshot.Accounts {
		if snapshot.Accounts[i].Account.ID == r.noAccountValue {
			snapshot.Accounts[i].LatestValue = nil
		}
	}
	return snapshot, nil
}

func (r reconciliationMissingRepository) ListCostBasisEvents(ctx context.Context, id domain.HoldingID, filter domain.CostBasisReadFilter) ([]domain.CostBasisEvent, error) {
	if id == r.noCost {
		return nil, nil
	}
	return r.Repository.ListCostBasisEvents(ctx, id, filter)
}

func (r reconciliationMissingRepository) StartingPointCost(ctx context.Context, id domain.HoldingID, filter domain.CostBasisReadFilter) (*domain.UnitPrice, error) {
	if id == r.noCost {
		return nil, nil
	}
	return r.Repository.StartingPointCost(ctx, id, filter)
}

func reconciliationMoney(t *testing.T, amount, currency string) domain.Money {
	t.Helper()
	value, err := domain.ParseMoney(amount, domain.CurrencyCode(currency))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func reconciliationQuantity(t *testing.T, amount string) domain.Quantity {
	t.Helper()
	value, err := domain.ParseQuantity(amount)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func reconciliationUnitCost(t *testing.T, amount string) domain.UnitPrice {
	t.Helper()
	value, err := domain.ParseUnitPrice(amount)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
