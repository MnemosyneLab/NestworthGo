package desktop

import (
	"fmt"
	"image"
	"image/color"
	"maps"
	"slices"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type portfolioHoldingsVisit struct {
	byAccount bool
	showZero  bool
	sort      holdingSort
	collapsed map[domain.HoldingGroupKey]bool
	expanded  map[domain.InstrumentID]bool
}

type portfolioHoldingKey struct {
	instrument domain.InstrumentID
	holding    domain.HoldingID
}

type portfolioHoldingActions struct {
	menu                                                   ui.Select
	toggle, instrument, account, history, returns, changes widget.Clickable
}

const holdingTableWidth = 736
const holdingToolbarWideWidth = 780

var holdingTableInset = layout.Inset{Left: ui.Space12, Right: ui.Space12}

// Headers, native subtotals, and holding rows use the same widths.
func holdingTableColumns(gtx layout.Context, cell func(layout.Context, int) layout.Dimensions, actions layout.Widget) layout.Dimensions {
	weights := [holdingSortCount]float32{32, 13, 16, 16, 16}
	var children [holdingSortCount + 1]layout.FlexChild
	for index, weight := range weights {
		children[index] = layout.Flexed(weight, func(gtx layout.Context) layout.Dimensions { return cell(gtx, index) })
	}
	children[holdingSortCount] = layout.Flexed(7, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, actions)
	})
	return (layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}).Layout(gtx, children[:]...)
}

type portfolioHoldings struct {
	visit        portfolioHoldingsVisit
	byInstrument widget.Clickable
	byAccount    widget.Clickable
	showZero     widget.Bool
	toggleGroups widget.Clickable
	sortButtons  map[domain.HoldingGroupKey]*[holdingSortCount]widget.Clickable
	groups       map[domain.HoldingGroupKey]*widget.Clickable
	disclosures  map[domain.InstrumentID]*widget.Clickable
	actions      map[portfolioHoldingKey]*portfolioHoldingActions
	listOrigin   image.Point
	rowOrigin    image.Point
	rowWidth     int
	rowHeights   []int
	projection   holdingsProjection
}

type holdingsProjection struct {
	read        *portfolioRead
	query       string
	showZero    bool
	byAccount   bool
	sort        holdingSort
	locale      i18n.Language
	groups      []domain.InstrumentHoldingGroupView
	accountRows map[domain.HoldingGroupKey][]portfolioHoldingRow
}

func (p *portfolioPage) holdingsVisit() portfolioHoldingsVisit {
	visit := p.holdings.visit
	visit.collapsed = maps.Clone(visit.collapsed)
	visit.expanded = maps.Clone(visit.expanded)
	return visit
}

func (p *portfolioPage) restoreHoldingsVisit(visit portfolioHoldingsVisit) {
	p.holdings.visit = portfolioHoldingsVisit{
		byAccount: visit.byAccount, showZero: visit.showZero, sort: visit.sort,
		collapsed: maps.Clone(visit.collapsed), expanded: maps.Clone(visit.expanded),
	}
	p.holdings.showZero.Value = visit.showZero
}

