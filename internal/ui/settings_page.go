package ui

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	fyneTheme "fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type selectOption struct {
	value string
	label string
}

// NewSettingsPage builds the presentation preferences and market-data routing
// page. Explicit provider selection is persisted to local JSON.
func NewSettingsPage(controller *Controller) fyne.CanvasObject {
	t := controller.translator
	pref := controller.preference

	var household fyne.CanvasObject
	if controller.bootstrap.Household != nil {
		household = householdSummaryCard(controller)
	}

	appearance := sectionCard(
		t.T("settings.appearance.title"),
		t.T("settings.appearance.description"),
		settingsRows(
			settingsRow(t.T("settings.appearance.mode"), preferenceSelect(controller,
				[]selectOption{
					{string(settings.AppearanceSystem), t.T("option.appearance.system")},
					{string(settings.AppearanceLight), t.T("option.appearance.light")},
					{string(settings.AppearanceDark), t.T("option.appearance.dark")},
				}, string(pref.Appearance), func(value string) {
					controller.updatePreference(func(next *settings.Settings) { next.Appearance = settings.Appearance(value) })
				})),
			settingsRow(t.T("settings.appearance.accent"), preferenceSelect(controller,
				[]selectOption{
					{string(settings.AccentNestworth), t.T("option.accent.nestworth")},
					{string(settings.AccentOcean), t.T("option.accent.ocean")},
					{string(settings.AccentForest), t.T("option.accent.forest")},
					{string(settings.AccentAmber), t.T("option.accent.amber")},
					{string(settings.AccentRose), t.T("option.accent.rose")},
				}, string(pref.Accent), func(value string) {
					controller.updatePreference(func(next *settings.Settings) { next.Accent = settings.Accent(value) })
				})),
		),
	)

	languageRegion := sectionCard(
		t.T("settings.language.title"),
		t.T("settings.language.description"),
		settingsRows(
			settingsRow(t.T("settings.language.language"), preferenceSelect(controller,
				[]selectOption{
					{string(settings.LanguageSystem), t.T("option.language.system")},
					{string(settings.LanguageEnglish), t.T("option.language.en")},
					{string(settings.LanguageZhCN), t.T("option.language.zhCN")},
					{string(settings.LanguageZhTW), t.T("option.language.zhTW")},
				}, string(pref.Language), func(value string) {
					controller.updatePreference(func(next *settings.Settings) { next.Language = settings.Language(value) })
				})),
			settingsRow(t.T("settings.language.timezone"), timezoneEntry(controller)),
			settingsRow(t.T("settings.language.weekStart"), preferenceSelect(controller,
				[]selectOption{
					{settings.WeekStartMonday, t.T("option.week.monday")},
					{settings.WeekStartSunday, t.T("option.week.sunday")},
				}, pref.WeekStart, func(value string) {
					controller.updatePreference(func(next *settings.Settings) { next.WeekStart = value })
				})),
			settingsRow(t.T("settings.language.dateFormat"), preferenceSelect(controller,
				[]selectOption{
					{settings.DateFormatISO, t.T("option.date.iso")},
					{settings.DateFormatDayFirst, t.T("option.date.dayFirst")},
					{settings.DateFormatMonthFirst, t.T("option.date.monthFirst")},
					{settings.DateFormatLocalized, t.T("option.date.localized")},
				}, pref.DateFormat, func(value string) {
					controller.updatePreference(func(next *settings.Settings) { next.DateFormat = value })
				})),
			settingsRow(t.T("settings.language.timeFormat"), preferenceSelect(controller,
				[]selectOption{
					{settings.TimeFormat24, t.T("option.time.24h")},
					{settings.TimeFormat12, t.T("option.time.12h")},
				}, pref.TimeFormat, func(value string) {
					controller.updatePreference(func(next *settings.Settings) { next.TimeFormat = value })
				})),
		),
	)

	currencyOptions := []selectOption{
		{value: "CNY", label: "CNY · ¥"},
		{value: "USD", label: "USD · $"},
		{value: "SGD", label: "SGD · S$"},
		{value: "EUR", label: "EUR · €"},
		{value: "JPY", label: "JPY · ¥"},
		{value: "HKD", label: "HKD · HK$"},
		{value: "TWD", label: "TWD · NT$"},
		{value: "GBP", label: "GBP · £"},
		{value: "AUD", label: "AUD · A$"},
	}
	numbers := sectionCard(
		t.T("settings.numbers.title"),
		t.T("settings.numbers.description"),
		settingsRows(
			settingsRow(t.T("settings.numbers.currency"), preferenceSelect(controller, currencyOptions, pref.Currency, func(value string) {
				controller.updatePreference(func(next *settings.Settings) { next.Currency = value })
			})),
			settingsRow(t.T("settings.numbers.decimal"), preferenceSelect(controller,
				[]selectOption{
					{settings.DecimalDot, "."},
					{settings.DecimalComma, ","},
				}, pref.DecimalSeparator, func(value string) {
					controller.updatePreference(func(next *settings.Settings) { next.DecimalSeparator = value })
				})),
			settingsRow(t.T("settings.numbers.grouping"), preferenceSelect(controller,
				[]selectOption{
					{settings.GroupingComma, ","},
					{settings.GroupingDot, "."},
					{settings.GroupingSpace, t.T("option.grouping.space")},
					{settings.GroupingApost, "'"},
					{settings.GroupingNone, t.T("option.grouping.none")},
				}, pref.GroupingSeparator, func(value string) {
					controller.updatePreference(func(next *settings.Settings) { next.GroupingSeparator = value })
				})),
			settingsRow(t.T("settings.numbers.places"), preferenceSelect(controller,
				[]selectOption{{"0", "0"}, {"2", "2"}, {"4", "4"}},
				fmtInt(pref.DecimalPlaces), func(value string) {
					controller.updatePreference(func(next *settings.Settings) {
						if value == "0" {
							next.DecimalPlaces = 0
						} else if value == "4" {
							next.DecimalPlaces = 4
						} else {
							next.DecimalPlaces = 2
						}
					})
				})),
			formatPreview(controller),
		),
	)

	var providers fyne.CanvasObject
	if options := fxProviderOptions(controller); len(options) > 0 {
		providers = sectionCard(
			t.T("settings.providers.title"),
			t.T("settings.providers.description"),
			settingsRows(
				settingsRow(t.T("settings.providers.fxProvider"), preferenceSelect(controller, options, pref.FXProvider, func(value string) {
					controller.updatePreference(func(next *settings.Settings) { next.FXProvider = value })
				})),
			),
		)
	}

	reset := widget.NewButtonWithIcon(t.T("common.reset"), fyneTheme.Current().Icon(fyneTheme.IconNameViewRefresh), func() {
		showResponsiveConfirm(controller.window, t.T("settings.reset.title"), t.T("settings.reset.body"), func(confirmed bool) {
			if confirmed {
				controller.resetPreference()
			}
		})
	})
	reset.Importance = widget.LowImportance
	status := widget.NewLabel(t.T("common.saved"))
	if controller.saveError != nil {
		status.SetText(t.T("settings.saveError"))
	}
	if controller.validationError != "" {
		status.SetText(t.T("common.invalid") + ": " + controller.validationError)
	}
	status.Wrapping = fyne.TextWrapWord
	footer := container.NewBorder(nil, nil, status, reset)

	sections := make([]fyne.CanvasObject, 0, 7)
	if household != nil {
		sections = append(sections, household)
	}
	sections = append(sections, appearance, languageRegion, numbers)
	if providers != nil {
		sections = append(sections, providers)
	}
	sections = append(sections, layout.NewSpacer(), footer)
	return container.NewVBox(sections...)
}

