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
	review, confirm, stale, posted, delta                                   i18n.MessageID
}

var cashMovementLabels = [...]movementLabels{
	history.CashContribution: {
		i18n.ContributionAdd, i18n.ContributionTitle, i18n.ContributionAccount, i18n.ContributionAmount,
		i18n.ContributionHistoryRequired, i18n.ContributionPreview, i18n.ContributionPreviewFailed,
		i18n.ContributionReview, i18n.ContributionConfirm, i18n.ContributionStale, i18n.ContributionPosted,
		i18n.ContributionDelta,
	},
	history.CashWithdrawal: {
		i18n.WithdrawalAdd, i18n.HistoryCashOut, i18n.WithdrawalAccount, i18n.WithdrawalAmount,
		i18n.CashMovementHistoryRequired, i18n.WithdrawalPreview, i18n.CashMovementPreviewFailed,
		i18n.WithdrawalReview, i18n.WithdrawalConfirm, i18n.CashMovementStale, i18n.WithdrawalPosted,
		i18n.WithdrawalDelta,
	},
	history.CashTransfer: {
		i18n.TransferAdd, i18n.HistoryCashTransfer, i18n.TransferFrom, i18n.TransferAmount,
		i18n.CashMovementHistoryRequired, i18n.TransferPreview, i18n.CashMovementPreviewFailed,
		i18n.TransferReview, i18n.TransferConfirm, i18n.CashMovementStale, i18n.TransferPosted,
		i18n.CashMovementDelta,
	},
	history.CashConversion: {
		i18n.ConversionAdd, i18n.HistoryFXConversion, i18n.ConversionAccount, i18n.ConversionSoldAmount,
		i18n.CashMovementHistoryRequired, i18n.ConversionPreview, i18n.CashMovementPreviewFailed,
		i18n.ConversionReview, i18n.ConversionConfirm, i18n.CashMovementStale, i18n.ConversionPosted,
		i18n.CashMovementDelta,
	},
	history.CashDebtDraw: {
		i18n.HistoryDebtDraw, i18n.HistoryDebtDraw, i18n.LiabilityChooseCash, i18n.HistoryDebtPrincipal,
		i18n.CashMovementHistoryRequired, i18n.LiabilityPreview, i18n.CashMovementPreviewFailed,
		i18n.HistoryDebtDraw, i18n.LiabilityConfirm, i18n.CashMovementStale, i18n.LiabilityPosted,
		i18n.CashMovementDelta,
	},
	history.CashDebtPayment: {
		i18n.HistoryDebtPayment, i18n.HistoryDebtPayment, i18n.LiabilityChooseCash, i18n.HistoryDebtPrincipal,
		i18n.CashMovementHistoryRequired, i18n.LiabilityPreview, i18n.CashMovementPreviewFailed,
		i18n.HistoryDebtPayment, i18n.LiabilityConfirm, i18n.CashMovementStale, i18n.LiabilityPosted,
		i18n.CashMovementDelta,
	},
}

func (p *historyCashMovements) formLabels() movementLabels {
	movement := p.draft.movement
	if p.linked == nil || movement == history.CashConversion {
		return cashMovementLabels[movement]
	}
	var action i18n.MessageID
	for _, entry := range p.linked.modes {
		if entry.kind == movement {
			action = entry.label
			break
		}
	}
	amount := i18n.HistoryRepeatAmount
	switch movement {
	case history.CashTransfer:
		amount = i18n.TransferSentAmount
	case history.CashDebtDraw, history.CashDebtPayment:
		amount = i18n.HistoryDebtPrincipal
	case history.CashValueUpdate:
		amount = i18n.ReconciliationNewValue
		if p.linked.liability {
			amount = i18n.AccountCardBalanceTarget
		}
	}
	return movementLabels{action: action, amount: amount, review: action, preview: i18n.LiabilityPreview,
		confirm: i18n.LiabilityConfirm, delta: i18n.CashMovementDelta, previewFailed: i18n.CashMovementPreviewFailed, stale: i18n.CashMovementStale}
}

