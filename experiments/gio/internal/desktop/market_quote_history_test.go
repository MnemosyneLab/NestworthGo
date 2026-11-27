package desktop

import (
	"context"
	"errors"
	"image"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/testfixture"
	quoteapi "github.com/waltwang/nestworth-go/internal/wailsapi/quote"
)

type marketHistoryFailOnce struct {
	*application.Service
	fxCalls atomic.Int32
}

func TestMarketQuoteHistoryTableRetainsUnplottedCurrency(t *testing.T) {
	observation := domain.QuoteSeriesPoint{Value: "12.34567891", Currency: "EUR", EffectiveDate: "2026-11-20"}
	history := &marketQuoteHistory{result: &marketQuoteHistoryRead{location: time.UTC, series: domain.QuoteSeries{
		Range: domain.TrendAllTime, DisplayCurrency: "USD", Points: []domain.QuoteSeriesPoint{observation},
		Observations: []domain.QuoteSeriesPoint{observation},
	}}}
	h := newShellHarness(t)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.shell.trail = newNavigationTrail(componentPreviewRoute)
	h.shell.routes[componentPreviewRoute].body = func(gtx layout.Context, view ui.View) layout.Dimensions {
		return quoteHistoryChart(view, history)(gtx)
	}
	h.frame()
	h.click("View data table", false)
	h.frame()
	assertPageLabel(t, &h.router, "EUR 12.34567891")
}

func (r *marketHistoryFailOnce) FXQuoteSeries(ctx context.Context, a, b domain.CurrencyCode, window domain.TrendRange, filter domain.QuoteSourceFilter) (domain.QuoteSeries, error) {
	if r.fxCalls.Add(1) == 1 {
		return domain.QuoteSeries{}, errors.New("temporary local read error")
	}
	return r.Service.FXQuoteSeries(ctx, a, b, window, filter)
}

