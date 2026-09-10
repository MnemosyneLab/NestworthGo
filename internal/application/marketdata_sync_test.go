package application

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type syncFakeProvider struct {
	key string
	now time.Time

	mu           sync.Mutex
	historyCalls int
	history      func(ctx context.Context, identity InstrumentMarketIdentity, rng DateRange, call int) (MappingOutcome[InstrumentDailyObservation], error)
	fx           func(ctx context.Context, identity FXMarketIdentity, rng DateRange) (MappingOutcome[FXDailyObservation], error)
}

func (p *syncFakeProvider) Key() string { return p.key }

func (p *syncFakeProvider) Capabilities() MarketDataCapabilities {
	return MarketDataCapabilities{LatestInstrument: true, LatestFX: true, InstrumentDailyHistory: true, FXDailyHistory: true}
}

func (p *syncFakeProvider) LatestInstrument(_ context.Context, identity InstrumentMarketIdentity) (LatestInstrumentQuote, error) {
	price, _ := domain.ParseUnitPrice("185.25")
	quoted := p.now
	if quoted.IsZero() {
		quoted = time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC)
	}
	return LatestInstrumentQuote{Price: price, Currency: identity.QuoteCurrency, SourceKey: p.key, QuotedAt: quoted.UTC()}, nil
}

func (p *syncFakeProvider) LatestFX(_ context.Context, identity FXMarketIdentity) (LatestFXQuote, error) {
	rate, _ := domain.ParseFxRate("1.35")
	quoted := p.now
	if quoted.IsZero() {
		quoted = time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC)
	}
	return LatestFXQuote{Rate: rate, BaseCurrency: identity.BaseCurrency, QuoteCurrency: identity.QuoteCurrency, SourceKey: p.key, QuotedAt: quoted.UTC()}, nil
}

func (p *syncFakeProvider) InstrumentDailyHistory(ctx context.Context, identity InstrumentMarketIdentity, rng DateRange) (MappingOutcome[InstrumentDailyObservation], error) {
	p.mu.Lock()
	p.historyCalls++
	call := p.historyCalls
	hook := p.history
	p.mu.Unlock()
	if hook == nil {
		return MappingOutcome[InstrumentDailyObservation]{Status: MappingUnsupported, Reason: "unsupported_price_basis"}, nil
	}
	return hook(ctx, identity, rng, call)
}

func (p *syncFakeProvider) FXDailyHistory(ctx context.Context, identity FXMarketIdentity, rng DateRange) (MappingOutcome[FXDailyObservation], error) {
	if p.fx == nil {
		return MappingOutcome[FXDailyObservation]{Status: MappingUnsupported, Reason: "unsupported"}, nil
	}
	return p.fx(ctx, identity, rng)
}

func mappedTiingoClose(date, value string) MappingOutcome[InstrumentDailyObservation] {
	effective := time.Date(2026, 9, 4, 20, 0, 0, 0, time.UTC)
	return MappingOutcome[InstrumentDailyObservation]{
		Status: MappingMapped,
		Batch: HistoryBatch[InstrumentDailyObservation]{
			Observations: []InstrumentDailyObservation{{
				MarketDate:       MarketDate(date),
				Value:            value,
				Currency:         "USD",
				ValueEffectiveAt: effective,
				Kind:             InstrumentObservationClose,
				PriceBasis:       PriceBasisTiingoRawClose,
				TimestampBasis:   TimestampBasisSessionClose,
			}},
			VerifiedRanges: []DateRange{{Start: MarketDate(date), End: MarketDate(date)}},
			Evidence:       ResponseEvidence{Adapter: "tiingo", PriceBasis: PriceBasisTiingoRawClose, SourcePolicy: string(PriceBasisTiingoRawClose)},
		},
	}
}

