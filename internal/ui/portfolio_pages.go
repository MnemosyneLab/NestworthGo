package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	fyneTheme "fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func accountValuationSummary(c *Controller, valuation domain.AccountValuation) string {
	if valuation.Account.ID == "" || valuation.BaseValue == nil {
		return c.translator.T("accounts.noValue")
	}
	value := format.Money(valuation.BaseValue.Amount, valuation.BaseValue.Currency.String(), c.preference)
	if !valuation.Complete {
		value += " · " + c.translator.T("portfolio.incomplete")
	}
	return value
}

func showPortfolioDialog(c *Controller, title string, content fyne.CanvasObject, preferred, minimum fyne.Size) *dialog.CustomDialog {
	scroll := container.NewVScroll(container.NewPadded(content))
	dialogView := dialog.NewCustom(title, c.translator.T("common.close"), scroll, c.window)
	dialogView.SetOnClosed(func() { c.cancelRefresh() })
	dialogView.Resize(fittedModalSize(c.window, preferred, minimum))
	dialogView.Show()
	return dialogView
}

func showAccountDetailDialog(c *Controller, accountID domain.AccountID) {
	ctx := context.Background()
	valuation, err := c.service.AccountValuation(ctx, accountID)
	if err != nil {
		c.setValidationError(c.translator.TranslateError(err))
		return
	}
	holdings, err := c.service.ListHoldings(ctx, accountID, false)
	if err != nil {
		c.setValidationError(c.translator.TranslateError(err))
		return
	}
	cashValues, err := c.service.ListAccountCashValues(ctx, accountID)
	if err != nil {
		c.setValidationError(c.translator.TranslateError(err))
		return
	}
	instruments, err := c.service.ListInstruments(ctx, false)
	if err != nil {
		c.setValidationError(c.translator.TranslateError(err))
		return
	}
	var detail *dialog.CustomDialog
	closeAndRefresh := func() {
		if detail != nil {
			detail.Hide()
		}
		c.reloadBackend()
		c.RefreshContent()
	}
	content := accountDetailContent(c, valuation, holdings, cashValues, instruments, closeAndRefresh)
	detail = showPortfolioDialog(c, c.translator.T("portfolio.accountDetailTitle"), content, fyne.NewSize(900, 720), fyne.NewSize(560, 360))
}

func accountDetailContent(c *Controller, valuation domain.AccountValuation, holdings []domain.Holding, cashValues []domain.AccountCashValue, instruments []domain.Instrument, changed func()) fyne.CanvasObject {
	t := c.translator
	rows := []fyne.CanvasObject{
		sectionTitle(valuation.Account.Name, fmt.Sprintf("%s · %s", enumLabel(c, string(valuation.Account.TrackingMode)), valuation.Account.DefaultCurrency.String())),
		container.NewHBox(badge(accountValuationSummary(c, valuation), PaletteFor(c.preference.Accent).Soft, PaletteFor(c.preference.Accent).Primary), completenessBadge(c, valuation.Complete)),
	}
	if len(valuation.MissingInputs) > 0 {
		missingRows := []fyne.CanvasObject{widget.NewLabelWithStyle(t.T("portfolio.missingInputs"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})}
		for _, missing := range valuation.MissingInputs {
			missingRows = append(missingRows, widget.NewLabel(missingInputText(c, missing, instruments)))
		}
		rows = append(rows, settingsSurface(container.NewVBox(missingRows...), fyne.NewSize(1, 70)))
	}

	if valuation.Account.TrackingMode == domain.TrackingHoldings {
		actions := container.NewHBox(
			widget.NewButtonWithIcon(t.T("portfolio.addHolding"), fyneTheme.Current().Icon(fyneTheme.IconNameContentAdd), func() {
				showHoldingCreateDialog(c, valuation.Account.ID, changed)
			}),
			widget.NewButton(t.T("portfolio.addCash"), func() { showCashCreateDialog(c, valuation.Account.ID, changed) }),
		)
		holdingRows := make([]fyne.CanvasObject, 0, len(holdings))
		instrumentByID := make(map[domain.InstrumentID]domain.Instrument, len(instruments))
		for _, instrument := range instruments {
			instrumentByID[instrument.ID] = instrument
		}
		componentByInstrument := make(map[domain.InstrumentID]domain.ValuationComponent)
		for _, component := range valuation.Components {
			if component.InstrumentID != nil {
				componentByInstrument[*component.InstrumentID] = component
			}
		}
		for _, holding := range holdings {
			instrument, ok := instrumentByID[holding.InstrumentID]
			if !ok {
				continue
			}
			component := componentByInstrument[holding.InstrumentID]
			holdingRows = append(holdingRows, holdingDetailRow(c, holding, instrument, component, changed))
		}
		if len(holdingRows) == 0 {
			holdingRows = append(holdingRows, emptyPanel(t.T("portfolio.noHoldings"), t.T("portfolio.noHoldingsDescription")))
		}
		holdingBody := container.NewVBox(actions, widget.NewSeparator())
		holdingBody.Add(container.NewVBox(holdingRows...))
		rows = append(rows, sectionCard(t.T("portfolio.holdings"), t.T("portfolio.holdingsDescription"), holdingBody))

		cashRows := make([]fyne.CanvasObject, 0)
		componentByCurrency := make(map[domain.CurrencyCode]domain.ValuationComponent)
		for _, component := range valuation.Components {
			if component.InstrumentID == nil {
				componentByCurrency[component.NativeCurrency] = component
			}
		}
		for _, cash := range latestCashValuesForUI(cashValues) {
			component := componentByCurrency[cash.Amount.Currency()]
			cashRows = append(cashRows, valueEvidenceRow(c, t.T("portfolio.cash"), cash.Amount.CanonicalAmount(), cash.Amount.Currency(), component, nil))
		}
		if len(cashRows) == 0 {
			cashRows = append(cashRows, emptyPanel(t.T("portfolio.noCash"), t.T("portfolio.noCashDescription")))
		}
		rows = append(rows, sectionCard(t.T("portfolio.cash"), t.T("portfolio.cashDescription"), container.NewVBox(cashRows...)))
	} else {
		componentRows := make([]fyne.CanvasObject, 0, len(valuation.Components))
		for _, component := range valuation.Components {
			componentRows = append(componentRows, valueEvidenceRow(c, t.T("portfolio.currentValue"), component.NativeAmount, component.NativeCurrency, component, nil))
		}
		if len(componentRows) == 0 {
			componentRows = append(componentRows, emptyPanel(t.T("portfolio.noValue"), t.T("portfolio.noValueDescription")))
		}
		rows = append(rows, sectionCard(t.T("portfolio.currentValue"), t.T("portfolio.currentValueDescription"), container.NewVBox(componentRows...)))
	}
	return container.New(layout.NewCustomPaddedVBoxLayout(12), rows...)
}

