package desktop

import (
	"fmt"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *availableFundsPage) sourceButtons(key string) *fundsLinks {
	buttons := p.links[key]
	if buttons == nil {
		buttons = new(fundsLinks)
		p.links[key] = buttons
	}
	return buttons
}

func (p *availableFundsPage) reservationButton(view ui.View, name string, source domain.LiquiditySourceRef) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		button := &p.sourceButtons(source.Key()).reserve
		if button.Clicked(gtx) {
			p.openReservations(source, name, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		label := fmt.Sprintf(view.Language.Text(i18n.ReserveManage), name)
		return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			view.Focus.Watch(gtx, button)
			return view.Button(button, label)(gtx)
		})
	}
}

func (p *availableFundsPage) layoutReservations(gtx layout.Context, view ui.View) layout.Dimensions {
	r, l := p.reservation, view.Language
	gtx = ui.LimitWidth(gtx, 690)
	if r.close.Clicked(gtx) && !p.reservationUnresolved() {
		if r.plan != nil {
			r.plan = nil
		} else if r.editing {
			r.editing, r.notice = false, i18n.NoMessage
		} else {
			p.reservation = nil
			gtx.Execute(key.FocusCmd{Tag: r.entry})
			gtx.Execute(op.InvalidateCmd{})
			return layout.Dimensions{}
		}
		gtx.Execute(op.InvalidateCmd{})
	}
	if r.readError && r.retry.Clicked(gtx) {
		p.readReservations()
		gtx.Execute(op.InvalidateCmd{})
	}
	title := i18n.ReserveTitle
	if r.editing {
		title = i18n.ReserveAdd
	}
	if r.plan != nil {
		title = i18n.ReserveReview
		if r.plan.release != "" {
			title = i18n.ReserveRelease
		}
	}
	source := r.name
	if r.current != nil && r.current.currency != "" {
		source += " · " + r.current.currency.String()
	}
	header := []layout.FlexChild{
		layout.Rigid(tradeTitle(view, title)),
		layout.Rigid(view.Message(source, view.Colors.Ink)),
	}
	if r.notice != i18n.NoMessage {
		color := view.Colors.ErrorInk
		if r.notice == i18n.ReserveSaved || r.notice == i18n.ReserveReleasedNotice {
			color = view.Colors.Ink
		}
		header = append(header, layout.Rigid(view.Message(l.Text(r.notice), color)))
	}
	switch {
	case r.write != nil:
		header = append(header, layout.Rigid(view.Message(l.Text(i18n.SettingsPending), view.Colors.SecondaryInk)))
	case r.read != nil:
		header = append(header, layout.Rigid(view.Message(l.Text(i18n.FundsLoading), view.Colors.SecondaryInk)))
	case r.readError:
		message := i18n.FundsReadFailed
		if r.checking {
			message = i18n.ReserveVerifyFailed
		}
		header = append(header, layout.Rigid(view.Message(l.Text(message), view.Colors.ErrorInk)),
			layout.Rigid(view.Button(&r.retry, l.Text(i18n.HistoryRetry))))
	case r.plan != nil:
		header = append(header, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.layoutReservationReview(gtx, view)
		}))
	case r.editing:
		header = append(header, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.layoutReservationDraft(gtx, view)
		}))
	default:
		header = append(header,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.W.Layout(gtx, view.Button(&r.close, l.Text(i18n.ReserveBack)))
			}),
			layout.Rigid(fundsAmount(view, i18n.FundsAvailableAfter, r.current.day.UnreservedNative)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.reservationListActions(gtx, view) }),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return r.layoutReservationList(gtx, view) }))
	}
	if r.readError && !r.checking {
		header = append(header, layout.Rigid(view.Button(&r.close, l.Text(i18n.ReserveBack))))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, header...)
}

func (p *availableFundsPage) reservationListActions(gtx layout.Context, view ui.View) layout.Dimensions {
	r, l := p.reservation, view.Language
	if !r.current.eligible {
		return view.Message(l.Text(i18n.ReserveSourceUnavailable), view.Colors.SecondaryInk)(gtx)
	}
	if r.add.Clicked(gtx) {
		r.editing, r.notice = true, i18n.NoMessage
		r.label.Editor.SetText("")
		r.amount.Editor.SetText("")
		r.label.Focus(gtx)
		gtx.Execute(op.InvalidateCmd{})
	}
	return layout.W.Layout(gtx, view.PrimaryButton(&r.add, l.Text(i18n.ReserveAdd)))
}

