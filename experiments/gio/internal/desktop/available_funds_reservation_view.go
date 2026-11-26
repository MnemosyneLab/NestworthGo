package desktop

import (
	"fmt"
	"image"
	"slices"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

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
			return view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: label})(gtx)
		})
	}
}

func (p *availableFundsPage) globalReservationButton(view ui.View, button *widget.Clickable) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if p.reservationLedger == nil {
			return layout.Dimensions{}
		}
		if button.Clicked(gtx) {
			p.openReservations(domain.LiquiditySourceRef{}, "", button)
			gtx.Execute(op.InvalidateCmd{})
		}
		view.Focus.Watch(gtx, button)
		return layout.W.Layout(gtx, view.ButtonOf(button, ui.ButtonSpec{
			Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: view.Language.Text(i18n.ReserveManageAll),
		}))
	}
}

func (p *availableFundsPage) layoutReservations(gtx layout.Context, view ui.View) layout.Dimensions {
	r, l := p.reservation, view.Language
	gtx, view = view.Measure(gtx, 690)
	if r.close.Clicked(gtx) && !p.reservationUnresolved() {
		if r.plan != nil {
			r.plan = nil
			if r.editing {
				r.label.Focus(gtx)
			} else {
				r.restoreFocus = true
			}
		} else if r.editing {
			r.editing, r.editRow, r.notice, r.restoreFocus = false, nil, i18n.NoMessage, true
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
	if r.source.Kind == "" {
		title = i18n.ReserveGlobalTitle
	}
	if r.editing {
		title = i18n.ReserveAdd
		if r.editRow != nil {
			title = i18n.ReserveEdit
		}
	}
	if r.plan != nil {
		title = i18n.ReserveReview
		if r.plan.release != "" {
			title = i18n.ReserveRelease
		}
	}
	var header []ui.ColumnChild
	height := 0
	appendRow := func(w layout.Widget) {
		header = append(header, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := w(gtx)
			height += dims.Size.Y + gtx.Dp(ui.Space8)
			return dims
		}))
	}
	appendRow(tradeTitle(view, title))
	if r.source.Kind != "" && !r.editing {
		name := r.name
		if r.current != nil {
			name = reservationSourceName(view, r.current.source(r.source), r.current.accounts[r.source.AccountID].Name)
		}
		appendRow(view.Message(name, view.Colors.Ink))
	}
	if !r.editing && r.plan == nil && !p.reservationUnresolved() {
		appendRow(func(gtx layout.Context) layout.Dimensions {
			if r.restoreFocus {
				focus := r.actionFocus
				if focus == nil || focus == &r.add && !r.canAdd() {
					focus = &r.close
				}
				gtx.Execute(key.FocusCmd{Tag: focus})
				r.restoreFocus = false
			}
			actions := []layout.Widget{view.Button(&r.close, l.Text(i18n.ReserveBack))}
			if r.current != nil && !r.readError && r.read == nil && r.canAdd() {
				actions = append(actions, func(gtx layout.Context) layout.Dimensions { return p.reservationListActions(gtx, view) })
			}
			return fundsWrappedActions(actions)(gtx)
		})
	}
	if r.notice != i18n.NoMessage {
		color := view.Colors.ErrorInk
		if r.notice == i18n.ReserveSaved || r.notice == i18n.ReserveReleasedNotice {
			color = view.Colors.Ink
		}
		appendRow(view.Message(l.Text(r.notice), color))
	}
	switch {
	case r.write != nil:
		appendRow(view.Message(l.Text(i18n.SettingsPending), view.Colors.SecondaryInk))
	case r.read != nil:
		appendRow(view.Message(l.Text(i18n.FundsLoading), view.Colors.SecondaryInk))
	case r.readError:
		message := i18n.FundsReadFailed
		if r.checking {
			message = i18n.ReserveVerifyFailed
		}
		appendRow(view.Message(l.Text(message), view.Colors.ErrorInk))
		appendRow(view.Button(&r.retry, l.Text(i18n.HistoryRetry)))
	case r.plan != nil:
		appendRow(func(gtx layout.Context) layout.Dimensions { return p.layoutReservationReview(gtx, view) })
	case r.editing:
		appendRow(func(gtx layout.Context) layout.Dimensions {
			view.Origin.Y += height
			return r.layoutReservationDraft(gtx, view)
		})
	default:
		if r.source.Kind != "" {
			appendRow(fundsAmount(view, i18n.FundsAvailableAfter, r.current.available(r.source)))
		}
		if !r.canAdd() {
			message := i18n.ReserveSourceUnavailable
			if r.source.Kind == "" {
				message = i18n.ReserveNoEligibleSources
			}
			appendRow(view.Message(l.Text(message), view.Colors.SecondaryInk))
		}
		header = append(header, ui.Fill(func(gtx layout.Context) layout.Dimensions { return r.layoutReservationList(gtx, view) }))
	}
	return (ui.Column{Gap: ui.Space8}).Layout(gtx, header...)
}