func completenessBadge(c *Controller, complete bool) fyne.CanvasObject {
	if complete {
		return badge(c.translator.T("portfolio.complete"), PaletteFor(c.preference.Accent).Soft, PaletteFor(c.preference.Accent).Primary)
	}
	return badge(c.translator.T("portfolio.incomplete"), PaletteFor(c.preference.Accent).Soft, PaletteFor(c.preference.Accent).Primary)
}

func holdingDetailRow(c *Controller, holding domain.Holding, instrument domain.Instrument, component domain.ValuationComponent, changed func()) fyne.CanvasObject {
	t := c.translator
	native := format.Money(component.NativeAmount, component.NativeCurrency.String(), c.preference)
	value := native
	if component.BaseAmount != nil {
		value += " → " + format.Money(component.BaseAmount.Amount, component.BaseAmount.Currency.String(), c.preference)
	}
	if !component.Available {
		value = t.T("portfolio.unavailable")
	}
	quantity := fmt.Sprintf("%s · %s", t.T("portfolio.quantity"), holding.Quantity.Canonical())
	info := container.NewVBox(
		widget.NewLabelWithStyle(instrument.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		mutedLabel(quantity+" · "+value),
		quoteEvidenceRows(c, component),
	)
	edit := rowActionButton(t.T("portfolio.editQuantity"), func() { showHoldingEditDialog(c, holding, changed) })
	price := rowActionButton(t.T("portfolio.manualPrice"), func() { showManualInstrumentQuoteDialog(c, instrument, changed) })
	archive := rowActionButton(t.T("portfolio.archiveHolding"), func() {
		runBackend(c, func() error { return c.service.ArchiveHolding(context.Background(), holding.ID, true) }, func(err error) {
			if err != nil {
				c.setValidationError(c.translator.TranslateError(err))
				return
			}
			changed()
		})
	})
	return settingsSurface(container.NewBorder(nil, nil, nil, rowActionBar(edit, price, archive), info), fyne.NewSize(1, 90))
}

func valueEvidenceRow(c *Controller, label, nativeAmount string, nativeCurrency domain.CurrencyCode, component domain.ValuationComponent, extra fyne.CanvasObject) fyne.CanvasObject {
	value := format.Money(nativeAmount, nativeCurrency.String(), c.preference)
	if component.BaseAmount != nil {
		value += " → " + format.Money(component.BaseAmount.Amount, component.BaseAmount.Currency.String(), c.preference)
	}
	if !component.Available {
		value = c.translator.T("portfolio.unavailable")
	}
	body := container.NewVBox(keyValueRow(label, value), quoteEvidenceRows(c, component))
	if extra != nil {
		body.Add(extra)
	}
	return settingsSurface(body, fyne.NewSize(1, 70))
}

func quoteEvidenceRows(c *Controller, component domain.ValuationComponent) fyne.CanvasObject {
	rows := make([]fyne.CanvasObject, 0, 2)
	if component.PriceEvidence != nil {
		rows = append(rows, mutedLabel(c.translator.T("portfolio.priceEvidence")+": "+quoteEvidenceText(c, component.PriceEvidence)))
	}
	if component.FXEvidence != nil {
		rows = append(rows, mutedLabel(c.translator.T("portfolio.fxEvidence")+": "+quoteEvidenceText(c, component.FXEvidence)))
	}
	if len(rows) == 0 {
		rows = append(rows, mutedLabel(c.translator.T("portfolio.identityConversion")))
	}
	return container.NewVBox(rows...)
}

func quoteEvidenceText(c *Controller, evidence *domain.QuoteEvidenceView) string {
	if evidence == nil {
		return c.translator.T("portfolio.noEvidence")
	}
	source := c.translator.T("portfolio.source." + string(evidence.Source))
	freshness := c.translator.T("portfolio.freshness." + string(evidence.Freshness))
	if source == "" {
		source = string(evidence.Source)
	}
	if freshness == "" {
		freshness = string(evidence.Freshness)
	}
	return fmt.Sprintf("%s · %s · %s", source, evidence.QuotedAt.Format("2006-01-02 15:04"), freshness)
}

func missingInputText(c *Controller, missing domain.MissingInputView, instruments []domain.Instrument) string {
	switch missing.Kind {
	case domain.MissingInstrumentPrice:
		name := c.translator.T("portfolio.unknownInstrument")
		if missing.InstrumentID != nil {
			name = missing.InstrumentID.String()
		}
		for _, instrument := range instruments {
			if missing.InstrumentID != nil && instrument.ID == *missing.InstrumentID {
				name = instrument.Name
				break
			}
		}
		return fmt.Sprintf("%s: %s", c.translator.T("portfolio.missingInstrumentPrice"), name)
	case domain.MissingFXRate:
		return fmt.Sprintf("%s: %s → %s", c.translator.T("portfolio.missingFXRate"), missing.QuoteCurrency, missing.BaseCurrency)
	case domain.MissingAccountValue:
		return c.translator.T("portfolio.missingAccountValue")
	default:
		return string(missing.Kind)
	}
}

func latestCashValuesForUI(values []domain.AccountCashValue) []domain.AccountCashValue {
	latest := map[domain.CurrencyCode]domain.AccountCashValue{}
	for _, value := range values {
		current, ok := latest[value.Amount.Currency()]
		if !ok || cashObservationLater(value, current) {
			latest[value.Amount.Currency()] = value
		}
	}
	result := make([]domain.AccountCashValue, 0, len(latest))
	for _, value := range latest {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Amount.Currency() < result[j].Amount.Currency() })
	return result
}

