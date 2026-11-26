package desktop

import (
	"fmt"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type instrumentDetailSection uint8

const (
	instrumentDetailHoldings instrumentDetailSection = iota
	instrumentDetailData
)

var instrumentDetailLabels = [...]i18n.MessageID{i18n.PortfolioInstrumentHoldings, i18n.PortfolioInstrumentData}

type instrumentDetailVisit struct {
	section   instrumentDetailSection
	positions [len(instrumentDetailLabels)]layout.Position
}

type portfolioInstrumentDetail struct {
	visit                   instrumentDetailVisit
	tabs                    [len(instrumentDetailLabels)]widget.Clickable
	history, source, manual widget.Clickable
	market                  *marketDataPage
}

func (p *portfolioPage) layoutInstrumentDetailHeader(gtx layout.Context, view ui.View) layout.Dimensions {
	if p.current == nil || p.current.instrument == nil {
		return layout.Dimensions{}
	}
	i, detail := p.current.instrument, &p.detail
	name := i.DisplayLabel()
	title := name
	if i.ArchivedAt != nil {
		title += " · " + view.Language.Text(i18n.DirectoryArchived)
	}
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
	rows := []ui.ColumnChild{ui.Rigid(func(gtx layout.Context) layout.Dimensions {
		return view.Label(gtx, title, 23, font.Medium, view.Colors.Ink)
	})}
	if fullName := strings.TrimSpace(i.Name); fullName != "" && fullName != name {
		rows = append(rows, ui.Rigid(view.Message(fullName, view.Colors.SecondaryInk)))
	}
	rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
		return view.TabTrack(gtx, len(detail.tabs), func(gtx layout.Context, index int) layout.Dimensions {
			label := view.Language.Text(instrumentDetailLabels[index])
			return view.TabSegment(&detail.tabs[index], label, instrumentDetailSection(index) == detail.visit.section)(gtx)
		})
	}))
	return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...)
}

func (p *portfolioPage) instrumentDataRows(view ui.View, instrument domain.Instrument) []layout.Widget {
	r, l := p.current, view.Language
	return []layout.Widget{func(gtx layout.Context) layout.Dimensions {
		price := l.Text(i18n.PortfolioPriceMissing)
		var fields []ui.ColumnChild
		if q := r.price; q != nil {
			price = fmt.Sprintf(l.Text(i18n.PortfolioCurrentPrice), q.Currency, portfolioDecimal(view, q.UnitPrice.Canonical()))
			if instrument.UsesMetalConversion() {
				price = fmt.Sprintf(l.Text(catalogMetalPriceUnit(instrument.QuantityUnit)), price)
			}
		}
		fields = append(fields, ui.Rigid(view.Body(ui.TextHeading, price, view.Colors.Ink)))
		if q := r.price; q != nil {
			evidence := marketQuoteEvidence(view, q.SourceKind, q.SourceKey, q.EffectiveDate, q.TimestampBasis, q.QuotedAt, q.Delayed)
			fields = append(fields, ui.Rigid(view.Message(evidence, view.Colors.SecondaryInk)))
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
}

func (p *portfolioPage) instrumentDataActions(view ui.View, instrument domain.Instrument) layout.Widget {
	detail := &p.detail
	return func(gtx layout.Context) layout.Dimensions {
		market := detail.market
		ready := p.request == nil && p.err == nil
		target := marketQuoteHistoryTarget{instrument: instrument.ID, name: instrument.DisplayLabel()}
		var actions []layout.Widget
		if market.quoteHistoryReader != nil {
			actions = append(actions, market.quoteHistoryAction(gtx, view, target, &detail.history, ready))
		}
		if market.instrumentSources != nil && instrument.ArchivedAt == nil {
			canEdit := ready && !market.providerBusy()
			if canEdit && detail.source.Clicked(gtx) {
				market.openInstrumentSource(instrument.ID, target.name, &detail.source)
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
		return (ui.Wrap{Gap: ui.Space8}).Layout(gtx, len(actions), func(gtx layout.Context, index int) layout.Dimensions {
			return actions[index](gtx)
		})
	}
}
