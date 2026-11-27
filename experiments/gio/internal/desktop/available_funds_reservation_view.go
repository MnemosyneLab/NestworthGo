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
		p.restoreReservationEntry(gtx, button)
		if button.Clicked(gtx) {
			p.openReservations(source, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		label := fmt.Sprintf(view.Language.Text(i18n.ReserveManage), name)
		return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			view.Focus.Watch(gtx, button)
			return view.ButtonOf(button, ui.ButtonSpec{
				Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: view.Language.Text(i18n.ReserveManageAll), Name: label,
			})(gtx)
		})
	}
}

func (p *availableFundsPage) globalReservationButton(view ui.View, button *widget.Clickable) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if p.reservationLedger == nil {
			return layout.Dimensions{}
		}
		p.restoreReservationEntry(gtx, button)
		if button.Clicked(gtx) {
			p.openReservations(domain.LiquiditySourceRef{}, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		view.Focus.Watch(gtx, button)
		return layout.W.Layout(gtx, view.ButtonOf(button, ui.ButtonSpec{
			Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: view.Language.Text(i18n.ReserveManageAll),
		}))
	}
}

func (p *availableFundsPage) restoreReservationEntry(gtx layout.Context, button *widget.Clickable) {
	if gtx.Enabled() && p.reservationFocus == button {
		p.reservationFocus = nil
		gtx.Execute(key.FocusCmd{Tag: button})
	}
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
		r.startDraft(nil, &r.add)
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
	l := view.Language
	target := r.focusTarget
	targetIndex := -1
	rows := make([]layout.Widget, 0, len(r.current.records)+2)
	released := make([]domain.LiquidityReservation, 0)
	addRecord := func(row domain.LiquidityReservation) {
		if row.ID == target {
			targetIndex = len(rows)
		}
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			title := row.Label
			source := reservationSourceName(view, r.current.source(row.Source), r.current.accounts[row.Source.AccountID].Name)
			fields := []layout.Widget{
				fundsAmount(view, i18n.FundsRequested, &row.Amount),
				view.Message(source, view.Colors.SecondaryInk),
			}
			if row.Active() {
				fields = append(fields, r.reservationRowActions(view, row, source, row.ID == target))
			} else {
				title = fmt.Sprintf(l.Text(i18n.ReserveReleasedRow), row.Label)
				if row.ID == target && gtx.Enabled() {
					gtx.Execute(key.FocusCmd{Tag: &r.sheet.close})
				}
			}
			return layout.Inset{Bottom: 8}.Layout(gtx, fundsCard(view, title, fields...))
		})
	}
	for _, row := range r.current.records {
		if row.Active() {
			addRecord(row)
		} else {
			released = append(released, row)
		}
	}
	if len(rows) == 0 {
		message := i18n.ReserveEmpty
		if r.source.Kind == "" {
			message = i18n.ReserveGlobalEmpty
		}
		rows = append(rows, view.Message(l.Text(message), view.Colors.SecondaryInk))
	}
	if len(released) > 0 {
		if r.releasedHistory.Clicked(gtx) {
			r.showReleased = !r.showReleased
			gtx.Execute(op.InvalidateCmd{})
		}
		if target != "" && slices.ContainsFunc(released, func(row domain.LiquidityReservation) bool { return row.ID == target }) {
			r.showReleased = true
		}
		message := i18n.ReserveReleasedHistoryMany
		if len(released) == 1 {
			message = i18n.ReserveReleasedHistoryOne
		}
		title := fmt.Sprintf(l.Text(message), len(released))
		rows = append(rows, analysisDisclosure(view, &r.releasedHistory, title, r.showReleased))
		if r.showReleased {
			for _, row := range released {
				addRecord(row)
			}
		}
	}
	if targetIndex >= 0 && gtx.Enabled() {
		r.list.Position = layout.Position{First: targetIndex}
		r.focusTarget = ""
	}
	r.list.Position = restoredPosition(r.list.Position, len(rows))
	return view.List(&r.list).Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
		return rows[i](gtx)
	})
}

func (r *fundsReservationSession) reservationRowActions(view ui.View, row domain.LiquidityReservation, source string, focus bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		buttons := r.actions[row.ID]
		if buttons == nil {
			buttons = new(fundsReservationActions)
			r.actions[row.ID] = buttons
		}
		if buttons.edit.Clicked(gtx) {
			r.startDraft(&row, &buttons.edit)
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
		editName, releaseName := edit, release
		if r.current.activeLabels[row.Label] > 1 {
			editName, releaseName = edit+" · "+source, release+" · "+source
		}
		actions := []layout.Widget{
			view.NamedButton(&buttons.edit, view.Language.Text(i18n.ReserveEdit), editName),
			view.NamedButton(&buttons.release, view.Language.Text(i18n.ReserveRelease), releaseName),
		}
		return (ui.Wrap{Gap: ui.Space8}).Layout(gtx, len(actions), func(gtx layout.Context, i int) layout.Dimensions {
			return actions[i](gtx)
		})
	}
}

func (r *fundsReservationSession) layoutReservationDraft(gtx layout.Context, view ui.View) layout.Dimensions {
	l := view.Language
	latest, changed := r.latestEdit()
	if changed && latest != nil && latest.Active() && r.loadLatest.Clicked(gtx) {
		r.startDraft(latest, r.actionFocus)
		changed = false
		gtx.Execute(op.InvalidateCmd{})
	}
	source := r.draftSource()
	var currency domain.CurrencyCode
	switch {
	case r.editRow != nil:
		currency = r.editRow.Currency
	case source != nil:
		currency = source.NativeCurrency
	}
	if !changed && r.review.Clicked(gtx) {
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
		options := ui.SelectOptions{Items: choices, Searchable: true}
		if r.selected == "" {
			options.ValueLabel = l.Text(i18n.ReserveChooseSource)
		}
		r.picker.Layout(gtx, view, ui.PickerField{Label: i18n.ReserveSource, Origin: view.Origin}, options,
			r.selected, func(id string) { r.selected = id })
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
	}
	return (ui.Column{Gap: ui.Space8}).Layout(gtx,
		ui.Rigid(sourceField),
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
			actions := []layout.Widget{view.Button(&r.close, l.Text(i18n.InstitutionsCancel))}
			switch {
			case !changed:
				actions = append(actions, view.PrimaryButton(&r.review, l.Text(i18n.ReserveReview)))
			case latest != nil && latest.Active():
				actions = append(actions, view.PrimaryButton(&r.loadLatest, l.Text(i18n.ReserveLoadLatest)))
			}
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
