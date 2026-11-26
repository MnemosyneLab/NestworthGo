package desktop

import (
	"fmt"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
	"github.com/waltwang/nestworth-go/internal/application"
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
	if p.sync != nil && p.sync.update() {
		p.read()
	}
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
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return layout.E.Layout(gtx, returnMonthButton(view, &p.health, i18n.HealthOpen, nil, true))
				})
		}),
	}
	if p.latest.service != nil || p.sync != nil {
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions { return p.marketOperations(gtx, view, action) }))
	} else {
		rows = append(rows, ui.Rigid(returnMonthButton(view, &p.reload, action, nil, p.request == nil)))
	}
	rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
		return view.TabTrack(gtx, len(marketTabs), func(gtx layout.Context, index int) layout.Dimensions {
			return view.TabSegment(&p.tabs[index], l.Text(marketTabs[index]), p.tab == marketTab(index))(gtx)
		})
	}))
	if p.tab == marketFX && p.manual.service != nil {
		if p.manual.addFX.Clicked(gtx) {
			p.openManualQuote(marketManualTarget{}, &p.manual.addFX)
			gtx.Execute(op.InvalidateCmd{})
		}
		rows = append([]ui.ColumnChild{ui.Rigid(returnMonthButton(view, &p.manual.addFX, i18n.MarketManualFX, nil, true))}, rows...)
	}
	if receipt := p.manual.latest; receipt.saved() && (receipt.fx.ID != "") == (p.tab == marketFX) {
		rows = append(rows, ui.Rigid(manualQuoteNotice(view, receipt)))
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
			return pageListGap(gtx, view, &p.list[marketFX], l.Text(i18n.MarketFX), len(result.fx), 0, func(gtx layout.Context, index int) layout.Dimensions {
				return p.marketFXCard(gtx, view, result.fx[index], index == 0, index == len(result.fx)-1)
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
		return pageListGap(gtx, view, &p.list[marketInstruments], l.Text(i18n.MarketInstruments), len(matches), 0, func(gtx layout.Context, index int) layout.Dimensions {
			row := matches[index]
			group := i18n.NoMessage
			if index == 0 || matches[index-1].instrument.Type != row.instrument.Type {
				group = marketInstrumentTypes[row.instrument.Type]
				if group == i18n.NoMessage {
					group = i18n.MarketOther
				}
			}
			last := index == len(matches)-1 || matches[index+1].instrument.Type != row.instrument.Type
			return p.marketInstrumentCard(gtx, view, row, group, last)
		})
	})}
}

