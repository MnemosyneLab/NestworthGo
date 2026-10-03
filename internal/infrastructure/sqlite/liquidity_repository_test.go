package sqlite

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestLiquidityReservationSourceRoundTrip(t *testing.T) {
	_, repository, household, account, instrument := seedPortfolioRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	quantity, _ := domain.ParseQuantity("1")
	holding, err := domain.NewHoldingForAccount(account, instrument, quantity, nil, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateHolding(ctx, holding); err != nil {
		t.Fatal(err)
	}
	for _, source := range []domain.LiquiditySourceRef{
		domain.AccountValueSourceRef(account.ID),
		domain.AccountCashSourceRef(account.ID, domain.CurrencyCode("USD")),
		domain.HoldingSourceRef(account.ID, holding.ID),
	} {
		t.Run(string(source.Kind), func(t *testing.T) {
			amount, _ := domain.ParseMoney("200", domain.CurrencyCode("USD"))
			saved := domain.LiquidityReservation{ID: domain.NewLiquidityReservationID(), HouseholdID: household.ID,
				Source: source, Label: "Reserve", Amount: amount, Currency: amount.Currency(), Revision: 1, CreatedAt: now, UpdatedAt: now}
			if err := saved.Validate(); err != nil {
				t.Fatal(err)
			}
			if err := repository.SaveLiquidityReservation(ctx, saved); err != nil {
				t.Fatal(err)
			}
			rows, err := repository.ListLiquidityReservations(ctx, household.ID, true)
			if err != nil {
				t.Fatal(err)
			}
			var loaded *domain.LiquidityReservation
			for i := range rows {
				if rows[i].ID == saved.ID {
					loaded = &rows[i]
				}
			}
			if loaded == nil {
				t.Fatal("reservation missing after save")
			}
			if !reflect.DeepEqual(loaded.Source, saved.Source) {
				t.Errorf("source = %+v, want %+v", loaded.Source, saved.Source)
			}
			if err := loaded.Validate(); err != nil {
				t.Errorf("readback is invalid: %v", err)
			}
			if loaded.Amount.CanonicalAmount() != "200" || loaded.Currency != saved.Currency || loaded.Amount.Currency() != saved.Currency {
				t.Errorf("reservation amount/currency changed: %+v", loaded)
			}
		})
	}
}

func TestBalanceReservationOverviewSurvivesReloadEditAndRelease(t *testing.T) {
	database, repository, household, _, _ := seedPortfolioRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	members, err := repository.ListMembers(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	account, ownership, initial, err := domain.NewAccount(domain.AccountInput{
		HouseholdID: household.ID, Name: "Bank", AccountType: domain.TypeBankAccount,
		BalanceSheetRole: domain.RoleAsset, TrackingMode: domain.TrackingBalance,
		DefaultCurrency: household.BaseCurrency, IncludeInNetWorth: true, InitialAmount: "1000",
		Ownership: []domain.OwnershipShare{{MemberID: members[0].ID, ShareBPS: domain.TotalOwnershipBPS}},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	value, err := domain.NewAccountValue(account, *initial, now, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateAccount(ctx, account, ownership, &value); err != nil {
		t.Fatal(err)
	}
	service := application.NewService(repository)
	service.SetClock(func() time.Time { return now })
	reservation, err := service.SaveLiquidityReservation(ctx, application.SaveReservationInput{
		Source: domain.AccountValueSourceRef(account.ID), Label: "Reserve", Amount: "200",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Reopen SQLite without rewriting or deleting the persisted reservation.
	path := database.Path
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	service = application.NewService(NewRepository(reopened))
	service.SetClock(func() time.Time { return now })
	assertAvailable := func(want string) {
		t.Helper()
		overview, err := service.LiquidityOverview(ctx, application.LiquidityOverviewQuery{})
		if err != nil {
			t.Fatalf("overview: %v", err)
		}
		if len(overview.Buckets) == 0 || overview.Buckets[0].KnownUnreservedSubtotal == nil || overview.Buckets[0].KnownUnreservedSubtotal.CanonicalAmount() != want {
			t.Fatalf("unreserved = %+v, want %s", overview.Buckets, want)
		}
	}
	assertAvailable("800")
	reservation, err = service.SaveLiquidityReservation(ctx, application.SaveReservationInput{
		ID: &reservation.ID, Source: reservation.Source, ExpectedRevision: reservation.Revision, Label: "Updated reserve", Amount: "250",
	})
	if err != nil {
		t.Fatal(err)
	}
	assertAvailable("750")
	if _, err := service.ReleaseLiquidityReservation(ctx, reservation.ID, reservation.Revision); err != nil {
		t.Fatal(err)
	}
	assertAvailable("1000")
}
