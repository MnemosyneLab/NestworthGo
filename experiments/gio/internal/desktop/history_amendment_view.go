package desktop

import (
	"fmt"
	"strings"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *historyPage) layoutAmendment(gtx layout.Context, view ui.View) layout.Dimensions {
	r, l := p.reads, view.Language
	d := r.detail
	form := d.amendment
	fix := form.action == history.CorrectActivity
	busy := form.preview != nil || form.commit != nil
	actionTitle, prepareTitle, reviewTitle, confirmTitle := i18n.HistoryReverseAction, i18n.HistoryReversePreview, i18n.HistoryReverseReview, i18n.HistoryReverseConfirm
	focus := &d.reverse
	if fix {
		actionTitle, prepareTitle, reviewTitle, confirmTitle = i18n.HistoryCorrectAction, i18n.HistoryCorrectPreview, i18n.HistoryCorrectReview, i18n.HistoryCorrectConfirm
		if form.custody != nil {
			actionTitle = i18n.CustodyAmendmentCorrect
		}
		focus = &d.correct
	}
	if form.commit == nil && !form.uncertain && form.cancel.Clicked(gtx) {
		if d.fromRowAction {
			r.back()
			gtx.Execute(op.InvalidateCmd{})
			return layout.Dimensions{}
		}
		d.amendment, d.amendmentFocus = nil, focus
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if form.review != nil && fix && !busy && !form.uncertain && form.edit.Clicked(gtx) {
		form.review, form.focus = nil, true
		form.scroll.Position = layout.Position{}
		gtx.Execute(op.InvalidateCmd{})
	}
	if !busy && form.review == nil && form.prepare.Clicked(gtx) {
		r.previewAmendment()
		gtx.Execute(op.InvalidateCmd{})
	}
	if !busy && form.review != nil && form.confirm.Clicked(gtx) {
		r.commitAmendment()
		gtx.Execute(op.InvalidateCmd{})
	}
	busy = form.preview != nil || form.commit != nil

	if form.custody != nil {
		return p.layoutCustodyAmendment(gtx, view, fix, busy, actionTitle, prepareTitle, reviewTitle, confirmTitle)
	}
	movement := form.movement
	accountName := func(id *domain.AccountID) string { return d.record.Accounts[*id] }
	title := actionTitle
	if form.review != nil {
		title = reviewTitle
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.Label(gtx, l.Text(title), 19, font.Medium, view.Colors.Ink)
		}),
	}
	if movement.Paired() {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.HistoryOriginalMovement), view.Colors.SecondaryInk)))
		children = append(children, pairedAmendmentEffects(view, d.record.Activity.Effects, accountName, false)...)
	} else {
		children = append(children, layout.Rigid(view.Message(accountName(movement.Principal.AccountID), view.Colors.Ink)),
			layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.HistoryOriginalAmount), historyDisplayEffectMoney(view, movement.Principal)), view.Colors.Ink)))
	}
	if fix {
		effective := contributionTimestamp(view, d.record.Activity.EffectiveAt, d.record.Location)
		children = append(children,
			layout.Rigid(view.Message(effective, view.Colors.SecondaryInk)),
			layout.Rigid(view.Message(l.Text(i18n.HistoryCorrectDescription), view.Colors.SecondaryInk)),
		)
	} else {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.HistoryReverseDescription), view.Colors.SecondaryInk)))
	}
	if review := form.review; review != nil {
		if !fix {
			effective := contributionTimestamp(view, review.Preview.Activity.EffectiveAt, d.record.Location)
			children = append(children, layout.Rigid(view.Message(effective, view.Colors.SecondaryInk)))
		}
		if movement.Paired() {
			label := i18n.HistoryReversalMovement
			if fix {
				label = i18n.HistoryReplacementMovement
			}
			children = append(children, layout.Rigid(view.Message(l.Text(label), view.Colors.SecondaryInk)))
			children = append(children, pairedAmendmentEffects(view, review.Preview.Effects, accountName, !fix)...)
			if fix {
				from, to := transferCurrencies(review.Preview.Effects)
				if rate := transferRateLabel(view, review.Preview.DerivedRate, from, to); rate != "" {
					children = append(children, layout.Rigid(view.Message(rate, view.Colors.Ink)))
				}
			}
		} else {
			label := i18n.HistoryReversalAmount
			if fix {
				label = i18n.HistoryReplacementAmount
			}
			for _, effect := range review.Preview.Effects {
				children = append(children, layout.Rigid(view.Message(fmt.Sprintf(l.Text(label), historyDisplayEffectMoney(view, effect)), view.Colors.Ink)))
			}
		}
		if fix && len(review.Preview.Review) > 0 {
			for _, endpoint := range review.Preview.Review {
				if endpoint.Currency == "" {
					continue
				}
				if movement.Paired() {
					children = append(children, layout.Rigid(view.Message(endpoint.AccountName, view.Colors.Ink)))
				}
				children = append(children,
					layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.ContributionBefore), historyDisplayMoney(view, endpoint.Before, endpoint.Currency)), view.Colors.SecondaryInk)),
					layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.HistoryChangeDelta), historyDisplayMoney(view, endpoint.Delta, endpoint.Currency)), view.Colors.Ink)),
					layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.HistoryBalanceAfterCorrection), historyDisplayMoney(view, endpoint.After, endpoint.Currency)), view.Colors.Ink)),
				)
			}
		} else {
			for _, endpoint := range review.Preview.Resulting {
				label := i18n.HistoryBalanceAfterReversal
				if fix {
					label = i18n.HistoryBalanceAfterCorrection
				}
				balance := fmt.Sprintf(l.Text(label), historyDisplayMoney(view, endpoint.Amount, endpoint.Currency))
				if movement.Paired() {
					balance = endpoint.Name + " · " + balance
				}
				children = append(children, layout.Rigid(view.Message(balance, view.Colors.Ink)))
			}
		}
	} else if fix {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if form.focus && !busy {
				form.amount.Focus(gtx)
				form.focus = false
			}
			label := i18n.HistoryNewAmount
			if movement.Paired() {
				label = i18n.TransferSentAmount
			}
			return form.amount.LayoutCurrency(gtx, view, ui.FieldOptions{Label: label, Disabled: busy}, movement.Principal.Money.Currency().String())
		}))
		if movement.CrossCurrency() {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return form.received.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.TransferReceivedAmount, Disabled: busy}, movement.Received.Money.Currency().String())
			}))
		}
		if movement.Paired() {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return form.fee.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.HistoryAmendmentFee, Disabled: busy}, movement.Principal.Money.Currency().String())
			}))
			if movement.Movement == history.CashTransfer {
				children = append(children, layout.Rigid(view.Message(l.Text(i18n.HistoryTransferCorrectionFee), view.Colors.SecondaryInk)))
			}
		}
	}

	if form.uncertain {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.ContributionUncertain), view.Colors.ErrorInk)))
	} else if form.problem != i18n.NoMessage {
		children = append(children, layout.Rigid(view.Message(l.Text(form.problem), view.Colors.ErrorInk)))
	}
	actions := amendmentActions(view, form, fix, prepareTitle, confirmTitle)
	gtx, view = view.Measure(gtx, 640)
	if !movement.Paired() {
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, append(children, layout.Rigid(actions))...)
	}
	return ui.Column{Gap: 8}.Layout(gtx,
		ui.Fill(func(gtx layout.Context) layout.Dimensions {
			return view.List(&form.scroll).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
			})
		}), ui.Rigid(actions),
	)
}