func (p *historyCashMovements) layoutForm(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.draft, view.Language
	labels := p.formLabels()
	gtx, view = view.Measure(gtx, 640)
	if d.review != nil {
		return p.layoutReview(gtx, view)
	}
	if d.cancel.Clicked(gtx) {
		p.discard()
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
	if ready && p.linked != nil {
		_, ready = d.accountByID(p.linked.account)
	} else if ready && p.account != "" {
		_, ready = d.accountByID(p.account)
	} else if ready && d.movement.Debt() {
		ready = d.hasDebtEndpoints()
	}
	if ready {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if busy {
				gtx = gtx.Disabled()
			}
			return p.layoutMovementFields(gtx, view, origin, p.clock())
		}))
		if p.linked != nil || p.account != "" {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return d.note.Layout(gtx, view, ui.FieldOptions{Label: i18n.PreviewNotes, Hint: i18n.PreviewNotesHint, Disabled: busy})
			}))
		}
	} else if !busy && d.sourceError == i18n.NoMessage {
		message := i18n.ContributionNoAccounts
		if d.movement == history.CashConversion {
			message = i18n.ConversionNoAccounts
		} else if d.movement.Debt() {
			message = i18n.LiabilityNoCash
			for _, account := range d.sources.Accounts {
				if !account.IsLiability() {
					message = i18n.LiabilityNoDebt
					break
				}
			}
		}
		children = append(children, layout.Rigid(view.Message(l.Text(message), view.Colors.SecondaryInk)))
	}
	if d.sourceError != i18n.NoMessage {
		children = append(children, layout.Rigid(view.Message(l.Text(d.sourceError), view.Colors.ErrorInk)))
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
		case ready && !busy && (p.linked == nil || p.linked.link() != nil):
			if p.linked != nil && (p.valueUnchanged() || d.movement == history.CashValueUpdate && d.amount.Text() == "") ||
				d.movement.Debt() && (d.account == "" || d.destination == "" || d.amount.Text() == "") {
				buttons = append(buttons, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.PrimaryButton(&d.preview, l.Text(labels.preview))(gtx.Disabled())
				}))
				break
			}
			if d.preview.Clicked(gtx) {
				p.preview()
				gtx.Execute(op.InvalidateCmd{})
			}
			buttons = append(buttons, layout.Rigid(view.PrimaryButton(&d.preview, l.Text(labels.preview))))
		}
		if d.sourceError == i18n.NoMessage && d.problem != i18n.NoMessage {
			buttons = append(buttons, layout.Flexed(1, view.Message(l.Text(d.problem), view.Colors.ErrorInk)))
		}
		return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx, buttons...)
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}

