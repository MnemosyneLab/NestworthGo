package desktop

import (
	"context"
	"fmt"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"github.com/shopspring/decimal"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type overviewTrendReader interface {
	NetWorthTrend(context.Context, domain.TrendRange) (domain.NetWorthTrend, error)
}

// overviewTrendOriginReader is the optional history-origin read. Without it
// the window is bounded by an unbounded past, as the Wails card does while the
// origin is unknown.
type overviewTrendOriginReader interface {
	HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
}

// overviewTrendDefaultPreset is the one-month shortcut, the card's first view.
const overviewTrendDefaultPreset = 2

const overviewTrendNoOrigin = "1970-01-01"

type overviewTrend struct {
	reader                          overviewTrendReader
	request                         <-chan historyResult[domain.NetWorthTrend]
	cancel                          context.CancelFunc
	current                         *domain.NetWorthTrend
	err                             error
	requested                       domain.TrendRange
	stale                           bool
	origin                          *wealthHistoryOrigin
	originRead                      <-chan historyResult[*wealthHistoryOrigin]
	originLoaded                    bool
	dates                           analysisDateShortcuts
	window                          analysisDateWindow
	chosen                          bool
	history, gaps, gap, retry, data widget.Clickable
	showData                        bool
	dayGaps                         map[string]*widget.Clickable
	chart                           charts.LineChart
	selected                        charts.Selection
}

// invalidate drops the loaded series and the history origin; the ledger may
// have changed under both.
func (t *overviewTrend) invalidate() {
	t.refetch()
	t.originLoaded = false
}

// refetch reads the series again for the selected window.
func (t *overviewTrend) refetch() {
	t.retire()
	t.stale = true
}

// bounds are the first day with history and today. Both are civil days in the
// household's history time zone.
func (t *overviewTrend) bounds(view ui.View, now time.Time) (earliest, latest string) {
	earliest, location := overviewTrendNoOrigin, view.TimeLocation
	if t.origin != nil {
		earliest, location = t.origin.first, t.origin.location
	}
	if location == nil {
		location = time.UTC
	}
	return earliest, now.In(location).Format(time.DateOnly)
}

// selection is the visible window: the user's choice, or the default month,
// kept inside [earliest, latest].
func (t *overviewTrend) selection(earliest, latest string) analysisDateWindow {
	window := t.window
	if !t.chosen {
		window = analysisPresetRange(overviewTrendDefaultPreset, latest, earliest)
	}
	window.from, window.to = max(window.from, earliest), min(window.to, latest)
	return window
}

// query is the trend range read for a window. The whole history reads as
// "all", so the series follows later history instead of freezing its dates.
func overviewTrendQuery(window analysisDateWindow, earliest, latest string) domain.TrendRange {
	if window.from == earliest && window.to == latest {
		return domain.TrendAllTime
	}
	return domain.TrendRange(window.from + ":" + window.to)
}

func (t *overviewTrend) retire() {
	if t.cancel != nil {
		t.cancel()
	}
	t.cancel, t.request = nil, nil
}

func (p *overviewPage) leave() {
	if p.loading {
		p.refresh()
	}
	if p.trend.request != nil {
		p.trend.invalidate()
	}
}

func (p *overviewPage) updateTrend(gtx layout.Context, view ui.View) {
	t := &p.trend
	if p.current == nil || p.current.value.AccountCount == 0 || !p.current.value.HistoryStarted || t.reader == nil {
		t.retire()
		return
	}
	if origin, ok := p.reader.(overviewTrendOriginReader); ok && !t.originLoaded && t.originRead == nil {
		t.originRead = runHistoryRead(&p.workers, p.invalidate, func() (*wealthHistoryOrigin, error) {
			origin, err := origin.HistoryOrigin(p.ctx)
			if err != nil || origin == nil {
				return nil, err
			}
			location, err := time.LoadLocation(origin.Timezone)
			if err != nil {
				return nil, err
			}
			return &wealthHistoryOrigin{first: origin.StartedAt.In(location).Format(time.DateOnly), location: location}, nil
		})
	}
	select {
	case result := <-t.originRead:
		t.originRead, t.originLoaded = nil, true
		if result.err == nil {
			t.origin = result.value
		}
	default:
	}
	select {
	case result := <-t.request:
		t.retire()
		t.err = result.err
		if result.err == nil {
			t.current = &result.value
			t.selected = charts.Selection{}
			clear(t.dayGaps)
		}
	default:
	}
	earliest, latest := t.bounds(view, p.clock())
	want := overviewTrendQuery(t.selection(earliest, latest), earliest, latest)
	if t.requested != want || (t.err != nil && t.retry.Clicked(gtx)) {
		t.refetch()
	}
	if t.request != nil || (!t.stale && (t.current != nil || t.err != nil)) {
		return
	}
	ctx, cancel := context.WithCancel(p.ctx)
	period := want
	t.cancel, t.err, t.stale, t.requested = cancel, nil, false, want
	t.request = runHistoryRead(&p.workers, p.invalidate, func() (domain.NetWorthTrend, error) {
		return t.reader.NetWorthTrend(ctx, period)
	})
}

func overviewTrendTotal(point domain.NetWorthTrendPoint) *domain.MoneyView {
	value := point.DisplayValuation.NetWorth
	if value.Quality.Availability != "" || point.NetWorth == nil || !point.Complete {
		return value.Total
	}
	return &domain.MoneyView{Amount: point.NetWorth.Amount().String(), Currency: point.NetWorth.Currency()}
}

func overviewTrendGaps(trend *domain.NetWorthTrend) (coverage, gaps int, first, last string) {
	for _, point := range trend.Points {
		if point.Current {
			continue
		}
		if first == "" {
			first = point.LocalDate
		}
		last = point.LocalDate
		coverage++
		if overviewTrendTotal(point) == nil {
			gaps++
		}
	}
	return
}

const overviewTrendSeries charts.ID = "overview-net-worth"

func overviewTrendSpec(view ui.View, trend *domain.NetWorthTrend) charts.LineSpec {
	l := view.Language
	from, _ := ui.ParseCivilDay(trend.StartDate)
	to, _ := ui.ParseCivilDay(trend.EndDate)
	spec := charts.LineSpec{
		Label: l.Text(i18n.OverviewNetWorthTrend), Summary: l.Text(i18n.OverviewTrendBasis),
		From: from, To: to, Height: 245, Area: true,
		FormatTime: func(day time.Time) string { return ui.FormatDay(day, l, view.DateFormat) },
		FormatValue: func(value float64) string {
			return overviewAllocationMoney(view, trend.Currency, decimal.NewFromFloat(value))
		},
		Messages: charts.Messages{Empty: l.Text(i18n.OverviewTrendEmpty), Missing: l.Text(i18n.OverviewNoValue), Selected: l.Text(i18n.ChartSelected)},
	}
	series := charts.LineSeries{ID: overviewTrendSeries, Name: l.Text(i18n.NetWorth)}
	for _, raw := range trend.Points {
		date, ok := ui.ParseCivilDay(raw.LocalDate)
		if !ok {
			continue
		}
		p := charts.LinePoint{ID: charts.ID(raw.LocalDate), Time: date, Provisional: raw.Current ||
			raw.DisplayValuation.NetWorth.Quality.Coverage == domain.CoverageProvisional}
		if value := overviewTrendTotal(raw); value != nil {
			p.Value, p.Text = wealthChartValue(value.Amount, false), valuationMoney(view, *value, false)
		}
		if raw.Current {
			p.Detail = append(p.Detail, l.Text(i18n.OverviewCurrentEstimate))
		}
		if !p.Value.Valid() {
			p.Detail = append(p.Detail, wealthAmount(view, i18n.NetWorth, raw.DisplayValuation.NetWorth))
		}
		if raw.MissingCount > 0 {
			p.Detail = append(p.Detail, fmt.Sprintf(l.Text(i18n.WealthHistoryMissing), raw.MissingCount))
		}
		series.Points = append(series.Points, p)
	}
	spec.Series = []charts.LineSeries{series}
	return spec
}

func (p *overviewPage) overviewTrendCard(gtx layout.Context, view ui.View, historyStarted bool) layout.Dimensions {
	l, t := view.Language, &p.trend
	header := []layout.FlexChild{layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return view.Line(gtx, ui.TextHeading, l.Text(i18n.OverviewNetWorthTrend), view.Colors.Ink)
	})}
	if p.openHistory != nil {
		label := i18n.OverviewOpenWealthHistory
		if !historyStarted {
			label = i18n.HistoryStartAction
		}
		if t.history.Clicked(gtx) {
			p.openHistory(historyStarted, &t.history)
			gtx.Execute(op.InvalidateCmd{})
		}
		header = append(header, layout.Rigid(returnMonthButton(view, &t.history, label, nil, true)))
	}
	rows := []ui.ColumnChild{ui.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space12)}.Layout(gtx, header...)
	})}
	if !historyStarted {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.WealthHistoryNoOrigin), view.Colors.SecondaryInk)))
	} else if t.reader != nil {
		rows = append(rows, p.overviewTrendRows(gtx, view)...)
	}
	return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
		return (ui.Column{Gap: ui.Space12}).Layout(gtx, rows...)
	})
}

