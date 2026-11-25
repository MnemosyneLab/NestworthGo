package desktop

import (
	"fmt"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

var marketTabs = [...]i18n.MessageID{i18n.MarketInstruments, i18n.MarketFX}

var marketInstrumentTypes = map[domain.InstrumentType]i18n.MessageID{
	domain.InstrumentStock:                 i18n.MarketStocks,
	domain.InstrumentETF:                   i18n.MarketETFs,
	domain.InstrumentMutualFund:            i18n.MarketFunds,
	domain.InstrumentCrypto:                i18n.MarketCrypto,
	domain.InstrumentBond:                  i18n.MarketBonds,
	domain.InstrumentPreciousMetal:         i18n.MarketMetals,
	domain.InstrumentBankInvestmentProduct: i18n.ManagedProducts,
	domain.InstrumentOther:                 i18n.MarketOther,
}

func (p *marketDataPage) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	p.update()
	l := view.Language
	for index := range p.tabs {
		if p.tabs[index].Clicked(gtx) && p.tab != marketTab(index) {
			p.tab = marketTab(index)
			p.read()
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	if p.reload.Clicked(gtx) && p.request == nil {
		p.read()
		gtx.Execute(op.InvalidateCmd{})
	}
	if p.openHealth != nil && p.health.Clicked(gtx) {
		p.openHealth(&p.health)
		gtx.Execute(op.InvalidateCmd{})
	}
	action := i18n.MarketReload
	if p.err != nil {
		action = i18n.HistoryRetry
	}
	rows := []ui.ColumnChild{
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			description := view.Message(l.Text(i18n.MarketDescription), view.Colors.SecondaryInk)
			if p.openHealth == nil {
				return description(gtx)
			}
			return (ui.Grid{MinColumn: 240, Gap: 8, MaxColumns: 2}).Layout(gtx, view, 2,
				func(gtx layout.Context, view ui.View, index int) layout.Dimensions {
					if index == 0 {
						return description(gtx)
					}
					return layout.W.Layout(gtx, returnMonthButton(view, &p.health, i18n.HealthOpen, nil, true))
				})
		}),
		ui.Rigid(returnMonthButton(view, &p.reload, action, nil, p.request == nil)),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return (ui.Wrap{Gap: 8}).Layout(gtx, len(marketTabs), func(gtx layout.Context, index int) layout.Dimensions {
				return analysisToggle(view, &p.tabs[index], l.Text(marketTabs[index]), p.tab == marketTab(index))(gtx)
			})
		}),
	}
	if p.tab == marketInstruments {
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.search.Layout(gtx, view, i18n.MarketSearch, i18n.MarketSearchHint, func(string) {
				p.list[marketInstruments].Position = layout.Position{}
				gtx.Execute(op.InvalidateCmd{})
			})
		}))
	} else {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.MarketFXDescription), view.Colors.SecondaryInk)))
	}
	switch {
	case p.request != nil:
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.MarketLoading), view.Colors.SecondaryInk)))
	case p.err != nil:
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.MarketFailed), view.Colors.ErrorInk)))
	}
	if result := p.current[p.tab]; result != nil {
		rows = append(rows, p.resultRows(view, *result)...)
	}
	return (ui.Column{Gap: 12, MinFill: 250}).Layout(gtx, rows...)
}

func (p *marketDataPage) resultRows(view ui.View, result marketDataResult) []ui.ColumnChild {
	l := view.Language
	if p.tab == marketFX {
		if len(result.fx) == 0 {
			return []ui.ColumnChild{ui.Rigid(view.Message(l.Text(i18n.MarketNoFX), view.Colors.SecondaryInk))}
		}
		return []ui.ColumnChild{ui.Fill(func(gtx layout.Context) layout.Dimensions {
			return analysisResultList(gtx, view, &p.list[marketFX], l.Text(i18n.MarketFX), len(result.fx), func(gtx layout.Context, index int) layout.Dimensions {
				return marketFXCard(gtx, view, result.fx[index])
			})
		})}
	}
	query := strings.ToLower(strings.TrimSpace(p.search.Field.Text()))
	matches := make([]marketInstrumentRow, 0, len(result.instruments))
	for _, row := range result.instruments {
		if marketInstrumentMatches(row.instrument, query) {
			matches = append(matches, row)
		}
	}
	if len(matches) == 0 {
		message := i18n.MarketNoInstruments
		if len(result.instruments) > 0 {
			message = i18n.MarketNoMatches
		}
		return []ui.ColumnChild{ui.Rigid(view.Message(l.Text(message), view.Colors.SecondaryInk))}
	}
	return []ui.ColumnChild{ui.Fill(func(gtx layout.Context) layout.Dimensions {
		return analysisResultList(gtx, view, &p.list[marketInstruments], l.Text(i18n.MarketInstruments), len(matches), func(gtx layout.Context, index int) layout.Dimensions {
			row := matches[index]
			group := i18n.NoMessage
			if index == 0 || matches[index-1].instrument.Type != row.instrument.Type {
				group = marketInstrumentTypes[row.instrument.Type]
				if group == i18n.NoMessage {
					group = i18n.MarketOther
				}
			}
			return marketInstrumentCard(gtx, view, row, group)
		})
	})}
}