func (p *portfolioPage) holdingsControls(gtx layout.Context, view ui.View) layout.Dimensions {
	h, l := &p.holdings, view.Language
	reset := func() {
		p.list.Position = layout.Position{}
		gtx.Execute(op.InvalidateCmd{})
	}
	if h.byInstrument.Clicked(gtx) && h.visit.byAccount {
		h.visit.byAccount = false
		reset()
	}
	if h.byAccount.Clicked(gtx) && !h.visit.byAccount {
		h.visit.byAccount = true
		reset()
	}
	if h.showZero.Update(gtx) {
		h.visit.showZero = h.showZero.Value
		reset()
	}
	tabs := layout.Widget(func(gtx layout.Context) layout.Dimensions {
		return view.TabTrack(gtx, 2, func(gtx layout.Context, i int) layout.Dimensions {
			if i == 0 {
				return view.TabSegment(&h.byInstrument, l.Text(i18n.PortfolioByInstrument), !h.visit.byAccount)(gtx)
			}
			return view.TabSegment(&h.byAccount, l.Text(i18n.PortfolioByAccount), h.visit.byAccount)(gtx)
		})
	})
	visible := p.visibleHoldingGroups(l)
	var collapse layout.Widget
	if len(visible) > 1 {
		hasOpen := slices.ContainsFunc(visible, func(group domain.InstrumentHoldingGroupView) bool {
			return !h.visit.collapsed[group.Key]
		})
		if h.toggleGroups.Clicked(gtx) {
			h.visit.collapsed = make(map[domain.HoldingGroupKey]bool, len(visible))
			if hasOpen {
				for _, group := range visible {
					h.visit.collapsed[group.Key] = true
				}
			}
			p.list.Position = layout.Position{}
			hasOpen = !hasOpen
			gtx.Execute(op.InvalidateCmd{})
		}
		label := i18n.PortfolioExpandAll
		if hasOpen {
			label = i18n.PortfolioCollapseAll
		}
		collapse = view.ButtonOf(&h.toggleGroups, ui.ButtonSpec{Variant: ui.ButtonGhost, Size: ui.ButtonSmall, Title: l.Text(label)})
	}
	search := func(gtx layout.Context) layout.Dimensions {
		return p.search.LayoutCompact(ui.LimitWidth(gtx, 220), view, i18n.PortfolioSearch, i18n.PortfolioSearchHint, func(string) { reset() })
	}
	trailing := func(gtx layout.Context) layout.Dimensions {
		pickers := []layout.Widget{view.Checkbox(&h.showZero, l.Text(i18n.PortfolioShowZero))}
		if collapse != nil {
			pickers = append(pickers, collapse)
		}
		return (ui.Wrap{Gap: ui.Space8, Middle: true}).Layout(gtx, len(pickers), func(gtx layout.Context, i int) layout.Dimensions {
			return pickers[i](gtx)
		})
	}
	if ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, holdingToolbarWideWidth) {
		return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
			layout.Rigid(tabs),
			layout.Rigid(search),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.E.Layout(gtx, trailing)
			}),
		)
	}
	return (ui.Wrap{Gap: ui.Space8, Middle: true}).Layout(gtx, 3, func(gtx layout.Context, i int) layout.Dimensions {
		switch i {
		case 0:
			return tabs(gtx)
		case 1:
			return search(gtx)
		default:
			return trailing(gtx)
		}
	})
}

func holdingInstrumentName(row domain.InstrumentHoldingsView) string {
	return domain.InstrumentDisplayLabel(row.Name, row.Symbol, row.ShortName)
}

// Filtering and ordering only affect displayed rows. The currency subtotals
// stay attached to the full group returned by the shared application service.
func (p *portfolioPage) visibleHoldingGroups(locale i18n.Language) []domain.InstrumentHoldingGroupView {
	query := normalizeDirectoryQuery(p.search.Field.Text())
	h := &p.holdings
	previous := h.projection
	if previous.read == p.current && previous.query == query && previous.showZero == h.visit.showZero &&
		previous.byAccount == h.visit.byAccount && previous.sort == h.visit.sort && previous.locale == locale {
		return previous.groups
	}
	projection := holdingsProjection{read: p.current, query: query, showZero: h.visit.showZero,
		byAccount: h.visit.byAccount, sort: h.visit.sort, locale: locale}
	if h.visit.byAccount {
		projection.accountRows = make(map[domain.HoldingGroupKey][]portfolioHoldingRow)
	}
	for _, group := range p.current.groups {
		filtered := domain.InstrumentHoldingGroupView{Key: group.Key, Subtotals: group.Subtotals}
		for _, instrument := range group.Instruments {
			if !strings.Contains(normalizeDirectoryQuery(strings.Join([]string{instrument.Name, instrument.Symbol, instrument.ShortName}, " ")), query) {
				continue
			}
			if !h.visit.showZero {
				instrument.Holdings = slices.DeleteFunc(slices.Clone(instrument.Holdings), func(member domain.InstrumentHoldingMember) bool {
					return member.Amounts.Quantity == "0"
				})
			}
			if len(instrument.Holdings) != 0 {
				filtered.Instruments = append(filtered.Instruments, instrument)
			}
		}
		if len(filtered.Instruments) == 0 {
			continue
		}
		var rows []portfolioHoldingRow
		for i := range filtered.Instruments {
			instrument := &filtered.Instruments[i]
			if h.visit.byAccount {
				for j := range instrument.Holdings {
					rows = append(rows, portfolioHoldingRow{instrument: instrument, member: &instrument.Holdings[j]})
				}
			} else {
				rows = append(rows, portfolioHoldingRow{instrument: instrument})
			}
		}
		sortPortfolioHoldingRows(rows, h.visit.sort, locale)
		if h.visit.byAccount {
			projection.accountRows[group.Key] = rows
		} else {
			ordered := make([]domain.InstrumentHoldingsView, len(rows))
			for i, row := range rows {
				ordered[i] = *row.instrument
			}
			filtered.Instruments = ordered
		}
		projection.groups = append(projection.groups, filtered)
	}
	h.projection = projection
	return projection.groups
}

