package desktop

import (
	"fmt"
	"image"
	"slices"
	"strings"
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

func (f *accountInvestment) layoutForm(gtx layout.Context, view ui.View, run investmentRuntime) layout.Dimensions {
	l := view.Language
	d := f.draft
	labels := d.labels()
	if !f.unresolved() && d.cancel.Clicked(gtx) {
		f.draft = nil
		f.restore = f.action(d)
		view.RestorePageScroll(0)
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	gtx, view = view.Measure(gtx, 690)
	if d.review != nil {
		return f.layoutReview(gtx, view, run)
	}
	if d.sourceProblem != i18n.NoMessage && d.retry.Clicked(gtx) {
		f.loadSources(run)
		gtx.Execute(op.InvalidateCmd{})
	}
	if d.previewing == nil && d.preview.Clicked(gtx) {
		f.preview(run)
		gtx.Execute(op.InvalidateCmd{})
	}
	if d.sources != nil && d.previewing == nil && d.useNow.Clicked(gtx) {
		d.captureTime(run.clock())
		d.problem = i18n.NoMessage
		gtx.Execute(op.InvalidateCmd{})
	}
	origin := view.Origin
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, tradeTitle(view, labels.action))
		}),
	}
	if d.custody != nil {
		children = append(children, f.custodyFields(view, &origin)...)
	} else if d.adjustment != nil {
		children = append(children, f.adjustmentFields(view, &origin)...)
	} else if d.pickAccount {
		if len(d.accounts) != 0 {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return fieldPosition(gtx, &origin, f.layoutImportAccount(view, origin, run))
			}))
		}
	} else {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, view.Message(d.account.Name+" · "+d.account.DefaultCurrency.String(), view.Colors.Ink))
		}))
	}
	if d.custody == nil && d.adjustment == nil && d.sources != nil && len(d.sources.Instruments) != 0 {
		if d.kind == domain.ActivityPositionTransfer {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return fieldPosition(gtx, &origin, view.Message(l.Text(i18n.PositionImportHelp), view.Colors.SecondaryInk))
			}))
		}
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return fieldPosition(gtx, &origin, d.layoutInvestmentInstrument(view, origin))
			}),
		)
		if (d.kind == domain.ActivityBuy || d.kind == domain.ActivitySell) && d.currency != "" && d.currency != d.account.DefaultCurrency {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return fieldPosition(gtx, &origin, view.Message(fmt.Sprintf(l.Text(i18n.TradeForeignSettlement), d.currency.String()), view.Colors.SecondaryInk))
			}))
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, d.layoutInvestmentAmounts(view))
		}))
	}
	if d.investmentFieldsVisible() {
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return d.layoutInvestmentTime(gtx, view, origin, run.clock())
			}),
			layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.ContributionTimezone), d.sources.Location), view.Colors.SecondaryInk)),
		)
		if d.kind != domain.ActivityCashDividend {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.W.Layout(gtx, view.Button(&d.useNow, l.Text(i18n.LiabilityUseNow)))
			}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return d.note.Layout(gtx, view, ui.FieldOptions{Label: i18n.PreviewNotes, Hint: i18n.PreviewNotesHint, Disabled: d.previewing != nil})
				}))
		}
	}
	switch {
	case d.loading != nil || d.previewing != nil || d.custody != nil && d.custody.loading != nil ||
		d.adjustment != nil && d.adjustment.loading != nil:
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)))
	case d.sourceProblem != i18n.NoMessage:
		children = append(children, layout.Rigid(view.Message(l.Text(d.sourceProblem), view.Colors.ErrorInk)))
	case d.sources != nil && !d.investmentFieldsVisible():
		children = append(children, layout.Rigid(view.Message(l.Text(labels.empty), view.Colors.SecondaryInk)))
	}
	if d.problem != i18n.NoMessage && d.problem != d.sourceProblem {
		children = append(children, layout.Rigid(view.Message(l.Text(d.problem), view.Colors.ErrorInk)))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		buttons := []layout.FlexChild{layout.Rigid(view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel)))}
		if d.sourceProblem != i18n.NoMessage {
			buttons = append(buttons, layout.Rigid(view.Button(&d.retry, l.Text(i18n.HistoryRetry))))
		} else if d.investmentFieldsVisible() {
			buttons = append(buttons, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if d.previewing != nil || !d.investmentInputComplete() {
					gtx = gtx.Disabled()
				}
				return view.PrimaryButton(&d.preview, l.Text(labels.review))(gtx)
			}))
		}
		return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx, buttons...)
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}

