package application

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"testing"
	"time"
)

// NW-005: partitioning a position across accounts must not change its total.
func TestInstrumentAggregationRoundsOnlyAfterSummation(t *testing.T) {
	db, err := sqlite.Open(t.TempDir() + "/synthetic.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := NewService(sqlite.NewRepository(db))
	ctx := context.Background()
	if err = app.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Synthetic review", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	b, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	i, err := app.CreateInstrument(ctx, InstrumentInput{Name: "Exact fixture", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"A", "B"} {
		a, err := app.CreateAccount(ctx, AccountInput{Name: name, AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{b.Members[0].ID}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = app.CreateHolding(ctx, HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "1"})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = app.AppendManualInstrumentQuote(ctx, i.ID, "100.00246", "2026-01-01T00:00:00Z", false); err != nil {
		t.Fatal(err)
	}
	groups, err := app.InstrumentHoldings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	portfolio, err := app.Portfolio(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("two holdings quantity 1 each x price 100.00246: group=%s; portfolio=%s", groups[0].Amounts.CurrentValue.Amount, portfolio.ValuedSubtotal.Amount)
	if portfolio.ValuedSubtotal.Amount != "200.0049" {
		t.Fatalf("portfolio=%+v", portfolio.ValuedSubtotal)
	}
	if groups[0].Amounts.CurrentValue.Amount != portfolio.ValuedSubtotal.Amount {
		t.Fatalf("aggregate differs from exact portfolio: %+v", groups[0].Amounts)
	}
}

func TestGainAggregatesKeepExactCostValueAndUnrealized(t *testing.T) {
	db, err := sqlite.Open(t.TempDir() + "/precision.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := NewService(sqlite.NewRepository(db))
	ctx := context.Background()
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	app.setClock(func() time.Time { return now })
	if err := app.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Precision", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	b, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var instruments []domain.Instrument
	for _, name := range []string{"X", "Y"} {
		i, e := app.CreateInstrument(ctx, InstrumentInput{Name: name, Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
		if e != nil {
			t.Fatal(e)
		}
		instruments = append(instruments, i)
		if _, e = app.AppendManualInstrumentQuote(ctx, i.ID, "100.00246", now.Format(time.RFC3339), false); e != nil {
			t.Fatal(e)
		}
	}
	for _, name := range []string{"A", "B"} {
		a, e := app.CreateAccount(ctx, AccountInput{Name: name, AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{b.Members[0].ID}})
		if e != nil {
			t.Fatal(e)
		}
		for _, i := range instruments {
			if _, e = app.CreateHolding(ctx, HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "1"}); e != nil {
				t.Fatal(e)
			}
		}
	}
	if _, err = app.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	for _, i := range instruments {
		if _, err = app.AppendManualInstrumentQuote(ctx, i.ID, "100.00749", now.Format(time.RFC3339), false); err != nil {
			t.Fatal(err)
		}
	}
	groups, err := app.InstrumentHoldings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range groups {
		if g.Amounts.TotalCost.Amount != "200.0049" || g.Amounts.CurrentValue.Amount != "200.015" || g.Amounts.UnrealizedGain.Amount != "0.0101" {
			t.Fatalf("instrument exact sums: %+v", g.Amounts)
		}
	}
	accounts, err := app.AccountGains(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range accounts {
		if a.TotalCost.Amount != "200.0049" || a.CurrentValue.Amount != "200.015" || a.UnrealizedGain.Amount != "0.0101" {
			t.Fatalf("account exact sums: cost=%+v value=%+v gain=%+v", a.TotalCost, a.CurrentValue, a.UnrealizedGain)
		}
	}
}
