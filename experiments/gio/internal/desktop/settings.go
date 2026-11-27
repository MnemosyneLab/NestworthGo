package desktop

import (
	"fmt"
	"image"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/preferences"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type settingsPage struct {
	prefs                       *preferenceState
	active                      settingsSection
	tabs                        [len(settingsTabs)]widget.Clickable
	fields                      [8]ui.Select
	fxProvider, quoteTTL, log   ui.Select
	credential                  settingsCredential
	data                        settingsDataRebuild
	copyLogPath                 widget.Clickable
	logPathCopied               bool
	swatches                    [5]widget.Clickable
	currency                    ui.TextField
	save, discard, reset, retry widget.Clickable
	confirmation                ui.Confirmation
	confirming, returnFocus     bool
	edits                       map[i18n.MessageID]string
	notice                      i18n.MessageID
	zones                       []ui.Choice
	choiceLanguage              i18n.Language
	choices                     map[i18n.MessageID][]ui.Choice
}

type settingsSection int

const (
	settingsAppearance settingsSection = iota
	settingsMarket
	settingsData
	settingsDiagnostics
	settingsAbout
)

var settingsTabs = [...]i18n.MessageID{i18n.SettingsTab, i18n.SettingsMarketTab, i18n.SettingsDataTab, i18n.SettingsDiagnosticsTab, i18n.SettingsAboutTab}

func newSettingsPage(prefs *preferenceState) *settingsPage {
	p := &settingsPage{
		prefs:    prefs,
		currency: ui.NewTextField(true), edits: make(map[i18n.MessageID]string),
		credential: settingsCredential{input: ui.NewTextField(true)},
	}
	p.zones = []ui.Choice{{ID: settings.TimezoneSystem}}
	for _, zone := range preferences.Timezones() {
		p.zones = append(p.zones, ui.Choice{ID: zone, Label: zone})
	}
	return p
}

// Apply only fields the user has edited. Quick-control changes in the sidebar
// remain visible in the form and survive a subsequent save of other fields.
func (p *settingsPage) draft() preferences.Values {
	v := p.prefs.saved
	for field, value := range p.edits {
		setPreferenceValue(&v, field, value)
	}
	return v
}

func setPreferenceValue(v *preferences.Values, field i18n.MessageID, value string) {
	switch field {
	case i18n.SettingsAppearance:
		v.Appearance = settings.Appearance(value)
	case i18n.SettingsAccent:
		v.Accent = settings.Accent(value)
	case i18n.SettingsLanguage:
		v.Language = settings.Language(value)
	case i18n.SettingsTimezone:
		v.Timezone = value
	case i18n.SettingsDate:
		v.DateFormat = value
	case i18n.SettingsTime:
		v.TimeFormat = value
	case i18n.SettingsDecimal:
		v.DecimalSeparator = value
	case i18n.SettingsGrouping:
		v.GroupingSeparator = value
	case i18n.SettingsFXProvider:
		v.FXProvider = value
	case i18n.SettingsQuoteCacheTTL:
		v.QuoteCacheTTL = value
	case i18n.SettingsLogLevel:
		v.LogLevel = value
	}
}

func (p *settingsPage) edit(field i18n.MessageID, value string) {
	v := p.prefs.saved
	setPreferenceValue(&v, field, value)
	if p.prefs.saved == v {
		delete(p.edits, field)
	} else {
		p.edits[field] = value
	}
	p.notice = i18n.NoMessage
}

func (p *settingsPage) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	p.data.update()
	if !p.prefs.ready {
		if p.prefs.err == nil {
			return view.Message(view.Language.Text(i18n.SettingsLoading), view.Colors.SecondaryInk)(gtx)
		}
		if p.retry.Clicked(gtx) {
			p.prefs.reload()
		}
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
			layout.Rigid(view.Message(view.Language.Text(i18n.SettingsLoadError), view.Colors.ErrorInk)),
			layout.Rigid(view.Button(&p.retry, view.Language.Text(i18n.SettingsLoadRetry))))
	}
	if p.returnFocus && !p.confirming {
		p.returnFocus = false
		gtx.Execute(key.FocusCmd{Tag: &p.reset})
	}
	p.updateTabs(gtx)
	return p.content(gtx, view, p.draft())
}

