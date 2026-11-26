package desktop

import (
	"image"
	"reflect"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type fundsPolicyPreset uint8

const (
	policyNow fundsPolicyPreset = iota
	policySoon
	policyDate
	policyExcluded
	policyCustom
)

var fundsPolicyPresets = [policyCustom]struct{ title, hint i18n.MessageID }{
	{i18n.FundsPolicyNow, i18n.FundsPolicyNowHint},
	{i18n.FundsPolicySoon, i18n.FundsPolicySoonHint},
	{i18n.FundsPolicyDate, i18n.FundsPolicyDateHint},
	{i18n.FundsPolicyExcluded, i18n.FundsPolicyExcludedHint},
}

type fundsPolicyFields struct {
	value                                       application.ProductPolicyInput
	cap, days, fee, earlyDays, earlyFee, gross  ui.TextField
	quickUnlock, unlock, receipt                ui.DatePicker
	access, basis, early, earlyBasis, earlyMode ui.Select
	presets                                     [policyCustom]widget.Clickable
	advanced                                    bool
	toggle                                      widget.Clickable
}

func newFundsPolicyFields(policy *domain.LiquidityPolicy) fundsPolicyFields {
	f := fundsPolicyFields{cap: ui.NewTextField(true), days: ui.NewTextField(true), fee: ui.NewTextField(true),
		earlyDays: ui.NewTextField(true), earlyFee: ui.NewTextField(true), gross: ui.NewTextField(true)}
	value := simpleFundsPolicy(policyNow, application.ProductPolicyInput{})
	value.AccessKind, value.SettlementDays, value.NormalExitFee = string(domain.AccessUnknown), nil, nil
	if policy != nil {
		value = application.ProductPolicyInput{
			AccessKind: string(policy.AccessKind), UnlockOn: policy.UnlockOn, SettlementDays: policy.SettlementDays,
			DayBasis: fundsPolicyEnum(policy.DayBasis), ReceiptOnOverride: policy.ReceiptOnOverride,
			AccessibleAmountCap: fundsPolicyMoney(policy.AccessibleAmountCap), NormalExitFee: fundsPolicyMoney(policy.NormalExitFee),
			EarlyKind: string(policy.EarlyKind), EarlySettlementDays: policy.EarlySettlementDays, EarlyDayBasis: fundsPolicyEnum(policy.EarlyDayBasis),
			EarlyFee: fundsPolicyMoney(policy.EarlyFee), EarlyAmountMode: fundsPolicyEnum(policy.EarlyAmountMode),
			EarlyGrossAmount: fundsPolicyMoney(policy.EarlyGrossAmount), Note: policy.Note,
		}
	}
	f.load(value)
	f.advanced = value.AccessKind != string(domain.AccessUnknown) && selectedFundsPolicy(value) == policyCustom
	return f
}

func fundsPolicyEnum[T ~string](value *T) *string {
	if value == nil {
		return nil
	}
	return productOptional(string(*value))
}

func fundsPolicyMoney(value *domain.Money) *string {
	if value == nil {
		return nil
	}
	return productOptional(value.CanonicalAmount())
}

func (f *fundsPolicyFields) load(value application.ProductPolicyInput) {
	f.value = value
	f.cap.Editor.SetText(productString(value.AccessibleAmountCap))
	f.days.Editor.SetText(productDays(value.SettlementDays))
	f.fee.Editor.SetText(productString(value.NormalExitFee))
	f.earlyDays.Editor.SetText(productDays(value.EarlySettlementDays))
	f.earlyFee.Editor.SetText(productString(value.EarlyFee))
	f.gross.Editor.SetText(productString(value.EarlyGrossAmount))
}

// These are the desktop quick choices. The application still validates the
// resulting fields and projects the actual receipt and amounts.
func simpleFundsPolicy(preset fundsPolicyPreset, current application.ProductPolicyInput) application.ProductPolicyInput {
	zero, one := 0, 1
	value := application.ProductPolicyInput{AccessKind: string(domain.AccessOnRequest), SettlementDays: &zero,
		DayBasis: productOptional(string(domain.DayBasisCalendar)), NormalExitFee: productOptional("0"),
		EarlyKind: string(domain.EarlyNotAllowed), Note: current.Note}
	switch preset {
	case policySoon:
		value.SettlementDays, value.DayBasis = &one, productOptional(string(domain.DayBasisWeekdays))
	case policyDate:
		value.AccessKind, value.UnlockOn = string(domain.AccessOnDate), current.UnlockOn
	case policyExcluded:
		value.AccessKind = string(domain.AccessExcluded)
	}
	return value
}

func selectedFundsPolicy(value application.ProductPolicyInput) fundsPolicyPreset {
	for i := range fundsPolicyPresets {
		preset := fundsPolicyPreset(i)
		if reflect.DeepEqual(value, simpleFundsPolicy(preset, value)) {
			return preset
		}
	}
	return policyCustom
}

func (f *fundsPolicyFields) input() (application.ProductPolicyInput, bool) {
	value := f.value
	days, daysOK := parseProductDays(f.days.Text())
	earlyDays, earlyOK := parseProductDays(f.earlyDays.Text())
	value.SettlementDays = days
	if days != nil && value.DayBasis == nil {
		value.DayBasis = productOptional(string(domain.DayBasisCalendar))
	}
	if earlyOK {
		value.EarlySettlementDays = earlyDays
	}
	value.AccessibleAmountCap, value.NormalExitFee = productOptional(f.cap.Text()), productOptional(f.fee.Text())
	value.EarlyFee, value.EarlyGrossAmount = productOptional(f.earlyFee.Text()), productOptional(f.gross.Text())
	return value, daysOK && (earlyOK || value.EarlyKind != string(domain.EarlyAllowed))
}

func (p *availableFundsPage) layoutPolicy(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.policy, view.Language
	busy := d.write != nil || d.checking
	conflict := d.problem == i18n.FundsPolicyConflict
	if !busy && d.cancel.Clicked(gtx) {
		p.cancelPolicy(false)
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if d.reload.Clicked(gtx) {
		if d.checking && p.err != nil {
			d.footerFocus = &d.reload
			p.read()
		} else if conflict && d.latest != nil {
			p.startPolicy(*d.latest, d.title, d.entry)
			gtx.Execute(op.InvalidateCmd{})
			return layout.Dimensions{}
		}
		gtx.Execute(op.InvalidateCmd{})
	}
	gtx, view = view.Measure(gtx, 660)
	origin := view.Origin
	place := func(body layout.Widget) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions { return fieldPosition(gtx, &origin, body) }
	}
	return ui.Column{Gap: ui.Space8}.Layout(gtx,
		ui.Rigid(place(tradeTitle(view, i18n.FundsPolicyTitle))),
		ui.Rigid(place(view.Message(d.title, view.Colors.SecondaryInk))),
		ui.Fill(func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Min: origin, Max: origin.Add(gtx.Constraints.Max)}
			view.Overlay.ScrollOutsidePanel(gtx, &d.list, bounds)
			formView := view.WithScrollAnchors(&d.list)
			fields := d.formFields(formView, &origin, p.localDate, !busy && !conflict)
			dims := view.List(&d.list).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
				children := make([]layout.FlexChild, 0, len(fields))
				for _, field := range fields {
					children = append(children, layout.Rigid(place(field)))
				}
				return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space8)}.Layout(gtx, children...)
			})
			view.Overlay.OffsetScrollAnchor(&d.list, bounds)
			return dims
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			problem, ink := d.problem, view.Colors.ErrorInk
			switch {
			case d.checking && p.err != nil:
				problem = i18n.FundsPolicyReadFailed
			case busy:
				problem, ink = i18n.FundsPolicyChecking, view.Colors.SecondaryInk
			}
			if problem == i18n.NoMessage {
				return layout.Dimensions{}
			}
			return view.Message(l.Text(problem), ink)(gtx)
		}),
		ui.Rigid(p.policyFooter(view, busy, conflict)),
	)
}

