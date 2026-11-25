package desktop

import (
	"fmt"
	"image"
	"strings"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func (p *marketDataPage) layoutManualQuote(gtx layout.Context, view ui.View) (event.Tag, marketManualReceipt) {
	d := p.manual.draft
	closeDraft := func() {
		d.cancelRead()
		p.manual.draft = nil
		gtx.Execute(op.InvalidateCmd{})
	}
	select {
	case result := <-d.lookup:
		d.cancelRead()
		d.lookupErr = result.err
		if result.err == nil {
			d.refs = &result.value
		}
	case result := <-d.saving:
		d.saving = nil
		if result.value.saved() {
			closeDraft()
			return d.source, result.value
		}
		d.problem = manualQuoteError(result.err)
		gtx.Execute(key.FocusCmd{Tag: &d.sheet.action})
	default:
	}
	view.Overlay = &d.overlay
	d.overlay.BeginFrame(gtx)
	if d.overlay.Trigger == nil {
		dismissed := d.sheet.dismissed(gtx)
		for _, target := range []event.Tag{&d.amount.Editor, &d.basePicker.Trigger, &d.quotePicker.Trigger, &d.datePicker.Trigger, &d.sheet.list} {
			for {
				// Only consume Escape here. Edit and IME events belong to the editor.
				e, ok := gtx.Event(key.Filter{Focus: target, Name: key.NameEscape})
				if !ok {
					break
				}
				if e.(key.Event).State == key.Press {
					dismissed = true
				}
			}
		}
		if dismissed && d.saving == nil {
			closeDraft()
			return d.source, marketManualReceipt{}
		}
	}
	l := view.Language
	title, saveLabel := i18n.MarketManualFX, i18n.MarketManualSaveFX
	if d.target.instrumentID != "" {
		title, saveLabel = i18n.MarketManualPrice, i18n.MarketManualSavePrice
	}
	var footer layout.Widget
	if d.lookupErr != nil {
		if d.sheet.action.Clicked(gtx) {
			p.loadManualQuoteReferences()
		}
		footer = view.PrimaryButton(&d.sheet.action, l.Text(i18n.HistoryRetry))
	} else if d.available() {
		if d.sheet.action.Clicked(gtx) {
			p.saveManualQuote()
			gtx.Execute(op.InvalidateCmd{})
		}
		if d.saving != nil {
			saveLabel = i18n.SettingsPending
		} else if d.problem == i18n.MarketManualSaveFailed {
			saveLabel = i18n.HistoryRetry
		}
		footer = func(gtx layout.Context) layout.Dimensions {
			button := func(gtx layout.Context) layout.Dimensions {
				if d.saving != nil || strings.TrimSpace(d.amount.Text()) == "" || !d.validPair() {
					gtx = gtx.Disabled()
				}
				return view.PrimaryButton(&d.sheet.action, l.Text(saveLabel))(gtx)
			}
			problem := d.problem
			if !d.validPair() {
				problem = i18n.MarketManualInvalidPair
			}
			if problem == i18n.NoMessage {
				return button(gtx)
			}
			return (ui.Column{Gap: 6}).Layout(gtx,
				ui.Rigid(view.Message(l.Text(problem), view.Colors.ErrorInk)), ui.Rigid(button))
		}
	}
	d.sheet.layoutPanel(gtx, view, l.Text(title), func(gtx layout.Context, view ui.View) layout.Dimensions {
		switch {
		case d.lookup != nil:
			return view.Message(l.Text(i18n.MarketLoading), view.Colors.SecondaryInk)(gtx)
		case d.lookupErr != nil:
			return view.Message(l.Text(i18n.MarketFailed), view.Colors.ErrorInk)(gtx)
		case !d.available():
			return view.Message(l.Text(i18n.MarketManualUnavailable), view.Colors.SecondaryInk)(gtx)
		}
		bounds := image.Rectangle{Min: view.Origin, Max: view.Origin.Add(gtx.Constraints.Max)}
		view = view.WithScrollAnchors(&d.sheet.list)
		d.overlay.ScrollOutsidePanel(gtx, &d.sheet.list, bounds)
		dims := analysisResultList(gtx, view, &d.sheet.list, l.Text(title), 1, func(gtx layout.Context, _ int) layout.Dimensions {
			rowView := view
			rowView.Origin = rowView.Origin.Add(image.Pt(gtx.Dp(10), gtx.Dp(5)))
			return d.layoutFields(gtx, rowView, p.manual.clock())
		})
		d.overlay.OffsetScrollAnchor(&d.sheet.list, bounds)
		return dims
	}, footer, d.saving == nil)
	d.overlay.Layout(gtx, view)
	return nil, marketManualReceipt{}
}

func (d *marketManualDraft) layoutFields(gtx layout.Context, view ui.View, now time.Time) layout.Dimensions {
	l := view.Language
	x, y := gtx.Constraints.Max.X, 0
	place := func(body func(layout.Context, ui.View) layout.Dimensions) {
		dims := ui.Place(gtx, view, image.Pt(0, y), x, body)
		y += dims.Size.Y + gtx.Dp(8)
	}
	label := i18n.MarketManualRate
	currency := ""
	if i := d.refs.instrument; i != nil {
		label, currency = i18n.MarketManualUnitPrice, i.QuoteCurrency.String()
		if i.QuantityUnit != "" {
			currency += " / " + i.QuantityUnit
		}
		place(func(gtx layout.Context, view ui.View) layout.Dimensions {
			return view.Body(ui.TextLabel, i.DisplayLabel()+" · "+currency, view.Colors.Ink)(gtx)
		})
	} else {
		choices := ui.SelectOptions{}
		for _, code := range domain.SupportedCurrencies() {
			choices.Items = append(choices.Items, ui.Choice{ID: code.String(), Label: code.String()})
		}
		for _, field := range []struct {
			label  i18n.MessageID
			picker *ui.Select
			value  *domain.CurrencyCode
		}{
			{i18n.MarketManualBase, &d.basePicker, &d.base},
			{i18n.MarketManualQuote, &d.quotePicker, &d.quote},
		} {
			place(func(gtx layout.Context, view ui.View) layout.Dimensions {
				field.picker.Layout(gtx, view, ui.PickerField{Label: field.label, Origin: view.Origin, Disabled: d.saving != nil},
					choices, field.value.String(), func(value string) { *field.value, d.problem = domain.CurrencyCode(value), i18n.NoMessage })
				return manualPickerSize(gtx)
			})
		}
	}
	place(func(gtx layout.Context, view ui.View) layout.Dimensions {
		options := ui.FieldOptions{Label: label, Disabled: d.saving != nil}
		if currency != "" {
			options.AccessibleName = l.Text(label) + " · " + currency
		}
		return d.amount.Layout(gtx, view, options)
	})
	if d.refs.instrument == nil && d.validPair() {
		amount := strings.TrimSpace(d.amount.Text())
		if amount == "" {
			amount = "…"
		}
		place(func(gtx layout.Context, view ui.View) layout.Dimensions {
			return view.Message(fmt.Sprintf(l.Text(i18n.TransferQuoteRate), d.base, amount, d.quote), view.Colors.SecondaryInk)(gtx)
		})
	}
	place(func(gtx layout.Context, view ui.View) layout.Dimensions {
		today := now.In(d.refs.location).Format(time.DateOnly)
		dates := ui.DateOptions{Today: today, Max: today, Clearable: true, Format: view.DateFormat, WeekStart: time.Monday}
		if view.WeekStart == settings.WeekStartSunday {
			dates.WeekStart = time.Sunday
		}
		d.datePicker.Layout(gtx, view, ui.PickerField{Label: i18n.MarketManualDate, Origin: view.Origin, Disabled: d.saving != nil}, dates, d.date,
			func(value string) { d.date, d.problem = value, i18n.NoMessage })
		return manualPickerSize(gtx)
	})
	place(func(gtx layout.Context, view ui.View) layout.Dimensions {
		return view.Message(fmt.Sprintf(l.Text(i18n.MarketManualDateHelp), d.refs.location), view.Colors.SecondaryInk)(gtx)
	})
	place(func(gtx layout.Context, view ui.View) layout.Dimensions {
		return view.Message(l.Text(i18n.MarketManualSourceHelp), view.Colors.SecondaryInk)(gtx)
	})
	return layout.Dimensions{Size: image.Pt(x, y)}
}

func manualPickerSize(gtx layout.Context) layout.Dimensions {
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(24+ui.ControlDefault))}
}

