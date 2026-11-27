package desktop

import (
	"fmt"
	"strings"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
)

type instrumentDetailSection uint8

const (
	instrumentDetailHoldings instrumentDetailSection = iota
	instrumentDetailPerformance
	instrumentDetailData
)

var instrumentDetailLabels = [...]i18n.MessageID{i18n.PortfolioInstrumentHoldings, i18n.PortfolioInstrumentPerformance, i18n.PortfolioInstrumentData}

type instrumentDetailVisit struct {
	section     instrumentDetailSection
	noteOpen    bool
	positions   [len(instrumentDetailLabels)]layout.Position
	historyTab  instrumentHistoryTab
	histories   [len(instrumentHistoryLabels)]instrumentHistoryVisit
	performance instrumentPerformanceVisit
	actionDates analysisDateWindow
}

type portfolioInstrumentDetail struct {
	visit           instrumentDetailVisit
	tabs            [len(instrumentDetailLabels)]widget.Clickable
	historyTabs     [len(instrumentHistoryLabels)]widget.Clickable
	histories       [len(instrumentHistoryLabels)]*marketQuoteHistory
	source, manual  widget.Clickable
	maintain, note  widget.Clickable
	openMaintenance func(healthFindingFocus, event.Tag)
	market          *marketDataPage
	performance     instrumentPerformance

	returns, changes, transactions widget.Clickable
	openAnalysis                   func(routeID, analysisSelection, string, event.Tag)
	openTransactions               func(history.Filters, string, event.Tag)
}

func (p *portfolioPage) instrumentDetailName() string {
	if p.current == nil || p.current.instrument == nil || p.current.instrument.ID != p.id {
		return ""
	}
	return p.current.instrument.DisplayLabel()
}

func (p *portfolioPage) layoutInstrumentDetailHeader(gtx layout.Context, view ui.View) layout.Dimensions {
	name := p.instrumentDetailName()
	if name == "" {
		return layout.Dimensions{}
	}
	instrument, detail := p.current.instrument, &p.detail
	l := view.Language
	for index := range detail.tabs {
		section := instrumentDetailSection(index)
		if detail.tabs[index].Clicked(gtx) && section != detail.visit.section {
			detail.visit.positions[detail.visit.section] = p.list.Position
			detail.visit.section = section
			p.list.Position = detail.visit.positions[section]
			view.RestorePageScroll(0)
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	rows := []ui.ColumnChild{ui.Rigid(view.Body(ui.TextHeading, name, view.Colors.Ink))}
	if instrument.ArchivedAt != nil {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.PortfolioInstrumentArchived), view.Colors.SecondaryInk)))
	}
	if fullName := strings.TrimSpace(instrument.Name); fullName != "" && !strings.EqualFold(fullName, name) {
		rows = append(rows, ui.Rigid(view.Message(fullName, view.Colors.SecondaryInk)))
	}
	rows = append(rows, ui.Rigid(instrumentMetadata(view, *instrument)))
	if note := productString(instrument.Note); note != "" {
		if detail.note.Clicked(gtx) {
			detail.visit.noteOpen = !detail.visit.noteOpen
			gtx.Execute(op.InvalidateCmd{})
		}
		rows = append(rows, ui.Rigid(analysisDisclosure(view, &detail.note, l.Text(i18n.PortfolioInstrumentNote), detail.visit.noteOpen)))
		if detail.visit.noteOpen {
			rows = append(rows, ui.Rigid(view.Message(note, view.Colors.Ink)))
		}
	}
	rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
		return view.TabTrack(gtx, len(detail.tabs), func(gtx layout.Context, index int) layout.Dimensions {
			label := view.Language.Text(instrumentDetailLabels[index])
			return view.TabSegment(&detail.tabs[index], label, instrumentDetailSection(index) == detail.visit.section)(gtx)
		})
	}))
	return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...)
}

