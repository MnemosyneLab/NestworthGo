package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReviewedMarketDataSyncPinsWorkThroughBackgroundStart(t *testing.T) {
	t.Parallel()
	type fxRequest struct {
		identity FXMarketIdentity
		dates    DateRange
	}
	var received []fxRequest
	original := &syncFakeProvider{key: FrankfurterProviderKey, fx: func(_ context.Context, identity FXMarketIdentity, dates DateRange) (MappingOutcome[FXDailyObservation], error) {
		received = append(received, fxRequest{identity: identity, dates: dates})
		return mappedFXRate(string(dates.Start), "1.234567"), nil
	}}
	replacement := &syncFakeProvider{key: "alternate", fx: original.fx}
	equity := &syncFakeProvider{key: YahooFinanceProviderKey}
	crypto := &syncFakeProvider{key: CoinGeckoProviderKey}
	service, _, _, _ := newSyncFixture(t, original, replacement, equity, crypto)
	ctx := t.Context()
	if err := service.SetFXProvider(original.Key()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetFXPreference(ctx, "USD", "SGD", "provider"); err != nil {
		t.Fatal(err)
	}
	request := SyncRequest{Scope: SyncScopeRepairAll}
	review, err := service.ReviewMarketDataSync(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	var approved []DateRange
	for _, item := range review.Items {
		if item.Kind == "history_fx" {
			approved = append(approved, DateRange{Start: MarketDate(item.StartDate), End: MarketDate(item.EndDate)})
		}
	}
	if len(approved) == 0 || review.ReviewID == "" {
		t.Fatalf("missing approved FX ranges: %+v", review)
	}
	checkStale := func(scope SyncRequest) {
		t.Helper()
		start, err := service.ConfirmMarketDataSync(ctx, scope, review.ReviewID)
		var problem *domain.Error
		if !errors.As(err, &problem) || problem.Code != domain.ErrStalePreview || start.Job.JobID != "" {
			t.Fatalf("unexpected stale confirmation = %+v, %v", start, err)
		}
	}
	checkStale(SyncRequest{Scope: SyncScopeRepairAll, ForceRecheck: true})
	if len(received) != 0 {
		t.Fatal("mismatched review reached provider")
	}

	var writeErr error
	nextDay := service.clock().AddDate(0, 0, 1)
	service.SetMarketDataSyncListener(func(event string, _ SyncJobSnapshot) {
		if event == SyncEventStarted {
			// Change eligible pairs, selected provider and the closed-day
			// boundary after admission but before the background worker runs.
			service.SetClock(func() time.Time { return nextDay })
			_, prefErr := service.SetFXPreference(ctx, "JPY", "SGD", "provider")
			writeErr = errors.Join(prefErr, service.SetFXProvider(replacement.Key()))
		}
	})
	start, err := service.ConfirmMarketDataSync(ctx, request, review.ReviewID)
	if err != nil || writeErr != nil || start.Job.PlanRevision != review.ReviewID {
		t.Fatalf("confirmed review = %+v, %v, source setup = %v", start, err, writeErr)
	}
	job := waitSyncTerminal(t, service)
	service.syncWG.Wait()
	if job.PlanRevision != review.ReviewID {
		t.Fatalf("worker changed approved plan identity: %s", job.PlanRevision)
	}
	checkStale(request)
	var fetched []DateRange
	for _, request := range received {
		if request.identity.ProviderKey != original.Key() ||
			fxPairKey(request.identity.BaseCurrency, request.identity.QuoteCurrency) != "SGD/USD" {
			t.Fatalf("provider received an unapproved target: %+v", request)
		}
		fetched = append(fetched, request.dates)
	}
	if !reflect.DeepEqual(fetched, approved) {
		t.Fatalf("worker fetched %+v, approved %+v", fetched, approved)
	}
	quotes, err := service.FXQuoteHistory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range quotes {
		if q.SourceKey == replacement.Key() || q.BaseCurrency == "JPY" || q.QuoteCurrency == "JPY" {
			t.Fatalf("worker stored unapproved FX quote: %+v", q)
		}
	}
}

func TestReviewedMarketDataSyncConsumesApprovalOnce(t *testing.T) {
	t.Parallel()
	service, _, instrument, _ := newSyncFixture(t, &syncFakeProvider{key: YahooFinanceProviderKey})
	request := SyncRequest{Scope: SyncScopeInstrument, InstrumentID: instrument.ID.String()}
	review, err := service.ReviewMarketDataSync(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	type confirmation struct {
		result SyncStartResult
		err    error
	}
	responses := make(chan confirmation, 2)
	err = service.WithRead(t.Context(), func(ctx context.Context) error {
		// Even callers sharing an outer operation permit must consume the
		// approval only once. Neither can change admission out of band.
		for range 2 {
			go func() {
				result, err := service.ConfirmMarketDataSync(ctx, request, review.ReviewID)
				responses <- confirmation{result: result, err: err}
			}()
		}
		accepted := 0
		for range 2 {
			confirmed := <-responses
			var problem *domain.Error
			switch {
			case confirmed.err == nil && confirmed.result.Job.JobID != "" && !confirmed.result.Attached:
				accepted++
			case errors.As(confirmed.err, &problem) && problem.Code == domain.ErrStalePreview:
			default:
				t.Errorf("unexpected confirmation: %+v, %v", confirmed.result, confirmed.err)
			}
		}
		if accepted != 1 {
			t.Errorf("accepted %d confirmations, want one", accepted)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	service.syncWG.Wait()
}

type syncBlockingLatestProvider struct {
	*syncFakeProvider
	requests chan InstrumentMarketIdentity
	release  chan struct{}
}

func (provider *syncBlockingLatestProvider) LatestInstrument(ctx context.Context, identity InstrumentMarketIdentity) (LatestInstrumentQuote, error) {
	provider.requests <- identity
	select {
	case <-ctx.Done():
		return LatestInstrumentQuote{}, ctx.Err()
	case <-provider.release:
		return provider.syncFakeProvider.LatestInstrument(ctx, identity)
	}
}

func (provider *syncBlockingLatestProvider) awaitRequest(t *testing.T, symbol string) {
	t.Helper()
	select {
	case identity := <-provider.requests:
		if identity.ProviderSymbol != symbol {
			t.Fatalf("queried %s, want %s", identity.ProviderSymbol, symbol)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("sync did not query the provider")
	}
}

func TestReviewedMarketDataSyncRejectsLatestWithChangedBinding(t *testing.T) {
	t.Parallel()
	provider := &syncBlockingLatestProvider{syncFakeProvider: &syncFakeProvider{key: YahooFinanceProviderKey},
		requests: make(chan InstrumentMarketIdentity, 4), release: make(chan struct{})}
	service, repo, instrument, _ := newSyncFixture(t, provider)
	provider.now = service.clock()
	t.Cleanup(service.CancelMarketDataSyncAndWait)
	ctx := t.Context()
	request := SyncRequest{Scope: SyncScopeInstrument, InstrumentID: instrument.ID.String()}
	review, err := service.ReviewMarketDataSync(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmMarketDataSync(ctx, request, review.ReviewID); err != nil {
		t.Fatal(err)
	}
	provider.awaitRequest(t, "AAPL")
	if _, err := service.UpdateInstrument(ctx, instrument.ID, InstrumentInput{ProviderSymbol: "MSFT"}); err != nil {
		t.Fatal(err)
	}
	close(provider.release)
	job := waitSyncTerminal(t, service)
	quotes, err := repo.ListInstrumentQuotes(ctx, instrument.ID)
	if err != nil || len(quotes) != 0 {
		t.Fatalf("old binding's response was stored: %+v, %v", quotes, err)
	}
	hasConflict := false
	for _, blocker := range job.Blockers {
		if blocker.Code == string(domain.ErrStalePreview) && blocker.TargetKey == instrumentTargetKey(instrument.ID) {
			hasConflict = true
		}
	}
	if !hasConflict {
		t.Fatalf("job did not report the changed quote binding: %+v", job)
	}
	review, err = service.ReviewMarketDataSync(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmMarketDataSync(ctx, request, review.ReviewID); err != nil {
		t.Fatal(err)
	}
	provider.awaitRequest(t, "MSFT")
	waitSyncTerminal(t, service)
	quotes, err = repo.ListInstrumentQuotes(ctx, instrument.ID)
	if err != nil || len(quotes) != 1 || quotes[0].SourceKey != provider.Key() {
		t.Fatalf("fresh binding did not store its provider result: %+v, %v", quotes, err)
	}
}

func TestReviewedMarketDataSyncDoesNotAttachToSameScopeLegacyJob(t *testing.T) {
	t.Parallel()
	provider := &syncBlockingLatestProvider{syncFakeProvider: &syncFakeProvider{key: YahooFinanceProviderKey},
		requests: make(chan InstrumentMarketIdentity, 4), release: make(chan struct{})}
	service, _, instrument, _ := newSyncFixture(t, provider)
	provider.now = service.clock()
	t.Cleanup(service.CancelMarketDataSyncAndWait)
	ctx := t.Context()
	request := SyncRequest{Scope: SyncScopeInstrument, InstrumentID: instrument.ID.String()}
	review, err := service.ReviewMarketDataSync(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := service.StartMarketDataSync(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	provider.awaitRequest(t, "AAPL")
	conflict, err := service.ConfirmMarketDataSync(ctx, request, review.ReviewID)
	if err != nil || !conflict.Conflict || conflict.Attached || conflict.Job.JobID != legacy.Job.JobID {
		t.Fatalf("reviewed confirmation reused a different plan: %+v, %v", conflict, err)
	}
	service.CancelSyncJob(legacy.Job.JobID)
	accepted, err := service.ConfirmMarketDataSync(ctx, request, review.ReviewID)
	if err != nil || accepted.Conflict || accepted.Attached || accepted.Job.PlanRevision != review.ReviewID {
		t.Fatalf("conflict consumed the valid approval: %+v, %v", accepted, err)
	}
	provider.awaitRequest(t, "AAPL")
	service.CancelSyncJob(accepted.Job.JobID)
}
