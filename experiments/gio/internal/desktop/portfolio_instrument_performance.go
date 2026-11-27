package desktop

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"github.com/shopspring/decimal"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type instrumentPerformanceReader interface {
	InstrumentQuotePerformanceSeries(context.Context, domain.InstrumentID, int) (domain.QuotePerformanceSeries, error)
	InstrumentQuoteReturns(context.Context, domain.InstrumentID) (domain.QuotePerformanceReport, error)
}

type instrumentPerformancePeriod uint8

const (
	performanceMonth instrumentPerformancePeriod = iota
	performanceWeek
	performanceQuarter
	performanceHalfYear
	performanceYear
	performanceAll
)

var instrumentPerformancePeriods = [...]struct {
	period       instrumentPerformancePeriod
	days         int
	short, label i18n.MessageID
}{
	{performanceWeek, 7, i18n.AnalysisWeekShort, i18n.AnalysisLastWeek},
	{performanceMonth, 30, i18n.AnalysisMonthShort, i18n.AnalysisLastMonth},
	{performanceQuarter, 90, i18n.AnalysisQuarterShort, i18n.AnalysisLastQuarter},
	{performanceHalfYear, 180, i18n.AnalysisHalfYearShort, i18n.AnalysisLastHalfYear},
	{performanceYear, 365, i18n.AnalysisYearShort, i18n.AnalysisLastYear},
	{performanceAll, 0, i18n.AnalysisAllDates, i18n.AnalysisWholeHistory},
}

type instrumentPerformanceVisit struct {
	period      instrumentPerformancePeriod
	selected    charts.Selection
	tableOpen   bool
	tableScroll layout.Position
	infoOpen    bool
}

type instrumentPerformance struct {
	period            instrumentPerformancePeriod
	buttons           [len(instrumentPerformancePeriods)]widget.Clickable
	series            instrumentPerformanceRead[domain.QuotePerformanceSeries]
	returns           instrumentPerformanceRead[domain.QuotePerformanceReport]
	chart             charts.LineChart
	table             chartDataTable
	selected          charts.Selection
	info              widget.Clickable
	infoOpen          bool
	attribution       widget.Clickable
	attributionCopied bool
}

func (p *instrumentPerformance) visit() instrumentPerformanceVisit {
	return instrumentPerformanceVisit{period: p.period, selected: p.selected, tableOpen: p.table.open, tableScroll: p.table.list.Position, infoOpen: p.infoOpen}
}

func (p *instrumentPerformance) restore(v instrumentPerformanceVisit) {
	p.period, p.selected = v.period, v.selected
	p.table.open, p.table.list.Position = v.tableOpen, v.tableScroll
	p.infoOpen = v.infoOpen
	p.attributionCopied = false
}

// The chart period and the fixed returns are independent reads. Changing the
// chart period cancels that request while the fixed returns remain available.
type instrumentPerformanceRead[T any] struct {
	request <-chan historyResult[T]
	cancel  context.CancelFunc
	value   *T
	err     error
	retry   widget.Clickable
}

func (r *instrumentPerformanceRead[T]) reset() {
	if r.cancel != nil {
		r.cancel()
	}
	r.request, r.cancel, r.value, r.err = nil, nil, nil, nil
}

func (r *instrumentPerformanceRead[T]) load(parent context.Context, workers *sync.WaitGroup, invalidate func(), read func(context.Context) (T, error)) {
	r.reset()
	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel
	r.request = runHistoryRead(workers, invalidate, func() (T, error) { return read(ctx) })
}

func (r *instrumentPerformanceRead[T]) update() {
	select {
	case result := <-r.request:
		r.reset()
		r.err = result.err
		if result.err == nil {
			r.value = &result.value
		}
	default:
	}
}

func (p *portfolioPage) instrumentPerformanceRows(view ui.View, instrument domain.Instrument) []layout.Widget {
	perf, l := &p.detail.performance, view.Language
	reader, supported := p.service.(instrumentPerformanceReader)
	if !supported {
		return []layout.Widget{view.Message(l.Text(i18n.PortfolioPerformanceUnsupported), view.Colors.SecondaryInk)}
	}
	if p.request != nil || p.err != nil {
		return nil
	}
	rows := perf.seriesRows(view, instrument, reader, p.ctx, &p.workers, p.invalidate)
	readReturns := func(ctx context.Context) (domain.QuotePerformanceReport, error) {
		return reader.InstrumentQuoteReturns(ctx, instrument.ID)
	}
	perf.returns.update()
	if perf.returns.request == nil && perf.returns.value == nil && perf.returns.err == nil {
		perf.returns.load(p.ctx, &p.workers, p.invalidate, readReturns)
	}
	if perf.returns.err != nil {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			if perf.returns.retry.Clicked(gtx) {
				perf.returns.load(p.ctx, &p.workers, p.invalidate, readReturns)
				gtx.Execute(op.InvalidateCmd{})
			}
			return overviewBreakdownCard(gtx, view, []ui.ColumnChild{
				ui.Rigid(view.Message(l.Text(i18n.PortfolioPerformanceReturnsFailed), view.Colors.ErrorInk)),
				ui.Rigid(returnMonthButton(view, &perf.returns.retry, i18n.HistoryRetry, nil, true)),
			})
		})
	} else if perf.returns.value != nil && len(perf.returns.value.Items) > 0 {
		rows = append(rows, instrumentPerformanceReturns(view, perf.returns.value.Items))
	}
	return rows
}