func (d *accountInvestmentDraft) investmentFieldsVisible() bool {
	if d.custody != nil {
		return d.custody.available()
	}
	if d.adjustment != nil {
		return d.adjustment.sources != nil && len(d.adjustment.sources.Holdings) != 0
	}
	return d.sources != nil && len(d.sources.Instruments) != 0
}

func (d *accountInvestmentDraft) investmentInputComplete() bool {
	quantity := strings.TrimSpace(d.quantity.Text()) != ""
	if d.custody != nil {
		return quantity && d.custody.from != "" && d.custody.to != ""
	}
	if d.adjustment != nil {
		return quantity && d.adjustment.holding != ""
	}
	if d.instrument == "" {
		return false
	}
	switch d.kind {
	case domain.ActivityPositionTransfer:
		return quantity && strings.TrimSpace(d.unitCost.Text()) != ""
	case domain.ActivityCashDividend:
		return strings.TrimSpace(d.gross.Text()) != ""
	default:
		return quantity && strings.TrimSpace(d.gross.Text()) != ""
	}
}

func (f *accountInvestment) layoutImportAccount(view ui.View, origin image.Point, run investmentRuntime) layout.Widget {
	d := f.draft
	items := make([]ui.Choice, 0, len(d.accounts))
	for _, account := range d.accounts {
		items = append(items, ui.Choice{ID: account.ID.String(), Label: account.Name + " · " + account.DefaultCurrency.String()})
	}
	return func(gtx layout.Context) layout.Dimensions {
		if d.focusAccount && gtx.Enabled() {
			gtx.Execute(key.FocusCmd{Tag: &d.selectAccount.Trigger})
			d.focusAccount = false
		}
		field := ui.PickerField{Label: i18n.PositionImportAccount, Origin: origin, Disabled: d.previewing != nil, Next: &d.selectInstrument.Trigger}
		d.selectAccount.Layout(gtx, view, field, ui.SelectOptions{Items: items, Searchable: true}, d.account.ID.String(), func(id string) {
			if id == d.account.ID.String() {
				return
			}
			at := slices.IndexFunc(d.accounts, func(account domain.Account) bool { return account.ID.String() == id })
			if at < 0 {
				return
			}
			d.account, d.focusInput = d.accounts[at], true
			d.chooseInstrument("")
			f.loadSources(run)
		})
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
	}
}

func (d *accountInvestmentDraft) layoutInvestmentInstrument(view ui.View, origin image.Point) layout.Widget {
	items := make([]ui.Choice, 0, len(d.sources.Instruments))
	for _, item := range d.sources.Instruments {
		label := item.SelectionLabel()
		if holding, ok := d.sources.Holdings[item.ID]; ok {
			label += " · " + fmt.Sprintf(view.Language.Text(i18n.AccountQuantity), portfolioDecimal(view, holding.Quantity.Canonical()))
		}
		items = append(items, ui.Choice{ID: item.ID.String(), Label: label, SearchText: item.Name + " " + label})
	}
	return func(gtx layout.Context) layout.Dimensions {
		if d.focusInput && d.instrument == "" && gtx.Enabled() {
			gtx.Execute(key.FocusCmd{Tag: &d.selectInstrument.Trigger})
			d.focusInput = false
		}
		field := ui.PickerField{Label: i18n.BuyInstrument, Origin: origin, Disabled: d.previewing != nil, Next: &d.quantity.Editor}
		if d.kind == domain.ActivityCashDividend {
			field.Next = &d.gross.Editor
		}
		d.selectInstrument.Layout(gtx, view, field, ui.SelectOptions{Items: items, Searchable: true}, d.instrument.String(), func(id string) {
			d.chooseInstrument(domain.InstrumentID(id))
		})
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
	}
}

