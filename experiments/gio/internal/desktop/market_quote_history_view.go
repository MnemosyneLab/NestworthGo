package desktop

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"gioui.org/io/clipboard"
	"gioui.org/io/event"
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

var marketHistorySourceLabels = [...]i18n.MessageID{
	i18n.QuoteHistoryAllSources, i18n.TransferQuoteManual, i18n.TransferQuoteProvider, i18n.TransferQuoteAgent,
}

func (p *marketDataPage) quoteHistoryAction(gtx layout.Context, view ui.View, target marketQuoteHistoryTarget, button *widget.Clickable, enabled bool) layout.Widget {
	if enabled && button.Clicked(gtx) {
		p.openQuoteHistory(marketQuoteHistoryVisit{target: target, source: button, window: marketQuoteFindingWindow(target.finding)})
		gtx.Execute(op.InvalidateCmd{})
	}
	return returnMonthButton(view, button, i18n.QuoteHistoryOpen, nil, enabled)
}

func (p *marketDataPage) marketHistoryAction(gtx layout.Context, view ui.View, key string, target marketQuoteHistoryTarget) []layout.Widget {
	if p.quoteHistoryReader == nil {
		return nil
	}
	if p.historyActions == nil {
		p.historyActions = make(map[string]*widget.Clickable)
	}
	return []layout.Widget{p.quoteHistoryAction(gtx, view, target, portfolioButton(p.historyActions, key), p.request == nil && p.err == nil)}
}