func (p *historyCashMovements) layoutMovementFields(gtx layout.Context, view ui.View, origin image.Point, now time.Time) layout.Dimensions {
	const pickerHeight = 60
	d := p.draft
	l := view.Language
	gap := gtx.Dp(16)
	width := (gtx.Constraints.Max.X - gap) / 2
	account, found := d.accountByID(d.account)
	destination, hasDestination := d.accountByID(d.destination)
	transfer, conversion := d.movement == history.CashTransfer, d.movement == history.CashConversion
	debt := d.movement.Debt()
	linkedTransfer := transfer && p.linked != nil
	crossCurrency := transfer && found && hasDestination && d.currency != d.toCurrency
	sourceEditable := !debt && !linkedTransfer && found && account.TrackingMode == domain.TrackingHoldings && (p.linked == nil || p.linked.currency == "" || conversion)
	targetEditable := conversion || (transfer && !linkedTransfer && hasDestination && destination.TrackingMode == domain.TrackingHoldings)
	labels := p.formLabels()
	return layout.Flex{Gap: gap}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			at := origin
			var children []layout.FlexChild
			var destinations []ui.Choice
			if linkedTransfer {
				children = p.layoutCheckpointTransferFields(view, &at)
			} else {
				var items []ui.Choice
				for _, item := range d.sources.Accounts {
					if debt && item.IsLiability() {
						continue
					}
					name := item.Name
					if item.TrackingMode != domain.TrackingHoldings {
						name += " · " + item.DefaultCurrency.String()
					}
					choice := ui.Choice{ID: item.ID.String(), Label: name, SearchText: item.Name}
					items = append(items, choice)
					if transfer && history.CanTransferTo(account, item) {
						destinations = append(destinations, choice)
					}
				}
				next := any(&d.amount.Editor)
				if debt || transfer && len(destinations) > 0 {
					next = &d.destinationPicker.Trigger
				}
				children = []layout.FlexChild{
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return fieldPosition(gtx, &at, func(gtx layout.Context) layout.Dimensions {
							if p.linked != nil || p.account != "" {
								return view.Message(account.Name, view.Colors.Ink)(gtx)
							}
							field := ui.PickerField{Label: labels.account, Origin: at, Disabled: !gtx.Enabled(), Next: next}
							d.accountPicker.Layout(gtx, view, field, ui.SelectOptions{Items: items, Searchable: true}, d.account.String(), func(value string) {
								d.chooseSource(domain.AccountID(value))
							})
							return layout.Dimensions{Size: image.Pt(width, gtx.Dp(pickerHeight))}
						})
					}),
				}
				if transfer {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return fieldPosition(gtx, &at, func(gtx layout.Context) layout.Dimensions {
							field := ui.PickerField{Label: i18n.TransferTo, Origin: at, Disabled: !gtx.Enabled() || len(destinations) == 0, Next: &d.amount.Editor, Previous: &d.accountPicker.Trigger}
							d.destinationPicker.Layout(gtx, view, field, ui.SelectOptions{Items: destinations, Searchable: true}, d.destination.String(), func(value string) {
								d.chooseDestination(domain.AccountID(value))
							})
							return layout.Dimensions{Size: image.Pt(width, gtx.Dp(pickerHeight))}
						})
					}))
				}
			}
			if debt {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return fieldPosition(gtx, &at, p.checkpointDebtPeer(view, at, account))
				}))
			}
			currency := d.currency
			if debt && account.IsLiability() && hasDestination {
				currency = destination.DefaultCurrency
			}
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return fieldPosition(gtx, &at, func(gtx layout.Context) layout.Dimensions {
					if d.focusAmount && gtx.Enabled() {
						d.amount.Focus(gtx)
						d.focusAmount = false
					}
					label, currencyLabel := labels.amount, i18n.CashCurrency
					if crossCurrency {
						label = i18n.TransferSentAmount
					}
					if transfer {
						currencyLabel = i18n.TransferSentCurrency
					}
					if conversion {
						currencyLabel = i18n.ConversionSoldCurrency
					}
					if sourceEditable {
						return layoutCashCurrencyAmount(gtx, view, at, &d.amount, label, &d.currencyPicker, currencyLabel, d.currency, "", d.chooseCurrency)
					}
					options := ui.FieldOptions{Label: label, Hint: i18n.AccountCreateAmountHint}
					if crossCurrency || debt || p.linked != nil {
						return d.amount.LayoutCurrency(gtx, view, options, currency.String())
					}
					return d.amount.Layout(gtx, view, options)
				})
			}))
			if d.movement == history.CashDebtPayment {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return d.fee.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.LiabilityInterest}, currency.String())
				}))
			}
			if d.movement == history.CashValueUpdate {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					value := p.linked.value()
					if value == "" {
						return layout.Dimensions{}
					}
					current := portfolioDecimal(view, value) + " " + currency.String()
					return view.Message(fmt.Sprintf(l.Text(i18n.ReconciliationCurrentValue), current), view.Colors.SecondaryInk)(gtx)
				}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !p.valueUnchanged() {
						return layout.Dimensions{}
					}
					return view.Message(l.Text(i18n.ReconciliationValueUnchanged), view.Colors.SecondaryInk)(gtx)
				}))
			}
			if (p.linked != nil || p.account != "") && !conversion && !transfer && !debt {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.layoutCashReason(gtx, view, at)
				}))
			}
			switch {
			case linkedTransfer && found && hasDestination && !crossCurrency:
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					value := d.amount.Text()
					if value == "" {
						value = l.Text(i18n.OverviewNoValue)
					} else {
						value += " " + d.toCurrency.String()
					}
					return view.Message(l.Text(i18n.TransferReceivedAmount)+" · "+value, view.Colors.SecondaryInk)(gtx)
				}))
			case found && (conversion || crossCurrency || targetEditable):
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label, currencyLabel := i18n.TransferReceivedAmount, i18n.TransferReceivedCurrency
					var amount *ui.TextField
					var excluded domain.CurrencyCode
					if crossCurrency || conversion {
						amount = &d.received
					}
					if conversion {
						label, currencyLabel, excluded = i18n.ConversionBoughtAmount, i18n.ConversionBoughtCurrency, d.currency
					}
					if targetEditable {
						return layoutCashCurrencyAmount(gtx, view, at, amount, label, &d.toCurrencyPicker, currencyLabel, d.toCurrency, excluded, d.chooseToCurrency)
					}
					return d.received.LayoutCurrency(gtx, view, ui.FieldOptions{Label: label}, d.toCurrency.String())
				}))
			case transfer && found && !linkedTransfer && len(destinations) == 0:
				children = append(children, layout.Rigid(view.Message(l.Text(i18n.TransferNoDestination), view.Colors.SecondaryInk)))
			case found && !sourceEditable && p.linked == nil:
				children = append(children, layout.Rigid(view.Message(d.currency.String(), view.Colors.SecondaryInk)))
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			p.syncTransferQuote()
			pickerOrigin := origin.Add(image.Pt(width+gap, 0))
			zone := d.sources.Location
			withQuote := d.fx.pair != (transferPair{})
			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return d.layoutMovementTime(gtx, view, pickerOrigin, now, withQuote)
				}),
			}
			if transfer || conversion {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					options := ui.FieldOptions{Label: i18n.TransferFeeInput}
					if found && (crossCurrency || sourceEditable || linkedTransfer) {
						return d.fee.LayoutCurrency(gtx, view, options, d.currency.String())
					}
					return d.fee.Layout(gtx, view, options)
				}))
			}
			children = append(children, layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.ContributionTimezone), zone), view.Colors.SecondaryInk)))
			if p.linked != nil {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if d.useNow.Clicked(gtx) {
						d.captureCurrentTime(now)
						gtx.Execute(op.InvalidateCmd{})
					}
					return view.Button(&d.useNow, l.Text(i18n.LiabilityUseNow))(gtx)
				}))
			}
			if withQuote {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.layoutTransferQuote(gtx, view)
				}))
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
		}),
	)
}

