package application

import (
	"context"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestInstrumentHoldingGroupsOrderSubtotalsAndOwnership(t *testing.T) {
	t.Parallel()
	db, err := sqlite.Open(t.TempDir() + "/groups.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := NewService(sqlite.NewRepository(db))
	ctx := context.Background()
	if err := app.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "H", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := app.CreateAccount(ctx, AccountInput{Name: "Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	instrument := func(name, kind, currency string) domain.InstrumentID {
		created, err := app.CreateInstrument(ctx, InstrumentInput{Name: name, Type: kind, QuoteCurrency: currency, QuoteSource: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		return created.ID
	}
	hold := func(id domain.InstrumentID, quantity, cost string) {
		if _, err := app.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: id.String(), Quantity: quantity, UnitCost: cost}); err != nil {
			t.Fatal(err)
		}
	}
	price := func(id domain.InstrumentID, value string) {
		if _, err := app.AppendManualInstrumentQuote(ctx, id, value, "2026-09-19T00:00:00Z", false); err != nil {
			t.Fatal(err)
		}
	}
	// Created out of display order, with two currencies in one group.
	crypto := instrument("Coin", "crypto", "USD")
	gold := instrument("Gold", "precious_metal", "USD")
	etf := instrument("Index ETF", "etf", "USD")
	fund := instrument("Balanced fund", "mutual_fund", "CNY")
	stockUSD := instrument("Apple", "stock", "USD")
	stockCNY := instrument("Moutai", "stock", "CNY")
	closedStock := instrument("Closed stock", "stock", "USD")
	unpriced := instrument("Other holding", "other", "USD")
	contract := instrument("Term deposit", "bank_investment_product", "USD")
	for _, item := range []struct {
		id        domain.InstrumentID
		qty, cost string
		quote     string
	}{{crypto, "2", "10", "20"}, {gold, "1", "100", "130"}, {etf, "3", "10", "11"}, {fund, "10", "1", "1.5"},
		{stockUSD, "2", "50", "60"}, {stockCNY, "1", "1000", "1100"}, {closedStock, "0", "5", "9"}, {unpriced, "1", "7", ""}, {contract, "1", "500", "500"}} {
		hold(item.id, item.qty, item.cost)
		if item.quote != "" {
			price(item.id, item.quote)
		}
	}

	groups, err := app.gain.InstrumentHoldingGroups(ctx, map[string]string{contract.String(): "product-1"})
	if err != nil {
		t.Fatal(err)
	}
	var keys []domain.HoldingGroupKey
	for _, group := range groups {
		keys = append(keys, group.Key)
	}
	want := []domain.HoldingGroupKey{domain.HoldingGroupStock, domain.HoldingGroupFund, domain.HoldingGroupMetal, domain.HoldingGroupCrypto, domain.HoldingGroupCashLike, domain.HoldingGroupOther}
	if len(keys) != len(want) {
		t.Fatalf("groups %v, want %v", keys, want)
	}
	for n := range want {
		if keys[n] != want[n] {
			t.Fatalf("groups %v, want %v", keys, want)
		}
	}
	byKey := map[domain.HoldingGroupKey]domain.InstrumentHoldingGroupView{}
	for _, group := range groups {
		byKey[group.Key] = group
	}
	// Stocks: one subtotal per native currency, never converted or merged, and
	// the closed (zero-quantity) position adds neither value nor an instrument.
	stocks := byKey[domain.HoldingGroupStock]
	if len(stocks.Instruments) != 3 || len(stocks.Subtotals) != 2 {
		t.Fatalf("stocks %+v", stocks)
	}
	cny, usd := stocks.Subtotals[0], stocks.Subtotals[1]
	if cny.Currency != "CNY" || cny.InstrumentCount != 1 || cny.Amounts.TotalCost.Amount != "1000" || cny.Amounts.CurrentValue.Amount != "1100" || cny.Amounts.UnrealizedGain.Amount != "100" {
		t.Fatalf("CNY stock subtotal %s %s %s", cny.Amounts.TotalCost.Amount, cny.Amounts.CurrentValue.Amount, cny.Amounts.UnrealizedGain.Amount)
	}
	if usd.Currency != "USD" || usd.InstrumentCount != 1 || usd.Amounts.TotalCost.Amount != "100" || usd.Amounts.CurrentValue.Amount != "120" || usd.Amounts.UnrealizedGain.Amount != "20" {
		t.Fatalf("USD stock subtotal %s %s %s", usd.Amounts.TotalCost.Amount, usd.Amounts.CurrentValue.Amount, usd.Amounts.UnrealizedGain.Amount)
	}
	// ETFs and mutual funds share one group but keep their currencies apart.
	funds := byKey[domain.HoldingGroupFund]
	if len(funds.Instruments) != 2 || len(funds.Subtotals) != 2 || funds.Subtotals[1].Amounts.CurrentValue.Amount != "33" || funds.Subtotals[1].Amounts.TotalCost.Amount != "30" {
		t.Fatalf("funds %+v", funds)
	}
	// A missing price leaves the value unavailable instead of becoming zero.
	other := byKey[domain.HoldingGroupOther].Subtotals[0]
	if other.Amounts.TotalCost == nil || other.Amounts.CurrentValue != nil || other.Amounts.ValueMissingReason == "" {
		t.Fatalf("unpriced subtotal %+v", other)
	}
	// A managed contract is cash-like even though it is stored as a bank product.
	if cash := byKey[domain.HoldingGroupCashLike]; len(cash.Instruments) != 1 || cash.Instruments[0].InstrumentID != contract {
		t.Fatalf("cash-like %+v", cash)
	}
	if _, ok := byKey[domain.HoldingGroupBankWealth]; ok {
		t.Fatal("an owned contract must not also appear as a bank wealth product")
	}

	// The service-level read joins real product ownership, so no contract exists here.
	service, err := app.InstrumentHoldingGroups(ctx)
	if err != nil || len(service) != 6 || service[len(service)-1].Key != domain.HoldingGroupOther {
		t.Fatalf("service groups %v %v", service, err)
	}
	scoped, err := app.AccountHoldingGroups(ctx, account.Account.ID)
	if err != nil || len(scoped) != len(service) {
		t.Fatalf("account groups %v %v", scoped, err)
	}
}