func cashObservationLater(left, right domain.AccountCashValue) bool {
	if !left.EffectiveAt.Equal(right.EffectiveAt) {
		return left.EffectiveAt.After(right.EffectiveAt)
	}
	if !left.CreatedAt.Equal(right.CreatedAt) {
		return left.CreatedAt.After(right.CreatedAt)
	}
	return left.ID.String() > right.ID.String()
}

func showHoldingCreateDialog(c *Controller, accountID domain.AccountID, changed func()) {
	instruments, err := c.service.ListInstruments(context.Background(), false)
	if err != nil {
		c.setValidationError(c.translator.TranslateError(err))
		return
	}
	if len(instruments) == 0 {
		c.setValidationError(c.translator.T("portfolio.noInstrumentsForHolding"))
		return
	}
	options := make([]string, 0, len(instruments))
	for _, instrument := range instruments {
		options = append(options, instrumentOptionLabel(instrument))
	}
	instrumentSelect := widget.NewSelect(options, nil)
	instrumentSelect.SetSelected(options[0])
	quantity := widget.NewEntry()
	quantity.SetPlaceHolder("0.00000000")
	items := []*widget.FormItem{
		widget.NewFormItem(c.translator.T("portfolio.instrument"), instrumentSelect),
		widget.NewFormItem(c.translator.T("portfolio.quantity"), quantity),
	}
	showResponsiveBackendForm(c, c.translator.T("portfolio.addHolding"), c.translator.T("common.save"), c.translator.T("common.cancel"), items, fyne.NewSize(560, 300), fyne.NewSize(460, 240), func() error {
		selected := instrumentIDForOption(instruments, instrumentSelect.Selected)
		_, err := c.service.CreateHolding(context.Background(), application.HoldingInput{AccountID: accountID.String(), InstrumentID: selected.String(), Quantity: quantity.Text})
		return err
	}, changed)
}

