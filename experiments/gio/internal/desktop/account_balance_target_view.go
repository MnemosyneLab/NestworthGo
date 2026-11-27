package desktop

import (
	"fmt"
	"image"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type balanceTargetCopy struct {
	action, review, confirm, target, noChange, invalidTarget, previewFailed, saved i18n.MessageID
}

func balanceTargetCopyFor(account domain.Account) balanceTargetCopy {
	if account.AccountType == domain.TypeCreditCard {
		return balanceTargetCopy{
			action: i18n.AccountCardBalanceAction, review: i18n.AccountCardBalanceReview,
			confirm: i18n.AccountCardBalanceConfirm, target: i18n.AccountCardBalanceTarget,
			noChange: i18n.AccountCardBalanceNoChange, invalidTarget: i18n.AccountCardBalanceInvalidTarget,
			previewFailed: i18n.AccountCardBalancePreviewFailed, saved: i18n.AccountCardBalanceSaved,
		}
	}
	if account.TrackingMode == domain.TrackingManualValue {
		return balanceTargetCopy{
			action: i18n.AccountUpdateValue, review: i18n.AccountValueReview, confirm: i18n.AccountValueConfirm,
			target: i18n.AccountValueTarget, noChange: i18n.AccountValueNoChange,
			invalidTarget: i18n.AccountCashInvalidTarget, previewFailed: i18n.AccountValuePreviewFailed, saved: i18n.AccountValueSaved,
		}
	}
	if account.AccountType != domain.TypeBankAccount {
		return balanceTargetCopy{
			action: i18n.AccountBalanceUpdate, review: i18n.AccountBalanceReview, confirm: i18n.AccountBalanceConfirm,
			target: i18n.AccountCashTarget, noChange: i18n.AccountCashNoChange,
			invalidTarget: i18n.AccountCashInvalidTarget, previewFailed: i18n.AccountBankBalancePreviewFailed, saved: i18n.AccountBalanceSaved,
		}
	}
	return balanceTargetCopy{
		action: i18n.AccountBankBalanceAction, review: i18n.AccountBankBalanceReview,
		confirm: i18n.AccountBankBalanceConfirm, target: i18n.AccountCashTarget,
		noChange: i18n.AccountBankBalanceNoChange, invalidTarget: i18n.AccountCashInvalidTarget,
		previewFailed: i18n.AccountBankBalancePreviewFailed, saved: i18n.AccountBankBalanceSaved,
	}
}

func (p *accountsPage) layoutBalanceTarget(gtx layout.Context, view ui.View) layout.Dimensions {
	f, l := p.balanceTarget, view.Language
	d := f.draft
	copy := balanceTargetCopyFor(d.account)
	gtx, view = view.Measure(gtx, 620)
	if !p.balanceTargetUnresolved() && d.cancel.Clicked(gtx) {
		f.draft = nil
		f.restore = f.entry
		p.restoreAccountRecord(f.entry)
		gtx.Execute(key.FocusCmd{Tag: f.entry})
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if d.review != nil {
		return p.layoutBalanceTargetReview(gtx, view)
	}
	preview := d.previewing == nil && d.prepare.Clicked(gtx)
	origin := view.Origin
	fields := []layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			return view.Label(gtx, l.Text(copy.action), 19, font.Medium, view.Colors.Ink)
		},
		view.Message(d.account.Name, view.Colors.Ink),
		func(gtx layout.Context) layout.Dimensions {
			if d.focus && gtx.Enabled() {
				d.amount.Focus(gtx)
				d.focus = false
			}
			return d.amount.LayoutCurrency(gtx, view, ui.FieldOptions{Label: copy.target}, d.account.DefaultCurrency.String())
		},
		func(gtx layout.Context) layout.Dimensions { return d.layoutReason(gtx, view, origin) },
		func(gtx layout.Context) layout.Dimensions {
			return d.layoutTime(gtx, view, origin, p.current.historyStartedAt, p.current.historyZone, p.clock())
		},
		view.Message(fmt.Sprintf(l.Text(i18n.ContributionTimezone), p.current.historyZone), view.Colors.SecondaryInk),
		func(gtx layout.Context) layout.Dimensions {
			if d.useNow.Clicked(gtx) {
				d.day, d.clock, d.problem = "", "", i18n.NoMessage
				gtx.Execute(op.InvalidateCmd{})
			}
			return view.Button(&d.useNow, l.Text(i18n.LiabilityUseNow))(gtx)
		},
		func(gtx layout.Context) layout.Dimensions {
			return d.note.Layout(gtx, view, ui.FieldOptions{Label: i18n.PreviewNotes, Hint: i18n.PreviewNotesHint})
		},
	}
	children := make([]layout.FlexChild, 0, len(fields)+2)
	for _, field := range fields {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if d.previewing != nil {
				gtx = gtx.Disabled()
			}
			return fieldPosition(gtx, &origin, field)
		}))
	}
	if d.problem != i18n.NoMessage {
		children = append(children, layout.Rigid(view.Message(l.Text(d.problem), view.Colors.ErrorInk)))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
			layout.Rigid(view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel))),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if d.previewing != nil {
					return view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)(gtx)
				}
				if strings.TrimSpace(d.amount.Text()) == "" {
					gtx = gtx.Disabled()
				}
				return view.PrimaryButton(&d.prepare, l.Text(copy.review))(gtx)
			}),
		)
	}))
	dims := layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
	if preview {
		p.previewBalanceTarget()
		gtx.Execute(op.InvalidateCmd{})
	}
	return dims
}

