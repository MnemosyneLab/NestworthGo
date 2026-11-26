package desktop

import (
	"fmt"
	"image/color"
	"maps"
	"slices"
	"strings"

	"gioui.org/io/event"
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
	toggle, instrument, account, history, returns, changes widget.Clickable
}

const holdingTableWidth = 780

var holdingTableInset = layout.Inset{Left: ui.Space12, Right: ui.Space12}

// Both the header and each virtualized row use these same column widths.
func holdingTableColumns(gtx layout.Context, cell func(layout.Context, int) layout.Dimensions, actions layout.Widget) layout.Dimensions {
	weights := [holdingSortCount]float32{2.5, 1, 1.28, 1.42, 1.55}
	var children [holdingSortCount + 1]layout.FlexChild
	for index, weight := range weights {
		children[index] = layout.Flexed(weight, func(gtx layout.Context) layout.Dimensions { return cell(gtx, index) })
	}
	children[holdingSortCount] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		width := min(gtx.Dp(82), gtx.Constraints.Max.X)
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
		return layout.E.Layout(gtx, actions)
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
	openActions  portfolioHoldingKey
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
	p.holdings.openActions = portfolioHoldingKey{}
}

func (p *portfolioPage) holdingsControls(gtx layout.Context, view ui.View) layout.Dimensions {
	h, l := &p.holdings, view.Language
	reset := func() {
		p.list.Position = layout.Position{}
		h.openActions = portfolioHoldingKey{}
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
	pickers := []layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			return view.TabTrack(gtx, 2, func(gtx layout.Context, i int) layout.Dimensions {
				if i == 0 {
					return view.TabSegment(&h.byInstrument, l.Text(i18n.PortfolioByInstrument), !h.visit.byAccount)(gtx)
				}
				return view.TabSegment(&h.byAccount, l.Text(i18n.PortfolioByAccount), h.visit.byAccount)(gtx)
			})
		},
		view.Checkbox(&h.showZero, l.Text(i18n.PortfolioShowZero)),
	}
	visible := p.visibleHoldingGroups(l)
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
		pickers = append(pickers, view.ButtonOf(&h.toggleGroups, ui.ButtonSpec{Variant: ui.ButtonGhost, Size: ui.ButtonSmall, Title: l.Text(label)}))
	}
	return (ui.Column{Gap: ui.Space8}).Layout(gtx,
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return (ui.Wrap{Gap: ui.Space8, Middle: true}).Layout(gtx, len(pickers), func(gtx layout.Context, i int) layout.Dimensions {
				return pickers[i](gtx)
			})
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.search.Layout(ui.LimitWidth(gtx, 440), view, i18n.PortfolioSearch, i18n.PortfolioSearchHint, func(string) { reset() })
		}),
	)
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
	for _, group := range groups {
		rows = append(rows, p.holdingGroupHeader(view, group))
		if h.visit.collapsed[group.Key] {
			continue
		}
		if h.visit.byAccount {
			for _, row := range h.projection.accountRows[group.Key] {
				rows = append(rows, p.holdingCard(view, *row.instrument, row.member, group.Key))
			}
			continue
		}
		for _, instrument := range group.Instruments {
			rows = append(rows, p.holdingCard(view, instrument, nil, group.Key))
			if !h.visit.expanded[instrument.InstrumentID] {
				continue
			}
			for i := range instrument.Holdings {
				rows = append(rows, p.holdingCard(view, instrument, &instrument.Holdings[i], group.Key))
			}
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

func (p *portfolioPage) holdingGroupHeader(view ui.View, group domain.InstrumentHoldingGroupView) layout.Widget {
	h := &p.holdings
	open := !h.visit.collapsed[group.Key]
	button := portfolioButton(h.groups, group.Key)
	card := sharedHoldingGroupHeader(view, group.Key, len(group.Instruments), group.Subtotals, button, open, true, func() {
		if h.visit.collapsed == nil {
			h.visit.collapsed = make(map[domain.HoldingGroupKey]bool)
		}
		h.visit.collapsed[group.Key] = open
	})
	if !open {
		return card
	}
	return func(gtx layout.Context) layout.Dimensions {
		return (ui.Column{Gap: ui.Space4}).Layout(gtx, ui.Rigid(card), ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.holdingSortHeader(gtx, view, group.Key)
		}))
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
		wide := compact && ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, holdingTableWidth)
		return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
			if wide && len(subtotals) > 0 {
				return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space16)}.Layout(gtx,
					layout.Flexed(1, disclosure), layout.Flexed(2.2, holdingSubtotalTable(view, subtotals)))
			}
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