func TestMarketQuoteHistoryThroughMarketAndFocusedHealth(t *testing.T) {
	ctx := t.Context()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "quote-history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := testfixture.Repository(db)
	service := testfixture.NewService(&testfixture.BaselineRepository{Repository: repo})
	clock := func() time.Time { return time.Date(2026, 11, 26, 3, 0, 0, 0, time.UTC) }
	testfixture.SetClock(service, clock)
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic quote history", BaseCurrency: "USD", MemberNames: []string{"Sample owner"}}); err != nil {
		t.Fatal(err)
	}
	asset, err := testfixture.CreateInstrument(service, ctx, application.InstrumentInput{
		Name: "Synthetic archive fund", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, asset.ID, "12.34567891", "2026-07-20T18:00:00Z", true); err != nil {
		t.Fatal(err)
	}
	if _, err := testfixture.SetFXPreference(service, ctx, "SGD", "USD", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualFXQuote(ctx, "USD", "SGD", "7", "2026-11-20T18:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := testfixture.CaptureSyntheticOrigin(service, ctx, "America/Los_Angeles"); err != nil {
		t.Fatal(err)
	}
	fixture := application.HealthIssue{ID: "fixture-fx", Kind: application.HealthKindMissingFXHistory, CurrencyA: "SGD",
		CurrencyB: "USD", Label: "SGD/USD", RangeStart: "2026-11-19", RangeEnd: "2026-11-21"}
	healthReader := &countedHealthReport{data: application.MarketDataHealthReport{Issues: []application.HealthIssue{fixture}}}
	reader := &marketHistoryFailOnce{Service: service}
	market := &marketDataPage{ctx: ctx, reader: service, quoteHistoryReader: reader, quoteHistoryClock: clock, invalidate: func() {}}
	health := &dataHealthPage{ctx: ctx, reader: healthReader, invalidate: func() {}}
	t.Cleanup(market.workers.Wait)
	t.Cleanup(health.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{marketData: market, dataHealth: health})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.shell.trail = newNavigationTrail(marketDataRoute)
	market.enter(marketDataVisit{search: "archive"})
	market.workers.Wait()
	h.frame()
	wait := func() {
		t.Helper()
		market.workers.Wait()
		h.frame()
	}
	wails := quoteapi.NewService(service)
	version := service.MobileDataVersion()

	h.click("View quote history", false)
	wait()
	d := market.quoteHistory
	if d == nil || d.result == nil || d.visit.window != (analysisDateWindow{"2026-10-26", "2026-11-25"}) ||
		!d.result.series.OutsideRange || d.result.current == nil || d.result.current.Value != "12.34567891" {
		t.Fatalf("instrument window or selected quote was wrong: %+v", d)
	}
	assertPageLabel(t, &h.router, "No observations in this date range.")
	h.click("Show all dates", false)
	wait()
	got := d.result.series
	want, err := wails.InstrumentQuoteSeries(ctx, asset.ID.String(), "all", "all")
	if err != nil || len(got.Observations) != 1 || len(want.Observations) != 1 ||
		got.Observations[0].Value != want.Observations[0].Value || !got.Observations[0].Delayed || !want.Observations[0].Delayed {
		t.Fatalf("local instrument evidence = %+v, Wails = %+v, err=%v", got, want, err)
	}
	h.size = image.Pt(640, 480)
	h.frame()
	d.sheet.list.ScrollTo(1)
	h.frame()
	h.click("View data table", false)
	h.frame()
	if !d.table.open {
		t.Fatal("the quote chart's data table did not open")
	}
	assertPageLabel(t, &h.router, "USD 12.34567891")
	h.press(key.NameHome)
	h.size = image.Pt(1120, 840)
	h.frame()
	h.click("Agent quote", false)
	wait()
	assertPageLabel(t, &h.router, "No saved observations for this source.")
	h.press(key.NameEscape)
	h.frame()
	h.focused(d.visit.source)
	if match := market.search.Field.Text(); match != "archive" {
		t.Fatalf("search changed to %q on close", match)
	}

	h.click("FX rates", false)
	wait()
	h.click("View quote history", false)
	wait()
	if market.quoteHistory.err == nil {
		t.Fatal("local retry fixture did not fail")
	}
	h.click("Retry", false)
	wait()
	d = market.quoteHistory
	if d.result == nil || len(d.result.series.Observations) != 1 ||
		d.result.series.Observations[0].Value != "0.142857142857" || d.result.storedPair != (marketFXPair{"USD", "SGD"}) ||
		d.result.current == nil || d.result.current.Value != "7" {
		t.Fatalf("historical and selected FX evidence were conflated: %+v", d)
	}
	h.click("Swap direction", false)
	wait()
	want, err = wails.FXQuoteSeries(ctx, "USD", "SGD", string(d.result.series.Range), "all")
	if err != nil || len(want.Observations) != 1 || d.result.series.Observations[0].Value != want.Observations[0].Value ||
		d.result.series.Observations[0].Value != "7" || d.result.observedDays != 1 {
		t.Fatalf("wrong directed FX read: %+v, Wails = %+v, %v", d.result, want, err)
	}
	h.click("Manual quote", false)
	wait()
	h.click("View data health", false)
	h.frame()
	health.workers.Wait()
	h.frame()
	if health.focus.currencyA != "SGD" || health.focus.currencyB != "USD" || health.focus.from != d.visit.window.from ||
		len(health.findings) != 1 || health.findings[0].issue.ID != fixture.ID {
		t.Fatalf("wrong Data Health selection: %+v, findings=%+v", health.focus, health.findings)
	}
	h.click("View quote history", false)
	wait()
	finding := market.quoteHistory
	if finding.result == nil || finding.visit.target.finding.id != fixture.ID ||
		finding.visit.window != (analysisDateWindow{"2026-11-19", "2026-11-21"}) {
		t.Fatalf("opening a finding lost the issue or dates: %+v", finding)
	}
	scans := healthReader.scans.Load()
	h.size = image.Pt(640, 480)
	h.frame()
	h.press(key.NameEscape)
	h.frame()
	h.focused(finding.visit.source)
	if healthReader.scans.Load() != scans || h.shell.activeRoute() != dataHealthRoute {
		t.Fatal("closing history changed or rechecked the finding")
	}
	h.click("Back", false)
	wait()
	restored := market.quoteHistory
	if restored == nil || restored.result == nil || restored.visit.target != d.visit.target || restored.visit.window != d.visit.window ||
		restored.visit.filter != d.visit.filter || !restored.visit.reverse || market.tab != marketFX {
		t.Fatalf("Back did not restore the selected series: %+v", restored)
	}
	if version != service.MobileDataVersion() {
		t.Fatal("reading the quote history changed the ledger")
	}
}

func TestMarketQuoteHistoryChartKeepsEvidenceAndLatestPointsDistinct(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	agent := domain.QuoteSeriesPoint{ID: "nav", Currency: "USD", Value: "12.34567891", SourceKind: domain.QuoteSourceAgent,
		ObservationKind: "close", PriceBasis: "agent_unit_nav_v1", TimestampBasis: "date_label", EffectiveDate: "2026-11-20", QuotedAt: at}
	latest := domain.QuoteSeriesPoint{ID: "one", Currency: "USD", Value: "15.12345678", SourceKind: domain.QuoteSourceProvider,
		SourceKey: "yahoo_finance", ObservationKind: "realtime", QuotedAt: at.Add(9 * time.Hour), Delayed: true}
	older := latest
	older.ID, older.QuotedAt = "two", latest.QuotedAt.Add(time.Hour)
	alternate := agent
	alternate.ID, alternate.Currency = "excluded", "JPY"
	read := &marketQuoteHistoryRead{location: loc, window: analysisDateWindow{"2026-11-20", "2026-11-20"},
		series: domain.QuoteSeries{DisplayCurrency: "USD", Range: "2026-11-20:2026-11-20", Points: []domain.QuoteSeriesPoint{older, latest, alternate, agent}}}
	view := ui.View{Language: i18n.English, TimeLocation: loc, DecimalSeparator: settings.DecimalComma}
	spec := quoteHistoryLine(view, read, marketFXPair{})
	if len(spec.Series) != 2 || len(spec.Series[0].Points) != 2 || len(spec.Series[1].Points) != 1 {
		t.Fatalf("unexpected chart series: %+v", spec.Series)
	}
	a, b := spec.Series[0].Points[0], spec.Series[0].Points[1]
	if a.ID != "one" || b.ID != "two" || !a.BreakBefore || !b.BreakBefore ||
		a.Text != "USD 15,12345678" || !strings.Contains(strings.Join(a.Detail, " "), "Delayed") {
		t.Fatalf("latest and exact provenance disagree: %+v, %+v", a, b)
	}
	nav := spec.Series[1].Points[0]
	if nav.ID != "nav" || !nav.Time.Equal(spec.From) || nav.Text != "USD 12,34567891" ||
		len(nav.Detail) != 2 || strings.Contains(strings.Join(nav.Detail, " "), "Quoted at") {
		t.Fatalf("civil NAV plotted as an observed timestamp: %+v", nav)
	}
}
