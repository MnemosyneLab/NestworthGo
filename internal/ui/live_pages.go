package ui

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"io"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	fyneTheme "fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/infrastructure/media"
)

func NewBlockedStartupPage(c *Controller, _ error) fyne.CanvasObject {
	t := c.translator
	content := container.NewVBox(
		widget.NewLabelWithStyle(t.T("startup.blockedTitle"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel(t.T("startup.blockedDescription")),
		badge(t.T("startup.readOnly"), PaletteFor(c.preference.Accent).Soft, PaletteFor(c.preference.Accent).Primary),
	)
	return container.NewCenter(surface(content, fyne.NewSize(540, 230)))
}

func NewOnboardingPage(c *Controller) fyne.CanvasObject {
	t := c.translator
	householdName := widget.NewEntry()
	householdName.SetPlaceHolder(t.T("onboarding.householdNamePlaceholder"))
	currency := widget.NewEntry()
	currency.SetText(c.preference.Currency)
	memberNames := widget.NewEntry()
	memberNames.SetPlaceHolder(t.T("onboarding.memberNamesPlaceholder"))
	errorLabel := widget.NewLabel("")
	errorLabel.Importance = widget.DangerImportance
	complete := widget.NewButtonWithIcon(t.T("onboarding.create"), fyneTheme.Current().Icon(fyneTheme.IconNameConfirm), func() {
		members := splitNames(memberNames.Text)
		err := c.service.CompleteOnboarding(context.Background(), application.OnboardingInput{HouseholdName: householdName.Text, BaseCurrency: currency.Text, MemberNames: members})
		if err != nil {
			errorLabel.SetText(c.translator.TranslateError(err))
			errorLabel.Refresh()
			return
		}
		c.reloadBackend()
		c.page = PageOverview
		c.validationError = ""
		c.Refresh()
	})
	form := widget.NewForm(
		widget.NewFormItem(t.T("onboarding.householdName"), householdName),
		widget.NewFormItem(t.T("onboarding.baseCurrency"), currency),
		widget.NewFormItem(t.T("onboarding.members"), memberNames),
	)
	content := container.NewVBox(
		widget.NewLabelWithStyle(t.T("onboarding.title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel(t.T("onboarding.description")),
		form,
		errorLabel,
		complete,
	)
	return container.NewCenter(surface(content, fyne.NewSize(620, 380)))
}

func NewLiveOverview(c *Controller) fyne.CanvasObject {
	result, err := c.service.Overview(context.Background(), domain.AccountFilter{})
	if err != nil {
		return errorPanel(c, c.translator.T("overview.loadError"), err)
	}
	currency := result.Currency.String()
	metric := func(title string, amount decimal.Decimal, detail string, accent color.Color) fyne.CanvasObject {
		return metricCard(title, format.Money(amount.String(), currency, c.preference), detail, accent)
	}
	palette := PaletteFor(c.preference.Accent)
	metrics := container.NewGridWithColumns(4,
		metric(c.translator.T("overview.assets"), result.Assets, currency, palette.Primary),
		metric(c.translator.T("overview.liabilities"), result.Liabilities, currency, palette.Primary),
		metric(c.translator.T("overview.netWorth"), result.NetWorth, currency, palette.Primary),
		metricCard(c.translator.T("overview.accounts"), fmt.Sprintf("%d", result.AccountCount), c.translator.T("overview.accountCountDetail"), palette.Primary),
	)
	if result.AccountCount == 0 {
		return container.NewVBox(metrics, emptyPanel(c.translator.T("overview.emptyTitle"), c.translator.T("overview.emptyDescription")))
	}
	breakdowns := container.NewGridWithColumns(2,
		breakdownCard(c, c.translator.T("overview.byCategory"), result.ByCategory, currency),
		breakdownCard(c, c.translator.T("overview.byMember"), result.ByMember, currency),
		breakdownCard(c, c.translator.T("overview.byInstitution"), result.ByInstitution, currency),
		breakdownCard(c, c.translator.T("overview.byGroup"), result.ByGroup, currency),
	)
	return container.NewVBox(metrics, widget.NewSeparator(), breakdowns)
}

func breakdownCard(c *Controller, title string, items []domain.BreakdownItem, currency string) fyne.CanvasObject {
	rows := make([]fyne.CanvasObject, 0, len(items)+1)
	rows = append(rows, widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	for _, item := range items {
		rows = append(rows, keyValueRow(enumLabel(c, item.Label), format.Money(item.Amount.String(), currency, c.preference)+"  "+decimal.NewFromInt(int64(item.ShareBPS)).Div(decimal.NewFromInt(100)).StringFixed(2)+"%"))
	}
	return settingsSurface(container.NewVBox(rows...), fyne.NewSize(1, 120))
}

func NewAccountsPage(c *Controller) fyne.CanvasObject {
	t := c.translator
	all := t.T("accounts.filterAll")
	categoryDomainValues := []string{string(domain.CategoryCashEquivalent), string(domain.CategoryInvestment), string(domain.CategoryProperty), string(domain.CategoryReceivable), string(domain.CategoryLiability)}
	categoryFilter := widget.NewSelect(append([]string{all}, enumOptions(c, categoryDomainValues)...), nil)
	if c.accountCategory == "" {
		categoryFilter.SetSelected(all)
	} else {
		categoryFilter.SetSelected(enumLabel(c, c.accountCategory))
	}
	categoryFilter.OnChanged = func(value string) {
		if value == all {
			c.accountCategory = ""
		} else {
			c.accountCategory = enumValue(c, value, categoryDomainValues)
		}
		c.RefreshContent()
	}
	memberValues := append([]string{all}, memberOptions(c.bootstrap.Members)...)
	memberFilter := widget.NewSelect(memberValues, nil)
	memberFilter.SetSelected(all)
	if c.accountMemberID != "" {
		memberFilter.SetSelected(memberLabelForID(c.bootstrap.Members, c.accountMemberID))
	}
	memberFilter.OnChanged = func(value string) {
		if value == all {
			c.accountMemberID = ""
		} else {
			c.accountMemberID = memberIDForName(c.bootstrap.Members, value)
		}
		c.RefreshContent()
	}
	institutionValues := append([]string{all}, institutionOptions(c.bootstrap.Institutions)...)
	institutionFilter := widget.NewSelect(institutionValues, nil)
	institutionFilter.SetSelected(all)
	if c.accountInstitutionID != "" {
		institutionFilter.SetSelected(institutionLabelForID(c.bootstrap.Institutions, c.accountInstitutionID))
	}
	institutionFilter.OnChanged = func(value string) {
		if value == all {
			c.accountInstitutionID = ""
		} else {
			c.accountInstitutionID = institutionIDForName(c.bootstrap.Institutions, value)
		}
		c.RefreshContent()
	}
	groupValues := append([]string{all}, groupOptions(c.bootstrap.Groups)...)
	groupFilter := widget.NewSelect(groupValues, nil)
	groupFilter.SetSelected(all)
	if c.accountGroupID != "" {
		groupFilter.SetSelected(groupLabelForID(c.bootstrap.Groups, c.accountGroupID))
	}
	groupFilter.OnChanged = func(value string) {
		if value == all {
			c.accountGroupID = ""
		} else {
			c.accountGroupID = groupIDForName(c.bootstrap.Groups, value)
		}
		c.RefreshContent()
	}
	scopeAll := t.T("accounts.filterAll")
	scopeValues := []string{scopeAll, t.T("accounts.filterSole"), t.T("accounts.filterShared")}
	scopeFilter := widget.NewSelect(scopeValues, nil)
	scopeFilter.SetSelected(scopeAll)
	if c.accountOwnershipScope == string(domain.OwnershipSole) {
		scopeFilter.SetSelected(t.T("accounts.filterSole"))
	} else if c.accountOwnershipScope == string(domain.OwnershipShared) {
		scopeFilter.SetSelected(t.T("accounts.filterShared"))
	}
	scopeFilter.OnChanged = func(value string) {
		switch value {
		case t.T("accounts.filterSole"):
			c.accountOwnershipScope = string(domain.OwnershipSole)
		case t.T("accounts.filterShared"):
			c.accountOwnershipScope = string(domain.OwnershipShared)
		default:
			c.accountOwnershipScope = ""
		}
		c.RefreshContent()
	}
	filterGrid := container.NewGridWithColumns(5, container.NewVBox(widget.NewLabel(t.T("accounts.category")), categoryFilter), container.NewVBox(widget.NewLabel(t.T("accounts.owner")), memberFilter), container.NewVBox(widget.NewLabel(t.T("nav.institutions")), institutionFilter), container.NewVBox(widget.NewLabel(t.T("nav.groups")), groupFilter), container.NewVBox(widget.NewLabel(t.T("accounts.ownershipScope")), scopeFilter))
	archivedToggle := widget.NewCheck(t.T("accounts.showArchived"), func(checked bool) { c.showArchived = checked; c.RefreshContent() })
	archivedToggle.SetChecked(c.showArchived)
	filterRow := container.NewBorder(nil, nil, nil, archivedToggle, filterGrid)
	filterResult := domain.AccountFilter{IncludeArchived: c.showArchived}
	if c.accountCategory != "" {
		if category, parseErr := domain.ParsePrimaryCategory(c.accountCategory); parseErr == nil {
			filterResult.Category = &category
		}
	}
	if c.accountMemberID != "" {
		if id, parseErr := domain.ParseMemberID(c.accountMemberID); parseErr == nil {
			filterResult.MemberID = &id
		}
	}
	if c.accountInstitutionID != "" {
		if id, parseErr := domain.ParseInstitutionID(c.accountInstitutionID); parseErr == nil {
			filterResult.InstitutionID = &id
		}
	}
	if c.accountGroupID != "" {
		if id, parseErr := domain.ParseGroupID(c.accountGroupID); parseErr == nil {
			filterResult.GroupID = &id
		}
	}
	filterResult.OwnershipScope = domain.OwnershipScope(c.accountOwnershipScope)
	records, err := c.service.ListAccounts(context.Background(), filterResult)
	if err != nil {
		return errorPanel(c, t.T("accounts.loadError"), err)
	}
	addAccount := widget.NewButtonWithIcon(t.T("accounts.create"), fyneTheme.Current().Icon(fyneTheme.IconNameContentAdd), func() {
		showAccountCreateDialog(c)
	})
	rows := []fyne.CanvasObject{liveFeedback(c), container.NewBorder(nil, nil, nil, addAccount, filterRow), widget.NewSeparator()}
	if len(records) == 0 {
		rows = append(rows, emptyPanel(t.T("accounts.emptyTitle"), t.T("accounts.emptyDescription")))
	} else {
		for _, record := range records {
			rows = append(rows, accountRow(c, record))
		}
	}
	return container.NewVBox(rows...)
}

func showAccountCreateDialog(c *Controller) {
	t := c.translator
	name := widget.NewEntry()
	selectedIcon := domain.DefaultAccountIcon
	iconButton := newIconPickerButton(c, selectedIcon, func(key string) { selectedIcon = key })
	amount := widget.NewEntry()
	amount.SetPlaceHolder("0.00")
	categoryValues := []string{string(domain.CategoryCashEquivalent), string(domain.CategoryInvestment), string(domain.CategoryProperty), string(domain.CategoryReceivable), string(domain.CategoryLiability)}
	category := widget.NewSelect(enumOptions(c, categoryValues), nil)
	category.SetSelected(enumLabel(c, string(domain.CategoryCashEquivalent)))
	secondaryValues := secondaryOptions(domain.CategoryCashEquivalent)
	secondary := widget.NewSelect(enumOptions(c, secondaryValues), nil)
	secondary.SetSelected(enumLabel(c, string(domain.SecondaryBankAccount)))
	trackingValues := trackingOptionsForCategory(domain.CategoryCashEquivalent)
	tracking := widget.NewSelect(enumOptions(c, trackingValues), nil)
	tracking.SetSelected(enumLabel(c, string(domain.TrackingBalance)))
	defaultOwner := map[domain.MemberID]string{}
	if len(c.bootstrap.Members) > 0 {
		defaultOwner[c.bootstrap.Members[0].ID] = ""
	}
	ownershipFields := newOwnershipFields(c, c.bootstrap.Members, defaultOwner, nil)
	includeNetWorth := widget.NewCheck(t.T("accounts.includeInNetWorth"), nil)
	includeNetWorth.SetChecked(true)
	includeInvestment := widget.NewCheck(t.T("accounts.includeInInvestment"), nil)
	includeLiquid := widget.NewCheck(t.T("accounts.includeInLiquidAssets"), nil)
	none := t.T("accounts.none")
	institution := widget.NewSelect(append([]string{none}, institutionOptions(c.bootstrap.Institutions)...), nil)
	institution.SetSelected(none)
	group := widget.NewSelect(append([]string{none}, groupOptions(c.bootstrap.Groups)...), nil)
	group.SetSelected(none)
	opened := newDateEntry(t.T("accounts.datePlaceholder"))
	closed := newDateEntry(t.T("accounts.datePlaceholder"))
	category.OnChanged = func(value string) {
		parsed, err := domain.ParsePrimaryCategory(enumValue(c, value, categoryValues))
		if err != nil {
			return
		}
		options := secondaryOptions(parsed)
		secondary.SetOptions(enumOptions(c, options))
		if len(options) > 0 {
			secondary.SetSelected(enumLabel(c, options[0]))
		}
		modes := trackingOptionsForCategory(parsed)
		tracking.SetOptions(enumOptions(c, modes))
		if len(modes) > 0 {
			tracking.SetSelected(enumLabel(c, modes[0]))
		}
	}
	ownershipHint := mutedLabel(t.T("accounts.ownershipHint"))
	ownershipHint.Wrapping = fyne.TextWrapWord
	ownershipBox := container.NewVBox(ownershipFieldsView(ownershipFields), ownershipHint)
	items := []*widget.FormItem{
		widget.NewFormItem(t.T("accounts.name"), name),
		widget.NewFormItem(t.T("common.icon"), iconButton),
		widget.NewFormItem(t.T("accounts.amount"), amount),
		widget.NewFormItem(t.T("accounts.category"), category),
		widget.NewFormItem(t.T("accounts.secondaryCategory"), secondary),
		widget.NewFormItem(t.T("accounts.trackingMode"), tracking),
		widget.NewFormItem(t.T("accounts.owner"), ownershipBox),
		widget.NewFormItem(t.T("nav.institutions"), institution),
		widget.NewFormItem(t.T("nav.groups"), group),
		widget.NewFormItem(t.T("accounts.openedOn"), dateFormField(opened)),
		widget.NewFormItem(t.T("accounts.closedOn"), dateFormField(closed)),
		widget.NewFormItem(t.T("accounts.includeInNetWorth"), includeNetWorth),
		widget.NewFormItem(t.T("accounts.includeInInvestment"), includeInvestment),
		widget.NewFormItem(t.T("accounts.includeInLiquidAssets"), includeLiquid),
	}
	showResponsiveForm(c.window, t.T("accounts.createTitle"), t.T("common.save"), t.T("common.cancel"), items, fyne.NewSize(760, 720), fyne.NewSize(560, 400), func(confirm bool) {
		if !confirm {
			return
		}
		memberIDs, percentages := collectOwnership(ownershipFields)
		input := application.AccountInput{Name: name.Text, IconKey: selectedIcon, IconKeySet: true, PrimaryCategory: enumValue(c, category.Selected, categoryValues), SecondaryCategory: enumValue(c, secondary.Selected, secondaryOptionsForSelectedCategory(c, category.Selected, categoryValues)), TrackingMode: enumValue(c, tracking.Selected, trackingOptionsForSelectedCategory(c, category.Selected, categoryValues)), DefaultCurrency: c.bootstrap.Household.BaseCurrency.String(), IncludeInNetWorth: includeNetWorth.Checked, IncludeInNetWorthSet: true, IncludeInInvestment: includeInvestment.Checked, IncludeInInvestmentSet: true, IncludeInLiquidAssets: includeLiquid.Checked, IncludeInLiquidAssetsSet: true, OpenedOn: stringPointer(dateEntryValue(opened)), OpenedOnSet: true, ClosedOn: stringPointer(dateEntryValue(closed)), ClosedOnSet: true, OwnerIDs: memberIDs, OwnershipPercentages: percentages, InitialAmount: amount.Text}
		if institution.Selected != none {
			input.InstitutionID = institutionIDForName(c.bootstrap.Institutions, institution.Selected)
		}
		if group.Selected != none {
			input.GroupID = groupIDForName(c.bootstrap.Groups, group.Selected)
		}
		runBackend(c, func() error {
			_, err := c.service.CreateAccount(context.Background(), input)
			return err
		}, func(err error) {
			if err != nil {
				c.setValidationError(c.translator.TranslateError(err))
				return
			}
			c.validationError = ""
			c.reloadBackend()
			c.RefreshContent()
		})
	})
}

// ownershipField binds one member's inclusion checkbox and optional exact
// percentage entry for the Account ownership editor.
type ownershipField struct {
	memberID domain.MemberID
	check    *widget.Check
	percent  *widget.Entry
}

// newOwnershipFields builds one row per candidate owner. selected pre-checks
// a member and pre-fills their percentage text (empty means "equal split").
// archived flags a member as no longer active so its label can say so; it is
// still selectable because the caller only includes archived members that
// already own the Account being edited.
func newOwnershipFields(c *Controller, members []domain.Member, selected map[domain.MemberID]string, archived map[domain.MemberID]bool) []*ownershipField {
	t := c.translator
	fields := make([]*ownershipField, 0, len(members))
	for _, member := range members {
		label := member.Name
		if archived != nil && archived[member.ID] {
			label = fmt.Sprintf("%s (%s)", member.Name, t.T("common.archived"))
		}
		check := widget.NewCheck(label, nil)
		percent := widget.NewEntry()
		percent.SetPlaceHolder(t.T("accounts.ownershipSharePlaceholder"))
		if text, ok := selected[member.ID]; ok {
			check.SetChecked(true)
			percent.SetText(text)
		}
		fields = append(fields, &ownershipField{memberID: member.ID, check: check, percent: percent})
	}
	return fields
}

// ownershipEditorCandidates picks which members the ownership editor should
// offer for an existing Account: every active member, plus any archived
// member who already owns the Account (so their exact share stays editable
// or removable, matching what the application layer allows in
// validateOwnershipMembersForUpdate). It also returns the pre-filled
// percentage text for current owners and which of the returned members are
// archived, keyed by MemberID rather than by name so duplicate member names
// never create ambiguity.
func ownershipEditorCandidates(allMembers []domain.Member, ownership domain.Ownership) ([]domain.Member, map[domain.MemberID]string, map[domain.MemberID]bool) {
	currentOwners := make(map[domain.MemberID]bool, len(ownership.Shares()))
	selected := make(map[domain.MemberID]string, len(ownership.Shares()))
	for _, share := range ownership.Shares() {
		currentOwners[share.MemberID] = true
		selected[share.MemberID] = decimal.NewFromInt(int64(share.ShareBPS)).Div(decimal.NewFromInt(100)).StringFixed(2)
	}
	archived := map[domain.MemberID]bool{}
	candidates := make([]domain.Member, 0, len(allMembers))
	for _, member := range allMembers {
		if member.ArchivedAt == nil || currentOwners[member.ID] {
			candidates = append(candidates, member)
			archived[member.ID] = member.ArchivedAt != nil
		}
	}
	return candidates, selected, archived
}

func ownershipFieldsView(fields []*ownershipField) fyne.CanvasObject {
	rows := make([]fyne.CanvasObject, 0, len(fields))
	for _, field := range fields {
		rows = append(rows, container.NewBorder(nil, nil, field.check, nil, field.percent))
	}
	return container.NewVBox(rows...)
}

// collectOwnership reads the checked owners and their percentages. It
// returns a nil percentage slice (equal split) when every checked owner left
// their percentage blank; otherwise every checked owner's text is passed
// through so the domain layer can validate it exactly.
func collectOwnership(fields []*ownershipField) ([]domain.MemberID, []string) {
	ids := make([]domain.MemberID, 0, len(fields))
	percentages := make([]string, 0, len(fields))
	anyPercent := false
	for _, field := range fields {
		if !field.check.Checked {
			continue
		}
		ids = append(ids, field.memberID)
		text := strings.TrimSpace(field.percent.Text)
		percentages = append(percentages, text)
		if text != "" {
			anyPercent = true
		}
	}
	if !anyPercent {
		return ids, nil
	}
	return ids, percentages
}
func mediaPreview(c *Controller, assetID *domain.MediaAssetID) fyne.CanvasObject {
	if assetID == nil {
		return nil
	}
	preview := canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	preview.FillMode = canvas.ImageFillContain
	preview.CornerRadius = 8
	go func(id domain.MediaAssetID) {
		asset, err := c.service.MediaAsset(context.Background(), id)
		if err != nil {
			return
		}
		decoded, _, err := image.Decode(bytes.NewReader(asset.Data))
		if err != nil {
			return
		}
		fyne.Do(func() {
			preview.Image = decoded
			preview.Refresh()
		})
	}(*assetID)
	// Keep the image in a fixed cell so its natural dimensions cannot expand
	// the reference row beyond the card's bounds. The outer padding keeps the
	// image away from the card's left and top edges.
	cell := container.NewGridWrap(fyne.NewSize(40, 40), preview)
	return container.New(layout.NewCustomPaddedLayout(8, 4, 8, 4), cell)
}

const rowActionButtonHeight = 32

func rowActionButton(label string, tapped func()) fyne.CanvasObject {
	button := widget.NewButton(label, tapped)
	// Keep row actions visually discoverable even when they are not hovered.
	// LowImportance intentionally removes the button background until hover;
	// the default importance uses the theme's subtle button surface instead.
	button.Importance = widget.MediumImportance
	return container.NewGridWrap(fyne.NewSize(button.MinSize().Width, rowActionButtonHeight), button)
}

func rowActionBar(buttons ...fyne.CanvasObject) fyne.CanvasObject {
	actions := container.New(layout.NewCustomPaddedHBoxLayout(6), buttons...)
	padded := container.New(layout.NewCustomPaddedLayout(6, 6, 4, 4), actions)
	return container.NewCenter(padded)
}

func accountRow(c *Controller, record domain.AccountRecord) fyne.CanvasObject {
	value := c.translator.T("accounts.noValue")
	currentAmount := ""
	if record.LatestValue != nil {
		currentAmount = record.LatestValue.Amount.CanonicalAmount()
		value = format.Money(record.LatestValue.Amount.CanonicalAmount(), record.LatestValue.Amount.Currency().String(), c.preference)
	}
	archived := record.Account.ArchivedAt != nil
	actionLabel := c.translator.T("accounts.archive")
	if archived {
		actionLabel = c.translator.T("common.restore")
	}
	archive := rowActionButton(actionLabel, func() {
		runBackend(c, func() error {
			return c.service.ArchiveAccount(context.Background(), record.Account.ID, !archived)
		}, func(err error) {
			if err != nil {
				c.setValidationError(c.translator.TranslateError(err))
				return
			}
			c.validationError = ""
			c.reloadBackend()
			c.RefreshContent()
		})
	})
	editButton := rowActionButton(c.translator.T("common.edit"), func() { showAccountEditDialog(c, record) })
	updateButton := rowActionButton(c.translator.T("accounts.updateValue"), func() {
		amount := widget.NewEntry()
		amount.SetText(currentAmount)
		date := newDateEntry(c.translator.T("accounts.datePlaceholder"))
		showResponsiveForm(c.window, c.translator.T("accounts.updateValue"), c.translator.T("common.save"), c.translator.T("common.cancel"), []*widget.FormItem{widget.NewFormItem(c.translator.T("accounts.amount"), amount), widget.NewFormItem(c.translator.T("accounts.effectiveDate"), dateFormField(date))}, fyne.NewSize(560, 300), fyne.NewSize(460, 250), func(confirm bool) {
			if !confirm {
				return
			}
			runBackend(c, func() error {
				_, err := c.service.AppendAccountValue(context.Background(), record.Account.ID, amount.Text, dateEntryValue(date))
				return err
			}, func(err error) {
				if err != nil {
					c.setValidationError(c.translator.TranslateError(err))
					return
				}
				c.validationError = ""
				c.reloadBackend()
				c.RefreshContent()
			})
		})
	})
	actions := rowActionBar(editButton, updateButton, archive)
	nameParts := []fyne.CanvasObject{widget.NewLabelWithStyle(record.Account.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})}
	nameParts = append([]fyne.CanvasObject{iconPreview(record.Account.IconKey)}, nameParts...)
	nameView := fyne.CanvasObject(container.NewHBox(nameParts...))
	return settingsSurface(container.NewBorder(nil, nil, nameView, actions, keyValueRow(enumLabel(c, record.Account.PrimaryCategory.String()), value)), fyne.NewSize(1, 60))
}
func showAccountEditDialog(c *Controller, record domain.AccountRecord) {
	t := c.translator
	if record.LatestValue == nil {
		c.setValidationError(t.T("accounts.noValue"))
		return
	}
	name := widget.NewEntry()
	name.SetText(record.Account.Name)
	selectedIcon := iconKeyValue(record.Account.IconKey, domain.DefaultAccountIcon)
	iconButton := newIconPickerButton(c, selectedIcon, func(key string) { selectedIcon = key })
	note := widget.NewEntry()
	if record.Account.Note != nil {
		note.SetText(*record.Account.Note)
	}
	allMembers, membersErr := c.service.ListMembers(context.Background(), true)
	if membersErr != nil {
		allMembers = c.bootstrap.Members
	}
	displayMembers, selectedOwnership, archivedFlags := ownershipEditorCandidates(allMembers, record.Ownership)
	ownershipFields := newOwnershipFields(c, displayMembers, selectedOwnership, archivedFlags)
	categoryValues := []string{string(domain.CategoryCashEquivalent), string(domain.CategoryInvestment), string(domain.CategoryProperty), string(domain.CategoryReceivable), string(domain.CategoryLiability)}
	category := widget.NewSelect(enumOptions(c, categoryValues), nil)
	category.SetSelected(enumLabel(c, record.Account.PrimaryCategory.String()))
	secondary := widget.NewSelect(enumOptions(c, secondaryOptions(record.Account.PrimaryCategory)), nil)
	secondary.SetSelected(enumLabel(c, string(record.Account.SecondaryCategory)))
	tracking := widget.NewSelect(enumOptions(c, []string{string(record.Account.TrackingMode)}), nil)
	tracking.SetSelected(enumLabel(c, string(record.Account.TrackingMode)))
	tracking.Disable()
	institution := widget.NewSelect(append([]string{t.T("accounts.none")}, institutionOptions(c.bootstrap.Institutions)...), nil)
	if record.Account.InstitutionID == nil {
		institution.SetSelected(t.T("accounts.none"))
	} else {
		institution.SetSelected(institutionLabelForID(c.bootstrap.Institutions, record.Account.InstitutionID.String()))
	}
	group := widget.NewSelect(append([]string{t.T("accounts.none")}, groupOptions(c.bootstrap.Groups)...), nil)
	if record.Account.GroupID == nil {
		group.SetSelected(t.T("accounts.none"))
	} else {
		group.SetSelected(groupLabelForID(c.bootstrap.Groups, record.Account.GroupID.String()))
	}
	opened := newDateEntry(t.T("accounts.datePlaceholder"))
	if record.Account.OpenedOn != nil {
		setDateEntryISO(opened, *record.Account.OpenedOn)
	}
	closed := newDateEntry(t.T("accounts.datePlaceholder"))
	if record.Account.ClosedOn != nil {
		setDateEntryISO(closed, *record.Account.ClosedOn)
	}
	includeNetWorth := widget.NewCheck(t.T("accounts.includeInNetWorth"), nil)
	includeNetWorth.SetChecked(record.Account.IncludeInNetWorth)
	includeInvestment := widget.NewCheck(t.T("accounts.includeInInvestment"), nil)
	includeInvestment.SetChecked(record.Account.IncludeInInvestment)
	includeLiquid := widget.NewCheck(t.T("accounts.includeInLiquidAssets"), nil)
	includeLiquid.SetChecked(record.Account.IncludeInLiquidAssets)
	category.OnChanged = func(value string) {
		if parsed, err := domain.ParsePrimaryCategory(enumValue(c, value, categoryValues)); err == nil {
			options := secondaryOptions(parsed)
			secondary.SetOptions(enumOptions(c, options))
			if len(options) > 0 {
				secondary.SetSelected(enumLabel(c, options[0]))
			}
		}
	}
	ownershipHint := mutedLabel(t.T("accounts.ownershipHint"))
	ownershipHint.Wrapping = fyne.TextWrapWord
	ownershipBox := container.NewVBox(ownershipFieldsView(ownershipFields), ownershipHint)
	items := []*widget.FormItem{widget.NewFormItem(t.T("accounts.name"), name), widget.NewFormItem(t.T("common.icon"), iconButton), widget.NewFormItem(t.T("accounts.category"), category), widget.NewFormItem(t.T("accounts.secondaryCategory"), secondary), widget.NewFormItem(t.T("accounts.trackingMode"), tracking), widget.NewFormItem(t.T("accounts.owner"), ownershipBox), widget.NewFormItem(t.T("accounts.note"), note), widget.NewFormItem(t.T("nav.institutions"), institution), widget.NewFormItem(t.T("nav.groups"), group), widget.NewFormItem(t.T("accounts.openedOn"), dateFormField(opened)), widget.NewFormItem(t.T("accounts.closedOn"), dateFormField(closed)), widget.NewFormItem(t.T("accounts.includeInNetWorth"), includeNetWorth), widget.NewFormItem(t.T("accounts.includeInInvestment"), includeInvestment), widget.NewFormItem(t.T("accounts.includeInLiquidAssets"), includeLiquid)}
	showResponsiveForm(c.window, t.T("accounts.edit"), t.T("common.save"), t.T("common.cancel"), items, fyne.NewSize(720, 680), fyne.NewSize(560, 360), func(confirm bool) {
		if !confirm {
			return
		}
		memberIDs, percentages := collectOwnership(ownershipFields)
		input := application.AccountInput{Name: name.Text, IconKey: selectedIcon, IconKeySet: true, PrimaryCategory: enumValue(c, category.Selected, categoryValues), SecondaryCategory: enumValue(c, secondary.Selected, secondaryOptionsForSelectedCategory(c, category.Selected, categoryValues)), TrackingMode: string(record.Account.TrackingMode), DefaultCurrency: record.Account.DefaultCurrency.String(), InstitutionIDSet: true, GroupIDSet: true, Note: stringPointer(note.Text), NoteSet: true, IncludeInNetWorth: includeNetWorth.Checked, IncludeInInvestment: includeInvestment.Checked, IncludeInLiquidAssets: includeLiquid.Checked, OpenedOn: stringPointer(dateEntryValue(opened)), OpenedOnSet: true, ClosedOn: stringPointer(dateEntryValue(closed)), ClosedOnSet: true, OwnerIDs: memberIDs, OwnershipPercentages: percentages, InitialAmount: record.LatestValue.Amount.CanonicalAmount()}
		if institution.Selected != t.T("accounts.none") {
			input.InstitutionID = institutionIDForName(c.bootstrap.Institutions, institution.Selected)
		}
		if group.Selected != t.T("accounts.none") {
			input.GroupID = groupIDForName(c.bootstrap.Groups, group.Selected)
		}
		runBackend(c, func() error {
			_, err := c.service.UpdateAccount(context.Background(), record.Account.ID, input)
			return err
		}, func(err error) {
			if err != nil {
				c.setValidationError(c.translator.TranslateError(err))
				return
			}
			c.validationError = ""
			c.reloadBackend()
			c.RefreshContent()
		})
	})
}

func NewMembersPage(c *Controller) fyne.CanvasObject {
	members, err := c.service.ListMembers(context.Background(), true)
	if err != nil {
		return errorPanel(c, c.translator.T("members.loadError"), err)
	}
	add := widget.NewButton(c.translator.T("common.add"), func() {
		showMemberCreateDialog(c)
	})
	rows := []fyne.CanvasObject{liveFeedback(c), simpleAddAction(add)}
	for _, member := range members {
		current := member
		rows = append(rows, referenceRow(c, current.Name, current.ArchivedAt != nil, func(archived bool) error { return c.service.ArchiveMember(context.Background(), current.ID, archived) }, mediaPreview(c, current.AvatarAssetID), func() {
			showMemberEditDialog(c, current)
		}))
	}
	return container.NewVBox(rows...)
}
func NewInstitutionsPage(c *Controller) fyne.CanvasObject {
	institutions, err := c.service.ListInstitutions(context.Background(), true)
	if err != nil {
		return errorPanel(c, c.translator.T("institutions.loadError"), err)
	}
	add := widget.NewButton(c.translator.T("common.add"), func() {
		showReferenceCreateDialog(c, c.translator.T("institutions.createTitle"), c.translator.T("institutions.name"), domain.DefaultInstitutionIcon, func(name, iconKey string) error {
			_, err := c.service.CreateInstitution(context.Background(), name, iconKey)
			return err
		})
	})
	rows := []fyne.CanvasObject{liveFeedback(c), simpleAddAction(add)}
	for _, item := range institutions {
		current := item
		rows = append(rows, referenceRow(c, current.Name, current.ArchivedAt != nil, func(archived bool) error {
			return c.service.ArchiveInstitution(context.Background(), current.ID, archived)
		}, iconPreviewWithFallback(current.IconKey, domain.DefaultInstitutionIcon), func() {
			showInstitutionEditDialog(c, current)
		}))
	}
	return container.NewVBox(rows...)
}
func NewGroupsPage(c *Controller) fyne.CanvasObject {
	groups, err := c.service.ListGroups(context.Background(), true)
	if err != nil {
		return errorPanel(c, c.translator.T("groups.loadError"), err)
	}
	add := widget.NewButton(c.translator.T("common.add"), func() {
		showReferenceCreateDialog(c, c.translator.T("groups.createTitle"), c.translator.T("groups.name"), domain.DefaultGroupIcon, func(name, iconKey string) error {
			_, err := c.service.CreateGroup(context.Background(), name, iconKey)
			return err
		})
	})
	rows := []fyne.CanvasObject{liveFeedback(c), simpleAddAction(add)}
	for _, item := range groups {
		current := item
		rows = append(rows, referenceRow(c, current.Name, current.ArchivedAt != nil, func(archived bool) error { return c.service.ArchiveGroup(context.Background(), current.ID, archived) }, iconPreviewWithFallback(current.IconKey, domain.DefaultGroupIcon), func() {
			showGroupEditDialog(c, current)
		}))
	}
	return container.NewVBox(rows...)
}

func simpleAddAction(action fyne.CanvasObject) fyne.CanvasObject {
	return container.New(layout.NewCustomPaddedLayout(8, 8, 0, 0), action)
}

func showReferenceCreateDialog(c *Controller, title, fieldLabel, defaultIcon string, create func(string, string) error) {
	name := widget.NewEntry()
	selectedIcon := defaultIcon
	items := []*widget.FormItem{widget.NewFormItem(fieldLabel, name)}
	if defaultIcon != "" {
		iconButton := newIconPickerButton(c, defaultIcon, func(key string) { selectedIcon = key })
		items = append(items, widget.NewFormItem(c.translator.T("common.icon"), iconButton))
	}
	showResponsiveForm(c.window, title, c.translator.T("common.save"), c.translator.T("common.cancel"), items, fyne.NewSize(520, 280), fyne.NewSize(440, 220), func(confirm bool) {
		if !confirm {
			return
		}
		runBackend(c, func() error { return create(name.Text, selectedIcon) }, func(err error) {
			if err != nil {
				c.setValidationError(c.translator.TranslateError(err))
				return
			}
			c.validationError = ""
			c.reloadBackend()
			c.RefreshContent()
		})
	})
}

func showMemberCreateDialog(c *Controller) {
	name := widget.NewEntry()
	var selectedImage []byte
	imageButton := newImagePickerButton(c, func(data []byte) { selectedImage = data })
	showResponsiveForm(c.window, c.translator.T("members.createTitle"), c.translator.T("common.save"), c.translator.T("common.cancel"), []*widget.FormItem{
		widget.NewFormItem(c.translator.T("members.name"), name),
		widget.NewFormItem(c.translator.T("common.media"), imageButton),
	}, fyne.NewSize(520, 320), fyne.NewSize(440, 240), func(confirm bool) {
		if !confirm {
			return
		}
		runBackend(c, func() error {
			member, err := c.service.CreateMember(context.Background(), name.Text)
			if err != nil || len(selectedImage) == 0 {
				return err
			}
			asset, err := c.service.CreateMediaAsset(context.Background(), "image/png", selectedImage)
			if err != nil {
				return err
			}
			return c.service.SetMemberAvatar(context.Background(), member.ID, asset.ID)
		}, func(err error) {
			if err != nil {
				c.setValidationError(c.translator.TranslateError(err))
				return
			}
			c.validationError = ""
			c.reloadBackend()
			c.RefreshContent()
		})
	})
}

func referenceRow(c *Controller, name string, archived bool, toggle func(bool) error, preview fyne.CanvasObject, edit func()) fyne.CanvasObject {
	status := c.translator.T("common.active")
	if archived {
		status = c.translator.T("common.archived")
	}
	actionLabel := c.translator.T("common.archive")
	if archived {
		actionLabel = c.translator.T("common.restore")
	}
	actionButton := rowActionButton(actionLabel, func() {
		runBackend(c, func() error { return toggle(!archived) }, func(err error) {
			if err != nil {
				c.setValidationError(c.translator.TranslateError(err))
				return
			}
			c.validationError = ""
			c.reloadBackend()
			c.RefreshContent()
		})
	})
	editButton := rowActionButton(c.translator.T("common.edit"), edit)
	nameParts := []fyne.CanvasObject{widget.NewLabelWithStyle(name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})}
	if preview != nil {
		nameParts = append([]fyne.CanvasObject{preview}, nameParts...)
	}
	nameView := fyne.CanvasObject(container.NewHBox(nameParts...))
	return settingsSurface(container.NewBorder(nil, nil, nameView, rowActionBar(editButton, actionButton), widget.NewLabel(status)), fyne.NewSize(1, 54))
}

func showMemberEditDialog(c *Controller, current domain.Member) {
	name := widget.NewEntry()
	name.SetText(current.Name)
	var selectedImage []byte
	imageButton := newImagePickerButton(c, func(data []byte) { selectedImage = data })
	showReferenceEditDialog(c, []*widget.FormItem{
		widget.NewFormItem(c.translator.T("members.name"), name),
		widget.NewFormItem(c.translator.T("common.media"), imageButton),
	}, func() error {
		if _, err := c.service.UpdateMember(context.Background(), current.ID, name.Text); err != nil {
			return err
		}
		if len(selectedImage) == 0 {
			return nil
		}
		asset, err := c.service.CreateMediaAsset(context.Background(), "image/png", selectedImage)
		if err != nil {
			return err
		}
		return c.service.SetMemberAvatar(context.Background(), current.ID, asset.ID)
	})
}

func showInstitutionEditDialog(c *Controller, current domain.Institution) {
	name := widget.NewEntry()
	name.SetText(current.Name)
	selectedIcon := iconKeyValue(current.IconKey, domain.DefaultInstitutionIcon)
	iconButton := newIconPickerButton(c, selectedIcon, func(key string) { selectedIcon = key })
	showReferenceEditDialog(c, []*widget.FormItem{
		widget.NewFormItem(c.translator.T("institutions.name"), name),
		widget.NewFormItem(c.translator.T("common.icon"), iconButton),
	}, func() error {
		if _, err := c.service.UpdateInstitution(context.Background(), current.ID, name.Text); err != nil {
			return err
		}
		if selectedIcon == iconKeyValue(current.IconKey, domain.DefaultInstitutionIcon) {
			return nil
		}
		return c.service.SetInstitutionIcon(context.Background(), current.ID, selectedIcon)
	})
}

func showGroupEditDialog(c *Controller, current domain.Group) {
	name := widget.NewEntry()
	name.SetText(current.Name)
	selectedIcon := iconKeyValue(current.IconKey, domain.DefaultGroupIcon)
	iconButton := newIconPickerButton(c, selectedIcon, func(key string) { selectedIcon = key })
	showReferenceEditDialog(c, []*widget.FormItem{
		widget.NewFormItem(c.translator.T("groups.name"), name),
		widget.NewFormItem(c.translator.T("common.icon"), iconButton),
	}, func() error {
		if _, err := c.service.UpdateGroup(context.Background(), current.ID, name.Text); err != nil {
			return err
		}
		if selectedIcon == iconKeyValue(current.IconKey, domain.DefaultGroupIcon) {
			return nil
		}
		return c.service.SetGroupIcon(context.Background(), current.ID, selectedIcon)
	})
}

func showReferenceEditDialog(c *Controller, items []*widget.FormItem, save func() error) {
	showResponsiveForm(c.window, c.translator.T("common.edit"), c.translator.T("common.save"), c.translator.T("common.cancel"), items, fyne.NewSize(560, 320), fyne.NewSize(440, 240), func(confirm bool) {
		if !confirm {
			return
		}
		runBackend(c, save, func(err error) {
			if err != nil {
				c.setValidationError(c.translator.TranslateError(err))
				return
			}
			c.validationError = ""
			c.reloadBackend()
			c.RefreshContent()
		})
	})
}

func newImagePickerButton(c *Controller, changed func([]byte)) *widget.Button {
	button := widget.NewButtonWithIcon(c.translator.T("common.media"), fyneTheme.Current().Icon(fyneTheme.IconNameFileImage), nil)
	button.Importance = widget.MediumImportance
	button.OnTapped = func() {
		openImagePicker(c.window, c.translator.T("common.media"), func(reader io.ReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			go func() {
				defer reader.Close()
				data, normalizeErr := media.ReadAndNormalize(reader)
				fyne.Do(func() {
					if normalizeErr != nil {
						c.setValidationError(c.translator.TranslateError(normalizeErr))
						return
					}
					changed(data)
				})
			}()
		})
	}
	return button
}

func errorPanel(c *Controller, title string, err error) fyne.CanvasObject {
	return container.NewCenter(surface(container.NewVBox(widget.NewLabelWithStyle(title, fyne.TextAlignCenter, fyne.TextStyle{Bold: true}), widget.NewLabel(c.translator.TranslateError(err))), fyne.NewSize(500, 160)))
}

func liveFeedback(c *Controller) fyne.CanvasObject {
	if c.validationError == "" {
		return container.NewWithoutLayout()
	}
	label := widget.NewLabel(c.validationError)
	label.Importance = widget.DangerImportance
	return label
}
func runBackend(c *Controller, work func() error, done func(error)) {
	go func() {
		err := work()
		fyne.Do(func() { done(err) })
	}()
}

func stringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
func enumLabel(c *Controller, value string) string {
	if translated := c.translator.T("enum." + value); translated != "" {
		return translated
	}
	return value
}
func enumOptions(c *Controller, values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, enumLabel(c, value))
	}
	return result
}

func enumValue(c *Controller, label string, values []string) string {
	for _, value := range values {
		if enumLabel(c, value) == label {
			return value
		}
	}
	return label
}

func secondaryOptionsForSelectedCategory(c *Controller, selected string, categories []string) []string {
	category, err := domain.ParsePrimaryCategory(enumValue(c, selected, categories))
	if err != nil {
		return nil
	}
	return secondaryOptions(category)
}

func trackingOptionsForSelectedCategory(c *Controller, selected string, categories []string) []string {
	category, err := domain.ParsePrimaryCategory(enumValue(c, selected, categories))
	if err != nil {
		return nil
	}
	return trackingOptionsForCategory(category)
}
func emptyPanel(title, description string) fyne.CanvasObject {
	return surface(container.NewVBox(widget.NewLabelWithStyle(title, fyne.TextAlignCenter, fyne.TextStyle{Bold: true}), widget.NewLabel(description)), fyne.NewSize(1, 150))
}

func referenceLabel(name, id string, duplicate bool) string {
	if !duplicate {
		return name
	}
	if len(id) > 8 {
		id = id[:8]
	}
	return fmt.Sprintf("%s (%s)", name, id)
}

func memberOptions(items []domain.Member) []string {
	counts := make(map[string]int)
	for _, item := range items {
		counts[item.Name]++
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, referenceLabel(item.Name, item.ID.String(), counts[item.Name] > 1))
	}
	return result
}

