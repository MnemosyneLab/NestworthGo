package desktop

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/marketdata"
)

type retryHealthRead struct {
	*application.Service
	fail bool
}

func (r *retryHealthRead) ScanMarketDataHealth(ctx context.Context) (application.MarketDataHealthReport, error) {
	if r.fail {
		r.fail = false
		return application.MarketDataHealthReport{}, errors.New("synthetic health read error")
	}
	return r.Service.ScanMarketDataHealth(ctx)
}

func TestDataHealthRecheckMatchesWails(t *testing.T) {
	ctx := t.Context()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "health.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	provider := &unavailableOverviewProvider{}
	service := application.NewService(sqlite.NewRepository(db), application.NewMarketDataRegistry(provider))
	now := time.Date(2026, time.November, 4, 8, 0, 0, 0, time.UTC)
	service.SetClock(func() time.Time { return now })
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic Data Health", BaseCurrency: "USD", MemberNames: []string{"Sample owner"}}); err != nil {
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
	if _, err := service.SetFXPreference(ctx, "JPY", "USD", "manual"); err != nil {
		t.Fatal(err)
	}
	market := &marketDataPage{ctx: ctx, reader: service, invalidate: func() {}}
	health := &dataHealthPage{ctx: ctx, reader: &retryHealthRead{Service: service, fail: true}, invalidate: func() {}}
	t.Cleanup(market.workers.Wait)
	t.Cleanup(health.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{marketData: market, dataHealth: health})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	h.click("Market data", false)
	market.workers.Wait()
	h.frame()
	h.click("View data health", false)
	h.frame()
	health.workers.Wait()
	h.frame()
	if h.shell.activeRoute() != dataHealthRoute || health.err == nil {
		t.Fatal("Data Health did not show the failed scan")
	}
	h.click("Retry", false)
	health.workers.Wait()
	h.frame()

	ref := marketdata.NewService(service, nil)
	want, err := ref.ScanMarketDataHealth()
	if err != nil || want.Healthy || want.IssueCount != 1 || want.PrerequisiteCount != 1 {
		t.Fatalf("Wails FX prerequisite = %+v, %v", want, err)
	}
	if health.report == nil || !reflect.DeepEqual(marketdata.HealthReportDTO(*health.report), want) {
		t.Fatalf("Gio health report = %+v, Wails = %+v", health.report, want)
	}
	assertPageLabel(t, &h.router, "Prerequisites")
	assertPageLabel(t, &h.router, "Missing initial manual FX rate · 1")
	if _, err := service.AppendManualFXQuote(ctx, "USD", "JPY", "140.1234", now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	h.click("Recheck", false)
	health.workers.Wait()
	h.frame()
	want, err = ref.ScanMarketDataHealth()
	if err != nil || !want.Healthy || want.IssueCount != 0 || health.report == nil || !reflect.DeepEqual(marketdata.HealthReportDTO(*health.report), want) {
		t.Fatalf("rechecked Gio report = %+v, Wails = %+v, %v", health.report, want, err)
	}
	assertPageLabel(t, &h.router, "No outstanding gaps")
	h.click("Back", false)
	market.workers.Wait()
	h.frame()
	if h.shell.activeRoute() != marketDataRoute {
		t.Fatal("Back did not return to Market data")
	}
	h.focused(&market.health)
	if provider.requests != 0 {
		t.Fatalf("local checks contacted a provider %d times", provider.requests)
	}
}

type fixedDataHealthRead application.MarketDataHealthReport

func (r fixedDataHealthRead) ScanMarketDataHealth(context.Context) (application.MarketDataHealthReport, error) {
	return application.MarketDataHealthReport(r), nil
}

func TestDataHealthOpensProviderSettingsAndRestoresFocus(t *testing.T) {
	ctx := t.Context()
	issue := application.HealthIssue{
		ID: "synthetic-provider", Kind: application.HealthKindMissingProviderKey, Severity: application.HealthSeverityBlocking,
		Label: "CoinGecko", Action: application.HealthActionProviderSettings,
	}
	health := &dataHealthPage{ctx: ctx, reader: fixedDataHealthRead{Issues: []application.HealthIssue{
		issue, {ID: "collapsed-snapshot", Kind: application.HealthKindSnapshotMissing, Collapsed: true},
	}, IssueCount: 1, PrerequisiteCount: 1}, invalidate: func() {}}
	t.Cleanup(health.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{marketData: &marketDataPage{}, dataHealth: health})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.shell.trail = newNavigationTrail(dataHealthRoute)
	health.read()
	health.workers.Wait()
	h.frame()
	if len(healthIssueRows(health.report.Issues)) != 1 {
		t.Fatal("collapsed prerequisite details escaped into the global findings")
	}
	h.click("Open provider settings", false)
	h.frame()
	if h.shell.activeRoute() != settingsRoute || h.shell.settingsPage.active != settingsMarket {
		t.Fatal("provider link did not open Settings on the Market data tab")
	}
	h.click("Back", false)
	health.workers.Wait()
	h.frame()
	h.focused(health.issueActions[issue.ID])
}