func (p *settingsPage) content(gtx layout.Context, view ui.View, draft preferences.Values) layout.Dimensions {
	viewport := image.Rect(0, view.Origin.Y, gtx.Constraints.Max.X, view.Origin.Y+gtx.Constraints.Min.Y)
	gtx.Constraints.Min.Y = 0
	width := min(gtx.Constraints.Max.X, gtx.Dp(896))
	intro := view.Message(view.Language.Text(i18n.SettingsIntro), view.Colors.SecondaryInk)(gtx)
	y := intro.Size.Y + gtx.Dp(18)
	paint.FillShape(gtx.Ops, view.Colors.Divider, clip.Rect(image.Rect(0, y, gtx.Constraints.Max.X, y+1)).Op())
	y += 1 + gtx.Dp(20)
	tab := ui.Place(gtx, view, image.Pt(0, y), width, func(gtx layout.Context, view ui.View) layout.Dimensions {
		gtx.Constraints.Min.X = 0
		return p.tabStrip(gtx, view)
	})
	y += tab.Size.Y + gtx.Dp(12)
	dims := ui.Place(gtx, view, image.Pt(0, y), width, func(gtx layout.Context, view ui.View) layout.Dimensions {
		switch p.active {
		case settingsMarket:
			return p.marketCard(gtx, view, draft, viewport)
		case settingsData:
			return p.data.card(gtx, view, p.prefs)
		case settingsDiagnostics:
			return p.diagnosticsCard(gtx, view, draft)
		case settingsAbout:
			return settingsAboutCard(gtx, view)
		default:
			return p.generalCard(gtx, view, draft)
		}
	})
	y += dims.Size.Y + gtx.Dp(20)
	footer := ui.Place(gtx, view, image.Pt(0, y), width, func(gtx layout.Context, view ui.View) layout.Dimensions {
		return p.footer(gtx, view, draft)
	})
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, y+footer.Size.Y)}
}

func (p *settingsPage) updateTabs(gtx layout.Context) {
	active := p.active
	for i := range p.tabs {
		button := &p.tabs[i]
		if button.Clicked(gtx) {
			active = settingsSection(i)
		}
		for _, press := range ui.PressedKeys(gtx, button, key.NameLeftArrow, key.NameRightArrow, key.NameHome, key.NameEnd) {
			switch press.Name {
			case key.NameLeftArrow:
				active = settingsSection((i + len(p.tabs) - 1) % len(p.tabs))
			case key.NameRightArrow:
				active = settingsSection((i + 1) % len(p.tabs))
			case key.NameHome:
				active = settingsAppearance
			case key.NameEnd:
				active = settingsAbout
			}
			gtx.Execute(key.FocusCmd{Tag: &p.tabs[active]})
		}
	}
	if active != p.active {
		p.credential.clearInput()
		p.logPathCopied = false
		p.active = active
		gtx.Execute(op.InvalidateCmd{})
	}
}

func (p *settingsPage) tabStrip(gtx layout.Context, view ui.View) layout.Dimensions {
	return view.TabTrack(gtx, len(p.tabs), func(gtx layout.Context, i int) layout.Dimensions {
		return view.TabSegment(&p.tabs[i], view.Language.Text(settingsTabs[i]), p.active == settingsSection(i))(gtx)
	})
}

type settingField struct {
	label   i18n.MessageID
	value   string
	options []ui.Choice
	help    string
}

// Settings container thresholds. They are widths of the card or footer, not of
// the window, so they stay local; the window itself uses ui.Breakpoint*.
const (
	settingsGap            unit.Dp = 18
	settingsMinColumn      unit.Dp = 215 // two columns once the card holds 448dp
	settingsSwatchRowWidth unit.Dp = 305 // below this the accent swatches wrap under the select
	settingsFooterRowWidth unit.Dp = 510 // below this the status wraps above the actions
)

