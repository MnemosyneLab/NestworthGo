package desktop

import (
	"image"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

func (s *shell) fundsReservationOpen() bool {
	return s.activeRoute() == fundsRoute && s.funds != nil && s.funds.reservation != nil
}

func (p *availableFundsPage) layoutReservationSheet(gtx layout.Context, view ui.View) {
	r := p.reservation
	view.Overlay = &r.overlay
	r.overlay.BeginFrame(gtx)

	canClose := !p.reservationUnresolved()
	if r.overlay.Trigger == nil {
		dismiss := r.sheet.dismissed(gtx)
		for {
			e, ok := gtx.Event(key.Filter{Name: key.NameEscape})
			if !ok {
				break
			}
			dismiss = dismiss || e.(key.Event).State == key.Press
		}
		if dismiss && canClose {
			p.reservation, p.reservationFocus = nil, r.entry
			gtx.Execute(op.InvalidateCmd{})
			return
		}
	}
	if canClose && r.close.Clicked(gtx) {
		switch {
		case r.plan != nil:
			r.plan = nil
			r.focusDraft, r.restoreFocus = r.editing, !r.editing
		case r.editing:
			r.editing, r.editRow, r.notice, r.restoreFocus = false, nil, i18n.NoMessage, true
		}
		gtx.Execute(op.InvalidateCmd{})
	}
	if r.readError && r.retry.Clicked(gtx) {
		p.readReservations()
		r.restoreFocus = true
		r.focusRetry = false
		gtx.Execute(op.InvalidateCmd{})
	}
	if reviewing := r.plan != nil; reviewing != r.showingReview {
		r.showingReview = reviewing
		r.sheet.list.Position = layout.Position{}
		if reviewing {
			gtx.Execute(key.FocusCmd{Tag: &r.close})
		} else if r.editing {
			r.focusDraft = true
		} else if r.notice == i18n.HistoryAmendmentStale {
			r.actionFocus, r.restoreFocus = &r.add, true
		}
	}

	title := i18n.ReserveTitle
	switch {
	case r.plan != nil:
		title = i18n.ReserveReview
		if r.plan.release != "" {
			title = i18n.ReserveRelease
		}
	case r.editing:
		title = i18n.ReserveAdd
		if r.editRow != nil {
			title = i18n.ReserveEdit
		}
	}
	r.sheet.layoutPanel(gtx, view, view.Language.Text(title), func(gtx layout.Context, view ui.View) layout.Dimensions {
		if !r.editing && r.plan == nil && r.read == nil && !r.readError {
			return p.layoutReservationManager(gtx, view)
		}
		return p.layoutReservationWork(gtx, view)
	}, nil, canClose)
	r.overlay.Layout(gtx, view)
}

func (p *availableFundsPage) layoutReservationManager(gtx layout.Context, view ui.View) layout.Dimensions {
	r, l := p.reservation, view.Language
	if r.restoreFocus {
		focus := r.actionFocus
		if focus == nil {
			focus = &r.add
		}
		if focus == &r.add && !r.canAdd() {
			focus = &r.sheet.close
		}
		gtx.Execute(key.FocusCmd{Tag: focus})
		r.restoreFocus = false
	}
	children := []ui.ColumnChild{
		ui.Rigid(view.Message(l.Text(i18n.ReserveHelp), view.Colors.SecondaryInk)),
		ui.Rigid(r.reservationNotice(view)),
	}
	if r.source.Kind != "" {
		name := reservationSourceName(view, r.current.source(r.source), r.current.accounts[r.source.AccountID].Name)
		children = append(children,
			ui.Rigid(view.Message(name, view.Colors.Ink)),
			ui.Rigid(fundsAmount(view, i18n.FundsAvailableAfter, r.current.available(r.source))))
	}
	if r.canAdd() {
		children = append(children, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.reservationListActions(gtx, view)
		}))
	} else {
		message := i18n.ReserveSourceUnavailable
		if r.source.Kind == "" {
			message = i18n.ReserveNoEligibleSources
		}
		children = append(children, ui.Rigid(view.Message(l.Text(message), view.Colors.SecondaryInk)))
	}
	children = append(children, ui.Fill(func(gtx layout.Context) layout.Dimensions {
		return r.layoutReservationList(gtx, view)
	}))
	return (ui.Column{Gap: ui.Space8}).Layout(gtx, children...)
}

func (p *availableFundsPage) layoutReservationWork(gtx layout.Context, view ui.View) layout.Dimensions {
	r, l := p.reservation, view.Language
	defer func() { r.focusRetry = r.readError }()
	bounds := image.Rectangle{Min: view.Origin, Max: view.Origin.Add(gtx.Constraints.Max)}
	view = view.WithScrollAnchors(&r.sheet.list)
	r.overlay.ScrollOutsidePanel(gtx, &r.sheet.list, bounds)
	dims := view.List(&r.sheet.list).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		var body layout.Widget
		switch {
		case r.write != nil:
			body = view.Message(l.Text(i18n.SettingsPending), view.Colors.SecondaryInk)
		case r.read != nil:
			body = view.Message(l.Text(i18n.FundsLoading), view.Colors.SecondaryInk)
		case r.readError:
			if !r.focusRetry {
				gtx.Execute(key.FocusCmd{Tag: &r.retry})
			}
			message := i18n.FundsReadFailed
			if r.checking {
				message = i18n.ReserveVerifyFailed
			}
			body = func(gtx layout.Context) layout.Dimensions {
				return (ui.Column{Gap: ui.Space8}).Layout(gtx,
					ui.Rigid(view.Message(l.Text(message), view.Colors.ErrorInk)),
					ui.Rigid(view.Button(&r.retry, l.Text(i18n.HistoryRetry))))
			}
		case r.plan != nil:
			body = func(gtx layout.Context) layout.Dimensions { return p.layoutReservationReview(gtx, view) }
		default:
			if r.focusDraft {
				r.focusDraft = false
				if r.source.Kind == "" && r.editRow == nil {
					gtx.Execute(key.FocusCmd{Tag: &r.picker.Trigger})
				} else {
					r.label.Focus(gtx)
				}
			}
			body = func(gtx layout.Context) layout.Dimensions { return r.layoutReservationDraft(gtx, view) }
		}
		notice := r.reservationNotice(view)
		return (ui.Column{Gap: ui.Space8}).Layout(gtx,
			ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				dims := view.Message(l.Text(i18n.ReserveHelp), view.Colors.SecondaryInk)(gtx)
				view.Origin.Y += dims.Size.Y + gtx.Dp(ui.Space8)
				return dims
			}),
			ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				dims := notice(gtx)
				if dims.Size.Y != 0 {
					view.Origin.Y += dims.Size.Y + gtx.Dp(ui.Space8)
				}
				return dims
			}),
			ui.Rigid(body))
	})
	r.overlay.OffsetScrollAnchor(&r.sheet.list, bounds)
	return dims
}

func (r *fundsReservationSession) reservationNotice(view ui.View) layout.Widget {
	if r.notice == i18n.NoMessage {
		return func(layout.Context) layout.Dimensions { return layout.Dimensions{} }
	}
	color := view.Colors.ErrorInk
	if r.notice == i18n.ReserveSaved || r.notice == i18n.ReserveReleasedNotice {
		color = view.Colors.Ink
	}
	return view.Message(view.Language.Text(r.notice), color)
}