func (p *portfolioPage) holdingsRows(view ui.View) []layout.Widget {
	l := view.Language
	groups := p.visibleHoldingGroups(l)
	if len(groups) == 0 {
		label := i18n.PortfolioEmpty
		if len(p.current.groups) != 0 {
			label = i18n.PortfolioZeroOnly
		}
		if strings.TrimSpace(p.search.Field.Text()) != "" {
			label = i18n.PortfolioNoMatches
		}
		return []layout.Widget{emptyState(view, l.Text(label))}
	}
	var rows []layout.Widget
	h := &p.holdings
	if h.groups == nil {
		h.groups = make(map[domain.HoldingGroupKey]*widget.Clickable)
		h.sortButtons = make(map[domain.HoldingGroupKey]*[holdingSortCount]widget.Clickable)
		h.disclosures = make(map[domain.InstrumentID]*widget.Clickable)
		h.actions = make(map[portfolioHoldingKey]*portfolioHoldingActions)
	}
	for groupIndex, group := range groups {
		var detailRows []func(bool) layout.Widget
		if !h.visit.collapsed[group.Key] {
			detailRows = append(detailRows, func(last bool) layout.Widget { return p.holdingSortRow(view, group.Key, last) })
			if h.visit.byAccount {
				for _, row := range h.projection.accountRows[group.Key] {
					instrument, member, key := *row.instrument, row.member, group.Key
					detailRows = append(detailRows, func(last bool) layout.Widget { return p.holdingCard(view, instrument, member, key, last) })
				}
			} else {
				for _, instrument := range group.Instruments {
					item, key := instrument, group.Key
					detailRows = append(detailRows, func(last bool) layout.Widget { return p.holdingCard(view, item, nil, key, last) })
					if !h.visit.expanded[instrument.InstrumentID] {
						continue
					}
					for i := range instrument.Holdings {
						item, member, key := instrument, &instrument.Holdings[i], group.Key
						detailRows = append(detailRows, func(last bool) layout.Widget { return p.holdingCard(view, item, member, key, last) })
					}
				}
			}
		}
		rows = append(rows, p.holdingGroupHeader(view, group, len(detailRows) == 0))
		for i := range detailRows {
			rows = append(rows, detailRows[i](i == len(detailRows)-1))
		}
		if groupIndex < len(groups)-1 {
			rows = append(rows, func(gtx layout.Context) layout.Dimensions {
				return layout.Spacer{Height: ui.Space12}.Layout(gtx)
			})
		}
	}
	return rows
}

func holdingGroupLabel(key domain.HoldingGroupKey) i18n.MessageID {
	switch key {
	case domain.HoldingGroupStock:
		return i18n.PortfolioGroupStock
	case domain.HoldingGroupFund:
		return i18n.PortfolioGroupFund
	case domain.HoldingGroupBond:
		return i18n.PortfolioGroupBond
	case domain.HoldingGroupBankWealth:
		return i18n.PortfolioGroupBankWealth
	case domain.HoldingGroupMetal:
		return i18n.PortfolioGroupMetal
	case domain.HoldingGroupCrypto:
		return i18n.PortfolioGroupCrypto
	case domain.HoldingGroupCashLike:
		return i18n.PortfolioGroupCashLike
	default:
		return i18n.PortfolioGroupOther
	}
}