var instrumentDetailTypes = map[domain.InstrumentType]i18n.MessageID{
	domain.InstrumentStock:                 i18n.PortfolioInstrumentStock,
	domain.InstrumentETF:                   i18n.PortfolioInstrumentETF,
	domain.InstrumentMutualFund:            i18n.PortfolioInstrumentFund,
	domain.InstrumentCrypto:                i18n.MarketCrypto,
	domain.InstrumentBond:                  i18n.PortfolioInstrumentBond,
	domain.InstrumentPreciousMetal:         i18n.PortfolioInstrumentMetal,
	domain.InstrumentBankInvestmentProduct: i18n.PortfolioInstrumentBankProduct,
	domain.InstrumentOther:                 i18n.InstitutionOther,
}

func instrumentMetadata(view ui.View, instrument domain.Instrument) layout.Widget {
	l := view.Language
	fields := []struct{ label, value string }{
		{l.Text(i18n.PortfolioInstrumentType), l.Text(instrumentDetailTypes[instrument.Type])},
		{l.Text(i18n.PortfolioInstrumentCurrency), instrument.QuoteCurrency.String()},
		{l.Text(i18n.PortfolioInstrumentSymbol), productString(instrument.Symbol)},
		{l.Text(i18n.PortfolioInstrumentMarket), productString(instrument.MarketCode)},
		{l.Text(i18n.PortfolioInstrumentCountry), productString(instrument.CountryCode)},
		{l.Text(i18n.PortfolioInstrumentISIN), productString(instrument.ISIN)},
	}
	cells := make([]layout.Widget, 0, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field.value) == "" {
			continue
		}
		cells = append(cells, func(gtx layout.Context) layout.Dimensions {
			return semanticRow(gtx, field.label+" · "+field.value, func(gtx layout.Context) layout.Dimensions {
				return (ui.Column{Gap: ui.Space4}).Layout(gtx,
					ui.Rigid(view.Body(ui.TextCaption, field.label, view.Colors.SecondaryInk)),
					ui.Rigid(view.Message(field.value, view.Colors.Ink)))
			})
		})
	}
	return func(gtx layout.Context) layout.Dimensions {
		return overviewBreakdownCard(gtx, view, []ui.ColumnChild{ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return (ui.Grid{MinColumn: 125, MaxColumns: 3, Gap: ui.Space8}).Layout(gtx, view, len(cells),
				func(gtx layout.Context, _ ui.View, index int) layout.Dimensions { return cells[index](gtx) })
		})})
	}
}

var instrumentQuoteSources = map[domain.QuoteSourceKind]i18n.MessageID{
	domain.QuoteSourceManual:   i18n.PortfolioInstrumentQuoteManual,
	domain.QuoteSourceProvider: i18n.PortfolioInstrumentQuoteProvider,
	domain.QuoteSourceAgent:    i18n.PortfolioInstrumentQuoteAgent,
}

func instrumentCurrentQuoteEvidence(view ui.View, quote domain.InstrumentQuote) string {
	l := view.Language
	source := l.Text(instrumentQuoteSources[quote.SourceKind])
	if quote.SourceKey != "" && quote.SourceKey != string(quote.SourceKind) && quote.SourceKind != domain.QuoteSourceAgent {
		source += " · " + quote.SourceKey
	}
	evidence := l.Text(i18n.ChartSource) + ": " + source
	day := ""
	if quote.TimestampBasis == "date_label" {
		day = quote.EffectiveDate
	}
	stamp := strings.TrimPrefix(portfolioQuoteStamp(view, "", day, quote.QuotedAt), " · ")
	if stamp != "" {
		evidence = fmt.Sprintf(l.Text(i18n.PortfolioInstrumentQuoteAsOf), stamp) + " · " + evidence
	}
	if quote.Delayed {
		evidence += " · " + l.Text(i18n.MarketDelayed)
	}
	return evidence
}

func instrumentCurrentQuotePrice(view ui.View, instrument domain.Instrument, quote domain.InstrumentQuote) string {
	price := format.QuotePrice(quote.UnitPrice.Canonical(), quote.Currency.String(), marketNumberPreferences(view))
	if instrument.UsesMetalConversion() {
		price = fmt.Sprintf(view.Language.Text(catalogMetalPriceUnit(instrument.QuantityUnit)), price)
	}
	return price
}

