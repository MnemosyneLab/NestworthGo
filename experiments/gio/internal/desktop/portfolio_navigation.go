package desktop

import (
	"gioui.org/io/event"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (s *shell) openHoldingHistory(instrument domain.InstrumentID, account domain.AccountID, name string, focus event.Tag) {
	s.openInstrumentTransactions(history.Filters{Account: account, Instrument: instrument}, name, focus)
}

func (s *shell) openInstrumentTransactions(filters history.Filters, name string, focus event.Tag) {
	s.pendingNavigation = &navigationRequest{
		target: navigationTarget{route: historyRoute, instrument: filters.Instrument},
		name:   name, focus: focus, historyFilters: filters,
	}
}

func (s *shell) openHoldingAnalysis(route routeID, instrument domain.InstrumentID, account domain.AccountID, name string, focus event.Tag) {
	s.openScopedAnalysis(route, instrumentAnalysisSelection(instrument, account, analysisDateWindow{}), name, focus)
}

func instrumentAnalysisSelection(instrument domain.InstrumentID, account domain.AccountID, dates analysisDateWindow) analysisSelection {
	return analysisSelection{
		scope:     domain.AnalysisScope{Kind: domain.ScopeInstrument, ID: instrument.String()},
		valuation: domain.ValuationBase, excludeCash: true, account: account.String(),
		from: domain.LocalDate(dates.from), to: domain.LocalDate(dates.to),
	}
}

func (s *shell) openScopedAnalysis(route routeID, selection analysisSelection, name string, focus event.Tag) {
	// An explicit detail range wins. Otherwise reuse dates from the last
	// analysis visit while replacing the rest with this object's scope.
	for i := s.trail.cursor - 1; i >= 0 && (selection.from == "" || selection.to == ""); i-- {
		entry := s.trail.entries[i]
		var previous analysisSelection
		switch entry.target.route {
		case returnAnalysisRoute:
			if entry.returns != nil {
				previous = entry.returns.selection
			}
		case assetChangesRoute:
			previous = entry.asset.selection
		}
		if previous.from != "" && previous.to != "" {
			selection.from, selection.to = previous.from, previous.to
			break
		}
	}
	request := navigationRequest{target: navigationTarget{route: route}, name: name, focus: focus}
	switch route {
	case returnAnalysisRoute:
		request.returns = &returnAnalysisVisit{selection: selection, month: s.returns.month, tab: returnTrendTab, metric: application.ContributionTotalReturn}
	case assetChangesRoute:
		request.asset = assetChangesVisit{selection: selection, tab: assetDriversTab}
	default:
		return
	}
	s.pendingNavigation = &request
}
