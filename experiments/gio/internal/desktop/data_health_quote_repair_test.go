package desktop

import (
	"reflect"
	"testing"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/marketdata"
)

func TestDataHealthScopedQuoteRepairRetainsFindingAndRetryScope(t *testing.T) {
	provider := &refreshTestProvider{cancelFirstHistory: true}
	_, market, service, wails := newMarketOperationsHarness(t, provider, true)
	issue := application.HealthIssue{ID: "fx-gap", Kind: application.HealthKindMissingFXHistory, Executable: true,
		Action: application.HealthActionRepair, RangeStart: "2026-11-03", RangeEnd: "2026-11-04", CurrencyA: "JPY", CurrencyB: "USD", Label: "JPY/USD"}
	reader := &countedHealthReport{data: application.MarketDataHealthReport{Issues: []application.HealthIssue{issue}}}
	health := &dataHealthPage{ctx: market.ctx, reader: reader, invalidate: func() {}, repair: market.sync}
	t.Cleanup(health.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{marketData: market, dataHealth: health})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.shell.trail = newNavigationTrail(dataHealthRoute)
	focus := healthFindingFocus{id: issue.ID, currencyA: issue.CurrencyA, currencyB: issue.CurrencyB, from: issue.RangeStart, to: issue.RangeEnd}
	health.enter(health.list.Position, focus)
	health.workers.Wait()
	h.frame()
	h.click("Preview historical repair", false)
	waitMarketOperations(t, h, func() bool { return market.sync.plan != nil })
	source := market.operation.source
	request := application.SyncRequest{Scope: application.SyncScopeFX, CurrencyA: "JPY", CurrencyB: "USD"}
	reference, err := wails.PreviewMarketDataSync(marketdata.SyncRequestDTO{Scope: string(request.Scope), CurrencyA: request.CurrencyA, CurrencyB: request.CurrencyB})
	if err != nil || !reflect.DeepEqual(marketSyncEvidence(*market.sync.plan), reference) {
		t.Fatalf("Gio plan = %+v, Wails = %+v, %v", market.sync.plan, reference, err)
	}
	h.press(key.NameEscape)
	h.frame()
	h.focused(source)
	if health.focus != focus || reader.scans.Load() != 1 || provider.historyCalls.Load() != 0 || provider.latestCalls.Load() != 0 {
		t.Fatal("dismissal changed selection, scanned, or made a provider request")
	}
	h.press(key.NameReturn)
	waitMarketOperations(t, h, func() bool { return market.sync.plan != nil })
	h.click("Start sync", false)
	waitMarketOperations(t, h, func() bool {
		return market.sync.job.Outcome == application.SyncOutcomeRunning && market.sync.job.Current != nil && provider.historyCalls.Load() == 1
	})
	h.click("Historical sync details", false)
	h.frame()
	if market.operation == nil || market.operation.route != dataHealthRoute || market.operation.kind != marketSyncDetails {
		t.Fatal("Data Health did not open progress for the shared job")
	}
	h.press(key.NameEscape)
	h.frame()
	h.focused(&health.repairDetails)
	h.click("Cancel repair", false)
	waitMarketOperations(t, h, func() bool {
		return market.sync.job.Outcome == application.SyncOutcomeCancelled && health.request == nil && reader.scans.Load() == 2
	})
	if health.focus != focus {
		t.Fatal("cancellation changed the finding")
	}
	h.click("Retry sync", false)
	waitMarketOperations(t, h, func() bool { return market.sync.plan != nil })
	if market.sync.plan.Scope != request || reader.scans.Load() != 2 {
		t.Fatalf("retry preview changed scope or scanned: %+v", market.sync.plan)
	}
	h.click("Start sync", false)
	waitMarketOperations(t, h, func() bool {
		return market.sync.job.Outcome == application.SyncOutcomePartial && health.request == nil && reader.scans.Load() == 3
	})
	quotes, err := service.FXQuoteHistory(t.Context())
	if err != nil || len(quotes) == 0 || market.sync.job.CommittedBatches != 1 {
		t.Fatalf("scope did not save qualified observations: %+v, %v", quotes, err)
	}
	for _, quote := range quotes {
		if quote.BaseCurrency != "JPY" || quote.QuoteCurrency != "USD" {
			t.Fatalf("scoped repair wrote to another pair: %+v", quote)
		}
	}
}