func (p *settingsPage) generalCard(gtx layout.Context, view ui.View, draft preferences.Values) layout.Dimensions {
	fields := p.formFields(view, draft)
	grid := ui.Grid{MinColumn: settingsMinColumn, Gap: settingsGap, MaxColumns: 2}
	return settingsCard(gtx, view, view.Language.Text(i18n.SettingsGeneral), func(gtx layout.Context, view ui.View) layout.Dimensions {
		return grid.Layout(gtx, view, len(fields)+1, func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
			if i == len(fields) {
				currency := draft.HouseholdCurrency
				if currency == "" {
					currency = "—"
				}
				if p.currency.Text() != currency {
					p.currency.Editor.SetText(currency)
				}
				return p.currency.Layout(gtx, view, ui.FieldOptions{Label: i18n.SettingsCurrency, ReadOnly: true, Help: i18n.SettingsCurrencyHelp})
			}
			field := fields[i]
			separator := field.label == i18n.SettingsDecimal || field.label == i18n.SettingsGrouping
			return p.field(gtx, view, i, field, separator && draft.DecimalSeparator == draft.GroupingSeparator)
		})
	})
}

func (p *settingsPage) field(gtx layout.Context, view ui.View, index int, field settingField, invalid bool) layout.Dimensions {
	width := gtx.Constraints.Max.X
	previous, next := event.Tag(nil), event.Tag(&p.currency.Editor)
	if index > 0 {
		previous = &p.fields[index-1].Trigger
	}
	if index+1 < len(p.fields) {
		next = &p.fields[index+1].Trigger
	}
	picker := ui.PickerField{Label: field.label, Origin: view.Origin, Previous: previous, Next: next, Disabled: p.prefs.busy}
	hasSwatches := field.label == i18n.SettingsAccent
	stackSwatches := hasSwatches && width < gtx.Dp(settingsSwatchRowWidth)
	selectWidth := width
	if hasSwatches && !stackSwatches {
		selectWidth -= gtx.Dp(145)
	}
	input := gtx
	input.Constraints.Min.X, input.Constraints.Max.X = selectWidth, selectWidth
	options := ui.SelectOptions{Items: field.options, Searchable: field.label == i18n.SettingsTimezone, Invalid: invalid}
	p.fields[index].Layout(input, view, picker, options, field.value, func(value string) { p.edit(field.label, value) })
	y := gtx.Dp(60)
	if hasSwatches {
		offset := image.Pt(selectWidth+gtx.Dp(5), gtx.Dp(25))
		if stackSwatches {
			offset = image.Pt(0, y+gtx.Dp(4))
			y += gtx.Dp(34)
		}
		ui.Place(gtx, view, offset, width-offset.X, func(gtx layout.Context, view ui.View) layout.Dimensions { return p.palette(gtx, view, field) })
	}
	if field.help != "" {
		d := ui.Place(gtx, view, image.Pt(0, y+gtx.Dp(4)), width, func(gtx layout.Context, view ui.View) layout.Dimensions {
			color := view.Colors.SecondaryInk
			if field.label == i18n.SettingsGrouping {
				color = view.Colors.ErrorInk
			}
			return view.Body(ui.TextCaption, field.help, color)(gtx)
		})
		y += gtx.Dp(4) + d.Size.Y
	}
	return layout.Dimensions{Size: image.Pt(width, y)}
}

func (p *settingsPage) palette(gtx layout.Context, view ui.View, field settingField) layout.Dimensions {
	if p.prefs.busy {
		gtx = gtx.Disabled()
	}
	step := gtx.Dp(ui.SwatchSize)
	for i, opt := range field.options {
		click := &p.swatches[i]
		if click.Clicked(gtx) && !p.prefs.busy {
			p.edit(field.label, opt.ID)
			gtx.Execute(op.InvalidateCmd{})
		}
		offset := op.Offset(image.Pt(i*step, 0)).Push(gtx.Ops)
		view.Swatch(gtx, click, ui.SwatchSpec{Fill: accentThemes[i].swatch, Name: opt.Label, Selected: opt.ID == field.value})
		offset.Pop()
	}
	return layout.Dimensions{Size: image.Pt(step*len(field.options), step)}
}

type settingOption struct {
	ID    string
	label i18n.MessageID
}

