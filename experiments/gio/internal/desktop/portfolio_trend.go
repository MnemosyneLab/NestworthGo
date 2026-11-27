package desktop

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type portfolioTrendReader interface {
	PortfolioTrend(context.Context, domain.TrendRange) (domain.PortfolioTrend, error)
}

type portfolioAllocation struct {
	composition overviewSummaryChart
	dates       analysisDateShortcuts
	window      analysisDateWindow

	request   <-chan historyResult[domain.PortfolioTrend]
	cancel    context.CancelFunc
	period    domain.TrendRange
	queriedOn string
	current   *domain.PortfolioTrend
	stale     bool
	err       error
	retry     widget.Clickable
	chart     charts.LineChart
	table     chartDataTable
	selected  charts.Selection
}

type portfolioAllocationVisit struct {
	window, presetWindow           analysisDateWindow
	preset                         int
	compositionPoint, trendPoint   charts.Selection
	compositionTable, trendTable   bool
	compositionScroll, trendScroll layout.Position
}

func (a *portfolioAllocation) visit() portfolioAllocationVisit {
	return portfolioAllocationVisit{window: a.window, presetWindow: a.dates.at, preset: a.dates.chosen,
		compositionPoint: a.composition.selected, trendPoint: a.selected,
		compositionTable: a.composition.table.open, trendTable: a.table.open,
		compositionScroll: a.composition.table.list.Position, trendScroll: a.table.list.Position}
}

func (a *portfolioAllocation) restore(v portfolioAllocationVisit) {
	a.window, a.dates.at, a.dates.chosen = v.window, v.presetWindow, v.preset
	a.composition.selected, a.selected = v.compositionPoint, v.trendPoint
	a.composition.table.open, a.table.open = v.compositionTable, v.trendTable
	a.composition.table.list.Position, a.table.list.Position = v.compositionScroll, v.trendScroll
}

func (a *portfolioAllocation) retireTrend() {
	if a.cancel != nil {
		a.cancel()
	}
	a.cancel, a.request = nil, nil
}

func (p *portfolioPage) portfolioTrendRange(gtx layout.Context, view ui.View) (analysisDateWindow, string, string) {
	data, a := p.current.allocation, &p.allocation
	first, location := data.firstDay, data.location
	if first == "" {
		first, location = overviewTrendNoOrigin, view.TimeLocation
	}
	if location == nil {
		location = time.UTC
	}
	now := gtx.Now
	if p.clock != nil {
		now = p.clock()
	}
	latest := now.In(location).Format(time.DateOnly)
	window := a.window
	if window.from == "" {
		window = analysisPresetRange(overviewTrendDefaultPreset, latest, first)
	}
	window.from, window.to = max(first, window.from), min(latest, window.to)
	if window.from > window.to {
		window = analysisPresetRange(overviewTrendDefaultPreset, latest, first)
	}
	return window, first, latest
}

func (p *portfolioPage) updateAllocationTrend(gtx layout.Context, view ui.View) {
	a := &p.allocation
	if p.id != "" || p.tabs.active != portfolioAllocationTab || p.current == nil || p.current.allocation == nil ||
		len(p.current.allocation.accounts) == 0 || p.request != nil {
		return
	}
	if p.current.allocation.originErr != nil {
		if a.retry.Clicked(gtx) {
			p.read()
		}
		return
	}
	reader, ok := p.service.(portfolioTrendReader)
	if !ok {
		return
	}
	select {
	case loaded := <-a.request:
		a.retireTrend()
		a.err = loaded.err
		if loaded.err == nil {
			a.current = &loaded.value
		}
	default:
	}
	window, first, latest := p.portfolioTrendRange(gtx, view)
	period := overviewTrendQuery(window.from == first && window.to == latest, window)
	if period != a.period || period == domain.TrendAllTime && latest != a.queriedOn || a.err != nil && a.retry.Clicked(gtx) {
		a.retireTrend()
		a.stale = true
	}
	if !a.stale || a.request != nil {
		return
	}
	ctx, cancel := context.WithCancel(p.ctx)
	a.cancel, a.period, a.queriedOn, a.err, a.stale = cancel, period, latest, nil, false
	a.request = runHistoryRead(&p.workers, p.invalidate, func() (domain.PortfolioTrend, error) {
		return reader.PortfolioTrend(ctx, period)
	})
}

func (p *portfolioPage) portfolioTrendCard(view ui.View) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		a, l := &p.allocation, view.Language
		rows := []ui.ColumnChild{ui.Rigid(view.Body(ui.TextHeading, l.Text(i18n.PortfolioValue), view.Colors.Ink))}
		if p.current.allocation.originErr != nil {
			rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.PortfolioTrendFailed), view.Colors.ErrorInk)),
				ui.Rigid(returnMonthButton(view, &a.retry, i18n.HistoryRetry, nil, p.request == nil)))
			return overviewBreakdownCard(gtx, view, rows)
		}
		window, first, latest := p.portfolioTrendRange(gtx, view)
		a.dates.showRange, a.dates.oneDayLabel = true, i18n.PortfolioTrendDay
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.dates.layout(gtx, view, window, first, latest, func(next analysisDateWindow) {
				a.window = next
				a.selected = charts.Selection{}
				a.table.list.Position = layout.Position{}
				gtx.Execute(op.InvalidateCmd{})
			})
		}))
		current := a.current
		if current != nil && current.Range != a.period {
			current = nil
		}
		switch {
		case a.request != nil:
			label := i18n.PortfolioLoading
			if current != nil {
				label = i18n.OverviewRefreshing
			}
			rows = append(rows, ui.Rigid(view.Message(l.Text(label), view.Colors.SecondaryInk)))
		case a.err != nil:
			rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.PortfolioTrendFailed), view.Colors.ErrorInk)),
				ui.Rigid(returnMonthButton(view, &a.retry, i18n.HistoryRetry, nil, true)))
			if current != nil {
				rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.PortfolioTrendRetained), view.Colors.SecondaryInk)))
			}
		case current == nil || len(current.Points) == 0:
			rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.PortfolioTrendEmpty), view.Colors.SecondaryInk)))
		}
		if current != nil && len(current.Points) != 0 {
			rows = append(rows, ui.Rigid(analysisLineBody(view, &a.chart, portfolioTrendSpec(view, *current), &a.selected)),
				ui.Rigid(a.table.layout(view, len(current.Points), func() charts.Table { return portfolioTrendTable(view, *current, a.selected) })))
		}
		return overviewBreakdownCard(gtx, view, rows)
	}
}