func (p *portfolioPage) holdingGroupHeader(view ui.View, group domain.InstrumentHoldingGroupView, last bool) layout.Widget {
	h := &p.holdings
	open := !h.visit.collapsed[group.Key]
	button := portfolioButton(h.groups, group.Key)
	toggle := func() {
		if h.visit.collapsed == nil {
			h.visit.collapsed = make(map[domain.HoldingGroupKey]bool)
		}
		h.visit.collapsed[group.Key] = open
	}
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		content := portfolioHoldingGroupHeaderContent(view, group.Key, len(group.Instruments), group.Subtotals, button, open, true, toggle)
		return view.RowCard(gtx, true, last, func(gtx layout.Context) layout.Dimensions {
			if len(group.Subtotals) > 0 && ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, holdingTableWidth) {
				return content(gtx)
			}
			return layout.UniformInset(ui.Space20).Layout(gtx, content)
		})
	}
}

func (p *portfolioPage) holdingSortRow(view ui.View, group domain.HoldingGroupKey, last bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return view.RowCard(gtx, false, last, func(gtx layout.Context) layout.Dimensions {
			return p.holdingSortHeader(gtx, view, group)
		})
	}
}

func sharedHoldingGroupHeader(view ui.View, group domain.HoldingGroupKey, count int, subtotals []domain.HoldingCurrencySubtotal, button *widget.Clickable, open, compact bool, toggle func()) layout.Widget {
	l := view.Language
	countLabel := i18n.PortfolioGroupCount
	if count == 1 {
		countLabel = i18n.PortfolioGroupOne
	}
	label := l.Text(holdingGroupLabel(group)) + " · " + fmt.Sprintf(l.Text(countLabel), count)
	disclosure := func(gtx layout.Context) layout.Dimensions {
		if button.Clicked(gtx) {
			toggle()
			gtx.Execute(op.InvalidateCmd{})
		}
		return analysisDisclosure(view, button, label, open)(gtx)
	}
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		if compact && len(subtotals) > 0 && ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, holdingTableWidth) {
			return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
				inset := holdingTableInset
				inset.Top, inset.Bottom = ui.Space8, ui.Space8
				return inset.Layout(gtx, holdingSubtotalTable(view, disclosure, subtotals))
			})
		}
		return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
			fields := []ui.ColumnChild{ui.Rigid(disclosure)}
			for _, subtotal := range subtotals {
				currency := subtotal.Currency.String()
				fields = append(fields,
					ui.Rigid(view.Body(ui.TextLabel, fmt.Sprintf(l.Text(i18n.PortfolioCurrencySubtotal), currency), view.Colors.SecondaryInk)),
					ui.Rigid(holdingMetrics(view, subtotal.Amounts, "", false)))
			}
			return (ui.Column{Gap: ui.Space8}).Layout(gtx, fields...)
		})
	}
}

func portfolioHoldingGroupHeaderContent(view ui.View, group domain.HoldingGroupKey, count int, subtotals []domain.HoldingCurrencySubtotal, button *widget.Clickable, open, compact bool, toggle func()) layout.Widget {
	l := view.Language
	countLabel := i18n.PortfolioGroupCount
	if count == 1 {
		countLabel = i18n.PortfolioGroupOne
	}
	label := l.Text(holdingGroupLabel(group)) + " · " + fmt.Sprintf(l.Text(countLabel), count)
	disclosure := func(gtx layout.Context) layout.Dimensions {
		if button.Clicked(gtx) {
			toggle()
			gtx.Execute(op.InvalidateCmd{})
		}
		return analysisDisclosure(view, button, label, open)(gtx)
	}
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		if compact && len(subtotals) > 0 && ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, holdingTableWidth) {
			inset := holdingTableInset
			inset.Top, inset.Bottom = ui.Space8, ui.Space8
			return inset.Layout(gtx, holdingSubtotalTable(view, disclosure, subtotals))
		}
		fields := []ui.ColumnChild{ui.Rigid(disclosure)}
		for _, subtotal := range subtotals {
			currency := subtotal.Currency.String()
			fields = append(fields,
				ui.Rigid(view.Body(ui.TextLabel, fmt.Sprintf(l.Text(i18n.PortfolioCurrencySubtotal), currency), view.Colors.SecondaryInk)),
				ui.Rigid(holdingMetrics(view, subtotal.Amounts, "", false)))
		}
		return (ui.Column{Gap: ui.Space8}).Layout(gtx, fields...)
	}
}