func (r *fundsReservationSession) layoutReservationList(gtx layout.Context, view ui.View) layout.Dimensions {
	if len(r.current.records) == 0 {
		return view.Message(view.Language.Text(i18n.ReserveEmpty), view.Colors.SecondaryInk)(gtx)
	}
	r.list.Position = restoredPosition(r.list.Position, len(r.current.records))
	return material.List(view.Theme, &r.list).Layout(gtx, len(r.current.records), func(gtx layout.Context, i int) layout.Dimensions {
		row := r.current.records[i]
		l := view.Language
		title := row.Label
		fields := []layout.Widget{fundsAmount(view, i18n.FundsRequested, &row.Amount)}
		if !row.Active() {
			title = fmt.Sprintf(l.Text(i18n.ReserveReleasedRow), row.Label)
		} else if r.current.eligible {
			fields = append(fields, r.releaseReservationAction(view, row))
		}
		return layout.Inset{Bottom: 8}.Layout(gtx, fundsCard(view, title, fields...))
	})
}

func (r *fundsReservationSession) releaseReservationAction(view ui.View, row domain.LiquidityReservation) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		button := r.release[row.ID]
		if button == nil {
			button = new(widget.Clickable)
			r.release[row.ID] = button
		}
		if button.Clicked(gtx) {
			r.plan = &fundsReservationPlan{source: r.source, version: r.current.version, release: row.ID,
				revision: row.Revision, label: row.Label, amount: row.Amount}
			r.notice = i18n.NoMessage
			gtx.Execute(op.InvalidateCmd{})
		}
		text := fmt.Sprintf(view.Language.Text(i18n.ReserveReleaseRow), row.Label)
		return layout.W.Layout(gtx, view.Button(button, text))
	}
}

func (r *fundsReservationSession) layoutReservationDraft(gtx layout.Context, view ui.View) layout.Dimensions {
	l := view.Language
	if !r.current.eligible {
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
			layout.Rigid(view.Message(l.Text(i18n.ReserveSourceUnavailable), view.Colors.ErrorInk)),
			layout.Rigid(view.Button(&r.close, l.Text(i18n.InstitutionsCancel))))
	}
	if r.review.Clicked(gtx) {
		r.reviewNew()
		gtx.Execute(op.InvalidateCmd{})
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(fundsAmount(view, i18n.ReserveCurrentAvailability, r.current.day.UnreservedNative)),
		layout.Rigid(view.Message(l.Text(i18n.ReserveHelp), view.Colors.SecondaryInk)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.label.Layout(gtx, view, ui.FieldOptions{Label: i18n.ReserveLabel})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.amount.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.ReserveAmount}, r.current.currency.String())
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(view.Button(&r.close, l.Text(i18n.InstitutionsCancel))),
				layout.Rigid(view.PrimaryButton(&r.review, l.Text(i18n.ReserveReview))))
		}))
}

func (p *availableFundsPage) layoutReservationReview(gtx layout.Context, view ui.View) layout.Dimensions {
	r, l := p.reservation, view.Language
	confirm := i18n.ReserveConfirm
	if r.plan.release != "" {
		confirm = i18n.ReserveConfirmRelease
	}
	if r.confirm.Clicked(gtx) {
		p.confirmReservation()
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(fundsCard(view, r.plan.label,
			fundsAmount(view, i18n.FundsRequested, &r.plan.amount),
			fundsAmount(view, i18n.ReserveCurrentAvailability, r.current.day.UnreservedNative))),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			cancel := i18n.InstitutionsCancel
			if r.editing {
				cancel = i18n.AccountCreateBackToDetails
			}
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
					layout.Rigid(view.Button(&r.close, l.Text(cancel))),
					layout.Rigid(view.PrimaryButton(&r.confirm, l.Text(confirm))))
			})
		}))
}