func (d *accountInvestmentDraft) layoutInvestmentAmounts(view ui.View) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if d.focusInput && d.instrument != "" && gtx.Enabled() {
			if d.kind == domain.ActivityCashDividend {
				d.gross.Focus(gtx)
			} else {
				d.quantity.Focus(gtx)
			}
			d.focusInput = false
		}
		currency := d.currency.String()
		if d.kind == domain.ActivityCashDividend {
			return d.gross.LayoutCurrency(gtx, view, ui.FieldOptions{
				Label: i18n.DividendAmount, Help: i18n.DividendNetHint, Disabled: d.previewing != nil,
			}, currency)
		}
		labels := d.labels()
		disabled := d.previewing != nil || d.instrument == ""
		if currency == "" {
			currency = d.account.DefaultCurrency.String()
		}
		count := 3
		if d.kind == domain.ActivityPositionTransfer {
			count = 2
		}
		return (ui.Grid{MinColumn: 190, MaxColumns: count, Gap: 8}).Layout(gtx, view, count, func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
			switch i {
			case 0:
				return d.quantity.Layout(gtx, view, ui.FieldOptions{Label: labels.quantity, Disabled: disabled})
			case 1:
				if d.kind == domain.ActivityPositionTransfer {
					return d.unitCost.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.HistoryStartOriginalCost, Disabled: disabled}, currency)
				}
				return d.gross.LayoutCurrency(gtx, view, ui.FieldOptions{Label: labels.gross, Disabled: disabled}, currency)
			default:
				return d.fee.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.BuyFee, Disabled: disabled}, currency)
			}
		})
	}
}

func (d *accountInvestmentDraft) layoutInvestmentTime(gtx layout.Context, view ui.View, origin image.Point, now time.Time) layout.Dimensions {
	zone := d.sources.Location
	today := now.In(zone).Format(time.DateOnly)
	weekStart := time.Monday
	if view.WeekStart == "sunday" {
		weekStart = time.Sunday
	}
	dates := ui.DateOptions{Min: d.sources.Origin.StartedAt.In(zone).Format(time.DateOnly), Today: today, Max: today, WeekStart: weekStart, Format: view.DateFormat}
	var timeOrigin image.Point
	return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			timeOrigin = origin.Add(image.Pt(gtx.Constraints.Max.X+gtx.Dp(8), 0))
			field := ui.PickerField{Label: i18n.PreviewEffectiveDate, Origin: origin, Disabled: d.previewing != nil, Next: &d.timePicker.Trigger}
			d.datePicker.Layout(gtx, view, field, dates, d.day, func(day string) {
				if day != d.day {
					d.defaultTime = time.Time{}
				}
				d.day, d.problem = day, i18n.NoMessage
			})
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			field := ui.PickerField{Label: i18n.PreviewEffectiveTime, Origin: timeOrigin, Disabled: d.previewing != nil, Previous: &d.datePicker.Trigger, Next: &d.preview}
			setTime := func(value string) {
				if value != d.time {
					d.defaultTime = time.Time{}
				}
				d.time, d.problem = value, i18n.NoMessage
			}
			if d.kind == domain.ActivityCashDividend {
				d.timePicker.Layout(gtx, view, field, d.time, view.TimeFormat == settings.TimeFormat12, false, setTime)
			} else {
				field.Next = &d.useNow
				d.timePicker.LayoutSeconds(gtx, view, field, d.time, view.TimeFormat == settings.TimeFormat12, setTime)
			}
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		}),
	)
}