func checkpointCashReasonLabel(reason domain.ActivityReason) i18n.MessageID {
	if label, found := liabilityReasonLabels[reason]; found {
		return label
	}
	return repeatReasonLabels[reason]
}

func (p *historyCashMovements) layoutCashReason(gtx layout.Context, view ui.View, at image.Point) layout.Dimensions {
	d := p.draft
	reasons := domain.MoneyOutReasons()
	liability := p.linked != nil && p.linked.liability
	switch {
	case d.movement == history.CashValueUpdate && liability:
		return view.Message(liabilityReasonFact(view.Language, domain.ReasonReconciliation), view.Colors.Ink)(gtx)
	case d.movement == history.CashValueUpdate:
		reasons = domain.ValueUpdateReasons()
	case liability && d.movement == history.CashContribution:
		reasons = domain.LiabilityIncreaseReasons()
	case liability:
		reasons = domain.LiabilityDecreaseReasons()
	case d.movement == history.CashContribution:
		reasons = domain.MoneyInReasons()
	}
	options := make([]ui.Choice, 0, len(reasons))
	for _, reason := range reasons {
		options = append(options, ui.Choice{ID: string(reason), Label: view.Language.Text(checkpointCashReasonLabel(reason))})
	}
	d.reasonPicker.Layout(gtx, view, ui.PickerField{Label: i18n.HistoryRepeatReason, Origin: at, Disabled: !gtx.Enabled(), Next: &d.datePicker.Trigger},
		ui.SelectOptions{Items: options}, string(d.reason), func(id string) {
			d.reason, d.problem = domain.ActivityReason(id), i18n.NoMessage
		})
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
}

