package desktop

import (
	"fmt"
	"image"
	"time"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type movementLabels struct {
	action, title, account, amount, historyRequired, preview, previewFailed i18n.MessageID
	review, confirm, stale, posted, viewOn, delta                           i18n.MessageID
}

var cashMovementLabels = [...]movementLabels{
	history.CashContribution: {
		i18n.ContributionAdd, i18n.ContributionTitle, i18n.ContributionAccount, i18n.ContributionAmount,
		i18n.ContributionHistoryRequired, i18n.ContributionPreview, i18n.ContributionPreviewFailed,
		i18n.ContributionReview, i18n.ContributionConfirm, i18n.ContributionStale, i18n.ContributionPosted,
		i18n.ContributionViewOn, i18n.ContributionDelta,
	},
	history.CashWithdrawal: {
		i18n.WithdrawalAdd, i18n.HistoryCashOut, i18n.WithdrawalAccount, i18n.WithdrawalAmount,
		i18n.CashMovementHistoryRequired, i18n.WithdrawalPreview, i18n.CashMovementPreviewFailed,
		i18n.WithdrawalReview, i18n.WithdrawalConfirm, i18n.CashMovementStale, i18n.WithdrawalPosted,
		i18n.WithdrawalViewOn, i18n.WithdrawalDelta,
	},
	history.CashTransfer: {
		i18n.TransferAdd, i18n.HistoryCashTransfer, i18n.TransferFrom, i18n.TransferAmount,
		i18n.CashMovementHistoryRequired, i18n.TransferPreview, i18n.CashMovementPreviewFailed,
		i18n.TransferReview, i18n.TransferConfirm, i18n.CashMovementStale, i18n.TransferPosted,
		i18n.TransferViewOn, i18n.CashMovementDelta,
	},
}

func (p *historyCashMovements) layoutForm(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.draft, view.Language
	labels := cashMovementLabels[d.movement]
	gtx = ui.LimitWidth(gtx, 640)
	if d.review != nil {
		return p.layoutReview(gtx, view)
	}
	if d.cancel.Clicked(gtx) {
		p.draft = nil
		gtx.Execute(key.FocusCmd{Tag: &p.actions[d.movement]})
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	origin := view.Origin
	children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return fieldPosition(gtx, &origin, func(gtx layout.Context) layout.Dimensions {
			return view.Label(gtx, l.Text(labels.action), 19, font.Medium, view.Colors.Ink)
		})
	})}
	busy := d.loading != nil || d.previewing != nil
	ready := d.sources != nil && d.sourceError == i18n.NoMessage && len(d.sources.Accounts) > 0
	if ready {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if busy {
				gtx = gtx.Disabled()
			}
			return d.layoutContributionFields(gtx, view, origin, p.clock())
		}))
	} else if !busy && d.sourceError == i18n.NoMessage {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.ContributionNoAccounts), view.Colors.SecondaryInk)))
	}
	if d.sourceError != i18n.NoMessage {
		children = append(children, layout.Rigid(view.Message(l.Text(d.sourceError), view.Colors.ErrorInk)))
	} else if d.problem != i18n.NoMessage {
		children = append(children, layout.Rigid(view.Message(l.Text(d.problem), view.Colors.ErrorInk)))
	}
	if busy {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		buttons := []layout.FlexChild{layout.Rigid(view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel)))}
		switch {
		case d.sourceError == i18n.ContributionSourcesFailed:
			if d.retrySources.Clicked(gtx) {
				p.loadSources()
				gtx.Execute(op.InvalidateCmd{})
			}
			buttons = append(buttons, layout.Rigid(view.Button(&d.retrySources, l.Text(i18n.HistoryRetry))))
		case ready && !busy:
			if d.preview.Clicked(gtx) {
				p.preview()
				gtx.Execute(op.InvalidateCmd{})
			}
			buttons = append(buttons, layout.Rigid(view.PrimaryButton(&d.preview, l.Text(labels.preview))))
		}
		return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx, buttons...)
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}

