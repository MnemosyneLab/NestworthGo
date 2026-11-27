package desktop

import (
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	wailsquote "github.com/waltwang/nestworth-go/internal/wailsapi/quote"
)

const performanceFixtureHousehold = "Synthetic instrument performance"

type instrumentPerformanceFixture struct {
	account              domain.AccountID
	full, sparse, manual domain.Instrument
}

func seedInstrumentPerformance(t *testing.T, latest time.Time) ([]byte, instrumentPerformanceFixture) {
	t.Helper()
	ctx := t.Context()
	db, _, app := openStage6Database(t, filepath.Join(t.TempDir(), "performance.db"), func() time.Time {
		return latest.AddDate(0, 0, 1).Add(12 * time.Hour)
	})
	if err := app.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: performanceFixtureHousehold, BaseCurrency: "USD", MemberNames: []string{"Sample owner"}}); err != nil {
		t.Fatal(err)
	}
	boot, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := app.CreateAccount(ctx, application.AccountInput{Name: "Performance Brokerage", AccountType: "brokerage", TrackingMode: "holdings",
		BalanceSheetRole: "asset", DefaultCurrency: "EUR", IncludeInNetWorth: true, IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	instrument := func(name, symbol, kind string) domain.Instrument {
		t.Helper()
		result, err := app.CreateInstrument(ctx, application.InstrumentInput{Name: name, Symbol: symbol, Type: kind, QuoteCurrency: "EUR", QuoteSource: "agent"})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	full := instrument("Synthetic performance ETF", "PERF", "etf")
	sparse := instrument("Synthetic sparse stock", "SPARSE", "stock")
	note := "Synthetic exact current unit quote"
	manual, err := app.CreateInstrument(ctx, application.InstrumentInput{Name: "Synthetic Apple common stock", Symbol: "AAPL", Type: "stock",
		MarketCode: "NASDAQ", CountryCode: "US", ISIN: "US0378331005", Note: &note, QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.AppendManualInstrumentQuote(ctx, manual.ID, "1234.56789123", latest.Add(23*time.Hour).Format(time.RFC3339), false); err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateHolding(ctx, application.HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: full.ID.String(), Quantity: "3"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.AppendManualFXQuote(ctx, "EUR", "USD", "1.2", latest.Format(time.DateOnly)); err != nil {
		t.Fatal(err)
	}
	var notes []application.AgentMarketDataItem
	closeOn := func(instrument domain.Instrument, days int, price string) {
		notes = append(notes, application.AgentMarketDataItem{InstrumentID: instrument.ID.String(), Currency: "EUR", Kind: "close", Value: price,
			Date: latest.AddDate(0, 0, -days).Format(time.DateOnly), Timezone: "UTC", SourceTitle: "Synthetic daily fixture"})
	}
	for i, days := range []int{365, 180, 90, 30, 7, 0} {
		closeOn(full, days, []string{"80", "90", "95", "100", "105", "120"}[i])
	}
	for i, days := range []int{2, 1, 0} {
		closeOn(sparse, days, []string{"100", "101", "102"}[i])
	}
	notes = append(notes, application.AgentMarketDataItem{InstrumentID: full.ID.String(), Currency: "EUR", Kind: "latest", Value: "999",
		QuotedAt: latest.Add(23 * time.Hour).Format(time.RFC3339), SourceTitle: "Synthetic intraday fixture"})
	if _, err := app.ImportAgentMarketData(ctx, uuid.NewString(), application.AgentMarketDataInput{Items: notes}); err != nil {
		t.Fatal(err)
	}
	return stage6SeedBytes(t, db), instrumentPerformanceFixture{account: account.Account.ID, full: full, sparse: sparse, manual: manual}
}

func TestExportInstrumentPerformanceFixtures(t *testing.T) {
	output := os.Getenv("NESTWORTH_STAGE6_QA_DIR")
	if output == "" {
		t.Skip("set NESTWORTH_STAGE6_QA_DIR to an existing disposable directory to export instrument performance fixtures")
	}
	root, err := os.MkdirTemp(output, "nestworth-performance-")
	if err != nil {
		t.Fatal(err)
	}
	latest := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -1)
	seed, _ := seedInstrumentPerformance(t, latest)
	for _, copy := range []string{"source", "gio", "wails"} {
		writeStage6Fixture(t, filepath.Join(root, copy), seed)
	}
	t.Logf("fixtures=%s latest_date=%s household=%q", root, latest.Format(time.DateOnly), performanceFixtureHousehold)
}

type instrumentPerformanceProbe struct {
	*application.Service
	seriesCalls  atomic.Int32
	returnsCalls atomic.Int32
	failReturns  atomic.Bool
}

func (s *instrumentPerformanceProbe) InstrumentQuotePerformanceSeries(ctx context.Context, id domain.InstrumentID, days int) (domain.QuotePerformanceSeries, error) {
	s.seriesCalls.Add(1)
	return s.Service.InstrumentQuotePerformanceSeries(ctx, id, days)
}

func (s *instrumentPerformanceProbe) InstrumentQuoteReturns(ctx context.Context, id domain.InstrumentID) (domain.QuotePerformanceReport, error) {
	s.returnsCalls.Add(1)
	if s.failReturns.Swap(false) {
		return domain.QuotePerformanceReport{}, errors.New("synthetic period-returns read failure")
	}
	return s.Service.InstrumentQuoteReturns(ctx, id)
}

func TestPortfolioInstrumentPerformanceMatchesWails(t *testing.T) {
	ctx := t.Context()
	latest := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return latest.Add(36 * time.Hour) }
	seed, fixture := seedInstrumentPerformance(t, latest)
	gioPath := writeStage6Fixture(t, filepath.Join(t.TempDir(), "gio"), seed)
	wailsPath := writeStage6Fixture(t, filepath.Join(t.TempDir(), "wails"), seed)
	gioProvider, wailsProvider := &unavailableOverviewProvider{}, &unavailableOverviewProvider{}
	_, _, gio := openStage6Database(t, gioPath, clock, application.NewMarketDataRegistry(gioProvider))
	_, _, wails := openStage6Database(t, wailsPath, clock, application.NewMarketDataRegistry(wailsProvider))
	adapter := wailsquote.NewService(wails)
	probe := &instrumentPerformanceProbe{Service: gio}
	probe.failReturns.Store(true)
	page := newPortfolioPage(ctx, probe, func() {})
	t.Cleanup(page.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{portfolio: page})
	h.size = image.Pt(1280, 1080)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 3 {
			h.frame()
			page.workers.Wait()
		}
		h.frame()
	}
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	label := func(label string) { t.Helper(); assertPageLabel(t, &h.router, label) }
	perf := &page.detail.performance
	checkSeries := func(instrument domain.InstrumentID, days int, percent, unavailable string) {
		t.Helper()
		want, err := adapter.InstrumentQuotePerformanceSeries(ctx, instrument.String(), days)
		got := perf.series.value
		if err != nil || got == nil || got.WindowDays != days || got.From != want.From || got.To != want.To || !reflect.DeepEqual(got.Item, want.Item) ||
			got.UnavailableReason != unavailable || got.UnavailableReason != want.UnavailableReason || len(got.Points) != len(want.Points) {
			t.Fatalf("Gio and Wails series differ for %d: %+v / %+v / %v", days, got, want, err)
		}
		if percent != "" && (got.Item == nil || got.Item.PeriodPercent != percent) {
			t.Fatalf("the requested period returned %+v, want %s%%", got.Item, percent)
		}
		for i, point := range got.Points {
			ref := want.Points[i]
			if point.Value != ref.Value || point.Value == "999" || point.EffectiveDate != ref.EffectiveDate || point.Currency != "EUR" ||
				point.Currency.String() != ref.Currency || point.BreakBefore != ref.BreakBefore || string(point.SourceKind) != ref.SourceKind {
				t.Fatalf("series point differs from saved daily evidence: %+v / %+v", point, ref)
			}
		}
	}

	settle()
	page.openHoldingInstrument(fixture.full.ID, fixture.account, fixture.full.DisplayLabel(), nil)
	settle()
	if probe.seriesCalls.Load() != 0 || probe.returnsCalls.Load() != 0 || page.accountScope != fixture.account {
		t.Fatal("the account holding loaded performance before its tab was selected")
	}
	click("Market performance")
	checkSeries(fixture.full.ID, 30, "20", "")
	label("+20.00%")
	label("2026-09-14 – 2026-10-14")
	label("Could not load period returns.")
	click("Retry")
	wantReturns, err := adapter.InstrumentQuoteReturns(ctx, fixture.full.ID.String())
	if err != nil || perf.returns.value == nil || !reflect.DeepEqual(*perf.returns.value, wantReturns) || len(wantReturns.Items) != 5 ||
		wantReturns.Items[1].AnnualizedPercent == nil || probe.seriesCalls.Load() != 1 || probe.returnsCalls.Load() != 2 {
		t.Fatalf("fixed returns retry did not match Wails independently: %+v / %+v / %v", perf.returns.value, wantReturns, err)
	}
	label("30-day return")
	label("Annualized · 243.33%")
	h.size = image.Pt(665, 480)
	settle()
	click("Last week")
	checkSeries(fixture.full.ID, 7, "14.2857142857142857142857", "")
	label("+14.29%")
	h.size = image.Pt(1280, 1080)
	settle()
	click("All history")
	checkSeries(fixture.full.ID, 0, "50", "")
	label("+50.00%")
	click("View data table")
	if !perf.table.open || probe.returnsCalls.Load() != 2 {
		t.Fatal("chart period changed the fixed returns or failed to open the saved-price table")
	}
	label("EUR 120")
	click("Data records")
	label("€999.00")
	click("Market performance")
	checkSeries(fixture.full.ID, 0, "50", "")
	click("Investment assets")
	click("Back")
	if page.id != fixture.full.ID || page.accountScope != fixture.account || page.detail.visit.section != instrumentDetailPerformance || !perf.table.open {
		t.Fatal("Back lost the instrument account, Performance tab or table")
	}
	checkSeries(fixture.full.ID, 0, "50", "")

	page.openInstrument(fixture.sparse.ID, fixture.sparse.DisplayLabel(), nil)
	settle()
	click("Market performance")
	checkSeries(fixture.sparse.ID, 30, "", "insufficient_daily_history")
	label("Not enough history for this period")
	if perf.series.value.Item != nil || len(perf.series.value.Points) != 3 || perf.returns.value == nil || len(perf.returns.value.Items) != 0 {
		t.Fatal("the sparse chart omitted saved closes or invented a covered return")
	}
	click("All history")
	checkSeries(fixture.sparse.ID, 0, "2", "")
	label("+2.00%")
	page.openHoldingInstrument(fixture.manual.ID, fixture.account, "Previous instrument label", nil)
	settle()
	if current := h.shell.trail.current(); current.name != "AAPL" || current.target.portfolioAccount != fixture.account {
		t.Fatalf("the instrument title lost its canonical label or account scope: %+v", current)
	}
	if heading := assertPageLabel(t, &h.router, "AAPL"); heading.Min.Y >= headerHeight {
		t.Fatalf("expected the instrument label in the shell heading, got %v", heading)
	}
	for _, text := range []string{"Synthetic Apple common stock", "Type · Stock", "Currency · USD", "Security symbol · AAPL",
		"Trading market · NASDAQ", "Country / region · US", "ISIN · US0378331005", "My holdings", "Market performance", "Data records"} {
		label(text)
	}
	click("Note")
	label("Synthetic exact current unit quote")
	click("Note")
	click("Market performance")
	checkSeries(fixture.manual.ID, 30, "", "insufficient_daily_history")
	label("Price and NAV changes describe this instrument, not your personal return.")
	label("No daily price history yet")
	label("Record or sync daily closing prices. Timestamped manual prices are available in Market Data.")
	if len(perf.series.value.Points) != 0 {
		t.Fatal("a timestamped manual quote was shown as a daily close")
	}
	click("Data records")
	label("Latest quote")
	label("US$1,234.56789123")
	label("as of 2026-10-14 23:00 · Source: Manual")
	current, err := adapter.CurrentInstrumentQuote(ctx, fixture.manual.ID.String())
	if err != nil || current == nil || current.UnitPrice != "1234.56789123" || current.SourceKind != "manual" {
		t.Fatalf("Gio metadata sample uses a different quote than Wails: %+v / %v", current, err)
	}

	h.shell.prefs.saved.Language = settings.LanguageZhCN
	h.shell.prefs.saved.DecimalSeparator = settings.DecimalComma
	h.shell.prefs.saved.GroupingSeparator = settings.GroupingDot
	settle()
	for _, text := range []string{"类型 · 股票", "货币 · USD", "我的持仓", "标的表现", "数据记录", "最新报价", "US$1.234,56789123", "更新于 2026-10-14 23:00 · 来源: 手动"} {
		label(text)
	}
	click("标的表现")
	label("暂无每日收盘价历史")
	label("记录或同步每日收盘价。带时间戳的手动价格可在市场数据中查看。")

	if gioProvider.requests != 0 || wailsProvider.requests != 0 {
		t.Fatal("displaying saved market performance requested live market data")
	}
}

func TestInstrumentPerformanceChartRespectsPriceBasisAndDenomination(t *testing.T) {
	view := ui.View{Language: i18n.English, Colors: &lightPalette, DecimalSeparator: ".", TimeLocation: time.UTC, DateFormat: settings.DateFormatISO}
	instrument := domain.Instrument{QuoteCurrency: "EUR"}
	data := domain.QuotePerformanceSeries{From: "2026-10-01", To: "2026-10-03", Points: []domain.QuotePerformancePoint{
		{EffectiveDate: "2026-10-01", Currency: "EUR", Value: "100.01234567"},
		{EffectiveDate: "2026-10-02", Currency: "EUR", Value: "101.00000001", BreakBefore: true},
		{EffectiveDate: "2026-10-03", Currency: "USD", Value: "102.12345678"},
	}}
	chart := instrumentPerformanceChart(view, instrument, data)
	points := chart.Series[0].Points
	if len(points) != 3 || points[0].Text != "EUR 100.01234567" || !points[1].Value.Valid() || !points[1].BreakBefore ||
		points[2].Value.Valid() || points[2].Text != "USD 102.12345678" || chart.From.Format(time.DateOnly) != data.From || chart.To.Format(time.DateOnly) != data.To {
		t.Fatalf("quote points lost exact evidence or bridge an incompatible price: %+v", points)
	}
}
