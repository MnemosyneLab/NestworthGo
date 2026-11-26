package desktop

import (
	"fmt"
	"slices"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"github.com/shopspring/decimal"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

const wealthTrendSeries charts.ID = "wealth-trend"

var wealthTrendMetrics = [...]i18n.MessageID{i18n.NetWorth, i18n.Assets, i18n.Liabilities}

type wealthTrend struct {
	metric   int
	toggles  [len(wealthTrendMetrics)]widget.Clickable
	selected charts.Selection
	chart    charts.LineChart
	days     map[string]*widget.Clickable
	from     widget.Clickable
	to       widget.Clickable
	open     widget.Clickable
}

func wealthTrendLine(view ui.View, data application.HistoricalOverviewSeriesResult, metric i18n.MessageID) charts.LineSpec {
	l := view.Language
	unit := domain.CurrencyCode(data.Overview.Left.Currency)
	from, _ := ui.ParseCivilDay(data.From)
	to, _ := ui.ParseCivilDay(data.To)
	label := l.Text(metric)
	series := charts.LineSeries{ID: wealthTrendSeries, Name: label, Points: make([]charts.LinePoint, 0, len(data.Points))}
	for i := range data.Points {
		p := &data.Points[i]
		date, ok := ui.ParseCivilDay(p.Date)
		if !ok {
			continue
		}
		series.Points = append(series.Points, wealthTrendPoint(view, p, date, unit, metric))
	}
	return charts.LineSpec{Label: l.Text(i18n.WealthTrendChart),
		Summary: label + " · " + wealthDay(view, data.From) + " – " + wealthDay(view, data.To) + " · " + string(unit),
		Series:  []charts.LineSeries{series}, From: from, To: to, Height: 260, Area: true,
		FormatTime: func(date time.Time) string { return ui.FormatDay(date, l, view.DateFormat) },
		FormatTick: func(date time.Time) string { return ui.FormatDay(date, l, view.DateFormat) },
		FormatValue: func(number float64) string {
			return valuationMoney(view, domain.MoneyView{Currency: unit, Amount: decimal.NewFromFloat(number).String()}, false)
		},
		Messages: charts.Messages{Empty: l.Text(i18n.WealthHistoryEmpty), Missing: l.Text(i18n.OverviewNoValue), Selected: l.Text(i18n.ChartSelected)}}
}

func wealthTrendPoint(view ui.View, p *application.HistoricalSeriesPoint, date time.Time, unit domain.CurrencyCode, metric i18n.MessageID) charts.LinePoint {
	l := view.Language
	point := charts.LinePoint{ID: charts.ID(p.Date), Time: date}
	if !p.Available {
		if p.GapReason == "nonexistent_civil_date" {
			point.Detail = []string{l.Text(i18n.WealthHistorySkippedDate)}
		}
		return point
	}
	display, legacy, connect := p.DisplayValuation.NetWorth, p.NetWorth, p.ConnectNetWorth
	switch metric {
	case i18n.Assets:
		display, legacy, connect = p.DisplayValuation.Assets, p.Assets, p.ConnectAssets
	case i18n.Liabilities:
		display, legacy, connect = p.DisplayValuation.Liabilities, p.Liabilities, p.ConnectLiabilities
	}
	amount := display.Total
	if amount == nil && display.Quality.Availability == "" && legacy != nil {
		amount = &domain.MoneyView{Currency: unit, Amount: *legacy}
	}
	point.BreakBefore = !connect
	point.Provisional = display.Quality.Coverage == domain.CoverageProvisional
	if amount != nil {
		if value, err := decimal.NewFromString(amount.Amount); err == nil {
			if metric == i18n.Liabilities {
				value = value.Abs().Neg()
			}
			geometry := value.InexactFloat64()
			if geometry != 0 || value.IsZero() {
				point.Value = charts.Num(geometry)
			}
			point.Text = valuationMoney(view, *amount, metric == i18n.Liabilities)
		}
	}
	if amount == nil && display.KnownSubtotal != nil {
		point.Detail = append(point.Detail, wealthAmount(view, metric, display))
	}
	if point.Provisional {
		point.Detail = append(point.Detail, l.Text(i18n.WealthHistoryProvisional))
	}
	point.Detail = append(point.Detail, wealthQualityCoverage(view, p, display.Quality))
	for _, missing := range p.Missing[:min(3, len(p.Missing))] {
		point.Detail = append(point.Detail, wealthMissing(l, missing))
	}
	return point
}

func (p *wealthHistoryPage) wealthTrendControls(gtx layout.Context, view ui.View) []ui.ColumnChild {
	t, l := &p.trend, view.Language
	for i := range t.toggles {
		if t.toggles[i].Clicked(gtx) {
			t.metric = i
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	result := p.current.series
	spec := wealthTrendLine(view, result, wealthTrendMetrics[t.metric])
	rows := []ui.ColumnChild{
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return (ui.Wrap{Gap: 8}).Layout(gtx, len(t.toggles), func(gtx layout.Context, i int) layout.Dimensions {
				return analysisToggle(view, &t.toggles[i], l.Text(wealthTrendMetrics[i]), t.metric == i)(gtx)
			})
		}),
		ui.Rigid(analysisLineCard(view, &t.chart, spec, &t.selected)),
	}
	selected := slices.IndexFunc(result.Points, func(point application.HistoricalSeriesPoint) bool {
		return string(t.selected.Item) == point.Date
	})
	if selected < 0 {
		return rows
	}
	point := result.Points[selected]
	if p.applyWealthTrendAction(gtx, point) {
		gtx.Execute(op.InvalidateCmd{})
	}
	return append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
		return (ui.Column{Gap: 5}).Layout(gtx,
			ui.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.WealthTrendSelectedDate), wealthDay(view, point.Date)), view.Colors.Ink)),
			ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				buttons := []layout.Widget{
					returnMonthButton(view, &t.from, i18n.WealthTrendSelectA, nil, point.Available),
					returnMonthButton(view, &t.to, i18n.WealthTrendSelectB, nil, point.Available),
					returnMonthButton(view, &t.open, i18n.WealthTrendOpenDay, nil, point.Available),
				}
				return (ui.Wrap{Gap: 8}).Layout(gtx, len(buttons), func(gtx layout.Context, index int) layout.Dimensions {
					return buttons[index](gtx)
				})
			}))
	}))
}

func (p *wealthHistoryPage) applyWealthTrendAction(gtx layout.Context, point application.HistoricalSeriesPoint) bool {
	if !point.Available {
		return false
	}
	t := &p.trend
	next := p.current.rangeRead
	switch {
	case t.from.Clicked(gtx):
		next.from = point.Date
	case t.to.Clicked(gtx):
		next.to = point.Date
	case t.open.Clicked(gtx):
		now := p.clock()
		p.accounts.draft = wealthAccountDates{from: point.Date, to: p.origin.lastClosed(now).Format(time.DateOnly)}
		p.lens = wealthAccountsLens
		p.readAccounts(p.accounts.draft)
		return true
	default:
		return false
	}
	if !p.origin.permits(next, p.clock()) {
		return false
	}
	p.draft = next
	p.readSeries(next)
	return true
}
