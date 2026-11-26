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
	list         widget.List
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
	d := &productTermsDraft{fields: newHoldingProductTerms(), shortName: ui.NewTextField(true),
		principal: detail.Contract.Principal, calendar: productFormCalendar{today: p.current.localDate, zone: p.current.timezone},
		list: widget.List{List: layout.List{Axis: layout.Vertical}}, focus: true}
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
			p.terms = nil
			p.refreshAfterProductWrite(i18n.ProductStandaloneTermsSaved)
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

func (p *productDetailPage) layoutTerms(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.terms, view.Language
	pending, stale := d.posting != nil, d.problem == i18n.ProductStandaloneTermsStale
	if !pending && d.cancel.Clicked(gtx) {
		p.terms, p.focusAction = nil, &p.startTermsEdit
		if d.input != nil && !stale {
			p.read()
		}
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if stale && p.request == nil && d.reload.Clicked(gtx) {
		if p.current != nil && p.current.detail != nil && p.err == nil {
			p.startTerms()
		} else {
			p.read()
		}
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	gtx, view = view.Measure(gtx, 660)
	origin := view.Origin
	formView := view.WithScrollAnchors(&d.list)
	currency, principal := d.principal.Currency().String(), d.principal.CanonicalAmount()
	fields := d.fields.fields(formView, &origin, d.calendar, currency, principal, d.input == nil, true, func(gtx layout.Context, field *ui.TextField) {
		if d.focus && gtx.Enabled() {
			field.Focus(gtx)
			d.focus = false
		}
	})
	fields = append(fields, func(gtx layout.Context) layout.Dimensions {
		return d.shortName.Layout(gtx, view, ui.FieldOptions{Label: i18n.ProductTermsShortName, Disabled: d.input != nil})
	}, view.Message(l.Text(i18n.ProductTermsShortNameHelp), view.Colors.SecondaryInk))
	place := func(body layout.Widget) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions { return fieldPosition(gtx, &origin, body) }
	}
	children := make([]layout.FlexChild, 0, len(fields))
	for _, field := range fields {
		children = append(children, layout.Rigid(place(field)))
	}
	return ui.Column{Gap: 8}.Layout(gtx,
		ui.Rigid(place(tradeTitle(view, i18n.ProductEditTerms))),
		ui.Rigid(place(fundsAmount(view, i18n.ProductPrincipal, &d.principal))),
		ui.Fill(func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Min: origin, Max: origin.Add(gtx.Constraints.Max)}
			view.Overlay.ScrollOutsidePanel(gtx, &d.list, bounds)
			dims := view.List(&d.list).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
			})
			view.Overlay.OffsetScrollAnchor(&d.list, bounds)
			return dims
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			if pending {
				return view.Message(l.Text(i18n.SettingsPending), view.Colors.SecondaryInk)(gtx)
			}
			if d.problem != i18n.NoMessage {
				return view.Message(l.Text(d.problem), view.Colors.ErrorInk)(gtx)
			}
			return layout.Dimensions{}
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			button, title := &d.save, i18n.SettingsSave
			enabled := !pending
			if stale {
				button, title, enabled = &d.reload, i18n.ProductTermsLoadLatest, p.request == nil
			} else if d.input != nil {
				title = i18n.HistoryRetry
			}
			return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(taskPagerButton(view, &d.cancel, i18n.InstitutionsCancel, !pending)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !enabled {
						gtx = gtx.Disabled()
					}
					if d.focusButton && gtx.Enabled() {
						gtx.Execute(key.FocusCmd{Tag: button})
						d.focusButton = false
					}
					if button == &d.save && d.save.Clicked(gtx) && enabled {
						p.postTerms()
						gtx.Execute(op.InvalidateCmd{})
					}
					view.Focus.Watch(gtx, button)
					return view.PrimaryButton(button, l.Text(title))(gtx)
				}))
		}))
}
