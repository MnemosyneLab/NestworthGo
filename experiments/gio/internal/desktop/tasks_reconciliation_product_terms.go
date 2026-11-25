package desktop

import (
	"image"
	"strconv"
	"strings"
	"time"

	"gioui.org/layout"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type holdingProductTerms struct {
	current                                                *liquidity.ProductDTO
	terms                                                  application.ProductTermsInput
	policy                                                 application.ProductPolicyInput
	name, note, days, fee, earlyDays, earlyFee, earlyGross ui.TextField
	start, maturity, unlock, receipt                       ui.DatePicker
	access, basis, early, earlyBasis, earlyMode            ui.Select
}

func newHoldingProductTerms() *holdingProductTerms {
	return &holdingProductTerms{
		name: ui.NewTextField(true), note: ui.NewTextField(true), days: ui.NewTextField(true), fee: ui.NewTextField(true),
		earlyDays: ui.NewTextField(true), earlyFee: ui.NewTextField(true), earlyGross: ui.NewTextField(true),
	}
}

func productTermInputs(p liquidity.ProductDTO) (application.ProductTermsInput, application.ProductPolicyInput) {
	amount := func(m *wire.MoneyView) *string {
		if m == nil {
			return nil
		}
		return &m.Amount
	}
	r := p.Policy
	return application.ProductTermsInput{Kind: p.Kind, Name: p.Name, ShortName: &p.ShortName, Note: p.Note, StartOn: p.StartOn,
			MaturityOn: p.MaturityOn, InterestMode: p.InterestMode, AnnualRate: p.AnnualRate, MaturityInterest: amount(p.MaturityInterest),
			InterestPaidThroughOn: p.InterestPaidThroughOn},
		application.ProductPolicyInput{AccessibleAmountCap: amount(r.AccessibleAmountCap), AccessKind: r.AccessKind, UnlockOn: r.UnlockOn,
			SettlementDays: r.SettlementDays, DayBasis: r.DayBasis, ReceiptOnOverride: r.ReceiptOnOverride, NormalExitFee: amount(r.NormalExitFee),
			EarlyKind: r.EarlyKind, EarlySettlementDays: r.EarlySettlementDays, EarlyDayBasis: r.EarlyDayBasis, EarlyFee: amount(r.EarlyFee),
			EarlyAmountMode: r.EarlyAmountMode, EarlyGrossAmount: amount(r.EarlyGrossAmount), Note: r.Note}
}

func (f *holdingProductTerms) receive(detail application.ProductDetail) {
	next := liquidity.FromProductDetail(detail).Product
	initialized := f.current != nil
	f.current = &next
	if initialized {
		return
	}
	f.terms, f.policy = productTermInputs(next)
	f.name.Editor.SetText(f.terms.Name)
	f.note.Editor.SetText(productString(f.terms.Note))
	f.days.Editor.SetText(productDays(f.policy.SettlementDays))
	f.fee.Editor.SetText(productString(f.policy.NormalExitFee))
	f.earlyDays.Editor.SetText(productDays(f.policy.EarlySettlementDays))
	f.earlyFee.Editor.SetText(productString(f.policy.EarlyFee))
	f.earlyGross.Editor.SetText(productString(f.policy.EarlyGrossAmount))
}

func productString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func productOptional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func productDays(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

func parseProductDays(value string) (*int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, true
	}
	days, err := strconv.Atoi(value)
	return &days, err == nil
}

func (f *holdingProductTerms) inputs() (application.ProductTermsInput, application.ProductPolicyInput, bool) {
	terms, policy := f.terms, f.policy
	if f.current.State != string(domain.ProductStateOpen) {
		terms, policy = productTermInputs(*f.current)
		terms.Name, terms.Note = f.name.Text(), productOptional(f.note.Text())
		return terms, policy, true
	}
	terms.Name, terms.Note = f.name.Text(), productOptional(f.note.Text())
	days, ok := parseProductDays(f.days.Text())
	early, earlyOK := parseProductDays(f.earlyDays.Text())
	policy.SettlementDays = days
	if days != nil && policy.DayBasis == nil {
		policy.DayBasis = productOptional(string(domain.DayBasisCalendar))
	}
	policy.NormalExitFee = productOptional(f.fee.Text())
	if policy.EarlyKind != string(domain.EarlyAllowed) {
		return terms, policy, ok
	}
	policy.EarlySettlementDays = early
	policy.EarlyFee = productOptional(f.earlyFee.Text())
	if productString(policy.EarlyAmountMode) == string(domain.EarlyAmountFixedGross) {
		policy.EarlyGrossAmount = productOptional(f.earlyGross.Text())
	}
	return terms, policy, ok && earlyOK
}

type productFormChoice struct {
	value string
	label i18n.MessageID
}

var productAccessChoices = []productFormChoice{
	{string(domain.AccessOnRequest), i18n.ProductEditOnRequest}, {string(domain.AccessOnDate), i18n.ProductEditOnDate},
	{string(domain.AccessUnknown), i18n.ProductEditUnknownAccess}, {string(domain.AccessExcluded), i18n.ProductEditExcluded},
}
var productBasisChoices = []productFormChoice{
	{string(domain.DayBasisCalendar), i18n.ProductEditCalendar}, {string(domain.DayBasisWeekdays), i18n.ProductEditWeekdays},
}
var productEarlyChoices = []productFormChoice{
	{string(domain.EarlyNotAllowed), i18n.ProductEditEarlyNo}, {string(domain.EarlyAllowed), i18n.ProductEditEarlyYes}, {string(domain.EarlyUnknown), i18n.ProductEditEarlyUnknown},
}
var productEarlyModeChoices = []productFormChoice{
	{string(domain.EarlyAmountCurrentValue), i18n.ProductEditEarlyCurrent}, {string(domain.EarlyAmountFixedGross), i18n.ProductEditEarlyGross},
}

func (c *holdingReconciliation) productTermsFields(view ui.View, origin *image.Point, d *holdingReconDraft, f *holdingProductTerms, ready, editing bool) []layout.Widget {
	if f.current == nil {
		return nil
	}
	l := view.Language
	text := func(field *ui.TextField, label i18n.MessageID, money bool) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			if ready && field == &f.name {
				d.focusAdjustmentField(gtx, field)
			}
			opts := ui.FieldOptions{Label: label, Disabled: !ready}
			if money {
				return field.LayoutCurrency(gtx, view, opts, d.product.currency)
			}
			return field.Layout(gtx, view, opts)
		}
	}
	rows := []layout.Widget{view.Message(l.Text(i18n.ProductEditTermsHeading), view.Colors.Ink), text(&f.name, i18n.ProductEditName, false)}
	if f.current.State != string(domain.ProductStateOpen) {
		return append(rows, text(&f.note, i18n.ProductEditNote, false), view.Message(l.Text(i18n.ProductEditClosed), view.Colors.SecondaryInk))
	}
	today := ""
	if c.zone != nil {
		today = c.clock().In(c.zone).Format(time.DateOnly)
	}
	week := time.Monday
	if view.WeekStart == "sunday" {
		week = time.Sunday
	}
	date := func(control *ui.DatePicker, label i18n.MessageID, value string, disabled bool, change func(string)) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			options := ui.DateOptions{Today: today, AllowFuture: true, Clearable: true, WeekStart: week, Format: view.DateFormat}
			field := ui.PickerField{Label: label, Origin: *origin, Disabled: disabled || !ready || c.zone == nil}
			control.Layout(gtx, view, field, options, value, change)
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
		}
	}
	selectField := func(control *ui.Select, label i18n.MessageID, options []productFormChoice, value string, change func(string)) layout.Widget {
		choices := make([]ui.Choice, 0, len(options))
		for _, option := range options {
			choices = append(choices, ui.Choice{ID: option.value, Label: l.Text(option.label)})
		}
		return func(gtx layout.Context) layout.Dimensions {
			control.Layout(gtx, view, ui.PickerField{Label: label, Origin: *origin, Disabled: !ready}, ui.SelectOptions{Items: choices}, value, change)
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
		}
	}
	rows = append(rows, date(&f.start, i18n.ProductEditStart, f.terms.StartOn, editing, func(day string) { f.terms.StartOn = day }),
		date(&f.maturity, i18n.ProductEditMaturity, productString(f.terms.MaturityOn), false, func(day string) { f.terms.MaturityOn = productOptional(day) }),
		text(&f.note, i18n.ProductEditNote, false), view.Message(l.Text(i18n.ProductEditPolicy), view.Colors.Ink),
		selectField(&f.access, i18n.ProductEditAccess, productAccessChoices, f.policy.AccessKind, func(value string) { f.policy.AccessKind = value }))
	if f.policy.AccessKind == string(domain.AccessOnDate) || f.policy.AccessKind == string(domain.AccessOnRequest) {
		rows = append(rows, date(&f.unlock, i18n.ProductEditUnlock, productString(f.policy.UnlockOn), false, func(day string) { f.policy.UnlockOn = productOptional(day) }))
	}
	basis := productString(f.policy.DayBasis)
	if basis == "" {
		basis = string(domain.DayBasisCalendar)
	}
	rows = append(rows, text(&f.days, i18n.ProductEditDays, false),
		selectField(&f.basis, i18n.ProductEditDayBasis, productBasisChoices, basis, func(value string) { f.policy.DayBasis = &value }),
		date(&f.receipt, i18n.ProductEditReceipt, productString(f.policy.ReceiptOnOverride), false, func(day string) { f.policy.ReceiptOnOverride = productOptional(day) }),
		text(&f.fee, i18n.ProductEditExitFee, true), view.Message(l.Text(i18n.ProductEditMissingPolicy), view.Colors.SecondaryInk),
		selectField(&f.early, i18n.ProductEditEarly, productEarlyChoices, f.policy.EarlyKind, func(value string) {
			f.policy.EarlyKind = value
			if f.policy.EarlyDayBasis == nil {
				f.policy.EarlyDayBasis = productOptional(string(domain.DayBasisCalendar))
			}
			if f.policy.EarlyAmountMode == nil {
				f.policy.EarlyAmountMode = productOptional(string(domain.EarlyAmountCurrentValue))
			}
		}))
	if f.policy.EarlyKind == string(domain.EarlyAllowed) {
		rows = append(rows, text(&f.earlyDays, i18n.ProductEditEarlyDays, false),
			selectField(&f.earlyBasis, i18n.ProductEditEarlyBasis, productBasisChoices, productString(f.policy.EarlyDayBasis), func(value string) { f.policy.EarlyDayBasis = &value }),
			text(&f.earlyFee, i18n.ProductEditEarlyFee, true),
			selectField(&f.earlyMode, i18n.ProductEditEarlyMode, productEarlyModeChoices, productString(f.policy.EarlyAmountMode), func(value string) {
				f.policy.EarlyAmountMode = &value
				f.policy.EarlyGrossAmount = nil
				f.earlyGross.Editor.SetText("")
			}))
		if productString(f.policy.EarlyAmountMode) == string(domain.EarlyAmountFixedGross) {
			rows = append(rows, text(&f.earlyGross, i18n.ProductEditEarlyGross, true))
		}
	}
	if basis == string(domain.DayBasisWeekdays) || f.policy.EarlyKind == string(domain.EarlyAllowed) && productString(f.policy.EarlyDayBasis) == string(domain.DayBasisWeekdays) {
		rows = append(rows, view.Message(l.Text(i18n.ProductEditWeekdayHelp), view.Colors.SecondaryInk))
	}
	return rows
}