func (d *cashMovementDraft) layoutMovementTime(gtx layout.Context, view ui.View, origin image.Point, now time.Time, sideBySide bool) layout.Dimensions {
	zone := d.sources.Location
	today := now.In(zone).Format(time.DateOnly)
	weekStart := time.Monday
	if view.WeekStart == "sunday" {
		weekStart = time.Sunday
	}
	dates := ui.DateOptions{Min: d.sources.Origin.StartedAt.In(zone).Format(time.DateOnly), Today: today, Max: today, WeekStart: weekStart, Format: view.DateFormat}
	next := any(&d.preview)
	if d.movement == history.CashTransfer || d.movement == history.CashConversion {
		next = &d.fee.Editor
	} else if d.secondsPicker {
		next = &d.useNow
	}
	drawDay := func(gtx layout.Context) layout.Dimensions {
		field := ui.PickerField{Label: i18n.PreviewEffectiveDate, Origin: origin, Disabled: !gtx.Enabled(), Next: &d.timePicker.Trigger}
		d.datePicker.Layout(gtx, view, field, dates, d.day, func(day string) {
			d.day, d.problem, d.timeEdited = day, i18n.NoMessage, true
		})
		if sideBySide {
			origin.X += gtx.Constraints.Max.X + gtx.Dp(8)
		} else {
			origin.Y += gtx.Dp(68)
		}
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
	}
	drawTime := func(gtx layout.Context) layout.Dimensions {
		field := ui.PickerField{Label: i18n.PreviewEffectiveTime, Origin: origin, Disabled: !gtx.Enabled(), Previous: &d.datePicker.Trigger, Next: next}
		setTime := func(clock string) { d.time, d.problem, d.timeEdited = clock, i18n.NoMessage, true }
		if !d.secondsPicker {
			d.timePicker.Layout(gtx, view, field, d.time, view.TimeFormat == settings.TimeFormat12, false, setTime)
		} else {
			d.timePicker.LayoutSeconds(gtx, view, field, d.time, view.TimeFormat == settings.TimeFormat12, setTime)
		}
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
	}
	if sideBySide {
		return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx, layout.Flexed(1, drawDay), layout.Flexed(1, drawTime))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, layout.Rigid(drawDay), layout.Rigid(drawTime))
}

func layoutCashCurrencyAmount(gtx layout.Context, view ui.View, at image.Point, amount *ui.TextField, label i18n.MessageID, picker *ui.Select, currencyLabel i18n.MessageID, currency, excluded domain.CurrencyCode, choose func(domain.CurrencyCode)) layout.Dimensions {
	items := supportedCurrencyChoices
	if excluded != "" {
		items = make([]ui.Choice, 0, len(supportedCurrencyChoices)-1)
		for _, choice := range supportedCurrencyChoices {
			if choice.ID != excluded.String() {
				items = append(items, choice)
			}
		}
	}
	drawPicker := func(gtx layout.Context, origin image.Point) layout.Dimensions {
		field := ui.PickerField{Label: currencyLabel, Origin: origin, Disabled: !gtx.Enabled()}
		picker.Layout(gtx, view, field, ui.SelectOptions{Items: items, Searchable: true}, currency.String(), func(value string) {
			choose(domain.CurrencyCode(value))
		})
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
	}
	if amount == nil {
		return drawPicker(gtx, at)
	}
	width := gtx.Dp(125)
	widthDelta := gtx.Constraints.Max.X - width
	return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			options := ui.FieldOptions{Label: label}
			if currency != "" {
				options.AccessibleName = view.Language.Text(label) + " · " + currency.String()
			}
			return amount.Layout(gtx, view, options)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			pickerOrigin := at.Add(image.Pt(widthDelta, 0))
			return drawPicker(gtx, pickerOrigin)
		}),
	)
}

func contributionTimestamp(view ui.View, instant time.Time, zone *time.Location) string {
	value := format.LocalDateTime(instant.In(zone), settings.Settings{DateFormat: view.DateFormat, TimeFormat: view.TimeFormat}, settings.Language(view.Language))
	return fmt.Sprintf(view.Language.Text(i18n.ContributionEffective), value, zone)
}