func portfolioTrendTotal(point domain.PortfolioTrendPoint) *domain.MoneyView {
	value := point.DisplayValuation
	if value.Quality.Availability != "" || !point.Complete || point.ValuedSubtotal == nil {
		return value.Total
	}
	return &domain.MoneyView{Amount: point.ValuedSubtotal.CanonicalAmount(), Currency: point.ValuedSubtotal.Currency()}
}

func portfolioTrendSpec(view ui.View, trend domain.PortfolioTrend) charts.LineSpec {
	l := view.Language
	spec := charts.LineSpec{Label: l.Text(i18n.PortfolioValue), Summary: l.Text(i18n.PortfolioTrendBasis), Height: 280, Area: true,
		FormatTime:  func(day time.Time) string { return ui.FormatDay(day, l, view.DateFormat) },
		FormatValue: func(value float64) string { return chartAxisNumber(view, value) },
		Messages:    charts.Messages{Empty: l.Text(i18n.PortfolioTrendEmpty), Missing: l.Text(i18n.OverviewNoValue), Selected: l.Text(i18n.ChartSelected)},
	}
	series := charts.LineSeries{ID: "portfolio-valued-holdings", Name: l.Text(i18n.PortfolioValuedHoldings)}
	for _, point := range trend.Points {
		date, ok := ui.ParseCivilDay(point.LocalDate)
		if !ok {
			continue
		}
		item := charts.LinePoint{ID: charts.ID(point.LocalDate), Time: date,
			Provisional: point.Current || point.DisplayValuation.Quality.Coverage == domain.CoverageProvisional}
		if total := portfolioTrendTotal(point); total != nil {
			item.Value, item.Text = wealthChartValue(total.Amount, false), valuationMoney(view, *total, false)
		}
		item.Detail = portfolioTrendDetails(view, point)
		if !item.Value.Valid() && point.DisplayValuation.KnownSubtotal != nil {
			item.Detail = append(item.Detail, valuationMoney(view, *point.DisplayValuation.KnownSubtotal, false))
		}
		series.Points = append(series.Points, item)
	}
	spec.Series = []charts.LineSeries{series}
	return spec
}

func portfolioTrendTable(view ui.View, trend domain.PortfolioTrend, selected charts.Selection) charts.Table {
	l := view.Language
	table := charts.Table{Columns: []charts.TableColumn{{Title: l.Text(i18n.ChartDate)},
		{Title: l.Text(i18n.PortfolioValuedHoldings), Numeric: true}, {Title: l.Text(i18n.OverviewKnownSubtotal), Numeric: true},
		{Title: l.Text(i18n.ChartCoverage), Wrap: true}}, Selected: charts.NoRow}
	for i, point := range trend.Points {
		total, subtotal := l.Text(i18n.OverviewNoValue), l.Text(i18n.OverviewNoValue)
		if value := portfolioTrendTotal(point); value != nil {
			total = valuationMoney(view, *value, false)
		}
		if value := point.DisplayValuation.KnownSubtotal; value != nil {
			subtotal = valuationMoney(view, *value, false)
		}
		table.Rows = append(table.Rows, []string{wealthDay(view, point.LocalDate), total, subtotal, strings.Join(portfolioTrendDetails(view, point), " · ")})
		if selected.Item == charts.ID(point.LocalDate) {
			table.Selected = i
		}
	}
	return table
}

func portfolioTrendDetails(view ui.View, point domain.PortfolioTrendPoint) []string {
	l := view.Language
	q := point.DisplayValuation.Quality
	label := i18n.PortfolioValuedHoldings
	switch {
	case portfolioTrendTotal(point) == nil:
		label = i18n.OverviewNoValue
		if point.DisplayValuation.KnownSubtotal != nil {
			label = i18n.OverviewKnownSubtotal
		}
	case point.Current:
		label = i18n.OverviewCurrentEstimate
	case q.CarriedCount > 0 || q.Coverage == domain.CoverageProvisional || q.Coverage == domain.CoverageUnavailable:
		label = i18n.WealthHistoryProvisional
	case q.Coverage == domain.CoverageVerified:
		label = i18n.PortfolioTrendVerified
	}
	parts := []string{l.Text(label)}
	if count := max(point.MissingCount, q.UnpricedCount); count > 0 {
		parts = append(parts, fmt.Sprintf(l.Text(i18n.WealthHistoryMissing), count))
	}
	return append(parts, trendQuoteCoverage(view, q)...)
}
