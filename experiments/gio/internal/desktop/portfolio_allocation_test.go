package desktop

import (
	"context"
	"errors"
	"image"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	portapi "github.com/waltwang/nestworth-go/internal/wailsapi/portfolio"
)

type portfolioTrendProbe struct {
	*application.Service
	mu     sync.Mutex
	reads  []domain.TrendRange
	failed atomic.Bool
}

func (s *portfolioTrendProbe) PortfolioTrend(ctx context.Context, period domain.TrendRange) (domain.PortfolioTrend, error) {
	s.mu.Lock()
	s.reads = append(s.reads, period)
	s.mu.Unlock()
	if s.failed.Swap(false) {
		return domain.PortfolioTrend{}, errors.New("synthetic trend read error")
	}
	return s.Service.PortfolioTrend(ctx, period)
}

func (s *portfolioTrendProbe) requests() []domain.TrendRange {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.reads)
}

func TestPortfolioAllocationAndTrendMatchWailsAndRetainScope(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	service, _, _, ids := openPortfolioFixture(t, "partial")
	wails, _, _, _ := openPortfolioFixture(t, "partial")
	adapter := portapi.NewService(wails)
	probe := &portfolioTrendProbe{Service: service}
	page := newPortfolioPage(ctx, probe, func() {})
	page.clock = func() time.Time { return accountTestClock }
	accounts := newAccountsPage(ctx, service, page.clock, func() {})
	t.Cleanup(func() {
		cancel()
		page.workers.Wait()
		accounts.workers.Wait()
	})
	h := newShellHarnessWithPages(t, ledgerPages{portfolio: page, accounts: accounts})
	h.size = image.Pt(1000, 1420)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 4 {
			h.frame()
			page.workers.Wait()
			accounts.workers.Wait()
		}
		h.frame()
	}
	click := func(label string) {
		t.Helper()
		h.click(label, false)
		settle()
	}
	last := func() domain.TrendRange {
		t.Helper()
		requests := probe.requests()
		if len(requests) == 0 {
			t.Fatal("the Allocation tab did not request a portfolio trend")
		}
		return requests[len(requests)-1]
	}

	settle()
	click("Investment assets")
	if len(probe.requests()) != 0 {
		t.Fatal("My holdings loaded the allocation trend")
	}
	click("Allocation & trend")
	if got := last(); got != domain.TrendAllTime {
		t.Fatalf("the short ledger should request the full available history, got %q", got)
	}
	if page.current.allocation.firstDay != "2026-11-20" || page.current.allocation.location.String() != "Asia/Singapore" {
		t.Fatalf("the window does not use the ledger's civil dates: %+v", page.current.allocation)
	}
	assertPageLabel(t, &h.router, "Allocation")
	assertPageLabel(t, &h.router, "Included accounts")
	actual := page.current.summary
	reference, err := adapter.Portfolio(ctx, account.AccountFilterRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if actual.ValuedSubtotal == nil || reference.ValuedSubtotal == nil || actual.ValuedSubtotal.Amount != "108.4442" ||
		actual.ValuedSubtotal.Amount != reference.ValuedSubtotal.Amount || len(actual.ByInstrumentType) != 1 || len(reference.ByInstrumentType) != 1 {
		t.Fatalf("portfolio subtotal or allocation disagreed: %+v / %+v", actual.ValuedSubtotal, reference.ValuedSubtotal)
	}
	group, other := actual.ByInstrumentType[0], reference.ByInstrumentType[0]
	if group.Key != other.Key || group.Amount.Amount != other.Amount.Amount || group.ShareBPS != other.ShareBPS {
		t.Fatalf("the allocation disagreed: %+v / %+v", group, other)
	}
	if listed := page.current.allocation.accounts; len(listed) != 2 || listed[0].Account.ID != ids.account || listed[0].Complete || !listed[1].Complete {
		t.Fatalf("the included accounts did not use the exact sorted portfolio results: %+v", listed)
	}
	trend := page.allocation.current
	wTrend, err := adapter.PortfolioTrend(ctx, string(last()))
	if err != nil || trend == nil || len(trend.Points) == 0 || len(trend.Points) != len(wTrend.Points) {
		t.Fatalf("portfolio trend did not match: %+v / %+v / %v", trend, wTrend, err)
	}
	for i, point := range trend.Points {
		want := wTrend.Points[i]
		if point.LocalDate != want.LocalDate || point.Current != want.Current || point.MissingCount != want.MissingCount ||
			point.DisplayValuation.Quality.Availability != want.DisplayValuation.Quality.Availability {
			t.Fatalf("trend day disagreed: %+v / %+v", point, want)
		}
	}
	current := trend.Points[len(trend.Points)-1]
	if current.LocalDate != "2026-11-22" || !current.Current || current.DisplayValuation.Total != nil ||
		current.DisplayValuation.KnownSubtotal == nil || current.DisplayValuation.KnownSubtotal.Amount != "108.4442" {
		t.Fatalf("current trend point hid an incomplete holding or included cash: %+v", current)
	}

	page.list.Position = layout.Position{First: 1}
	h.size.Y = 760
	settle()
	click("Last day")
	if got := last(); got != "2026-11-22:2026-11-22" {
		t.Fatalf("Last day requested %q", got)
	}
	probe.failed.Store(true)
	click("Previous date range")
	if got := last(); got != "2026-11-21:2026-11-21" || page.allocation.err == nil {
		t.Fatalf("Previous date did not preserve closed-day scope and show the error, got %q / %v", got, page.allocation.err)
	}
	assertPageLabel(t, &h.router, "Could not load the portfolio trend.")
	click("Retry")
	if page.allocation.err != nil || page.allocation.current == nil || page.allocation.current.Range != last() {
		t.Fatal("the failed range was not retried")
	}
	click("Next date range")
	if got := last(); got != "2026-11-22:2026-11-22" {
		t.Fatalf("Next date requested %q", got)
	}
	h.size.Y = 1420
	settle()
	// Both charts use the same translated action label. Activate the trend
	// disclosure by keyboard so the composition disclosure cannot match it.
	h.router.Source().Execute(key.FocusCmd{Tag: &page.allocation.table.toggle})
	h.frame()
	h.focused(&page.allocation.table.toggle)
	h.press(key.NameReturn)
	settle()
	if !page.allocation.table.open {
		t.Fatal("the portfolio trend table did not open")
	}
	page.list.Position = layout.Position{First: 2}
	settle()
	click("Open account · Harbor Brokerage")
	if accounts.target.account != ids.account {
		t.Fatalf("allocation link opened another account: %v", accounts.target.account)
	}
	click("Back")
	if page.tabs.active != portfolioAllocationTab || last() != "2026-11-22:2026-11-22" || !page.allocation.table.open {
		t.Fatal("Back did not restore the allocation tab, period, and table disclosure")
	}
	h.focused(portfolioButton(page.accounts, ids.account))
	reads := len(probe.requests())
	click("Instruments")
	if len(probe.requests()) != reads {
		t.Fatal("the catalog read the allocation trend")
	}
	click("Allocation & trend")
	if last() != "2026-11-22:2026-11-22" {
		t.Fatal("switching tabs discarded the period selection")
	}
}