func showHoldingEditDialog(c *Controller, holding domain.Holding, changed func()) {
	quantity := widget.NewEntry()
	quantity.SetText(holding.Quantity.Canonical())
	showResponsiveBackendForm(c, c.translator.T("portfolio.editQuantity"), c.translator.T("common.save"), c.translator.T("common.cancel"), []*widget.FormItem{widget.NewFormItem(c.translator.T("portfolio.quantity"), quantity)}, fyne.NewSize(520, 240), fyne.NewSize(420, 200), func() error {
		_, err := c.service.UpdateHoldingQuantity(context.Background(), holding.ID, quantity.Text)
		return err
	}, changed)
}

func showCashCreateDialog(c *Controller, accountID domain.AccountID, changed func()) {
	amount := widget.NewEntry()
	currency := widget.NewEntry()
	currency.SetText(c.bootstrap.Household.BaseCurrency.String())
	effective := newDateEntry(c.translator.T("accounts.datePlaceholder"))
	items := []*widget.FormItem{
		widget.NewFormItem(c.translator.T("portfolio.amount"), amount),
		widget.NewFormItem(c.translator.T("accounts.currency"), currency),
		widget.NewFormItem(c.translator.T("accounts.effectiveDate"), dateFormField(effective)),
	}
	showResponsiveBackendForm(c, c.translator.T("portfolio.addCash"), c.translator.T("common.save"), c.translator.T("common.cancel"), items, fyne.NewSize(560, 320), fyne.NewSize(460, 250), func() error {
		_, err := c.service.AppendAccountCashValue(context.Background(), accountID, amount.Text, strings.ToUpper(strings.TrimSpace(currency.Text)), dateEntryValue(effective))
		return err
	}, changed)
}

func showManualInstrumentQuoteDialog(c *Controller, instrument domain.Instrument, changed func()) {
	price := widget.NewEntry()
	quotedAt := newDateEntry(c.translator.T("accounts.datePlaceholder"))
	delayed := widget.NewCheck(c.translator.T("portfolio.delayed"), nil)
	items := []*widget.FormItem{
		widget.NewFormItem(c.translator.T("portfolio.price"), price),
		widget.NewFormItem(c.translator.T("portfolio.quoteDate"), dateFormField(quotedAt)),
		widget.NewFormItem(tOr(c, "portfolio.delayed"), delayed),
	}
	showResponsiveBackendForm(c, c.translator.T("portfolio.manualPrice"), c.translator.T("portfolio.savePrice"), c.translator.T("common.cancel"), items, fyne.NewSize(560, 340), fyne.NewSize(460, 260), func() error {
		_, err := c.service.AppendManualInstrumentQuote(context.Background(), instrument.ID, price.Text, dateEntryValue(quotedAt), delayed.Checked)
		return err
	}, changed)
}

