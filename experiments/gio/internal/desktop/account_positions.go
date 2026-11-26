package desktop

import (
	"fmt"
	"maps"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

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
	filter      accountPositionsFilter
	sort        holdingSort
	collapsed   map[domain.HoldingGroupKey]bool
	openActions domain.HoldingID
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
}

func (p *accountsPage) accountPositionRows(view ui.View, account domain.Account) []layout.Widget {
	l, positions := view.Language, &p.positions
	rows := []layout.Widget{accountSection(view, l.Text(i18n.AccountInvestments))}
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
		return append(rows, accountLine(view, l.Text(i18n.AccountNoInvestments)))
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return view.TabTrack(gtx, len(accountPositionsLabels), func(gtx layout.Context, i int) layout.Dimensions {
			button, filter := &positions.filter[i], accountPositionsFilter(i)
			if button.Clicked(gtx) && positions.visit.filter != filter {
				positions.visit.filter, positions.visit.openActions = filter, ""
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
		return append(rows, accountLine(view, l.Text(message)))
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return layoutHoldingSortControls(gtx, view, &positions.visit.sort, &positions.sort, false, func() {
			gtx.Execute(op.InvalidateCmd{})
		})
	})
	if positions.groups == nil {
		positions.groups = make(map[domain.HoldingGroupKey]*widget.Clickable)
	}
	for _, group := range groups {
		open := !positions.visit.collapsed[group.key]
		button := portfolioButton(positions.groups, group.key)
		rows = append(rows, sharedHoldingGroupHeader(view, group.key, len(group.rows), group.subtotals, button, open, func() {
			if positions.visit.collapsed == nil {
				positions.visit.collapsed = make(map[domain.HoldingGroupKey]bool)
			}
			positions.visit.collapsed[group.key] = open
		}))
		if open {
			for _, row := range group.rows {
				rows = append(rows, p.accountHoldingCard(view, account, row))
			}
		}
	}
	return rows
}

func (p *accountsPage) accountHoldingCard(view ui.View, account domain.Account, row portfolioHoldingRow) layout.Widget {
	l, instrument, member := view.Language, row.instrument, row.member
	name := holdingInstrumentName(*instrument)
	title := name
	if instrument.Archived {
		title += " · " + l.Text(i18n.DirectoryArchived)
	}
	amounts := member.Amounts
	closed := amounts.Quantity == "0"
	quantity := fmt.Sprintf(l.Text(i18n.AccountQuantity), portfolioDecimal(view, amounts.Quantity))
	native := l.Text(i18n.OverviewNoValue)
	if amounts.CurrentValue != nil {
		native = valuationMoney(view, *amounts.CurrentValue, false)
	}
	fields := []layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			return semanticRow(gtx, title+" · "+quantity+" · "+native, view.Body(ui.TextLabel, title, view.Colors.Ink))
		},
		holdingAmounts(view, amounts, instrument.QuantityUnit),
	}
	if closed {
		fields = append(fields, view.Body(ui.TextCaption, l.Text(i18n.AccountPositionsClosedNote), view.Colors.SecondaryInk))
	}
	if p.openInstrument != nil {
		button := portfolioButton(p.instruments, instrument.InstrumentID)
		fields = append(fields, func(gtx layout.Context) layout.Dimensions {
			if button.Clicked(gtx) {
				p.openInstrument(instrument.InstrumentID, account.ID, name, button)
				gtx.Execute(op.InvalidateCmd{})
			}
			return layout.W.Layout(gtx, view.Button(button, fmt.Sprintf(l.Text(i18n.PortfolioOpenInstrument), name)))
		})
	}
	holding := p.current.holdings[member.HoldingID]
	if p.canImportPosition() && !closed && holding.ID != "" && holding.ArchivedAt == nil {
		fields = append(fields, p.holdingPositionActions(view, account, holding, name))
	}
	dividend := p.canImportPosition() && !closed && !instrument.Archived && holding.ID != "" &&
		instrument.QuoteCurrency == account.DefaultCurrency
	if p.openHoldingHistory != nil || p.openHoldingAnalysis != nil || dividend {
		fields = append(fields, p.accountHoldingActions(view, account, row, holding, name, dividend))
	}
	return sectionCard(view, fields)
}

func (p *accountsPage) accountHoldingActions(view ui.View, account domain.Account, row portfolioHoldingRow, holding domain.Holding, name string, canDividend bool) layout.Widget {
	positions, l := &p.positions, view.Language
	id, instrument := row.member.HoldingID, row.instrument.InstrumentID
	if positions.actions == nil {
		positions.actions = make(map[domain.HoldingID]*portfolioHoldingActions)
	}
	if positions.actions[id] == nil {
		positions.actions[id] = new(portfolioHoldingActions)
	}
	controls := positions.actions[id]
	return func(gtx layout.Context) layout.Dimensions {
		open := positions.visit.openActions == id
		if controls.toggle.Clicked(gtx) {
			positions.visit.openActions = ""
			if !open {
				positions.visit.openActions = id
			}
			open = !open
			gtx.Execute(op.InvalidateCmd{})
		}
		buttons := []layout.Widget{analysisDisclosure(view, &controls.toggle, fmt.Sprintf(l.Text(i18n.PortfolioActionsFor), name), open)}
		if open {
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
			if canDividend {
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
						view.RestorePageScroll(0)
						gtx.Execute(op.InvalidateCmd{})
					}
					return f.accountPositionAction(gtx, view, button, l.Text(i18n.DividendOpen))
				})
			}
		}
		return (ui.Wrap{Gap: ui.Space8, Middle: true}).Layout(gtx, len(buttons), func(gtx layout.Context, i int) layout.Dimensions {
			return buttons[i](gtx)
		})
	}
}
