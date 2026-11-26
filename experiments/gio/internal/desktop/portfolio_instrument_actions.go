package desktop

import (
	"gioui.org/io/event"
	"gioui.org/layout"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *portfolioPage) instrumentRelatedActions(view ui.View, instrument domain.Instrument) layout.Widget {
	detail := &p.detail
	name, account := instrument.DisplayLabel(), p.accountScope
	var actions []layout.Widget
	if detail.openAnalysis != nil {
		open := func(route routeID, focus event.Tag) {
			selection := instrumentAnalysisSelection(instrument.ID, account, detail.visit.actionDates)
			detail.openAnalysis(route, selection, name, focus)
		}
		actions = append(actions,
			accountDetailButton(view, &detail.returns, i18n.NavReturns, ui.ButtonGhost, func() {
				open(returnAnalysisRoute, &detail.returns)
			}),
			accountDetailButton(view, &detail.changes, i18n.NavAssetChanges, ui.ButtonGhost, func() {
				open(assetChangesRoute, &detail.changes)
			}))
	}
	if detail.openTransactions != nil {
		actions = append(actions, accountDetailButton(view, &detail.transactions, i18n.PortfolioRelatedTransactions, ui.ButtonOutline, func() {
			dates := detail.visit.actionDates
			detail.openTransactions(history.Filters{Instrument: instrument.ID, Account: account, From: dates.from, To: dates.to}, name, &detail.transactions)
		}))
	}
	if len(actions) == 0 {
		return nil
	}
	return accountDetailActions(view, actions)
}
