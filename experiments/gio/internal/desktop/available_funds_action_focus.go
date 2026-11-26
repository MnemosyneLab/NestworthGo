package desktop

import (
	"slices"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *availableFundsPage) openActionFocus(view ui.View) {
	focus := p.actionFocus
	if focus.kind == 0 || p.current == nil || p.request != nil {
		return
	}
	p.actionFocus = fundsActionFocus{}
	sources := p.current.report.Sources
	at := slices.IndexFunc(sources, func(row application.LiquidityReadSource) bool {
		return row.Result.Ref.Key() == focus.sourceKey && row.Result.Ref.AccountID == focus.account
	})
	switch focus.kind {
	case fundsActionPolicy:
		switch {
		case at < 0:
			p.actionNotice = i18n.TasksTargetMissing
		case p.policyLedger == nil || !editableFundsPolicy(sources[at]):
			p.actionNotice = i18n.FundsPolicyReadOnly
		default:
			p.startPolicy(sources[at], p.sourceTitle(view, sources[at].Result), nil)
		}
	case fundsActionReservations:
		if p.reservationLedger == nil {
			p.actionNotice = i18n.TasksTargetMissing
			return
		}
		var ref domain.LiquiditySourceRef
		var name string
		if at >= 0 {
			ref, name = sources[at].Result.Ref, sources[at].Result.DisplayName
		}
		p.openReservations(ref, name, &p.reload)
		p.reservation.focusTarget = focus.reservation
		if at < 0 && focus.sourceKey != "" {
			p.reservation.notice = i18n.TasksReserveSourceChanged
		}
	}
}
