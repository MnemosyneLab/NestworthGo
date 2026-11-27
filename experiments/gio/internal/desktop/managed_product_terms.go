package desktop

import (
	"context"
	"errors"
	"image"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type productTermsLedger interface {
	UpdateProductTerms(context.Context, application.UpdateProductTermsInput) (application.ProductDetail, error)
}

type productTermsDraft struct {
	fields       *holdingProductTerms
	shortName    ui.TextField
	principal    domain.Money
	calendar     productFormCalendar
	sheet        analysisSheet
	overlay      ui.Overlay
	input        *application.UpdateProductTermsInput
	posting      <-chan historyResult[application.ProductDetail]
	save, cancel widget.Clickable
	reload       widget.Clickable
	problem      i18n.MessageID
	focus        bool
	focusButton  bool
}

func (p *productDetailPage) startTerms() {
	detail := p.current.detail
	sheet := newAnalysisSheet()
	sheet.modal, sheet.focus = true, false
	d := &productTermsDraft{fields: newHoldingProductTerms(), shortName: ui.NewTextField(true),
		principal: detail.Contract.Principal, calendar: productFormCalendar{today: p.current.localDate, zone: p.current.timezone},
		sheet: sheet, focus: true}
	d.fields.receive(*detail)
	d.shortName.Editor.SetText(detail.Contract.ShortName)
	p.terms, p.saved = d, i18n.NoMessage
}

func (p *productDetailPage) postTerms() {
	d := p.terms
	if d.posting != nil || d.problem == i18n.ProductStandaloneTermsStale {
		return
	}
	if d.input == nil {
		terms, policy, ok := d.fields.inputs()
		if !ok {
			d.problem = i18n.ProductEditValidDays
			return
		}
		shortName := d.shortName.Text()
		terms.ShortName = &shortName
		d.input = &application.UpdateProductTermsInput{ProductID: p.id, ExpectedRevision: d.fields.current.Revision,
			Terms: terms, Policy: policy}
	}
	input := *d.input
	d.problem = i18n.NoMessage
	d.posting = runHistoryRead(&p.workers, p.invalidate, func() (application.ProductDetail, error) {
		return p.termsLedger.UpdateProductTerms(p.ctx, input)
	})
}

func (p *productDetailPage) pollTerms() {
	d := p.terms
	if d == nil {
		return
	}
	select {
	case result := <-d.posting:
		d.posting = nil
		if result.err == nil && result.value.Contract.ID == d.input.ProductID && result.value.Contract.Revision == d.input.ExpectedRevision+1 {
			position := p.list.Position
			p.terms, p.focusAction = nil, &p.startTermsEdit
			p.refreshAfterProductWrite(i18n.ProductStandaloneTermsSaved)
			p.list.Position = position
			return
		}
		// The standalone Wails command uses the contract revision as a CAS.
		// Retry an uncertain response with the same revision and payload. A
		// conflict requires the user to reload and inspect the saved terms.
		var issue *domain.Error
		if errors.As(result.err, &issue) {
			switch issue.Code {
			case domain.ErrRevisionConflict:
				d.problem = i18n.ProductStandaloneTermsStale
				d.focusButton = true
				p.read()
				return
			case domain.ErrValidation, domain.ErrDecimalOverflow:
				d.input, d.problem = nil, i18n.ProductStandaloneTermsInvalid
				return
			}
		}
		d.problem = i18n.ProductStandaloneTermsRetry
		d.focusButton = true
	default:
	}
}

func (p *productDetailPage) layoutTermsSheet(gtx layout.Context, view ui.View) {
	d, l := p.terms, view.Language
	pending, stale := d.posting != nil, d.problem == i18n.ProductStandaloneTermsStale
	view.Overlay = &d.overlay
	d.overlay.BeginFrame(gtx)

	if d.overlay.Trigger == nil {
		if !pending && (productSheetDismissed(gtx, &d.sheet) || d.cancel.Clicked(gtx)) {
			p.terms, p.focusAction = nil, &p.startTermsEdit
			if d.input != nil && !stale {
				p.read()
			}
			gtx.Execute(op.InvalidateCmd{})
			return
		}
		if stale && p.request == nil && d.reload.Clicked(gtx) {
			if p.current != nil && p.current.detail != nil && p.err == nil {
				p.startTerms()
			} else {
				p.read()
			}
			gtx.Execute(op.InvalidateCmd{})
			return
		}
		if !pending {
			last := &d.save
			if stale {
				last = &d.reload
				if p.request != nil {
					last = &d.cancel
				}
			}
			catalogSheetWrapFocus(gtx, &d.sheet.close, last)
		}
	}

	save := !pending && !stale && d.overlay.Trigger == nil && d.save.Clicked(gtx)
	d.sheet.layoutPanel(gtx, view, l.Text(i18n.ProductEditTerms), func(gtx layout.Context, view ui.View) layout.Dimensions {
		return productSheetRows(gtx, view, &d.sheet, &d.overlay, d.termsSheetFields)
	}, p.termsSheetFooter(view, pending, stale), !pending)
	d.overlay.Layout(gtx, view)
	if save {
		p.postTerms()
		gtx.Execute(op.InvalidateCmd{})
	}
}

func (d *productTermsDraft) termsSheetFields(gtx layout.Context, view ui.View, origin *image.Point) []layout.Widget {
	l := view.Language
	fields := d.fields
	open := fields.current.State == string(domain.ProductStateOpen)
	if d.focus && open && fields.deposit != nil && gtx.Enabled() {
		focus := &fields.maturity.Trigger
		if d.calendar.today == "" {
			focus = &fields.deposit.mode.Trigger
		}
		gtx.Execute(key.FocusCmd{Tag: focus})
		d.focus = false
	}
	var rows []layout.Widget
	currency, principal := d.principal.Currency(), d.principal.CanonicalAmount()
	if open {
		label := l.Text(i18n.ProductAddPrincipal)
		amount := valuationMoney(view, domain.MoneyView{Currency: currency, Amount: principal}, false)
		rows = append(rows, labeledRow(view, label+" · "+amount, label, amount, view.Colors.Ink))
	}
	rows = append(rows, fields.termsFields(view, origin, d.calendar, currency.String(), principal, d.input == nil, true, true,
		func(gtx layout.Context, field *ui.TextField) {
			if d.focus && gtx.Enabled() {
				field.Focus(gtx)
				d.focus = false
			}
		}, nil)...)
	return append(rows, func(gtx layout.Context) layout.Dimensions {
		return d.shortName.Layout(gtx, view, ui.FieldOptions{Label: i18n.ProductTermsShortName,
			Help: i18n.ProductTermsShortNameHelp, Disabled: d.input != nil})
	})
}

func (p *productDetailPage) termsSheetFooter(view ui.View, pending, stale bool) layout.Widget {
	d, l := p.terms, view.Language
	return func(gtx layout.Context) layout.Dimensions {
		message, ink := d.problem, view.Colors.ErrorInk
		if pending {
			message, ink = i18n.SettingsPending, view.Colors.SecondaryInk
		}
		var rows []ui.ColumnChild
		if message != i18n.NoMessage {
			rows = append(rows, ui.Rigid(view.Message(l.Text(message), ink)))
		}
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			button, title := &d.save, i18n.FundsPolicySave
			enabled := !pending
			if stale {
				button, title, enabled = &d.reload, i18n.ProductTermsLoadLatest, p.request == nil
			} else if d.input != nil {
				title = i18n.HistoryRetry
			}
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
					layout.Rigid(taskPagerButton(view, &d.cancel, i18n.InstitutionsCancel, !pending)),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !enabled {
							gtx = gtx.Disabled()
						}
						if d.focusButton && gtx.Enabled() {
							gtx.Execute(key.FocusCmd{Tag: button})
							d.focusButton = false
						}
						return view.PrimaryButton(button, l.Text(title))(gtx)
					}))
			})
		}))
		return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...)
	}
}
