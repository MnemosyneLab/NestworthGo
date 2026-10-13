package application

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestAccountDisplayRefreshTargets(t *testing.T) {
	amount, err := domain.ParseMoney("10", "SGD")
	if err != nil {
		t.Fatal(err)
	}
	quantity, err := domain.ParseQuantity("1")
	if err != nil {
		t.Fatal(err)
	}
	archived := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name       string
		mode       domain.TrackingMode
		source     string
		preference *domain.FXPreference
		archived   bool
		want       []string
		skipped    string
	}{
		{name: "holding bridge", mode: domain.TrackingHoldings, source: "holding", want: []string{"fx:CNY/SGD", "fx:CNY/USD"}},
		{name: "cash bridge", mode: domain.TrackingHoldings, source: "cash", want: []string{"fx:CNY/SGD", "fx:CNY/USD"}},
		{name: "balance different recorded currency", mode: domain.TrackingBalance, source: "value", want: []string{"fx:CNY/SGD", "fx:CNY/USD"}},
		{name: "manual different recorded currency", mode: domain.TrackingManualValue, source: "value", want: []string{"fx:CNY/SGD", "fx:CNY/USD"}},
		{name: "duplicate holding and cash", mode: domain.TrackingHoldings, source: "both", want: []string{"fx:CNY/SGD", "fx:CNY/USD"}},
		{name: "empty account", mode: domain.TrackingHoldings, want: []string{}},
		{name: "zero cash needs no display FX", mode: domain.TrackingHoldings, source: "zero", want: []string{"fx:CNY/SGD"}},
		{name: "same native and default", mode: domain.TrackingHoldings, source: "same", want: []string{"fx:CNY/SGD"}},
		{name: "archived account", mode: domain.TrackingHoldings, source: "both", archived: true, want: []string{}},
		{name: "explicit provider pair is required", mode: domain.TrackingHoldings, source: "both", preference: &domain.FXPreference{HouseholdID: "household", CurrencyA: "SGD", CurrencyB: "USD", SourceKind: domain.QuoteSourceProvider}, want: []string{"fx:CNY/SGD", "fx:SGD/USD"}},
		{name: "explicit manual pair stays skipped", mode: domain.TrackingHoldings, source: "both", preference: &domain.FXPreference{HouseholdID: "household", CurrencyA: "SGD", CurrencyB: "USD", SourceKind: domain.QuoteSourceManual}, want: []string{"fx:CNY/SGD", "fx:SGD/USD"}, skipped: "fx:SGD/USD"},
		{name: "manual bridge leg stays skipped", mode: domain.TrackingHoldings, source: "both", preference: &domain.FXPreference{HouseholdID: "household", CurrencyA: "CNY", CurrencyB: "USD", SourceKind: domain.QuoteSourceManual}, want: []string{"fx:CNY/SGD", "fx:CNY/USD"}, skipped: "fx:CNY/USD"},
	} {
		t.Run(test.name, func(t *testing.T) {
			account := domain.Account{ID: "account", DefaultCurrency: "USD", TrackingMode: test.mode}
			if test.archived {
				account.ArchivedAt = &archived
			}
			if test.source == "same" {
				account.DefaultCurrency = "SGD"
			}
			record := domain.AccountRecord{Account: account}
			snapshot := domain.PortfolioSnapshot{Household: &domain.Household{ID: "household", BaseCurrency: "CNY"}}
			if test.source == "value" {
				record.LatestValue = &domain.AccountValue{Amount: amount}
			}
			snapshot.Accounts = []domain.AccountRecord{record}
			if test.source == "holding" || test.source == "both" {
				snapshot.Instruments = []domain.Instrument{{ID: "instrument", QuoteCurrency: "SGD", QuoteSource: domain.QuoteSourceManual}}
				snapshot.Holdings = []domain.Holding{{AccountID: account.ID, InstrumentID: "instrument", Quantity: quantity}}
			}
			if test.source == "cash" || test.source == "both" || test.source == "same" || test.source == "zero" {
				cash := amount
				if test.source == "zero" {
					cash, err = domain.ParseMoney("0", "SGD")
					if err != nil {
						t.Fatal(err)
					}
				}
				snapshot.CashValues = []domain.AccountCashValue{{AccountID: account.ID, Amount: cash}}
			}
			if test.preference != nil {
				snapshot.FXPreferences = []domain.FXPreference{*test.preference}
			}
			keys := []string{}
			for _, target := range refreshTargetsForSnapshot(snapshot) {
				if target.kind != RefreshFXTarget {
					continue
				}
				keys = append(keys, target.key)
				if target.skip != (target.key == test.skipped) {
					t.Fatalf("source preference changed: %+v", target)
				}
			}
			if !reflect.DeepEqual(keys, test.want) {
				t.Fatalf("required FX = %v, want %v", keys, test.want)
			}
			if extras := extraFXRefreshTargetsForSnapshot(snapshot); len(extras) != 0 {
				t.Fatalf("display dependency left in optional group: %+v", extras)
			}
		})
	}
}