func showInstrumentManagementDialog(c *Controller) {
	instruments, err := c.service.ListInstruments(context.Background(), true)
	if err != nil {
		c.setValidationError(c.translator.TranslateError(err))
		return
	}
	var manager *dialog.CustomDialog
	closeAndRefresh := func() {
		if manager != nil {
			manager.Hide()
		}
		c.reloadBackend()
		c.RefreshContent()
	}
	rows := []fyne.CanvasObject{
		sectionTitle(c.translator.T("portfolio.instruments"), c.translator.T("portfolio.instrumentsDescription")),
		providerDisclaimer(c),
		widget.NewButtonWithIcon(c.translator.T("portfolio.addInstrument"), fyneTheme.Current().Icon(fyneTheme.IconNameContentAdd), func() {
			showInstrumentFormDialog(c, nil, closeAndRefresh)
		}),
	}
	if len(instruments) == 0 {
		rows = append(rows, emptyPanel(c.translator.T("portfolio.noInstruments"), c.translator.T("portfolio.noInstrumentsDescription")))
	}
	for _, instrument := range instruments {
		current := instrument
		status := c.translator.T("common.active")
		action := c.translator.T("portfolio.archiveInstrument")
		if instrument.ArchivedAt != nil {
			status = c.translator.T("common.archived")
			action = c.translator.T("common.restore")
		}
		refreshStatus := widget.NewLabel("")
		refreshStatus.Hide()
		info := container.NewVBox(
			widget.NewLabelWithStyle(instrument.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			mutedLabel(fmt.Sprintf("%s · %s · %s", instrument.QuoteCurrency, enumLabel(c, string(instrument.Type)), status)),
			refreshStatus,
		)
		actions := []fyne.CanvasObject{
			rowActionButton(c.translator.T("common.edit"), func() { showInstrumentFormDialog(c, &current, closeAndRefresh) }),
		}
		if instrument.ArchivedAt == nil {
			actions = append(actions, rowActionButton(c.translator.T("portfolio.manualPrice"), func() { showManualInstrumentQuoteDialog(c, current, closeAndRefresh) }))
			if instrumentProviderRefreshAvailable(c, current) {
				actions = append(actions, rowActionButtonState(c.translator.T("portfolio.refreshPrice"), func() {
					refreshStatus.Show()
					refreshStatus.SetText(c.translator.T("portfolio.refreshing"))
					refreshStatus.Importance = widget.WarningImportance
					refreshStatus.Refresh()
					c.startRefreshInDialog(refreshRequest{operation: refreshInstrumentOperation, instrument: current.ID}, func(result application.RefreshResult, err error) {
						if err != nil {
							refreshStatus.SetText(c.translator.TranslateError(err))
							refreshStatus.Importance = widget.DangerImportance
							refreshStatus.Refresh()
							return
						}
						if refreshResultNeedsAttention(result) {
							refreshStatus.SetText(refreshResultTextFor(c, result))
							refreshStatus.Importance = widget.WarningImportance
							refreshStatus.Refresh()
							return
						}
						closeAndRefresh()

					})
				}, c.refreshPending))
			}
		}
		actions = append(actions, rowActionButton(action, func() {
			runBackend(c, func() error {
				return c.service.ArchiveInstrument(context.Background(), current.ID, current.ArchivedAt == nil)
			}, func(err error) {
				if err != nil {
					c.setValidationError(c.translator.TranslateError(err))
					return
				}
				closeAndRefresh()
			})
		}))
		rows = append(rows, settingsSurface(container.NewBorder(nil, nil, nil, rowActionBar(actions...), info), fyne.NewSize(1, 76)))
	}
	manager = showPortfolioDialog(c, c.translator.T("portfolio.instruments"), container.New(layout.NewCustomPaddedVBoxLayout(12), rows...), fyne.NewSize(820, 680), fyne.NewSize(540, 320))
}

func showInstrumentFormDialog(c *Controller, current *domain.Instrument, changed func()) {
	t := c.translator
	name := widget.NewEntry()
	typeValues := []string{string(domain.InstrumentStock), string(domain.InstrumentETF), string(domain.InstrumentMutualFund), string(domain.InstrumentCrypto), string(domain.InstrumentBond), string(domain.InstrumentPreciousMetal), string(domain.InstrumentBankInvestmentProduct), string(domain.InstrumentOther)}
	instrumentType := widget.NewSelect(enumOptions(c, typeValues), nil)
	quoteCurrency := widget.NewEntry()
	symbol := widget.NewEntry()
	marketCode := widget.NewEntry()
	countryCode := widget.NewEntry()
	isin := widget.NewEntry()
	note := widget.NewEntry()
	providerAvailable := instrumentProviderCapabilityAvailable(c)
	providerSource := widget.NewSelect([]string{t.T("portfolio.source.manual")}, nil)
	providerSymbol := widget.NewEntry()
	providerSymbol.SetPlaceHolder(t.T("portfolio.providerSymbolPlaceholder"))
	if current == nil {
		instrumentType.SetSelected(enumLabel(c, string(domain.InstrumentETF)))
		quoteCurrency.SetText(c.bootstrap.Household.BaseCurrency.String())
	} else {
		name.SetText(current.Name)
		instrumentType.SetSelected(enumLabel(c, string(current.Type)))
		quoteCurrency.SetText(current.QuoteCurrency.String())
		setOptionalEntry(symbol, current.Symbol)
		setOptionalEntry(marketCode, current.MarketCode)
		setOptionalEntry(countryCode, current.CountryCode)
		setOptionalEntry(isin, current.ISIN)
		setOptionalEntry(note, current.Note)
		setOptionalEntry(providerSymbol, current.ProviderSymbol)
	}
	if providerAvailable {
		providerSource.SetOptions([]string{t.T("portfolio.source.manual"), t.T("portfolio.source.provider")})
		if current != nil && current.QuoteSource == domain.QuoteSourceProvider {
			providerSource.SetSelected(t.T("portfolio.source.provider"))
		} else {
			providerSource.SetSelected(t.T("portfolio.source.manual"))
		}
	} else {
		providerSource.SetSelected(t.T("portfolio.source.manual"))
	}
	items := []*widget.FormItem{
		widget.NewFormItem(t.T("portfolio.instrumentName"), name),
		widget.NewFormItem(t.T("portfolio.instrumentType"), instrumentType),
		widget.NewFormItem(t.T("portfolio.quoteCurrency"), quoteCurrency),
		widget.NewFormItem(t.T("portfolio.symbol"), symbol),
		widget.NewFormItem(t.T("portfolio.marketCode"), marketCode),
		widget.NewFormItem(t.T("portfolio.countryCode"), countryCode),
		widget.NewFormItem(t.T("portfolio.isin"), isin),
		widget.NewFormItem(t.T("portfolio.note"), note),
	}
	if providerAvailable {
		providerSymbol.Validator = func(value string) error {
			if providerSource.Selected == t.T("portfolio.source.provider") && strings.TrimSpace(value) == "" {
				return fmt.Errorf("%s", t.T("portfolio.providerBindingRequired"))
			}
			return nil
		}
		items = append(items,
			widget.NewFormItem(t.T("portfolio.source"), providerSource),
			widget.NewFormItem(t.T("portfolio.provider"), mutedLabel(t.T("portfolio.yahooFinance"))),
			widget.NewFormItem(t.T("portfolio.providerSymbol"), providerSymbol),
			widget.NewFormItem(t.T("portfolio.yahooDisclaimerLabel"), mutedLabel(t.T("portfolio.yahooDisclaimer"))),
		)
	}
	title := t.T("portfolio.addInstrument")
	if current != nil {
		title = t.T("portfolio.editInstrument")
	}
	showResponsiveBackendForm(c, title, t.T("common.save"), t.T("common.cancel"), items, fyne.NewSize(660, 600), fyne.NewSize(500, 340), func() error {
		quoteSource := string(domain.QuoteSourceManual)
		providerKey := ""
		providerBinding := ""
		if providerAvailable && providerSource.Selected == t.T("portfolio.source.provider") {
			quoteSource = string(domain.QuoteSourceProvider)
			providerKey = application.YahooFinanceProviderKey
			providerBinding = providerSymbol.Text
		}
		input := application.InstrumentInput{Name: name.Text, Type: enumValue(c, instrumentType.Selected, typeValues), QuoteCurrency: strings.ToUpper(strings.TrimSpace(quoteCurrency.Text)), Symbol: symbol.Text, MarketCode: marketCode.Text, CountryCode: countryCode.Text, ISIN: isin.Text, Note: stringPointer(note.Text), QuoteSource: quoteSource, ProviderKey: providerKey, ProviderSymbol: providerBinding}
		if current == nil {
			_, err := c.service.CreateInstrument(context.Background(), input)
			return err
		}
		_, err := c.service.UpdateInstrument(context.Background(), current.ID, input)
		return err
	}, changed)
}

func setOptionalEntry(entry *widget.Entry, value *string) {
	if value != nil {
		entry.SetText(*value)
	}
}

func instrumentOptionLabel(instrument domain.Instrument) string {
	if instrument.Symbol != nil && strings.TrimSpace(*instrument.Symbol) != "" {
		return fmt.Sprintf("%s (%s · %s)", instrument.Name, *instrument.Symbol, instrument.QuoteCurrency)
	}
	return fmt.Sprintf("%s (%s)", instrument.Name, instrument.QuoteCurrency)
}

func instrumentIDForOption(instruments []domain.Instrument, option string) domain.InstrumentID {
	for _, instrument := range instruments {
		if instrumentOptionLabel(instrument) == option {
			return instrument.ID
		}
	}
	return ""
}

func showFXManagementDialog(c *Controller) {
	ctx := context.Background()
	valuations, err := c.service.AccountValuations(ctx, domain.AccountFilter{})
	if err != nil {
		c.setValidationError(c.translator.TranslateError(err))
		return
	}
	preferences, err := c.service.ListFXPreferences(ctx)
	if err != nil {
		c.setValidationError(c.translator.TranslateError(err))
		return
	}
	required := map[string]domain.MissingInputView{}
	for _, valuation := range valuations {
		for _, component := range valuation.Components {
			if c.bootstrap.Household != nil && component.NativeCurrency != c.bootstrap.Household.BaseCurrency {
				key := fxPairKey(component.NativeCurrency, c.bootstrap.Household.BaseCurrency)
				required[key] = domain.MissingInputView{AccountID: valuation.Account.ID, BaseCurrency: c.bootstrap.Household.BaseCurrency, QuoteCurrency: component.NativeCurrency}
			}
		}
		for _, item := range valuation.MissingInputs {
			if item.Kind != domain.MissingFXRate {
				continue
			}
			required[fxPairKey(item.QuoteCurrency, item.BaseCurrency)] = item
		}
	}
	preferenceByPair := make(map[string]domain.FXPreference, len(preferences))
	for _, preference := range preferences {
		preferenceByPair[fxPairKey(preference.CurrencyA, preference.CurrencyB)] = preference
	}
	keys := make([]string, 0, len(required))
	for key := range required {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var fxDialog *dialog.CustomDialog
	closeAndRefresh := func() {
		if fxDialog != nil {
			fxDialog.Hide()
		}
		c.reloadBackend()
		c.RefreshContent()
	}
	rows := []fyne.CanvasObject{
		sectionTitle(c.translator.T("portfolio.fxRates"), c.translator.T("portfolio.fxRatesDescription")),
		mutedLabel(c.translator.T("portfolio.fxOrientationExplanation")),
		fxProviderDisclaimer(c),
	}
	refreshStatus := widget.NewLabel("")
	var refreshRequiredFX *widget.Button
	refreshRequiredFX = widget.NewButton(c.translator.T("portfolio.refreshFX"), func() {
		refreshRequiredFX.Disable()
		refreshStatus.SetText(c.translator.T("portfolio.refreshing"))
		refreshStatus.Importance = widget.WarningImportance
		refreshStatus.Refresh()
		c.startRefreshInDialog(refreshRequest{operation: refreshRequiredFXOperation}, func(result application.RefreshResult, err error) {
			if err != nil {
				refreshRequiredFX.Enable()
				refreshStatus.SetText(c.translator.TranslateError(err))
				refreshStatus.Importance = widget.DangerImportance
				refreshStatus.Refresh()
				return
			}
			if refreshResultNeedsAttention(result) {
				refreshRequiredFX.Enable()
				refreshStatus.SetText(refreshResultTextFor(c, result))
				refreshStatus.Importance = widget.WarningImportance
				refreshStatus.Refresh()
				return
			}
			closeAndRefresh()
		})
	})
	if !fxProviderCapabilityAvailable(c) || c.refreshPending {
		refreshRequiredFX.Disable()
	}
	rows = append(rows, container.NewHBox(refreshRequiredFX), refreshStatus)
	if len(keys) == 0 {
		rows = append(rows, emptyPanel(c.translator.T("portfolio.noRequiredFX"), c.translator.T("portfolio.noRequiredFXDescription")))
	}
	for _, key := range keys {
		item := required[key]
		native, base := item.QuoteCurrency, item.BaseCurrency
		preference, hasPreference := preferenceByPair[key]
		sourceOptions := []string{c.translator.T("portfolio.source.manual")}
		if fxProviderCapabilityAvailable(c) || (hasPreference && preference.SourceKind == domain.QuoteSourceProvider) {
			sourceOptions = append(sourceOptions, c.translator.T("portfolio.source.provider"))
		}
		source := widget.NewSelect(sourceOptions, nil)
		if hasPreference && preference.SourceKind == domain.QuoteSourceProvider {
			source.SetSelected(c.translator.T("portfolio.source.provider"))
			if !fxProviderCapabilityAvailable(c) {
				source.Disable()
			}
		} else {
			source.SetSelected(c.translator.T("portfolio.source.manual"))
		}
		direction := widget.NewSelect([]string{fmt.Sprintf("%s → %s", native, base), fmt.Sprintf("%s → %s", base, native)}, nil)
		direction.SetSelected(direction.Options[0])
		rate := widget.NewEntry()
		quotedAt := newDateEntry(c.translator.T("accounts.datePlaceholder"))
		save := widget.NewButton(c.translator.T("portfolio.saveRate"), nil)
		refreshStatus := widget.NewLabel("")
		var refresh *widget.Button
		refresh = widget.NewButton(c.translator.T("portfolio.refreshRate"), func() {
			refresh.Disable()
			refreshStatus.SetText(c.translator.T("portfolio.refreshing"))
			refreshStatus.Importance = widget.WarningImportance
			refreshStatus.Refresh()
			c.startRefreshInDialog(refreshRequest{operation: refreshFXOperation, currencyA: native.String(), currencyB: base.String()}, func(result application.RefreshResult, err error) {
				if err != nil {
					refresh.Enable()
					refreshStatus.SetText(c.translator.TranslateError(err))
					refreshStatus.Importance = widget.DangerImportance
					refreshStatus.Refresh()
					return
				}
				if refreshResultNeedsAttention(result) {
					refresh.Enable()
					refreshStatus.SetText(refreshResultTextFor(c, result))
					refreshStatus.Importance = widget.WarningImportance
					refreshStatus.Refresh()
					return
				}
				closeAndRefresh()
			})
		})
		if !fxProviderCapabilityAvailable(c) || c.refreshPending || source.Selected != c.translator.T("portfolio.source.provider") {
			refresh.Disable()
		}
		source.OnChanged = func(value string) {
			if fxProviderCapabilityAvailable(c) && !c.refreshPending && value == c.translator.T("portfolio.source.provider") {
				refresh.Enable()
				return
			}
			refresh.Disable()
		}
		save.OnTapped = func() {
			save.Disable()
			runBackend(c, func() error {
				if source.Selected == c.translator.T("portfolio.source.provider") {
					_, err := c.service.SetFXPreference(context.Background(), native.String(), base.String(), string(domain.QuoteSourceProvider))
					return err
				}
				baseCurrency, quoteCurrency := native, base
				if direction.Selected == direction.Options[1] {
					baseCurrency, quoteCurrency = base, native
				}
				_, err := c.service.AppendManualFXQuote(context.Background(), baseCurrency.String(), quoteCurrency.String(), rate.Text, dateEntryValue(quotedAt))
				return err
			}, func(err error) {
				if err != nil {
					save.Enable()
					c.setValidationError(c.translator.TranslateError(err))
					return
				}
				closeAndRefresh()
			})
		}
		form := container.NewVBox(
			widget.NewLabelWithStyle(fmt.Sprintf("%s / %s", native, base), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			settingsRow(c.translator.T("portfolio.source"), source),
			settingsRow(c.translator.T("portfolio.fxDirection"), direction),
			settingsRow(c.translator.T("portfolio.rate"), rate),
			settingsRow(c.translator.T("portfolio.quoteDate"), dateFormField(quotedAt)),
			container.NewBorder(nil, nil, nil, container.NewHBox(refresh, save), nil),
			refreshStatus,
		)
		rows = append(rows, settingsSurface(form, fyne.NewSize(1, 230)))
	}
	fxDialog = showPortfolioDialog(c, c.translator.T("portfolio.fxRates"), container.New(layout.NewCustomPaddedVBoxLayout(12), rows...), fyne.NewSize(760, 680), fyne.NewSize(540, 320))
}

func fxPairKey(first, second domain.CurrencyCode) string {
	a, b, err := domain.NormalizeFXPair(first, second)
	if err != nil {
		return first.String() + "|" + second.String()
	}
	return a.String() + "|" + b.String()
}

func tOr(c *Controller, key string) string { return c.translator.T(key) }

func providerDisclaimer(c *Controller) fyne.CanvasObject {
	label := mutedLabel(c.translator.T("portfolio.yahooDisclaimer"))
	label.Wrapping = fyne.TextWrapWord
	return settingsSurface(label, fyne.NewSize(1, 54))
}

func fxProviderDisclaimer(c *Controller) fyne.CanvasObject {
	key := application.YahooFinanceProviderKey
	if provider := selectedFXProvider(c); provider != nil {
		key = strings.ToLower(strings.TrimSpace(provider.Key()))
	}
	disclaimerKey := "portfolio.yahooDisclaimer"
	if key == settings.FXProviderFrankfurter {
		disclaimerKey = "portfolio.frankfurterDisclaimer"
	}
	label := mutedLabel(c.translator.T(disclaimerKey))
	label.Wrapping = fyne.TextWrapWord
	return settingsSurface(label, fyne.NewSize(1, 54))
}

func instrumentProviderCapabilityAvailable(c *Controller) bool {
	if c.service == nil || c.service.MarketDataRegistry() == nil {
		return false
	}
	provider, err := c.service.MarketDataRegistry().Resolve(application.YahooFinanceProviderKey)
	return err == nil && provider.Capabilities().LatestInstrument
}

func fxProviderCapabilityAvailable(c *Controller) bool {
	provider := selectedFXProvider(c)
	return provider != nil && provider.Capabilities().LatestFX
}

func selectedFXProvider(c *Controller) application.MarketDataProvider {
	if c == nil || c.service == nil || c.service.MarketDataRegistry() == nil {
		return nil
	}
	registry := c.service.MarketDataRegistry()
	if key := c.service.FXProviderKey(); key != "" {
		if provider, err := registry.Resolve(key); err == nil {
			return provider
		}
	}
	provider, err := registry.Default()
	if err != nil {
		return nil
	}
	return provider
}

func instrumentProviderBindingComplete(instrument domain.Instrument) bool {
	return instrument.QuoteSource == domain.QuoteSourceProvider && instrument.ProviderKey != nil && strings.TrimSpace(*instrument.ProviderKey) != "" && instrument.ProviderSymbol != nil && strings.TrimSpace(*instrument.ProviderSymbol) != ""
}

func instrumentProviderRefreshAvailable(c *Controller, instrument domain.Instrument) bool {
	if instrument.ArchivedAt != nil || !instrumentProviderBindingComplete(instrument) || c.service == nil || c.service.MarketDataRegistry() == nil {
		return false
	}
	provider, err := c.service.MarketDataRegistry().Resolve(*instrument.ProviderKey)
	return err == nil && provider.Capabilities().LatestInstrument
}