func (p *marketDataPage) marketInstrumentCard(gtx layout.Context, view ui.View, row marketInstrumentRow, group i18n.MessageID, last bool) layout.Dimensions {
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
		heading := view.Body(ui.TextHeading, l.Text(group), view.Colors.Ink)
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			// A group starts with air above it; its rows join into one card.
			return layout.Inset{Top: ui.Space12, Bottom: ui.Space4}.Layout(gtx, heading)
		}))
	}
	caption := ""
	if i.QuoteSource == domain.QuoteSourceProvider && i.ProviderKey != nil && i.ProviderSymbol != nil {
		caption = instrumentProviderLabel(l, *i.ProviderKey) + " · " + *i.ProviderSymbol
	}
	actions := p.marketHistoryAction(gtx, view, i.ID.String(), marketQuoteHistoryTarget{instrument: i.ID, name: i.DisplayLabel()})
	if p.instrumentSources != nil {
		actions = append(actions, p.instrumentSourceAction(gtx, view, i))
	}
	actions = append(actions, p.marketAgentAction(gtx, view, i.ID.String(), marketQuoteHistoryTarget{instrument: i.ID, name: i.DisplayLabel()})...)
	if p.manual.service != nil && manualPriceAvailable(i) {
		if p.manual.prices == nil {
			p.manual.prices = make(map[string]*widget.Clickable)
		}
		button := portfolioButton(p.manual.prices, i.ID.String())
		if button.Clicked(gtx) && p.request == nil && p.err == nil {
			p.openManualQuote(marketManualTarget{instrumentID: i.ID}, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		actions = append(actions, returnMonthButton(view, button, i18n.MarketManualPrice, nil, p.request == nil && p.err == nil))
	}
	if i.QuoteSource == domain.QuoteSourceProvider && i.ArchivedAt == nil {
		actions = append(actions, p.marketTargetActions(gtx, view, marketLatestTarget{instrument: i.ID},
			application.SyncRequest{Scope: application.SyncScopeInstrument, InstrumentID: i.ID.String()})...)
	}
	rows = append(rows, ui.Rigid(marketQuoteRow(view, group != i18n.NoMessage, last, marketRowSpec{
		icon: lucide.TrendingUp, name: name, caption: caption, badges: marketBadges(view, i.QuoteCurrency.String(), i.QuoteSource),
		value: price, evidence: evidence,
	}, actions)))
	return (ui.Column{}).Layout(gtx, rows...)
}

func (p *marketDataPage) marketFXCard(gtx layout.Context, view ui.View, row marketFXRow, first, last bool) layout.Dimensions {
	l := view.Language
	rate, evidence := l.Text(i18n.MarketNoQuote), ""
	if q := row.quote; q != nil {
		rate = fmt.Sprintf(l.Text(i18n.TransferQuoteRate), q.BaseCurrency, portfolioDecimal(view, q.Rate.Canonical()), q.QuoteCurrency)
		evidence = marketQuoteEvidence(view, q.SourceKind, q.SourceKey, q.EffectiveDate, q.TimestampBasis, q.QuotedAt, q.Delayed)
	}
	actions := p.marketHistoryAction(gtx, view, row.pair.first.String()+"/"+row.pair.second.String(), marketQuoteHistoryTarget{pair: row.pair})
	if p.fxSourceWriter != nil {
		actions = append(actions, p.fxSourceAction(gtx, view, row))
	}
	actions = append(actions, p.marketAgentAction(gtx, view, row.pair.first.String()+"/"+row.pair.second.String(), marketQuoteHistoryTarget{pair: row.pair})...)
	if row.source == domain.QuoteSourceProvider {
		actions = append(actions, p.marketTargetActions(gtx, view, marketLatestTarget{pair: row.pair}, application.SyncRequest{
			Scope: application.SyncScopeFX, CurrencyA: row.pair.first.String(), CurrencyB: row.pair.second.String(),
		})...)
	}
	return marketQuoteRow(view, first, last, marketRowSpec{
		icon: lucide.GitCompareArrows, name: row.pair.first.String() + "/" + row.pair.second.String(),
		badges: marketBadges(view, "", row.source), value: rate, evidence: evidence,
	}, actions)(gtx)
}

// marketRowSpec is what a quote row shows: a glyph, the name with chips and an
// optional caption on the left, the latest value and its evidence on the right.
type marketRowSpec struct {
	icon            ui.Glyph
	name, caption   string
	badges          []marketBadge
	value, evidence string
}

type marketBadge struct {
	tone ui.BadgeTone
	text string
}

// marketBadges are the currency and quote-source chips of a row.
func marketBadges(view ui.View, currency string, source domain.QuoteSourceKind) []marketBadge {
	var badges []marketBadge
	if currency != "" {
		badges = append(badges, marketBadge{ui.BadgeNeutral, currency})
	}
	tone := ui.BadgeNeutral
	switch source {
	case domain.QuoteSourceProvider:
		tone = ui.BadgeSuccess
	case domain.QuoteSourceManual, domain.QuoteSourceAgent:
		tone = ui.BadgeAccent
	}
	return append(badges, marketBadge{tone, marketSourceLabel(view.Language, source)})
}

// marketQuoteRow is one row of the joined quote card. A wide row keeps the
// actions on its trailing edge; a narrow one puts them under the figures.
func marketQuoteRow(view ui.View, first, last bool, spec marketRowSpec, actions []layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return view.RowCard(gtx, first, last, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.Inset{Top: ui.Space12, Bottom: ui.Space12, Left: ui.Space16, Right: ui.Space16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				colors := view.Colors
				tile := func(gtx layout.Context) layout.Dimensions {
					return view.IconTile(gtx, spec.icon, 32, colors.Tint(colors.Accent, 0.10), colors.Accent)
				}
				left := func(gtx layout.Context) layout.Dimensions {
					chips := func(gtx layout.Context) layout.Dimensions {
						return (ui.Wrap{Gap: ui.Space6, Middle: true}).Layout(gtx, len(spec.badges)+boolInt(spec.caption != ""), func(gtx layout.Context, i int) layout.Dimensions {
							if i < len(spec.badges) {
								return view.Badge(gtx, spec.badges[i].tone, spec.badges[i].text)
							}
							return view.Line(gtx, ui.TextCaption, spec.caption, colors.SecondaryInk)
						})
					}
					return (ui.Column{Gap: ui.Space4}).Layout(gtx,
						ui.Rigid(view.Body(ui.TextLabel, spec.name, colors.Ink)), ui.Rigid(chips))
				}
				figures := func() layout.Widget {
					return func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						children := []layout.FlexChild{layout.Rigid(view.Body(ui.TextLabel, spec.value, colors.Ink))}
						if spec.evidence != "" {
							children = append(children, layout.Rigid(layout.Spacer{Height: ui.Space4}.Layout),
								layout.Rigid(view.Body(ui.TextCaption, spec.evidence, colors.SecondaryInk)))
						}
						return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
					}
				}
				buttons := marketActionRow(view, actions)
				fill := func(w layout.Widget) layout.Widget {
					return func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return w(gtx)
					}
				}
				if ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, 900) {
					children := []layout.FlexChild{
						layout.Rigid(tile), layout.Flexed(1, fill(left)), layout.Flexed(1, fill(figures())),
					}
					if len(actions) > 0 {
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							// A fixed slot keeps the figures column aligned across rows.
							gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(300))
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							return layout.E.Layout(gtx, buttons)
						}))
					}
					return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space16)}.Layout(gtx, children...)
				}
				body := []ui.ColumnChild{ui.Rigid(left), ui.Rigid(figures())}
				if len(actions) > 0 {
					body = append(body, ui.Rigid(buttons))
				}
				return layout.Flex{Alignment: layout.Start, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
					layout.Rigid(tile),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return (ui.Column{Gap: ui.Space8}).Layout(gtx, body...)
					}))
			})
		})
	}
}

func boolInt(on bool) int {
	if on {
		return 1
	}
	return 0
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