func (f *accountInvestment) layoutReview(gtx layout.Context, view ui.View, run investmentRuntime) layout.Dimensions {
	d, l := f.draft, view.Language
	labels := d.labels()
	if !f.unresolved() && d.edit.Clicked(gtx) {
		d.review, d.focusInput = nil, true
		view.RestorePageScroll(0)
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	confirm := &d.confirm
	if d.uncertain {
		confirm = &d.retryConfirm
	}
	if d.posting == nil && confirm.Clicked(gtx) {
		f.post(run)
		gtx.Execute(op.InvalidateCmd{})
	}
	if d.focusReview && gtx.Enabled() {
		gtx.Execute(key.FocusCmd{Tag: confirm})
		view.RestorePageScroll(0)
		d.focusReview = false
	}
	review := d.review
	preview := review.Preview
	contextName := d.account.Name
	if d.custody != nil {
		if source, ok := d.custody.source(); ok {
			contextName = source.Instrument.SelectionLabel()
		}
	} else if d.adjustment != nil {
		if item, ok := d.adjustment.selected(); ok {
			contextName = item.Instrument.SelectionLabel() + " · " + item.Account.Name
		}
	} else if at := slices.IndexFunc(d.sources.Instruments, func(i domain.Instrument) bool { return i.ID == d.instrument }); at >= 0 {
		contextName += " · " + d.sources.Instruments[at].SelectionLabel()
	}
	timestamp := contributionTimestamp(view, preview.Activity.EffectiveAt, review.Timezone)
	if d.kind != domain.ActivityCashDividend {
		timestamp = tradeTimestamp(view, preview.Activity.EffectiveAt, review.Timezone)
	}
	children := []layout.FlexChild{
		layout.Rigid(tradeTitle(view, labels.review)),
		layout.Rigid(view.Message(contextName, view.Colors.Ink)),
		layout.Rigid(view.Message(timestamp, view.Colors.SecondaryInk)),
	}
	fact := func(id i18n.MessageID, value string) layout.FlexChild {
		return layout.Rigid(view.Message(fmt.Sprintf(l.Text(id), value), view.Colors.Ink))
	}
	switch d.kind {
	case domain.ActivityCashDividend:
		for _, endpoint := range preview.Review {
			money := func(amount string) string { return portfolioDecimal(view, amount) + " " + endpoint.Currency.String() }
			children = append(children,
				fact(i18n.ContributionBefore, money(endpoint.Before)),
				fact(i18n.DividendNetCredit, money(endpoint.Delta)),
				fact(i18n.ContributionAfter, money(endpoint.After)))
		}
	case domain.ActivityPositionTransfer:
		if d.custody != nil {
			children = append(children, d.custody.reviewFacts(view, preview, fact)...)
			break
		}
		if d.adjustment != nil {
			children = append(children, d.adjustment.reviewFacts(view, preview, fact)...)
			break
		}
		for _, endpoint := range preview.Resulting {
			if endpoint.Target == domain.EffectTargetHoldingQuantity {
				children = append(children, fact(i18n.AccountQuantity, portfolioDecimal(view, endpoint.Quantity)))
			}
		}
		if preview.DerivedUnitPrice != nil {
			cost := portfolioDecimal(view, preview.DerivedUnitPrice.Canonical()) + " " + d.currency.String()
			children = append(children, fact(i18n.PositionImportCostFact, cost))
		}
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.PositionImportNoCash), view.Colors.SecondaryInk)))
	default:
		children = append(children, d.investmentTradeFacts(gtx, view, fact)...)
	}
	if note := preview.Activity.Note; note != nil {
		children = append(children, layout.Rigid(view.Message(*note, view.Colors.Ink)))
	}
	if d.uncertain {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.ContributionUncertain), view.Colors.ErrorInk)))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layoutTargetBalanceConfirmation(gtx, view, d.posting != nil, d.uncertain, &d.cancel, &d.edit, confirm, labels.confirm)
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(5)}.Layout(gtx, children...)
}