func institutionOptions(items []domain.Institution) []string {
	counts := make(map[string]int)
	for _, item := range items {
		counts[item.Name]++
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, referenceLabel(item.Name, item.ID.String(), counts[item.Name] > 1))
	}
	return result
}

func groupOptions(items []domain.Group) []string {
	counts := make(map[string]int)
	for _, item := range items {
		counts[item.Name]++
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, referenceLabel(item.Name, item.ID.String(), counts[item.Name] > 1))
	}
	return result
}

func memberIDForName(items []domain.Member, name string) string {
	for index, item := range items {
		if memberOptionLabel(items, index) == name {
			return item.ID.String()
		}
	}
	return ""
}

func institutionIDForName(items []domain.Institution, name string) string {
	for index, item := range items {
		if institutionOptionLabel(items, index) == name {
			return item.ID.String()
		}
	}
	return ""
}

func groupIDForName(items []domain.Group, name string) string {
	for index, item := range items {
		if groupOptionLabel(items, index) == name {
			return item.ID.String()
		}
	}
	return ""
}

func memberOptionLabel(items []domain.Member, index int) string {
	counts := make(map[string]int)
	for _, item := range items {
		counts[item.Name]++
	}
	return referenceLabel(items[index].Name, items[index].ID.String(), counts[items[index].Name] > 1)
}