func holdingSubtotalTable(view ui.View, disclosure layout.Widget, subtotals []domain.HoldingCurrencySubtotal) layout.Widget {
	l, colors := view.Language, view.Colors
	return func(gtx layout.Context) layout.Dimensions {
		headings := [...]i18n.MessageID{i18n.PortfolioColumnCost, i18n.PortfolioColumnValue, i18n.PortfolioColumnGain}
		column := func(index int) layout.Widget {
			return func(gtx layout.Context) layout.Dimensions {
				rows := []ui.ColumnChild{ui.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.LineEnd(gtx, ui.TextCaption, l.Text(headings[index]), colors.SecondaryInk)
				})}
				for _, subtotal := range subtotals {
					values := holdingMetricValues(view, subtotal.Amounts, "", false)
					metric := values[index]
					label := fmt.Sprintf(l.Text(metric.label), metric.value)
					if metric.reason != "" {
						label += " · " + l.Text(holdingMissingReason(metric.reason))
					}
					scope := fmt.Sprintf(l.Text(i18n.PortfolioCurrencySubtotal), subtotal.Currency.String())
					rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
						return semanticRow(gtx, scope, func(gtx layout.Context) layout.Dimensions {
							return semanticRow(gtx, label, func(gtx layout.Context) layout.Dimensions {
								return view.LineEnd(gtx, ui.TextLabel, metric.value, metric.ink)
							})
						})
					}))
				}
				return (ui.Column{Gap: ui.Space4}).Layout(gtx, rows...)
			}
		}
		summary := func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
				layout.Flexed(1, column(0)),
				layout.Flexed(1, column(1)),
				layout.Flexed(1, column(2)),
			)
		}
		return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
			layout.Rigid(disclosure),
			layout.Flexed(1, summary),
		)
	}
}

