package desktop

import (
	"context"
	"image"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/testfixture"
)

type countedHealthReport struct {
	data  application.MarketDataHealthReport
	scans atomic.Int32
}

func (r *countedHealthReport) ScanMarketDataHealth(context.Context) (application.MarketDataHealthReport, error) {
	r.scans.Add(1)
	return r.data, nil
}

func TestDataHealthManualFindingReturnsWithoutRescanUntilSave(t *testing.T) {
	ctx := t.Context()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "focused-manual.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	service := application.NewService(&testfixture.BaselineRepository{Repository: sqlite.NewRepository(db)})
	clock := func() time.Time { return time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC) }
	service.SetClock(clock)
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic focused manual quote", BaseCurrency: "USD", MemberNames: []string{"Sample owner"}}); err != nil {
		t.Fatal(err)
	}
	asset, err := service.CreateInstrument(ctx, application.InstrumentInput{Name: "Synthetic missing price", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	price := application.HealthIssue{ID: "price", Kind: application.HealthKindMissingManualPrice, Action: application.HealthActionManualEntry,
		InstrumentID: asset.ID.String(), Label: asset.Name, RangeStart: "2026-11-10", RangeEnd: "2026-11-24"}
	rate := application.HealthIssue{ID: "rate", Kind: application.HealthKindMissingManualFX, Action: application.HealthActionManualEntry,
		CurrencyA: "JPY", CurrencyB: "USD", Label: "Synthetic exchange rate"}
	reader := &countedHealthReport{data: application.MarketDataHealthReport{Issues: []application.HealthIssue{price, rate}}}
	market := &marketDataPage{ctx: ctx, reader: service, invalidate: func() {}, manual: marketManualQuotes{service: service, clock: clock}}
	health := &dataHealthPage{ctx: ctx, reader: reader, invalidate: func() {}}
	t.Cleanup(market.workers.Wait)
	t.Cleanup(health.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{marketData: market, dataHealth: health})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(700, 480)
	h.frame()

	for _, tt := range []struct {
		issue  application.HealthIssue
		target marketManualTarget
	}{
		{price, marketManualTarget{instrumentID: asset.ID, date: "2026-11-22"}},
		{rate, marketManualTarget{base: "JPY", quote: "USD"}},
	} {
		focus := healthFindingFocus{id: tt.issue.ID, label: tt.issue.Label, from: "2026-11-22", to: "2026-11-24"}
		h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: dataHealthRoute, health: focus}}
		h.frame()
		health.workers.Wait()
		h.frame()
		visit, report, scans := h.shell.trail.current().id, health.report, reader.scans.Load()
		back := semanticBounds(t, &h.router, "Back", false)
		h.click("Add manual quote", false)
		market.workers.Wait()
		h.frame()
		if draft := market.manual.draft; draft == nil || draft.target != tt.target || !draft.available() {
			t.Fatalf("%s opened %+v, want %+v", tt.issue.ID, draft, tt.target)
		}
		checkRetained := func() {
			t.Helper()
			if market.manual.draft != nil || health.focus != focus || h.shell.trail.current().id != visit || health.report != report || reader.scans.Load() != scans {
				t.Fatal("closing the quote form replaced or rechecked the focused finding")
			}
			h.focused(health.issueActions[tt.issue.ID])
		}
		market.manual.draft.amount.Editor.SetText("12.34")
		h.press(key.NameEscape)
		h.frame()
		checkRetained()
		h.press(key.NameReturn)
		market.workers.Wait()
		h.frame()
		// The toolbar is behind the modal. Back first dismisses the modal.
		h.clickAt(back.Min.Add(back.Max).Div(2))
		h.frame()
		checkRetained()
	}

	if q, err := service.CurrentInstrumentQuote(ctx, asset.ID); err != nil || q != nil {
		t.Fatalf("cancel saved a price = %+v, %v", q, err)
	}
	scans := reader.scans.Load()
	h.press(key.NameReturn)
	market.workers.Wait()
	h.frame()
	market.manual.draft.amount.Editor.SetText("0.0067")
	h.frame()
	h.click("Save manual FX quote", false)
	market.workers.Wait()
	h.frame()
	health.workers.Wait()
	h.frame()
	if reader.scans.Load() != scans+1 || market.manual.draft != nil || !health.manualSaved.saved() {
		t.Fatal("a saved FX quote did not recheck precisely once")
	}
	h.focused(health.issueActions[rate.ID])
}
