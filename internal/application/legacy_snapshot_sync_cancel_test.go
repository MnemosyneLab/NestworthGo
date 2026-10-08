package application

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type coverageCancelPersister struct {
	testHistoryPersist
	committed chan struct{}
}

func (p coverageCancelPersister) PersistInstrumentHistory(ctx context.Context, request CommitInstrumentHistoryRequest) (CommitHistoryResult, error) {
	result, err := p.testHistoryPersist.PersistInstrumentHistory(ctx, request)
	if err == nil {
		close(p.committed)
		<-ctx.Done() // Cancel after the real commit, before snapshot maintenance.
	}
	return result, err
}

func TestLegacyCancelledSyncThenOrdinaryIncomeRepairsIncompleteTail(t *testing.T) {
	t.Parallel()
	db, err := sqlite.Open(t.TempDir() + "/cancelled-sync.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewRepository(db)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	fake := &syncFakeProvider{key: TiingoProviderKey, history: func(_ context.Context, _ InstrumentMarketIdentity, _ DateRange, _ int) (MappingOutcome[InstrumentDailyObservation], error) {
		result := mappedTiingoClose("2026-08-04", "5")
		result.Batch.Observations[0].ValueEffectiveAt = time.Date(2026, 8, 4, 20, 0, 0, 0, time.UTC)
		result.Batch.VerifiedRanges = []DateRange{{Start: "2026-08-01", End: "2026-08-05"}}
		return result, nil
	}}
	s := NewService(repo, NewMarketDataRegistry(fake))
	s.setClock(func() time.Time { return now })
	s.SetLiveDatabasePath(db.Path)
	committed := make(chan struct{})
	s.SetHistoryPersister(coverageCancelPersister{testHistoryPersist{repo}, committed})
	if err := s.CompleteOnboarding(t.Context(), OnboardingInput{HouseholdName: "Synthetic cancelled sync", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	b, err := s.Bootstrap(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	cash, err := s.CreateAccount(t.Context(), AccountInput{Name: "Cash", IncludeInNetWorth: true, AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "100", OwnerIDs: []domain.MemberID{b.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	broker, err := s.CreateAccount(t.Context(), AccountInput{Name: "Broker", IncludeInNetWorth: true, IncludeInPortfolio: true, AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{b.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := s.CreateInstrument(t.Context(), InstrumentInput{Name: "Synthetic ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "provider", ProviderKey: TiingoProviderKey, ProviderSymbol: "SYNTHETIC", MarketCode: "US"})
	if err != nil {
		t.Fatal(err)
	}
	price, _ := domain.ParseUnitPrice("5")
	if err := repo.AppendInstrumentQuote(t.Context(), domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrument.ID, UnitPrice: price, Currency: "USD", SourceKind: domain.QuoteSourceProvider, SourceKey: TiingoProviderKey, QuotedAt: now, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	holding, err := s.CreateHolding(t.Context(), HoldingInput{AccountID: broker.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "10", UnitCost: "5"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistoryWithCosts(t.Context(), "UTC", map[domain.HoldingID]string{holding.ID: "5"}); err != nil {
		t.Fatal(err)
	}
	now = time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	start, err := s.StartMarketDataSync(t.Context(), SyncRequest{Scope: SyncScopeInstrument, InstrumentID: instrument.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-committed:
	case <-time.After(5 * time.Second):
		t.Fatal("history commit did not arrive")
	}
	if _, ok := s.CancelSyncJob(start.Job.JobID); !ok {
		t.Fatal("cancel rejected")
	}
	terminal := waitSyncTerminal(t, s)
	if terminal.Outcome != SyncOutcomeCancelled {
		t.Fatal(terminal)
	}
	state, err := repo.DailySnapshotState(t.Context(), b.Household.ID)
	if err != nil || state.DirtyFrom == nil || *state.DirtyFrom != "2026-08-01" || state.DirtyTo == nil || *state.DirtyTo != "2026-08-05" || state.LastCompletedClosedOn != nil {
		t.Fatal("cancelled bounded state", state, err)
	}
	now = time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	read := func(want string, generation int) {
		t.Helper()
		trend, err := s.NetWorthTrend(t.Context(), domain.TrendRange("2026-08-06:2026-08-11"))
		if err != nil || trend.Complete || trend.Change != nil || trend.SummaryReason != "missing_boundary" {
			t.Fatal("incomplete valuation contract", trend, err)
		}
		rows, err := repo.ListDailyValuationSnapshots(t.Context(), b.Household.ID, time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC))
		if err != nil || len(rows) != 6 {
			t.Fatal(rows, err)
		}
		for _, row := range rows {
			if row.NetWorthAmount == nil {
				t.Fatal("missing stored subtotal", row.LocalDate)
			}
			if row.Complete || row.NetWorthAmount.CanonicalAmount() != want || row.InputGeneration != generation {
				t.Fatalf("%s incomplete subtotal=%s generation=%d; want %s / %d", row.LocalDate, row.NetWorthAmount.CanonicalAmount(), row.InputGeneration, want, generation)
			}
		}
		t.Log("incomplete tail subtotal/generation", want, generation)
	}
	read("150", state.InputGeneration)
	if _, err := s.RecordChange(t.Context(), domain.MoneyAddedInput{HouseholdID: b.Household.ID, AccountID: cash.Account.ID, Amount: mustMoney(t, "50", "USD"), Reason: domain.ReasonIncome, EffectiveAt: time.Date(2026, 8, 3, 13, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	read("200", state.InputGeneration+1)
	if err := s.ensureClosedDaySnapshots(t.Context(), "2026-08-01", "2026-08-11"); err != nil {
		t.Fatal(err)
	}
	read("200", state.InputGeneration+1)
	after, err := repo.DailySnapshotState(t.Context(), b.Household.ID)
	if err != nil || after.DirtyFrom == nil || *after.DirtyFrom != "2026-08-12" || after.DirtyTo == nil || *after.DirtyTo != "2026-08-15" {
		t.Fatal("uncovered tail lost", after, err)
	}
	health, err := s.ScanMarketDataHealth(t.Context())
	if err != nil || health.Healthy {
		t.Fatal("incomplete inputs advertised healthy", health, err)
	}
	for _, issue := range health.Issues {
		if issue.Kind == HealthKindSnapshotMissing && issue.Executable {
			t.Fatal("missing inputs advertised as executable snapshot work", issue)
		}
	}
}
