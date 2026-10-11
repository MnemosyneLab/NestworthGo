package application

import (
	"context"
	"fmt"
	"github.com/waltwang/nestworth-go/internal/domain"
	"runtime"
	"sort"
	"testing"
	"time"
)

// Synthetic in-memory repository isolates orchestration overhead from disk I/O.
func upcomingScaleSnapshot() domain.LiquiditySnapshot {
	now := time.Date(2026, 10, 11, 4, 0, 0, 0, time.UTC)
	house := domain.NewHouseholdID()
	snapshot := domain.LiquiditySnapshot{Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: house, BaseCurrency: "USD"}}}
	bank := domain.Account{ID: domain.NewAccountID(), HouseholdID: house, Name: "Synthetic products", AccountType: domain.TypeBankAccount, TrackingMode: domain.TrackingHoldings, DefaultCurrency: "USD"}
	snapshot.Portfolio.Accounts = append(snapshot.Portfolio.Accounts, domain.AccountRecord{Account: bank})
	money, _ := domain.ParseMoney("100", "USD")
	zero, _ := domain.ParseMoney("0", "USD")
	reserve, _ := domain.ParseMoney("60", "USD")
	quantity, _ := domain.ParseQuantity("1")
	price, _ := domain.ParseUnitPrice("100")
	for i := 0; i < 1000; i++ {
		var ref domain.LiquiditySourceRef
		if i < 500 {
			a := domain.Account{ID: domain.NewAccountID(), HouseholdID: house, Name: fmt.Sprintf("Cash %04d", i), AccountType: domain.TypeBankAccount, TrackingMode: domain.TrackingBalance, DefaultCurrency: "USD"}
			snapshot.Portfolio.Accounts = append(snapshot.Portfolio.Accounts, domain.AccountRecord{Account: a, LatestValue: &domain.AccountValue{AccountID: a.ID, Amount: money, EffectiveAt: now}})
			ref = domain.AccountValueSourceRef(a.ID)
		} else {
			holding := domain.NewHoldingID()
			instrument := domain.NewInstrumentID()
			product := domain.NewProductContractID()
			maturity := "2026-10-20"
			days := 0
			basis := domain.DayBasisCalendar
			ref = domain.HoldingSourceRef(bank.ID, holding)
			snapshot.Portfolio.Holdings = append(snapshot.Portfolio.Holdings, domain.Holding{ID: holding, AccountID: bank.ID, InstrumentID: instrument, Quantity: quantity})
			snapshot.Portfolio.Instruments = append(snapshot.Portfolio.Instruments, domain.Instrument{ID: instrument, HouseholdID: house, Name: fmt.Sprintf("Product %04d", i), Type: domain.InstrumentBankInvestmentProduct, QuoteCurrency: "USD", QuoteSource: domain.QuoteSourceManual})
			snapshot.Portfolio.InstrumentQuotes = append(snapshot.Portfolio.InstrumentQuotes, domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrument, UnitPrice: price, Currency: "USD", SourceKind: domain.QuoteSourceManual, SourceKey: "manual", QuotedAt: now, CreatedAt: now})
			snapshot.Contracts = append(snapshot.Contracts, domain.ProductContract{ID: product, AccountID: bank.ID, HoldingID: holding, InstrumentID: instrument, Kind: domain.ProductTermDeposit, Name: fmt.Sprintf("Product %04d", i), Currency: "USD", Principal: money, InterestMode: domain.InterestNone, State: domain.ProductStateOpen, MaturityOn: &maturity})
			snapshot.Policies = append(snapshot.Policies, domain.LiquidityPolicy{ID: domain.NewLiquidityPolicyID(), HouseholdID: house, Source: ref, AccessKind: domain.AccessOnDate, UnlockOn: &maturity, SettlementDays: &days, DayBasis: &basis, NormalExitFee: &zero, EarlyKind: domain.EarlyNotAllowed, Revision: 1})
		}
		for j := 0; j < 2; j++ {
			snapshot.Reservations = append(snapshot.Reservations, domain.LiquidityReservation{ID: domain.NewLiquidityReservationID(), HouseholdID: house, Source: ref, Label: fmt.Sprintf("Reserve %d-%d", i, j), Amount: reserve, Currency: "USD", Revision: 1})
		}
	}
	return snapshot
}
func TestUpcomingActionsScaleReadBound(t *testing.T) {
	spy := &upcomingSnapshotSpy{snapshot: upcomingScaleSnapshot()}
	s := NewService(spy)
	s.SetClock(func() time.Time { return time.Date(2026, 10, 11, 4, 0, 0, 0, time.UTC) })
	got, e := s.UpcomingActions(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if spy.reads.Load() != 1 || got.Counts.ReservationIssues != 1000 || got.Counts.Scheduled != 500 {
		t.Fatalf("reads=%d counts=%+v", spy.reads.Load(), got.Counts)
	}
}
func BenchmarkUpcomingActionsScale(b *testing.B) {
	spy := &upcomingSnapshotSpy{snapshot: upcomingScaleSnapshot()}
	s := NewService(spy)
	s.SetClock(func() time.Time { return time.Date(2026, 10, 11, 4, 0, 0, 0, time.UTC) })
	ctx := context.Background()
	o, snapshot, sources, e := s.prepareLiquidity(ctx, LiquidityOverviewQuery{}, s.clock())
	if e != nil {
		b.Fatal(e)
	}
	b.Run("projection", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, e := domain.ProjectUpcomingActions(o, sources, snapshot); e != nil {
				b.Fatal(e)
			}
		}
	})
	b.Run("endpoint", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, e := s.UpcomingActions(ctx); e != nil {
				b.Fatal(e)
			}
		}
	})
	b.Run("overview", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, e := s.LiquidityOverview(ctx, LiquidityOverviewQuery{}); e != nil {
				b.Fatal(e)
			}
		}
	})
}

// Timing ratios are a report, not a noisy CI correctness assertion.
func TestUpcomingActionsPerformanceReport(t *testing.T) {
	if testing.Short() {
		t.Skip("timing report")
	}
	spy := &upcomingSnapshotSpy{snapshot: upcomingScaleSnapshot()}
	s := NewService(spy)
	s.SetClock(func() time.Time { return time.Date(2026, 10, 11, 4, 0, 0, 0, time.UTC) })
	ctx := context.Background()
	o, snapshot, sources, e := s.prepareLiquidity(ctx, LiquidityOverviewQuery{}, s.clock())
	if e != nil {
		t.Fatal(e)
	}
	measure := func(fn func() error) time.Duration {
		for i := 0; i < 3; i++ {
			if e := fn(); e != nil {
				t.Fatal(e)
			}
		}
		samples := []time.Duration{}
		for i := 0; i < 20; i++ {
			start := time.Now()
			if e := fn(); e != nil {
				t.Fatal(e)
			}
			samples = append(samples, time.Since(start))
		}
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		return samples[18]
	}
	projection := measure(func() error { _, e := domain.ProjectUpcomingActions(o, sources, snapshot); return e })
	endpoint := measure(func() error { _, e := s.UpcomingActions(ctx); return e })
	overview := measure(func() error { _, e := s.LiquidityOverview(ctx, LiquidityOverviewQuery{}); return e })
	t.Logf("%s/%s %s; 1000 sources / 2000 reservations / 500 products; warmed 20-run P95 projection=%s endpoint=%s overview=%s endpoint review threshold=%s", runtime.GOOS, runtime.GOARCH, runtime.Version(), projection, endpoint, overview, time.Duration(float64(overview)*1.25)+50*time.Millisecond)
}
