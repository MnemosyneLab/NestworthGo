package desktop

import (
	"image"
	"reflect"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
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
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return (ui.Wrap{Gap: ui.Space8}).Layout(gtx, len(actions), func(gtx layout.Context, i int) layout.Dimensions { return actions[i](gtx) })
		})
	}
}

func (d *fundsPolicyDraft) formFields(gtx layout.Context, view ui.View, origin *image.Point, today string, ready bool) []layout.Widget {
	f, l := &d.fields, view.Language
	value, valid := f.input()
	selected := policyCustom
	if valid {
		selected = selectedFundsPolicy(value)
	}
	if ready {
		selected = d.selectPolicyPreset(gtx, selected)
		d.movePolicyTab(gtx, selected)
	}
	var choiceTop int
	choices := []ui.ColumnChild{ui.Rigid(func(gtx layout.Context) layout.Dimensions {
		dims := tradeTitle(view, i18n.FundsPolicyQuick)(gtx)
		choiceTop += dims.Size.Y + gtx.Dp(ui.Space8)
		return dims
	})}
	for preset, copy := range fundsPolicyPresets {
		choices = append(choices, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			choice := &f.presets[preset]
			chosen := fundsPolicyPreset(preset) == selected
			variant := ui.ButtonOutline
			if chosen {
				variant = ui.ButtonSoft
			}
			dims := view.ButtonOf(choice, ui.ButtonSpec{Variant: variant, Toggle: true, Radio: true, Selected: chosen,
				Fill: true, Name: l.Text(copy.title), Description: l.Text(copy.hint), Content: func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Start, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return fundsPolicyRadioIndicator(gtx, view, chosen)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space2)}.Layout(gtx,
								layout.Rigid(view.Body(ui.TextLabel, l.Text(copy.title), view.Colors.Ink)),
								layout.Rigid(view.Body(ui.TextCaption, l.Text(copy.hint), view.Colors.SecondaryInk)))
						}))
				}})(gtx)
			d.notePolicyFocus(choice, choiceTop, dims.Size.Y)
			choiceTop += dims.Size.Y + gtx.Dp(ui.Space8)
			return dims
		}))
	}
	fields := []layout.Widget{func(gtx layout.Context) layout.Dimensions {
		choiceTop = origin.Y
		return accessibility.Section(gtx, &f.presets, l.Text(i18n.FundsPolicyQuick), func(gtx layout.Context) layout.Dimensions {
			return (ui.Column{Gap: ui.Space8}).Layout(gtx, choices...)
		})
	}}
	dateField := func(control *ui.DatePicker, label i18n.MessageID, value *string, change func(*string)) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			week := time.Monday
			if view.WeekStart == settings.WeekStartSunday {
				week = time.Sunday
			}
			control.Layout(gtx, view, ui.PickerField{Label: label, Origin: *origin, Disabled: !ready},
				ui.DateOptions{Today: today, AllowFuture: true, Clearable: true, WeekStart: week, Format: view.DateFormat},
				productString(value), func(day string) { change(productOptional(day)) })
			height := gtx.Dp(ui.PickerHeight)
			d.notePolicyFocus(&control.Trigger, origin.Y, height)
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
		}
	}
	setUnlock := func(value *string) { f.value.UnlockOn = value }
	if selected == policyDate {
		fields = append(fields, dateField(&f.quickUnlock, i18n.FundsPolicyQuickDate, f.value.UnlockOn, setUnlock))
	}
	fields = append(fields, view.Body(ui.TextCaption, l.Text(i18n.FundsPolicyQuickHint), view.Colors.SecondaryInk), func(gtx layout.Context) layout.Dimensions {
		if ready && f.toggle.Clicked(gtx) {
			f.advanced = !f.advanced
			gtx.Execute(op.InvalidateCmd{})
		}
		label := i18n.FundsPolicyAdvanced
		if selected == policyCustom && f.value.AccessKind != string(domain.AccessUnknown) {
			label = i18n.FundsPolicyCustom
		}
		dims := analysisDisclosure(view, &f.toggle, l.Text(label), f.advanced)(gtx)
		d.notePolicyFocus(&f.toggle, origin.Y, dims.Size.Y)
		return dims
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

func fundsPolicyRadioIndicator(gtx layout.Context, view ui.View, selected bool) layout.Dimensions {
	size := image.Pt(gtx.Dp(16), gtx.Dp(16))
	bounds := image.Rectangle{Max: size}
	ink := view.Colors.SecondaryInk
	if selected {
		ink = view.Colors.Accent
		ui.FillRoundRect(gtx, bounds.Inset(gtx.Dp(4)), 4, ink)
	}
	ui.StrokeRoundRect(gtx, bounds, 8, 1, ink)
	return layout.Dimensions{Size: size}
}

// The quick choices share a selection; direction keys move focus and apply
// the newly focused preset immediately, as with the standard radio group.
func (d *fundsPolicyDraft) selectPolicyPreset(gtx layout.Context, selected fundsPolicyPreset) fundsPolicyPreset {
	f := &d.fields
	if d.openingFocus {
		d.openingFocus = false
		d.focusPolicyControl(gtx, f.presetTarget(selected))
	}
	next := selected
	for index := range f.presets {
		choice := &f.presets[index]
		if choice.Clicked(gtx) {
			next = fundsPolicyPreset(index)
		}
		for _, pressed := range ui.PressedKeys(gtx, choice, key.NameLeftArrow, key.NameRightArrow, key.NameUpArrow, key.NameDownArrow) {
			step := 1
			if pressed.Name == key.NameLeftArrow || pressed.Name == key.NameUpArrow {
				step = -1
			}
			target := (index + step + len(f.presets)) % len(f.presets)
			next = fundsPolicyPreset(target)
			d.focusPolicyControl(gtx, &f.presets[target])
		}
	}
	if next != selected {
		f.load(simpleFundsPolicy(next, f.value))
		d.problem = i18n.NoMessage
		gtx.Execute(op.InvalidateCmd{})
	}
	return next
}

func (f *fundsPolicyFields) presetTarget(selected fundsPolicyPreset) *widget.Clickable {
	if selected == policyCustom {
		selected = policyNow
	}
	return &f.presets[selected]
}

func (d *fundsPolicyDraft) movePolicyTab(gtx layout.Context, selected fundsPolicyPreset) {
	f := &d.fields
	selectedTarget, after := f.presetTarget(selected), &f.toggle
	if selected == policyDate {
		after = &f.quickUnlock.Trigger
	}
	move := func(from, next, previous *widget.Clickable) {
		filter := key.Filter{Focus: from, Name: key.NameTab}
		switch {
		case next == nil:
			filter.Required = key.ModShift
		case previous != nil:
			filter.Optional = key.ModShift
		}
		for {
			e, ok := gtx.Event(filter)
			if !ok {
				return
			}
			pressed := e.(key.Event)
			if pressed.State == key.Press {
				target := next
				if pressed.Modifiers.Contain(key.ModShift) {
					target = previous
				}
				d.focusPolicyControl(gtx, target)
			}
		}
	}
	move(&d.sheet.close, selectedTarget, nil)
	for index := range f.presets {
		move(&f.presets[index], after, &d.sheet.close)
	}
	move(after, nil, selectedTarget)
}

func (d *fundsPolicyDraft) focusPolicyControl(gtx layout.Context, target *widget.Clickable) {
	d.revealFocus, d.revealBounds = target, [2]int{}
	gtx.Execute(key.FocusCmd{Tag: target})
}

func (d *fundsPolicyDraft) notePolicyFocus(target *widget.Clickable, top, height int) {
	if d.revealFocus == target {
		d.revealBounds = [2]int{top, top + height}
	}
}