func holdingProductTermsReviewRows(view ui.View, before, after liquidity.ProductDTO) []layout.Widget {
	l := view.Language
	text := func(value string) string {
		if value == "" {
			return l.Text(i18n.OverviewNoValue)
		}
		return value
	}
	money := func(value *wire.MoneyView) string {
		if value == nil {
			return ""
		}
		return holdingSnapshotValue(view, value.Amount, value.Currency)
	}
	choice := func(value string, options []productFormChoice) string {
		for _, o := range options {
			if o.value == value {
				return l.Text(o.label)
			}
		}
		return ""
	}
	date := func(value *string) string {
		if value == nil {
			return ""
		}
		return historyDay(view, *value)
	}
	rows := []layout.Widget{view.Message(l.Text(i18n.ProductEditAfter), view.Colors.SecondaryInk)}
	fact := func(label i18n.MessageID, old, next string) {
		value := text(next)
		if old != next {
			value = text(old) + " → " + value
		}
		rows = append(rows, view.Message(l.Text(label)+" · "+value, view.Colors.Ink))
	}
	fact(i18n.ProductEditName, before.Name, after.Name)
	fact(i18n.ProductEditStart, historyDay(view, before.StartOn), historyDay(view, after.StartOn))
	fact(i18n.ProductEditMaturity, date(before.MaturityOn), date(after.MaturityOn))
	fact(i18n.ProductEditNote, productString(before.Note), productString(after.Note))
	b, a := before.Policy, after.Policy
	fact(i18n.ProductEditAccess, choice(b.AccessKind, productAccessChoices), choice(a.AccessKind, productAccessChoices))
	fact(i18n.ProductEditUnlock, date(b.UnlockOn), date(a.UnlockOn))
	fact(i18n.ProductEditDays, productDays(b.SettlementDays), productDays(a.SettlementDays))
	fact(i18n.ProductEditDayBasis, choice(productString(b.DayBasis), productBasisChoices), choice(productString(a.DayBasis), productBasisChoices))
	fact(i18n.ProductEditReceipt, date(b.ReceiptOnOverride), date(a.ReceiptOnOverride))
	fact(i18n.ProductEditExitFee, money(b.NormalExitFee), money(a.NormalExitFee))
	fact(i18n.ProductEditEarly, choice(b.EarlyKind, productEarlyChoices), choice(a.EarlyKind, productEarlyChoices))
	if b.EarlyKind == string(domain.EarlyAllowed) || a.EarlyKind == string(domain.EarlyAllowed) {
		fact(i18n.ProductEditEarlyDays, productDays(b.EarlySettlementDays), productDays(a.EarlySettlementDays))
		fact(i18n.ProductEditEarlyBasis, choice(productString(b.EarlyDayBasis), productBasisChoices), choice(productString(a.EarlyDayBasis), productBasisChoices))
		fact(i18n.ProductEditEarlyFee, money(b.EarlyFee), money(a.EarlyFee))
		fact(i18n.ProductEditEarlyMode, choice(productString(b.EarlyAmountMode), productEarlyModeChoices), choice(productString(a.EarlyAmountMode), productEarlyModeChoices))
		if b.EarlyGrossAmount != nil || a.EarlyGrossAmount != nil {
			fact(i18n.ProductEditEarlyGross, money(b.EarlyGrossAmount), money(a.EarlyGrossAmount))
		}
	}
	return rows
}
