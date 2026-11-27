package desktop

import (
	"image"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type accountActionSheet struct {
	panel   analysisSheet
	overlay ui.Overlay
	cancel  *widget.Clickable
}

func (p *accountsPage) accountActionOpen() bool {
	return p.accountActionTitle() != i18n.NoMessage
}

func (p *accountsPage) accountActionTitle() i18n.MessageID {
	switch {
	case p.maintenance != nil && p.maintenance.draft != nil:
		return i18n.AccountMaintenanceEdit
	case p.productSetup != nil:
		return i18n.ProductAddAction
	case p.balanceTarget != nil && p.balanceTarget.draft != nil:
		return balanceTargetCopyFor(p.balanceTarget.draft.account).action
	case p.detail.record.open || p.detail.cashActions.active() ||
		p.liabilityPosting != nil && p.liabilityPosting.draft != nil ||
		p.cashBalance != nil && p.cashBalance.draft != nil ||

		p.investment != nil && p.investment.draft != nil:
		return i18n.LiabilityRecord
	default:
		return i18n.NoMessage
	}
}

func (p *accountsPage) updateAccountActionTriggers(gtx layout.Context) {
	if f := p.liabilityPosting; f != nil {
		if f.draft == nil && p.canPostLiability() && f.open.Clicked(gtx) {
			p.startLiabilityPosting()
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	if f := p.cashBalance; f != nil {
		if f.draft == nil && p.canSetCashBalance() && f.open.Clicked(gtx) {
			p.startCashBalance()
			gtx.Execute(op.InvalidateCmd{})
		}
		if f.saved.AccountID == p.target.account && f.saved.ActivityID != nil && f.openActivity != nil && f.view.Clicked(gtx) {
			f.openActivity(*f.saved.ActivityID, &f.view)
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	if f := p.balanceTarget; f != nil {
		if f.draft == nil && p.canUpdateBalanceTarget() && f.open.Clicked(gtx) {
			p.startBalanceTarget()
			gtx.Execute(op.InvalidateCmd{})
		}
		if f.saved.account == p.target.account && f.saved.activity != "" && f.openActivity != nil && f.view.Clicked(gtx) {
			f.openActivity(f.saved.activity, &f.view)
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	if f := p.investment; f != nil {
		if f.draft == nil {
			switch {
			case p.canTradeAccount() && f.buy.Clicked(gtx):
				p.startInvestment(domain.ActivityBuy)
				gtx.Execute(op.InvalidateCmd{})
			case p.canSell() && f.sell.Clicked(gtx):
				p.startInvestment(domain.ActivitySell)
				gtx.Execute(op.InvalidateCmd{})
			case p.canRecordDividend() && f.dividend.Clicked(gtx):
				p.startInvestment(domain.ActivityCashDividend)
				gtx.Execute(op.InvalidateCmd{})
			case p.canImportPosition() && f.importOpen.Clicked(gtx):
				p.startInvestment(domain.ActivityPositionTransfer)
				gtx.Execute(op.InvalidateCmd{})
			case p.canImportPosition() && f.custodyOpen.Clicked(gtx):
				f.startCustody(p.investmentRuntime(), positionScope{account: p.freshDetail().record.Account})
				gtx.Execute(op.InvalidateCmd{})
			case p.canImportPosition() && f.adjustmentOpen.Clicked(gtx):
				f.startAdjustment(p.investmentRuntime(), positionScope{account: p.freshDetail().record.Account})
				gtx.Execute(op.InvalidateCmd{})
			}
		}
	}
}

func (p *accountsPage) layoutAccountActionSheet(gtx layout.Context, view ui.View) {
	title := p.accountActionTitle()
	if title == i18n.NoMessage {
		return
	}
	cancel := p.accountActionCancel()
	if p.actionSheet == nil {
		p.actionSheet = &accountActionSheet{panel: newAnalysisSheet(), cancel: cancel}
		p.actionSheet.panel.width, p.actionSheet.panel.modal = 680, true
		// The form retains its existing initial field focus.
		p.actionSheet.panel.focus = false
	}
	sheet := p.actionSheet
	if sheet.cancel != cancel {
		sheet.cancel, sheet.panel.list.Position = cancel, layout.Position{}
	}
	view.Overlay = &sheet.overlay
	sheet.overlay.BeginFrame(gtx)
	canClose := !p.accountActionUnresolved()
	if sheet.overlay.Trigger == nil {
		dismiss := sheet.panel.dismissed(gtx)
		for {
			e, ok := gtx.Event(key.Filter{Name: key.NameEscape})
			if !ok {
				break
			}
			dismiss = dismiss || e.(key.Event).State == key.Press
		}
		if dismiss && canClose && cancel != nil {
			// Use the same cancel path as the visible form button. Each form
			// owns its pending-write guard and its initiating focus target.
			cancel.Click()
		}
	}
	var footer layout.Widget
	submitSettings := false
	if p.maintenance != nil && p.maintenance.draft != nil {
		footer = p.accountSettingsFooter(view, &submitSettings)
	}
	sheet.panel.layoutPanel(gtx, view, view.Language.Text(title), func(gtx layout.Context, v ui.View) layout.Dimensions {
		bounds := image.Rectangle{Min: v.Origin, Max: v.Origin.Add(gtx.Constraints.Max)}
		v = v.WithScrollAnchors(&sheet.panel.list)
		sheet.overlay.ScrollOutsidePanel(gtx, &sheet.panel.list, bounds)
		dims := v.List(&sheet.panel.list).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			return p.layoutAccountAction(gtx, v)
		})
		sheet.overlay.OffsetScrollAnchor(&sheet.panel.list, bounds)
		return dims
	}, footer, canClose)
	if submitSettings {
		m := p.maintenance
		input, issue := m.draft.input()
		m.issue = issue
		if issue == i18n.NoMessage {
			p.submitAccountMaintenance(nil, input)
		}
		gtx.Execute(op.InvalidateCmd{})
	}
	if !p.accountActionOpen() {
		p.actionSheet = nil
		gtx.Execute(op.InvalidateCmd{})
		return
	}
	sheet.overlay.Layout(gtx, view)
}

func (p *accountsPage) accountActionUnresolved() bool {
	return p.maintenance != nil && (p.maintenance.pending != nil || p.maintenance.review != nil) ||
		p.detail.cashActions.unresolved() || p.cashBalanceUnresolved() || p.balanceTargetUnresolved() ||
		p.liabilityPostingUnresolved() || p.investment.unresolved() || p.productSetup.unresolved()
}

func (p *accountsPage) accountActionCancel() *widget.Clickable {
	switch {
	case p.detail.record.open:
		return &p.detail.record.cancel
	case p.maintenance != nil && p.maintenance.draft != nil:
		return &p.maintenance.draft.cancel
	case p.detail.cashActions.active():
		return &p.detail.cashActions.form.draft.cancel
	case p.liabilityPosting != nil && p.liabilityPosting.draft != nil:
		return &p.liabilityPosting.draft.cancel
	case p.cashBalance != nil && p.cashBalance.draft != nil:
		return &p.cashBalance.draft.cancel
	case p.balanceTarget != nil && p.balanceTarget.draft != nil:
		return &p.balanceTarget.draft.cancel
	case p.investment != nil && p.investment.draft != nil:
		return &p.investment.draft.cancel
	case p.productSetup != nil:
		return &p.productSetup.cancel
	default:
		return nil
	}
}

func (p *accountsPage) layoutAccountAction(gtx layout.Context, view ui.View) layout.Dimensions {
	switch {
	case p.detail.record.open:
		return p.layoutAccountRecord(gtx, view)
	case p.maintenance != nil && p.maintenance.draft != nil:
		return p.layoutAccountSettings(gtx, view)
	case p.detail.cashActions.active():
		return p.layoutAccountCashMovement(gtx, view)
	case p.liabilityPosting != nil && p.liabilityPosting.draft != nil:
		return p.layoutLiabilityPosting(gtx, view)
	case p.cashBalance != nil && p.cashBalance.draft != nil:
		return p.layoutCashBalance(gtx, view)
	case p.balanceTarget != nil && p.balanceTarget.draft != nil:
		return p.layoutBalanceTarget(gtx, view)
	case p.investment != nil && p.investment.draft != nil:
		return p.investment.layoutForm(gtx, view, p.investmentRuntime())
	case p.productSetup != nil:
		d := p.productSetup
		if !d.poll() {
			return d.layout(gtx, view, p.productSetupCalendar(), p.cancelProductSetup)
		}
		p.cancelProductSetup()
		p.productSaved = true
		p.refresh()
	}
	return layout.Dimensions{}
}