func (p *marketDataPage) layoutQuoteHistory(gtx layout.Context, view ui.View, mayOpenHealth bool) (event.Tag, *healthFindingFocus) {
	d := p.quoteHistory
	d.update()
	tags := []event.Tag{&d.swap, &d.showAll, &d.table.toggle, &d.table.list, &d.health, &d.attribution, &d.chart, &d.dates.previous, &d.dates.next}
	for i := range d.dates.presets {
		tags = append(tags, &d.dates.presets[i])
	}
	for i := range d.filters {
		tags = append(tags, &d.filters[i])
	}
	if d.sheet.dismissed(gtx, tags...) {
		p.closeQuoteHistory()
		gtx.Execute(op.InvalidateCmd{})
		return d.visit.source, nil
	}
	if mayOpenHealth && d.health.Clicked(gtx) && d.result != nil {
		target := d.visit.target
		focus := healthFindingFocus{from: d.visit.window.from, to: d.visit.window.to, label: d.name()}
		focus.instrument = target.instrument.String()
		focus.currencyA, focus.currencyB = target.pair.first.String(), target.pair.second.String()
		if d.result.series.Range == domain.TrendAllTime {
			focus.from, focus.to = "", ""
		}
		gtx.Execute(op.InvalidateCmd{})
		return nil, &focus
	}
	title := fmt.Sprintf(view.Language.Text(i18n.QuoteHistoryTitle), d.name())
	action := i18n.NoMessage
	if d.err != nil {
		action = i18n.HistoryRetry
		if d.sheet.action.Clicked(gtx) {
			p.readQuoteHistory(d)
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	d.sheet.layout(gtx, view, title, p.quoteHistoryContent(view, d, mayOpenHealth), action)
	return nil, nil
}

func (d *marketQuoteHistory) name() string {
	if d.visit.target.instrument != "" {
		if d.result != nil && d.result.instrument != nil {
			return d.result.instrument.DisplayLabel()
		}
		return d.visit.target.name
	}
	pair := d.visit.direction()
	return pair.first.String() + "/" + pair.second.String()
}

func (p *marketDataPage) quoteHistoryContent(view ui.View, d *marketQuoteHistory, mayOpenHealth bool) layout.Widget {
	l := view.Language
	if d.result == nil {
		status := i18n.MarketLoading
		if d.err != nil {
			status = i18n.MarketFailed
		}
		return view.Message(l.Text(status), view.Colors.SecondaryInk)
	}
	chart := quoteHistoryChart(view, d)
	count := 1
	if chart != nil {
		count = 2
	}
	return func(gtx layout.Context) layout.Dimensions {
		return analysisResultList(gtx, view, &d.sheet.list, l.Text(i18n.QuoteHistoryTitleList), count, func(gtx layout.Context, index int) layout.Dimensions {
			switch index {
			case 0:
				return p.quoteHistoryControls(gtx, view, d, mayOpenHealth, true)
			case 1:
				return chart(gtx)
			default:
				return layout.Dimensions{}
			}
		})
	}
}

func quoteHistoryChart(view ui.View, d *marketQuoteHistory) layout.Widget {
	data := d.result
	if data == nil || d.request != nil || d.err != nil || len(data.series.Observations) == 0 {
		return nil
	}
	pair := d.visit.direction()
	return func(gtx layout.Context) layout.Dimensions {
		if d.request != nil {
			return layout.Dimensions{}
		}
		spec := quoteHistoryLine(view, data, pair)
		table := func() charts.Table { return quoteHistoryTable(view, data, pair) }
		if len(spec.Series) == 0 {
			return overviewBreakdownCard(gtx, view, []ui.ColumnChild{
				ui.Rigid(d.table.layout(view, len(data.series.Observations), table)),
			})
		}
		return analysisLineCardTable(view, &d.chart, spec, &d.selected, &d.table, len(data.series.Observations), func(charts.LineSpec) charts.Table {
			return table()
		})(gtx)
	}
}

func (p *marketDataPage) quoteHistoryControls(gtx layout.Context, view ui.View, d *marketQuoteHistory, mayOpenHealth, showCurrent bool) layout.Dimensions {
	data, l := d.result, view.Language
	change := func() {
		d.selected = charts.Selection{}
		p.readQuoteHistory(d)
		gtx.Execute(op.InvalidateCmd{})
	}
	for i := range d.filters {
		if d.filters[i].Clicked(gtx) && d.visit.filter != i {
			d.visit.filter = i
			change()
		}
	}
	pair := d.visit.direction()
	if pair.first != "" && d.swap.Clicked(gtx) {
		d.visit.reverse = !d.visit.reverse
		pair = d.visit.direction()
		change()
	}
	if d.showAll.Clicked(gtx) && data.series.OutsideRange && d.request == nil && d.err == nil {
		d.selectWindow(analysisDateWindow{marketQuoteEarliest, data.latest})
		change()
	}
	message := func(text string) ui.ColumnChild {
		return ui.Rigid(view.Message(text, view.Colors.SecondaryInk))
	}
	rows := []ui.ColumnChild{}
	if scope := d.visit.target.finding; scope.active() && scope.from != "" {
		rows = append(rows, message(fmt.Sprintf(l.Text(i18n.QuoteHistoryFinding), healthDateRange(view, scope.from, scope.to))))
	}
	rangeLabel := healthDateRange(view, d.visit.window.from, d.visit.window.to)
	if d.visit.window.from == marketQuoteEarliest && d.visit.window.to == data.latest {
		rangeLabel = l.Text(i18n.AnalysisWholeHistory)
	}
	rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
		return d.dates.layout(gtx, view, d.visit.window, marketQuoteEarliest, data.latest, func(window analysisDateWindow) {
			d.selectWindow(window)
			change()
		})
	}), message(rangeLabel),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return (ui.Wrap{Gap: 6}).Layout(gtx, len(d.filters), func(gtx layout.Context, i int) layout.Dimensions {
				return analysisToggle(view, &d.filters[i], l.Text(marketHistorySourceLabels[i]), i == d.visit.filter)(gtx)
			})
		}))
	var actions []layout.Widget
	if mayOpenHealth {
		actions = append(actions, returnMonthButton(view, &d.health, i18n.HealthOpen, nil, true))
	}
	if len(actions) > 0 {
		rows = append(rows, ui.Rigid(marketActionRow(view, actions)))
	}
	if instrument := data.instrument; instrument != nil {
		source := marketSourceLabel(l, instrument.QuoteSource)
		if instrument.ProviderKey != nil && instrument.QuoteSource == domain.QuoteSourceProvider {
			source += " · " + *instrument.ProviderKey
		}
		rows = append(rows, message(fmt.Sprintf(l.Text(i18n.QuoteHistoryConfigured), source)))
		if instrument.UsesMetalConversion() {
			rows = append(rows, message(l.Text(i18n.QuoteHistoryMetalReference)))
		}
		if instrument.ProviderKey != nil && *instrument.ProviderKey == application.CoinGeckoProviderKey {
			rows = append(rows, ui.Rigid(quoteHistoryAttribution(view, &d.attribution, &d.attributionCopied)))
		}
	}
	if pair.first != "" {
		rows = append(rows, message(fmt.Sprintf(l.Text(i18n.QuoteHistoryConfigured), marketSourceLabel(l, data.fxSource))),
			message(l.Text(i18n.MarketFXDescription)),
			message(fmt.Sprintf(l.Text(i18n.QuoteHistoryDirection), pair.first, pair.second)),
			ui.Rigid(returnMonthButton(view, &d.swap, i18n.QuoteHistorySwap, nil, true)))
	}
	if showCurrent {
		rows = append(rows, ui.Rigid(view.Body(ui.TextLabel, l.Text(i18n.QuoteHistoryCurrent), view.Colors.Ink)))
		if data.current != nil {
			rows = append(rows, message(quoteHistoryAmount(view, *data.current, data.storedPair, data.instrument)))
			for _, line := range quoteHistoryEvidence(view, *data.current) {
				rows = append(rows, message(line))
			}
		} else {
			rows = append(rows, message(l.Text(i18n.MarketNoQuote)))
		}
	}
	var status i18n.MessageID
	switch {
	case d.request != nil:
		status = i18n.MarketLoading
	case d.err != nil:
		status = i18n.MarketFailed
	case data.series.OutsideRange:
		status = i18n.QuoteHistoryRangeEmpty
	case len(data.series.Observations) == 0:
		status = i18n.QuoteHistoryEmpty
	default:
		rows = append(rows, message(fmt.Sprintf(l.Text(i18n.QuoteHistoryCoverage), len(data.series.Observations), data.observedDays)))
		if slices.ContainsFunc(data.series.Observations, func(point domain.QuoteSeriesPoint) bool {
			return data.series.DisplayCurrency != "" && point.Currency != "" && point.Currency != data.series.DisplayCurrency
		}) {
			rows = append(rows, message(l.Text(i18n.QuoteHistoryOffCurrency)))
		}
	}
	if status != i18n.NoMessage {
		rows = append(rows, message(l.Text(status)))
	}
	if status == i18n.QuoteHistoryRangeEmpty {
		rows = append(rows, ui.Rigid(returnMonthButton(view, &d.showAll, i18n.QuoteHistoryShowAll, nil, true)))
	}
	return (ui.Column{Gap: 7}).Layout(gtx, rows...)
}

