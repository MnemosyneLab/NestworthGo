package desktop

import (
	"fmt"
	"image"
	"slices"
	"strings"
	"time"
	"unicode"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/settings"
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
	if p.err != nil && p.request == nil && p.reload.Clicked(gtx) {
		p.read()
		gtx.Execute(op.InvalidateCmd{})
	}
	if p.openHealth != nil && p.health.Clicked(gtx) {
		p.openHealth(&p.health)
		gtx.Execute(op.InvalidateCmd{})
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
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions { return p.marketOperations(gtx, view) }))
	} else if p.err != nil {
		rows = append(rows, ui.Rigid(returnMonthButton(view, &p.reload, i18n.HistoryRetry, nil, p.request == nil)))
	}
	rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
		return view.TabTrack(gtx, len(marketTabs), func(gtx layout.Context, index int) layout.Dimensions {
			return view.TabSegment(&p.tabs[index], l.Text(marketTabs[index]), p.tab == marketTab(index))(gtx)
		})
	}))
	if receipt := p.manual.latest; receipt.saved() && (receipt.fx.ID != "") == (p.tab == marketFX) {
		rows = append(rows, ui.Rigid(manualQuoteNotice(view, receipt)))
	}
	if p.focus.from != "" {
		window := historyDay(view, p.focus.from)
		if p.focus.to != "" && p.focus.to != p.focus.from {
			window += " – " + historyDay(view, p.focus.to)
		}
		label := fmt.Sprintf(l.Text(i18n.MarketFindingWindow), window)
		rows = append(rows, ui.Rigid(view.Body(ui.TextCaption, label, view.Colors.SecondaryInk)))
	}
	if p.tab == marketInstruments {
		rows = append(rows, ui.Rigid(p.marketInstrumentControls(view)))
		if p.catalogNotice != i18n.NoMessage {
			rows = append(rows, ui.Rigid(view.Message(l.Text(p.catalogNotice), view.Colors.Ink)))
		}
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

func (p *marketDataPage) marketInstrumentControls(view ui.View) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return p.search.Layout(ui.LimitWidth(gtx, 440), view, i18n.MarketSearch, i18n.MarketSearchHint, func(string) {
			p.list[marketInstruments].Position = layout.Position{}
			gtx.Execute(op.InvalidateCmd{})
		})
	}
}