func marketInstrumentCard(gtx layout.Context, view ui.View, row marketInstrumentRow, group i18n.MessageID) layout.Dimensions {
	l := view.Language
	i := row.instrument
	name := i.DisplayLabel()
	if i.ArchivedAt != nil {
		name += " · " + l.Text(i18n.DirectoryArchived)
	}
	price, evidence := l.Text(i18n.MarketNoQuote), ""
	if q := row.quote; q != nil {
		price = fmt.Sprintf(l.Text(i18n.PortfolioCurrentPrice), q.Currency, portfolioDecimal(view, q.UnitPrice.Canonical()))
		evidence = marketQuoteEvidence(view, q.SourceKind, q.SourceKey, q.EffectiveDate, q.TimestampBasis, q.QuotedAt, q.Delayed)
	}
	var rows []ui.ColumnChild
	if group != i18n.NoMessage {
		rows = append(rows, ui.Rigid(view.Body(ui.TextLabel, l.Text(group), view.Colors.Ink)))
	}
	source := i.QuoteCurrency.String() + " · " + marketSourceLabel(l, i.QuoteSource)
	rows = append(rows, ui.Rigid(marketQuoteCard(view, name, source, price, evidence)))
	return (ui.Column{Gap: 8}).Layout(gtx, rows...)
}

func marketFXCard(gtx layout.Context, view ui.View, row marketFXRow) layout.Dimensions {
	l := view.Language
	rate, evidence := l.Text(i18n.MarketNoQuote), ""
	if q := row.quote; q != nil {
		rate = fmt.Sprintf(l.Text(i18n.TransferQuoteRate), q.BaseCurrency, portfolioDecimal(view, q.Rate.Canonical()), q.QuoteCurrency)
		evidence = marketQuoteEvidence(view, q.SourceKind, q.SourceKey, q.EffectiveDate, q.TimestampBasis, q.QuotedAt, q.Delayed)
	}
	return marketQuoteCard(view, row.pair.first.String()+"/"+row.pair.second.String(), marketSourceLabel(l, row.source), rate, evidence)(gtx)
}

func marketQuoteCard(view ui.View, name, source, value, evidence string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return (ui.Grid{MinColumn: 244, Gap: 8, MaxColumns: 2, Balanced: true}).Layout(gtx, view, 2,
					func(gtx layout.Context, view ui.View, index int) layout.Dimensions {
						if index == 0 {
							return (ui.Column{Gap: 3}).Layout(gtx,
								ui.Rigid(view.Body(ui.TextLabel, name, view.Colors.Ink)),
								ui.Rigid(view.Message(source, view.Colors.SecondaryInk)))
						}
						rows := []ui.ColumnChild{ui.Rigid(view.Message(value, view.Colors.Ink))}
						if evidence != "" {
							rows = append(rows, ui.Rigid(view.Message(evidence, view.Colors.SecondaryInk)))
						}
						return (ui.Column{Gap: 3}).Layout(gtx, rows...)
					})
			})
		})
	}
}

func marketSourceLabel(l i18n.Language, kind domain.QuoteSourceKind) string {
	if message := transferQuoteSource[kind]; message != i18n.NoMessage {
		return l.Text(message)
	}
	return l.Text(i18n.MarketNoSource)
}

func marketQuoteEvidence(view ui.View, kind domain.QuoteSourceKind, source, date, basis string, at time.Time, delayed bool) string {
	if source == string(kind) || kind == domain.QuoteSourceAgent {
		source = ""
	}
	if basis != "date_label" {
		date = ""
	}
	evidence := marketSourceLabel(view.Language, kind) + portfolioQuoteStamp(view, source, date, at)
	if delayed {
		evidence += " · " + view.Language.Text(i18n.MarketDelayed)
	}
	return evidence
}