// seriesRows is shared by instrument detail and the expanded account holding.
func (perf *instrumentPerformance) seriesRows(view ui.View, instrument domain.Instrument, reader instrumentPerformanceReader, ctx context.Context, workers *sync.WaitGroup, invalidate func()) []layout.Widget {
	l := view.Language
	period := perf.period
	var days int
	for _, choice := range instrumentPerformancePeriods {
		if choice.period == period {
			days = choice.days
			break
		}
	}
	readSeries := func(ctx context.Context) (domain.QuotePerformanceSeries, error) {
		return reader.InstrumentQuotePerformanceSeries(ctx, instrument.ID, days)
	}
	perf.series.update()
	if perf.series.request == nil && perf.series.value == nil && perf.series.err == nil {
		perf.series.load(ctx, workers, invalidate, readSeries)
	}
	data := perf.series.value
	return []layout.Widget{func(gtx layout.Context) layout.Dimensions {
		fields := []ui.ColumnChild{ui.Rigid(perf.controls(view, data, period))}
		if perf.infoOpen {
			fields = append(fields, ui.Rigid(view.Message(l.Text(i18n.PortfolioPerformanceBasis), view.Colors.SecondaryInk)))
			fields = append(fields, ui.Rigid(view.Message(l.Text(i18n.PortfolioInstrumentMarketNote), view.Colors.SecondaryInk)))
			if instrument.UsesMetalConversion() {
				fields = append(fields, ui.Rigid(view.Message(l.Text(i18n.QuoteHistoryMetalReference), view.Colors.SecondaryInk)))
			}
		}
		switch {
		case perf.series.err != nil:
			if perf.series.retry.Clicked(gtx) {
				perf.series.load(ctx, workers, invalidate, readSeries)
				gtx.Execute(op.InvalidateCmd{})
			}
			fields = append(fields, ui.Rigid(view.Message(l.Text(i18n.MarketFailed), view.Colors.ErrorInk)),
				ui.Rigid(returnMonthButton(view, &perf.series.retry, i18n.HistoryRetry, nil, true)))
		case data == nil || period != perf.period:
			fields = append(fields, ui.Rigid(view.Message(l.Text(i18n.MarketLoading), view.Colors.SecondaryInk)))
		case data.Item == nil && len(data.Points) == 0:
			fields = append(fields,
				ui.Rigid(view.Body(ui.TextHeading, l.Text(i18n.PortfolioPerformanceEmpty), view.Colors.Ink)),
				ui.Rigid(view.Message(l.Text(i18n.PortfolioPerformanceEmptyHint), view.Colors.SecondaryInk)))
		}
		if data != nil && period == perf.period && len(data.Points) > 0 {
			spec := instrumentPerformanceChart(view, instrument, *data)
			observations := make([]domain.QuoteSeriesPoint, len(data.Points))
			for i, point := range data.Points {
				observations[len(data.Points)-1-i] = performanceObservation(point, instrument.QuoteCurrency)
			}
			fields = append(fields, ui.Rigid(analysisLineBodyTable(view, &perf.chart, spec, &perf.selected, &perf.table, len(data.Points), func(charts.LineSpec) charts.Table {
				return quoteObservationsTable(view, observations, marketFXPair{}, &instrument)
			})))
		}
		if instrument.ProviderKey != nil && *instrument.ProviderKey == application.CoinGeckoProviderKey {
			fields = append(fields, ui.Rigid(quoteHistoryAttribution(view, &perf.attribution, &perf.attributionCopied)))
		}
		return instrumentPerformanceCard(gtx, view, fields)
	}}
}

// instrumentPerformanceCard keeps this chart section compact like Wails while
// preserving the same card surface used elsewhere in the investment pages.
func instrumentPerformanceCard(gtx layout.Context, view ui.View, fields []ui.ColumnChild) layout.Dimensions {
	return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: ui.Space16, Bottom: 12, Left: ui.Space16, Right: ui.Space16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return (ui.Column{Gap: ui.Space4}).Layout(gtx, fields...)
		})
	})
}

