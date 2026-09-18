package application

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestRepairPreviewAndExecutionSkipRecentLatest(t *testing.T) {
	ctx := context.Background()
	provider := &syncFakeProvider{key: YahooFinanceProviderKey, now: time.Date(2026, 9, 9, 16, 0, 0, 0, time.UTC)}
	service, _, _, instrument := newSyncFixture(t, &syncFakeProvider{key: TiingoProviderKey}, provider)
	if _, err := service.RefreshInstrument(ctx, instrument.ID); err != nil {
		t.Fatal(err)
	}
	request := SyncRequest{Scope: SyncScopeInstrument, InstrumentID: instrument.ID.String()}
	preview, err := service.PreviewMarketDataSync(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if preview.LatestInstrumentCount != 0 {
		t.Fatalf("latest=%d", preview.LatestInstrumentCount)
	}
	cached := false
	for _, item := range preview.Items {
		if item.Kind == "latest_instrument" && item.Status == "cached" && item.Label == instrument.Name {
			cached = true
		}
	}
	if !cached {
		t.Fatalf("missing named cache decision: %+v", preview.Items)
	}
	targets, err := service.syncLatestTargets(ctx, request)
	if err != nil || len(targets) != 1 || !targets[0].skip {
		t.Fatalf("targets=%+v err=%v", targets, err)
	}
	request.ForceRecheck = true
	forced, err := service.PreviewMarketDataSync(ctx, request)
	if err != nil || forced.LatestInstrumentCount != 1 {
		t.Fatalf("force=%+v %v", forced, err)
	}
}

func TestLatestRecheckPersistsAcrossRestartWithoutChangingObservation(t *testing.T) {
	_, service, fake, _, instrument := newRefreshFixture(t)
	ctx := context.Background()
	now := service.clock()
	if _, err := service.RefreshAll(ctx); err != nil {
		t.Fatal(err)
	}
	now = now.Add(13 * time.Hour)
	service.setClock(func() time.Time { return now })
	if _, err := service.RefreshAll(ctx); err != nil {
		t.Fatal(err)
	} // unchanged observations, new successful check
	fake.mu.Lock()
	fake.calls = nil
	fake.mu.Unlock()
	restarted := NewService(service.repository, service.MarketDataRegistry())
	restarted.setClock(func() time.Time { return now.Add(time.Minute) })
	restarted.SetFXProvider(service.FXProviderKey())
	result, err := restarted.RefreshMissingOrStale(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertRefreshStatus(t, result, instrumentTargetKey(instrument.ID), RefreshCached)
	if fake.callCount() != 0 {
		t.Fatalf("restart re-fetched unchanged observations: %v", fake.callNames())
	}
}

func TestHistoryKeepsSevenDayLeadEvenWithOpeningAnchor(t *testing.T) {
	need, err := planInstrumentRepairNeed(domain.InstrumentHistoryCoverage{ProviderKey: YahooFinanceProviderKey, ProviderSymbol: "QQQM", Market: "US", CloseMarketDates: []string{"2026-09-17"}}, "2026-09-18", "2026-09-17")
	if err != nil {
		t.Fatal(err)
	}
	if need.FetchRange.Start != "2026-09-11" || need.OpeningAnchorMissing {
		t.Fatalf("need=%+v", need)
	}
}

func TestOldSyncFailureIsNotCurrentHealthIssue(t *testing.T) {
	service, _, _, instrument := newSyncFixture(t, &syncFakeProvider{key: TiingoProviderKey}, &syncFakeProvider{key: YahooFinanceProviderKey})
	service.currentSyncID = "old"
	service.syncJobs["old"] = &syncJobState{snapshot: SyncJobSnapshot{Outcome: SyncOutcomePartial, Blockers: []SyncBlocker{{TargetKey: instrumentTargetKey(instrument.ID), Code: string(domain.ErrProviderAuthentication)}}}}
	report, err := service.ScanMarketDataHealth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range report.Issues {
		if issue.Kind == HealthKindSyncFailure {
			t.Fatalf("old error is current health: %+v", issue)
		}
	}
}

func TestRefreshPublishesNamedRunningAndCompletedItems(t *testing.T) {
	_, service, _, _, instrument := newRefreshFixture(t)
	var events []SyncItemProgress
	ctx := WithRefreshProgress(context.Background(), func(item SyncItemProgress) { events = append(events, item) })
	if _, err := service.RefreshInstrument(ctx, instrument.ID); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Status != "running" || events[1].Status != "fetched" || events[0].Label != instrument.Name || events[0].Symbol == "" {
		t.Fatalf("events=%+v", events)
	}
}