var generalOptions = map[i18n.MessageID][]settingOption{
	i18n.SettingsAppearance: {{string(settings.AppearanceSystem), i18n.SettingsAppearanceSystem}, {string(settings.AppearanceLight), i18n.SettingsAppearanceLight}, {string(settings.AppearanceDark), i18n.SettingsAppearanceDark}},
	i18n.SettingsAccent:     {{string(settings.AccentIndigo), i18n.IndigoAccent}, {string(settings.AccentLavender), i18n.LavenderAccent}, {string(settings.AccentMint), i18n.MintAccent}, {string(settings.AccentSky), i18n.SkyAccent}, {string(settings.AccentPeach), i18n.PeachAccent}},
	i18n.SettingsLanguage:   {{string(settings.LanguageSystem), i18n.SettingsLanguageSystem}, {string(settings.LanguageEnglish), i18n.SettingsLanguageEn}, {string(settings.LanguageZhCN), i18n.SettingsLanguageZhCN}, {string(settings.LanguageZhTW), i18n.SettingsLanguageZhTW}, {string(settings.LanguageJa), i18n.SettingsLanguageJa}},
	i18n.SettingsDate:       {{settings.DateFormatISO, i18n.SettingsDateISO}, {settings.DateFormatDayFirst, i18n.SettingsDateDayFirst}, {settings.DateFormatMonthFirst, i18n.SettingsDateMonthFirst}, {settings.DateFormatLocalized, i18n.SettingsDateLocalized}},
	i18n.SettingsTime:       {{settings.TimeFormat24, i18n.SettingsTime24}, {settings.TimeFormat12, i18n.SettingsTime12}},
	i18n.SettingsDecimal:    {{settings.DecimalDot, i18n.SettingsDot}, {settings.DecimalComma, i18n.SettingsComma}},
	i18n.SettingsGrouping:   {{settings.GroupingComma, i18n.SettingsComma}, {settings.GroupingDot, i18n.SettingsDot}, {settings.GroupingSpace, i18n.SettingsSpace}, {settings.GroupingApost, i18n.SettingsApostrophe}, {settings.GroupingNone, i18n.SettingsNoGrouping}},
}

func (p *settingsPage) localizeChoices(lang i18n.Language) {
	if lang == p.choiceLanguage {
		return
	}
	p.choices = make(map[i18n.MessageID][]ui.Choice, len(generalOptions))
	for field, options := range generalOptions {
		items := make([]ui.Choice, 0, len(options))
		for _, opt := range options {
			items = append(items, ui.Choice{ID: opt.ID, Label: lang.Text(opt.label)})
		}
		p.choices[field] = items
	}
	p.zones[0].Label = lang.Text(i18n.SettingsTimezoneSystem)
	p.choiceLanguage = lang
}

func (p *settingsPage) formFields(view ui.View, v preferences.Values) []settingField {
	l := view.Language
	p.localizeChoices(l)
	zones := p.zones
	// Older persisted files may use an IANA alias outside the canonical list.
	if v.Timezone != settings.TimezoneSystem && !p.hasZone(v.Timezone) {
		zones = append([]ui.Choice{{ID: v.Timezone, Label: v.Timezone}}, zones...)
	}
	zone := v.Timezone
	if zone == settings.TimezoneSystem {
		zone = p.prefs.systemTimezone
	}
	zoneHelp := fmt.Sprintf(l.Text(i18n.SettingsTimezoneResolved), zone)
	if v.HistoryTimezone != "" && v.HistoryTimezone != zone {
		zoneHelp += "\n" + fmt.Sprintf(l.Text(i18n.SettingsTimezoneDiff), v.HistoryTimezone, zone)
	}
	var separatorError string
	if v.DecimalSeparator == v.GroupingSeparator {
		separatorError = l.Text(i18n.SettingsSeparatorConflict)
	}
	return []settingField{
		{i18n.SettingsAppearance, string(v.Appearance), p.choices[i18n.SettingsAppearance], ""},
		{i18n.SettingsAccent, string(v.Accent), p.choices[i18n.SettingsAccent], ""},
		{i18n.SettingsLanguage, string(v.Language), p.choices[i18n.SettingsLanguage], ""},
		{i18n.SettingsTimezone, v.Timezone, zones, zoneHelp},
		{i18n.SettingsDate, v.DateFormat, p.choices[i18n.SettingsDate], ""},
		{i18n.SettingsTime, v.TimeFormat, p.choices[i18n.SettingsTime], ""},
		{i18n.SettingsDecimal, v.DecimalSeparator, p.choices[i18n.SettingsDecimal], l.Text(i18n.SettingsNumberHelp)},
		{i18n.SettingsGrouping, v.GroupingSeparator, p.choices[i18n.SettingsGrouping], separatorError},
	}
}