func (p *availableFundsPage) policyFooter(view ui.View, busy, conflict bool) layout.Widget {
	d, l := p.policy, view.Language
	return func(gtx layout.Context) layout.Dimensions {
		button := func(target *widget.Clickable, text i18n.MessageID, enabled bool, style ui.ButtonVariant, click func()) layout.Widget {
			return func(gtx layout.Context) layout.Dimensions {
				if !enabled {
					gtx = gtx.Disabled()
				}
				if d.footerFocus == target && gtx.Enabled() {
					d.footerFocus = nil
					gtx.Execute(key.FocusCmd{Tag: target})
				}
				if click != nil && gtx.Enabled() && target.Clicked(gtx) {
					click()
					gtx.Execute(op.InvalidateCmd{})
				}
				return view.ButtonOf(target, ui.ButtonSpec{Variant: style, Title: l.Text(text)})(gtx)
			}
		}
		actions := []layout.Widget{button(&d.cancel, i18n.InstitutionsCancel, !busy, ui.ButtonOutline, nil)}
		switch {
		case d.checking && p.err != nil:
			actions = append(actions, button(&d.reload, i18n.HistoryRetry, true, ui.ButtonPrimary, nil))
		case conflict && d.latest != nil:
			actions = append(actions, button(&d.reload, i18n.FundsPolicyLoadLatest, true, ui.ButtonPrimary, nil))
		case !conflict:
			if d.revision > 0 {
				actions = append(actions, button(&d.reset, i18n.FundsPolicyReset, !busy, ui.ButtonOutline, func() {
					d.confirming = true
					d.confirmation.Open()
				}))
			}
			actions = append(actions, button(&d.save, i18n.FundsPolicySave, !busy, ui.ButtonPrimary, func() { p.postPolicy(false) }))
		}
		return (ui.Wrap{Gap: ui.Space8}).Layout(gtx, len(actions), func(gtx layout.Context, i int) layout.Dimensions { return actions[i](gtx) })
	}
}

