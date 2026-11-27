package desktop

import (
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

func (d *fundsProductSetup) layout(gtx layout.Context, view ui.View, calendar productFormCalendar, cancel func()) layout.Dimensions {
	l := view.Language
	gtx, view = view.Measure(gtx, 720)
	if !d.unresolved() && d.cancel.Clicked(gtx) {
		cancel()
		view.RestorePageScroll(0)
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if d.form == nil {
		children := []layout.FlexChild{layout.Rigid(tradeTitle(view, i18n.ProductChooseAccount))}
		for _, choice := range d.layoutAccountChoices(gtx, view, func() { view.RestorePageScroll(0) }) {
			children = append(children, layout.Rigid(choice))
		}
		children = append(children, layout.Rigid(view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel))))
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
	}
	if d.plan != nil {
		return d.layoutReview(gtx, view)
	}
	if d.review.Clicked(gtx) {
		d.requestReview()
		gtx.Execute(op.InvalidateCmd{})
	}
	ready := d.preview == nil
	origin := view.Origin
	rows := d.form.fields(view, &origin, calendar, d.account.Name, &d.currency, d.cashValue(view), ready,
		func(gtx layout.Context, field *ui.TextField) {
			if d.focus {
				field.Focus(gtx)
				d.focus = false
			}
		}, func() { d.problem = i18n.NoMessage })
	if d.problem != i18n.NoMessage {
		rows = append(rows, view.Message(l.Text(d.problem), view.Colors.ErrorInk))
	}
	if !ready {
		rows = append(rows, view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk))
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
			layout.Rigid(view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel))),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !ready {
					gtx = gtx.Disabled()
				}
				return view.PrimaryButton(&d.review, l.Text(i18n.HoldingReconReview))(gtx)
			}))
	})
	children := make([]layout.FlexChild, 0, len(rows))
	for _, row := range rows {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, row)
		}))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}

func (d *fundsProductSetup) layoutReview(gtx layout.Context, view ui.View) layout.Dimensions {
	if !d.unresolved() && d.edit.Clicked(gtx) {
		d.plan, d.focus = nil, true
		view.RestorePageScroll(0)
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if d.confirm.Clicked(gtx) {
		d.confirmReview()
		gtx.Execute(op.InvalidateCmd{})
	}
	if d.focus {
		if !d.uncertain {
			view.RestorePageScroll(0)
		}
		gtx.Execute(key.FocusCmd{Tag: &d.confirm})
		d.focus = false
	}
	rows := append([]layout.Widget{tradeTitle(view, i18n.HoldingReconReview)}, d.reviewRows(view)...)
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return layoutTargetBalanceConfirmation(gtx, view, d.post != nil, d.uncertain, &d.cancel, &d.edit, &d.confirm, i18n.HoldingReconConfirm)
	})
	children := make([]layout.FlexChild, 0, len(rows))
	for _, row := range rows {
		children = append(children, layout.Rigid(row))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}

func (d *fundsProductSetup) layoutAccountChoices(gtx layout.Context, view ui.View, advance func()) []layout.Widget {
	choices := make([]layout.Widget, 0, len(d.choices))
	for _, account := range d.choices {
		button := portfolioButton(d.accountButtons, account.ID)
		if button.Clicked(gtx) {
			d.chooseAccount(account, view.Language.Text(i18n.ProductAddDeposit))
			advance()
			gtx.Execute(op.InvalidateCmd{})
			break
		}
		if d.focus {
			gtx.Execute(key.FocusCmd{Tag: button})
			d.focus = false
		}
		choices = append(choices, view.Button(button, account.Name))
	}
	return choices
}

func (d *fundsProductSetup) reviewRows(view ui.View) []layout.Widget {
	review := liquidity.FromProductOperationPreview(d.plan.review)
	rows := []layout.Widget{view.Message(d.account.Name, view.Colors.Ink)}
	rows = append(rows, holdingProductAddFacts(view, []byte(review.NormalizedJSON))...)
	rows = append(rows, productReviewProjection(view, &review)...)
	if d.uncertain {
		rows = append(rows, view.Message(view.Language.Text(i18n.ContributionUncertain), view.Colors.ErrorInk))
	}
	return rows
}
