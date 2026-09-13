package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestRebuildHistoricalSnapshotsInvalidStartIsValidationError(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/invalid-snapshot-range.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	_, err = NewService(sqlite.NewRepository(database)).RebuildHistoricalSnapshots(context.Background(), "not-a-date", "2026-08-01")
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != domain.ErrValidation || domainErr.Field != "dateRange" {
		t.Fatalf("err = %v, want validation/dateRange domain error", err)
	}
}

func TestPostOriginEntitiesUseCreationBaselinesBeforeLaterEdits(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/creation-baselines.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	clock := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Creation baselines", BaseCurrency: "CNY", MemberNames: []string{"Owner one", "Owner two"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ownerOne, ownerTwo := bootstrap.Members[0].ID, bootstrap.Members[1].ID
	if _, err := service.CreateAccount(ctx, AccountInput{Name: "Existing", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "100", OwnerIDs: []domain.MemberID{ownerOne}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	created, err := service.CreateAccount(ctx, AccountInput{Name: "Created after start", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "50", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{ownerOne}})
	if err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	if _, err := service.UpdateAccount(ctx, created.Account.ID, AccountInput{OwnerIDs: []domain.MemberID{ownerTwo}, IncludeInNetWorth: false, IncludeInNetWorthSet: true}); err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	origin, err := service.HistoryOrigin(ctx)
	if err != nil || origin == nil {
		t.Fatalf("origin = %+v, err=%v", origin, err)
	}
	early, err := service.historicalPortfolioSnapshot(ctx, origin, time.Date(2026, 8, 2, 23, 59, 59, 999000000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	earlyRecord, ok := accountRecordByID(early.Accounts, created.Account.ID)
	if !ok || !earlyRecord.Account.IncludeInNetWorth || len(earlyRecord.Ownership.Shares()) != 1 || earlyRecord.Ownership.Shares()[0].MemberID != ownerOne {
		t.Fatalf("creation baseline leaked later edit: %+v", earlyRecord)
	}
	late, err := service.historicalPortfolioSnapshot(ctx, origin, time.Date(2026, 8, 3, 23, 59, 59, 999000000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	lateRecord, ok := accountRecordByID(late.Accounts, created.Account.ID)
	if !ok || lateRecord.Account.IncludeInNetWorth || len(lateRecord.Ownership.Shares()) != 1 || lateRecord.Ownership.Shares()[0].MemberID != ownerTwo {
		t.Fatalf("later account edit not reconstructed: %+v", lateRecord)
	}

	clock = time.Date(2026, 8, 2, 13, 0, 0, 0, time.UTC)
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Created instrument", Type: "stock", QuoteCurrency: "CNY", QuoteSource: "provider", ProviderKey: "fake", ProviderSymbol: "NEW"})
	if err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 8, 3, 13, 0, 0, 0, time.UTC)
	if err := service.SetInstrumentQuoteSource(ctx, instrument.ID, "manual"); err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	early, err = service.historicalPortfolioSnapshot(ctx, origin, time.Date(2026, 8, 2, 23, 59, 59, 999000000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	late, err = service.historicalPortfolioSnapshot(ctx, origin, time.Date(2026, 8, 3, 23, 59, 59, 999000000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	var earlyInstrument, lateInstrument domain.Instrument
	for _, value := range early.Instruments {
		if value.ID == instrument.ID {
			earlyInstrument = value
		}
	}
	for _, value := range late.Instruments {
		if value.ID == instrument.ID {
			lateInstrument = value
		}
	}
	if earlyInstrument.QuoteSource != domain.QuoteSourceProvider || lateInstrument.QuoteSource != domain.QuoteSourceManual {
		t.Fatalf("instrument preference baselines = early=%+v late=%+v", earlyInstrument, lateInstrument)
	}
}

func TestRebuildLoadsOneImmutableBatchAndResumesCursor(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/batch-rebuild.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	clock := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Batch", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, _ := service.Bootstrap(ctx)
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "100", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	amount, _ := domain.ParseMoney("25", "CNY")
	if _, err := service.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: amount, Reason: domain.ReasonIncome, EffectiveAt: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	count, err := service.RebuildHistoricalSnapshots(ctx, "2026-08-01", "2026-08-03")
	if err != nil || count != 3 {
		t.Fatalf("rebuilt count=%d err=%v", count, err)
	}
	state, err := service.DailySnapshotState(ctx, bootstrap.Household.ID)
	if err != nil || state.LastCompletedClosedOn == nil || *state.LastCompletedClosedOn != "2026-08-03" || state.DirtyFrom == nil || *state.DirtyFrom != "2026-08-04" {
		t.Fatalf("snapshot cursor = %+v, err=%v", state, err)
	}
	var snapshots int
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM daily_valuation_snapshots WHERE household_id = ?", bootstrap.Household.ID.String()).Scan(&snapshots); err != nil {
		t.Fatal(err)
	}
	if snapshots != 3 {
		t.Fatalf("snapshots=%d, want 3", snapshots)
	}
}

type cancelAfterSnapshotRepository struct {
	Repository
	base      GenerationAwareSnapshotRepository
	cancel    context.CancelFunc
	cancelAt  int
	saveCount int
	once      sync.Once
}

func (r *cancelAfterSnapshotRepository) SaveDailyValuationSnapshotAndMarkCompletedAtGeneration(ctx context.Context, snapshot domain.DailyValuationSnapshot, updatedAt time.Time, expectedGeneration int) (bool, error) {
	appended, err := r.base.SaveDailyValuationSnapshotAndMarkCompletedAtGeneration(ctx, snapshot, updatedAt, expectedGeneration)
	if err == nil {
		r.saveCount++
		if r.saveCount == r.cancelAt {
			r.once.Do(r.cancel)
		}
	}
	return appended, err
}

func (r *cancelAfterSnapshotRepository) CompleteDailySnapshotRangeAtGeneration(ctx context.Context, householdID domain.HouseholdID, targetDate string, updatedAt time.Time, expectedGeneration int) error {
	return r.base.CompleteDailySnapshotRangeAtGeneration(ctx, householdID, targetDate, updatedAt, expectedGeneration)
}

func TestRebuildDirtySnapshotsPreservesRemainingRangeAcrossInterruptedBatch(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/cross-batch-rebuild.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	baseRepository := sqlite.NewRepository(database)
	firstRunCtx, cancelFirstRun := context.WithCancel(context.Background())
	repository := &cancelAfterSnapshotRepository{Repository: baseRepository, base: baseRepository, cancel: cancelFirstRun, cancelAt: 31}
	service := NewService(repository)
	clock := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	if err := service.CompleteOnboarding(context.Background(), OnboardingInput{HouseholdName: "Cross-batch rebuild", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateAccount(context.Background(), AccountInput{Name: "Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "100", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}}); err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	if _, err := service.StartHistory(context.Background(), "UTC"); err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	if _, err := database.SQL.ExecContext(context.Background(), `UPDATE history_snapshot_state SET dirty_from = ?, dirty_to = ? WHERE household_id = ?`, "2026-10-01", "2026-11-15", bootstrap.Household.ID.String()); err != nil {
		t.Fatal(err)
	}

	appended, err := service.RebuildDirtySnapshots(firstRunCtx)
	if !errors.Is(err, context.Canceled) || appended != 0 {
		t.Fatalf("interrupted rebuild = appended %d err %v, want 0/context.Canceled", appended, err)
	}
	cancelFirstRun()
	state, err := service.DailySnapshotState(context.Background(), bootstrap.Household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.DirtyFrom == nil || *state.DirtyFrom != "2026-11-01" || state.DirtyTo == nil || *state.DirtyTo != "2026-11-15" {
		t.Fatalf("remaining dirty range after interruption = %+v, want 2026-11-01..2026-11-15", state)
	}

	appended, err = service.RebuildDirtySnapshots(context.Background())
	if err != nil || appended != 15 {
		t.Fatalf("restart rebuild = appended %d err %v, want 15/nil", appended, err)
	}
	state, err = service.DailySnapshotState(context.Background(), bootstrap.Household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.DirtyFrom != nil || state.DirtyTo != nil {
		t.Fatalf("dirty range after restart = %+v, want cleared", state)
	}
	var snapshots int
	if err := database.SQL.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM daily_valuation_snapshots WHERE household_id = ?`, bootstrap.Household.ID.String()).Scan(&snapshots); err != nil {
		t.Fatal(err)
	}
	if snapshots != 46 {
		t.Fatalf("snapshots after restart = %d, want 46", snapshots)
	}
}

func TestActivityPageComposesFiltersAndKeysetCursor(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/activity-page.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	clock := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Activity page", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "100", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	for _, entry := range []struct {
		amount string
		date   time.Time
		kind   domain.ActivityKind
	}{
		{amount: "10", date: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC), kind: domain.ActivityCashIn},
		{amount: "5", date: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), kind: domain.ActivityCashOut},
		{amount: "20", date: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC), kind: domain.ActivityCashIn},
	} {
		money, parseErr := domain.ParseMoney(entry.amount, "CNY")
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		var input any = domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: money, Reason: domain.ReasonIncome, EffectiveAt: entry.date}
		if entry.kind == domain.ActivityCashOut {
			input = domain.MoneyRemovedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: money, Reason: domain.ReasonExpense, EffectiveAt: entry.date}
		}
		if _, recordErr := service.RecordChange(ctx, input); recordErr != nil {
			t.Fatal(recordErr)
		}
	}

	accountID := account.Account.ID
	page, err := service.ListActivityPage(ctx, domain.ActivityQuery{AccountID: &accountID, FromLocalDate: "2026-08-02", ToLocalDate: "2026-08-03", Limit: 1})
	if err != nil || len(page.Activities) != 1 || page.Activities[0].Kind != domain.ActivityCashIn || !page.HasMore || page.Next == nil {
		t.Fatalf("first activity page = %+v, err=%v", page, err)
	}
	kindPage, err := service.ListActivityPage(ctx, domain.ActivityQuery{AccountID: &accountID, FromLocalDate: "2026-08-02", ToLocalDate: "2026-08-03", Kinds: []domain.ActivityKind{domain.ActivityCashOut}, Limit: 1})
	if err != nil || len(kindPage.Activities) != 1 || kindPage.Activities[0].Kind != domain.ActivityCashOut {
		t.Fatalf("kind filter = %+v, err=%v", kindPage, err)
	}

	page, err = service.ListActivityPage(ctx, domain.ActivityQuery{AccountID: &accountID, Limit: 1})
	if err != nil || len(page.Activities) != 1 || page.Activities[0].Kind != domain.ActivityCashIn || page.Next == nil {
		t.Fatalf("unfiltered first page = %+v, err=%v", page, err)
	}
	seen := []domain.ActivityID{page.Activities[0].ID}
	for page.Next != nil {
		page, err = service.ListActivityPage(ctx, domain.ActivityQuery{AccountID: &accountID, After: page.Next, Limit: 1})
		if err != nil || len(page.Activities) != 1 {
			t.Fatalf("keyset page = %+v, err=%v", page, err)
		}
		seen = append(seen, page.Activities[0].ID)
	}
	if len(seen) != 3 || seen[0] == seen[1] || seen[1] == seen[2] || seen[0] == seen[2] {
		t.Fatalf("keyset traversal = %v", seen)
	}
}

func TestBackdatedManualQuotesClampHistoryAndKeepFXProvenance(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/backdated-quotes.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Quotes", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, _ := service.Bootstrap(ctx)
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "USD fund", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "10", "2026-07-20", false); err != nil {
		t.Fatal(err)
	}
	if err := service.SetInstrumentQuoteSource(ctx, instrument.ID, "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetFXPreference(ctx, "USD", "CNY", "manual"); err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "10", "2026-07-20", false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualFXQuote(ctx, "USD", "CNY", "7", "2026-07-20"); err != nil {
		t.Fatal(err)
	}
	var dirty string
	if err := database.SQL.QueryRow("SELECT dirty_from FROM history_snapshot_state WHERE household_id = ?", bootstrap.Household.ID.String()).Scan(&dirty); err != nil {
		t.Fatal(err)
	}
	if dirty != "2026-08-01" {
		t.Fatalf("dirty_from = %q, want origin date", dirty)
	}
	count, err := service.RebuildHistoricalSnapshots(ctx, "2026-08-01", "2026-08-02")
	if err != nil || count != 2 {
		t.Fatalf("backdated rebuild count=%d err=%v", count, err)
	}
	snapshots, err := service.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, time.Time{}, time.Time{})
	if err != nil || len(snapshots) != 2 {
		t.Fatalf("snapshots=%d err=%v", len(snapshots), err)
	}
	for _, snapshot := range snapshots {
		if snapshot.NetWorthAmount == nil || snapshot.NetWorthAmount.CanonicalAmount() != "70" {
			t.Fatalf("backdated quote snapshot = %+v", snapshot)
		}
		if len(snapshot.Items) != 1 || snapshot.Items[0].QuoteID == nil || snapshot.Items[0].FXQuoteID == nil {
			t.Fatalf("quote provenance = %+v", snapshot.Items)
		}
	}
	if err := database.SQL.QueryRow("SELECT dirty_from FROM history_snapshot_state WHERE household_id = ?", bootstrap.Household.ID.String()).Scan(&dirty); err != nil {
		t.Fatal(err)
	}
	if dirty != "2026-08-03" {
		t.Fatalf("dirty_from after rebuild = %q, want next date", dirty)
	}
}

func TestArchiveIntervalsRebuildAndRetainActiveHoldings(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/archive-intervals.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Archive intervals", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Fund", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "10", "2026-07-30", false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "1"}); err != nil {
		t.Fatal(err)
	}
	zeroInstrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Zero fund", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	zeroHolding, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: zeroInstrument.ID.String(), Quantity: "0"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "10", "2026-07-30", false); err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	if err := service.ArchiveInstrument(ctx, instrument.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := service.ArchiveHolding(ctx, zeroHolding.ID, true); err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	if err := service.ArchiveInstrument(ctx, instrument.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := service.ArchiveHolding(ctx, zeroHolding.ID, false); err != nil {
		t.Fatal(err)
	}

	origin, err := service.HistoryOrigin(ctx)
	if err != nil || origin == nil {
		t.Fatalf("origin = %+v, err=%v", origin, err)
	}
	for _, point := range []struct {
		date         string
		cutoff       time.Time
		wantArchived bool
	}{
		{date: "2026-08-01", cutoff: time.Date(2026, 8, 1, 23, 59, 59, 0, time.UTC), wantArchived: false},
		{date: "2026-08-02", cutoff: time.Date(2026, 8, 2, 23, 59, 59, 0, time.UTC), wantArchived: true},
		{date: "2026-08-03", cutoff: time.Date(2026, 8, 3, 23, 59, 59, 0, time.UTC), wantArchived: true},
		{date: "2026-08-04", cutoff: time.Date(2026, 8, 4, 23, 59, 59, 0, time.UTC), wantArchived: false},
	} {
		snapshot, snapshotErr := service.historicalPortfolioSnapshot(ctx, origin, point.cutoff)
		if snapshotErr != nil {
			t.Fatal(snapshotErr)
		}
		var foundInstrument domain.Instrument
		for _, item := range snapshot.Instruments {
			if item.ID == instrument.ID {
				foundInstrument = item
			}
		}
		if (foundInstrument.ArchivedAt != nil) != point.wantArchived {
			t.Fatalf("instrument archive at %s = %v, want %v", point.date, foundInstrument.ArchivedAt, point.wantArchived)
		}
		for _, holding := range snapshot.Holdings {
			if holding.ID != zeroHolding.ID {
				continue
			}
			if (holding.ArchivedAt != nil) != point.wantArchived {
				t.Fatalf("zero holding archive at %s = %v, want %v", point.date, holding.ArchivedAt, point.wantArchived)
			}
		}
		if len(snapshot.Holdings) != 2 {
			t.Fatalf("holdings at %s = %+v", point.date, snapshot.Holdings)
		}
	}

	clock = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	if count, err := service.RebuildHistoricalSnapshots(ctx, "2026-08-01", "2026-08-04"); err != nil || count != 4 {
		t.Fatalf("archive rebuild count=%d err=%v", count, err)
	}
	snapshots, err := service.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, snapshot := range snapshots {
		if snapshot.NetWorthAmount == nil || snapshot.NetWorthAmount.CanonicalAmount() != "10" {
			t.Fatalf("active holding disappeared during instrument archive: %+v", snapshot)
		}
	}
}

func TestSnapshotHashIncludesFXPreferenceEvidence(t *testing.T) {
	amount, err := domain.ParseMoney("10", "CNY")
	if err != nil {
		t.Fatal(err)
	}
	netWorth, err := domain.ParseSignedMoney("10", "CNY")
	if err != nil {
		t.Fatal(err)
	}
	firstPreference := domain.NewFXPreferenceObservationID()
	secondPreference := domain.NewFXPreferenceObservationID()
	first := domain.DailyValuationSnapshotItem{
		ID:                        domain.NewDailyValuationSnapshotItemID(),
		AccountID:                 domain.NewAccountID(),
		NativeAmount:              "10",
		NativeCurrency:            domain.CurrencyCode("CNY"),
		BaseAmount:                &amount,
		Complete:                  true,
		FXPreferenceObservationID: &firstPreference,
	}
	second := first
	second.FXPreferenceObservationID = &secondPreference
	firstHash := snapshotContentHash("2026-08-01", time.Date(2026, 8, 1, 23, 59, 59, 0, time.UTC), amount, amount, netWorth, []domain.DailyValuationSnapshotItem{first})
	secondHash := snapshotContentHash("2026-08-01", time.Date(2026, 8, 1, 23, 59, 59, 0, time.UTC), amount, amount, netWorth, []domain.DailyValuationSnapshotItem{second})
	if firstHash == secondHash {
		t.Fatalf("snapshot hash ignored FX preference evidence: %q", firstHash)
	}
}
