package desktop

import (
	"fmt"
	"maps"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type accountPositionsFilter int

const (
	accountPositionsCurrent accountPositionsFilter = iota
	accountPositionsClosed
	accountPositionsAll
)

var accountPositionsLabels = [...]i18n.MessageID{
	i18n.AccountPositionsCurrent, i18n.AccountPositionsClosed, i18n.AccountPositionsAll,
}

type accountPositionsVisit struct {
	filter                accountPositionsFilter
	sort                  holdingSort
	collapsed             map[domain.HoldingGroupKey]bool
	openActions, expanded domain.HoldingID
	trend                 instrumentPerformanceVisit
}

func (v accountPositionsVisit) copy() accountPositionsVisit {
	v.collapsed = maps.Clone(v.collapsed)
	return v
}

type accountPositions struct {
	visit   accountPositionsVisit
	filter  [len(accountPositionsLabels)]widget.Clickable
	sort    [holdingSortCount]widget.Clickable
	groups  map[domain.HoldingGroupKey]*widget.Clickable
	actions map[domain.HoldingID]*portfolioHoldingActions
	retry   widget.Clickable
	expand  map[domain.HoldingID]*widget.Clickable
	trend   instrumentPerformance
}

func (p *accountsPage) accountPositionRows(view ui.View, account domain.Account, first int) []layout.Widget {
	l, positions := view.Language, &p.positions
	rows := []layout.Widget{accountSection(view, l.Text(i18n.AccountInvestments))}
	appendShortcuts := func(rows []layout.Widget) []layout.Widget {
		if actions := p.accountInvestmentShortcuts(view); len(actions) != 0 {
			p.detail.investmentActionsRow = first + len(rows)
			return append(rows, accountDetailActions(view, actions))
		}
		return rows
	}
	if p.current.holdingsErr != nil {
		return append(rows, accountLine(view, l.Text(i18n.AccountPositionsReadFailed)), func(gtx layout.Context) layout.Dimensions {
			if p.request != nil {
				gtx = gtx.Disabled()
			}
			if positions.retry.Clicked(gtx) {
				p.refresh()
			}
			return layout.W.Layout(gtx, view.Button(&positions.retry, l.Text(i18n.HistoryRetry)))
		})
	}
	type groupRows struct {
		key       domain.HoldingGroupKey
		rows      []portfolioHoldingRow
		subtotals []domain.HoldingCurrencySubtotal
	}
	var groups []groupRows
	ordinary := false
	for _, group := range p.current.holdingGroups {
		if group.Key == domain.HoldingGroupCashLike {
			continue
		}
		var rendered []portfolioHoldingRow
		for i := range group.Instruments {
			instrument := &group.Instruments[i]
			for j := range instrument.Holdings {
				member := &instrument.Holdings[j]
				if member.AccountID != account.ID {
					continue
				}
				ordinary = true
				closed := member.Amounts.Quantity == "0"
				if positions.visit.filter == accountPositionsAll || closed == (positions.visit.filter == accountPositionsClosed) {
					rendered = append(rendered, portfolioHoldingRow{instrument: instrument, member: member})
				}
			}
		}
		if len(rendered) == 0 {
			continue
		}
		sortPortfolioHoldingRows(rendered, positions.visit.sort, l)
		subtotals := group.Subtotals
		if positions.visit.filter == accountPositionsClosed {
			subtotals = nil
		}
		groups = append(groups, groupRows{group.Key, rendered, subtotals})
	}
	if !ordinary {
		return appendShortcuts(append(rows, accountLine(view, l.Text(i18n.AccountNoInvestments))))
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return view.TabTrack(gtx, len(accountPositionsLabels), func(gtx layout.Context, i int) layout.Dimensions {
			button, filter := &positions.filter[i], accountPositionsFilter(i)
			if button.Clicked(gtx) && positions.visit.filter != filter {
				positions.visit.filter, positions.visit.openActions, positions.visit.expanded = filter, "", ""
				positions.trend.series.reset()
				gtx.Execute(op.InvalidateCmd{})
			}
			return view.TabSegment(button, l.Text(accountPositionsLabels[i]), positions.visit.filter == filter)(gtx)
		})
	})
	if positions.visit.filter == accountPositionsAll {
		rows = append(rows, accountLine(view, l.Text(i18n.AccountPositionsSubtotalHelp)))
	}
	if len(groups) == 0 {
		message := i18n.AccountPositionsNoCurrent
		if positions.visit.filter == accountPositionsClosed {
			message = i18n.AccountPositionsNoClosed
		}
		return appendShortcuts(append(rows, accountLine(view, l.Text(message))))
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		wide := ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, holdingTableWidth)
		return layoutHoldingSortControls(gtx, view, &positions.visit.sort, &positions.sort, wide, func() {
			gtx.Execute(op.InvalidateCmd{})
		})
	})
	if positions.groups == nil {
		positions.groups = make(map[domain.HoldingGroupKey]*widget.Clickable)
	}
	for _, group := range groups {
		open := !positions.visit.collapsed[group.key]
		button := portfolioButton(positions.groups, group.key)
		rows = append(rows, sharedHoldingGroupHeader(view, group.key, len(group.rows), group.subtotals, button, open, true, func() {
			if positions.visit.collapsed == nil {
				positions.visit.collapsed = make(map[domain.HoldingGroupKey]bool)
			}
			positions.visit.collapsed[group.key] = open
			if open {
				for _, row := range group.rows {
					if row.member.HoldingID == positions.visit.expanded {
						positions.visit.expanded = ""
						positions.trend.series.reset()
					}
				}
			}
		}))
		if open {
			for _, row := range group.rows {
				rows = append(rows, p.accountHoldingRow(view, account, row, group.key))
				if positions.visit.expanded == row.member.HoldingID {
					rows = append(rows, p.accountHoldingTrend(view, *row.instrument)...)
				}
			}
		}
	}
	return appendShortcuts(rows)
}

