package application

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestAccountDisplayArchivedInstrumentRefreshTargets(t *testing.T) {
	archived := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	provider, symbol := "fake", "RETAINED"
	for _, test := range []struct {
		name            string
		quantity        string
		archivedAccount bool
		archivedHolding bool
		wantFX          []string
	}{
		{name: "retained nonzero holding", quantity: "1", wantFX: []string{"fx:CNY/SGD", "fx:CNY/USD"}},
		{name: "zero holding", quantity: "0", wantFX: []string{}},
		{name: "archived account", quantity: "1", archivedAccount: true, wantFX: []string{}},
		{name: "archived holding", quantity: "1", archivedHolding: true, wantFX: []string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			quantity, err := domain.ParseQuantity(test.quantity)
			if err != nil {
				t.Fatal(err)
			}
			account := domain.Account{ID: "account", DefaultCurrency: "USD", TrackingMode: domain.TrackingHoldings}
			holding := domain.Holding{ID: "holding", AccountID: account.ID, InstrumentID: "instrument", Quantity: quantity}
			if test.archivedAccount {
				account.ArchivedAt = &archived
			}
			if test.archivedHolding {
				holding.ArchivedAt = &archived
			}
			snapshot := domain.PortfolioSnapshot{
				Household:   &domain.Household{ID: "household", BaseCurrency: "CNY"},
				Accounts:    []domain.AccountRecord{{Account: account}},
				Holdings:    []domain.Holding{holding},
				Instruments: []domain.Instrument{{ID: "instrument", QuoteCurrency: "SGD", QuoteSource: domain.QuoteSourceProvider, ProviderKey: &provider, ProviderSymbol: &symbol, ArchivedAt: &archived}},
			}
			keys := []string{}
			instrumentSkipped := false
			for _, target := range refreshTargetsForSnapshot(snapshot) {
				if target.kind == RefreshFXTarget {
					keys = append(keys, target.key)
					if target.skip {
						t.Fatalf("retained holding FX unexpectedly skipped: %+v", target)
					}
				} else if target.instrument.ID == "instrument" {
					instrumentSkipped = target.skip && target.skipCode == domain.ErrUnavailable
				}
			}
			if !reflect.DeepEqual(keys, test.wantFX) || !instrumentSkipped {
				t.Fatalf("FX=%v want=%v archived price skipped=%v", keys, test.wantFX, instrumentSkipped)
			}
		})
	}
}

func TestAccountDisplayArchivedInstrumentRefreshToValuation(t *testing.T) {
	for _, entry := range []string{"all", "missing_or_stale", "required_fx"} {
		for _, existingNativeFX := range []bool{false, true} {
			name := entry + "/both_legs_missing"
			if existingNativeFX {
				name = entry + "/native_leg_present"
			}
			t.Run(name, func(t *testing.T) {
				ctx := context.Background()
				database, err := sqlite.Open(t.TempDir() + "/archived-display-refresh.db")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = database.Close() })
				fake := newRefreshFakeProvider()
				service := NewService(sqlite.NewRepository(database), NewMarketDataRegistry(fake))
				now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
				service.setClock(func() time.Time { return now })
				if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Retained holding", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
					t.Fatal(err)
				}
				bootstrap, err := service.Bootstrap(ctx)
				if err != nil {
					t.Fatal(err)
				}
				account, err := service.CreateAccount(ctx, AccountInput{Name: "USD account", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}}})
				if err != nil {
					t.Fatal(err)
				}
				instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Retained SGD asset", Type: "stock", QuoteCurrency: "SGD", QuoteSource: "manual"})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "3"}); err != nil {
					t.Fatal(err)
				}
				if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "10", "2026-08-23", false); err != nil {
					t.Fatal(err)
				}
				if err := seedLegacyInstrumentArchive(t, service, instrument.ID); err != nil {
					t.Fatal(err)
				}
				// No cash components or other accounts may mask the retained
				// holding's otherwise absent USD/CNY refresh dependency.
				if existingNativeFX {
					if _, err := service.AppendManualFXQuote(ctx, "SGD", "CNY", "5", "2026-08-23"); err != nil {
						t.Fatal(err)
					}
					if _, err := service.SetFXPreference(ctx, "SGD", "CNY", "manual"); err != nil {
						t.Fatal(err)
					}
				}
				for _, item := range []struct{ from, rate string }{{"SGD", "5"}, {"USD", "7.5"}} {
					rate, err := domain.ParseFxRate(item.rate)
					if err != nil {
						t.Fatal(err)
					}
					fake.fx[item.from+"/CNY"] = struct {
						quote LatestFXQuote
						err   error
					}{quote: LatestFXQuote{Rate: rate, BaseCurrency: domain.CurrencyCode(item.from), QuoteCurrency: "CNY", SourceKey: fake.Key(), QuotedAt: now}}
				}
				before, err := service.AccountValuation(ctx, account.Account.ID)
				if err != nil || before.DisplayValue == nil || before.DisplayValue.Value != nil || fake.callCount() != 0 {
					t.Fatalf("before refresh: %+v %v calls=%v", before.DisplayValue, err, fake.callNames())
				}
				if existingNativeFX && (!before.Complete || before.BaseValue == nil || before.BaseValue.Amount != "150") {
					t.Fatalf("retained household value unavailable: %+v", before)
				}
				var result RefreshResult
				switch entry {
				case "all":
					result, err = service.RefreshAll(ctx)
				case "missing_or_stale":
					result, err = service.RefreshMissingOrStale(ctx)
				case "required_fx":
					result, err = service.RefreshRequiredFX(ctx)
				}
				if err != nil {
					t.Fatal(err)
				}
				assertRefreshStatus(t, result, "fx:CNY/USD", RefreshFetched)
				wantCalls := []string{"fx:SGD/CNY", "fx:USD/CNY"}
				if existingNativeFX {
					assertRefreshStatus(t, result, "fx:CNY/SGD", RefreshSkipped)
					wantCalls = []string{"fx:USD/CNY"}
				} else {
					assertRefreshStatus(t, result, "fx:CNY/SGD", RefreshFetched)
				}
				if entry != "required_fx" {
					assertRefreshStatus(t, result, instrumentTargetKey(instrument.ID), RefreshSkipped)
				}
				if !reflect.DeepEqual(fake.callNames(), wantCalls) {
					t.Fatalf("provider calls=%v want=%v; archived price must not refresh", fake.callNames(), wantCalls)
				}
				after, err := service.AccountValuation(ctx, account.Account.ID)
				if err != nil || after.DisplayValue == nil || !after.DisplayValue.Complete || after.DisplayValue.Value == nil || after.DisplayValue.Value.Amount != "20" || after.DisplayValue.Currency != "USD" || after.BaseValue == nil || after.BaseValue.Amount != "150" {
					t.Fatalf("retained holding display after refresh: %+v %v", after, err)
				}
				if fake.callCount() != len(wantCalls) {
					t.Fatal("valuation read contacted a provider")
				}
			})
		}
	}
}