func (d *balanceTargetDraft) layoutReason(gtx layout.Context, view ui.View, origin image.Point) layout.Dimensions {
	l := view.Language
	if d.account.IsLiability() {
		return view.Message(liabilityReasonFact(l, domain.ReasonReconciliation), view.Colors.Ink)(gtx)
	}
	var options []ui.Choice
	for _, reason := range domain.ValueUpdateReasons() {
		options = append(options, ui.Choice{ID: string(reason), Label: l.Text(checkpointCashReasonLabel(reason))})
	}
	d.reasonPicker.Layout(gtx, view, ui.PickerField{Label: i18n.HistoryRepeatReason, Origin: origin, Disabled: !gtx.Enabled(),
		Previous: &d.amount.Editor, Next: &d.datePicker.Trigger}, ui.SelectOptions{Items: options}, string(d.reason), func(id string) {
		d.reason, d.problem = domain.ActivityReason(id), i18n.NoMessage
	})
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
}

func (d *balanceTargetDraft) layoutTime(gtx layout.Context, view ui.View, origin image.Point, start time.Time, zone *time.Location, now time.Time) layout.Dimensions {
	view.Origin = origin
	local := now.In(zone)
	today := local.Format(time.DateOnly)
	day, clock := d.day, d.clock
	if day == "" && clock == "" {
		day, clock = today, local.Format("15:04")
	}
	weekStart := time.Monday
	if view.WeekStart == "sunday" {
		weekStart = time.Sunday
	}
	dates := ui.DateOptions{Min: start.In(zone).Format(time.DateOnly), Max: today, Today: today, WeekStart: weekStart, Format: view.DateFormat}
	return (ui.Grid{MinColumn: 160, MaxColumns: 2, Gap: ui.Space8}).Layout(gtx, view, 2, func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
		field := ui.PickerField{Origin: view.Origin, Disabled: !gtx.Enabled()}
		if i == 0 {
			field.Label, field.Next = i18n.PreviewEffectiveDate, &d.timePicker.Trigger
			d.datePicker.Layout(gtx, view, field, dates, day, func(picked string) {
				d.day, d.clock, d.problem = picked, clock, i18n.NoMessage
			})
		} else {
			field.Label, field.Previous, field.Next = i18n.PreviewEffectiveTime, &d.datePicker.Trigger, &d.useNow
			d.timePicker.Layout(gtx, view, field, clock, view.TimeFormat == settings.TimeFormat12, false, func(picked string) {
				d.day, d.clock, d.problem = day, picked, i18n.NoMessage
			})
		}
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
	})
}