func amendmentActions(view ui.View, form *historyAmendmentDraft, fix bool, prepareTitle, confirmTitle i18n.MessageID) layout.Widget {
	l := view.Language
	return func(gtx layout.Context) layout.Dimensions {
		if form.reviewFocus && form.commit == nil && form.review != nil {
			gtx.Execute(key.FocusCmd{Tag: &form.confirm})
			form.reviewFocus = false
		}
		if form.commit != nil {
			return view.Message(l.Text(i18n.SettingsPending), view.Colors.SecondaryInk)(gtx)
		}
		if form.uncertain {
			return view.PrimaryButton(&form.confirm, l.Text(i18n.ContributionRetryPost))(gtx)
		}
		buttons := []layout.Widget{view.Button(&form.cancel, l.Text(i18n.InstitutionsCancel))}
		switch {
		case form.preview != nil:
			buttons = append(buttons, view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk))
		case form.review == nil:
			buttons = append(buttons, func(gtx layout.Context) layout.Dimensions {
				if form.custody != nil && fix && (!form.custody.selected() || strings.TrimSpace(form.amount.Text()) == "") {
					gtx = gtx.Disabled()
				}
				return view.PrimaryButton(&form.prepare, l.Text(prepareTitle))(gtx)
			})
		default:
			if fix {
				label := i18n.HistoryAmendmentEdit
				if form.custody != nil {
					label = i18n.AccountCreateBackToDetails
				}
				buttons = append(buttons, view.Button(&form.edit, l.Text(label)))
			}
			buttons = append(buttons, view.PrimaryButton(&form.confirm, l.Text(confirmTitle)))
		}
		return (ui.Wrap{Gap: 8}).Layout(gtx, len(buttons), func(gtx layout.Context, i int) layout.Dimensions {
			return buttons[i](gtx)
		})
	}
}

func pairedAmendmentEffects(view ui.View, effects []domain.ActivityEffect, account func(*domain.AccountID) string, reverse bool) []layout.FlexChild {
	l := view.Language
	var rows []layout.FlexChild
	for _, effect := range effects {
		caption := effectCaption(view, effect, true, !reverse, false, 0, account, nil)
		if reverse && effect.Role == domain.EffectRoleFee {
			caption += " · " + fmt.Sprintf(l.Text(i18n.HistoryReversedEffect), l.Text(i18n.HistoryFee))
		}
		rows = append(rows, layout.Rigid(view.Message(caption, view.Colors.Ink)))
	}
	return rows
}

func (r *historyReads) amendmentLinks(view ui.View) layout.Widget {
	d, l := r.detail, view.Language
	links := d.record.Links
	pairs := [...]struct {
		id    domain.ActivityID
		label i18n.MessageID
	}{
		{links.Original, i18n.HistoryViewOriginal},
		{links.CorrectionReversal, i18n.HistoryViewCorrectionReversal},
		{links.Reversal, i18n.HistoryViewReversal},
		{links.Replacement, i18n.HistoryViewReplacement},
	}
	return func(gtx layout.Context) layout.Dimensions {
		var buttons []layout.FlexChild
		for i, link := range pairs {
			if link.id == "" {
				continue
			}
			button := &d.linked[i]
			buttons = append(buttons, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if button.Clicked(gtx) {
					r.open(link.id, button)
					gtx.Execute(op.InvalidateCmd{})
				}
				view.Focus.Watch(gtx, button)
				return view.Button(button, l.Text(link.label))(gtx)
			}))
		}
		if len(buttons) == 0 {
			return layout.Dimensions{}
		}
		return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx, buttons...)
	}
}
