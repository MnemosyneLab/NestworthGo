package desktop

import (
	"gioui.org/io/event"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (s *shell) openHoldingHistory(instrument domain.InstrumentID, account domain.AccountID, name string, focus event.Tag) {
	s.pendingNavigation = &navigationRequest{
		target: navigationTarget{route: historyRoute, instrument: instrument},
		name:   name, focus: focus, historyFilters: history.Filters{Account: account, Instrument: instrument},
	}
}

func (s *shell) openHoldingAnalysis(route routeID, instrument domain.InstrumentID, account domain.AccountID, name string, focus event.Tag) {
	selection := analysisSelection{
		scope:     domain.AnalysisScope{Kind: domain.ScopeInstrument, ID: instrument.String()},
		valuation: domain.ValuationBase, excludeCash: true, account: account.String(),
	}
	// As in the Wails object action, carry the selected dates and replace the
	// rest of the analysis scope with this holding.
	for i := s.trail.cursor - 1; i >= 0; i-- {
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