func (r *fundsReservationSession) canAdd() bool {
	if r.source.Kind != "" {
		return reservableFundsSource(r.current.source(r.source))
	}
	return slices.ContainsFunc(r.current.sources, func(source domain.LiquiditySourceResult) bool {
		return reservableFundsSource(&source)
	})
}

func (p *availableFundsPage) reservationListActions(gtx layout.Context, view ui.View) layout.Dimensions {
	r, l := p.reservation, view.Language
	if r.add.Clicked(gtx) {
		r.startDraft(gtx, nil, &r.add)
		if r.source.Kind == "" {
			gtx.Execute(key.FocusCmd{Tag: &r.picker.Trigger})
		}
		gtx.Execute(op.InvalidateCmd{})
	}
	return layout.W.Layout(gtx, view.PrimaryButton(&r.add, l.Text(i18n.ReserveAdd)))
}

func reservationSourceName(view ui.View, source *domain.LiquiditySourceResult, account string) string {
	if source == nil {
		return strings.TrimSpace(strings.Join([]string{account, view.Language.Text(i18n.ReserveSourceUnavailable)}, " "))
	}
	name := source.DisplayName
	switch source.Ref.Kind {
	case domain.SourceAccountCash:
		if account != "" {
			name = fmt.Sprintf(view.Language.Text(i18n.FundsCashSource), account)
		}
	case domain.SourceHolding:
		if account != "" {
			name = account + " · " + name
		}
	}
	return name + " · " + source.NativeCurrency.String()
}

func (r *fundsReservationSession) layoutReservationList(gtx layout.Context, view ui.View) layout.Dimensions {
	if len(r.current.records) == 0 {
		message := i18n.ReserveEmpty
		if r.source.Kind == "" {
			message = i18n.ReserveGlobalEmpty
		}
		return view.Message(view.Language.Text(message), view.Colors.SecondaryInk)(gtx)
	}
	records := slices.Clone(r.current.records)
	slices.SortStableFunc(records, func(a, b domain.LiquidityReservation) int {
		switch {
		case a.Active() == b.Active():
			return 0
		case a.Active():
			return -1
		default:
			return 1
		}
	})
	target := r.focusTarget
	if target != "" && gtx.Enabled() {
		at := slices.IndexFunc(records, func(row domain.LiquidityReservation) bool { return row.ID == target })
		r.list.Position = layout.Position{First: at}
		r.focusTarget = ""
	}
	r.list.Position = restoredPosition(r.list.Position, len(records))
	return view.List(&r.list).Layout(gtx, len(records), func(gtx layout.Context, i int) layout.Dimensions {
		row := records[i]
		l := view.Language
		title := row.Label
		source := reservationSourceName(view, r.current.source(row.Source), r.current.accounts[row.Source.AccountID].Name)
		fields := []layout.Widget{
			fundsAmount(view, i18n.FundsRequested, &row.Amount),
			view.Message(source, view.Colors.SecondaryInk),
		}
		if !row.Active() {
			title = fmt.Sprintf(l.Text(i18n.ReserveReleasedRow), row.Label)
			if row.ID == target && gtx.Enabled() {
				gtx.Execute(key.FocusCmd{Tag: &r.close})
			}
		} else {
			fields = append(fields, r.reservationRowActions(view, row, row.ID == target))
		}
		return layout.Inset{Bottom: 8}.Layout(gtx, fundsCard(view, title, fields...))
	})
}

