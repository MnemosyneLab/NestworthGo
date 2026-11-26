package desktop

import (
	"context"
	"image"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

// overviewRangeFixture records the trend ranges the Overview card asks for.
type overviewRangeFixture struct {
	overviewHistoryFixture
	mu     sync.Mutex
	ranges []domain.TrendRange
	origin time.Time
}

func (f *overviewRangeFixture) NetWorthTrend(ctx context.Context, period domain.TrendRange) (domain.NetWorthTrend, error) {
	f.mu.Lock()
	f.ranges = append(f.ranges, period)
	f.mu.Unlock()
	return f.overviewHistoryFixture.NetWorthTrend(ctx, period)
}

func (f *overviewRangeFixture) HistoryOrigin(context.Context) (*domain.HistoryOrigin, error) {
	return &domain.HistoryOrigin{Timezone: "UTC", StartedAt: f.origin}, nil
}

func (f *overviewRangeFixture) requested() []domain.TrendRange {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.ranges)
}

func TestOverviewTrendReadsTheSelectedWindowAndNavigatesPeriods(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	now := time.Date(2026, 11, 25, 16, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	fixture := &overviewRangeFixture{origin: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	fixture.overviewReader = overviewReaderFunc(func(context.Context, domain.AccountFilter) (domain.OverviewResult, error) {
		return domain.OverviewResult{Currency: "USD", AccountCount: 1, HistoryStarted: true}, nil
	})
	fixture.trend = domain.NetWorthTrend{Currency: "USD", StartDate: "2026-11-23", EndDate: "2026-11-25",
		Points: []domain.NetWorthTrendPoint{{LocalDate: "2026-11-23"}, {LocalDate: "2026-11-25", Current: true}}}
	page := newOverviewPage(ctx, fixture, clock, func() {})
	t.Cleanup(func() {
		cancel()
		page.workers.Wait()
	})
	h := newShellHarnessWithPages(t, ledgerPages{overview: page})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(1160, 1060)
	settle := func() {
		for range 3 {
			h.frame()
			page.workers.Wait()
		}
	}
	settle()
	last := func() domain.TrendRange {
		ranges := fixture.requested()
		return ranges[len(ranges)-1]
	}
	if got := last(); got != "2026-10-26:2026-11-25" {
		t.Fatalf("first read asked for %q, want the last month as explicit dates", got)
	}

	h.click("Last week", false)
	settle()
	if got := last(); got != "2026-11-19:2026-11-25" {
		t.Fatalf("Last week asked for %q", got)
	}
	h.click("Previous date range", false)
	settle()
	if got := last(); got != "2026-11-12:2026-11-18" {
		t.Fatalf("previous period asked for %q", got)
	}
	h.click("Next date range", false)
	settle()
	if got := last(); got != "2026-11-19:2026-11-25" {
		t.Fatalf("next period asked for %q", got)
	}
	h.click("All history", false)
	settle()
	if got := last(); got != domain.TrendAllTime {
		t.Fatalf("whole history asked for %q, want %q so later history is followed", got, domain.TrendAllTime)
	}
	reads := len(fixture.requested())
	settle()
	if len(fixture.requested()) != reads {
		t.Fatal("an unchanged window read the series again")
	}
}

func TestOverviewAllHistoryFollowsTheLedgerDay(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	// 07:00 UTC is still 25 November in Los Angeles, and already 26 November in UTC.
	now := time.Date(2026, 11, 26, 7, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	fixture := &overviewRollingFixture{now: clock, loc: loc}
	fixture.overviewReader = overviewReaderFunc(func(context.Context, domain.AccountFilter) (domain.OverviewResult, error) {
		return domain.OverviewResult{Currency: "USD", AccountCount: 1, HistoryStarted: true}, nil
	})
	page := newOverviewPage(ctx, fixture, clock, func() {})
	t.Cleanup(func() {
		cancel()
		page.workers.Wait()
	})
	h := newShellHarnessWithPages(t, ledgerPages{overview: page})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(1160, 1060)
	settle := func() {
		t.Helper()
		for range 3 {
			h.frame()
			page.workers.Wait()
		}
	}
	settle()
	last := func() domain.TrendRange {
		t.Helper()
		ranges := fixture.requested()
		if len(ranges) == 0 {
			t.Fatal("overview did not read a trend")
		}
		return ranges[len(ranges)-1]
	}
	if got := last(); got != "2026-10-26:2026-11-25" {
		t.Fatalf("ledger day is 25 November, first read asked for %q", got)
	}

	h.click("All history", false)
	settle()
	if got := last(); got != domain.TrendAllTime {
		t.Fatalf("All history asked for %q", got)
	}
	if !liabilityTextAppears(h.router.AppendSemantics(nil), "2026-11-25") {
		t.Fatal("All history on the 25th did not show that day")
	}
	allReads := len(fixture.requested())

	now = time.Date(2026, 11, 26, 8, 30, 0, 0, time.UTC) // 00:30 on the 26th in Los Angeles; UTC is still the 26th.
	settle()
	ranges := fixture.requested()
	if len(ranges) != allReads+1 || ranges[len(ranges)-1] != domain.TrendAllTime {
		t.Fatalf("crossing the ledger day asked for %v, want a new all query", ranges)
	}
	if !liabilityTextAppears(h.router.AppendSemantics(nil), "2026-11-26") {
		t.Fatal("the refreshed all-history series does not include the 26th")
	}

	h.click("Last week", false)
	settle()
	h.click("Previous date range", false)
	settle()
	if got := last(); got != "2026-11-13:2026-11-19" {
		t.Fatalf("paged week asked for %q", got)
	}
	fixed := len(fixture.requested())
	now = time.Date(2026, 11, 27, 8, 30, 0, 0, time.UTC) // 00:30 on the 27th in Los Angeles.
	settle()
	ranges = fixture.requested()
	if len(ranges) != fixed || ranges[len(ranges)-1] != "2026-11-13:2026-11-19" {
		t.Fatalf("fixed history rolled to %v", ranges)
	}
	if liabilityTextAppears(h.router.AppendSemantics(nil), "2026-11-27") {
		t.Fatal("the fixed range displayed the new ledger day")
	}
}

// overviewRollingFixture answers trend reads with a series ending on the
// ledger-local day of the test clock.
type overviewRollingFixture struct {
	overviewRangeFixture
	now func() time.Time
	loc *time.Location
}

func (f *overviewRollingFixture) NetWorthTrend(_ context.Context, period domain.TrendRange) (domain.NetWorthTrend, error) {
	f.mu.Lock()
	f.ranges = append(f.ranges, period)
	f.mu.Unlock()
	end := f.now().In(f.loc).Format(time.DateOnly)
	return domain.NetWorthTrend{
		Currency: "USD", StartDate: "2026-01-01", EndDate: end, Range: period,
		Points: []domain.NetWorthTrendPoint{
			{LocalDate: "2026-01-01"},
			{LocalDate: end, Current: true},
		},
	}, nil
}

func (f *overviewRollingFixture) HistoryOrigin(context.Context) (*domain.HistoryOrigin, error) {
	return &domain.HistoryOrigin{Timezone: f.loc.String(), StartedAt: time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)}, nil
}
