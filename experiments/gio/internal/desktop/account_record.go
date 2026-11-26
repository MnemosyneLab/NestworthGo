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
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

type accountRecordControls struct {
	trigger, cancel widget.Clickable
	choices         map[i18n.MessageID]*widget.Clickable
	open, restore   bool
	focus           bool
}

type accountRecordChoice struct {
	group, label i18n.MessageID
	start        func(layout.Context)
}

func (p *accountsPage) accountRecordChoices(view ui.View, account domain.Account) []accountRecordChoice {
	entry := &p.detail.record.trigger
	var choices []accountRecordChoice
	if p.canTradeAccount() {
		invest := func(kind domain.ActivityKind, label i18n.MessageID) {
			choices = append(choices, accountRecordChoice{i18n.HistoryEntryInvestments, label, func(layout.Context) {
				p.startInvestment(kind)
				p.investment.draft.originAction = entry
			}})
		}
		invest(domain.ActivityBuy, i18n.AccountRecordBuy)
		if p.canSell() {
			invest(domain.ActivitySell, i18n.AccountRecordSell)
		}
		if p.canRecordDividend() {
			invest(domain.ActivityCashDividend, i18n.DividendOpen)
		}
		if p.canImportPosition() {
			invest(domain.ActivityPositionTransfer, i18n.OpeningPositionOpen)
			position := func(label i18n.MessageID, start func(investmentRuntime, positionScope)) {
				choices = append(choices, accountRecordChoice{i18n.HistoryEntryInvestments, label, func(layout.Context) {
					start(p.investmentRuntime(), positionScope{account: account})
					p.investment.draft.originAction = entry
				}})
			}
			position(i18n.HistoryPositionTransfer, p.investment.startCustody)
			position(i18n.AccountRecordPositionAdjustment, p.investment.startAdjustment)
		}
	}
	if p.canMoveCash() {
		cash := func(kind history.CashMovement, label i18n.MessageID) {
			choices = append(choices, accountRecordChoice{i18n.HistoryEntryCash, label, func(gtx layout.Context) {
				p.beginAccountCash(gtx, view, kind, entry)
			}})
		}
		cash(history.CashContribution, i18n.AccountRecordAddMoney)
		cash(history.CashWithdrawal, i18n.AccountRecordRemoveMoney)
		cash(history.CashTransfer, i18n.AccountCashTransferAction)
		if account.TrackingMode == domain.TrackingHoldings {
			cash(history.CashConversion, i18n.AccountCashConvertAction)
		}
	}
	if p.canPostLiability() {
		debt := func(kind historyapi.ChangeCommandKind, label i18n.MessageID) {
			choices = append(choices, accountRecordChoice{i18n.HistoryEntryDebt, label, func(layout.Context) {
				p.startLiabilityPosting()
				p.liabilityPosting.entry = entry
				p.liabilityPosting.draft.selectKind(kind)
			}})
		}
		if account.IsLiability() {
			increase, decrease := i18n.LiabilityDebtIncrease, i18n.LiabilityDebtDecrease
			if account.AccountType == domain.TypeCreditCard {
				increase, decrease = i18n.LiabilityCardPurchase, i18n.LiabilityCardRefund
			}
			debt(historyapi.ChangeMoneyAdded, increase)
			debt(historyapi.ChangeMoneyRemoved, decrease)
			debt(historyapi.ChangeValueUpdate, i18n.AccountRecordDebtValue)
		}
		debt(historyapi.ChangeDebtDraw, i18n.HistoryDebtDraw)
		debt(historyapi.ChangeDebtPayment, i18n.HistoryDebtPayment)
	}
	if !account.IsLiability() && p.canUpdateBalanceTarget() {
		copy := balanceTargetCopyFor(account)
		choices = append(choices, accountRecordChoice{i18n.AccountRecordValue, copy.action, func(layout.Context) {
			p.startBalanceTarget()
			p.balanceTarget.entry = entry
		}})
	}
	return choices
}

func (p *accountsPage) detailRecordButton(view ui.View, account domain.Account) layout.Widget {
	if len(p.accountRecordChoices(view, account)) == 0 {
		return nil
	}
	d := &p.detail.record
	return func(gtx layout.Context) layout.Dimensions {
		if d.trigger.Clicked(gtx) {
			d.open, d.focus = true, true
			view.RestorePageScroll(0)
			gtx.Execute(op.InvalidateCmd{})
		}
		cash := &p.detail.cashActions
		trade := p.investment
		tradeRestore := trade != nil && trade.restore == &d.trigger
		if gtx.Enabled() && (d.restore || tradeRestore || cash.restore == &d.trigger) {
			if tradeRestore {
				trade.restore = nil
			}
			d.restore, cash.restore = false, nil
			gtx.Execute(key.FocusCmd{Tag: &d.trigger})
		}
		view.Focus.Watch(gtx, &d.trigger)
		return view.ButtonOf(&d.trigger, ui.ButtonSpec{Variant: ui.ButtonPrimary, Size: ui.ButtonSmall, Title: view.Language.Text(i18n.LiabilityRecord)})(gtx)
	}
}

func (p *accountsPage) restoreAccountRecord(entry *widget.Clickable) {
	if entry == &p.detail.record.trigger {
		p.detail.record.restore = true
	}
}

func (p *accountsPage) layoutAccountRecord(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := &p.detail.record, view.Language
	closed := d.cancel.Clicked(gtx)
	for {
		event, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		closed = closed || event.(key.Event).State == key.Press
	}
	if closed {
		d.open, d.restore = false, true
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	rows := []ui.ColumnChild{
		ui.Rigid(view.Body(ui.TextHeading, l.Text(i18n.LiabilityRecord), view.Colors.Ink)),
		ui.Rigid(view.Body(ui.TextLabel, p.name(), view.Colors.Ink)),
	}
	if row := p.freshDetail(); row != nil {
		choices := p.accountRecordChoices(view, row.record.Account)
		if d.choices == nil {
			d.choices = make(map[i18n.MessageID]*widget.Clickable)
		}
		for i := 0; i < len(choices); {
			group := choices[i].group
			var actions []layout.Widget
			for i < len(choices) && choices[i].group == group {
				choice := choices[i]
				button := d.choices[choice.label]
				if button == nil {
					button = new(widget.Clickable)
					d.choices[choice.label] = button
				}
				first := i == 0
				actions = append(actions, func(gtx layout.Context) layout.Dimensions {
					if first && d.focus && gtx.Enabled() {
						d.focus = false
						gtx.Execute(key.FocusCmd{Tag: button})
					}
					return accountDetailButton(view, button, choice.label, ui.ButtonOutline, func() {
						d.open = false
						choice.start(gtx)
					})(gtx)
				})
				i++
			}
			rows = append(rows, ui.Rigid(view.Body(ui.TextLabel, l.Text(group), view.Colors.SecondaryInk)),
				ui.Rigid(accountDetailActions(view, actions)))
		}
	}
	rows = append(rows, ui.Rigid(view.ButtonOf(&d.cancel, ui.ButtonSpec{Variant: ui.ButtonGhost, Title: l.Text(i18n.InstitutionsCancel)})))
	return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...)
}
