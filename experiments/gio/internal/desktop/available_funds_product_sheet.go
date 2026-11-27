package desktop

import (
	"fmt"
	"image"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *availableFundsPage) layoutProductSetupSheet(gtx layout.Context, view ui.View) {
	d := p.setup
	calendar := productFormCalendar{today: p.localDate, zone: p.timezone}
	view.Overlay = &d.overlay
	d.overlay.BeginFrame(gtx)

	if d.overlay.Trigger == nil && !d.unresolved() && (productSheetDismissed(gtx, &d.sheet) || d.cancel.Clicked(gtx)) {
		p.closeProductSetup()
		gtx.Execute(op.InvalidateCmd{})
		return
	}

	title := i18n.ProductAddAction
	switch {
	case d.form == nil:
		title = i18n.ProductChooseAccount
		d.sheet.width = 450
	case d.plan != nil:
		title = i18n.HoldingReconReview
		d.sheet.width = 576
	default:
		d.sheet.width = 576
	}
	preview := d.form != nil && d.plan == nil && d.preview == nil && d.overlay.Trigger == nil && d.review.Clicked(gtx)
	if d.plan != nil {
		switch {
		case !d.unresolved() && d.edit.Clicked(gtx):
			d.plan, d.focus = nil, true
			d.sheet.list.Position = layout.Position{}
			gtx.Execute(op.InvalidateCmd{})
			return
		case d.confirm.Clicked(gtx):
			d.confirmReview()
			gtx.Execute(op.InvalidateCmd{})
		}
		if d.focus {
			d.focus = false
			if !d.uncertain {
				d.sheet.list.Position = layout.Position{}
			}
			gtx.Execute(key.FocusCmd{Tag: &d.confirm})
		}
	}
	if d.overlay.Trigger == nil {
		switch {
		case d.form == nil && len(d.choices) != 0:
			catalogSheetWrapFocus(gtx, &d.sheet.close, portfolioButton(d.accountButtons, d.choices[len(d.choices)-1].ID))
		case d.plan != nil && !d.unresolved():
			catalogSheetWrapFocus(gtx, &d.sheet.close, &d.confirm)
		case d.form != nil && d.plan == nil:
			last := &d.review
			if d.preview != nil {
				last = &d.sheet.close
			}
			catalogSheetWrapFocus(gtx, &d.sheet.close, last)
		}
	}

	d.sheet.layoutPanel(gtx, view, view.Language.Text(title), func(gtx layout.Context, view ui.View) layout.Dimensions {
		principalFocusTop := -1
		dims := productSheetRows(gtx, view, &d.sheet, &d.overlay, func(gtx layout.Context, view ui.View, origin *image.Point) []layout.Widget {
			switch {
			case d.form == nil:
				return d.layoutAccountChoices(gtx, view, func() { d.sheet.list.Position = layout.Position{} })
			case d.plan != nil:
				return d.reviewRows(view)
			default:
				return d.setupSheetFields(gtx, view, origin, calendar, &principalFocusTop)
			}
		})
		if principalFocusTop >= 0 {
			position := &d.sheet.list.Position
			top := principalFocusTop - view.Origin.Y
			minOffset := max(0, top+gtx.Dp(ui.PickerHeight)-gtx.Constraints.Max.Y)
			offset := min(max(position.Offset, minOffset), top)
			if position.Offset != offset {
				position.Offset = offset
				gtx.Execute(op.InvalidateCmd{})
			}
		}
		return dims
	}, d.setupSheetFooter(view), !d.unresolved())
	d.overlay.Layout(gtx, view)

	// Read the form only after its editors and any picker have processed input.
	if preview {
		if strings.TrimSpace(d.form.principal.Text()) == "" {
			d.principalRequired, d.focus, d.problem = true, true, i18n.NoMessage
		} else {
			d.principalRequired = false
			d.requestReview()
		}
		gtx.Execute(op.InvalidateCmd{})
	}
}

func productSheetDismissed(gtx layout.Context, sheet *analysisSheet) bool {
	dismiss := sheet.dismissed(gtx)
	for {
		e, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			return dismiss
		}
		dismiss = dismiss || e.(key.Event).State == key.Press
	}
}

func productSheetRows(gtx layout.Context, view ui.View, sheet *analysisSheet, overlay *ui.Overlay, rows func(layout.Context, ui.View, *image.Point) []layout.Widget) layout.Dimensions {
	bounds := image.Rectangle{Min: view.Origin, Max: view.Origin.Add(gtx.Constraints.Max)}
	view = view.WithScrollAnchors(&sheet.list)
	overlay.ScrollOutsidePanel(gtx, &sheet.list, bounds)
	dims := view.List(&sheet.list).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		origin := view.Origin
		fields := rows(gtx, view, &origin)
		children := make([]ui.ColumnChild, 0, len(fields))
		for _, field := range fields {
			children = append(children, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				return fieldPosition(gtx, &origin, field)
			}))
		}
		return (ui.Column{Gap: ui.Space8}).Layout(gtx, children...)
	})
	overlay.OffsetScrollAnchor(&sheet.list, bounds)
	return dims
}

func (d *fundsProductSetup) setupSheetFields(gtx layout.Context, view ui.View, origin *image.Point, calendar productFormCalendar, principalFocusTop *int) []layout.Widget {
	if d.currencyFocus {
		d.focus, d.currencyFocus = false, false
		gtx.Execute(key.FocusCmd{Tag: &d.form.currency.Trigger})
	}
	cash := view.Language.Text(i18n.OverviewNoValue)
	if amount, ok := d.cashAmount(); ok {
		cash = valuationMoney(view, domain.MoneyView{Currency: domain.CurrencyCode(d.currency), Amount: amount}, false)
	}
	return d.form.sheetFields(view, origin, calendar, d.account.Name, &d.currency, cash, d.preview == nil,
		func(gtx layout.Context, field *ui.TextField) {
			if d.focus {
				field.Focus(gtx)
				*principalFocusTop = origin.Y
				d.focus = false
			}
		}, func() { d.problem = i18n.NoMessage }, func(value string) string {
			if !d.principalRequired || strings.TrimSpace(value) != "" {
				return ""
			}
			return fmt.Sprintf(view.Language.Text(i18n.ProductSetupRequired), view.Language.Text(i18n.ProductAddPrincipal))
		})
}

func (d *fundsProductSetup) setupSheetFooter(view ui.View) layout.Widget {
	if d.form == nil {
		return nil
	}
	l := view.Language
	return func(gtx layout.Context) layout.Dimensions {
		width := gtx.Constraints.Max.X
		actions := func(gtx layout.Context) layout.Dimensions {
			if d.plan != nil {
				return layoutTargetBalanceConfirmation(gtx, view, d.post != nil, d.uncertain, &d.cancel, &d.edit, &d.confirm, i18n.HoldingReconConfirm)
			}
			return layout.Flex{Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
				layout.Rigid(taskPagerButton(view, &d.review, i18n.ReconciliationStartPreview, d.preview == nil)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.PrimaryButton(&d.confirm, l.Text(i18n.HoldingReconConfirm))(gtx.Disabled())
				}))
		}
		message := d.problem
		ink := view.Colors.ErrorInk
		if d.preview != nil {
			message, ink = i18n.HistoryLoading, view.Colors.SecondaryInk
		}
		var rows []ui.ColumnChild
		if d.plan == nil && message != i18n.NoMessage {
			rows = append(rows, ui.Rigid(view.Message(l.Text(message), ink)))
		}
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = width
			return layout.E.Layout(gtx, actions)
		}))
		return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...)
	}
}