func (r *fundsReservationSession) reservationRowActions(view ui.View, row domain.LiquidityReservation, focus bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		buttons := r.actions[row.ID]
		if buttons == nil {
			buttons = new(fundsReservationActions)
			r.actions[row.ID] = buttons
		}
		if buttons.edit.Clicked(gtx) {
			r.startDraft(gtx, &row, &buttons.edit)
			gtx.Execute(op.InvalidateCmd{})
		}
		if buttons.release.Clicked(gtx) {
			r.plan = &fundsReservationPlan{source: row.Source, version: r.current.version, release: row.ID,
				revision: row.Revision, label: row.Label, amount: row.Amount}
			r.actionFocus, r.notice = &buttons.release, i18n.NoMessage
			gtx.Execute(op.InvalidateCmd{})
		}
		if focus && gtx.Enabled() {
			gtx.Execute(key.FocusCmd{Tag: &buttons.edit})
		}
		edit := fmt.Sprintf(view.Language.Text(i18n.ReserveEditRow), row.Label)
		release := fmt.Sprintf(view.Language.Text(i18n.ReserveReleaseRow), row.Label)
		actions := []layout.Widget{view.Button(&buttons.edit, edit), view.Button(&buttons.release, release)}
		return (ui.Wrap{Gap: ui.Space8}).Layout(gtx, len(actions), func(gtx layout.Context, i int) layout.Dimensions {
			return actions[i](gtx)
		})
	}
}

func (r *fundsReservationSession) layoutReservationDraft(gtx layout.Context, view ui.View) layout.Dimensions {
	l := view.Language
	source := r.draftSource()
	var currency domain.CurrencyCode
	switch {
	case r.editRow != nil:
		currency = r.editRow.Currency
	case source != nil:
		currency = source.NativeCurrency
	}
	if r.review.Clicked(gtx) {
		r.reviewNew()
		gtx.Execute(op.InvalidateCmd{})
	}
	sourceField := func(gtx layout.Context) layout.Dimensions {
		if r.source.Kind != "" || r.editRow != nil {
			ref := r.source
			if r.editRow != nil {
				ref = r.editRow.Source
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(view.Body(ui.TextLabel, l.Text(i18n.ReserveSource), view.Colors.Ink)),
				layout.Rigid(view.Message(reservationSourceName(view, source, r.current.accounts[ref.AccountID].Name), view.Colors.SecondaryInk)))
		}
		choices := make([]ui.Choice, 0, len(r.current.sources))
		counts := map[string]int{}
		for _, item := range r.current.sources {
			if reservableFundsSource(&item) {
				title := reservationSourceName(view, &item, r.current.accounts[item.AccountID].Name)
				choices = append(choices, ui.Choice{ID: item.Ref.Key(), Label: title})
				counts[title]++
			}
		}
		seen := map[string]int{}
		for i, choice := range choices {
			if counts[choice.Label] > 1 {
				seen[choice.Label]++
				choices[i].Label = fmt.Sprintf("%s · %d", choice.Label, seen[choice.Label])
			}
		}
		r.picker.Layout(gtx, view, ui.PickerField{Label: i18n.ReserveSource, Origin: view.Origin},
			ui.SelectOptions{Items: choices, Searchable: true}, r.selected, func(id string) { r.selected = id })
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
	}
	return (ui.Column{Gap: ui.Space8}).Layout(gtx,
		ui.Rigid(sourceField),
		ui.Rigid(view.Message(l.Text(i18n.ReserveHelp), view.Colors.SecondaryInk)),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.label.Layout(gtx, view, ui.FieldOptions{Label: i18n.ReserveLabel})
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			if currency == "" {
				return r.amount.Layout(gtx, view, ui.FieldOptions{Label: i18n.ReserveAmount})
			}
			return r.amount.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.ReserveAmount}, currency.String())
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			actions := []layout.Widget{view.Button(&r.close, l.Text(i18n.InstitutionsCancel)),
				view.PrimaryButton(&r.review, l.Text(i18n.ReserveReview))}
			return (ui.Wrap{Gap: ui.Space8}).Layout(gtx, len(actions), func(gtx layout.Context, i int) layout.Dimensions {
				return actions[i](gtx)
			})
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
			fundsAmount(view, i18n.ReserveCurrentAvailability, r.current.available(r.plan.source)))),
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