func manualQuoteNotice(view ui.View, receipt marketManualReceipt) layout.Widget {
	l := view.Language
	var value, provenance string
	if q := receipt.instrument; q.ID != "" {
		value = receipt.name + " · " + l.Text(i18n.MarketManualUnitPrice) + " · " + q.Currency.String() + " " + portfolioDecimal(view, q.UnitPrice.Canonical())
		provenance = marketQuoteEvidence(view, q.SourceKind, q.SourceKey, q.EffectiveDate, q.TimestampBasis, q.QuotedAt, q.Delayed)
	} else {
		q := receipt.fx
		value = fmt.Sprintf(l.Text(i18n.TransferQuoteRate), q.BaseCurrency, portfolioDecimal(view, q.Rate.Canonical()), q.QuoteCurrency)
		provenance = marketQuoteEvidence(view, q.SourceKind, q.SourceKey, q.EffectiveDate, q.TimestampBasis, q.QuotedAt, q.Delayed)
	}
	return func(gtx layout.Context) layout.Dimensions {
		rows := []ui.ColumnChild{
			ui.Rigid(view.Body(ui.TextLabel, l.Text(i18n.MarketManualSaved), view.Colors.Ink)),
			ui.Rigid(view.Message(value+" · "+provenance, view.Colors.SecondaryInk)),
		}
		if receipt.partial {
			rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.MarketManualDerivedPending), view.Colors.WarningInk)))
		}
		return (ui.Column{Gap: 3}).Layout(gtx, rows...)
	}
}