func mappedFXRate(date, rate string) MappingOutcome[FXDailyObservation] {
	effective := time.Date(2026, 9, 6, 16, 0, 0, 0, time.UTC)
	return MappingOutcome[FXDailyObservation]{
		Status: MappingMapped,
		Batch: HistoryBatch[FXDailyObservation]{
			Observations: []FXDailyObservation{{
				MarketDate:       MarketDate(date),
				Rate:             rate,
				BaseCurrency:     "USD",
				QuoteCurrency:    "SGD",
				ValueEffectiveAt: effective,
				Kind:             FXObservationDailyReference,
				TimestampBasis:   TimestampBasisPolicyDerived,
			}},
			VerifiedRanges: []DateRange{{Start: MarketDate(date), End: MarketDate(date)}},
			Evidence:       ResponseEvidence{Adapter: "frankfurter", SourcePolicy: "frankfurter_v2"},
		},
	}
}

func newSyncFixture(t *testing.T, providers ...MarketDataProvider) (*Service, *sqlite.Repository, domain.Instrument, domain.Instrument) {
	t.Helper()
	sgt, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	originClock := time.Date(2026, 9, 6, 12, 0, 0, 0, sgt)
	repairClock := time.Date(2026, 9, 10, 0, 5, 0, 0, sgt)
	database, err := sqlite.Open(t.TempDir() + "/sync.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	repo := sqlite.NewRepository(database)
	registry := NewMarketDataRegistry(providers...)
	service := NewService(repo, registry)
	service.SetLiveDatabasePath(database.Path)
	service.SetHistoryPersister(testHistoryPersist{repo: repo})
	service.syncSleep = func(ctx context.Context, wait time.Duration) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return nil
	}
	service.setClock(func() time.Time { return originClock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Sync", BaseCurrency: "SGD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, accountErr := service.CreateAccount(ctx, AccountInput{
		Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInPortfolio: true,
		Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}},
	})
	if accountErr != nil {
		t.Fatal(accountErr)
	}
	first, err := service.CreateInstrument(ctx, InstrumentInput{
		Name: "Apple", Type: "stock", QuoteCurrency: "USD", QuoteSource: "provider",
		ProviderKey: TiingoProviderKey, ProviderSymbol: "AAPL", MarketCode: "US",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateInstrument(ctx, InstrumentInput{
		Name: "Microsoft", Type: "stock", QuoteCurrency: "USD", QuoteSource: "provider",
		ProviderKey: YahooFinanceProviderKey, ProviderSymbol: "MSFT", MarketCode: "US",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "Asia/Singapore"); err != nil {
		t.Fatal(err)
	}
	service.setClock(func() time.Time { return repairClock })
	for _, provider := range providers {
		if fake, ok := provider.(*syncFakeProvider); ok {
			fake.now = repairClock
		}
	}
	return service, repo, first, second
}

func waitSyncTerminal(t *testing.T, service *Service) SyncJobSnapshot {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snap, ok := service.GetCurrentSyncJob()
		if ok && snap.Outcome != "" && snap.Outcome != SyncOutcomeRunning {
			return snap
		}
		time.Sleep(5 * time.Millisecond)
	}
	snap, _ := service.GetCurrentSyncJob()
	t.Fatalf("job did not finish: %+v", snap)
	return SyncJobSnapshot{}
}

func TestCapHistoryRangesDoesNotExpandScope(t *testing.T) {
	ranges := CapHistoryRanges([]DateRange{{Start: "2025-09-06", End: "2026-09-08"}}, 366)
	if len(ranges) != 2 {
		t.Fatalf("capped ranges = %d, want 2", len(ranges))
	}
	if ranges[0].Start != "2025-09-06" || ranges[len(ranges)-1].End != "2026-09-08" {
		t.Fatalf("capped = %+v", ranges)
	}
}

func TestPreviewMarketDataSyncEstimatesRequestsAndPrerequisites(t *testing.T) {
	tiingo := &syncFakeProvider{key: TiingoProviderKey}
	yahoo := &syncFakeProvider{key: YahooFinanceProviderKey}
	service, _, first, _ := newSyncFixture(t, tiingo, yahoo)
	preview, err := service.PreviewMarketDataSync(context.Background(), SyncRequest{Scope: SyncScopeRepairAll})
	if err != nil {
		t.Fatal(err)
	}
	if preview.EstimatedRequestCount <= 0 || preview.InstrumentTargets < 1 || preview.FetchRanges < 1 {
		t.Fatalf("preview = %+v", preview)
	}
	if preview.ConfigRevision != domain.MarketDataResolverPolicy {
		t.Fatalf("revision = %s", preview.ConfigRevision)
	}
	single, err := service.PreviewMarketDataSync(context.Background(), SyncRequest{Scope: SyncScopeInstrument, InstrumentID: first.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if single.InstrumentTargets != 1 || single.LatestFXCount != 0 {
		t.Fatalf("single-instrument preview = %+v", single)
	}
}

func TestStartMarketDataSyncAttachesEquivalentAndRejectsDifferentScope(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	tiingo := &syncFakeProvider{key: TiingoProviderKey, history: func(ctx context.Context, identity InstrumentMarketIdentity, rng DateRange, call int) (MappingOutcome[InstrumentDailyObservation], error) {
		if identity.ProviderSymbol == "AAPL" {
			startOnce.Do(func() { close(started) })
			select {
			case <-release:
			case <-ctx.Done():
				return MappingOutcome[InstrumentDailyObservation]{}, ctx.Err()
			}
		}
		return mappedTiingoClose("2026-09-04", "185.25"), nil
	}}
	yahoo := &syncFakeProvider{key: YahooFinanceProviderKey}
	service, _, first, _ := newSyncFixture(t, tiingo, yahoo)
	firstStart, err := service.StartMarketDataSync(context.Background(), SyncRequest{Scope: SyncScopeRepairAll})
	if err != nil || firstStart.Job.JobID == "" {
		t.Fatalf("start = %+v err=%v", firstStart, err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("job did not reach history fetch")
	}
	attached, err := service.StartMarketDataSync(context.Background(), SyncRequest{Scope: SyncScopeRepairAll})
	if err != nil || !attached.Attached || attached.Job.JobID != firstStart.Job.JobID {
		t.Fatalf("attach = %+v err=%v", attached, err)
	}
	current, ok := service.GetCurrentSyncJob()
	if !ok || current.JobID != firstStart.Job.JobID || current.Outcome != SyncOutcomeRunning {
		t.Fatalf("reopen snapshot = %+v ok=%v", current, ok)
	}
	conflict, err := service.StartMarketDataSync(context.Background(), SyncRequest{Scope: SyncScopeInstrument, InstrumentID: first.ID.String()})
	if err != nil || !conflict.Conflict || conflict.Reason != "different_scope_not_scheduled" {
		t.Fatalf("conflict = %+v err=%v", conflict, err)
	}
	close(release)
	waitSyncTerminal(t, service)
}

func TestCancelSyncJobRetainsCommittedBatches(t *testing.T) {
	startedSecond := make(chan struct{})
	release := make(chan struct{})
	var secondOnce sync.Once
	tiingo := &syncFakeProvider{key: TiingoProviderKey, history: func(ctx context.Context, identity InstrumentMarketIdentity, rng DateRange, call int) (MappingOutcome[InstrumentDailyObservation], error) {
		if identity.ProviderSymbol == "AAPL" && call == 1 {
			return mappedTiingoClose("2026-09-04", "185.25"), nil
		}
		secondOnce.Do(func() { close(startedSecond) })
		select {
		case <-release:
			return mappedTiingoClose("2026-09-08", "186.00"), nil
		case <-ctx.Done():
			return MappingOutcome[InstrumentDailyObservation]{}, ctx.Err()
		}
	}}
	service, repo, first, _ := newSyncFixture(t, tiingo)
	start, err := service.StartMarketDataSync(context.Background(), SyncRequest{Scope: SyncScopeRepairAll})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-startedSecond:
	case <-time.After(2 * time.Second):
		t.Fatal("second range did not start")
	}
	cancelled, ok := service.CancelSyncJob(start.Job.JobID)
	if !ok || cancelled.Outcome != SyncOutcomeCancelled {
		t.Fatalf("cancelled = %+v ok=%v", cancelled, ok)
	}
	if cancelled.CommittedBatches < 1 {
		t.Fatalf("expected retained committed batch, got %+v", cancelled)
	}
	ctx := context.Background()
	coverage, err := repo.ListInstrumentHistoryCoverage(ctx, cancelled.HouseholdID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range coverage {
		if item.InstrumentID == first.ID && len(item.CloseMarketDates) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("cancelled job dropped committed closes")
	}
	state, err := repo.DailySnapshotState(ctx, cancelled.HouseholdID)
	if err != nil {
		t.Fatal(err)
	}
	if state.DirtyFrom == nil || *state.DirtyFrom == "" {
		t.Fatalf("dirty range was cleared on cancel: %+v", state)
	}
	close(release)
}

func TestSyncJobRateLimitStopsProviderAndRecordsEligibility(t *testing.T) {
	tiingo := &syncFakeProvider{key: TiingoProviderKey, history: func(context.Context, InstrumentMarketIdentity, DateRange, int) (MappingOutcome[InstrumentDailyObservation], error) {
		return MappingOutcome[InstrumentDailyObservation]{}, &domain.Error{Code: domain.ErrProviderRateLimit, Message: "provider rate limit reached"}
	}}
	frankfurter := &syncFakeProvider{key: FrankfurterProviderKey, fx: func(context.Context, FXMarketIdentity, DateRange) (MappingOutcome[FXDailyObservation], error) {
		return mappedFXRate("2026-09-08", "1.35"), nil
	}}
	service, _, _, _ := newSyncFixture(t, tiingo, frankfurter)
	if err := service.SetFXProvider(FrankfurterProviderKey); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetFXPreference(context.Background(), "USD", "SGD", "provider"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartMarketDataSync(context.Background(), SyncRequest{Scope: SyncScopeRepairAll}); err != nil {
		t.Fatal(err)
	}
	snap := waitSyncTerminal(t, service)
	if snap.NextEligibilityAt == nil {
		t.Fatalf("expected next eligibility, got %+v", snap)
	}
	foundRate := false
	foundFX := false
	for _, blocker := range snap.Blockers {
		if blocker.Code == string(domain.ErrProviderRateLimit) {
			foundRate = true
		}
	}
	for _, item := range snap.Items {
		if item.Kind == "fx" && item.Status == "fetched" {
			foundFX = true
		}
	}
	if !foundRate {
		t.Fatalf("rate limit not recorded: %+v", snap.Blockers)
	}
	if !foundFX {
		t.Fatalf("frankfurter work did not continue after tiingo 429: %+v", snap.Items)
	}
	if snap.Outcome != SyncOutcomePartial {
		t.Fatalf("outcome = %s, want partial", snap.Outcome)
	}
}

func TestSyncJobDoesNotPersistAfterWorkspaceFence(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	tiingo := &syncFakeProvider{key: TiingoProviderKey, history: func(ctx context.Context, identity InstrumentMarketIdentity, rng DateRange, call int) (MappingOutcome[InstrumentDailyObservation], error) {
		startOnce.Do(func() { close(started) })
		select {
		case <-release:
			return mappedTiingoClose("2026-09-04", "185.25"), nil
		case <-ctx.Done():
			return MappingOutcome[InstrumentDailyObservation]{}, ctx.Err()
		}
	}}
	service, repo, first, _ := newSyncFixture(t, tiingo)
	if _, err := service.StartMarketDataSync(context.Background(), SyncRequest{Scope: SyncScopeRepairAll}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("job did not start fetch")
	}
	service.refreshEpoch.Add(1)
	close(release)
	snap := waitSyncTerminal(t, service)
	ctx := context.Background()
	coverage, err := repo.ListInstrumentHistoryCoverage(ctx, snap.HouseholdID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range coverage {
		if item.InstrumentID == first.ID && len(item.CloseMarketDates) > 0 {
			t.Fatalf("fenced job wrote closes: %+v", item.CloseMarketDates)
		}
	}
}

func TestYahooHistoryFailClosedIsPartialNotInvented(t *testing.T) {
	tiingo := &syncFakeProvider{key: TiingoProviderKey, history: func(context.Context, InstrumentMarketIdentity, DateRange, int) (MappingOutcome[InstrumentDailyObservation], error) {
		return mappedTiingoClose("2026-09-04", "185.25"), nil
	}}
	yahoo := &syncFakeProvider{key: YahooFinanceProviderKey, history: func(context.Context, InstrumentMarketIdentity, DateRange, int) (MappingOutcome[InstrumentDailyObservation], error) {
		return MappingOutcome[InstrumentDailyObservation]{Status: MappingUnsupported, Reason: "unsupported_price_basis"}, nil
	}}
	service, repo, first, second := newSyncFixture(t, tiingo, yahoo)
	if _, err := service.StartMarketDataSync(context.Background(), SyncRequest{Scope: SyncScopeRepairAll}); err != nil {
		t.Fatal(err)
	}
	snap := waitSyncTerminal(t, service)
	if snap.Outcome != SyncOutcomePartial && snap.Outcome != SyncOutcomeSucceeded {
		t.Fatalf("outcome = %s", snap.Outcome)
	}
	ctx := context.Background()
	coverage, err := repo.ListInstrumentHistoryCoverage(ctx, snap.HouseholdID)
	if err != nil {
		t.Fatal(err)
	}
	var appleCloses, msftCloses int
	for _, item := range coverage {
		if item.InstrumentID == first.ID {
			appleCloses = len(item.CloseMarketDates)
		}
		if item.InstrumentID == second.ID {
			msftCloses = len(item.CloseMarketDates)
		}
	}
	if appleCloses == 0 {
		t.Fatal("tiingo closes were not committed")
	}
	if msftCloses != 0 {
		t.Fatalf("yahoo history invented %d closes", msftCloses)
	}
	foundYahooBlocker := false
	for _, blocker := range snap.Blockers {
		if blocker.Reason == "unsupported_price_basis" || blocker.Code == string(MappingUnsupported) {
			foundYahooBlocker = true
		}
	}
	if !foundYahooBlocker {
		t.Fatalf("yahoo blocker missing: %+v", snap.Blockers)
	}
}

func TestTransientHistoryRetriesThenSucceeds(t *testing.T) {
	calls := 0
	tiingo := &syncFakeProvider{key: TiingoProviderKey, history: func(context.Context, InstrumentMarketIdentity, DateRange, int) (MappingOutcome[InstrumentDailyObservation], error) {
		calls++
		if calls < 3 {
			return MappingOutcome[InstrumentDailyObservation]{}, &domain.Error{Code: domain.ErrProviderUnavailable, Message: "provider is unavailable"}
		}
		return mappedTiingoClose("2026-09-04", "185.25"), nil
	}}
	service, _, _, _ := newSyncFixture(t, tiingo)
	if _, err := service.StartMarketDataSync(context.Background(), SyncRequest{Scope: SyncScopeRepairAll}); err != nil {
		t.Fatal(err)
	}
	snap := waitSyncTerminal(t, service)
	if calls < 3 {
		t.Fatalf("attempts = %d, want 3", calls)
	}
	if snap.CommittedBatches < 1 {
		t.Fatalf("expected persist after retries: %+v", snap)
	}
}

type testHistoryPersist struct {
	repo *sqlite.Repository
}

func (p testHistoryPersist) PersistInstrumentHistory(ctx context.Context, request CommitInstrumentHistoryRequest) (CommitHistoryResult, error) {
	commit := sqlite.InstrumentHistoryCommit{
		HouseholdID: request.HouseholdID, InstrumentID: request.InstrumentID,
		ProviderKey: request.Identity.ProviderKey, ProviderSymbol: request.Identity.ProviderSymbol,
		QuoteCurrency: request.Identity.QuoteCurrency, Market: request.Market,
		Status: string(request.Outcome.Status), Reason: request.Outcome.Reason,
		Adapter: request.Outcome.Batch.Evidence.Adapter, SourcePolicy: SourcePolicyVersion(request.Outcome.Batch.Evidence),
		FetchedAt: request.FetchedAt, NextCheckAt: request.Outcome.Batch.NextCheckAt,
	}
	for _, observation := range request.Outcome.Batch.Observations {
		commit.Observations = append(commit.Observations, sqlite.InstrumentHistoryObservation{
			MarketDate: string(observation.MarketDate), Value: observation.Value, Currency: observation.Currency,
			ValueEffectiveAt: observation.ValueEffectiveAt, ProviderTimestamp: observation.ProviderTimestamp,
			Kind: string(observation.Kind), PriceBasis: string(observation.PriceBasis), TimestampBasis: string(observation.TimestampBasis),
			SplitFactor: observation.SplitFactor, DividendCash: observation.DividendCash,
		})
	}
	for _, rng := range request.Outcome.Batch.VerifiedRanges {
		commit.VerifiedRanges = append(commit.VerifiedRanges, sqlite.DateSpan{Start: string(rng.Start), End: string(rng.End)})
	}
	for _, rng := range request.Outcome.Batch.PendingRanges {
		commit.PendingRanges = append(commit.PendingRanges, sqlite.DateSpan{Start: string(rng.Start), End: string(rng.End)})
	}
	result, err := p.repo.CommitInstrumentHistory(ctx, commit)
	if err != nil {
		return CommitHistoryResult{}, err
	}
	return CommitHistoryResult{PersistedObservations: result.PersistedObservations, NewRevisions: result.NewRevisions, CoverageDays: result.CoverageDays, CanonicalSlots: result.CanonicalSlots, InputGeneration: result.InputGeneration, Unchanged: result.Unchanged}, nil
}

func (p testHistoryPersist) PersistFXHistory(ctx context.Context, request CommitFXHistoryRequest) (CommitHistoryResult, error) {
	commit := sqlite.FXHistoryCommit{
		HouseholdID: request.HouseholdID, ProviderKey: request.Identity.ProviderKey,
		BaseCurrency: request.Identity.BaseCurrency, QuoteCurrency: request.Identity.QuoteCurrency,
		Status: string(request.Outcome.Status), Reason: request.Outcome.Reason,
		Adapter: request.Outcome.Batch.Evidence.Adapter, SourcePolicy: SourcePolicyVersion(request.Outcome.Batch.Evidence),
		FetchedAt: request.FetchedAt, NextCheckAt: request.Outcome.Batch.NextCheckAt,
	}
	for _, observation := range request.Outcome.Batch.Observations {
		commit.Observations = append(commit.Observations, sqlite.FXHistoryObservation{
			MarketDate: string(observation.MarketDate), Rate: observation.Rate,
			BaseCurrency: observation.BaseCurrency, QuoteCurrency: observation.QuoteCurrency,
			ValueEffectiveAt: observation.ValueEffectiveAt, Kind: string(observation.Kind), TimestampBasis: string(observation.TimestampBasis),
		})
	}
	for _, rng := range request.Outcome.Batch.VerifiedRanges {
		commit.VerifiedRanges = append(commit.VerifiedRanges, sqlite.DateSpan{Start: string(rng.Start), End: string(rng.End)})
	}
	result, err := p.repo.CommitFXHistory(ctx, commit)
	if err != nil {
		return CommitHistoryResult{}, err
	}
	return CommitHistoryResult{PersistedObservations: result.PersistedObservations, NewRevisions: result.NewRevisions, CoverageDays: result.CoverageDays, CanonicalSlots: result.CanonicalSlots, InputGeneration: result.InputGeneration, Unchanged: result.Unchanged}, nil
}