func (p *portfolioPage) holdingCard(view ui.View, row domain.InstrumentHoldingsView, member *domain.InstrumentHoldingMember, group domain.HoldingGroupKey, last bool) layout.Widget {
	l, h := view.Language, &p.holdings
	name := holdingInstrumentName(row)
	fullName := strings.TrimSpace(row.Name)
	title := name
	identity := portfolioHoldingKey{instrument: row.InstrumentID}
	amounts, account := row.Amounts, domain.AccountID("")
	compactMember := member != nil && !h.visit.byAccount
	if member != nil {
		identity.holding = member.HoldingID
		account, amounts = member.AccountID, member.Amounts
	}
	if row.Archived && !compactMember {
		title += " · " + l.Text(i18n.DirectoryArchived)
	}
	var instrument func(bool) layout.Widget
	if !compactMember {
		button := portfolioButton(p.instruments, row.InstrumentID)
		if member != nil {
			button = &p.holdingActions(identity).instrument
		}
		instrument = func(compact bool) layout.Widget {
			return func(gtx layout.Context) layout.Dimensions {
				if button.Clicked(gtx) {
					if account != "" && p.openHoldingInstrument != nil {
						p.openHoldingInstrument(row.InstrumentID, account, name, button)
					} else if p.openInstrument != nil {
						p.openInstrument(row.InstrumentID, name, button)
					}
					gtx.Execute(op.InvalidateCmd{})
				}
				spec := ui.ButtonSpec{Variant: ui.ButtonSoft, Title: title, Name: title}
				if compact {
					spec.Variant, spec.Size = ui.ButtonLink, ui.ButtonCompact
					if fullName != "" && fullName != name {
						spec.Description = fullName
					}
				}
				return layout.W.Layout(gtx, view.ButtonOf(button, spec))
			}
		}
	}
	var accountControl, accountDisclosure layout.Widget
	if member == nil {
		disclosure := portfolioButton(h.disclosures, row.InstrumentID)
		count := len(row.Holdings)
		countLabel := i18n.PortfolioAccountCount
		if count == 1 {
			countLabel = i18n.PortfolioAccountOne
		}
		accountCount := fmt.Sprintf(l.Text(countLabel), count)
		accountControl = view.Body(ui.TextCaption, accountCount, view.Colors.SecondaryInk)
		accountDisclosure = func(gtx layout.Context) layout.Dimensions {
			open := h.visit.expanded[row.InstrumentID]
			if disclosure.Clicked(gtx) {
				if h.visit.expanded == nil {
					h.visit.expanded = make(map[domain.InstrumentID]bool)
				}
				open = !open
				h.visit.expanded[row.InstrumentID] = open
				gtx.Execute(op.InvalidateCmd{})
			}
			icon := lucide.ChevronRight
			label := i18n.PortfolioShowAccountHoldings
			if open {
				icon = lucide.ChevronDown
				label = i18n.PortfolioHideAccountHoldings
			}
			view.Focus.Watch(gtx, disclosure)
			return view.ButtonOf(disclosure, ui.ButtonSpec{Variant: ui.ButtonGhost, Size: ui.ButtonCompact,
				Name: fmt.Sprintf(l.Text(label), name), Toggle: true, Selected: open, Icon: icon})(gtx)
		}
	} else {
		accountControl = p.holdingAccountLink(view, &p.holdingActions(identity).account, member.AccountID, member.AccountName)
	}
	unit := row.QuantityUnit
	if group == domain.HoldingGroupCashLike {
		unit = l.Text(i18n.PortfolioContractUnit)
	}
	identityContent := func(gtx layout.Context, trailing layout.Widget) layout.Dimensions {
		if instrument == nil {
			children := []layout.FlexChild{layout.Flexed(1, accountControl)}
			if trailing != nil {
				children = append(children, layout.Rigid(trailing))
			}
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}.Layout(gtx, children...)
		}
		rows := []ui.ColumnChild{ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			children := make([]layout.FlexChild, 0, 3)
			if member == nil {
				children = append(children, layout.Rigid(accountDisclosure))
			}
			children = append(children, layout.Flexed(1, instrument(true)))
			if trailing != nil {
				children = append(children, layout.Rigid(trailing))
			}
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space4)}.Layout(gtx, children...)
		})}
		indent := layout.Inset{Left: ui.Space32}
		if fullName != "" && fullName != name {
			rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				return indent.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return view.Line(gtx, ui.TextCaption, fullName, view.Colors.SecondaryInk)
				})
			}))
		}
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return indent.Layout(gtx, accountControl)
		}))
		return (ui.Column{}).Layout(gtx, rows...)
	}
	narrowHeading := func(gtx layout.Context) layout.Dimensions {
		return identityContent(gtx, p.holdingActionMenu(view, identity, account, name))
	}
	compactHeading := func(gtx layout.Context) layout.Dimensions { return identityContent(gtx, nil) }
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		if ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, holdingTableWidth) {
			return view.RowCard(gtx, false, last, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: ui.Space8, Bottom: ui.Space8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return p.compactHoldingRow(gtx, view, compactHeading, identity, account, name, holdingMetricValues(view, amounts, unit, true))
				})
			})
		}
		return view.RowCard(gtx, false, last, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(ui.Space16).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				rows := []ui.ColumnChild{
					ui.Rigid(narrowHeading), ui.Rigid(view.Hairline), ui.Rigid(holdingMetrics(view, amounts, unit, true)),
				}
				return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...)
			})
		})
	}
}