func (d *accountInvestmentDraft) investmentTradeFacts(gtx layout.Context, view ui.View, fact func(i18n.MessageID, string) layout.FlexChild) []layout.FlexChild {
	preview, review, l := d.review.Preview, d.review, view.Language
	trade := preview.Activity.TradeDetail
	currency := trade.Gross.Currency()
	money := func(amount string) string { return portfolioDecimal(view, amount) + " " + currency.String() }
	fee := "0"
	if trade.Fee != nil {
		fee = trade.Fee.CanonicalAmount()
	}
	entryFacts := []layout.FlexChild{
		fact(i18n.AccountQuantity, portfolioDecimal(view, trade.Quantity.Canonical())),
		fact(i18n.BuyExecutionPrice, money(trade.UnitPrice.Canonical())),
		fact(d.labels().grossFact, money(trade.Gross.CanonicalAmount())),
		fact(i18n.BuyFeeFact, money(fee)),
	}
	var ledgerFacts []layout.FlexChild
	cost := review.ExistingCost
	existingBuy := cost != nil && trade.Side == domain.TradeBuy
	switch {
	case trade.Side == domain.TradeSell:
		for _, endpoint := range preview.Review {
			if endpoint.Target == domain.EffectTargetHoldingQuantity {
				ledgerFacts = append(ledgerFacts, fact(i18n.SellRemainingQuantity, portfolioDecimal(view, endpoint.After)))
			}
		}
		remaining, realized := l.Text(i18n.OverviewNoValue), l.Text(i18n.OverviewNoValue)
		if cost != nil && cost.RemainingCost != nil {
			remaining = money(cost.RemainingCost.Amount)
		}
		if cost != nil && cost.RealizedGain != nil {
			realized = money(cost.RealizedGain.Amount)
		}
		ledgerFacts = append(ledgerFacts, fact(i18n.SellRemainingCost, remaining), fact(i18n.SellRealizedGain, realized))
	case existingBuy:
		for _, endpoint := range preview.Review {
			if endpoint.Target == domain.EffectTargetHoldingQuantity {
				ledgerFacts = append(ledgerFacts,
					fact(i18n.BuyBeforeQuantity, portfolioDecimal(view, endpoint.Before)),
					fact(i18n.BuyAfterQuantity, portfolioDecimal(view, endpoint.After)))
			}
		}
		before, after, average := l.Text(i18n.OverviewNoValue), l.Text(i18n.OverviewNoValue), l.Text(i18n.OverviewNoValue)
		if cost.PreviousCost != nil {
			before = money(cost.PreviousCost.Amount)
		}
		if cost.RemainingCost != nil {
			after = money(cost.RemainingCost.Amount)
		}
		if cost.AverageCost != nil {
			average = money(cost.AverageCost.Canonical())
		}
		ledgerFacts = append(ledgerFacts, fact(i18n.BuyPreviousCost, before), fact(i18n.BuyTotalCost, after), fact(i18n.BuyAverageCost, average))
		entryFacts = append(entryFacts, fact(i18n.BuyLotCost, money(review.UnitCost.Canonical())))
	default:
		entryFacts = append(entryFacts, fact(i18n.BuyCostBasis, money(review.UnitCost.Canonical())))
	}
	for _, endpoint := range preview.Review {
		if endpoint.Target == domain.EffectTargetAccountCash {
			delta := endpoint.Delta
			if delta != "" && delta != "0" && !strings.HasPrefix(delta, "-") {
				delta = "+" + delta
			}
			ledgerFacts = append(ledgerFacts,
				fact(i18n.ContributionBefore, money(endpoint.Before)),
				fact(i18n.BuyCashDelta, money(delta)),
				fact(i18n.ContributionAfter, money(endpoint.After)))
		}
	}
	if existingBuy {
		return []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			stack := layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(5)}
			return layout.Flex{Gap: gtx.Dp(14)}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return stack.Layout(gtx, entryFacts...) }),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return stack.Layout(gtx, ledgerFacts...) }))
		})}
	}
	return append(entryFacts, ledgerFacts...)
}

func tradeTitle(view ui.View, id i18n.MessageID) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return view.Label(gtx, view.Language.Text(id), 19, font.Medium, view.Colors.Ink)
	}
}

func tradeTimestamp(view ui.View, instant time.Time, zone *time.Location) string {
	value := format.LocalDateTimeSeconds(instant.In(zone), settings.Settings{DateFormat: view.DateFormat, TimeFormat: view.TimeFormat}, settings.Language(view.Language))
	return fmt.Sprintf(view.Language.Text(i18n.ContributionEffective), value, zone)
}
