package application

import (
	"context"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type holdingsSnapshotCounter struct {
	Repository
	reads int
}

func (r *holdingsSnapshotCounter) ReadPortfolioSnapshot(ctx context.Context, filter domain.AccountFilter) (domain.PortfolioSnapshot, error) {
	r.reads++
	return r.Repository.ReadPortfolioSnapshot(ctx, filter)
}

func TestInstrumentHoldingsSnapshotNativeFXAndScope(t *testing.T) {
	db, err := sqlite.Open(t.TempDir() + "/holdings.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := &holdingsSnapshotCounter{Repository: sqlite.NewRepository(db)}
	app := NewService(repo)
	ctx := context.Background()
	if err := app.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "H", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	createAccount := func(name string) domain.AccountID {
		record, err := app.CreateAccount(ctx, AccountInput{Name: name, AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
		if err != nil {
			t.Fatal(err)
		}
		return record.Account.ID
	}
	a, b, archived := createAccount("A"), createAccount("B"), createAccount("Archived")
	createInstrument := func() domain.InstrumentID {
		instrument, err := app.CreateInstrument(ctx, InstrumentInput{Name: "Same name", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		return instrument.ID
	}
	id, other := createInstrument(), createInstrument()
	createHolding := func(account domain.AccountID, instrument domain.InstrumentID, quantity string) domain.HoldingID {
		h, err := app.CreateHolding(ctx, HoldingInput{AccountID: account.String(), InstrumentID: instrument.String(), Quantity: quantity})
		if err != nil {
			t.Fatal(err)
		}
		return h.ID
	}
	createHolding(a, id, "0.1")
	createHolding(b, id, "0.2")
	createHolding(archived, id, "50")
	createHolding(a, other, "0")
	archivedHolding := createHolding(b, other, "20")
	if err := app.ArchiveHolding(ctx, archivedHolding, true); err != nil {
		t.Fatal(err)
	}
	if err := app.ArchiveAccount(ctx, archived, true); err != nil {
		t.Fatal(err)
	}
	if _, err := app.AppendManualInstrumentQuote(ctx, id, "100", "2026-09-19T00:00:00Z", false); err != nil {
		t.Fatal(err)
	}
	// No USD/CNY FX: native values must still be available.
	repo.reads = 0
	groups, err := app.InstrumentHoldings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if repo.reads != 1 {
		t.Fatalf("snapshots = %d", repo.reads)
	}
	if len(groups) != 2 {
		t.Fatalf("different IDs merged: %+v", groups)
	}
	for _, g := range groups {
		if g.InstrumentID == id {
			if len(g.Holdings) != 2 || g.Amounts.Quantity != "0.3" || g.Amounts.CurrentValue == nil || g.Amounts.CurrentValue.Amount != "30" || g.Amounts.UnrealizedGain == nil {
				t.Fatalf("native summary: %+v", g)
			}
			if g.Holdings[0].AccountID != a || g.Holdings[0].Amounts.CurrentValue.Amount != "10" {
				t.Fatalf("member: %+v", g.Holdings)
			}
		} else if len(g.Holdings) != 1 || g.Amounts.Quantity != "0" || g.Amounts.CurrentValue == nil || g.Amounts.CurrentValue.Amount != "0" {
			t.Fatalf("zero/archived filter: %+v", g)
		}
	}
	// Missing price is a field-level absence, not a fabricated zero.
	missing := createInstrument()
	createHolding(a, missing, "1")
	groups, err = app.InstrumentHoldings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range groups {
		if g.InstrumentID == missing && (g.Amounts.TotalCost == nil || g.Amounts.CurrentValue != nil || g.Amounts.ValueMissingReason == "") {
			t.Fatalf("missing field: %+v", g)
		}
	}
	// Archiving identity must not hide an active position.
	if err := app.ArchiveInstrument(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	groups, err = app.InstrumentHoldings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, g := range groups {
		if g.InstrumentID == id {
			found = true
			if !g.Archived {
				t.Fatal("missing archived flag")
			}
		}
	}
	if !found {
		t.Fatal("archived identity dropped active positions")
	}
}

func TestAggregateHoldingAmountsIndependentCompleteness(t *testing.T) {
	usd := domain.CurrencyCode("USD")
	first := domain.InstrumentHoldingMember{Amounts: domain.HoldingAmounts{Quantity: "0.1", TotalCost: &domain.MoneyView{Amount: "20", Currency: usd}, CurrentValue: &domain.MoneyView{Amount: "10", Currency: usd}, UnrealizedGain: &domain.SignedMoneyView{Amount: "-10", Currency: usd}}}
	second := domain.InstrumentHoldingMember{Amounts: domain.HoldingAmounts{Quantity: "0.2", TotalCost: &domain.MoneyView{Amount: "40", Currency: usd}, CurrentValue: &domain.MoneyView{Amount: "20", Currency: usd}, UnrealizedGain: &domain.SignedMoneyView{Amount: "-20", Currency: usd}}}
	result, err := aggregateHoldingAmounts([]domain.InstrumentHoldingMember{first, second}, usd)
	if err != nil {
		t.Fatal(err)
	}
	if result.Quantity != "0.3" || result.TotalCost.Amount != "60" || result.CurrentValue.Amount != "30" || result.UnrealizedGain.Amount != "-30" {
		t.Fatalf("sum: %+v", result)
	}
	second.Amounts.TotalCost = nil
	second.Amounts.CostMissingReason = "unavailable"
	second.Amounts.UnrealizedGain = nil
	second.Amounts.GainMissingReason = "unavailable"
	result, err = aggregateHoldingAmounts([]domain.InstrumentHoldingMember{first, second}, usd)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalCost != nil || result.UnrealizedGain != nil || result.CurrentValue.Amount != "30" || result.CostMissingReason != "unavailable" {
		t.Fatalf("partial totals: %+v", result)
	}
	second.Amounts.CurrentValue = &domain.MoneyView{Amount: "20", Currency: "CNY"}
	if _, err = aggregateHoldingAmounts([]domain.InstrumentHoldingMember{first, second}, usd); err == nil {
		t.Fatal("mixed currencies accepted")
	}
}