func (p *portfolioPage) compactHoldingRow(gtx layout.Context, view ui.View, identity layout.Widget, id portfolioHoldingKey, account domain.AccountID, name string, metrics []holdingMetric) layout.Dimensions {
	l := view.Language
	rows := []ui.ColumnChild{
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return holdingTableInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return holdingTableColumns(gtx, func(gtx layout.Context, index int) layout.Dimensions {
					if index == int(holdingSortName) {
						return identity(gtx)
					}
					field := metrics[index-1]
					return holdingAmountCell(gtx, view, fmt.Sprintf(l.Text(field.label), field.value), field)
				}, p.holdingActionMenu(view, id, account, name))
			})
		}),
	}
	var reasons []string
	for _, field := range metrics {
		if field.reason != "" && !slices.Contains(reasons, field.reason) {
			reasons = append(reasons, field.reason)
		}
	}
	if len(reasons) > 0 {
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return holdingTableInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return (ui.Wrap{Gap: ui.Space8}).Layout(gtx, len(reasons), func(gtx layout.Context, index int) layout.Dimensions {
					return view.Body(ui.TextCaption, l.Text(holdingMissingReason(reasons[index])), view.Colors.SecondaryInk)(gtx)
				})
			})
		}))
	}
	return (ui.Column{Gap: ui.Space4}).Layout(gtx, rows...)
}

func holdingAmountCell(gtx layout.Context, view ui.View, label string, metric holdingMetric) layout.Dimensions {
	return (layout.Inset{Right: ui.Space8}).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return semanticRow(gtx, label, func(gtx layout.Context) layout.Dimensions {
			return view.LineEnd(gtx, ui.TextLabel, metric.value, metric.ink)
		})
	})
}

func (p *portfolioPage) holdingActions(id portfolioHoldingKey) *portfolioHoldingActions {
	if p.holdings.actions[id] == nil {
		p.holdings.actions[id] = new(portfolioHoldingActions)
	}
	return p.holdings.actions[id]
}

func (p *portfolioPage) holdingAccountLink(view ui.View, button *widget.Clickable, account domain.AccountID, name string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if p.openAccount == nil {
			return view.Body(ui.TextCaption, name, view.Colors.SecondaryInk)(gtx)
		}
		if button.Clicked(gtx) {
			p.openAccount(account, name, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		label := fmt.Sprintf(view.Language.Text(i18n.HistoryOpenAccount), name)
		return layout.W.Layout(gtx, view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonLink, Size: ui.ButtonSmall, Title: name, Name: label}))
	}
}

func (p *portfolioPage) holdingActionMenu(view ui.View, id portfolioHoldingKey, account domain.AccountID, name string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		h, l := &p.holdings, view.Language
		if p.openHoldingHistory == nil && p.openHoldingAnalysis == nil {
			return layout.Dimensions{}
		}
		controls := p.holdingActions(id)
		label := fmt.Sprintf(l.Text(i18n.PortfolioActionsFor), name)
		items := make([]ui.Choice, 0, 3)
		if p.openHoldingAnalysis != nil {
			items = append(items,
				ui.Choice{ID: "return", Label: l.Text(i18n.PortfolioViewReturn)},
				ui.Choice{ID: "changes", Label: l.Text(i18n.PortfolioViewChanges)},
			)
		}
		if p.openHoldingHistory != nil {
			items = append(items, ui.Choice{ID: "history", Label: l.Text(i18n.PortfolioInstrumentHistory)})
		}
		if len(items) == 0 {
			return layout.Dimensions{}
		}
		size := gtx.Dp(ui.ControlCompact)
		anchor := image.Rectangle{
			Min: h.rowOrigin.Add(image.Pt(max(0, h.rowWidth-size-gtx.Dp(ui.Space12)), gtx.Dp(ui.Space12))),
			Max: h.rowOrigin.Add(image.Pt(max(0, h.rowWidth-size-gtx.Dp(ui.Space12))+size, gtx.Dp(ui.Space12)+size)),
		}
		trigger := &controls.menu.Trigger
		controls.menu.Menu(gtx, view, anchor, items, label, "", nil, nil, 232, func(action string) {
			switch action {
			case "return":
				if p.openHoldingAnalysis != nil {
					p.openHoldingAnalysis(returnAnalysisRoute, id.instrument, account, name, trigger)
				}
			case "changes":
				if p.openHoldingAnalysis != nil {
					p.openHoldingAnalysis(assetChangesRoute, id.instrument, account, name, trigger)
				}
			case "history":
				if p.openHoldingHistory != nil {
					p.openHoldingHistory(id.instrument, account, name, trigger)
				}
			}
		})
		return view.ButtonOf(trigger, ui.ButtonSpec{Variant: ui.ButtonGhost, Size: ui.ButtonCompact,
			Name: label, Icon: lucide.Ellipsis})(gtx)
	}
}

