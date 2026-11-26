package desktop

import (
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type instrumentHistoryTab uint8

const (
	instrumentPriceHistory instrumentHistoryTab = iota
	instrumentFXHistory
)

var instrumentHistoryLabels = [...]i18n.MessageID{i18n.PortfolioPriceHistory, i18n.PortfolioFXHistory}

type instrumentHistoryVisit struct {
	window             analysisDateWindow
	filter             int
	reverse, tableOpen bool
	tableScroll        layout.Position
}

func (d *portfolioInstrumentDetail) savedVisit() instrumentDetailVisit {
	visit := d.visit
	visit.performance = d.performance.visit()
	for i, history := range d.histories {
		if history != nil {
			visit.histories[i] = instrumentHistoryVisit{window: history.visit.window, filter: history.visit.filter,
				reverse: history.visit.reverse, tableOpen: history.table.open, tableScroll: history.table.list.Position}
		}
	}
	return visit
}

func (d *portfolioInstrumentDetail) restoreVisit(visit instrumentDetailVisit) {
	d.visit = visit
	d.performance.restore(visit.performance)
}

func (d *portfolioInstrumentDetail) retireReads() {
	d.visit = d.savedVisit()
	for i, history := range d.histories {
		if history != nil {
			history.retireRead()
			d.histories[i] = nil
		}
	}
	d.performance.series.reset()
	d.performance.returns.reset()
}

func (p *portfolioPage) instrumentHistoryRows(view ui.View, instrument domain.Instrument) []layout.Widget {
	detail, l := &p.detail, view.Language
	base := p.current.base
	hasFX := base != "" && instrument.QuoteCurrency != base
	if !hasFX {
		detail.visit.historyTab = instrumentPriceHistory
	}
	tab := detail.visit.historyTab
	var header layout.Widget = view.Body(ui.TextHeading, l.Text(i18n.PortfolioPriceHistory), view.Colors.Ink)
	if hasFX {
		header = func(gtx layout.Context) layout.Dimensions {
			return view.TabTrack(gtx, len(instrumentHistoryLabels), func(gtx layout.Context, i int) layout.Dimensions {
				button := &detail.historyTabs[i]
				next := instrumentHistoryTab(i)
				if button.Clicked(gtx) && next != detail.visit.historyTab {
					detail.visit.historyTab = next
					gtx.Execute(op.InvalidateCmd{})
				}
				return view.TabSegment(button, l.Text(instrumentHistoryLabels[i]), next == detail.visit.historyTab)(gtx)
			})
		}
	}
	history := detail.histories[tab]
	if history == nil {
		state := detail.visit.histories[tab]
		target := marketQuoteHistoryTarget{instrument: instrument.ID, name: instrument.DisplayLabel()}
		if tab == instrumentFXHistory {
			target = marketQuoteHistoryTarget{pair: marketFXPair{instrument.QuoteCurrency, base}}
		}
		history = &marketQuoteHistory{visit: marketQuoteHistoryVisit{target: target, window: state.window, filter: state.filter, reverse: state.reverse}}
		history.table.open, history.table.list.Position = state.tableOpen, state.tableScroll
		detail.histories[tab] = history
		detail.market.readQuoteHistory(history)
	}
	history.update()
	controls := func(gtx layout.Context) layout.Dimensions {
		var fields []ui.ColumnChild
		if history.result != nil {
			fields = append(fields, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				return detail.market.quoteHistoryControls(gtx, view, history, false, false)
			}))
		} else {
			status := i18n.MarketLoading
			if history.err != nil {
				status = i18n.MarketFailed
			}
			fields = append(fields, ui.Rigid(view.Message(l.Text(status), view.Colors.SecondaryInk)))
		}
		if history.err != nil {
			button := &history.sheet.action
			if button.Clicked(gtx) {
				detail.market.readQuoteHistory(history)
				gtx.Execute(op.InvalidateCmd{})
			}
			fields = append(fields, ui.Rigid(returnMonthButton(view, button, i18n.HistoryRetry, nil, true)))
		}
		return overviewBreakdownCard(gtx, view, fields)
	}
	rows := []layout.Widget{header, controls}
	if chart := quoteHistoryChart(view, history); chart != nil {
		rows = append(rows, chart)
	}
	return rows
}