func (perf *instrumentPerformance) controls(view ui.View, data *domain.QuotePerformanceSeries, period instrumentPerformancePeriod) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := view.Language
		value, ink := "—", view.Colors.SecondaryInk
		date := ""
		if data != nil && period == perf.period {
			if data.Item != nil {
				value, ink = instrumentPerformancePercent(view, data.Item.PeriodPercent, true)
				date = performanceDateRange(view, data.Item.BaselineOn, data.Item.LatestOn)
			} else {
				date = l.Text(performanceUnavailable(data.UnavailableReason))
			}
		}
		if perf.info.Clicked(gtx) {
			perf.infoOpen = !perf.infoOpen
			gtx.Execute(op.InvalidateCmd{})
		}
		info := func(gtx layout.Context) layout.Dimensions {
			view.Focus.Watch(gtx, &perf.info)
			return view.ButtonOf(&perf.info, ui.ButtonSpec{Variant: ui.ButtonGhost, Size: ui.ButtonCompact,
				Name: l.Text(i18n.PortfolioPerformanceInfo), ShortName: l.Text(i18n.PortfolioPerformanceInfo),
				Icon: lucide.Info, Toggle: true, Selected: perf.infoOpen})(gtx)
		}
		metric := func(gtx layout.Context) layout.Dimensions {
			return (ui.Column{Gap: ui.Space4}).Layout(gtx,
				ui.Rigid(view.Body(ui.TextMetricLG, value, ink)),
				ui.Rigid(func(gtx layout.Context) layout.Dimensions {
					dateWidth := marketTextWidth(gtx, view, ui.TextCaption, date)
					return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints = layout.Constraints{Min: image.Pt(dateWidth, 0), Max: image.Pt(dateWidth, gtx.Constraints.Max.Y)}
							return view.Line(gtx, ui.TextCaption, date, view.Colors.SecondaryInk)
						}), layout.Rigid(info))
				}),
			)
		}
		if performanceControlsFit(gtx, view, date) {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space16)}.Layout(gtx,
				layout.Flexed(1, metric), layout.Rigid(perf.periodButtons(view)))
		}
		return (ui.Column{Gap: ui.Space8}).Layout(gtx,
			ui.Rigid(metric), ui.Rigid(perf.periodButtons(view)))
	}
}

func performanceControlsFit(gtx layout.Context, view ui.View, date string) bool {
	periodWidth := 0
	for index, choice := range instrumentPerformancePeriods {
		if index > 0 {
			periodWidth += gtx.Dp(ui.Space4)
		}
		periodWidth += marketTextWidth(gtx, view, ui.TextCaption, view.Language.Text(choice.short)) + 2*gtx.Dp(ui.Space8)
	}
	metricWidth := marketTextWidth(gtx, view, ui.TextMetricLG, "+100.00%")
	dateWidth := marketTextWidth(gtx, view, ui.TextCaption, date)
	infoWidth := gtx.Dp(ui.ControlCompact + ui.Space8)
	return gtx.Constraints.Max.X >= max(metricWidth, dateWidth)+infoWidth+periodWidth+gtx.Dp(ui.Space16)
}

func (p *instrumentPerformance) periodButtons(view ui.View) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return (ui.Wrap{Gap: ui.Space4}).Layout(gtx, len(instrumentPerformancePeriods), func(gtx layout.Context, i int) layout.Dimensions {
			choice, button := instrumentPerformancePeriods[i], &p.buttons[i]
			if button.Clicked(gtx) && p.period != choice.period {
				p.period = choice.period
				p.series.reset()
				p.selected, p.table.list.Position = charts.Selection{}, layout.Position{}
				gtx.Execute(op.InvalidateCmd{})
			}
			variant := ui.ButtonOutline
			if p.period == choice.period {
				variant = ui.ButtonSoft
			}
			l := view.Language
			return view.ButtonOf(button, ui.ButtonSpec{Variant: variant, Size: ui.ButtonCompact, Title: l.Text(choice.short),
				Name: l.Text(choice.label), Toggle: true, Selected: p.period == choice.period})(gtx)
		})
	}
}

func performanceUnavailable(reason string) i18n.MessageID {
	switch reason {
	case "unsupported_instrument":
		return i18n.PortfolioPerformanceUnsupported
	case "incomparable_daily_prices":
		return i18n.PortfolioPerformanceIncomparable
	default:
		return i18n.PortfolioPerformanceNoHistory
	}
}