func institutionOptionLabel(items []domain.Institution, index int) string {
	counts := make(map[string]int)
	for _, item := range items {
		counts[item.Name]++
	}
	return referenceLabel(items[index].Name, items[index].ID.String(), counts[items[index].Name] > 1)
}

func groupOptionLabel(items []domain.Group, index int) string {
	counts := make(map[string]int)
	for _, item := range items {
		counts[item.Name]++
	}
	return referenceLabel(items[index].Name, items[index].ID.String(), counts[items[index].Name] > 1)
}

func memberLabelForID(items []domain.Member, id string) string {
	for index, item := range items {
		if item.ID.String() == id {
			return memberOptionLabel(items, index)
		}
	}
	return ""
}

func institutionLabelForID(items []domain.Institution, id string) string {
	for index, item := range items {
		if item.ID.String() == id {
			return institutionOptionLabel(items, index)
		}
	}
	return ""
}

func groupLabelForID(items []domain.Group, id string) string {
	for index, item := range items {
		if item.ID.String() == id {
			return groupOptionLabel(items, index)
		}
	}
	return ""
}

func splitNames(value string) []string {
	value = strings.ReplaceAll(value, "，", ",")
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if name := strings.TrimSpace(part); name != "" {
			result = append(result, name)
		}
	}
	return result
}