func (p *overviewPage) overviewTrendRows(gtx layout.Context, view ui.View) []ui.ColumnChild {
	l, t := view.Language, &p.trend
	earliest, latest := t.bounds(view, p.clock())
	t.dates.showRange = true
	rows := []ui.ColumnChild{ui.Rigid(func(gtx layout.Context) layout.Dimensions {
		return t.dates.layout(gtx, view, t.selection(earliest, latest), earliest, latest, func(window analysisDateWindow) {
			t.window, t.chosen = window, true
			gtx.Execute(op.InvalidateCmd{})
		})
	})}
	switch {
	case t.request != nil:
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.WealthHistoryLoading), view.Colors.SecondaryInk)))
	case t.err != nil:
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.WealthHistoryFailed), view.Colors.ErrorInk)),
			ui.Rigid(returnMonthButton(view, &t.retry, i18n.HistoryRetry, nil, true)))
		if t.current != nil {
			rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.WealthHistoryRetained), view.Colors.SecondaryInk)))
		}
	}
	trend := t.current
	if trend == nil {
		return rows
	}
	if len(trend.Points) == 0 {
		return append(rows, ui.Rigid(view.Message(l.Text(i18n.OverviewTrendEmpty), view.Colors.SecondaryInk)))
	}
	closed, gaps, from, to := overviewTrendGaps(trend)
	caption := fmt.Sprintf(l.Text(i18n.OverviewTrendCoverage), closed-gaps, closed)
	if closed == 0 {
		caption = l.Text(i18n.OverviewTrendEmpty)
	}
	change, amount := i18n.OverviewTrendChange, trend.Change
	if amount == nil && trend.EstimatedChange != nil {
		change, amount = i18n.OverviewTrendEstimate, trend.EstimatedChange
	}
	rows = append(rows,
		ui.Rigid(view.Message(wealthDay(view, trend.StartDate)+" – "+wealthDay(view, trend.EndDate)+" · "+string(trend.Currency), view.Colors.SecondaryInk)),
		ui.Rigid(view.Body(ui.TextMoneyMD, l.Text(change)+": "+returnMoney(view, amount), view.Colors.Ink)),
		ui.Rigid(view.Message(caption, view.Colors.SecondaryInk)))
	if gaps != 0 && p.openHealth != nil {
		if t.gaps.Clicked(gtx) {
			p.openHealth(healthFindingFocus{from: from, to: to}, &t.gaps)
			gtx.Execute(op.InvalidateCmd{})
		}
		rows = append(rows, ui.Rigid(returnMonthButton(view, &t.gaps, i18n.OverviewTrendGaps, nil, true)))
	}
	rows = append(rows, ui.Rigid(analysisLineBody(view, &t.chart, overviewTrendSpec(view, trend), &t.selected)))
	for _, point := range trend.Points {
		if charts.ID(point.LocalDate) == t.selected.Item && !point.Current && overviewTrendTotal(point) == nil && p.openHealth != nil {
			if t.gap.Clicked(gtx) {
				p.openHealth(healthFindingFocus{from: point.LocalDate, to: point.LocalDate}, &t.gap)
				gtx.Execute(op.InvalidateCmd{})
			}
			rows = append(rows, ui.Rigid(returnMonthButton(view, &t.gap, i18n.OverviewTrendSelectedGap, nil, true)))
			break
		}
	}
	if t.data.Clicked(gtx) {
		t.showData = !t.showData
		gtx.Execute(op.InvalidateCmd{})
	}
	rows = append(rows, ui.Rigid(analysisDisclosure(view, &t.data, l.Text(i18n.OverviewTrendData), t.showData)))
	return rows
}