func (p *historyCashMovements) layoutReview(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.draft, view.Language
	labels := p.formLabels()
	review := d.review
	if !d.uncertain && d.posting == nil && d.edit.Clicked(gtx) {
		d.review, d.focusAmount = nil, true
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if !d.uncertain && d.posting == nil && d.cancel.Clicked(gtx) {
		p.discard()
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if d.posting == nil && d.post.Clicked(gtx) {
		p.post()
		gtx.Execute(op.InvalidateCmd{})
	}
	stamp := contributionTimestamp(view, review.Preview.Activity.EffectiveAt, review.Timezone)
	if p.linked != nil {
		stamp = tradeTimestamp(view, review.Preview.Activity.EffectiveAt, review.Timezone)
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if d.focusReview && gtx.Enabled() {
				gtx.Execute(key.FocusCmd{Tag: &d.post})
				d.focusReview = false
			}
			return view.Label(gtx, l.Text(labels.review), 19, font.Medium, view.Colors.Ink)
		}),
		layout.Rigid(view.Message(stamp, view.Colors.SecondaryInk)),
	}
	if p.linked != nil {
		if d.movement == history.CashContribution || d.movement == history.CashWithdrawal || d.movement == history.CashValueUpdate {
			children = append(children, layout.Rigid(view.Message(l.Text(checkpointCashReasonLabel(review.Preview.Activity.Reason)), view.Colors.Ink)))
		}
		if note := review.Preview.Activity.Note; note != nil && *note != "" {
			children = append(children, layout.Rigid(view.Message(*note, view.Colors.Ink)))
		}
	}
	var endpoints []layout.FlexChild
	for i, item := range review.Preview.Review {
		currency := item.Currency.String()
		title := item.AccountName
		if d.movement == history.CashTransfer || d.movement == history.CashConversion {
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
	if d.movement == history.CashTransfer || d.movement == history.CashConversion {
		from, to := transferCurrencies(review.Preview.Effects)
		crossCurrency := from != to
		for _, effect := range review.Preview.Effects {
			var label i18n.MessageID
			switch effect.Role {
			case domain.EffectRoleTransferFrom:
				label = i18n.TransferPrincipal
				if crossCurrency {
					label = i18n.TransferSentPrincipal
				}
			case domain.EffectRoleTransferTo:
				if crossCurrency {
					label = i18n.TransferReceivedPrincipal
				}
			case domain.EffectRoleFee:
				label = i18n.TransferFee
			}
			if label != i18n.NoMessage {
				children = append(children, layout.Rigid(view.Message(fmt.Sprintf(l.Text(label), movementMoney(effect)), view.Colors.Ink)))
			}
		}
		if rate := transferRateLabel(view, review.Preview.DerivedRate, from, to); rate != "" {
			children = append(children, layout.Rigid(view.Message(rate, view.Colors.Ink)))
		}
	}
	if d.movement.Debt() {
		for _, effect := range review.Preview.Effects {
			switch effect.Role {
			case domain.EffectRolePrincipal:
				children = append(children, layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.LiabilityAmountFact), movementMoney(effect)), view.Colors.Ink)))
			case domain.EffectRoleFee:
				children = append(children, layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.LiabilityInterestFact), movementMoney(effect)), view.Colors.Ink)))
			}
		}
	}
	if d.uncertain {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.ContributionUncertain), view.Colors.ErrorInk)))
	}
	if d.problem == i18n.CashReceiptSaveFailed {
		children = append(children, layout.Rigid(view.Message(l.Text(d.problem), view.Colors.ErrorInk)))
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

func transferCurrencies(effects []domain.ActivityEffect) (domain.CurrencyCode, domain.CurrencyCode) {
	var sent, received domain.CurrencyCode
	for _, effect := range effects {
		if effect.Money == nil {
			continue
		}
		switch effect.Role {
		case domain.EffectRoleTransferFrom:
			sent = effect.Money.Currency()
		case domain.EffectRoleTransferTo:
			received = effect.Money.Currency()
		}
	}
	return sent, received
}

func transferRateLabel(view ui.View, rate *domain.FxRate, sent, received domain.CurrencyCode) string {
	if rate == nil || sent == "" || received == "" || sent == received {
		return ""
	}
	return fmt.Sprintf(view.Language.Text(i18n.TransferExecutedRate), sent, rate.Canonical(), received)
}

func movementMoney(effect domain.ActivityEffect) string {
	if effect.Money == nil {
		return ""
	}
	return effect.Money.CanonicalAmount() + " " + effect.Money.Currency().String()
}
