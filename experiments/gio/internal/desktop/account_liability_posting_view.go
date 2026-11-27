package desktop

import (
	"fmt"
	"image"
	"strings"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

var liabilityPostingKinds = [...]struct {
	kind  historyapi.ChangeCommandKind
	label i18n.MessageID
}{
	{historyapi.ChangeMoneyAdded, i18n.LiabilityDebtIncrease},
	{historyapi.ChangeMoneyRemoved, i18n.LiabilityDebtDecrease},
	{historyapi.ChangeDebtDraw, i18n.HistoryDebtDraw},
	{historyapi.ChangeDebtPayment, i18n.HistoryDebtPayment},
	{historyapi.ChangeValueUpdate, i18n.LiabilityBalanceCorrection},
}

var liabilityReasonLabels = map[domain.ActivityReason]i18n.MessageID{
	domain.ReasonCardPurchase:   i18n.LiabilityCardPurchase,
	domain.ReasonCardRefund:     i18n.LiabilityCardRefund,
	domain.ReasonInterest:       i18n.HistoryCashReasonInterest,
	domain.ReasonFee:            i18n.HistoryCashReasonFee,
	domain.ReasonOther:          i18n.HistoryCashReasonOther,
	domain.ReasonReconciliation: i18n.LiabilityReasonReconciliation,
}

func (p *accountsPage) layoutLiabilityPosting(gtx layout.Context, view ui.View) layout.Dimensions {
	f, l := p.liabilityPosting, view.Language
	d := f.draft
	if d.starting != nil {
		dims := f.history.layoutStartingPoint(gtx, view, d.starting)
		if !d.starting.open {
			view.RestorePageScroll(0)
		}
		return dims
	}
	if d.startingSaved && d.focusAmount {
		view.RestorePageScroll(0)
	}
	if !p.liabilityPostingUnresolved() && d.cancel.Clicked(gtx) {
		f.draft = nil
		f.restore = f.entry == &f.open
		p.restoreAccountRecord(f.entry)
		view.RestorePageScroll(0)
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if d.review != nil {
		return p.layoutLiabilityReview(gtx, view)
	}
	if d.sourceProblem != i18n.NoMessage && d.retry.Clicked(gtx) {
		p.loadLiabilityPosting()
	}
	if d.sources != nil && d.loading == nil && d.previewing == nil {
		if d.useNow.Clicked(gtx) {
			d.captureTime(p.clock())
			d.problem = i18n.NoMessage
		}
		if d.preview.Clicked(gtx) {
			p.previewLiabilityPosting()
		}
	}
	gtx, view = view.Measure(gtx, 660)
	at := view.Origin
	children := []layout.Widget{
		tradeTitle(view, liabilityEntryLabel(d.account)),
		view.Message(d.account.Name+" · "+d.account.DefaultCurrency.String(), view.Colors.Ink),
	}
	if d.startingSaved {
		children = append(children, view.Message(l.Text(i18n.HistoryStartSaved), view.Colors.Ink))
	}
	if d.sources != nil {
		children = append(children, func(gtx layout.Context) layout.Dimensions {
			view.Origin = at
			return p.layoutLiabilityFields(view)(gtx)
		})
	}
	if d.loading != nil || d.previewing != nil {
		children = append(children, view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk))
	} else if d.sourceProblem != i18n.NoMessage {
		children = append(children, view.Message(l.Text(d.sourceProblem), view.Colors.ErrorInk))
	} else if d.problem != i18n.NoMessage {
		children = append(children, view.Message(l.Text(d.problem), view.Colors.ErrorInk))
	}
	buttons := []layout.Widget{view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel))}
	if d.sourceProblem != i18n.NoMessage && (d.sourceProblem != i18n.CashMovementHistoryRequired || d.startingSaved) {
		buttons = append(buttons, view.Button(&d.retry, l.Text(i18n.HistoryRetry)))
	}
	if d.sources != nil && d.sourceProblem == i18n.NoMessage {
		buttons = append(buttons, func(gtx layout.Context) layout.Dimensions {
			if d.loading != nil || d.previewing != nil || strings.TrimSpace(d.amount.Text()) == "" || (d.debt() && d.sources.Counterparty(d.counterparty).ID == "") {
				gtx = gtx.Disabled()
			}
			return view.PrimaryButton(&d.preview, l.Text(i18n.LiabilityPreview))(gtx)
		})
	}
	children = append(children, liabilityButtons(buttons...))
	return layoutLiabilityColumn(gtx, &at, children)
}