func (p *accountsPage) accountInvestmentShortcuts(view ui.View) []layout.Widget {
	var actions []layout.Widget
	add := func(button *widget.Clickable, label i18n.MessageID) {
		actions = append(actions, func(gtx layout.Context) layout.Dimensions {
			p.investment.watchAccountAction(gtx, view, button)
			return view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonOutline, Size: ui.ButtonSmall,
				Title: view.Language.Text(label)})(gtx)
		})
	}
	f := p.investment
	if p.canTradeAccount() {
		label := i18n.BuyExistingOpen
		if p.canFirstBuy() {
			label = i18n.BuyOpen
		}
		add(&f.buy, label)
	}
	if p.canSell() {
		add(&f.sell, i18n.SellOpen)
	}
	if p.canRecordDividend() {
		add(&f.dividend, i18n.DividendOpen)
	}
	if p.canImportPosition() {
		add(&f.importOpen, i18n.OpeningPositionOpen)
		add(&f.custodyOpen, i18n.CustodyOpen)
		add(&f.adjustmentOpen, i18n.QuantityAdjustOpen)
	}
	return actions
}

// The account list restores its prior offset on Back. Changed cash or
// composition above a shortcut can push it outside that saved viewport.
func (p *accountsPage) revealAccountDetailAction(focus event.Tag) bool {
	f, detail := p.investment, &p.detail
	if focus == nil || detail.revealedShortcut == focus {
		return false
	}
	if button, ok := focus.(*widget.Clickable); ok {
		if row, found := detail.productRows[button]; found {
			p.list.Position = layout.Position{First: row}
			detail.revealedShortcut = focus
			return true
		}
	}
	if f == nil || detail.investmentActionsRow < 0 {
		return false
	}
	switch focus {
	case &f.buy, &f.sell, &f.dividend, &f.importOpen, &f.custodyOpen, &f.adjustmentOpen:
		p.list.Position = layout.Position{First: detail.investmentActionsRow}
		detail.revealedShortcut = focus
		return true
	default:
		return false
	}
}

