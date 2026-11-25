package desktop

import (
	"context"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/marketdata"
)

type repairTestProvider struct {
	unavailableOverviewProvider
	calls atomic.Int32
	now   time.Time
}

func (*repairTestProvider) Capabilities() application.MarketDataCapabilities {
	return application.MarketDataCapabilities{LatestFX: true}
}

func (p *repairTestProvider) LatestFX(ctx context.Context, identity application.FXMarketIdentity) (application.LatestFXQuote, error) {
	if p.calls.Add(1) == 1 {
		<-ctx.Done()
		return application.LatestFXQuote{}, ctx.Err()
	}
	rate, err := domain.ParseFxRate("140.1234")
	return application.LatestFXQuote{Rate: rate, BaseCurrency: identity.BaseCurrency,
		QuoteCurrency: identity.QuoteCurrency, SourceKey: p.Key(), QuotedAt: p.now}, err
}

func TestDataHealthRepairPreviewCancelAndVerifiedRetry(t *testing.T) {
	ctx := t.Context()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "repair.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	now := time.Date(2026, time.November, 4, 8, 0, 0, 0, time.UTC)
	provider := &repairTestProvider{now: now}
	service := application.NewService(sqlite.NewRepository(db), application.NewMarketDataRegistryWithDefault(provider.Key(), provider))
	service.SetClock(func() time.Time { return now })
	t.Cleanup(service.CancelMarketDataSyncAndWait)
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic repair", BaseCurrency: "USD", MemberNames: []string{"Sample owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateAccount(ctx, application.AccountInput{
		Name: "Synthetic yen", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "JPY", InitialAmount: "100", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetFXPreference(ctx, "JPY", "USD", "provider"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "Asia/Singapore"); err != nil {
		t.Fatal(err)
	}
	ref := marketdata.NewService(service, nil)
	want, err := ref.PreviewMarketDataSync(marketdata.SyncRequestDTO{Scope: string(application.SyncScopeRepairAll)})
	if err != nil || want.EstimatedRequestCount == 0 {
		t.Fatalf("Wails preview = %+v, %v", want, err)
	}
	repair := &dataHealthRepair{service: service}
	service.SetMarketDataSyncListener(func(string, application.SyncJobSnapshot) { repair.revision.Add(1) })
	health := &dataHealthPage{ctx: ctx, reader: service, invalidate: func() {}, repair: repair}
	market := &marketDataPage{ctx: ctx, reader: service, invalidate: func() {}}
	t.Cleanup(market.workers.Wait)
	t.Cleanup(health.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{marketData: market, dataHealth: health})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.shell.trail = newNavigationTrail(dataHealthRoute)
	health.read()
	health.workers.Wait()
	h.frame()

	openPlan := func() {
		t.Helper()
		h.click("Repair all", false)
		health.workers.Wait()
		h.frame()
		if repair.plan == nil || !reflect.DeepEqual(marketdata.SyncPreviewDTO(*repair.plan), want) {
			t.Fatalf("Gio preview = %+v, Wails = %+v", repair.plan, want)
		}
	}
	openPlan()
	if provider.calls.Load() != 0 {
		t.Fatal("the preview contacted a provider")
	}
	h.frame()
	h.press(key.NameEscape)
	h.frame()
	h.focused(&repair.open)
	if _, exists := service.GetCurrentSyncJob(); exists || provider.calls.Load() != 0 {
		t.Fatal("dismissing the plan started a repair")
	}

	openPlan()
	h.click("Start repair", false)
	health.workers.Wait()
	waitGioRepair(t, h, func() bool {
		return repair.job.Outcome == application.SyncOutcomeRunning && provider.calls.Load() == 1
	})
	firstID := repair.job.JobID
	h.click("Cancel repair", false)
	h.frame()
	if repair.cancelRead == nil && repair.job.Outcome == application.SyncOutcomeRunning {
		t.Fatalf("cancel click did not reach the running job: focused=%t stage=%+v", h.router.Source().Focused(&repair.cancel), h.shell.stage.Position)
	}
	health.workers.Wait()
	waitGioRepair(t, h, func() bool {
		return repair.job.Outcome == application.SyncOutcomeCancelled && health.request == nil
	})
	assertPageLabel(t, &h.router, "Repair cancelled")
	if health.report == nil || health.report.ExecutableCount != 1 {
		t.Fatalf("cancelling claimed the gap was resolved: %+v", health.report)
	}

	openPlan()
	h.click("Start repair", false)
	health.workers.Wait()
	waitGioRepair(t, h, func() bool {
		return repair.job.JobID != firstID && repair.job.Outcome != application.SyncOutcomeRunning &&
			repair.job.JobID != "" && health.request == nil
	})
	if repair.job.Outcome != application.SyncOutcomeSucceeded || health.report == nil || health.report.Healthy || health.report.ExecutableCount != 1 {
		t.Fatalf("completed repair = %+v, health = %+v", repair.job, health.report)
	}
	quote, err := service.CurrentFXQuote(ctx, "JPY", "USD")
	if err != nil || quote == nil || quote.Rate.Canonical() != "140.1234" || quote.SourceKind != domain.QuoteSourceProvider {
		t.Fatalf("saved quote = %+v, %v", quote, err)
	}
	if provider.calls.Load() != 2 {
		t.Fatalf("provider calls = %d, want one cancelled and one successful", provider.calls.Load())
	}
	if scan, err := ref.ScanMarketDataHealth(); err != nil || !reflect.DeepEqual(marketdata.HealthReportDTO(*health.report), scan) {
		t.Fatalf("verified report = %+v, Wails = %+v, %v", health.report, scan, err)
	}
}

func waitGioRepair(t *testing.T, h *previewHarness, condition func() bool) {
	t.Helper()
	end := time.Now().Add(4 * time.Second)
	for !condition() && time.Now().Before(end) {
		h.frame()
		time.Sleep(time.Millisecond)
	}
	if !condition() {
		t.Fatalf("repair did not reach expected state: job=%+v report=%+v", h.shell.dataHealth.repair.job, h.shell.dataHealth.report)
	}
}