func quoteHistoryAttribution(view ui.View, button *widget.Clickable, copied *bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := view.Language
		if button.Clicked(gtx) {
			gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(l.Text(i18n.QuoteHistoryCoinGeckoURL)))})
			*copied = true
		}
		rows := []ui.ColumnChild{ui.Rigid(returnMonthButton(view, button, i18n.QuoteHistoryCoinGecko, nil, true))}
		if *copied {
			rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.QuoteHistoryCoinGeckoCopied), view.Colors.SecondaryInk)))
		}
		return (ui.Column{Gap: 7}).Layout(gtx, rows...)
	}
}

// quoteHistoryTable lists every saved observation in the range, as the Wails
// quote chart's data table does: when it was observed, the exact value and its
// evidence.
func quoteHistoryTable(view ui.View, data *marketQuoteHistoryRead, pair marketFXPair) charts.Table {
	return quoteObservationsTable(view, data.series.Observations, pair, data.instrument)
}

func quoteObservationsTable(view ui.View, observations []domain.QuoteSeriesPoint, pair marketFXPair, instrument *domain.Instrument) charts.Table {
	l := view.Language
	value := i18n.ChartPrice
	if pair.first != "" {
		value = i18n.ChartRate
	}
	table := charts.Table{Columns: []charts.TableColumn{{Title: l.Text(i18n.ChartObservedAt)}, {Title: l.Text(value), Numeric: true},
		{Title: l.Text(i18n.ChartDetails), Wrap: true}}, Selected: charts.NoRow}
	for _, point := range observations {
		observed := wealthDay(view, point.EffectiveDate)
		if point.TimestampBasis != "date_label" && !point.QuotedAt.IsZero() {
			observed = strings.TrimPrefix(portfolioQuoteStamp(view, "", "", point.QuotedAt), " · ")
		}
		table.Rows = append(table.Rows, []string{observed, quoteHistoryAmount(view, point, pair, instrument),
			strings.Join(quoteHistoryEvidence(view, point), " · ")})
	}
	return table
}

func quoteHistoryAmount(view ui.View, point domain.QuoteSeriesPoint, pair marketFXPair, instrument *domain.Instrument) string {
	number := portfolioDecimal(view, point.Value)
	if pair.first != "" {
		return fmt.Sprintf(view.Language.Text(i18n.QuoteHistoryRate), pair.first, number, pair.second)
	}
	amount := strings.TrimSpace(point.Currency.String() + " " + number)
	if instrument != nil && instrument.UsesMetalConversion() {
		if instrument.QuantityUnit == "g" {
			amount = fmt.Sprintf(view.Language.Text(i18n.QuoteHistoryPerGram), amount)
		} else if instrument.QuantityUnit == "troy_oz" {
			amount = fmt.Sprintf(view.Language.Text(i18n.QuoteHistoryPerOunce), amount)
		}
	}
	return amount
}

