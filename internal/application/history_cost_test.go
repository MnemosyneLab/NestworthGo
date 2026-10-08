package application

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestStartingPointCostOverridePersistsAndZeroHoldingNeedsNoCost(t *testing.T) {
	t.Parallel()
	database, err := sqlite.Open(t.TempDir() + "/starting-point-cost.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Capture", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	priced, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Priced ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	unpriced, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Empty ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	positive, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: priced.ID.String(), Quantity: "2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: unpriced.ID.String(), Quantity: "0"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, priced.ID, "100", "2026-08-24", false); err != nil {
		t.Fatal(err)
	}
	draft, err := service.StartingPointDraft(ctx)
	if err != nil || len(draft) != 1 || draft[0].HoldingID != positive.ID || draft[0].UnitCost != "100" {
		t.Fatalf("Starting Point draft = %+v, err=%v", draft, err)
	}
	if _, err := service.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{positive.ID: "123.45"}); err != nil {
		t.Fatal(err)
	}
	origin, err := service.HistoryOrigin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	components, err := service.repository.ListHistoryOriginComponents(ctx, origin.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, component := range components {
		if component.HoldingID != nil && *component.HoldingID == positive.ID {
			if component.UnitCost == nil || component.UnitCost.Canonical() != "123.45" {
				t.Fatalf("Starting Point override = %v, want 123.45", component.UnitCost)
			}
			return
		}
	}
	t.Fatal("positive holding Starting Point component was not found")
}

func TestCreateHoldingWithAdvancingClockPersistsOneAdjustment(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		unitCost string
		wantCost string
	}{
		{name: "cost override", unitCost: "77.25", wantCost: "77.25"},
		{name: "quote fallback", wantCost: "100"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database, err := sqlite.Open(t.TempDir() + "/advancing-clock.db")
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			service := NewService(sqlite.NewRepository(database))
			now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
			service.setClock(func() time.Time { return now })
			ctx := context.Background()
			if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Capture", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
				t.Fatal(err)
			}
			bootstrap, err := service.Bootstrap(ctx)
			if err != nil {
				t.Fatal(err)
			}
			account, err := service.CreateAccount(ctx, AccountInput{Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
			if err != nil {
				t.Fatal(err)
			}
			instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "100", "2026-08-24", false); err != nil {
				t.Fatal(err)
			}
			if _, err := service.StartHistory(ctx, "UTC"); err != nil {
				t.Fatal(err)
			}
			// Cross a millisecond boundary on every clock read during creation.
			// Two separate reads for Now and EffectiveAt must fail before the fix.
			var clockReads atomic.Int64
			service.setClock(func() time.Time {
				return now.Add(time.Duration(clockReads.Add(1)) * time.Millisecond)
			})
			holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "3", UnitCost: tc.unitCost})
			if err != nil {
				t.Fatal(err)
			}
			// A genuinely future change still fails without appending any facts.
			unitCost := mustUnitPrice(t, tc.wantCost)
			_, err = service.RecordChange(ctx, domain.PositionAdjustmentInput{HouseholdID: bootstrap.Household.ID, HoldingID: holding.ID, Quantity: mustQuantity(t, "1"), Added: true, UnitCost: &unitCost, EffectiveAt: service.clock().Add(time.Hour)})
			var changeErr *domain.Error
			if !errors.As(err, &changeErr) || changeErr.Code != domain.ErrInvalidChangeTime || changeErr.Field != "effectiveAt" {
				t.Fatalf("future change error = %v, want ErrInvalidChangeTime on effectiveAt", err)
			}
			holdings, err := service.ListHoldings(ctx, account.Account.ID, false)
			if err != nil || len(holdings) != 1 || holdings[0].ID != holding.ID || holdings[0].Quantity.Canonical() != "3" {
				t.Fatalf("persisted holdings = %+v, err=%v", holdings, err)
			}
			activities, err := service.ListActivities(ctx, 10)
			if err != nil || len(activities) != 1 {
				t.Fatalf("persisted activities = %+v, err=%v", activities, err)
			}
			activity := activities[0]
			if activity.Kind != domain.ActivityPositionTransfer || activity.Reason != domain.ReasonReconciliation || len(activity.Effects) != 1 || activity.Effects[0].HoldingID == nil || *activity.Effects[0].HoldingID != holding.ID || activity.Effects[0].Quantity == nil || activity.Effects[0].Quantity.Canonical() != "3" {
				t.Fatalf("persisted adjustment = %+v", activity)
			}
			if !activity.EffectiveAt.Equal(activity.CreatedAt) {
				t.Fatalf("activity effective time %s differs from creation time %s", activity.EffectiveAt, activity.CreatedAt)
			}
			events, err := service.repository.ListCostBasisEvents(ctx, holding.ID, domain.CostBasisReadFilter{})
			if err != nil || len(events) != 1 {
				t.Fatalf("persisted cost events = %+v, err=%v", events, err)
			}
			event := events[0]
			if event.Kind != domain.CostBasisAdjustmentIn || event.ActivityID != activity.ID || event.Quantity.Canonical() != "3" || event.UnitCost == nil || event.UnitCost.Canonical() != tc.wantCost || !event.EffectiveAt.Equal(activity.EffectiveAt) {
				t.Fatalf("cost event = %+v, want one linked adjustment of 3 units at %s and %s", event, tc.wantCost, activity.EffectiveAt)
			}
		})
	}
}

func TestCreateHoldingCostOverridePersistsAlreadyExistedAdjustment(t *testing.T) {
	t.Parallel()
	database, err := sqlite.Open(t.TempDir() + "/already-existed-cost.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Capture", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Initial ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: initial.ID.String(), Quantity: "0"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Already Existed ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, created.ID, "100", "2026-08-24", false); err != nil {
		t.Fatal(err)
	}
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: created.ID.String(), Quantity: "3", UnitCost: "77.25"})
	if err != nil {
		t.Fatal(err)
	}
	events, err := service.repository.ListCostBasisEvents(ctx, holding.ID, domain.CostBasisReadFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Kind != domain.CostBasisAdjustmentIn || events[0].UnitCost == nil || events[0].UnitCost.Canonical() != "77.25" {
		t.Fatalf("already existed cost event = %+v", events)
	}
}