func (p *accountsPage) layoutLiabilityFields(view ui.View) layout.Widget {
	d, l := p.liabilityPosting.draft, view.Language
	return func(gtx layout.Context) layout.Dimensions {
		if d.loading != nil || d.previewing != nil {
			gtx = gtx.Disabled()
		}
		at := view.Origin
		choices := make([]layout.Widget, 0, len(liabilityPostingKinds))
		for i, option := range liabilityPostingKinds {
			if !d.account.IsLiability() && option.kind != historyapi.ChangeDebtDraw && option.kind != historyapi.ChangeDebtPayment {
				continue
			}
			if d.kinds[i].Clicked(gtx) {
				d.selectKind(option.kind)
				gtx.Execute(op.InvalidateCmd{})
			}
			label := option.label
			if d.account.AccountType == domain.TypeCreditCard {
				if option.kind == historyapi.ChangeMoneyAdded {
					label = i18n.LiabilityCardPurchase
				} else if option.kind == historyapi.ChangeMoneyRemoved {
					label = i18n.LiabilityCardRefund
				}
			}
			choices = append(choices, view.TabButton(&d.kinds[i], l.Text(label), d.kind == option.kind))
		}
		children := []layout.Widget{liabilityButtons(choices...)}
		if d.debt() {
			children = append(children, func(gtx layout.Context) layout.Dimensions {
				items := make([]ui.Choice, 0, len(d.sources.Counterparties))
				for _, account := range d.sources.Counterparties {
					items = append(items, ui.Choice{ID: account.ID.String(), Label: account.Name + " · " + account.DefaultCurrency.String(), SearchText: account.Name})
				}
				label, empty := i18n.LiabilityChooseDebt, i18n.LiabilityNoDebt
				if d.account.IsLiability() {
					label, empty = i18n.LiabilityChooseCash, i18n.LiabilityNoCash
				}
				if len(items) == 0 {
					return view.Message(l.Text(empty), view.Colors.SecondaryInk)(gtx)
				}
				selected := d.sources.Counterparty(d.counterparty)
				if d.focusAmount && selected.ID == "" && gtx.Enabled() {
					gtx.Execute(key.FocusCmd{Tag: &d.accountPicker.Trigger})
					d.focusAmount = false
				}
				field := ui.PickerField{Label: label, Origin: at, Disabled: !gtx.Enabled(), Next: &d.amount.Editor}
				d.accountPicker.Layout(gtx, view, field, ui.SelectOptions{Items: items, Searchable: true}, selected.ID.String(), func(value string) {
					d.counterparty, d.problem = domain.AccountID(value), i18n.NoMessage
				})
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
			})
		}
		children = append(children, d.layoutLiabilityAmounts(view, &at))
		reasons := history.LiabilityReasons(d.kind)
		if len(reasons) > 0 {
			children = append(children, func(gtx layout.Context) layout.Dimensions {
				if len(reasons) == 1 {
					return view.Message(liabilityReasonFact(l, reasons[0]), view.Colors.Ink)(gtx)
				}
				items := make([]ui.Choice, 0, len(reasons))
				for _, reason := range reasons {
					items = append(items, ui.Choice{ID: string(reason), Label: l.Text(liabilityReasonLabels[reason])})
				}
				field := ui.PickerField{Label: i18n.HistoryRepeatReason, Origin: at, Disabled: !gtx.Enabled(),
					Previous: &d.currencyPicker.Trigger, Next: &d.datePicker.Trigger}
				d.reasonPicker.Layout(gtx, view, field, ui.SelectOptions{Items: items}, string(d.reason), func(value string) {
					d.reason, d.problem = domain.ActivityReason(value), i18n.NoMessage
				})
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
			})
		}
		children = append(children, func(gtx layout.Context) layout.Dimensions {
			return d.layoutLiabilityTime(gtx, view, at, p.clock())
		})
		timeHint := fmt.Sprintf(l.Text(i18n.ContributionTimezone), d.sources.Location)
		if !d.exactTime.IsZero() {
			timeHint = tradeTimestamp(view, d.exactTime, d.sources.Location)
		}
		children = append(children,
			view.Message(timeHint, view.Colors.SecondaryInk),
			view.Button(&d.useNow, l.Text(i18n.LiabilityUseNow)),
			func(gtx layout.Context) layout.Dimensions {
				return d.note.Layout(gtx, view, ui.FieldOptions{Label: i18n.PreviewNotes, Hint: i18n.PreviewNotesHint})
			},
		)
		return layoutLiabilityColumn(gtx, &at, children)
	}
}