func (d *cashMovementDraft) layoutContributionFields(gtx layout.Context, view ui.View, origin image.Point, now time.Time) layout.Dimensions {
	const pickerHeight = 60
	l := view.Language
	gap := gtx.Dp(16)
	width := (gtx.Constraints.Max.X - gap) / 2
	account, found := d.accountByID(d.account)
	labels := cashMovementLabels[d.movement]
	return layout.Flex{Gap: gap}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			var items, destinations []ui.Choice
			for _, item := range d.sources.Accounts {
				choice := ui.Choice{ID: item.ID.String(), Label: item.Name + " · " + item.DefaultCurrency.String(), SearchText: item.Name}
				items = append(items, choice)
				if history.CanTransferTo(account, item) {
					destinations = append(destinations, choice)
				}
			}
			next := any(&d.amount.Editor)
			if d.movement == history.CashTransfer && len(destinations) > 0 {
				next = &d.destinationPicker.Trigger
			}
			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					field := ui.PickerField{Label: labels.account, Origin: origin, Disabled: !gtx.Enabled(), Next: next}
					d.accountPicker.Layout(gtx, view, field, ui.SelectOptions{Items: items, Searchable: true}, d.account.String(), func(value string) {
						d.account = domain.AccountID(value)
						if !d.canSelectDestination(d.destination) {
							d.destination = ""
						}
						d.problem = i18n.NoMessage
					})
					return layout.Dimensions{Size: image.Pt(width, gtx.Dp(pickerHeight))}
				}),
			}
			if d.movement == history.CashTransfer {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					at := origin.Add(image.Pt(0, gtx.Dp(pickerHeight+8)))
					field := ui.PickerField{Label: i18n.TransferTo, Origin: at, Disabled: !gtx.Enabled() || len(destinations) == 0, Next: &d.amount.Editor, Previous: &d.accountPicker.Trigger}
					d.destinationPicker.Layout(gtx, view, field, ui.SelectOptions{Items: destinations, Searchable: true}, d.destination.String(), func(value string) {
						d.destination, d.problem = domain.AccountID(value), i18n.NoMessage
					})
					return layout.Dimensions{Size: image.Pt(width, gtx.Dp(pickerHeight))}
				}))
			}
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if d.focusAmount && gtx.Enabled() {
						d.amount.Focus(gtx)
						d.focusAmount = false
					}
					return d.amount.Layout(gtx, view, ui.FieldOptions{Label: labels.amount, Hint: i18n.AccountCreateAmountHint})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if found && d.movement == history.CashTransfer && len(destinations) == 0 {
						return view.Message(l.Text(i18n.TransferNoDestination), view.Colors.SecondaryInk)(gtx)
					}
					if !found {
						return layout.Dimensions{}
					}
					return view.Message(account.DefaultCurrency.String(), view.Colors.SecondaryInk)(gtx)
				}),
			)
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			pickerOrigin := origin.Add(image.Pt(width+gap, 0))
			zone := d.sources.Location
			today := now.In(zone).Format(time.DateOnly)
			weekStart := time.Monday
			if view.WeekStart == "sunday" {
				weekStart = time.Sunday
			}
			next := any(&d.preview)
			if d.movement == history.CashTransfer {
				next = &d.fee.Editor
			}
			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					field := ui.PickerField{Label: i18n.PreviewEffectiveDate, Origin: pickerOrigin, Disabled: !gtx.Enabled(), Next: &d.timePicker.Trigger}
					dates := ui.DateOptions{Min: d.sources.Origin.StartedAt.In(zone).Format(time.DateOnly), Today: today, Max: today, WeekStart: weekStart, Format: view.DateFormat}
					d.datePicker.Layout(gtx, view, field, dates, d.day, func(day string) {
						d.day, d.problem = day, i18n.NoMessage
					})
					pickerOrigin.Y += gtx.Dp(pickerHeight + 8)
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(pickerHeight))}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					field := ui.PickerField{Label: i18n.PreviewEffectiveTime, Origin: pickerOrigin, Disabled: !gtx.Enabled(), Previous: &d.datePicker.Trigger, Next: next}
					d.timePicker.Layout(gtx, view, field, d.time, view.TimeFormat == settings.TimeFormat12, false, func(clock string) {
						d.time, d.problem = clock, i18n.NoMessage
					})
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(pickerHeight))}
				}),
			}
			if d.movement == history.CashTransfer {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return d.fee.Layout(gtx, view, ui.FieldOptions{Label: i18n.TransferFeeInput})
				}))
			}
			children = append(children, layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.ContributionTimezone), zone), view.Colors.SecondaryInk)))
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
		}),
	)
}

func contributionTimestamp(view ui.View, instant time.Time, zone *time.Location) string {
	value := format.LocalDateTime(instant.In(zone), settings.Settings{DateFormat: view.DateFormat, TimeFormat: view.TimeFormat}, settings.Language(view.Language))
	return fmt.Sprintf(view.Language.Text(i18n.ContributionEffective), value, zone)
}

