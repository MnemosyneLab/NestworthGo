package desktop

import (
	"context"
	"image"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type overviewHistoryFixture struct {
	overviewReader
	trend domain.NetWorthTrend
}

func (f overviewHistoryFixture) NetWorthTrend(_ context.Context, period domain.TrendRange) (domain.NetWorthTrend, error) {
	data := f.trend
	data.Range = period
	return data, nil
}

func TestOverviewLinksNetWorthHistoryAndExactCoverageScope(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	now := time.Date(2026, 11, 25, 16, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	valued := domain.HouseholdDisplayValuation{
		NetWorth: domain.ValuationAmount{Total: &domain.MoneyView{Amount: "110.25", Currency: "USD"},
			Quality: domain.ValuationQuality{Availability: domain.ValuationFull}},
	}
	partial := domain.HouseholdDisplayValuation{
		NetWorth: domain.ValuationAmount{KnownSubtotal: &domain.MoneyView{Amount: "110.25", Currency: "USD"},
			Quality: domain.ValuationQuality{Availability: domain.ValuationPartial}},
	}
	fixture := overviewHistoryFixture{
		overviewReader: overviewReaderFunc(func(context.Context, domain.AccountFilter) (domain.OverviewResult, error) {
			return domain.OverviewResult{Currency: "USD", AccountCount: 2, HistoryStarted: true, DisplayValuation: partial,
				MissingInputs: []domain.MissingInputView{{Kind: domain.MissingFXRate, BaseCurrency: "USD", QuoteCurrency: "AUD", AccountName: "Synthetic Australian cash"}}}, nil
		}),
		trend: domain.NetWorthTrend{Currency: "USD", StartDate: "2026-11-23", EndDate: "2026-11-25",
			Points: []domain.NetWorthTrendPoint{
				{LocalDate: "2026-11-23", Complete: true, DisplayValuation: valued},
				{LocalDate: "2026-11-24", DisplayValuation: partial, MissingCount: 1},
				{LocalDate: "2026-11-25", Current: true, DisplayValuation: partial, MissingCount: 1},
			}},
	}
	page := newOverviewPage(ctx, fixture, clock, func() {})
	health := &dataHealthPage{ctx: ctx, invalidate: func() {}, reader: fixedDataHealthRead{Issues: []application.HealthIssue{
		{ID: "inside", Kind: application.HealthKindSnapshotIncomplete, Label: "Synthetic inside", CurrencyA: "AUD", CurrencyB: "USD",
			RangeStart: "2026-11-24", RangeEnd: "2026-11-24", Collapsed: true},
		{ID: "outside", Kind: application.HealthKindSnapshotIncomplete, Label: "Synthetic outside", CurrencyA: "CAD", CurrencyB: "USD",
			RangeStart: "2026-11-22", RangeEnd: "2026-11-22"},
	}}}
	wealth := newWealthHistoryPage(ctx, wealthHistoryTestReader{}, clock, func() {})
	ledger := newHistoryPage(history.New(ctx, activityReaderFunc(func(context.Context, domain.ActivityQuery) (domain.ActivityPage, error) {
		return domain.ActivityPage{}, nil
	}), func() {}))
	t.Cleanup(func() {
		cancel()
		page.workers.Wait()
		health.workers.Wait()
		wealth.workers.Wait()
		ledger.model.Wait()
	})
	h := newShellHarnessWithPages(t, ledgerPages{overview: page, wealthHistory: wealth, history: ledger, dataHealth: health})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(1160, 1060)
	h.frame()
	page.workers.Wait()
	h.frame()
	page.workers.Wait()
	h.frame()
	assertPageLabel(t, &h.router, "Closed days with a valuation · 1 of 2")
	h.click("Open Wealth history", false)
	h.frame()
	if h.shell.activeRoute() != wealthHistoryRoute {
		t.Fatal("Overview did not open Wealth history")
	}
	h.click("Back", false)
	h.frame()
	h.focused(&page.trend.history)

	h.click("View period coverage gaps", false)
	h.frame()
	health.workers.Wait()
	h.frame()
	if h.shell.activeRoute() != dataHealthRoute || health.focus.from != "2026-11-23" || health.focus.to != "2026-11-24" {
		t.Fatalf("Overview focused coverage on %+v", health.focus)
	}
	assertPageLabel(t, &h.router, "Coverage findings · 2026-11-23 – 2026-11-24")
	assertPageLabel(t, &h.router, "Synthetic inside")
	if findPageLabel(&h.router, "Synthetic outside") != nil {
		t.Fatal("period scope included an outside finding")
	}
	h.click("Back", false)
	h.frame()
	h.focused(&page.trend.gaps)
	h.press(key.NameReturn)
	h.frame()
	if h.shell.activeRoute() != dataHealthRoute {
		t.Fatal("restored period control did not reopen Data health")
	}
	h.click("Back", false)
	h.frame()

	// The open table sits under the chart; a taller window shows its rows.
	h.size = image.Pt(1160, 1600)
	page.list.ScrollTo(page.trendIndex())
	h.frame()
	if !page.trend.table.open {
		h.click("View data table", false)
		h.frame()
	}
	h.click("View this date’s coverage gap", false)
	h.frame()
	if health.focus.from != "2026-11-24" || health.focus.to != "2026-11-24" {
		t.Fatalf("daily valuation data sent %+v", health.focus)
	}
	h.click("Back", false)
	h.frame()
	h.focused(page.trend.dayGaps["2026-11-24"])
	page.list.ScrollTo(0)
	h.frame()

	h.click("Missing exchange rate · USD/AUD · Synthetic Australian cash", false)
	h.frame()
	health.workers.Wait()
	h.frame()
	if health.focus.currencyA != "USD" || health.focus.currencyB != "AUD" || health.focus.from != "" {
		t.Fatalf("current missing valuation focused %+v", health.focus)
	}
	assertPageLabel(t, &h.router, "Synthetic inside")
	if findPageLabel(&h.router, "Synthetic outside") != nil {
		t.Fatal("FX focus included a different pair")
	}
}