func (p *accountsPage) accountHoldingRow(view ui.View, account domain.Account, row portfolioHoldingRow, group domain.HoldingGroupKey) layout.Widget {
	l, instrument, member := view.Language, row.instrument, row.member
	positions := &p.positions
	if positions.expand == nil {
		positions.expand = make(map[domain.HoldingID]*widget.Clickable)
	}
	button := portfolioButton(positions.expand, member.HoldingID)
	name := holdingInstrumentName(*instrument)
	title := instrument.Name
	if title == "" {
		title = name
	}
	metadata := l.Text(holdingGroupLabel(group))
	if instrument.Symbol != "" && instrument.Symbol != title {
		metadata = instrument.Symbol + " · " + metadata
	}
	if instrument.Archived {
		metadata += " · " + l.Text(i18n.DirectoryArchived)
	}
	identity := func(gtx layout.Context) layout.Dimensions {
		open := positions.visit.expanded == member.HoldingID
		if button.Clicked(gtx) {
			positions.trend.series.reset()
			positions.trend.restore(instrumentPerformanceVisit{})
			positions.visit.expanded = member.HoldingID
			if open {
				positions.visit.expanded = ""
			}
			open = !open
			gtx.Execute(op.InvalidateCmd{})
		}
		var details []layout.FlexChild
		details = append(details, layout.Flexed(1, analysisDisclosure(view, button, title, open)))
		if p.openInstrument != nil {
			link := portfolioButton(p.instruments, instrument.InstrumentID)
			details = append(details, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				view.Focus.Watch(gtx, link)
				if link.Clicked(gtx) {
					p.openInstrument(instrument.InstrumentID, account.ID, name, link)
					gtx.Execute(op.InvalidateCmd{})
				}
				return view.ButtonOf(link, ui.ButtonSpec{Variant: ui.ButtonGhost, Size: ui.ButtonCompact, Icon: lucide.ArrowUpRight,
					Name: fmt.Sprintf(l.Text(i18n.PortfolioOpenInstrument), name)})(gtx)
			}))
		}
		return (ui.Column{Gap: ui.Space2}).Layout(gtx,
			ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				return (layout.Flex{Alignment: layout.Middle}).Layout(gtx, details...)
			}),
			ui.Rigid(view.Body(ui.TextCaption, metadata, view.Colors.SecondaryInk)))
	}
	holding := p.current.holdings[member.HoldingID]
	closed := member.Amounts.Quantity == "0"
	canAdjust := p.canImportPosition() && !closed && holding.ID != "" && holding.ArchivedAt == nil
	actions, links := p.accountHoldingActions(view, account, row, holding, name, canAdjust)
	return func(gtx layout.Context) layout.Dimensions {
		wide := ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, holdingTableWidth)
		fields := []ui.ColumnChild{
			ui.Rigid(view.Hairline),
			ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !wide {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, identity), layout.Rigid(actions))
				}
				metrics := holdingMetricValues(view, member.Amounts, instrument.QuantityUnit, true)
				return holdingTableColumns(gtx, func(gtx layout.Context, index int) layout.Dimensions {
					if index == int(holdingSortName) {
						return identity(gtx)
					}
					metric := metrics[index-1]
					return holdingAmountCell(gtx, view, fmt.Sprintf(l.Text(metric.label), metric.value), metric)
				}, actions)
			}),
		}
		if !wide {
			fields = append(fields, ui.Rigid(holdingMetrics(view, member.Amounts, instrument.QuantityUnit, true)))
		} else {
			var reasons map[string]bool
			for _, metric := range holdingMetricValues(view, member.Amounts, instrument.QuantityUnit, true) {
				if metric.reason == "" || reasons[metric.reason] {
					continue
				}
				if reasons == nil {
					reasons = make(map[string]bool)
				}
				reasons[metric.reason] = true
				fields = append(fields, ui.Rigid(view.Body(ui.TextCaption, l.Text(holdingMissingReason(metric.reason)), view.Colors.SecondaryInk)))
			}
		}
		if closed {
			fields = append(fields, ui.Rigid(view.Body(ui.TextCaption, l.Text(i18n.AccountPositionsClosedNote), view.Colors.SecondaryInk)))
		}
		if positions.visit.openActions == member.HoldingID {
			fields = append(fields, ui.Rigid(links))
		}
		return holdingTableInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return (ui.Column{Gap: ui.Space4}).Layout(gtx, fields...)
		})
	}
}

