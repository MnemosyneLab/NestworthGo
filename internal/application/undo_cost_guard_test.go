package application

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type dependentCostTimeline struct {
	Repository
	activities []domain.Activity
}

func (r dependentCostTimeline) ListActivitiesUntil(context.Context, domain.HouseholdID, time.Time) ([]domain.Activity, error) {
	return r.activities, nil
}

func TestUndoRejectsLaterCostCorrectionForSameHolding(t *testing.T) {
	t.Parallel()
	holdingID := domain.NewHoldingID()
	original := domain.Activity{ID: domain.NewActivityID()}
	correction := domain.Activity{ID: domain.NewActivityID(), Effects: []domain.ActivityEffect{{Target: domain.EffectTargetHoldingCost, HoldingID: &holdingID}}}
	service := NewService(dependentCostTimeline{activities: []domain.Activity{original, correction}})
	quantityEffect := domain.ActivityEffect{Target: domain.EffectTargetHoldingQuantity, HoldingID: &holdingID}
	if err := service.rejectDependentHoldingUndo(context.Background(), original, []domain.ActivityEffect{quantityEffect}, time.Now()); !hasDomainCode(err, domain.ErrInvalidChange) {
		t.Fatalf("undo with later cost correction = %v", err)
	}
}

func TestUndoRejectsLaterHoldingTradesThatWouldLoseTheirCostBasis(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "undo-dependent-cost.db"))
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
	broker, err := service.CreateAccount(ctx, AccountInput{Name: "Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Fund", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, broker.Account.ID, "1000", "USD", "2026-01-02"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	first, err := service.RecordChange(ctx, domain.TradeInput{HouseholdID: bootstrap.Household.ID, Side: domain.TradeBuy, SettlementAccountID: broker.Account.ID, InstrumentID: instrument.ID, Quantity: reconciliationQuantity(t, "10"), Gross: reconciliationMoney(t, "100", "USD"), EffectiveAt: now})
	if err != nil {
		t.Fatal(err)
	}
	holdings, err := service.ListHoldings(ctx, broker.Account.ID, false)
	if err != nil || len(holdings) != 1 {
		t.Fatalf("holdings = %+v, %v", holdings, err)
	}
	holdingID := holdings[0].ID
	now = now.Add(time.Hour)
	if _, err := service.RecordChange(ctx, domain.TradeInput{HouseholdID: bootstrap.Household.ID, Side: domain.TradeSell, SettlementAccountID: broker.Account.ID, InstrumentID: instrument.ID, HoldingID: holdingID, Quantity: reconciliationQuantity(t, "10"), Gross: reconciliationMoney(t, "100", "USD"), EffectiveAt: now}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	if _, err := service.RecordChange(ctx, domain.TradeInput{HouseholdID: bootstrap.Household.ID, Side: domain.TradeBuy, SettlementAccountID: broker.Account.ID, InstrumentID: instrument.ID, HoldingID: holdingID, Quantity: reconciliationQuantity(t, "10"), Gross: reconciliationMoney(t, "200", "USD"), EffectiveAt: now}); err != nil {
		t.Fatal(err)
	}
	before, err := service.changeState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	beforeActivities, err := service.ListActivities(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.PreviewUndoChangeGuarded(ctx, first.Activity.ID); !hasDomainCode(err, domain.ErrInvalidChange) {
		t.Fatalf("dependent undo preview = %v", err)
	}
	if _, err := service.UndoChange(ctx, first.Activity.ID); !hasDomainCode(err, domain.ErrInvalidChange) {
		t.Fatalf("dependent undo commit = %v", err)
	}
	after, err := service.changeState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	afterActivities, err := service.ListActivities(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterActivities) != len(beforeActivities) || !after.Holdings[holdingID].Current.Decimal().Equal(before.Holdings[holdingID].Current.Decimal()) || !after.Cash[broker.Account.ID]["USD"].Amount().Equal(before.Cash[broker.Account.ID]["USD"].Amount()) {
		t.Fatalf("rejected undo changed state: before=%+v after=%+v", before, after)
	}
}
