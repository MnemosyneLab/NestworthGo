package desktop

import (
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type accountCashActions struct {
	form               *historyCashMovements
	open, fx, transfer widget.Clickable
	deposit, withdraw  widget.Clickable
	restore, source    *widget.Clickable
}

func (a *accountCashActions) active() bool {
	return a.form != nil && a.form.draft != nil
}

func (a *accountCashActions) unresolved() bool {
	return a.active() && (a.form.writing() || a.form.draft.uncertain)
}

func (a *accountCashActions) leave() {
	if a.active() {
		a.form.discard()
	}
	a.source, a.restore = nil, nil
}

func (p *accountsPage) canMoveCash() bool {
	row := p.freshDetail()
	return p.detail.cashActions.form != nil && row != nil && history.CanUseCashMovement(row.record)
}

func (p *accountsPage) beginAccountCash(gtx layout.Context, move history.CashMovement, source *widget.Clickable) {
	a := &p.detail.cashActions
	a.source, a.restore = source, nil
	a.form.account = p.freshDetail().record.Account.ID
	a.form.start(move)
	gtx.Execute(op.InvalidateCmd{})
}

func (p *accountsPage) cashMovementButton(gtx layout.Context, view ui.View, button *widget.Clickable, label i18n.MessageID, variant ui.ButtonVariant) layout.Dimensions {
	a := &p.detail.cashActions
	if a.restore == button && gtx.Enabled() {
		a.restore = nil
		gtx.Execute(key.FocusCmd{Tag: button})
	}
	view.Focus.Watch(gtx, button)
	return view.ButtonOf(button, ui.ButtonSpec{Variant: variant, Size: ui.ButtonSmall, Title: view.Language.Text(label)})(gtx)
}

func (p *accountsPage) cashMovementActions(view ui.View) []layout.Widget {
	a := &p.detail.cashActions
	if !p.canMoveCash() {
		return nil
	}
	button := func(click *widget.Clickable, move history.CashMovement, label i18n.MessageID) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			if click.Clicked(gtx) {
				p.beginAccountCash(gtx, move, click)
			}
			return p.cashMovementButton(gtx, view, click, label, ui.ButtonOutline)
		}
	}
	actions := []layout.Widget{button(&a.open, history.CashContribution, i18n.AccountCashDepositOrWithdraw)}
	if p.freshDetail().record.Account.TrackingMode == domain.TrackingHoldings {
		actions = append(actions, button(&a.fx, history.CashConversion, i18n.AccountCashConvertAction))
	}
	return append(actions, button(&a.transfer, history.CashTransfer, i18n.AccountCashTransferAction))
}

func (p *accountsPage) layoutAccountCashMovement(gtx layout.Context, view ui.View) layout.Dimensions {
	a := &p.detail.cashActions

	rows := []ui.ColumnChild{}
	kind := a.form.draft.movement
	if a.form.draft.review == nil && (kind == history.CashContribution || kind == history.CashWithdrawal) {
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			if a.deposit.Clicked(gtx) && kind != history.CashContribution {
				a.form.discard()
				a.form.start(history.CashContribution)
				gtx.Execute(op.InvalidateCmd{})
			}
			if a.withdraw.Clicked(gtx) && kind != history.CashWithdrawal {
				a.form.discard()
				a.form.start(history.CashWithdrawal)
				gtx.Execute(op.InvalidateCmd{})
			}
			return view.TabTrack(gtx, 2, func(gtx layout.Context, i int) layout.Dimensions {
				if i == 0 {
					return view.TabSegment(&a.deposit, view.Language.Text(i18n.ContributionAdd), kind == history.CashContribution)(gtx)
				}
				return view.TabSegment(&a.withdraw, view.Language.Text(i18n.WithdrawalAdd), kind == history.CashWithdrawal)(gtx)
			})
		}))
	}
	rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
		dims := a.form.layoutForm(gtx, view)
		if !a.active() {
			a.restore, a.source = a.source, nil
			view.RestorePageScroll(0)
		}
		return dims
	}))
	return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...)
}