func (p *portfolioPage) instrumentDataRows(view ui.View, instrument domain.Instrument) []layout.Widget {
	r, l := p.current, view.Language
	rows := []layout.Widget{func(gtx layout.Context) layout.Dimensions {
		price := l.Text(i18n.MarketNoCurrentPrice)
		if q := r.price; q != nil {
			price = instrumentCurrentQuotePrice(view, instrument, *q)
		}
		fields := []ui.ColumnChild{
			ui.Rigid(view.Message(l.Text(i18n.PortfolioInstrumentDataNote), view.Colors.SecondaryInk)),
			ui.Rigid(view.Body(ui.TextHeading, l.Text(i18n.PortfolioInstrumentLatestQuote), view.Colors.Ink)),
			ui.Rigid(view.Body(ui.TextMoneyMD, price, view.Colors.Ink)),
		}
		if q := r.price; q != nil {
			fields = append(fields, ui.Rigid(view.Message(instrumentCurrentQuoteEvidence(view, *q), view.Colors.SecondaryInk)))
		}
		if r.base != "" && instrument.QuoteCurrency != r.base {
			fx := fmt.Sprintf(l.Text(i18n.PortfolioFXMissing), instrument.QuoteCurrency, r.base)
			if q := r.fx; q != nil {
				fx = fmt.Sprintf(l.Text(i18n.PortfolioMarketFX), q.BaseCurrency, portfolioDecimal(view, q.Rate.Canonical()), q.QuoteCurrency) +
					" · " + marketQuoteEvidence(view, q.SourceKind, q.SourceKey, q.EffectiveDate, q.TimestampBasis, q.QuotedAt, q.Delayed)
			}
			fields = append(fields, ui.Rigid(view.Message(fx, view.Colors.SecondaryInk)))
		}
		if p.detail.market != nil {
			fields = append(fields, ui.Rigid(p.instrumentDataActions(view, instrument)))
		}
		return overviewBreakdownCard(gtx, view, fields)
	}}
	if p.detail.market != nil && p.detail.market.quoteHistoryReader != nil && p.request == nil && p.err == nil {
		rows = append(rows, p.instrumentHistoryRows(view, instrument)...)
	}
	return rows
}

func (p *portfolioPage) instrumentDataActions(view ui.View, instrument domain.Instrument) layout.Widget {
	detail := &p.detail
	return func(gtx layout.Context) layout.Dimensions {
		market := detail.market
		ready := p.request == nil && p.err == nil
		var actions []layout.Widget
		if market.instrumentSources != nil && instrument.ArchivedAt == nil {
			canEdit := ready && !market.providerBusy()
			if canEdit && detail.source.Clicked(gtx) {
				market.openInstrumentSource(instrument.ID, instrument.DisplayLabel(), &detail.source)
				gtx.Execute(op.InvalidateCmd{})
			}
			actions = append(actions, returnMonthButton(view, &detail.source, i18n.InstrumentBindingOpen, nil, canEdit))
		}
		if market.manual.service != nil && manualPriceAvailable(instrument) {
			if ready && detail.manual.Clicked(gtx) {
				market.openManualQuote(marketManualTarget{instrumentID: instrument.ID}, &detail.manual)
				gtx.Execute(op.InvalidateCmd{})
			}
			actions = append(actions, returnMonthButton(view, &detail.manual, i18n.MarketManualPrice, nil, ready))
		}
		if detail.openMaintenance != nil {
			if ready && detail.maintain.Clicked(gtx) {
				window := detail.savedVisit().histories[detail.visit.historyTab].window
				detail.openMaintenance(healthFindingFocus{instrument: instrument.ID.String(), account: p.accountScope.String(),
					from: window.from, to: window.to, label: instrument.DisplayLabel(), action: application.HealthActionInstrumentEditor}, &detail.maintain)
				gtx.Execute(op.InvalidateCmd{})
			}
			actions = append(actions, returnMonthButton(view, &detail.maintain, i18n.PortfolioMaintainData, nil, ready))
		}
		return (ui.Wrap{Gap: ui.Space8}).Layout(gtx, len(actions), func(gtx layout.Context, index int) layout.Dimensions {
			return actions[index](gtx)
		})
	}
}