func TestAccountDisplayRefreshToValuation(t *testing.T) {
	for _, entry := range []string{"all", "missing_or_stale", "required_fx"} {
		for _, policy := range []string{"bridge", "provider_pair", "manual_pair"} {
			t.Run(entry+"/"+policy, func(t *testing.T) {
				ctx := context.Background()
				database, err := sqlite.Open(t.TempDir() + "/display-refresh.db")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = database.Close() })
				fake := newRefreshFakeProvider()
				service := NewService(sqlite.NewRepository(database), NewMarketDataRegistry(fake))
				now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
				service.setClock(func() time.Time { return now })
				if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Display refresh", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
					t.Fatal(err)
				}
				bootstrap, err := service.Bootstrap(ctx)
				if err != nil {
					t.Fatal(err)
				}
				account, err := service.CreateAccount(ctx, AccountInput{Name: "USD account with SGD assets", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}}})
				if err != nil {
					t.Fatal(err)
				}
				instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "SGD asset", Type: "stock", QuoteCurrency: "SGD", QuoteSource: "manual"})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "2"}); err != nil {
					t.Fatal(err)
				}
				if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "10", "2026-08-23", false); err != nil {
					t.Fatal(err)
				}
				if _, err := service.AppendAccountCashValue(ctx, account.Account.ID, "10", "SGD", "2026-08-23"); err != nil {
					t.Fatal(err)
				}
				preferences, err := service.repository.ListFXPreferences(ctx, bootstrap.Household.ID)
				if err != nil || len(preferences) != 0 {
					t.Fatalf("fixture unexpectedly seeded FX preferences: %+v %v", preferences, err)
				}
				if policy != "bridge" {
					source := "provider"
					if policy == "manual_pair" {
						source = "manual"
					}
					if _, err := service.SetFXPreference(ctx, "SGD", "USD", source); err != nil {
						t.Fatal(err)
					}
				}
				for _, item := range []struct{ from, to, rate string }{{"SGD", "CNY", "5"}, {"USD", "CNY", "7.5"}, {"SGD", "USD", "0.8"}} {
					rate, err := domain.ParseFxRate(item.rate)
					if err != nil {
						t.Fatal(err)
					}
					fake.fx[item.from+"/"+item.to] = struct {
						quote LatestFXQuote
						err   error
					}{quote: LatestFXQuote{Rate: rate, BaseCurrency: domain.CurrencyCode(item.from), QuoteCurrency: domain.CurrencyCode(item.to), SourceKey: fake.Key(), QuotedAt: now}}
				}
				before, err := service.AccountValuation(ctx, account.Account.ID)
				if err != nil || before.DisplayValue == nil || before.DisplayValue.Value != nil || fake.callCount() != 0 {
					t.Fatalf("before refresh: %+v %v calls=%v", before.DisplayValue, err, fake.callNames())
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
				assertRefreshStatus(t, result, "fx:CNY/SGD", RefreshFetched)
				wantCalls := []string{"fx:SGD/CNY"}
				wantAmount := ""
				if policy == "bridge" {
					assertRefreshStatus(t, result, "fx:CNY/USD", RefreshFetched)
					wantCalls = append(wantCalls, "fx:USD/CNY")
					wantAmount = "20"
				} else if policy == "provider_pair" {
					assertRefreshStatus(t, result, "fx:SGD/USD", RefreshFetched)
					wantCalls = append(wantCalls, "fx:SGD/USD")
					wantAmount = "24"
				} else {
					assertRefreshStatus(t, result, "fx:SGD/USD", RefreshSkipped)
				}
				calls := fake.callNames()
				sort.Strings(calls)
				sort.Strings(wantCalls)
				if !reflect.DeepEqual(calls, wantCalls) {
					t.Fatalf("provider calls = %v, want %v", calls, wantCalls)
				}
				after, err := service.AccountValuation(ctx, account.Account.ID)
				if err != nil || !after.Complete || after.BaseValue == nil || after.BaseValue.Amount != "150" {
					t.Fatalf("household valuation: %+v %v", after, err)
				}
				if wantAmount == "" {
					if after.DisplayValue.Complete || after.DisplayValue.Value != nil {
						t.Fatalf("manual preference bypassed: %+v", after.DisplayValue)
					}
				} else if !after.DisplayValue.Complete || after.DisplayValue.Value == nil || after.DisplayValue.Value.Amount != wantAmount || after.DisplayValue.Currency != "USD" {
					t.Fatalf("display after refresh: %+v, want %s USD", after.DisplayValue, wantAmount)
				}
				if fake.callCount() != len(wantCalls) {
					t.Fatal("valuation read performed provider I/O")
				}
				if entry == "missing_or_stale" {
					if _, err := service.RefreshMissingOrStale(ctx); err != nil {
						t.Fatal(err)
					}
					if fake.callCount() != len(wantCalls) {
						t.Fatal("new display dependencies bypassed the existing quote cache")
					}
				}
			})
		}
	}
}