func holdingAmounts(view ui.View, amounts domain.HoldingAmounts, unit string) layout.Widget {
	return holdingMetrics(view, amounts, unit, true)
}

type holdingMetric struct {
	label         i18n.MessageID
	value, reason string
	ink           color.NRGBA
}

func holdingMetricLabel(language i18n.Language, label i18n.MessageID) string {
	pattern := language.Text(label)
	const marker = "\x00"
	if prefix, ok := strings.CutSuffix(fmt.Sprintf(pattern, marker), marker); ok {
		return strings.TrimRight(prefix, " ·:：")
	}
	return pattern
}

func holdingMetricValues(view ui.View, amounts domain.HoldingAmounts, unit string, includeQuantity bool) []holdingMetric {
	l, colors := view.Language, view.Colors
	unavailable := l.Text(i18n.OverviewNoValue)
	money := func(value *domain.MoneyView) string {
		if value == nil {
			return unavailable
		}
		return valuationMoney(view, *value, false)
	}
	gain, gainInk := unavailable, colors.Ink
	if value := amounts.UnrealizedGain; value != nil {
		gain = wealthExactMoney(view, &value.Amount, value.Currency.String(), false, true)
		switch {
		case strings.HasPrefix(gain, "+"):
			gainInk = colors.Success
		case strings.HasPrefix(gain, "-"):
			gainInk = colors.Destructive
		}
	}
	fields := make([]holdingMetric, 0, 4)
	if includeQuantity {
		quantity := portfolioDecimal(view, amounts.Quantity)
		if unit != "" {
			quantity += " " + unit
		}
		fields = append(fields, holdingMetric{label: i18n.AccountQuantity, value: quantity, ink: colors.Ink})
	}
	return append(fields,
		holdingMetric{i18n.PortfolioCost, money(amounts.TotalCost), amounts.CostMissingReason, colors.Ink},
		holdingMetric{i18n.PortfolioNativeValue, money(amounts.CurrentValue), amounts.ValueMissingReason, colors.Ink},
		holdingMetric{i18n.PortfolioUnrealized, gain, amounts.GainMissingReason, gainInk})
}

func holdingMetrics(view ui.View, amounts domain.HoldingAmounts, unit string, includeQuantity bool) layout.Widget {
	l, colors := view.Language, view.Colors
	fields := holdingMetricValues(view, amounts, unit, includeQuantity)
	return func(gtx layout.Context) layout.Dimensions {
		return (ui.Grid{MinColumn: 128, MaxColumns: len(fields), Gap: ui.Space8, Balanced: true}).Layout(gtx, view, len(fields), func(gtx layout.Context, view ui.View, index int) layout.Dimensions {
			field := fields[index]
			label := holdingMetricLabel(l, field.label)
			summary := fmt.Sprintf(l.Text(field.label), field.value)
			column := ui.Column{Gap: ui.Space4}
			amount := func(gtx layout.Context) layout.Dimensions {
				return semanticRow(gtx, summary, func(gtx layout.Context) layout.Dimensions {
					return column.Layout(gtx,
						ui.Rigid(view.Body(ui.TextCaption, label, colors.SecondaryInk)),
						ui.Rigid(view.Body(ui.TextLabel, field.value, field.ink)),
					)
				})
			}
			if field.reason == "" {
				return amount(gtx)
			}
			reason := l.Text(holdingMissingReason(field.reason))
			return column.Layout(gtx, ui.Rigid(amount), ui.Rigid(view.Body(ui.TextCaption, reason, colors.SecondaryInk)))
		})
	}
}

func holdingMissingReason(reason string) i18n.MessageID {
	switch reason {
	case "holding acquisition cost is unavailable":
		return i18n.PortfolioCostMissing
	case "current instrument price is unavailable":
		return i18n.PortfolioPriceMissing
	case "current foreign-exchange rate is unavailable":
		return i18n.PortfolioHoldingFXMissing
	default:
		return i18n.PortfolioHoldingEvidenceMissing
	}
}
