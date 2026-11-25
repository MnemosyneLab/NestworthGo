package desktop

import (
	"image"
	"time"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

var repeatReasonLabels = map[domain.ActivityReason]i18n.MessageID{
	domain.ReasonIncome: i18n.HistoryCashReasonIncome, domain.ReasonInterest: i18n.HistoryCashReasonInterest,
	domain.ReasonContribution: i18n.ContributionTitle, domain.ReasonGift: i18n.HistoryCashReasonGift, domain.ReasonOther: i18n.HistoryCashReasonOther,
	domain.ReasonExpense: i18n.HistoryCashReasonExpense, domain.ReasonFee: i18n.HistoryCashReasonFee, domain.ReasonTax: i18n.HistoryCashReasonTax,
}

func (p *historyPage) layoutRepeat(gtx layout.Context, view ui.View) layout.Dimensions {
	r, l := p.reads, view.Language
	visit := r.detail
	d := visit.repeatDraft
	gtx, view = view.Measure(gtx, 680)
	if d.review != nil {
		return p.layoutRepeatReview(gtx, view)
	}
	if d.cancel.Clicked(gtx) {
		visit.repeatDraft = nil
		if d.refused {
			r.loadDetail()
		}
		gtx.Execute(key.FocusCmd{Tag: &visit.repeat})
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	busy := d.loading != nil || d.previewing != nil
	if d.loadFailed && d.retryLoad.Clicked(gtx) {
		r.loadRepeatSources()
	}
	origin := view.Origin
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, func(gtx layout.Context) layout.Dimensions {
				return view.Label(gtx, l.Text(i18n.HistoryRepeatAction), 19, font.Medium, view.Colors.Ink)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, view.Message(l.Text(historyKindLabel(visit.record.Activity.Kind, visit.record.Activity.Reason)), view.Colors.SecondaryInk))
		}),
	}
	ready := d.sources != nil && !d.loadFailed && len(d.sources.Accounts) != 0
	if ready {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if busy || d.refused {
				gtx = gtx.Disabled()
			}
			return d.layoutRepeatFields(gtx, view, origin, r.clock())
		}))
	}
	problem := d.problem
	if d.loadFailed {
		problem = i18n.ContributionSourcesFailed
	} else if !busy && !ready {
		problem = i18n.ContributionNoAccounts
	} else if ready && problem == i18n.NoMessage {
		if _, ok := d.selectedAccount(); !ok {
			problem = i18n.HistoryRepeatAccountUnavailable
		}
	}
	if problem != i18n.NoMessage {
		children = append(children, layout.Rigid(view.Message(l.Text(problem), view.Colors.ErrorInk)))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		buttons := []layout.FlexChild{layout.Rigid(view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel)))}
		switch {
		case busy:
			buttons = append(buttons, layout.Rigid(view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)))
		case d.loadFailed:
			buttons = append(buttons, layout.Rigid(view.Button(&d.retryLoad, l.Text(i18n.HistoryRetry))))
		case ready && !d.refused:
			if d.prepare.Clicked(gtx) {
				r.previewRepeat()
			}
			buttons = append(buttons, layout.Rigid(view.PrimaryButton(&d.prepare, l.Text(i18n.HistoryRepeatPreview))))
		}
		return layout.Flex{Gap: gtx.Dp(8), Alignment: layout.Middle}.Layout(gtx, buttons...)
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}

func (d *historyRepeatDraft) layoutRepeatFields(gtx layout.Context, view ui.View, origin image.Point, now time.Time) layout.Dimensions {
	l := view.Language
	gap := gtx.Dp(16)
	width := (gtx.Constraints.Max.X - gap) / 2
	return layout.Flex{Gap: gap}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			at := origin
			var accounts []ui.Choice
			for _, account := range d.sources.Accounts {
				accounts = append(accounts, ui.Choice{ID: account.ID.String(), Label: account.Name + " · " + account.DefaultCurrency.String(), SearchText: account.Name})
			}
			reasons := domain.MoneyOutReasons()
			if d.added {
				reasons = domain.MoneyInReasons()
			}
			var options []ui.Choice
			for _, reason := range reasons {
				if label, ordinary := repeatReasonLabels[reason]; ordinary {
					options = append(options, ui.Choice{ID: string(reason), Label: l.Text(label)})
				}
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return fieldPosition(gtx, &at, func(gtx layout.Context) layout.Dimensions {
						d.accountPicker.Layout(gtx, view, ui.PickerField{Label: i18n.HistoryRepeatAccount, Origin: at, Disabled: !gtx.Enabled(), Next: &d.amount.Editor},
							ui.SelectOptions{Items: accounts, Searchable: true}, d.account.String(), func(id string) {
								if domain.AccountID(id) != d.account {
									d.account, d.reason, d.problem = domain.AccountID(id), domain.ReasonOther, i18n.NoMessage
									account, _ := d.selectedAccount()
									d.currency = account.DefaultCurrency
								}
							})
						return layout.Dimensions{Size: image.Pt(width, gtx.Dp(60))}
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return fieldPosition(gtx, &at, func(gtx layout.Context) layout.Dimensions {
						if d.focusAmount && gtx.Enabled() {
							d.amount.Focus(gtx)
							d.focusAmount = false
						}
						account, found := d.selectedAccount()
						if found && account.TrackingMode == domain.TrackingHoldings {
							return layoutCashCurrencyAmount(gtx, view, at, &d.amount, i18n.HistoryRepeatAmount, &d.currencyPicker,
								i18n.CashCurrency, d.currency, "", func(code domain.CurrencyCode) {
									d.currency, d.problem = code, i18n.NoMessage
								})
						}
						return d.amount.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.HistoryRepeatAmount, Disabled: !gtx.Enabled()}, d.currency.String())
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					d.reasonPicker.Layout(gtx, view, ui.PickerField{Label: i18n.HistoryRepeatReason, Origin: at, Disabled: !gtx.Enabled()},
						ui.SelectOptions{Items: options}, string(d.reason), func(reason string) {
							d.reason, d.problem = domain.ActivityReason(reason), i18n.NoMessage
						})
					return layout.Dimensions{Size: image.Pt(width, gtx.Dp(60))}
				}),
			)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			at := origin.Add(image.Pt(width+gap, 0))
			return d.layoutRepeatDateAndNote(gtx, view, at, now)
		}),
	)
}