func (p *settingsPage) hasZone(zone string) bool {
	for _, opt := range p.zones {
		if opt.ID == zone {
			return true
		}
	}
	return false
}

func (p *settingsPage) footer(gtx layout.Context, view ui.View, draft preferences.Values) layout.Dimensions {
	dirty := draft != p.prefs.saved
	canSave := dirty && draft.DecimalSeparator != draft.GroupingSeparator && !p.prefs.busy
	if canSave && p.save.Clicked(gtx) {
		p.notice = i18n.NoMessage
		p.prefs.save(p.prefs.saved, draft, func() { clear(p.edits); p.notice = i18n.SettingsSaved })
	}
	if !p.prefs.busy && p.discard.Clicked(gtx) {
		clear(p.edits)
		p.prefs.err = nil
		gtx.Execute(op.InvalidateCmd{})
		p.notice = i18n.SettingsDiscarded
	}
	if !p.prefs.busy && p.reset.Clicked(gtx) {
		p.confirming = true
		p.prefs.err = nil
		p.confirmation.Open()
		p.notice = i18n.NoMessage
		gtx.Execute(op.InvalidateCmd{})
	}
	return view.Panel(gtx, view.Colors.Divider, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(16).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			status := i18n.SettingsSaveHelp
			if dirty {
				status = i18n.SettingsUnsaved
			}
			if p.notice != i18n.NoMessage {
				status = p.notice
			}
			ink := view.Colors.SecondaryInk
			if p.prefs.err != nil {
				status = i18n.SettingsSaveError
				ink = view.Colors.ErrorInk
			}
			message := view.Body(ui.TextCaption, view.Language.Text(status), ink)
			actions := func(gtx layout.Context) layout.Dimensions { return p.actions(gtx, view, canSave, dirty) }
			reset := func(gtx layout.Context) layout.Dimensions {
				if p.prefs.busy {
					gtx = gtx.Disabled()
				}
				return view.ButtonOf(&p.reset, ui.ButtonSpec{Variant: ui.ButtonGhost, Title: view.Language.Text(i18n.SettingsReset)})(gtx)
			}
			statusAndActions := func(gtx layout.Context) layout.Dimensions {
				if gtx.Constraints.Max.X < gtx.Dp(settingsFooterRowWidth) {
					return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(10)}.Layout(gtx, layout.Rigid(message), layout.Rigid(actions))
				}
				return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(12)}.Layout(gtx, layout.Flexed(1, message), layout.Rigid(actions))
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(12)}.Layout(gtx,
				layout.Rigid(reset),
				layout.Rigid(statusAndActions),
			)
		})
	})
}

func (p *settingsPage) actions(gtx layout.Context, view ui.View, canSave, dirty bool) layout.Dimensions {
	return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !canSave {
				gtx = gtx.Disabled()
			}
			label := i18n.SettingsSave
			if p.prefs.busy {
				label = i18n.SettingsPending
			}
			return view.PrimaryButton(&p.save, view.Language.Text(label))(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !dirty {
				return layout.Dimensions{}
			}
			if p.prefs.busy {
				gtx = gtx.Disabled()
			}
			return view.Button(&p.discard, view.Language.Text(i18n.SettingsDiscard))(gtx)
		}),
	)
}

// layoutConfirmation is called by the window after drawing its disabled content.
func (p *settingsPage) layoutConfirmation(gtx layout.Context, view ui.View) {
	label := i18n.SettingsReset
	if p.prefs.busy {
		label = i18n.SettingsPending
	}
	options := ui.ConfirmationOptions{
		Title:   view.Language.Text(i18n.SettingsResetTitle),
		Body:    view.Language.Text(i18n.SettingsResetBody),
		Cancel:  view.Language.Text(i18n.SettingsCancel),
		Confirm: view.Language.Text(label),
		Busy:    p.prefs.busy,
	}
	if p.prefs.err != nil {
		options.Problem = view.Language.Text(i18n.SettingsSaveError)
	}
	p.confirmation.Layout(gtx, view, options, func() {
		p.confirming = false
		p.returnFocus = true
		gtx.Execute(op.InvalidateCmd{})
	}, func() {
		p.prefs.reset(func() {
			clear(p.edits)
			p.notice = i18n.SettingsRestored
			p.confirming = false
			p.returnFocus = true
		})
		gtx.Execute(op.InvalidateCmd{})
	})
}
