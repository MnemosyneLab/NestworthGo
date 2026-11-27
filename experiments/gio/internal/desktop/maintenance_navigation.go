package desktop

import (
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

var maintenanceChildren = [...]struct {
	route routeID
	label i18n.MessageID
}{
	{dataHealthRoute, i18n.MaintenanceIssues},
	{marketDataRoute, i18n.MaintenanceQuotes},
}

func isMaintenanceRoute(route routeID) bool {
	return route == dataHealthRoute || route == marketDataRoute
}

func (s *shell) hasMaintenanceTabs() bool {
	return s.dataHealth != nil && s.marketData != nil && isMaintenanceRoute(s.activeRoute())
}

func (s *shell) layoutMaintenanceTabs(gtx layout.Context, view ui.View) layout.Dimensions {
	inset := layout.Inset{Top: ui.Space12, Left: pageInset.Left, Right: pageInset.Right}
	return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return view.TabTrack(gtx, len(s.maintenanceTabs), func(gtx layout.Context, i int) layout.Dimensions {
			tab, button := maintenanceChildren[i], &s.maintenanceTabs[i]
			selected := s.activeRoute() == tab.route
			if button.Clicked(gtx) && !selected {
				s.pendingNavigation = &navigationRequest{target: navigationTarget{route: tab.route}, focus: button, maintenanceTab: true}
				gtx.Execute(op.InvalidateCmd{})
			}
			view.Focus.Watch(gtx, button)
			return view.TabSegment(button, view.Language.Text(tab.label), selected)(gtx)
		})
	})
}

// An old instrument reference with only an account and dates is a records
// link. Issue context keeps the repair route. Provider settings have their
// own destination whichever Maintenance child initiated the request.
func resolveMaintenanceTarget(request navigationRequest) navigationRequest {
	target := request.target
	if !isMaintenanceRoute(target.route) {
		return request
	}
	focus := target.health
	if focus.action == application.HealthActionProviderSettings {
		request.target = navigationTarget{route: settingsRoute}
		request.providerSettings, request.settingsProvider = true, focus.provider
		return request
	}
	if target.route != marketDataRoute || focus.instrument == "" || focus.id != "" || focus.kind != "" ||
		focus.action != "" || focus.reason != "" || focus.executableSpecified || focus.snapshot != (snapshotFindingKey{}) ||
		focus.currencyA != "" || focus.currencyB != "" {
		return request
	}
	request.target = navigationTarget{route: portfolioRoute, instrument: domain.InstrumentID(focus.instrument),
		portfolioAccount: domain.AccountID(focus.account), instrumentData: true, health: focus}
	if request.name == "" {
		request.name = focus.label
	}
	return request
}