func (p *accountsPage) layoutBalanceTargetReview(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.balanceTarget.draft, view.Language
	copy := balanceTargetCopyFor(d.account)
	if !p.balanceTargetUnresolved() && d.edit.Clicked(gtx) {
		d.review, d.focus = nil, true
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	confirm := &d.confirm
	if d.uncertain {
		confirm = &d.retryConfirm
	}
	if d.confirming == nil && confirm.Clicked(gtx) {
		p.confirmBalanceTarget()
		gtx.Execute(op.InvalidateCmd{})
	}
	plan := d.review
	help := i18n.AccountBankBalanceReconcile
	if d.account.TrackingMode == domain.TrackingManualValue {
		help = i18n.AccountValueReconcile
	}
	effective, err := targetBalanceTimestamp(view, plan.at, plan.zone)
	if err != nil {
		return view.Message(l.Text(copy.previewFailed), view.Colors.ErrorInk)(gtx)
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.Label(gtx, l.Text(copy.review), 19, font.Medium, view.Colors.Ink)
		}),
		layout.Rigid(view.Message(d.account.Name, view.Colors.Ink)),
		layout.Rigid(view.Message(l.Text(help), view.Colors.SecondaryInk)),
	}
	if d.account.IsLiability() {
		owed := fmt.Sprintf(l.Text(i18n.AccountCardBalanceOwedAfter), plan.target+" "+plan.currency.String())
		children = append(children, layout.Rigid(view.Message(owed, view.Colors.Ink)))
	}
	children = append(children, targetBalanceFacts(view, i18n.AccountCashDelta, plan.before, plan.after, plan.delta, plan.currency.String(), effective)...)
	children = append(children, layout.Rigid(view.Message(fmt.Sprintf("%s · %s",
		l.Text(i18n.HistoryRepeatReason), l.Text(checkpointCashReasonLabel(plan.reason))), view.Colors.Ink)))
	if plan.note != nil {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.PreviewNotes)+" · "+*plan.note, view.Colors.Ink)))
	}
	if d.uncertain {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.AccountCashUncertain), view.Colors.ErrorInk)))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layoutTargetBalanceConfirmation(gtx, view, d.confirming != nil, d.uncertain,
			&d.cancel, &d.edit, confirm, copy.confirm)
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}

func targetBalanceTimestamp(view ui.View, instant time.Time, zone string) (string, error) {
	location, err := time.LoadLocation(zone)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(view.Language.Text(i18n.ContributionEffective), instant.In(location).Format("2006-01-02 15:04:05.000"), location), nil
}

func targetBalanceFacts(view ui.View, deltaLabel i18n.MessageID, before, target, delta, currency, effective string) []layout.FlexChild {
	l := view.Language
	amount := func(value string) string { return value + " " + currency }
	rows := []layout.FlexChild{
		layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.ContributionBefore), amount(before)), view.Colors.Ink)),
		layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.ContributionAfter), amount(target)), view.Colors.Ink)),
	}
	if delta != "" {
		if !strings.HasPrefix(delta, "-") && !strings.HasPrefix(delta, "+") {
			delta = "+" + delta
		}
		rows = append(rows, layout.Rigid(view.Message(fmt.Sprintf(l.Text(deltaLabel), amount(delta)), view.Colors.Ink)))
	}
	return append(rows, layout.Rigid(view.Message(effective, view.Colors.SecondaryInk)))
}

func layoutTargetBalanceConfirmation(gtx layout.Context, view ui.View, pending, uncertain bool,
	cancel, edit, confirm *widget.Clickable, label i18n.MessageID) layout.Dimensions {
	l := view.Language
	if pending {
		return view.Message(l.Text(i18n.SettingsPending), view.Colors.SecondaryInk)(gtx)
	}
	if uncertain {
		return view.PrimaryButton(confirm, l.Text(i18n.ContributionRetryPost))(gtx)
	}
	return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(view.Button(cancel, l.Text(i18n.InstitutionsCancel))),
		layout.Rigid(view.Button(edit, l.Text(i18n.AccountCreateBackToDetails))),
		layout.Rigid(view.PrimaryButton(confirm, l.Text(label))),
	)
}