func (p *marketDataPage) resultRows(view ui.View, result marketDataResult) []ui.ColumnChild {
	l := view.Language
	if p.tab == marketFX {
		rows := result.fx
		if pair := marketFindingFXPair(p.focus); pair != (marketFXPair{}) {
			i := slices.IndexFunc(rows, func(row marketFXRow) bool { return row.pair == pair })
			if i < 0 {
				rows = nil
			} else {
				rows = rows[i : i+1]
			}
		}
		if len(rows) == 0 {
			return []ui.ColumnChild{ui.Rigid(view.Message(l.Text(i18n.MarketNoFX), view.Colors.SecondaryInk))}
		}
		return []ui.ColumnChild{ui.Fill(func(gtx layout.Context) layout.Dimensions {
			return pageListGap(gtx, view, &p.list[marketFX], l.Text(i18n.MarketFX), len(rows), 0, func(gtx layout.Context, index int) layout.Dimensions {
				return p.marketFXCard(gtx, view, rows[index], index == 0, index == len(rows)-1)
			})
		})}
	}
	query := strings.ToLower(strings.TrimSpace(p.search.Field.Text()))
	matches := make([]marketInstrumentRow, 0, len(result.instruments))
	for _, row := range result.instruments {
		if (p.focus.instrument == "" || row.instrument.ID.String() == p.focus.instrument) && marketInstrumentMatches(row.instrument, query) {
			matches = append(matches, row)
		}
	}
	if len(matches) == 0 {
		message := i18n.MarketNoInstruments
		if query != "" && len(result.instruments) > 0 {
			message = i18n.MarketNoMatches
		} else if p.focus.instrument != "" {
			message = i18n.PortfolioInstrumentUnavailable
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
	subtitle := strings.TrimSpace(i.Name)
	if strings.EqualFold(name, subtitle) {
		subtitle = ""
	}
	if i.ArchivedAt != nil {
		name += " · " + l.Text(i18n.DirectoryArchived)
	}
	price, evidence := l.Text(i18n.MarketNoCurrentPrice), ""
	if q := row.quote; q != nil {
		value := format.QuotePrice(q.UnitPrice.Canonical(), q.Currency.String(), marketNumberPreferences(view))
		if i.UsesMetalConversion() {
			value = fmt.Sprintf(l.Text(catalogMetalPriceUnit(i.QuantityUnit)), value)
		}
		price = fmt.Sprintf(l.Text(i18n.MarketLatestPrice), value)
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
	target := marketQuoteHistoryTarget{instrument: i.ID, name: i.DisplayLabel(), finding: p.focus}
	actions := p.marketHistoryAction(gtx, view, i.ID.String(), target)
	if p.instrumentSources != nil {
		actions = append(actions, p.instrumentSourceAction(gtx, view, i))
	}
	actions = append(actions, p.marketAgentAction(gtx, view, i.ID.String(), target)...)
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
	actions = p.marketInstrumentRowActions(gtx, view, i, actions)
	icon := directoryGlyph(productString(i.IconKey), domain.DefaultInstrumentIcon(i.Type))
	if icon == nil {
		icon = lucide.TrendingUp
	}
	rows = append(rows, ui.Rigid(marketQuoteRow(view, group != i18n.NoMessage, last, marketRowSpec{
		icon: icon, name: name, subtitle: subtitle, caption: caption, badges: marketBadges(view, i.QuoteCurrency.String(), i.QuoteSource),
		value: price, evidence: evidence,
	}, actions)))
	return (ui.Column{}).Layout(gtx, rows...)
}

func (p *marketDataPage) marketInstrumentRowActions(gtx layout.Context, view ui.View, i domain.Instrument, secondary []layout.Widget) []layout.Widget {
	if p.openInstrumentDetails == nil && p.openCatalogEditor == nil {
		return secondary
	}
	if p.instrumentActions == nil {
		p.instrumentActions = make(map[domain.InstrumentID]*marketInstrumentActions)
	}
	a := p.instrumentActions[i.ID]
	if a == nil {
		a = &marketInstrumentActions{}
		p.instrumentActions[i.ID] = a
	}
	l := view.Language
	ready := p.request == nil && p.err == nil
	button := func(control *widget.Clickable, label, name string, variant ui.ButtonVariant, open func()) layout.Widget {
		if ready && control.Clicked(gtx) {
			open()
			gtx.Execute(op.InvalidateCmd{})
		}
		return func(gtx layout.Context) layout.Dimensions {
			if !ready {
				gtx = gtx.Disabled()
			}
			return view.ButtonOf(control, ui.ButtonSpec{Variant: variant, Size: ui.ButtonSmall, Title: label, Name: name})(gtx)
		}
	}
	var actions []layout.Widget
	if p.openInstrumentDetails != nil {
		actions = append(actions, button(&a.details, l.Text(i18n.PortfolioCatalogDetails), fmt.Sprintf(l.Text(i18n.MarketOpenInstrumentDetails), i.Name),
			ui.ButtonGhost, func() { p.openInstrumentDetails(i.ID, i.DisplayLabel(), &a.details) }))
	}
	if p.openCatalogEditor != nil {
		actions = append(actions, button(&a.edit, l.Text(i18n.InstitutionsEdit), fmt.Sprintf(l.Text(i18n.MarketEditInstrument), i.SelectionLabel()),
			ui.ButtonOutline, func() {
				p.catalogNotice = i18n.NoMessage
				p.openCatalogEditor(i, &a.edit)
			}))
	}
	if len(secondary) == 0 {
		return actions
	}
	if a.more.Clicked(gtx) {
		a.expanded = !a.expanded
		gtx.Execute(op.InvalidateCmd{})
	}
	name := l.Text(i18n.MarketMoreActions) + " · " + i.DisplayLabel()
	actions = append(actions, view.ButtonOf(&a.more, ui.ButtonSpec{
		Variant: ui.ButtonGhost, Size: ui.ButtonSmall, Name: name, Icon: lucide.Ellipsis, Toggle: true, Selected: a.expanded,
	}))
	if a.expanded {
		actions = append(actions, secondary...)
	}
	return actions
}

func marketNumberPreferences(view ui.View) settings.Settings {
	return settings.Settings{DecimalSeparator: view.DecimalSeparator, GroupingSeparator: view.GroupingSeparator}
}

func (p *marketDataPage) marketFXCard(gtx layout.Context, view ui.View, row marketFXRow, first, last bool) layout.Dimensions {
	l := view.Language
	rate, evidence := l.Text(i18n.MarketNoQuote), ""
	if q := row.quote; q != nil {
		rate = fmt.Sprintf(l.Text(i18n.TransferQuoteRate), q.BaseCurrency, format.ExactDecimal(q.Rate.Canonical(), marketNumberPreferences(view)), q.QuoteCurrency)
		evidence = marketQuoteEvidence(view, q.SourceKind, q.SourceKey, q.EffectiveDate, q.TimestampBasis, q.QuotedAt, q.Delayed)
	}
	target := marketQuoteHistoryTarget{pair: row.pair, finding: p.focus}
	actions := p.marketHistoryAction(gtx, view, row.pair.first.String()+"/"+row.pair.second.String(), target)
	if p.fxSourceWriter != nil {
		actions = append(actions, p.fxSourceAction(gtx, view, row))
	}
	actions = append(actions, p.marketAgentAction(gtx, view, row.pair.first.String()+"/"+row.pair.second.String(), target)...)
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
	icon                    ui.Glyph
	name, subtitle, caption string
	badges                  []marketBadge
	value, evidence         string
	catalogActions          bool
	actionLabels            []string
}

type marketBadge struct {
	tone ui.BadgeTone
	text string
}

func marketTextWidth(gtx layout.Context, view ui.View, style ui.TextStyle, value string) int {
	if value == "" {
		return 0
	}
	gtx.Constraints = layout.Constraints{Max: image.Pt(1<<16, 1<<16)}
	macro := op.Record(gtx.Ops)
	label := view.TextLabelStyle(style, value, view.Colors.Ink)
	label.MaxLines = 1
	width := label.Layout(gtx).Size.X
	macro.Stop()
	if width > gtx.Sp(style.Size) {
		return width
	}
	// Software-only Gio tests have no registered platform font faces, so the
	// shaper can report one em for every string. Keep a conservative intrinsic
	// width floor there; native windows use the actual shaped width above.
	fallback, em := 0, gtx.Sp(style.Size)
	for _, r := range value {
		fallback += marketFallbackRuneWidth(em, r)
	}
	return max(width, fallback)
}

func marketFallbackRuneWidth(em int, r rune) int {
	switch {
	case unicode.Is(unicode.Han, r), unicode.Is(unicode.Hiragana, r), unicode.Is(unicode.Katakana, r), unicode.Is(unicode.Hangul, r):
		return em
	case unicode.IsSpace(r):
		return max(1, em/3)
	case unicode.IsPunct(r):
		return max(1, em*2/5)
	case unicode.IsSymbol(r):
		return em
	default:
		// Generic fallbacks can be wider than the Latin face used in local
		// screenshots. Bias toward common sans-serif glyph advances instead
		// of assuming a narrow geometric face.
		return max(1, em*3/5)
	}
}

func marketRowActionsAreTrailing(gtx layout.Context, view ui.View, spec marketRowSpec, actionCount int) bool {
	return gtx.Constraints.Max.X >= marketRowActionsMinimumWidth(gtx, view, spec, actionCount)
}

func marketRowActionsMinimumWidth(gtx layout.Context, view ui.View, spec marketRowSpec, actionCount int) int {
	leftWidth := max(
		marketTextWidth(gtx, view, ui.TextLabel, spec.name),
		marketTextWidth(gtx, view, ui.TextCaption, spec.subtitle),
		marketTextWidth(gtx, view, ui.TextCaption, spec.caption),
	)
	for _, badge := range spec.badges {
		leftWidth = max(leftWidth, marketTextWidth(gtx, view, ui.TextCaptionStrong, badge.text)+gtx.Dp(16))
	}
	figureWidth := max(
		marketTextWidth(gtx, view, ui.TextLabel, spec.value),
		marketTextWidth(gtx, view, ui.TextCaption, spec.evidence),
	)
	children, fixed := 3, gtx.Dp(32)
	if actionCount > 0 {
		children++
		if spec.catalogActions {
			fixed += marketCatalogActionWidth(gtx, view, spec.actionLabels)
		} else {
			fixed += min(gtx.Constraints.Max.X, gtx.Dp(300))
		}
	}
	fixed += gtx.Dp(ui.Space16) * (children - 1)
	if spec.catalogActions {
		return fixed + leftWidth + figureWidth
	}
	return fixed + 2*max(leftWidth, figureWidth)
}

func marketCatalogActionWidth(gtx layout.Context, view ui.View, labels []string) int {
	width := 0
	for index, label := range labels {
		if index > 0 {
			width += gtx.Dp(ui.Space6)
		}
		width += marketTextWidth(gtx, view, ui.TextCaption, label) + 2*gtx.Dp(ui.Space12)
	}
	return width
}

func marketCatalogMetadataWidth(gtx layout.Context, view ui.View, spec marketRowSpec) int {
	width, children := 0, 0
	if spec.caption != "" {
		width = marketTextWidth(gtx, view, ui.TextCaption, spec.caption)
		children++
	}
	for _, badge := range spec.badges {
		if children > 0 {
			width += gtx.Dp(ui.Space6)
		}
		width += marketTextWidth(gtx, view, ui.TextCaptionStrong, badge.text) + gtx.Dp(ui.Space16)
		children++
	}
	return width
}

func marketCatalogCompactFits(gtx layout.Context, view ui.View, spec marketRowSpec) bool {
	if !spec.catalogActions || len(spec.actionLabels) == 0 {
		return false
	}
	innerWidth := gtx.Constraints.Max.X
	iconWidth, gap := gtx.Dp(32), gtx.Dp(ui.Space12)
	identityWidth := marketTextWidth(gtx, view, ui.TextLabel, spec.name)
	actionWidth := marketCatalogActionWidth(gtx, view, spec.actionLabels)
	if iconWidth+2*gap+identityWidth+actionWidth > innerWidth {
		return false
	}
	valueWidth := marketTextWidth(gtx, view, ui.TextLabel, spec.value)
	evidenceWidth := marketTextWidth(gtx, view, ui.TextCaption, spec.evidence)
	figureWidth := max(valueWidth, evidenceWidth)
	indent := iconWidth + gap
	columnWidth := (innerWidth - indent - gap) / 2
	metadataWidth := marketCatalogMetadataWidth(gtx, view, spec)
	compact := columnWidth > 0 && metadataWidth <= columnWidth && figureWidth <= columnWidth
	return compact
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
			// Catalog cards are intrinsic-height rows. The page scroll container
			// can hand children a tall minimum constraint; carrying that through a
			// compact row vertically centers identity and actions in unused space.
			if spec.catalogActions {
				gtx.Constraints.Min.Y = 0
			}
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
					rows := []ui.ColumnChild{ui.Rigid(view.Body(ui.TextLabel, spec.name, colors.Ink))}
					if spec.subtitle != "" {
						rows = append(rows, ui.Rigid(view.Body(ui.TextCaption, spec.subtitle, colors.SecondaryInk)))
					}
					return (ui.Column{Gap: ui.Space4}).Layout(gtx, append(rows, ui.Rigid(chips))...)
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
				compactCatalog := func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = 0
					metadata := func(gtx layout.Context) layout.Dimensions {
						count := len(spec.badges) + boolInt(spec.caption != "")
						return ui.Wrap{Gap: ui.Space6, Middle: true}.Layout(gtx, count, func(gtx layout.Context, index int) layout.Dimensions {
							if spec.caption != "" {
								if index == 0 {
									return view.Line(gtx, ui.TextCaption, spec.caption, colors.SecondaryInk)
								}
								index--
								return view.Badge(gtx, spec.badges[index].tone, spec.badges[index].text)
							}
							return view.Badge(gtx, spec.badges[index].tone, spec.badges[index].text)
						})
					}
					figuresEnd := func(gtx layout.Context) layout.Dimensions {
						children := []layout.FlexChild{
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return view.LineEnd(gtx, ui.TextLabel, spec.value, colors.Ink)
							}),
						}
						if spec.evidence != "" {
							children = append(children,
								layout.Rigid(layout.Spacer{Height: ui.Space4}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return view.LineEnd(gtx, ui.TextCaption, spec.evidence, colors.SecondaryInk)
								}),
							)
						}
						return layout.Flex{Axis: layout.Vertical, Alignment: layout.End, Gap: gtx.Dp(ui.Space4)}.Layout(gtx, children...)
					}
					// Flex records operations at the current origin. Build each row
					// inside Column's child callback so its text, quote, and actions
					// are replayed together at that row's actual vertical offset.
					return (ui.Column{Gap: ui.Space4}).Layout(gtx,
						ui.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
								layout.Rigid(tile),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return view.Line(gtx, ui.TextLabel, spec.name, colors.Ink)
								}),
								layout.Rigid(buttons),
							)
						}),
						ui.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Spacer{Width: ui.Space32 + ui.Space12}.Layout(gtx)
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									width := gtx.Constraints.Max.X
									dims := metadata(gtx)
									dims.Size.X = width
									return dims
								}),
								layout.Flexed(1, figuresEnd),
							)
						}),
					)
				}
				fill := func(w layout.Widget) layout.Widget {
					return func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return w(gtx)
					}
				}
				horizontal := func(gtx layout.Context) layout.Dimensions {
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
				if spec.catalogActions {
					compactFits := marketCatalogCompactFits(gtx, view, spec)
					if !ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, ui.BreakpointMedium) && compactFits {
						return compactCatalog(gtx)
					}
					if ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, ui.BreakpointMedium) && marketRowActionsAreTrailing(gtx, view, spec, len(actions)) {
						return horizontal(gtx)
					}
					if compactFits {
						return compactCatalog(gtx)
					}
				} else if marketRowActionsAreTrailing(gtx, view, spec, len(actions)) {
					return horizontal(gtx)
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
