package desktop

import (
	"fmt"

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
	fix := form.action == history.CorrectCash
	busy := form.preview != nil || form.commit != nil
	actionTitle, prepareTitle, reviewTitle, confirmTitle := i18n.HistoryReverseAction, i18n.HistoryReversePreview, i18n.HistoryReverseReview, i18n.HistoryReverseConfirm
	focus := &d.reverse
	if fix {
		actionTitle, prepareTitle, reviewTitle, confirmTitle = i18n.HistoryCorrectAction, i18n.HistoryCorrectPreview, i18n.HistoryCorrectReview, i18n.HistoryCorrectConfirm
		focus = &d.correct
	}
	if form.commit == nil && !form.uncertain && form.cancel.Clicked(gtx) {
		d.amendment = nil
		gtx.Execute(key.FocusCmd{Tag: focus})
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if form.review != nil && fix && !busy && !form.uncertain && form.edit.Clicked(gtx) {
		form.review, form.focus = nil, true
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

	original, _ := history.CashAmendmentEffect(d.record.Activity)
	amount := movementMoney(original)
	title := actionTitle
	if form.review != nil {
		title = reviewTitle
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.Label(gtx, l.Text(title), 19, font.Medium, view.Colors.Ink)
		}),
		layout.Rigid(view.Message(d.record.Accounts[*original.AccountID], view.Colors.Ink)),
		layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.HistoryOriginalAmount), amount), view.Colors.Ink)),
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
		for _, effect := range review.Preview.Effects {
			label := i18n.HistoryReversalAmount
			if fix {
				label = i18n.HistoryReplacementAmount
			}
			children = append(children, layout.Rigid(view.Message(fmt.Sprintf(l.Text(label), movementMoney(effect)), view.Colors.Ink)))
		}
		for _, endpoint := range review.Preview.Resulting {
			label := i18n.HistoryBalanceAfterReversal
			if fix {
				label = i18n.HistoryBalanceAfterCorrection
			}
			children = append(children, layout.Rigid(view.Message(fmt.Sprintf(l.Text(label), endpoint.Amount+" "+endpoint.Currency.String()), view.Colors.Ink)))
		}
	} else if fix {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if form.focus && !busy {
				form.amount.Focus(gtx)
				form.focus = false
			}
			return form.amount.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.HistoryNewAmount, Disabled: busy}, form.currency.String())
		}))
	}

	if form.uncertain {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.ContributionUncertain), view.Colors.ErrorInk)))
	} else if form.problem != i18n.NoMessage {
		children = append(children, layout.Rigid(view.Message(l.Text(form.problem), view.Colors.ErrorInk)))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if form.commit != nil {
			return view.Message(l.Text(i18n.SettingsPending), view.Colors.SecondaryInk)(gtx)
		}
		if form.uncertain {
			return view.PrimaryButton(&form.confirm, l.Text(i18n.ContributionRetryPost))(gtx)
		}
		buttons := []layout.FlexChild{layout.Rigid(view.Button(&form.cancel, l.Text(i18n.InstitutionsCancel)))}
		switch {
		case form.preview != nil:
			buttons = append(buttons, layout.Rigid(view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)))
		case form.review == nil:
			buttons = append(buttons, layout.Rigid(view.PrimaryButton(&form.prepare, l.Text(prepareTitle))))
		default:
			if fix {
				buttons = append(buttons, layout.Rigid(view.Button(&form.edit, l.Text(i18n.HistoryAmendmentEdit))))
			}
			buttons = append(buttons, layout.Rigid(view.PrimaryButton(&form.confirm, l.Text(confirmTitle))))
		}
		return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx, buttons...)
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(ui.LimitWidth(gtx, 640), children...)
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