func (p *historyCashMovements) layoutReview(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.draft, view.Language
	labels := cashMovementLabels[d.movement]
	review := d.review
	if !d.uncertain && d.posting == nil && d.edit.Clicked(gtx) {
		d.review, d.focusAmount = nil, true
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if !d.uncertain && d.posting == nil && d.cancel.Clicked(gtx) {
		p.draft = nil
		gtx.Execute(key.FocusCmd{Tag: &p.actions[d.movement]})
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if d.posting == nil && d.post.Clicked(gtx) {
		p.post()
		gtx.Execute(op.InvalidateCmd{})
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.Label(gtx, l.Text(labels.review), 19, font.Medium, view.Colors.Ink)
		}),
		layout.Rigid(view.Message(contributionTimestamp(view, review.Preview.Activity.EffectiveAt, review.Timezone), view.Colors.SecondaryInk)),
	}
	var endpoints []layout.FlexChild
	for i, item := range review.Preview.Review {
		currency := item.Currency.String()
		title := item.AccountName
		if d.movement == history.CashTransfer {
			field := i18n.TransferFrom
			if i == 1 {
				field = i18n.TransferTo
			}
			title = l.Text(field) + " · " + title
		}
		endpoints = append(endpoints, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(9)}.Layout(gtx,
				layout.Rigid(view.Message(title, view.Colors.Ink)),
				layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.ContributionBefore), item.Before+" "+currency), view.Colors.SecondaryInk)),
				layout.Rigid(view.Message(fmt.Sprintf(l.Text(labels.delta), item.Delta+" "+currency), view.Colors.Ink)),
				layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.ContributionAfter), item.After+" "+currency), view.Colors.Ink)),
			)
		}))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Gap: gtx.Dp(16)}.Layout(gtx, endpoints...)
	}))
	if d.movement == history.CashTransfer {
		for _, effect := range review.Preview.Effects {
			if effect.Role == domain.EffectRoleTransferFrom || effect.Role == domain.EffectRoleFee {
				label := i18n.TransferPrincipal
				if effect.Role == domain.EffectRoleFee {
					label = i18n.TransferFee
				}
				children = append(children, layout.Rigid(view.Message(fmt.Sprintf(l.Text(label), movementMoney(effect)), view.Colors.Ink)))
			}
		}
	}
	if d.uncertain {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.ContributionUncertain), view.Colors.ErrorInk)))
	}
	if d.posting != nil {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.SettingsPending), view.Colors.SecondaryInk)))
	} else {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if d.uncertain {
				return view.PrimaryButton(&d.post, l.Text(i18n.ContributionRetryPost))(gtx)
			}
			return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel))),
				layout.Rigid(view.Button(&d.edit, l.Text(i18n.AccountCreateBackToDetails))),
				layout.Rigid(view.PrimaryButton(&d.post, l.Text(labels.confirm))),
			)
		}))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(9)}.Layout(gtx, children...)
}

func (p *historyCashMovements) layoutDetail(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.detail, view.Language
	labels := cashMovementLabels[d.movement]
	if d.back.Clicked(gtx) {
		p.detail = nil
		gtx.Execute(key.FocusCmd{Tag: p.detailButton(d.id)})
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if d.err != nil && d.retry.Clicked(gtx) {
		p.readDetail()
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(10)}.Layout(gtx,
				layout.Rigid(view.Button(&d.back, l.Text(i18n.ContributionBack))),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.Label(gtx, l.Text(labels.title), 19, font.Medium, view.Colors.Ink)
				}),
			)
		}),
	}
	if d.posted {
		children = append(children, layout.Rigid(view.Message(l.Text(labels.posted), view.Colors.Ink)))
	}
	switch {
	case d.pending != nil:
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)))
	case d.err != nil:
		children = append(children,
			layout.Rigid(view.Message(l.Text(i18n.ContributionDetailFailed), view.Colors.ErrorInk)),
			layout.Rigid(view.Button(&d.retry, l.Text(i18n.HistoryRetry))),
		)
	case d.record != nil:
		record := d.record
		children = append(children, layout.Rigid(view.Message(contributionTimestamp(view, record.Activity.EffectiveAt, record.Location), view.Colors.SecondaryInk)))
		accountName := func(id *domain.AccountID) string {
			if id != nil && record.Accounts[*id] != "" {
				return record.Accounts[*id]
			}
			return l.Text(i18n.AccountReferenceUnavailable)
		}
		for _, effect := range record.Activity.Effects {
			if effect.Money == nil {
				continue
			}
			amount := movementMoney(effect)
			if d.movement == history.CashTransfer {
				switch effect.Role {
				case domain.EffectRoleTransferFrom:
					amount = fmt.Sprintf(l.Text(i18n.TransferSent), accountName(effect.AccountID), amount)
				case domain.EffectRoleTransferTo:
					amount = fmt.Sprintf(l.Text(i18n.TransferReceived), accountName(effect.AccountID), amount)
				case domain.EffectRoleFee:
					amount = fmt.Sprintf(l.Text(i18n.TransferFeePaid), accountName(effect.AccountID), amount)
				}
			} else {
				children = append(children, layout.Rigid(view.Message(accountName(effect.AccountID), view.Colors.Ink)))
				amount = fmt.Sprintf(l.Text(labels.delta), amount)
			}
			children = append(children, layout.Rigid(view.Message(amount, view.Colors.Ink)))
		}
		for _, result := range record.Activity.Resulting {
			balance := result.Amount + " " + result.Currency.String()
			label := fmt.Sprintf(l.Text(i18n.ContributionAfter), balance)
			if d.movement == history.CashTransfer {
				label = fmt.Sprintf(l.Text(i18n.TransferAccountBalance), accountName(result.AccountID), balance)
			}
			children = append(children, layout.Rigid(view.Message(label, view.Colors.Ink)))
		}
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(9)}.Layout(ui.LimitWidth(gtx, 640), children...)
}

func movementMoney(effect domain.ActivityEffect) string {
	if effect.Money == nil {
		return ""
	}
	return effect.Money.CanonicalAmount() + " " + effect.Money.Currency().String()
}