func (d *fundsPolicyDraft) formFields(view ui.View, origin *image.Point, today string, ready bool) []layout.Widget {
	f, l := &d.fields, view.Language
	value, valid := f.input()
	selected := policyCustom
	if valid {
		selected = selectedFundsPolicy(value)
	}
	fields := []layout.Widget{tradeTitle(view, i18n.FundsPolicyQuick)}
	for preset, copy := range fundsPolicyPresets {
		fields = append(fields, func(gtx layout.Context) layout.Dimensions {
			if !ready {
				gtx = gtx.Disabled()
			}
			choice := &f.presets[preset]
			if preset == int(policyNow) && d.openingFocus && gtx.Enabled() {
				d.openingFocus = false
				gtx.Execute(key.FocusCmd{Tag: choice})
			}
			if gtx.Enabled() && choice.Clicked(gtx) && fundsPolicyPreset(preset) != selected {
				f.load(simpleFundsPolicy(fundsPolicyPreset(preset), f.value))
				d.problem = i18n.NoMessage
				gtx.Execute(op.InvalidateCmd{})
			}
			variant := ui.ButtonOutline
			if fundsPolicyPreset(preset) == selected {
				variant = ui.ButtonSoft
			}
			return view.ButtonOf(choice, ui.ButtonSpec{Variant: variant, Toggle: true, Selected: fundsPolicyPreset(preset) == selected,
				Fill: true, Name: l.Text(copy.title), Description: l.Text(copy.hint), Content: func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space2)}.Layout(gtx,
						layout.Rigid(view.Body(ui.TextLabel, l.Text(copy.title), view.Colors.Ink)),
						layout.Rigid(view.Body(ui.TextCaption, l.Text(copy.hint), view.Colors.SecondaryInk)))
				}})(gtx)
		})
	}
	dateField := func(control *ui.DatePicker, label i18n.MessageID, value *string, change func(*string)) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			week := time.Monday
			if view.WeekStart == settings.WeekStartSunday {
				week = time.Sunday
			}
			control.Layout(gtx, view, ui.PickerField{Label: label, Origin: *origin, Disabled: !ready},
				ui.DateOptions{Today: today, AllowFuture: true, Clearable: true, WeekStart: week, Format: view.DateFormat},
				productString(value), func(day string) { change(productOptional(day)) })
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
		}
	}
	setUnlock := func(value *string) { f.value.UnlockOn = value }
	if selected == policyDate {
		fields = append(fields, dateField(&f.quickUnlock, i18n.FundsPolicyQuickDate, f.value.UnlockOn, setUnlock))
	}
	fields = append(fields, view.Body(ui.TextCaption, l.Text(i18n.FundsPolicyQuickHint), view.Colors.SecondaryInk), func(gtx layout.Context) layout.Dimensions {
		if f.toggle.Clicked(gtx) {
			f.advanced = !f.advanced
			gtx.Execute(op.InvalidateCmd{})
		}
		label := i18n.FundsPolicyAdvanced
		if selected == policyCustom && f.value.AccessKind != string(domain.AccessUnknown) {
			label = i18n.FundsPolicyCustom
		}
		return view.TabButton(&f.toggle, l.Text(label), f.advanced)(gtx)
	})
	if !f.advanced {
		return fields
	}
	textField := func(control *ui.TextField, label i18n.MessageID, money bool) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			options := ui.FieldOptions{Label: label, Disabled: !ready}
			if money {
				return control.LayoutCurrency(gtx, view, options, d.currency)
			}
			return control.Layout(gtx, view, options)
		}
	}
	selectField := func(control *ui.Select, label i18n.MessageID, choices []productFormChoice, value string, change func(string)) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			options := make([]ui.Choice, 0, len(choices))
			for _, choice := range choices {
				options = append(options, ui.Choice{ID: choice.value, Label: l.Text(choice.label)})
			}
			control.Layout(gtx, view, ui.PickerField{Label: label, Origin: *origin, Disabled: !ready}, ui.SelectOptions{Items: options}, value, change)
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
		}
	}
	calendar := func(value *string) string {
		if value == nil {
			return string(domain.DayBasisCalendar)
		}
		return *value
	}
	fields = append(fields,
		textField(&f.cap, i18n.FundsPolicyCap, true), view.Body(ui.TextCaption, l.Text(i18n.FundsPolicyCapHint), view.Colors.SecondaryInk),
		selectField(&f.access, i18n.ProductEditAccess, productAccessChoices, f.value.AccessKind, func(value string) { f.value.AccessKind = value }))
	if f.value.AccessKind == string(domain.AccessOnDate) || f.value.AccessKind == string(domain.AccessOnRequest) {
		fields = append(fields, dateField(&f.unlock, i18n.ProductEditUnlock, f.value.UnlockOn, setUnlock))
	}
	fields = append(fields,
		textField(&f.days, i18n.ProductEditDays, false),
		selectField(&f.basis, i18n.ProductEditDayBasis, productBasisChoices, calendar(f.value.DayBasis), func(value string) { f.value.DayBasis = &value }),
		dateField(&f.receipt, i18n.ProductEditReceipt, f.value.ReceiptOnOverride, func(value *string) { f.value.ReceiptOnOverride = value }),
		textField(&f.fee, i18n.ProductEditExitFee, true), view.Body(ui.TextCaption, l.Text(i18n.ProductEditMissingPolicy), view.Colors.SecondaryInk),
		selectField(&f.early, i18n.ProductEditEarly, productEarlyChoices, f.value.EarlyKind, func(value string) {
			f.value.EarlyKind = value
			if f.value.EarlyDayBasis == nil {
				f.value.EarlyDayBasis = productOptional(string(domain.DayBasisCalendar))
			}
			if f.value.EarlyAmountMode == nil {
				f.value.EarlyAmountMode = productOptional(string(domain.EarlyAmountCurrentValue))
			}
		}))
	if f.value.EarlyKind == string(domain.EarlyAllowed) {
		mode := productString(f.value.EarlyAmountMode)
		fields = append(fields,
			textField(&f.earlyDays, i18n.ProductEditEarlyDays, false),
			selectField(&f.earlyBasis, i18n.ProductEditEarlyBasis, productBasisChoices, calendar(f.value.EarlyDayBasis), func(value string) { f.value.EarlyDayBasis = &value }),
			textField(&f.earlyFee, i18n.ProductEditEarlyFee, true),
			selectField(&f.earlyMode, i18n.ProductEditEarlyMode, productEarlyModeChoices, mode, func(value string) {
				f.value.EarlyAmountMode = &value
				f.gross.Editor.SetText("")
			}))
		if mode == string(domain.EarlyAmountFixedGross) {
			fields = append(fields, textField(&f.gross, i18n.ProductEditEarlyGross, true))
		}
	}
	if calendar(f.value.DayBasis) == string(domain.DayBasisWeekdays) || f.value.EarlyKind == string(domain.EarlyAllowed) && calendar(f.value.EarlyDayBasis) == string(domain.DayBasisWeekdays) {
		fields = append(fields, view.Body(ui.TextCaption, l.Text(i18n.ProductEditWeekdayHelp), view.Colors.SecondaryInk))
	}
	return fields
}
