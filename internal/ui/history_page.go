package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/waltwang/nestworth-go/internal/domain"
)

const (
	historyMoneyAdded       = "money_added"
	historyMoneyRemoved     = "money_removed"
	historyCashTransfer     = "cash_transfer"
	historyPositionTransfer = "position_transfer"
	historyTrade            = "trade"
	historyValueUpdate      = "value_update"
	historyDebt             = "debt"
)

// Stable side values for the trade/debt side select; never compare its
// translated labels.
const (
	historySideBuy     = "buy"
	historySideSell    = "sell"
	historySideDraw    = "draw"
	historySidePayment = "payment"
)

type historyHoldingOption struct {
	holding    domain.Holding
	instrument domain.Instrument
	label      string
}

// NewHistoryPage keeps the form family-facing while every command is still
// constructed and validated by the application/domain boundary.
func NewHistoryPage(c *Controller) fyne.CanvasObject {
	t := c.translator
	if c.service == nil {
		return NewComingSoonPage(c, "page.historyTitle", "page.historyDescription", theme.IconNameHistory)
	}
	ctx := context.Background()
	started, startErr := c.service.HistoryStarted(ctx)
	if startErr != nil {
		return surface(widget.NewLabel(t.TranslateError(startErr)), fyne.NewSize(520, 220))
	}
	accounts, err := c.service.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil {
		return surface(widget.NewLabel(t.TranslateError(err)), fyne.NewSize(520, 220))
	}
	instruments, err := c.service.ListInstruments(ctx, false)
	if err != nil {
		return surface(widget.NewLabel(t.TranslateError(err)), fyne.NewSize(520, 220))
	}
	instrumentByID := make(map[domain.InstrumentID]domain.Instrument, len(instruments))
	for _, instrument := range instruments {
		instrumentByID[instrument.ID] = instrument
	}
	accountsByLabel := make(map[string]domain.AccountRecord, len(accounts))
	accountLabels := make([]string, 0, len(accounts))
	for _, account := range accounts {
		label := fmt.Sprintf("%s · %s", account.Account.Name, account.Account.DefaultCurrency.String())
		accountLabels = append(accountLabels, label)
		accountsByLabel[label] = account
	}
	trackingIDs := make([]domain.AccountID, 0, len(accounts))
	trackingByID := make(map[domain.AccountID]domain.AccountRecord, len(accounts))
	for _, account := range accounts {
		if account.Account.TrackingMode != domain.TrackingHoldings {
			continue
		}
		trackingIDs = append(trackingIDs, account.Account.ID)
		trackingByID[account.Account.ID] = account
	}
	holdingsByAccount, holdingsErr := c.service.HoldingsByAccounts(ctx, trackingIDs)
	if holdingsErr != nil {
		return surface(widget.NewLabel(t.TranslateError(holdingsErr)), fyne.NewSize(520, 220))
	}
	holdings := make([]historyHoldingOption, 0)
	holdingByLabel := make(map[string]historyHoldingOption)
	for accountID, list := range holdingsByAccount {
		account, ok := trackingByID[accountID]
		if !ok {
			continue
		}
		for _, holding := range list {
			instrument, ok := instrumentByID[holding.InstrumentID]
			if !ok {
				continue
			}
			option := historyHoldingOption{holding: holding, instrument: instrument, label: fmt.Sprintf("%s · %s", instrument.Name, account.Account.Name)}
			holdings = append(holdings, option)
			holdingByLabel[option.label] = option
		}
	}

	kindKeys := []string{historyMoneyAdded, historyMoneyRemoved, historyCashTransfer, historyPositionTransfer, historyTrade, historyValueUpdate, historyDebt}
	kindLabels := make([]string, 0, len(kindKeys))
	kindByLabel := make(map[string]string, len(kindKeys))
	for _, key := range kindKeys {
		label := t.T("history.kind." + key)
		kindLabels = append(kindLabels, label)
		kindByLabel[label] = key
	}
	kindSelect := widget.NewSelect(kindLabels, nil)
	if c.historyRecordKind != "" {
		for label, key := range kindByLabel {
			if key == c.historyRecordKind {
				kindSelect.SetSelected(label)
			}
		}
	}
	if kindSelect.Selected == "" && len(kindLabels) > 0 {
		kindSelect.SetSelected(kindLabels[0])
	}
	if c.historyRecordKind == "" {
		c.historyRecordKind = kindByLabel[kindSelect.Selected]
	}
	accountSelect := widget.NewSelect(accountLabels, nil)
	if len(accountLabels) > 0 {
		accountSelect.SetSelected(accountLabels[0])
	}
	destinationSelect := widget.NewSelect(accountLabels, nil)
	if len(accountLabels) > 1 {
		destinationSelect.SetSelected(accountLabels[1])
	} else if len(accountLabels) > 0 {
		destinationSelect.SetSelected(accountLabels[0])
	}
	holdingLabels := make([]string, 0, len(holdings))
	for _, holding := range holdings {
		holdingLabels = append(holdingLabels, holding.label)
	}
	fromHoldingSelect := widget.NewSelect(holdingLabels, nil)
	toHoldingSelect := widget.NewSelect(holdingLabels, nil)
	if len(holdingLabels) > 0 {
		fromHoldingSelect.SetSelected(holdingLabels[0])
		toHoldingSelect.SetSelected(holdingLabels[0])
	}
	side := newValueSelect(nil)
	reason := newValueSelect(historyReasonOptions(t))
	reason.Select(string(domain.ReasonIncome))
	amount := widget.NewEntry()
	amount.SetPlaceHolder("0.00")
	secondaryAmount := widget.NewEntry()
	secondaryAmount.SetPlaceHolder("0.00")
	quantity := widget.NewEntry()
	quantity.SetPlaceHolder("0")
	effectiveDate := widget.NewEntry()
	effectiveDate.SetPlaceHolder("YYYY-MM-DD")
	effectiveTime := widget.NewEntry()
	effectiveTime.SetPlaceHolder("HH:MM")
	note := widget.NewEntry()
	note.SetPlaceHolder(t.T("history.notePlaceholder"))
	feedback := widget.NewLabel("")

	command := func() (any, error) {
		bootstrap, bootstrapErr := c.service.Bootstrap(ctx)
		if bootstrapErr != nil {
			return nil, bootstrapErr
		}
		if bootstrap.Household == nil {
			return nil, &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
		}
		when, whenErr := historyEffectiveAtWithTime(c, effectiveDate.Text, effectiveTime.Text)
		if whenErr != nil {
			return nil, whenErr
		}
		var noteValue *string
		if strings.TrimSpace(note.Text) != "" {
			value := strings.TrimSpace(note.Text)
			noteValue = &value
		}
		kind := kindByLabel[kindSelect.Selected]
		from, fromOK := accountsByLabel[accountSelect.Selected]
		to, toOK := accountsByLabel[destinationSelect.Selected]
		switch kind {
		case historyMoneyAdded, historyMoneyRemoved, historyValueUpdate:
			if !fromOK {
				return nil, historyValidation("account", t.T("history.selectAccount"))
			}
			money, moneyErr := domain.ParseMoney(amount.Text, from.Account.DefaultCurrency)
			if moneyErr != nil {
				return nil, moneyErr
			}
			reasonValue := historySelectedReason(reason)
			if kind == historyMoneyAdded {
				return domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: from.Account.ID, Amount: money, Reason: reasonValue, EffectiveAt: when, Note: noteValue}, nil
			}
			if kind == historyMoneyRemoved {
				return domain.MoneyRemovedInput{HouseholdID: bootstrap.Household.ID, AccountID: from.Account.ID, Amount: money, Reason: reasonValue, EffectiveAt: when, Note: noteValue}, nil
			}
			return domain.ValueUpdateInput{HouseholdID: bootstrap.Household.ID, AccountID: from.Account.ID, NewValue: money, Reason: domain.ReasonReconciliation, EffectiveAt: when, Note: noteValue}, nil
		case historyCashTransfer:
			if !fromOK || !toOK {
				return nil, historyValidation("account", t.T("history.selectAccount"))
			}
			sent, moneyErr := domain.ParseMoney(amount.Text, from.Account.DefaultCurrency)
			if moneyErr != nil {
				return nil, moneyErr
			}
			if from.Account.DefaultCurrency == to.Account.DefaultCurrency {
				return domain.CashTransferInput{HouseholdID: bootstrap.Household.ID, FromAccountID: from.Account.ID, ToAccountID: to.Account.ID, Sent: sent, Received: sent, EffectiveAt: when, Note: noteValue}, nil
			}
			received, receivedErr := domain.ParseMoney(secondaryAmount.Text, to.Account.DefaultCurrency)
			if receivedErr != nil {
				return nil, receivedErr
			}
			return domain.CashTransferInput{HouseholdID: bootstrap.Household.ID, FromAccountID: from.Account.ID, ToAccountID: to.Account.ID, Sent: sent, Received: received, EffectiveAt: when, Note: noteValue}, nil
		case historyPositionTransfer:
			fromHolding, fromHoldingOK := holdingByLabel[fromHoldingSelect.Selected]
			toHolding, toHoldingOK := holdingByLabel[toHoldingSelect.Selected]
			if !fromHoldingOK || !toHoldingOK {
				return nil, historyValidation("holding", t.T("history.selectHolding"))
			}
			parsedQuantity, quantityErr := domain.ParseQuantity(quantity.Text)
			if quantityErr != nil {
				return nil, quantityErr
			}
			return domain.PositionTransferInput{HouseholdID: bootstrap.Household.ID, FromHoldingID: fromHolding.holding.ID, ToHoldingID: toHolding.holding.ID, Quantity: parsedQuantity, EffectiveAt: when, Note: noteValue}, nil
		case historyTrade:
			holding, holdingOK := holdingByLabel[fromHoldingSelect.Selected]
			if !fromOK || !holdingOK {
				return nil, historyValidation("holding", t.T("history.selectHolding"))
			}
			parsedQuantity, quantityErr := domain.ParseQuantity(quantity.Text)
			if quantityErr != nil {
				return nil, quantityErr
			}
			gross, grossErr := domain.ParseMoney(amount.Text, holding.instrument.QuoteCurrency)
			if grossErr != nil {
				return nil, grossErr
			}
			var fee *domain.Money
			if strings.TrimSpace(secondaryAmount.Text) != "" {
				parsedFee, feeErr := domain.ParseMoney(secondaryAmount.Text, gross.Currency())
				if feeErr != nil {
					return nil, feeErr
				}
				fee = &parsedFee
			}
			tradeSide := domain.TradeBuy
			if side.Value() == historySideSell {
				tradeSide = domain.TradeSell
			}
			return domain.TradeInput{HouseholdID: bootstrap.Household.ID, Side: tradeSide, SettlementAccountID: from.Account.ID, HoldingID: holding.holding.ID, InstrumentID: holding.holding.InstrumentID, Quantity: parsedQuantity, Gross: gross, Fee: fee, EffectiveAt: when, Note: noteValue}, nil
		case historyDebt:
			if !fromOK || !toOK {
				return nil, historyValidation("account", t.T("history.selectAccount"))
			}
			principal, principalErr := domain.ParseMoney(amount.Text, from.Account.DefaultCurrency)
			if principalErr != nil {
				return nil, principalErr
			}
			if side.Value() == historySideDraw {
				return domain.DebtDrawInput{HouseholdID: bootstrap.Household.ID, DebtAccountID: from.Account.ID, CashAccountID: to.Account.ID, Principal: principal, EffectiveAt: when, Note: noteValue}, nil
			}
			var fee *domain.Money
			if strings.TrimSpace(secondaryAmount.Text) != "" {
				parsedFee, feeErr := domain.ParseMoney(secondaryAmount.Text, principal.Currency())
				if feeErr != nil {
					return nil, feeErr
				}
				fee = &parsedFee
			}
			return domain.DebtPaymentInput{HouseholdID: bootstrap.Household.ID, DebtAccountID: from.Account.ID, CashAccountID: to.Account.ID, Principal: principal, InterestOrFee: fee, EffectiveAt: when, Note: noteValue}, nil
		default:
			return nil, historyValidation("type", t.T("history.selectType"))
		}
	}

	previewButton := widget.NewButton(t.T("history.preview"), func() {
		input, commandErr := command()
		if commandErr != nil {
			feedback.SetText(t.TranslateError(commandErr))
			return
		}
		preview, previewErr := c.service.PreviewChange(ctx, input)
		if previewErr != nil {
			feedback.SetText(t.TranslateError(previewErr))
			return
		}
		feedback.SetText(historyPreviewText(t, preview))
	})
	saveButton := widget.NewButton(t.T("common.save"), func() {
		input, commandErr := command()
		if commandErr != nil {
			feedback.SetText(t.TranslateError(commandErr))
			return
		}
		if _, saveErr := c.service.RecordChange(ctx, input); saveErr != nil {
			feedback.SetText(t.TranslateError(saveErr))
			return
		}
		feedback.SetText(t.T("common.saved"))
		c.RefreshContent()
	})
	form := widget.NewForm()
	rebuildForm := func() {
		form.Items = nil
		form.Append(t.T("history.type"), kindSelect)
		kind := kindByLabel[kindSelect.Selected]
		switch kind {
		case historyMoneyAdded, historyMoneyRemoved:
			form.Append(t.T("history.account"), accountSelect)
			form.Append(t.T("history.amount"), amount)
			form.Append(t.T("history.reason"), reason.widget)
		case historyValueUpdate:
			form.Append(t.T("history.account"), accountSelect)
			form.Append(t.T("history.amount"), amount)
		case historyCashTransfer:
			form.Append(t.T("history.account"), accountSelect)
			form.Append(t.T("history.destination"), destinationSelect)
			form.Append(t.T("history.amount"), amount)
			from, fromOK := accountsByLabel[accountSelect.Selected]
			to, toOK := accountsByLabel[destinationSelect.Selected]
			if !fromOK || !toOK || from.Account.DefaultCurrency != to.Account.DefaultCurrency {
				form.Append(t.T("history.receivedAmount"), secondaryAmount)
			}
		case historyPositionTransfer:
			form.Append(t.T("history.holding"), fromHoldingSelect)
			form.Append(t.T("history.destinationHolding"), toHoldingSelect)
			form.Append(t.T("history.quantity"), quantity)
		case historyTrade:
			side.SetOptions([]selectOption{
				{value: historySideBuy, label: t.T("history.side.buy")},
				{value: historySideSell, label: t.T("history.side.sell")},
			})
			switch side.Value() {
			case historySideBuy, historySideSell:
			default:
				side.Select(historySideBuy)
			}
			form.Append(t.T("history.account"), accountSelect)
			form.Append(t.T("history.holding"), fromHoldingSelect)
			form.Append(t.T("history.side"), side.widget)
			form.Append(t.T("history.amount"), amount)
			form.Append(t.T("history.fee"), secondaryAmount)
			form.Append(t.T("history.quantity"), quantity)
		case historyDebt:
			side.SetOptions([]selectOption{
				{value: historySideDraw, label: t.T("history.side.draw")},
				{value: historySidePayment, label: t.T("history.side.payment")},
			})
			switch side.Value() {
			case historySideDraw, historySidePayment:
			default:
				side.Select(historySideDraw)
			}
			form.Append(t.T("history.account"), accountSelect)
			form.Append(t.T("history.destination"), destinationSelect)
			form.Append(t.T("history.side"), side.widget)
			form.Append(t.T("history.amount"), amount)
			if side.Value() == historySidePayment {
				form.Append(t.T("history.fee"), secondaryAmount)
			}
		default:
			side.SetOptions(nil)
		}
		form.Append(t.T("history.effectiveDate"), effectiveDate)
		form.Append(t.T("history.effectiveTime"), effectiveTime)
		form.Append(t.T("history.note"), note)
		form.Refresh()
	}
	kindSelect.OnChanged = func(value string) {
		c.historyRecordKind = kindByLabel[value]
		rebuildForm()
	}
	accountSelect.OnChanged = func(string) { rebuildForm() }
	destinationSelect.OnChanged = func(string) { rebuildForm() }
	side.widget.OnChanged = func(string) { rebuildForm() }
	rebuildForm()
	formBox := container.NewVBox(widget.NewLabelWithStyle(t.T("history.recordTitle"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), form, container.NewHBox(previewButton, saveButton), feedback)
	if !started {
		startButton := widget.NewButton(t.T("history.start"), func() {
			showStartingPointDialog(c, func() { c.RefreshContent() }, func(err error) {
				if err != nil {
					feedback.SetText(t.TranslateError(err))
				}
			})
		})
		formBox.Objects = append([]fyne.CanvasObject{widget.NewLabel(t.T("history.startDescription")), startButton}, formBox.Objects...)
		form.Refresh()
	}

	filterAccountLabels := append([]string{t.T("history.filterAll")}, accountLabels...)
	filterAccount := widget.NewSelect(filterAccountLabels, nil)
	if c.historyAccountFilter == "" {
		filterAccount.SetSelected(filterAccountLabels[0])
	} else {
		filterAccount.SetSelected(c.historyAccountFilter)
	}
	filterKindLabels := append([]string{t.T("history.filterAll")}, kindLabels...)
	filterKind := widget.NewSelect(filterKindLabels, nil)
	filterKind.SetSelected(t.T("history.filterAll"))
	for label, key := range kindByLabel {
		if key == c.historyKindFilter {
			filterKind.SetSelected(label)
			break
		}
	}
	fromDate := widget.NewEntry()
	fromDate.SetPlaceHolder("YYYY-MM-DD")
	fromDate.SetText(c.historyFromDate)
	toDate := widget.NewEntry()
	toDate.SetPlaceHolder("YYYY-MM-DD")
	toDate.SetText(c.historyToDate)
	resetTimeline := func() {
		c.historyTimeline = nil
		c.historyTimelineNext = nil
		c.historyTimelineLoaded = false
		c.historyTimelineFetch = false
	}
	filterAccount.OnChanged = func(value string) {
		c.historyAccountFilter = value
		resetTimeline()
		c.RefreshContent()
	}
	filterKind.OnChanged = func(value string) {
		c.historyKindFilter = kindByLabel[value]
		resetTimeline()
		c.RefreshContent()
	}
	applyDateFilter := func() {
		c.historyFromDate = strings.TrimSpace(fromDate.Text)
		c.historyToDate = strings.TrimSpace(toDate.Text)
		resetTimeline()
		c.RefreshContent()
	}
	applyDateButton := widget.NewButton(t.T("history.applyFilters"), applyDateFilter)
	var accountID *domain.AccountID
	if c.historyAccountFilter != "" && c.historyAccountFilter != t.T("history.filterAll") {
		if account, ok := accountsByLabel[c.historyAccountFilter]; ok {
			value := account.Account.ID
			accountID = &value
		}
	}
	if !c.historyTimelineLoaded || c.historyTimelineFetch {
		page, pageErr := c.service.ListActivityPage(ctx, domain.ActivityQuery{AccountID: accountID, Kinds: historyTimelineKinds(c.historyKindFilter), FromLocalDate: c.historyFromDate, ToLocalDate: c.historyToDate, After: c.historyTimelineNext, Limit: 100})
		if pageErr != nil {
			err = pageErr
		} else {
			if !c.historyTimelineLoaded {
				c.historyTimeline = nil
			}
			c.historyTimeline = append(c.historyTimeline, page.Activities...)
			c.historyTimelineNext = page.Next
			c.historyTimelineLoaded = true
		}
		c.historyTimelineFetch = false
	}
	activities := c.historyTimeline
	activityErr := err
	rows := []fyne.CanvasObject{widget.NewLabelWithStyle(t.T("history.timeline"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), container.NewHBox(widget.NewLabel(t.T("history.filterAccount")), filterAccount, widget.NewLabel(t.T("history.filterType")), filterKind), container.NewHBox(widget.NewLabel(t.T("history.fromDate")), fromDate, widget.NewLabel(t.T("history.toDate")), toDate, applyDateButton)}
	if origin, originErr := c.service.HistoryOrigin(ctx); originErr == nil && origin != nil {
		location, locationErr := time.LoadLocation(origin.Timezone)
		if locationErr != nil {
			location = time.UTC
		}
		rows = append(rows, mutedLabel(fmt.Sprintf("%s · %s", t.T("history.startingPoint"), origin.StartedAt.In(location).Format("2006-01-02"))))
	}
	if activityErr != nil {
		rows = append(rows, widget.NewLabel(t.TranslateError(activityErr)))
	} else {
		for _, activity := range activities {
			rows = append(rows, historyActivityRow(c, activity))
		}
		if len(activities) == 0 {
			rows = append(rows, widget.NewLabel(t.T("history.empty")))
		}
		if c.historyTimelineNext != nil {
			rows = append(rows, widget.NewButton(t.T("history.loadMore"), func() { c.historyTimelineFetch = true; c.RefreshContent() }))
		}
	}
	return surface(container.NewVBox(formBox, widget.NewSeparator(), container.NewVBox(rows...)), fyne.NewSize(680, 640))
}

func showStartingPointDialog(c *Controller, success func(), failure func(error)) {
	ctx := context.Background()
	draft, err := c.service.StartingPointDraft(ctx)
	if err != nil {
		failure(err)
		return
	}
	if len(draft) == 0 {
		runBackend(c, func() error {
			_, startErr := c.service.StartHistory(ctx, c.preference.Timezone)
			return startErr
		}, func(startErr error) {
			if startErr != nil {
				failure(startErr)
				return
			}
			success()
		})
		return
	}
	entries := make(map[domain.HoldingID]*widget.Entry, len(draft))
	items := make([]*widget.FormItem, 0, len(draft)+1)
	items = append(items, widget.NewFormItem(c.translator.T("history.startCostDescription"), widget.NewLabel(c.translator.T("history.startCostHelp"))))
	for _, item := range draft {
		entry := widget.NewEntry()
		entry.SetText(item.UnitCost)
		entry.SetPlaceHolder("0.00")
		entries[item.HoldingID] = entry
		items = append(items, widget.NewFormItem(fmt.Sprintf("%s · %s", item.InstrumentName, item.Currency), entry))
	}
	showResponsiveBackendForm(c, c.translator.T("history.startCostTitle"), c.translator.T("common.save"), c.translator.T("common.cancel"), items, fyne.NewSize(620, 480), fyne.NewSize(480, 280), func() error {
		costs := make(map[domain.HoldingID]string, len(entries))
		for holdingID, entry := range entries {
			costs[holdingID] = strings.TrimSpace(entry.Text)
		}
		_, startErr := c.service.StartHistoryWithCosts(ctx, c.preference.Timezone, costs)
		return startErr
	}, success)
}

func historyTimelineKinds(filter string) []domain.ActivityKind {
	switch filter {
	case historyMoneyAdded, domain.ActivityCashIn.String():
		return []domain.ActivityKind{domain.ActivityCashIn}
	case historyMoneyRemoved, domain.ActivityCashOut.String():
		return []domain.ActivityKind{domain.ActivityCashOut}
	case historyCashTransfer:
		return []domain.ActivityKind{domain.ActivityCashTransfer}
	case historyPositionTransfer:
		return []domain.ActivityKind{domain.ActivityPositionTransfer}
	case historyTrade:
		return []domain.ActivityKind{domain.ActivityBuy, domain.ActivitySell}
	case historyValueUpdate:
		return []domain.ActivityKind{domain.ActivityValueUpdate}
	case historyDebt:
		return []domain.ActivityKind{domain.ActivityDebtDraw, domain.ActivityDebtPayment}
	default:
		return nil
	}
}

func historyPreviewText(t interface{ T(string) string }, preview domain.ChangePreview) string {
	value := "0"
	for _, effect := range preview.Effects {
		if effect.Money != nil {
			value = fmt.Sprintf("%s %s", effect.Money.CanonicalAmount(), effect.Money.Currency().String())
			break
		}
		if effect.Quantity != nil {
			value = effect.Quantity.Canonical()
			break
		}
	}
	key := "history.preview.change"
	switch preview.Activity.Kind {
	case domain.ActivityCashIn:
		key = "history.preview.moneyAdded"
	case domain.ActivityCashOut:
		key = "history.preview.moneyRemoved"
	case domain.ActivityCashTransfer:
		key = "history.preview.cashTransfer"
	case domain.ActivityPositionTransfer:
		key = "history.preview.positionTransfer"
	case domain.ActivityBuy, domain.ActivitySell:
		key = "history.preview.trade"
	case domain.ActivityValueUpdate:
		key = "history.preview.valueUpdate"
	case domain.ActivityDebtDraw:
		key = "history.preview.debtDraw"
	case domain.ActivityDebtPayment:
		key = "history.preview.debtPayment"
	}
	return fmt.Sprintf(t.T(key), value)
}

func historyActivityRow(c *Controller, activity domain.Activity) fyne.CanvasObject {
	t := c.translator
	label := widget.NewLabel(fmt.Sprintf("%s · %s · %s", activity.EffectiveLocalDate, historyKindLabel(t, activity.Kind), historyReasonLabel(t, activity.Reason)))
	undo := widget.NewButtonWithIcon(t.T("history.undo"), theme.Current().Icon(theme.IconNameContentUndo), func() {
		if _, err := c.service.UndoChange(context.Background(), activity.ID); err != nil {
			c.validationError = c.translator.TranslateError(err)
		}
		c.RefreshContent()
	})
	undo.Importance = widget.LowImportance
	var fix fyne.CanvasObject
	if _, ok := historyFixCommand(activity); ok {
		fixButton := widget.NewButton(t.T("history.fix"), func() { showHistoryFixDialog(c, activity) })
		fixButton.Importance = widget.LowImportance
		fix = fixButton
	} else {
		fix = mutedLabel(t.T("history.fixUnavailable"))
	}
	return container.NewBorder(nil, nil, nil, container.NewHBox(fix, undo), label)
}

func historyFixCommand(activity domain.Activity) (any, bool) {
	for _, effect := range activity.Effects {
		if effect.AccountID == nil || effect.Money == nil {
			continue
		}
		if activity.Kind == domain.ActivityCashIn {
			return domain.MoneyAddedInput{HouseholdID: activity.HouseholdID, AccountID: *effect.AccountID, Amount: *effect.Money, Reason: activity.Reason, EffectiveAt: activity.EffectiveAt, Note: activity.Note}, true
		}
		if activity.Kind == domain.ActivityCashOut {
			return domain.MoneyRemovedInput{HouseholdID: activity.HouseholdID, AccountID: *effect.AccountID, Amount: *effect.Money, Reason: activity.Reason, EffectiveAt: activity.EffectiveAt, Note: activity.Note}, true
		}
	}
	return nil, false
}

func showHistoryFixDialog(c *Controller, activity domain.Activity) {
	t := c.translator
	_, ok := historyFixCommand(activity)
	if !ok {
		c.setValidationError(t.T("history.fixUnavailable"))
		return
	}
	accounts, err := c.service.ListAccounts(context.Background(), domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		c.setValidationError(t.TranslateError(err))
		return
	}
	accountByLabel := make(map[string]domain.AccountRecord, len(accounts))
	accountLabels := make([]string, 0, len(accounts))
	var originalAccountID domain.AccountID
	var originalAmount domain.Money
	for _, effect := range activity.Effects {
		if effect.AccountID != nil && effect.Money != nil {
			originalAccountID = *effect.AccountID
			originalAmount = *effect.Money
			break
		}
	}
	for _, account := range accounts {
		label := fmt.Sprintf("%s · %s", account.Account.Name, account.Account.DefaultCurrency.String())
		accountByLabel[label] = account
		accountLabels = append(accountLabels, label)
	}
	accountSelect := widget.NewSelect(accountLabels, nil)
	for label, account := range accountByLabel {
		if account.Account.ID == originalAccountID {
			accountSelect.SetSelected(label)
			break
		}
	}
	amount := widget.NewEntry()
	amount.SetText(originalAmount.CanonicalAmount())
	reason := newValueSelect(historyReasonOptions(t))
	reason.Select(string(activity.Reason))
	location, locationErr := time.LoadLocation(c.preference.Timezone)
	if locationErr != nil || c.preference.Timezone == "" {
		location = time.UTC
	}
	localEffective := activity.EffectiveAt.In(location)
	date := widget.NewEntry()
	date.SetText(localEffective.Format("2006-01-02"))
	clock := widget.NewEntry()
	clock.SetText(localEffective.Format("15:04"))
	note := widget.NewEntry()
	if activity.Note != nil {
		note.SetText(*activity.Note)
	}
	items := []*widget.FormItem{
		widget.NewFormItem(t.T("history.account"), accountSelect),
		widget.NewFormItem(t.T("history.amount"), amount),
		widget.NewFormItem(t.T("history.reason"), reason.widget),
		widget.NewFormItem(t.T("history.effectiveDate"), date),
		widget.NewFormItem(t.T("history.effectiveTime"), clock),
		widget.NewFormItem(t.T("history.note"), note),
	}
	showResponsiveBackendForm(c, t.T("history.fix"), t.T("common.save"), t.T("common.cancel"), items, fyne.NewSize(600, 460), fyne.NewSize(460, 300), func() error {
		account, ok := accountByLabel[accountSelect.Selected]
		if !ok {
			return historyValidation("account", t.T("history.selectAccount"))
		}
		money, parseErr := domain.ParseMoney(amount.Text, account.Account.DefaultCurrency)
		if parseErr != nil {
			return parseErr
		}
		when, whenErr := historyEffectiveAtWithTime(c, date.Text, clock.Text)
		if whenErr != nil {
			return whenErr
		}
		var noteValue *string
		if strings.TrimSpace(note.Text) != "" {
			value := strings.TrimSpace(note.Text)
			noteValue = &value
		}
		if !historyFixChanged(activity, account.Account.ID, money, historySelectedReason(reason), when, noteValue) {
			return historyValidation("fix", t.T("history.fixNoChange"))
		}
		if activity.Kind == domain.ActivityCashIn {
			command := domain.MoneyAddedInput{HouseholdID: activity.HouseholdID, AccountID: account.Account.ID, Amount: money, Reason: historySelectedReason(reason), EffectiveAt: when, Note: noteValue}
			_, fixErr := c.service.FixChange(context.Background(), activity.ID, command)
			return fixErr
		} else {
			command := domain.MoneyRemovedInput{HouseholdID: activity.HouseholdID, AccountID: account.Account.ID, Amount: money, Reason: historySelectedReason(reason), EffectiveAt: when, Note: noteValue}
			_, fixErr := c.service.FixChange(context.Background(), activity.ID, command)
			return fixErr
		}
	}, func() {
		c.validationError = ""
		c.RefreshContent()
	})
}

func historyFixChanged(activity domain.Activity, accountID domain.AccountID, amount domain.Money, reason domain.ActivityReason, effectiveAt time.Time, note *string) bool {
	if activity.Reason != reason || !activity.EffectiveAt.Equal(effectiveAt) || !sameOptionalString(activity.Note, note) {
		return true
	}
	for _, effect := range activity.Effects {
		if effect.AccountID != nil && effect.Money != nil {
			return *effect.AccountID != accountID || effect.Money.CanonicalAmount() != amount.CanonicalAmount() || effect.Money.Currency() != amount.Currency()
		}
	}
	return true
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func historyEffectiveAt(c *Controller, value string) (time.Time, error) {
	return historyEffectiveAtWithTime(c, value, "12:00")
}

func historyEffectiveAtWithTime(c *Controller, date, clock string) (time.Time, error) {
	date = strings.TrimSpace(date)
	clock = strings.TrimSpace(clock)
	if date == "" && clock == "" {
		return time.Now(), nil
	}
	if date == "" {
		return time.Time{}, historyValidation("effectiveDate", "date is required when a time is provided")
	}
	if clock == "" {
		clock = "12:00"
	}
	timezone := c.preference.Timezone
	if timezone == "" {
		timezone = "UTC"
	}
	return domain.ResolveLocalDateTime(date, clock, timezone)
}

// historyReasonOptions pairs each activity reason enum with its translated
// label; the enum string is the stable value handed to business logic.
func historyReasonOptions(t interface{ T(string) string }) []selectOption {
	return []selectOption{
		{value: string(domain.ReasonIncome), label: t.T("history.reason.income")},
		{value: string(domain.ReasonContribution), label: t.T("history.reason.contribution")},
		{value: string(domain.ReasonExpense), label: t.T("history.reason.expense")},
		{value: string(domain.ReasonReconciliation), label: t.T("history.reason.reconciliation")},
		{value: string(domain.ReasonOther), label: t.T("history.reason.other")},
	}
}

// historySelectedReason returns the chosen reason, falling back to income when
// nothing valid is selected (e.g. a fix dialog opened on an unusual activity).
func historySelectedReason(reason *valueSelect) domain.ActivityReason {
	if value := reason.Value(); value != "" {
		return domain.ActivityReason(value)
	}
	return domain.ReasonIncome
}

func historyKindLabel(t interface{ T(string) string }, kind domain.ActivityKind) string {
	return t.T("history.kind." + kind.String())
}

func historyReasonLabel(t interface{ T(string) string }, reason domain.ActivityReason) string {
	return t.T("history.reason." + string(reason))
}

func activityHasAccount(activity domain.Activity, accountID domain.AccountID) bool {
	for _, effect := range activity.Effects {
		if effect.AccountID != nil && *effect.AccountID == accountID {
			return true
		}
	}
	return false
}

func historyValidation(field, message string) error {
	return &domain.Error{Code: domain.ErrValidation, Field: field, Message: message}
}