func holdingSubtotalTable(view ui.View, subtotals []domain.HoldingCurrencySubtotal) layout.Widget {
	l, colors := view.Language, view.Colors
	columns := func(first layout.Widget, amounts [3]layout.Widget) ui.ColumnChild {
		return ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
				layout.Flexed(0.45, first), layout.Flexed(1, amounts[0]), layout.Flexed(1.2, amounts[1]), layout.Flexed(1.4, amounts[2]))
		})
	}
	var captions [3]layout.Widget
	for i, label := range [...]i18n.MessageID{i18n.PortfolioCost, i18n.PortfolioNativeValue, i18n.PortfolioUnrealized} {
		captions[i] = func(gtx layout.Context) layout.Dimensions {
			return layout.E.Layout(gtx, view.Body(ui.TextCaption, holdingMetricLabel(l, label), colors.SecondaryInk))
		}
	}
	rows := []ui.ColumnChild{columns(view.Body(ui.TextCaption, l.Text(i18n.AnalysisCurrency), colors.SecondaryInk), captions)}
	for _, subtotal := range subtotals {
		currency := subtotal.Currency.String()
		name := fmt.Sprintf(l.Text(i18n.PortfolioCurrencySubtotal), currency)
		label := func(gtx layout.Context) layout.Dimensions {
			return semanticRow(gtx, name, view.Body(ui.TextCaptionStrong, currency, colors.SecondaryInk))
		}
		var amounts [3]layout.Widget
		for i, metric := range holdingMetricValues(view, subtotal.Amounts, "", false) {
			amounts[i] = func(gtx layout.Context) layout.Dimensions {
				name := fmt.Sprintf(l.Text(metric.label), metric.value)
				if metric.reason != "" {
					name += " · " + l.Text(holdingMissingReason(metric.reason))
				}
				return semanticRow(gtx, name, func(gtx layout.Context) layout.Dimensions {
					return view.LineEnd(gtx, ui.TextLabel, metric.value, metric.ink)
				})
			}
		}
		rows = append(rows, columns(label, amounts))
	}
	return func(gtx layout.Context) layout.Dimensions {
		return (ui.Column{Gap: ui.Space4}).Layout(gtx, rows...)
	}
}

func (p *portfolioPage) holdingCard(view ui.View, row domain.InstrumentHoldingsView, member *domain.InstrumentHoldingMember, group domain.HoldingGroupKey) layout.Widget {
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
						spec.Title += " · " + fullName
						spec.Description = fullName
					}
				}
				return layout.W.Layout(gtx, view.ButtonOf(button, spec))
			}
		}
	}
	var accountControl layout.Widget
	if member == nil {
		disclosure := portfolioButton(h.disclosures, row.InstrumentID)
		accountControl = func(gtx layout.Context) layout.Dimensions {
			open := h.visit.expanded[row.InstrumentID]
			if disclosure.Clicked(gtx) {
				if h.visit.expanded == nil {
					h.visit.expanded = make(map[domain.InstrumentID]bool)
				}
				open = !open
				h.visit.expanded[row.InstrumentID] = open
				gtx.Execute(op.InvalidateCmd{})
			}
			count := len(row.Holdings)
			countLabel := i18n.PortfolioAccountCount
			if count == 1 {
				countLabel = i18n.PortfolioAccountOne
			}
			return analysisDisclosure(view, disclosure, fmt.Sprintf(l.Text(countLabel), count), open)(gtx)
		}
	} else {
		accountControl = p.holdingAccountLink(view, &p.holdingActions(identity).account, member.AccountID, member.AccountName)
	}
	unit := row.QuantityUnit
	if group == domain.HoldingGroupCashLike {
		unit = l.Text(i18n.PortfolioContractUnit)
	}
	narrowHeading := func(gtx layout.Context) layout.Dimensions {
		var fields []ui.ColumnChild
		if instrument != nil {
			fields = append(fields, ui.Rigid(instrument(false)))
			if fullName != "" && fullName != name {
				fields = append(fields, ui.Rigid(view.Body(ui.TextCaption, fullName, view.Colors.SecondaryInk)))
			}
		}
		fields = append(fields, ui.Rigid(accountControl))
		return (ui.Column{Gap: ui.Space4}).Layout(gtx, fields...)
	}
	compactHeading := func(gtx layout.Context) layout.Dimensions {
		if instrument == nil {
			return layout.Inset{Left: ui.Space16}.Layout(gtx, accountControl)
		}
		support := layout.Flexed(1, accountControl)
		if member == nil {
			support = layout.Rigid(accountControl)
		}
		return (layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space4)}).Layout(gtx,
			layout.Flexed(1, instrument(true)), support)
	}
	links := p.holdingActionLinks(view, identity, account, name)
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		if ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, holdingTableWidth) {
			return p.compactHoldingRow(gtx, view, compactHeading, identity, name, holdingMetricValues(view, amounts, unit, true), links)
		}
		wrapped := func(gtx layout.Context) layout.Dimensions {
			return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
				rows := []ui.ColumnChild{
					ui.Rigid(narrowHeading), ui.Rigid(view.Hairline), ui.Rigid(holdingMetrics(view, amounts, unit, true)),
					ui.Rigid(p.holdingActionToggle(view, identity, name, false)),
				}
				if h.openActions == identity {
					rows = append(rows, ui.Rigid(links))
				}
				return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...)
			})
		}
		if compactMember {
			return layout.Inset{Left: ui.Space16}.Layout(gtx, wrapped)
		}
		return wrapped(gtx)
	}
}