func (d *historyRepeatDraft) layoutRepeatDateAndNote(gtx layout.Context, view ui.View, at image.Point, now time.Time) layout.Dimensions {
	zone := d.sources.Location
	today := now.In(zone).Format(time.DateOnly)
	week := time.Monday
	if view.WeekStart == "sunday" {
		week = time.Sunday
	}
	dates := ui.DateOptions{Min: d.sources.Origin.StartedAt.In(zone).Format(time.DateOnly), Today: today, Max: today, WeekStart: week, Format: view.DateFormat}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gap := gtx.Dp(8)
			return layout.Flex{Gap: gap}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					d.datePicker.Layout(gtx, view, ui.PickerField{Label: i18n.PreviewEffectiveDate, Origin: at, Disabled: !gtx.Enabled(), Next: &d.timePicker.Trigger},
						dates, d.day, func(day string) {
							d.day, d.exactAt, d.problem = day, time.Time{}, i18n.NoMessage
						})
					at.X += gtx.Constraints.Max.X + gap
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					d.timePicker.Layout(gtx, view, ui.PickerField{Label: i18n.PreviewEffectiveTime, Origin: at, Disabled: !gtx.Enabled(), Previous: &d.datePicker.Trigger},
						d.minute, view.TimeFormat == settings.TimeFormat12, false, func(clock string) {
							d.minute, d.exactAt, d.problem = clock, time.Time{}, i18n.NoMessage
						})
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return d.note.Layout(gtx, view, ui.FieldOptions{Label: i18n.HistoryRepeatNote, Disabled: !gtx.Enabled()})
		}),
	)
}

func (p *historyPage) layoutRepeatReview(gtx layout.Context, view ui.View) layout.Dimensions {
	r, l := p.reads, view.Language
	d := r.detail.repeatDraft
	if d.missingPlan {
		if d.inspect.Clicked(gtx) {
			r.back()
			gtx.Execute(op.InvalidateCmd{})
		}
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
			layout.Rigid(view.Message(l.Text(i18n.HistoryRepeatPlanMissing), view.Colors.ErrorInk)),
			layout.Rigid(view.Button(&d.inspect, l.Text(i18n.ContributionBack))),
		)
	}
	if !p.repeatUnresolved() && d.edit.Clicked(gtx) {
		d.review, d.focusAmount = nil, true
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	at, err := time.Parse(time.RFC3339Nano, d.review.EffectiveAt)
	if err != nil {
		return view.Message(l.Text(i18n.CashMovementPreviewFailed), view.Colors.ErrorInk)(gtx)
	}
	timestamp, err := targetBalanceTimestamp(view, at, d.sources.Location.String())
	if err != nil {
		return view.Message(l.Text(i18n.ContributionInvalidTime), view.Colors.ErrorInk)(gtx)
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.Label(gtx, l.Text(i18n.HistoryRepeatPreview), 19, font.Medium, view.Colors.Ink)
		}),
	}
	line := d.review.Review[0]
	facts := []layout.Widget{func(gtx layout.Context) layout.Dimensions {
		lines := []layout.FlexChild{layout.Rigid(view.Message(line.AccountName+" · "+l.Text(repeatReasonLabels[d.reason]), view.Colors.Ink))}
		lines = append(lines, targetBalanceFacts(view, i18n.CashMovementDelta, line.Before, line.After, line.Delta, line.Currency, timestamp)...)
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, lines...)
	}}
	if d.note.Text() != "" {
		facts = append(facts, view.Message(d.note.Text(), view.Colors.SecondaryInk))
	}
	children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return view.List(&d.reviewList).Layout(gtx, len(facts), func(gtx layout.Context, i int) layout.Dimensions {
			return layout.Inset{Bottom: 8}.Layout(gtx, facts[i])
		})
	}))
	if d.uncertain {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.AccountCashUncertain), view.Colors.ErrorInk)))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		switch {
		case d.committing != nil:
			return view.Message(l.Text(i18n.SettingsPending), view.Colors.SecondaryInk)(gtx)
		case d.uncertain:
			if d.retry.Clicked(gtx) {
				r.confirmRepeat()
			}
			return view.PrimaryButton(&d.retry, l.Text(i18n.ContributionRetryPost))(gtx)
		default:
			if d.confirm.Clicked(gtx) {
				r.confirmRepeat()
			}
			return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(view.Button(&d.edit, l.Text(i18n.HistoryRepeatEdit))),
				layout.Rigid(view.PrimaryButton(&d.confirm, l.Text(i18n.HistoryRepeatConfirm))),
			)
		}
	}))
	gtx = ui.FitWindow(gtx)
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}
