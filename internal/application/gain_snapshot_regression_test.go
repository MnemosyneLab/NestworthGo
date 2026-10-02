package application

import (
	"context"
	"fmt"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"testing"
	"time"
)

type gainInterleavedSnapshot struct {
	Repository
	afterRead func()
}

func (r *gainInterleavedSnapshot) ReadGainSnapshot(ctx context.Context) (domain.GainSnapshot, error) {
	snapshot, err := r.Repository.ReadGainSnapshot(ctx)
	if err == nil && r.afterRead != nil {
		fn := r.afterRead
		r.afterRead = nil
		fn()
	}
	return snapshot, err
}
func TestGainReadUsesOneSnapshotAcrossConcurrentSale(t *testing.T) {
	db, err := sqlite.Open(t.TempDir() + "/synthetic.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := sqlite.NewRepository(db)
	app := NewService(repo)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db.SQL.SetMaxOpenConns(1)
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	app.setClock(func() time.Time { return now })
	if err = app.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Synthetic", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	b, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	a, err := app.CreateAccount(ctx, AccountInput{Name: "Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{b.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	i, err := app.CreateInstrument(ctx, InstrumentInput{Name: "Fixture", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	h, err := app.CreateHolding(ctx, HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "10"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = app.AppendManualInstrumentQuote(ctx, i.ID, "80", "2026-08-24T12:00:00Z", false); err != nil {
		t.Fatal(err)
	}
	if _, err = app.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	wrapped := &gainInterleavedSnapshot{Repository: repo, afterRead: func() {
		// The reader is still active, but its short SQLite transaction must already
		// be released. A real writer on the sole connection completes before replay.
		done := make(chan error, 1)
		go func() {
			_, e := app.RecordChange(ctx, domain.TradeInput{HouseholdID: b.Household.ID, Side: domain.TradeSell, SettlementAccountID: a.Account.ID, HoldingID: h.ID, InstrumentID: i.ID, Quantity: mustQuantity(t, "10"), Gross: mustMoney(t, "1000", "USD"), EffectiveAt: now})
			done <- e
		}()
		select {
		case e := <-done:
			if e != nil {
				t.Fatal(e)
			}
		case <-ctx.Done():
			t.Fatal("gain read retained the only database connection")
		}
	}}
	view, err := NewGainService(wrapped, func() time.Time { return now }).HoldingGain(ctx, h.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("interleaved result quantity=%s cost=%s current=%s unrealized=%s realized=%s", view.Quantity, view.TotalCost.Amount, view.CurrentValue.Amount, view.UnrealizedGain.Amount, view.RealizedGain.Amount)
	if view.Quantity != "10" || view.TotalCost.Amount != "800" || view.RealizedGain.Amount != "0" {
		t.Fatalf("gain read mixed pre-sale and post-sale data: %+v", view)
	}
	settled, err := NewGainService(repo, func() time.Time { return now }).HoldingGain(ctx, h.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("after sale stable quantity=%s cost=%s current=%s unrealized=%s realized=%s", settled.Quantity, settled.TotalCost.Amount, settled.CurrentValue.Amount, settled.UnrealizedGain.Amount, settled.RealizedGain.Amount)
	if settled.Quantity != "0" || settled.TotalCost.Amount != "0" || settled.RealizedGain.Amount != "200" {
		t.Fatalf("sale not visible in next snapshot: %+v", settled)
	}
}

type gainSnapshotOnlyRepository struct {
	Repository // nil: any unintended live read fails this test immediately.
	inputs     domain.GainSnapshot
	reads      int
}

func (r *gainSnapshotOnlyRepository) ReadGainSnapshot(context.Context) (domain.GainSnapshot, error) {
	r.reads++
	return r.inputs, nil
}

func TestEveryGainEntryUsesOnlyOneMaterializedSnapshot(t *testing.T) {
	f := newGoldenValuationFixture(t, true)
	ctx := context.Background()
	if _, err := f.service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	inputs, err := f.repository.ReadGainSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	holdingID := findHoldingForInstrument(t, f.repository, f.account.Account.ID, f.qqq.ID)
	repo := &gainSnapshotOnlyRepository{inputs: inputs}
	gain := NewGainService(repo)
	calls := map[string]func() error{
		"HoldingGain":        func() error { _, e := gain.HoldingGain(ctx, holdingID); return e },
		"AccountGain":        func() error { _, e := gain.AccountGain(ctx, f.account.Account.ID); return e },
		"AccountGains":       func() error { _, e := gain.AccountGains(ctx, nil); return e },
		"InstrumentHoldings": func() error { _, e := gain.InstrumentHoldings(ctx); return e },
		"RealizedGain":       func() error { _, e := gain.RealizedGain(ctx, domain.GainScope{}, domain.TrendAllTime); return e },
		"RealizedGainInRange": func() error {
			_, e := gain.RealizedGainInRange(ctx, domain.GainScope{}, "2026-01-01", "2026-12-31")
			return e
		},
		"DividendIncome": func() error { _, e := gain.DividendIncome(ctx, domain.GainScope{}, domain.TrendAllTime); return e },
		"DividendIncomeInRange": func() error {
			_, e := gain.DividendIncomeInRange(ctx, domain.GainScope{}, "2026-01-01", "2026-12-31")
			return e
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			repo.reads = 0
			if e := call(); e != nil {
				t.Fatal(e)
			}
			if repo.reads != 1 {
				t.Fatalf("snapshots=%d", repo.reads)
			}
		})
	}
}

func TestGainSnapshotRepresentative500Positions(t *testing.T) {
	app, ctx, b, _ := newOnboardedService(t, "gain-snapshot-scale", []string{"Owner"})
	a, err := app.CreateAccount(ctx, AccountInput{Name: "Scale", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{b.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	var first domain.HoldingID
	now := app.clock().UTC().Format(time.RFC3339)
	for n := 0; n < 500; n++ {
		i, e := app.CreateInstrument(ctx, InstrumentInput{Name: fmt.Sprintf("Synthetic %d", n), Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
		if e != nil {
			t.Fatal(e)
		}
		h, e := app.CreateHolding(ctx, HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "1"})
		if e != nil {
			t.Fatal(e)
		}
		if n == 0 {
			first = h.ID
		}
		if _, e = app.AppendManualInstrumentQuote(ctx, i.ID, "80.12345678", now, false); e != nil {
			t.Fatal(e)
		}
	}
	if _, err = app.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	start := time.Now()
	holding, e := app.HoldingGain(ctx, first)
	if e != nil {
		t.Fatal(e)
	}
	single := time.Since(start)
	start = time.Now()
	accounts, e := app.AccountGains(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	batch := time.Since(start)
	start = time.Now()
	groups, e := app.InstrumentHoldings(ctx)
	if e != nil {
		t.Fatal(e)
	}
	instruments := time.Since(start)
	if holding.Quantity != "1" || len(accounts) != 1 || len(accounts[0].Holdings) != 500 || len(groups) != 500 {
		t.Fatal("incomplete representative gain reads")
	}
	t.Logf("500 positive positions with history costs: HoldingGain=%s AccountGains=%s InstrumentHoldings=%s; snapshot intentionally materializes household inputs", single, batch, instruments)
}