func (p *accountsPage) accountHoldingTrend(view ui.View, instrument domain.InstrumentHoldingsView) []layout.Widget {
	reader, ok := p.service.(instrumentPerformanceReader)
	if !ok {
		return []layout.Widget{view.Message(view.Language.Text(i18n.PortfolioPerformanceUnsupported), view.Colors.SecondaryInk)}
	}
	identity := domain.Instrument{ID: instrument.InstrumentID, Type: instrument.Type, QuoteCurrency: instrument.QuoteCurrency, QuantityUnit: instrument.QuantityUnit}
	return p.positions.trend.seriesRows(view, identity, reader, p.ctx, &p.workers, p.invalidate)
}

func (p *accountsPage) accountHoldingActions(view ui.View, account domain.Account, row portfolioHoldingRow, holding domain.Holding, name string, canAdjust bool) (layout.Widget, layout.Widget) {
	positions, l := &p.positions, view.Language
	id, instrument := row.member.HoldingID, row.instrument.InstrumentID
	if positions.actions == nil {
		positions.actions = make(map[domain.HoldingID]*portfolioHoldingActions)
	}
	if positions.actions[id] == nil {
		positions.actions[id] = new(portfolioHoldingActions)
	}
	controls := positions.actions[id]
	toggle := func(gtx layout.Context) layout.Dimensions {
		open := positions.visit.openActions == id
		if controls.toggle.Clicked(gtx) {
			positions.visit.openActions = id
			if open {
				positions.visit.openActions = ""
			}
			open = !open
			gtx.Execute(op.InvalidateCmd{})
		}
		view.Focus.Watch(gtx, &controls.toggle)
		return view.ButtonOf(&controls.toggle, ui.ButtonSpec{Variant: ui.ButtonGhost, Size: ui.ButtonCompact,
			Name: fmt.Sprintf(l.Text(i18n.PortfolioActionsFor), name), Icon: lucide.Ellipsis, Toggle: true, Selected: open})(gtx)
	}
	return toggle, func(gtx layout.Context) layout.Dimensions {
		var buttons []layout.Widget
		link := func(button *widget.Clickable, label i18n.MessageID, jump func(event.Tag)) layout.Widget {
			return func(gtx layout.Context) layout.Dimensions {
				if button.Clicked(gtx) {
					positions.visit.openActions = ""
					jump(&controls.toggle)
					gtx.Execute(op.InvalidateCmd{})
				}
				return view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: l.Text(label)})(gtx)
			}
		}
		if p.openHoldingAnalysis != nil {
			buttons = append(buttons,
				link(&controls.returns, i18n.PortfolioViewReturn, func(focus event.Tag) {
					p.openHoldingAnalysis(returnAnalysisRoute, instrument, account.ID, name, focus)
				}),
				link(&controls.changes, i18n.PortfolioViewChanges, func(focus event.Tag) {
					p.openHoldingAnalysis(assetChangesRoute, instrument, account.ID, name, focus)
				}))
		}
		if p.openHoldingHistory != nil {
			buttons = append(buttons, link(&controls.history, i18n.PortfolioInstrumentHistory, func(focus event.Tag) {
				p.openHoldingHistory(instrument, account.ID, name, focus)
			}))
		}
		if canAdjust && !row.instrument.Archived {
			f := p.investment
			if f.dividendHoldings == nil {
				f.dividendHoldings = make(map[domain.HoldingID]*widget.Clickable)
			}
			button := portfolioButton(f.dividendHoldings, id)
			buttons = append(buttons, func(gtx layout.Context) layout.Dimensions {
				if button.Clicked(gtx) {
					f.draft = newAccountInvestmentDraft(domain.ActivityCashDividend, account)
					f.draft.lockedHolding, f.draft.originAction = holding, button
					f.loadSources(p.investmentRuntime())
					gtx.Execute(op.InvalidateCmd{})
				}
				return f.accountPositionAction(gtx, view, button, l.Text(i18n.DividendOpen))
			})
		}
		if canAdjust {
			buttons = append(buttons, p.holdingPositionActions(view, account, holding, name))
		}
		return (ui.Wrap{Gap: ui.Space8, Middle: true}).Layout(gtx, len(buttons), func(gtx layout.Context, i int) layout.Dimensions {
			return buttons[i](gtx)
		})
	}
}
