package desktop

import (
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

func (p *availableFundsPage) layoutProductSetup(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.setup, view.Language
	gtx, view = view.Measure(gtx, 720)
	if !p.productSetupUnresolved() && d.cancel.Clicked(gtx) {
		p.setup, p.restoreAddFocus = nil, true
		view.RestorePageScroll(0)
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if d.form == nil {
		children := []layout.FlexChild{layout.Rigid(tradeTitle(view, i18n.ProductChooseAccount))}
		for _, account := range d.choices {
			button := d.accountButtons[account.ID]
			if button == nil {
				button = new(widget.Clickable)
				d.accountButtons[account.ID] = button
			}
			if button.Clicked(gtx) {
				p.chooseProductAccount(account, l.Text(i18n.ProductAddDeposit))
				view.RestorePageScroll(0)
				gtx.Execute(op.InvalidateCmd{})
				break
			}
			if d.focus {
				gtx.Execute(key.FocusCmd{Tag: button})
				d.focus = false
			}
			children = append(children, layout.Rigid(view.Button(button, account.Name)))
		}
		children = append(children, layout.Rigid(view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel))))
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
	}
	if d.plan != nil {
		return p.layoutProductSetupReview(gtx, view)
	}
	if d.review.Clicked(gtx) {
		p.reviewProductSetup()
		gtx.Execute(op.InvalidateCmd{})
	}
	ready := d.preview == nil
	origin := view.Origin
	var calendar productFormCalendar
	if p.current != nil {
		overview := p.current.report.Overview
		calendar = productFormCalendar{today: overview.LocalDate, zone: overview.Timezone}
	}
	rows := []layout.Widget{tradeTitle(view, i18n.ProductAddAction)}
	rows = append(rows, d.form.fields(view, &origin, calendar, d.account.Name, &d.currency, d.cashValue(view), ready,
		func(gtx layout.Context, field *ui.TextField) {
			if d.focus {
				field.Focus(gtx)
				d.focus = false
			}
		}, func() { d.problem = i18n.NoMessage })...)
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

func (p *availableFundsPage) layoutProductSetupReview(gtx layout.Context, view ui.View) layout.Dimensions {
	d := p.setup
	if !p.productSetupUnresolved() && d.edit.Clicked(gtx) {
		d.plan, d.focus = nil, true
		view.RestorePageScroll(0)
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if d.confirm.Clicked(gtx) {
		p.postProductSetup()
		gtx.Execute(op.InvalidateCmd{})
	}
	if d.focus {
		if !d.uncertain {
			view.RestorePageScroll(0)
		}
		gtx.Execute(key.FocusCmd{Tag: &d.confirm})
		d.focus = false
	}
	review := liquidity.FromProductOperationPreview(d.plan.review)
	rows := []layout.Widget{tradeTitle(view, i18n.HoldingReconReview), view.Message(d.account.Name, view.Colors.Ink)}
	rows = append(rows, holdingProductAddFacts(view, []byte(review.NormalizedJSON))...)
	rows = append(rows, productReviewProjection(view, &review)...)
	if d.uncertain {
		rows = append(rows, view.Message(view.Language.Text(i18n.ContributionUncertain), view.Colors.ErrorInk))
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return layoutTargetBalanceConfirmation(gtx, view, d.post != nil, d.uncertain, &d.cancel, &d.edit, &d.confirm, i18n.HoldingReconConfirm)
	})
	children := make([]layout.FlexChild, 0, len(rows))
	for _, row := range rows {
		children = append(children, layout.Rigid(row))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}