func TestPortfolioTrendDistinguishesChartTotalsFromKnownSubtotals(t *testing.T) {
	view := ui.View{Language: i18n.English, DecimalSeparator: ".", GroupingSeparator: ","}
	money := func(amount string) *domain.MoneyView { return &domain.MoneyView{Amount: amount, Currency: "USD"} }
	trend := domain.PortfolioTrend{Currency: "USD", Points: []domain.PortfolioTrendPoint{
		{LocalDate: "2026-11-19", DisplayValuation: domain.ValuationAmount{Total: money("25.125"), Quality: domain.ValuationQuality{Availability: domain.ValuationFull, Coverage: domain.CoverageVerified}}},
		{LocalDate: "2026-11-20", MissingCount: 1, DisplayValuation: domain.ValuationAmount{KnownSubtotal: money("10.25"), Quality: domain.ValuationQuality{Availability: domain.ValuationPartial}}},
		{LocalDate: "2026-11-21", Current: true, DisplayValuation: domain.ValuationAmount{Total: money("0"), Quality: domain.ValuationQuality{Availability: domain.ValuationFull, Coverage: domain.CoverageProvisional}}},
	}}
	points := portfolioTrendSpec(view, trend).Series[0].Points
	if !points[0].Value.Valid() || points[0].Value.Number != 25.125 || points[1].Value.Valid() ||
		!points[2].Value.Valid() || points[2].Value.Number != 0 || !points[2].Provisional {
		t.Fatalf("chart changed a complete, partial or zero amount: %+v", points)
	}
	table := portfolioTrendTable(view, trend, charts.Selection{Item: "2026-11-20"})
	if table.Selected != 1 || table.Rows[1][1] != "Unavailable" || table.Rows[1][2] != "US$10.25" ||
		table.Rows[1][3] != "Known subtotal · Missing inputs 1" || table.Rows[2][3] != "Current estimate" {
		t.Fatalf("the chart table did not keep the coverage and known subtotal independent: %+v", table)
	}
}