// householdSummaryCard shows the read-only Household name and base currency
// required by the v0.1.1 release contract (docs/releases/v0.1.1.md). Both
// values are set once during onboarding and are not editable here.
func householdSummaryCard(controller *Controller) fyne.CanvasObject {
	t := controller.translator
	household := controller.bootstrap.Household
	return sectionCard(
		t.T("settings.household.title"),
		t.T("settings.household.description"),
		settingsRows(
			settingsRow(t.T("settings.household.name"), widget.NewLabel(household.Name)),
			settingsRow(t.T("settings.household.baseCurrency"), widget.NewLabel(household.BaseCurrency.String())),
		),
	)
}

func preferenceSelect(controller *Controller, options []selectOption, current string, changed func(string)) fyne.CanvasObject {
	labels := make([]string, 0, len(options))
	valuesByLabel := make(map[string]string, len(options))
	selected := ""
	for _, option := range options {
		labels = append(labels, option.label)
		valuesByLabel[option.label] = option.value
		if option.value == current {
			selected = option.label
		}
	}
	control := widget.NewSelect(labels, nil)
	if selected != "" {
		control.Selected = selected
	}
	control.OnChanged = func(label string) {
		if value, ok := valuesByLabel[label]; ok {
			changed(value)
		}
	}
	return control
}