func liabilityReasonFact(l i18n.Language, reason domain.ActivityReason) string {
	return l.Text(i18n.HistoryRepeatReason) + " · " + l.Text(liabilityReasonLabels[reason])
}

func layoutLiabilityColumn(gtx layout.Context, at *image.Point, children []layout.Widget) layout.Dimensions {
	rows := make([]layout.FlexChild, 0, len(children))
	for _, child := range children {
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, at, child)
		}))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, rows...)
}

func (d *liabilityPostingDraft) layoutLiabilityAmounts(view ui.View, at *image.Point) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		view.Origin = *at
		if d.focusAmount && gtx.Enabled() {
			d.amount.Focus(gtx)
			d.focusAmount = false
		}
		currency, amountLabel, count := d.currency, i18n.HistoryRepeatAmount, 2
		if d.debt() {
			currency, amountLabel = d.account.DefaultCurrency, i18n.HistoryDebtPrincipal
			if cash := d.sources.Counterparty(d.counterparty); d.account.IsLiability() && cash.ID != "" {
				currency = cash.DefaultCurrency
			}
			if d.kind == historyapi.ChangeDebtDraw {
				count = 1
			}
		} else if d.kind == historyapi.ChangeValueUpdate {
			amountLabel, count = i18n.AccountCardBalanceTarget, 1
		}
		return (ui.Grid{MinColumn: 160, MaxColumns: 2, Gap: 8}).Layout(gtx, view, count, func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
			if i == 0 {
				field := ui.FieldOptions{Label: amountLabel}
				if d.debt() || d.kind == historyapi.ChangeValueUpdate {
					return d.amount.LayoutCurrency(gtx, view, field, currency.String())
				}
				return d.amount.Layout(gtx, view, field)
			}
			if d.debt() {
				return d.fee.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.LiabilityInterest}, currency.String())
			}
			var items []ui.Choice
			for _, code := range domain.SupportedCurrencies() {
				items = append(items, ui.Choice{ID: code.String(), Label: code.String()})
			}
			field := ui.PickerField{Label: i18n.AnalysisCurrency, Origin: view.Origin, Disabled: !gtx.Enabled(), Next: &d.reasonPicker.Trigger}
			d.currencyPicker.Layout(gtx, view, field, ui.SelectOptions{Items: items, Searchable: true}, currency.String(), func(value string) {
				d.currency, d.problem = domain.CurrencyCode(value), i18n.NoMessage
			})
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
		})
	}
}

func (d *liabilityPostingDraft) layoutLiabilityTime(gtx layout.Context, view ui.View, at image.Point, now time.Time) layout.Dimensions {
	view.Origin = at
	zone := d.sources.Location
	today := now.In(zone).Format(time.DateOnly)
	weekStart := time.Monday
	if view.WeekStart == "sunday" {
		weekStart = time.Sunday
	}
	dates := ui.DateOptions{Min: d.sources.Origin.StartedAt.In(zone).Format(time.DateOnly), Today: today, Max: today, WeekStart: weekStart, Format: view.DateFormat}
	return (ui.Grid{MinColumn: 160, MaxColumns: 2, Gap: 8}).Layout(gtx, view, 2, func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
		field := ui.PickerField{Origin: view.Origin, Disabled: !gtx.Enabled()}
		if i == 0 {
			field.Label, field.Next = i18n.PreviewEffectiveDate, &d.timePicker.Trigger
			if !d.debt() {
				field.Previous = &d.amount.Editor
				if d.kind != historyapi.ChangeValueUpdate {
					field.Previous = &d.reasonPicker.Trigger
				}
			}
			d.datePicker.Layout(gtx, view, field, dates, d.date, func(day string) {
				d.date, d.exactTime, d.problem = day, time.Time{}, i18n.NoMessage
			})
		} else {
			field.Label, field.Previous, field.Next = i18n.PreviewEffectiveTime, &d.datePicker.Trigger, &d.useNow
			d.timePicker.Layout(gtx, view, field, d.time, view.TimeFormat == settings.TimeFormat12, false, func(clock string) {
				d.time, d.exactTime, d.problem = clock, time.Time{}, i18n.NoMessage
			})
		}
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
	})
}