func instrumentPerformancePercent(view ui.View, value string, signed bool) (string, color.NRGBA) {
	number, err := decimal.NewFromString(value)
	if err != nil {
		return view.Language.Text(i18n.OverviewNoValue), view.Colors.SecondaryInk
	}
	ink, prefix := view.Colors.Ink, ""
	if number.IsPositive() {
		ink = view.Colors.Success
		if signed {
			prefix = "+"
		}
	} else if number.IsNegative() {
		ink = view.Colors.Destructive
	}
	return prefix + portfolioDecimal(view, number.RoundBank(2).StringFixed(2)) + "%", ink
}

func performanceDateRange(view ui.View, from, to string) string {
	return wealthDay(view, from) + " – " + wealthDay(view, to)
}

func instrumentPerformanceReturns(view ui.View, items []domain.QuotePerformance) layout.Widget {
	l := view.Language
	return func(gtx layout.Context) layout.Dimensions {
		return overviewBreakdownCard(gtx, view, []ui.ColumnChild{
			ui.Rigid(view.Body(ui.TextHeading, l.Text(i18n.PortfolioPerformanceReturns), view.Colors.Ink)),
			ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				return (ui.Grid{MinColumn: 170, MaxColumns: 3, Gap: ui.Space12}).Layout(gtx, view, len(items), func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
					item := items[i]
					value, ink := instrumentPerformancePercent(view, item.PeriodPercent, true)
					fields := []ui.ColumnChild{
						ui.Rigid(view.Body(ui.TextCaption, fmt.Sprintf(l.Text(i18n.PortfolioPerformanceDays), item.WindowDays), view.Colors.SecondaryInk)),
						ui.Rigid(view.Body(ui.TextMetricMD, value, ink)),
						ui.Rigid(view.Body(ui.TextCaption, performanceDateRange(view, item.BaselineOn, item.LatestOn), view.Colors.SecondaryInk)),
					}
					if item.AnnualizedPercent != nil {
						annualized, _ := instrumentPerformancePercent(view, *item.AnnualizedPercent, false)
						fields = append(fields, ui.Rigid(view.Body(ui.TextCaption, fmt.Sprintf(l.Text(i18n.PortfolioPerformanceAnnualized), annualized), view.Colors.SecondaryInk)))
					}
					return (ui.Column{Gap: ui.Space4}).Layout(gtx, fields...)
				})
			}),
		})
	}
}

func instrumentPerformanceChart(view ui.View, instrument domain.Instrument, data domain.QuotePerformanceSeries) charts.LineSpec {
	l := view.Language
	spec := charts.LineSpec{Label: l.Text(i18n.ChartPrice) + " · " + instrument.QuoteCurrency.String(), Height: 160,
		Area: true, FormatTime: func(day time.Time) string { return ui.FormatDay(day, l, view.DateFormat) },
		FormatValue: func(value float64) string { return chartAxisNumber(view, value) },
		Messages:    charts.Messages{Empty: l.Text(i18n.PortfolioPerformanceNoHistory), Missing: l.Text(i18n.OverviewNoValue), Selected: l.Text(i18n.ChartSelected)}}
	spec.From, _ = ui.ParseCivilDay(data.From)
	spec.To, _ = ui.ParseCivilDay(data.To)
	series := charts.LineSeries{ID: "daily-instrument-prices", Name: l.Text(i18n.ChartPrice), Color: view.Colors.Accent}
	if data.Item != nil {
		_, series.Color = instrumentPerformancePercent(view, data.Item.PeriodPercent, true)
	}
	for _, point := range data.Points {
		day, ok := ui.ParseCivilDay(point.EffectiveDate)
		if !ok {
			continue
		}
		quote := performanceObservation(point, instrument.QuoteCurrency)
		value := charts.NoValue
		if quote.Currency == instrument.QuoteCurrency {
			value = wealthChartValue(point.Value, false)
		}
		series.Points = append(series.Points, charts.LinePoint{ID: charts.ID(point.EffectiveDate), Time: day, Value: value,
			Text: quoteHistoryAmount(view, quote, marketFXPair{}, &instrument), Detail: quoteHistoryEvidence(view, quote), BreakBefore: point.BreakBefore})
	}
	spec.Series = []charts.LineSeries{series}
	return spec
}

func performanceObservation(point domain.QuotePerformancePoint, currency domain.CurrencyCode) domain.QuoteSeriesPoint {
	if point.Currency != "" {
		currency = point.Currency
	}
	return domain.QuoteSeriesPoint{Currency: currency, ObservationKind: point.ObservationKind, EffectiveDate: point.EffectiveDate,
		PriceBasis: point.PriceBasis, TimestampBasis: point.TimestampBasis, QuotedAt: point.QuotedAt, Value: point.Value,
		SourceKind: point.SourceKind, SourceKey: point.SourceKey, Delayed: point.Delayed}
}