func (p *overviewPage) overviewTrendDataRow(gtx layout.Context, view ui.View, point domain.NetWorthTrendPoint) layout.Dimensions {
	values := point.DisplayValuation
	day := wealthDay(view, point.LocalDate)
	if point.Current {
		day += " · " + view.Language.Text(i18n.OverviewCurrentEstimate)
	}
	rows := []ui.ColumnChild{
		ui.Rigid(view.Body(ui.TextLabel, day, view.Colors.Ink)),
		ui.Rigid(view.Message(wealthAmount(view, i18n.NetWorth, values.NetWorth), view.Colors.Ink)),
		ui.Rigid(view.Message(wealthAmount(view, i18n.Assets, values.Assets), view.Colors.Ink)),
		ui.Rigid(view.Message(wealthAmount(view, i18n.Liabilities, values.Liabilities), view.Colors.Ink)),
	}
	if p.openHealth != nil && !point.Current && overviewTrendTotal(point) == nil {
		if p.trend.dayGaps == nil {
			p.trend.dayGaps = make(map[string]*widget.Clickable)
		}
		button := portfolioButton(p.trend.dayGaps, point.LocalDate)
		if button.Clicked(gtx) {
			p.openHealth(healthFindingFocus{from: point.LocalDate, to: point.LocalDate}, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		rows = append(rows, ui.Rigid(returnMonthButton(view, button, i18n.OverviewTrendSelectedGap, nil, true)))
	}
	return (ui.Column{Gap: 3}).Layout(gtx, rows...)
}