func (p *portfolioPage) compactHoldingRow(gtx layout.Context, view ui.View, identity layout.Widget, id portfolioHoldingKey, name string, metrics []holdingMetric, links layout.Widget) layout.Dimensions {
	l := view.Language
	rows := []ui.ColumnChild{
		ui.Rigid(view.Hairline),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return holdingTableInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return holdingTableColumns(gtx, func(gtx layout.Context, index int) layout.Dimensions {
					if index == int(holdingSortName) {
						return identity(gtx)
					}
					field := metrics[index-1]
					return semanticRow(gtx, fmt.Sprintf(l.Text(field.label), field.value), func(gtx layout.Context) layout.Dimensions {
						return view.LineEnd(gtx, ui.TextLabel, field.value, field.ink)
					})
				}, p.holdingActionToggle(view, id, name, true))
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
	if p.holdings.openActions == id {
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions { return holdingTableInset.Layout(gtx, links) }))
	}
	return (ui.Column{Gap: ui.Space4}).Layout(gtx, rows...)
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

func (p *portfolioPage) holdingActionToggle(view ui.View, id portfolioHoldingKey, name string, compact bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		h, l := &p.holdings, view.Language
		if p.openHoldingHistory == nil && p.openHoldingAnalysis == nil {
			return layout.Dimensions{}
		}
		controls := p.holdingActions(id)
		open := h.openActions == id
		if controls.toggle.Clicked(gtx) {
			h.openActions = portfolioHoldingKey{}
			if !open {
				h.openActions = id
			}
			open = !open
			gtx.Execute(op.InvalidateCmd{})
		}
		label := fmt.Sprintf(l.Text(i18n.PortfolioActionsFor), name)
		if compact {
			return view.ButtonOf(&controls.toggle, ui.ButtonSpec{Variant: ui.ButtonGhost, Size: ui.ButtonCompact,
				Name: label, Icon: lucide.Ellipsis, Toggle: true, Selected: open})(gtx)
		}
		return analysisDisclosure(view, &controls.toggle, label, open)(gtx)
	}
}

func (p *portfolioPage) holdingActionLinks(view ui.View, id portfolioHoldingKey, account domain.AccountID, name string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		h, l := &p.holdings, view.Language
		if h.openActions != id {
			return layout.Dimensions{}
		}
		controls := p.holdingActions(id)
		action := func(button *widget.Clickable, label i18n.MessageID, selectAction func(event.Tag)) layout.Widget {
			return func(gtx layout.Context) layout.Dimensions {
				if button.Clicked(gtx) {
					h.openActions = portfolioHoldingKey{}
					selectAction(&controls.toggle)
					gtx.Execute(op.InvalidateCmd{})
				}
				return view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: l.Text(label)})(gtx)
			}
		}
		var children []layout.Widget
		if p.openHoldingAnalysis != nil {
			children = append(children, action(&controls.returns, i18n.PortfolioViewReturn, func(focus event.Tag) {
				p.openHoldingAnalysis(returnAnalysisRoute, id.instrument, account, name, focus)
			}), action(&controls.changes, i18n.PortfolioViewChanges, func(focus event.Tag) {
				p.openHoldingAnalysis(assetChangesRoute, id.instrument, account, name, focus)
			}))
		}
		if p.openHoldingHistory != nil {
			children = append(children, action(&controls.history, i18n.PortfolioInstrumentHistory, func(focus event.Tag) {
				p.openHoldingHistory(id.instrument, account, name, focus)
			}))
		}
		return (ui.Wrap{Gap: ui.Space8, Middle: true}).Layout(gtx, len(children), func(gtx layout.Context, i int) layout.Dimensions {
			return children[i](gtx)
		})
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
			children := []ui.ColumnChild{
				ui.Rigid(view.Body(ui.TextCaption, label, colors.SecondaryInk)),
				ui.Rigid(view.Body(ui.TextLabel, field.value, field.ink)),
			}
			if field.reason != "" {
				children = append(children, ui.Rigid(view.Body(ui.TextCaption, l.Text(holdingMissingReason(field.reason)), colors.SecondaryInk)))
			}
			return semanticRow(gtx, fmt.Sprintf(l.Text(field.label), field.value), func(gtx layout.Context) layout.Dimensions {
				return (ui.Column{Gap: ui.Space4}).Layout(gtx, children...)
			})
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