func trackingOptionsForCategory(category domain.PrimaryCategory) []string {
	if category == domain.CategoryInvestment {
		return []string{string(domain.TrackingManualValue)}
	}
	if category == domain.CategoryProperty || category == domain.CategoryReceivable {
		return []string{string(domain.TrackingManualValue)}
	}
	return []string{string(domain.TrackingBalance)}
}

func secondaryOptions(primary domain.PrimaryCategory) []string {
	result := make([]string, 0, len(secondaryByPrimaryForUI[primary]))
	for _, value := range secondaryByPrimaryForUI[primary] {
		result = append(result, string(value))
	}
	return result
}

var secondaryByPrimaryForUI = map[domain.PrimaryCategory][]domain.SecondaryCategory{
	domain.CategoryCashEquivalent: {domain.SecondaryCash, domain.SecondaryBankAccount, domain.SecondaryDigitalWallet, domain.SecondaryBrokerCash, domain.SecondaryOtherCashEquivalent},
	domain.CategoryInvestment:     {domain.SecondaryBrokerageAccount, domain.SecondaryInvestmentFundAccount, domain.SecondaryBankInvestmentProduct, domain.SecondaryInsurance, domain.SecondaryManualInvestment, domain.SecondaryOtherInvestment},
	domain.CategoryProperty:       {domain.SecondaryRealEstate, domain.SecondaryVehicle, domain.SecondaryCollectible, domain.SecondaryOtherProperty},
	domain.CategoryReceivable:     {domain.SecondaryLoanReceivable, domain.SecondaryOtherReceivable},
	domain.CategoryLiability:      {domain.SecondaryCreditCard, domain.SecondaryMortgage, domain.SecondaryAutoLoan, domain.SecondaryConsumerLoan, domain.SecondaryPersonalDebt, domain.SecondaryOtherLiability},
}