func (p *accountsPage) layoutLiabilityReview(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.liabilityPosting.draft, view.Language
	if !p.liabilityPostingUnresolved() && d.edit.Clicked(gtx) {
		d.review, d.focusAmount = nil, true
		view.RestorePageScroll(0)
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	confirm := &d.confirm
	if d.uncertain {
		confirm = &d.retryConfirm
	}
	if d.posting == nil && confirm.Clicked(gtx) {
		p.confirmLiabilityPosting()
	}
	if d.focusReview && gtx.Enabled() {
		view.RestorePageScroll(0)
		gtx.Execute(key.FocusCmd{Tag: confirm})
		d.focusReview = false
	}
	gtx, view = view.Measure(gtx, 660)
	review := d.review
	activity := review.Preview.Activity
	title := historyKindLabel(activity.Kind, activity.Reason)
	if !d.debt() {
		title = liabilityKindLabel(activity.Kind, activity.Reason)
	}
	children := []layout.FlexChild{
		layout.Rigid(tradeTitle(view, title)),
		layout.Rigid(view.Message(tradeTimestamp(view, activity.EffectiveAt, review.Timezone), view.Colors.SecondaryInk)),
	}
	if !d.debt() {
		children = append(children, layout.Rigid(view.Message(liabilityReasonFact(l, activity.Reason), view.Colors.Ink)))
	}
	for _, effect := range review.Preview.Effects {
		if effect.Role == domain.EffectRoleDebt || effect.Money == nil || d.kind == historyapi.ChangeValueUpdate {
			continue
		}
		label := i18n.LiabilityAmountFact
		if effect.Role == domain.EffectRoleFee {
			label = i18n.LiabilityInterestFact
		}
		money := effect.Money
		children = append(children, layout.Rigid(view.Message(fmt.Sprintf(l.Text(label), portfolioDecimal(view, money.CanonicalAmount())+" "+money.Currency().String()), view.Colors.Ink)))
	}
	for _, endpoint := range review.Preview.Review {
		money := func(value string) string { return portfolioDecimal(view, value) + " " + endpoint.Currency.String() }
		line := fmt.Sprintf(l.Text(i18n.LiabilityBalanceReview), endpoint.AccountName, money(endpoint.Before), money(endpoint.After))
		children = append(children, layout.Rigid(view.Message(line, view.Colors.Ink)))
	}
	if activity.Note != nil {
		children = append(children, layout.Rigid(view.Message(*activity.Note, view.Colors.Ink)))
	}
	switch {
	case d.posting != nil:
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.SettingsPending), view.Colors.SecondaryInk)))
	case d.uncertain:
		children = append(children,
			layout.Rigid(view.Message(l.Text(i18n.AccountCashUncertain), view.Colors.ErrorInk)),
			layout.Rigid(view.PrimaryButton(confirm, l.Text(i18n.ContributionRetryPost))),
		)
	default:
		children = append(children, layout.Rigid(liabilityButtons(
			view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel)),
			view.Button(&d.edit, l.Text(i18n.AccountCreateBackToDetails)),
			view.PrimaryButton(confirm, l.Text(i18n.LiabilityConfirm)),
		)))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}

func liabilityButtons(buttons ...layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return (ui.Wrap{Gap: 8}).Layout(gtx, len(buttons), func(gtx layout.Context, i int) layout.Dimensions {
			return buttons[i](gtx)
		})
	}
}