func quoteHistoryEvidence(view ui.View, point domain.QuoteSeriesPoint) []string {
	l := view.Language
	source := marketSourceLabel(l, point.SourceKind)
	if point.SourceKind == domain.QuoteSourceProvider && point.SourceKey != "" && point.SourceKey != string(point.SourceKind) {
		source += " · " + point.SourceKey
	}
	if point.Delayed {
		source += " · " + l.Text(i18n.MarketDelayed)
	}
	lines := []string{l.Text(quoteHistoryKind(point)) + " · " + source}
	if point.EffectiveDate != "" {
		lines = append(lines, fmt.Sprintf(l.Text(i18n.QuoteHistoryEffective), wealthDay(view, point.EffectiveDate)))
	}
	// A civil date label is not an observed instant. Policy-derived instants
	// remain separately identified from exchange timestamps.
	if point.TimestampBasis != "date_label" && !point.QuotedAt.IsZero() {
		label := i18n.QuoteHistoryQuoted
		if point.TimestampBasis == string(application.TimestampBasisPolicyDerived) {
			label = i18n.QuoteHistoryPolicyTime
		}
		stamp := strings.TrimPrefix(portfolioQuoteStamp(view, "", "", point.QuotedAt), " · ")
		lines = append(lines, fmt.Sprintf(l.Text(label), stamp))
	}
	return lines
}

func quoteHistoryKind(point domain.QuoteSeriesPoint) i18n.MessageID {
	switch {
	case point.PriceBasis == "agent_unit_nav_v1":
		return i18n.QuoteHistoryNAV
	case point.SourceKind == domain.QuoteSourceManual:
		return i18n.QuoteHistoryManual
	case point.ObservationKind == string(application.FXObservationLatest):
		return i18n.QuoteHistoryLatest
	case point.ObservationKind == string(application.InstrumentObservationRealtime):
		return i18n.QuoteHistoryRealtime
	case point.ObservationKind == string(application.FXObservationDailyReference) ||
		point.ObservationKind == string(application.InstrumentObservationClose) && point.SourceKey == application.CoinGeckoProviderKey:
		return i18n.QuoteHistoryDaily
	case point.ObservationKind == string(application.InstrumentObservationClose):
		return i18n.QuoteHistoryClose
	default:
		return i18n.QuoteHistoryOther
	}
}

func quoteHistoryLine(view ui.View, data *marketQuoteHistoryRead, pair marketFXPair) charts.LineSpec {
	l := view.Language
	spec := charts.LineSpec{Label: l.Text(i18n.QuoteHistoryChart),
		Summary: l.Text(i18n.QuoteHistoryChartHelp), Height: 230,
		FormatTime: func(day time.Time) string { return ui.FormatDay(day.In(data.location), l, view.DateFormat) },
		FormatValue: func(value float64) string {
			return portfolioDecimal(view, decimal.NewFromFloat(value).String())
		},
		Messages: charts.Messages{Empty: l.Text(i18n.QuoteHistoryEmpty), Missing: l.Text(i18n.MarketNoQuote), Selected: l.Text(i18n.ChartSelected)}}
	if data.series.Range != domain.TrendAllTime {
		spec.From, _ = ui.ParseCivilDay(data.window.from)
		spec.To, _ = ui.ParseCivilDay(data.window.to)
		spec.From = time.Date(spec.From.Year(), spec.From.Month(), spec.From.Day(), 0, 0, 0, 0, data.location)
		spec.To = time.Date(spec.To.Year(), spec.To.Month(), spec.To.Day(), 0, 0, 0, 0, data.location).AddDate(0, 0, 1)
	}
	groups := make(map[string]int)
	for _, obs := range data.series.Points {
		if display := data.series.DisplayCurrency; display != "" && obs.Currency != "" && obs.Currency != display {
			continue
		}
		value, err := decimal.NewFromString(obs.Value)
		if err != nil {
			continue
		}
		kind := quoteHistoryKind(obs)
		stamp := obs.QuotedAt
		if obs.TimestampBasis == "date_label" {
			if day, ok := ui.ParseCivilDay(obs.EffectiveDate); ok {
				stamp = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, data.location)
			}
		}
		if stamp.IsZero() {
			continue
		}
		geometry := value.InexactFloat64()
		if geometry == 0 && !value.IsZero() {
			continue
		}
		key := fmt.Sprintf("%d/%s", kind, obs.Currency)
		index, found := groups[key]
		if !found {
			index = len(spec.Series)
			groups[key] = index
			spec.Series = append(spec.Series, charts.LineSeries{ID: charts.ID(key), Name: l.Text(kind)})
		}
		point := charts.LinePoint{ID: charts.ID(obs.ID), Time: stamp, Value: charts.Num(geometry), Detail: quoteHistoryEvidence(view, obs),
			Text: quoteHistoryAmount(view, obs, pair, data.instrument), BreakBefore: kind == i18n.QuoteHistoryLatest || kind == i18n.QuoteHistoryRealtime}
		spec.Series[index].Points = append(spec.Series[index].Points, point)
	}
	for i := range spec.Series {
		slices.SortStableFunc(spec.Series[i].Points, func(a, b charts.LinePoint) int { return a.Time.Compare(b.Time) })
	}
	return spec
}
