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

type portfolioHoldings struct {
	visit        portfolioHoldingsVisit
	byInstrument widget.Clickable
	byAccount    widget.Clickable
	showZero     widget.Bool
	toggleGroups widget.Clickable
	sortButtons  [holdingSortCount]widget.Clickable
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
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			if len(visible) == 0 {
				return layout.Dimensions{}
			}
			return p.holdingSortControls(gtx, view, reset)
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
	return sharedHoldingGroupHeader(view, group.Key, len(group.Instruments), group.Subtotals, button, open, func() {
		if h.visit.collapsed == nil {
			h.visit.collapsed = make(map[domain.HoldingGroupKey]bool)
		}
		h.visit.collapsed[group.Key] = open
	})
}

func sharedHoldingGroupHeader(view ui.View, group domain.HoldingGroupKey, count int, subtotals []domain.HoldingCurrencySubtotal, button *widget.Clickable, open bool, toggle func()) layout.Widget {
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
	fields := []ui.ColumnChild{ui.Rigid(disclosure)}
	for _, subtotal := range subtotals {
		currency := subtotal.Currency.String()
		fields = append(fields,
			ui.Rigid(view.Body(ui.TextLabel, fmt.Sprintf(l.Text(i18n.PortfolioCurrencySubtotal), currency), view.Colors.SecondaryInk)),
			ui.Rigid(holdingMetrics(view, subtotal.Amounts, "", false)))
	}
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
			return (ui.Column{Gap: ui.Space8}).Layout(gtx, fields...)
		})
	}
}

func (p *portfolioPage) holdingCard(view ui.View, row domain.InstrumentHoldingsView, member *domain.InstrumentHoldingMember, group domain.HoldingGroupKey) layout.Widget {
	l, h := view.Language, &p.holdings
	name := holdingInstrumentName(row)
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
	var fields []ui.ColumnChild
	if !compactMember {
		button := portfolioButton(p.instruments, row.InstrumentID)
		if member != nil {
			button = &p.holdingActions(identity).instrument
		}
		fields = append(fields, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			if button.Clicked(gtx) {
				if account != "" && p.openHoldingInstrument != nil {
					p.openHoldingInstrument(row.InstrumentID, account, name, button)
				} else if p.openInstrument != nil {
					p.openInstrument(row.InstrumentID, name, button)
				}
				gtx.Execute(op.InvalidateCmd{})
			}
			return layout.W.Layout(gtx, view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonSoft, Title: title, Name: title}))
		}))
		if fullName := strings.TrimSpace(row.Name); fullName != "" && fullName != name {
			fields = append(fields, ui.Rigid(view.Body(ui.TextCaption, fullName, view.Colors.SecondaryInk)))
		}
	}
	if member == nil {
		disclosure := portfolioButton(h.disclosures, row.InstrumentID)
		fields = append(fields, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
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
		}))
	} else {
		fields = append(fields, ui.Rigid(p.holdingAccountLink(view, &p.holdingActions(identity).account, member.AccountID, member.AccountName)))
	}
	unit := row.QuantityUnit
	if group == domain.HoldingGroupCashLike {
		unit = l.Text(i18n.PortfolioContractUnit)
	}
	heading := func(gtx layout.Context) layout.Dimensions {
		return (ui.Column{Gap: ui.Space4}).Layout(gtx, fields...)
	}
	metrics := holdingMetrics(view, amounts, unit, true)
	actions := p.holdingActionRow(view, identity, account, name)
	return func(gtx layout.Context) layout.Dimensions {
		wrapped := func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
				if ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, 795) {
					return (layout.Flex{Gap: gtx.Dp(ui.Space16)}).Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return (ui.Column{Gap: ui.Space4}).Layout(gtx, ui.Rigid(heading), ui.Rigid(actions))
						}),
						layout.Flexed(2.3, metrics))
				}
				return (ui.Column{Gap: ui.Space8}).Layout(gtx,
					ui.Rigid(heading), ui.Rigid(view.Hairline), ui.Rigid(metrics), ui.Rigid(actions))
			})
		}
		if compactMember {
			return layout.Inset{Left: ui.Space16}.Layout(gtx, wrapped)
		}
		return wrapped(gtx)
	}
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

func (p *portfolioPage) holdingActionRow(view ui.View, id portfolioHoldingKey, account domain.AccountID, name string) layout.Widget {
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
		children := []layout.Widget{analysisDisclosure(view, &controls.toggle, fmt.Sprintf(l.Text(i18n.PortfolioActionsFor), name), open)}
		if open {
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

func holdingMetrics(view ui.View, amounts domain.HoldingAmounts, unit string, includeQuantity bool) layout.Widget {
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
	fields = append(fields,
		holdingMetric{i18n.PortfolioCost, money(amounts.TotalCost), amounts.CostMissingReason, colors.Ink},
		holdingMetric{i18n.PortfolioNativeValue, money(amounts.CurrentValue), amounts.ValueMissingReason, colors.Ink},
		holdingMetric{i18n.PortfolioUnrealized, gain, amounts.GainMissingReason, gainInk})
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