func fxProviderOptions(controller *Controller) []selectOption {
	if controller == nil || controller.service == nil || controller.service.MarketDataRegistry() == nil {
		return nil
	}
	options := make([]selectOption, 0)
	for _, provider := range controller.service.MarketDataRegistry().Providers() {
		if provider == nil || !provider.Capabilities().LatestFX {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(provider.Key()))
		if key == "" {
			continue
		}
		options = append(options, selectOption{value: key, label: fxProviderLabel(controller, key)})
	}
	return options
}

func fxProviderLabel(controller *Controller, key string) string {
	switch key {
	case application.YahooFinanceProviderKey:
		return controller.translator.T("settings.provider.yahoo")
	case settings.FXProviderFrankfurter:
		return controller.translator.T("settings.provider.frankfurter")
	default:
		return key
	}
}

func timezoneEntry(controller *Controller) fyne.CanvasObject {
	pref := controller.preference
	t := controller.translator
	systemLabel := t.T("option.timezone.system")
	options := []string{systemLabel, "UTC", "Asia/Singapore", "Asia/Shanghai", "Asia/Taipei", "Asia/Tokyo", "Europe/London", "America/New_York", "America/Los_Angeles"}
	entry := widget.NewSelectEntry(options)
	value := pref.Timezone
	if value == settings.TimezoneSystem {
		value = systemLabel
	}
	entry.SetText(value)
	apply := func(raw string) {
		value := strings.TrimSpace(raw)
		if value == systemLabel {
			value = settings.TimezoneSystem
		}
		if err := format.ValidateTimezone(value); err != nil {
			controller.setValidationError(controller.translator.TranslateError(err))
			return
		}
		controller.updatePreference(func(next *settings.Settings) { next.Timezone = value })
	}
	entry.OnChanged = func(value string) {
		for _, option := range options {
			if value == option {
				apply(value)
				return
			}
		}
	}
	entry.OnSubmitted = apply
	return entry
}

func formatPreview(controller *Controller) fyne.CanvasObject {
	pref := controller.preference
	formatted := format.Money("1234567.895", pref.Currency, pref)
	when, err := format.DateTime(time.Date(2026, time.August, 21, 14, 35, 0, 0, time.UTC), pref, controller.translator.Language())
	if err != nil {
		when = controller.translator.TranslateError(err)
	}
	previewTitle := widget.NewLabelWithStyle(controller.translator.T("settings.numbers.preview"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	previewTitle.SizeName = fyneTheme.SizeNameCaptionText
	return surface(container.New(layout.NewCustomPaddedVBoxLayout(8),
		previewTitle,
		keyValueRow(controller.translator.T("format.sampleLabel"), formatted),
		keyValueRow(controller.translator.T("format.todayLabel"), when),
	), fyne.NewSize(1, 92))
}

func fmtInt(value int) string {
	if value == 0 {
		return "0"
	}
	if value == 4 {
		return "4"
	}
	return "2"
}
