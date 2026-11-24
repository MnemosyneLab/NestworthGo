package desktop

import (
	"fmt"
	"image"
	"time"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func (p *historyContributions) layoutForm(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.draft, view.Language
	gtx = ui.LimitWidth(gtx, 640)
	if d.review != nil {
		return p.layoutReview(gtx, view)
	}
	if d.cancel.Clicked(gtx) {
		p.draft = nil
		gtx.Execute(key.FocusCmd{Tag: &p.add})
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	origin := view.Origin
	children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return fieldPosition(gtx, &origin, func(gtx layout.Context) layout.Dimensions {
			return view.Label(gtx, l.Text(i18n.ContributionAdd), 19, font.Medium, view.Colors.Ink)
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
			buttons = append(buttons, layout.Rigid(view.PrimaryButton(&d.preview, l.Text(i18n.ContributionPreview))))
		}
		return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx, buttons...)
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}

func (d *contributionDraft) layoutContributionFields(gtx layout.Context, view ui.View, origin image.Point, now time.Time) layout.Dimensions {
	const pickerHeight = 60
	l := view.Language
	gap := gtx.Dp(16)
	width := (gtx.Constraints.Max.X - gap) / 2
	account, found := d.selected()
	return layout.Flex{Gap: gap}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			var items []ui.Choice
			for _, item := range d.sources.Accounts {
				items = append(items, ui.Choice{ID: item.ID.String(), Label: item.Name + " · " + item.DefaultCurrency.String(), SearchText: item.Name})
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					field := ui.PickerField{Label: i18n.ContributionAccount, Origin: origin, Disabled: !gtx.Enabled(), Next: &d.amount.Editor}
					d.accountPicker.Layout(gtx, view, field, ui.SelectOptions{Items: items, Searchable: true}, d.account.String(), func(value string) {
						d.account = domain.AccountID(value)
						d.problem = i18n.NoMessage
					})
					return layout.Dimensions{Size: image.Pt(width, gtx.Dp(pickerHeight))}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if d.focusAmount && gtx.Enabled() {
						d.amount.Focus(gtx)
						d.focusAmount = false
					}
					return d.amount.Layout(gtx, view, ui.FieldOptions{Label: i18n.ContributionAmount, Hint: i18n.AccountCreateAmountHint})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !found {
						return layout.Dimensions{}
					}
					return view.Message(account.DefaultCurrency.String(), view.Colors.SecondaryInk)(gtx)
				}),
			)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			pickerOrigin := origin.Add(image.Pt(width+gap, 0))
			zone := d.sources.Location
			today := now.In(zone).Format(time.DateOnly)
			weekStart := time.Monday
			if view.WeekStart == "sunday" {
				weekStart = time.Sunday
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
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
					field := ui.PickerField{Label: i18n.PreviewEffectiveTime, Origin: pickerOrigin, Disabled: !gtx.Enabled(), Previous: &d.datePicker.Trigger, Next: &d.preview}
					d.timePicker.Layout(gtx, view, field, d.time, view.TimeFormat == settings.TimeFormat12, false, func(clock string) {
						d.time, d.problem = clock, i18n.NoMessage
					})
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(pickerHeight))}
				}),
				layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.ContributionTimezone), zone), view.Colors.SecondaryInk)),
			)
		}),
	)
}

func contributionTimestamp(view ui.View, instant time.Time, zone *time.Location) string {
	value := format.LocalDateTime(instant.In(zone), settings.Settings{DateFormat: view.DateFormat, TimeFormat: view.TimeFormat}, settings.Language(view.Language))
	return fmt.Sprintf(view.Language.Text(i18n.ContributionEffective), value, zone)
}

func (p *historyContributions) layoutReview(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.draft, view.Language
	review := d.review
	if !d.uncertain && d.posting == nil && d.edit.Clicked(gtx) {
		d.review, d.focusAmount = nil, true
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if !d.uncertain && d.posting == nil && d.cancel.Clicked(gtx) {
		p.draft = nil
		gtx.Execute(key.FocusCmd{Tag: &p.add})
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if d.posting == nil && d.post.Clicked(gtx) {
		p.post()
		gtx.Execute(op.InvalidateCmd{})
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.Label(gtx, l.Text(i18n.ContributionReview), 19, font.Medium, view.Colors.Ink)
		}),
		layout.Rigid(view.Message(contributionTimestamp(view, review.Preview.Activity.EffectiveAt, review.Timezone), view.Colors.SecondaryInk)),
	}
	for _, item := range review.Preview.Review {
		currency := item.Currency.String()
		children = append(children,
			layout.Rigid(view.Message(item.AccountName, view.Colors.Ink)),
			layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.ContributionBefore), item.Before+" "+currency), view.Colors.SecondaryInk)),
			layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.ContributionDelta), item.Delta+" "+currency), view.Colors.Ink)),
			layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.ContributionAfter), item.After+" "+currency), view.Colors.Ink)),
		)
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
				layout.Rigid(view.PrimaryButton(&d.post, l.Text(i18n.ContributionConfirm))),
			)
		}))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(9)}.Layout(gtx, children...)
}

func (p *historyContributions) layoutDetail(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.detail, view.Language
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
					return view.Label(gtx, l.Text(i18n.ContributionTitle), 19, font.Medium, view.Colors.Ink)
				}),
			)
		}),
	}
	if d.posted {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.ContributionPosted), view.Colors.Ink)))
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
		account := record.Account
		if account == "" {
			account = l.Text(i18n.AccountReferenceUnavailable)
		}
		children = append(children,
			layout.Rigid(view.Message(account, view.Colors.Ink)),
			layout.Rigid(view.Message(contributionTimestamp(view, record.Activity.EffectiveAt, record.Location), view.Colors.SecondaryInk)),
		)
		for _, effect := range record.Activity.Effects {
			if effect.Money != nil {
				amount := effect.Money.CanonicalAmount() + " " + effect.Money.Currency().String()
				children = append(children, layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.ContributionDelta), amount), view.Colors.Ink)))
			}
		}
		for _, result := range record.Activity.Resulting {
			children = append(children, layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.ContributionAfter), result.Amount+" "+result.Currency.String()), view.Colors.Ink)))
		}
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(9)}.Layout(ui.LimitWidth(gtx, 640), children...)
}
