package desktop

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/gesture"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/preferences"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type settingsPage struct {
	prefs                                      *preferenceState
	scroll                                     widget.List
	fields                                     [8]ui.Select
	swatches                                   [5]gesture.Click
	currency                                   ui.TextField
	save, discard, reset, retry                widget.Clickable
	cancelReset, confirmReset                  widget.Clickable
	confirming, focusConfirmation, returnFocus bool
	edits                                      map[i18n.MessageID]string
	notice                                     i18n.MessageID
	zones                                      []ui.Choice
	choiceLanguage                             i18n.Language
	choices                                    map[i18n.MessageID][]ui.Choice
}

func newSettingsPage(prefs *preferenceState) *settingsPage {
	p := &settingsPage{
		prefs: prefs, scroll: widget.List{List: layout.List{Axis: layout.Vertical}},
		currency: ui.NewTextField(true), edits: make(map[i18n.MessageID]string),
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
		setGeneralValue(&v, field, value)
	}
	return v
}

func setGeneralValue(v *preferences.Values, field i18n.MessageID, value string) {
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
	}
}

func (p *settingsPage) edit(field i18n.MessageID, value string) {
	v := p.prefs.saved
	setGeneralValue(&v, field, value)
	if p.prefs.saved == v {
		delete(p.edits, field)
	} else {
		p.edits[field] = value
	}
	p.notice = i18n.NoMessage
}

func (p *settingsPage) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	if !p.prefs.ready {
		if p.prefs.err == nil {
			return view.Message(view.Language.Text(i18n.SettingsLoading), view.Colors.SecondaryInk)(gtx)
		}
		if p.retry.Clicked(gtx) {
			p.prefs.reload()
		}
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
			layout.Rigid(view.Message(view.Language.Text(i18n.SettingsLoadError), view.Colors.ErrorInk)),
			layout.Rigid(view.Button(&p.retry, view.Language.Text(i18n.InstitutionsRetry))))
	}
	if p.returnFocus && !p.confirming {
		p.returnFocus = false
		gtx.Execute(key.FocusCmd{Tag: &p.reset})
	}
	draft := p.draft()
	gtx = ui.LimitWidth(gtx, 896)
	dims := material.List(view.Theme, &p.scroll).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		return p.content(gtx, view, draft)
	})
	// The controls register their bounds in content coordinates; the list
	// finalizes its viewport offset only after laying out the content.
	viewport := image.Rectangle{Min: view.Origin, Max: view.Origin.Add(gtx.Constraints.Max)}
	offset := image.Pt(0, -p.scroll.Position.Offset)
	for i := range p.fields {
		view.Overlay.OffsetAnchor(&p.fields[i].Trigger, offset, viewport)
	}
	return dims
}

// placeSettings positions a child and keeps its popup anchor in the same
// coordinate system. The child's measured height includes wrapped translations.
func placeSettings(gtx layout.Context, view ui.View, offset image.Point, width int, child pageLayout) layout.Dimensions {
	defer op.Offset(offset).Push(gtx.Ops).Pop()
	gtx.Constraints.Min, gtx.Constraints.Max.X = image.Pt(width, 0), width
	view.Origin = view.Origin.Add(offset)
	return child(gtx, view)
}

func (p *settingsPage) content(gtx layout.Context, view ui.View, draft preferences.Values) layout.Dimensions {
	width := gtx.Constraints.Max.X
	y := view.Message(view.Language.Text(i18n.SettingsIntro), view.Colors.SecondaryInk)(gtx).Size.Y + gtx.Dp(12)
	tab := placeSettings(gtx, view, image.Pt(0, y), width, func(gtx layout.Context, view ui.View) layout.Dimensions {
		gtx.Constraints.Min.X = 0
		return view.Panel(gtx, view.Colors.Divider, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 14, Right: 14, Top: 7, Bottom: 7}.Layout(gtx, view.Message(view.Language.Text(i18n.SettingsTab), view.Colors.Ink))
		})
	})
	y += tab.Size.Y + gtx.Dp(14)
	dims := placeSettings(gtx, view, image.Pt(0, y), width, func(gtx layout.Context, view ui.View) layout.Dimensions {
		return view.Panel(gtx, view.Colors.Divider, func(gtx layout.Context) layout.Dimensions {
			return p.generalCard(gtx, view, draft)
		})
	})
	y += dims.Size.Y + gtx.Dp(14)
	footer := placeSettings(gtx, view, image.Pt(0, y), width, func(gtx layout.Context, view ui.View) layout.Dimensions {
		return p.footer(gtx, view, draft)
	})
	return layout.Dimensions{Size: image.Pt(width, y+footer.Size.Y)}
}

type settingField struct {
	label   i18n.MessageID
	value   string
	options []ui.Choice
	help    string
}

func (p *settingsPage) generalCard(gtx layout.Context, view ui.View, draft preferences.Values) layout.Dimensions {
	width := gtx.Constraints.Max.X
	pad, gap := gtx.Dp(20), gtx.Dp(18)
	y := pad
	title := placeSettings(gtx, view, image.Pt(pad, y), width-2*pad, func(gtx layout.Context, view ui.View) layout.Dimensions {
		return view.Label(gtx, view.Language.Text(i18n.SettingsGeneral), 16, font.SemiBold, view.Colors.Ink)
	})
	y += title.Size.Y + gap
	columns := 1
	if width-2*pad >= gtx.Dp(448) {
		columns = 2
	}
	cellWidth := (width - 2*pad - (columns-1)*gap) / columns
	fields := p.formFields(view, draft)
	for start := 0; start < len(fields)+1; start += columns {
		h := 0
		for col := 0; col < columns && start+col <= len(fields); col++ {
			i := start + col
			dims := placeSettings(gtx, view, image.Pt(pad+col*(cellWidth+gap), y), cellWidth, func(gtx layout.Context, view ui.View) layout.Dimensions {
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
			h = max(h, dims.Size.Y)
		}
		y += h + gap
	}
	return layout.Dimensions{Size: image.Pt(width, y-gap+pad)}
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
	stackSwatches := hasSwatches && width < gtx.Dp(305)
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
		placeSettings(gtx, view, offset, width-offset.X, func(gtx layout.Context, view ui.View) layout.Dimensions { return p.palette(gtx, view, field) })
	}
	if field.help != "" {
		d := placeSettings(gtx, view, image.Pt(0, y+gtx.Dp(4)), width, func(gtx layout.Context, view ui.View) layout.Dimensions {
			color := view.Colors.SecondaryInk
			if field.label == i18n.SettingsGrouping {
				color = view.Colors.ErrorInk
			}
			return view.Message(field.help, color)(gtx)
		})
		y += gtx.Dp(4) + d.Size.Y
	}
	return layout.Dimensions{Size: image.Pt(width, y)}
}

func (p *settingsPage) palette(gtx layout.Context, view ui.View, field settingField) layout.Dimensions {
	for i, opt := range field.options {
		click := &p.swatches[i]
		for {
			e, ok := click.Update(gtx.Source)
			if !ok {
				break
			}
			if e.Kind == gesture.KindClick && !p.prefs.busy {
				p.edit(field.label, opt.ID)
				gtx.Execute(op.InvalidateCmd{})
			}
		}
		diameter := gtx.Dp(25)
		offset := op.Offset(image.Pt(i*gtx.Dp(28), 0)).Push(gtx.Ops)
		b := image.Rect(0, 0, diameter, diameter)
		area := clip.Ellipse(b).Push(gtx.Ops)
		if !p.prefs.busy {
			pointer.CursorPointer.Add(gtx.Ops)
			click.Add(gtx.Ops)
		}
		if opt.ID == field.value {
			paint.Fill(gtx.Ops, view.Colors.Accent)
		}
		paint.FillShape(gtx.Ops, view.Colors.Surface, clip.Ellipse(b.Inset(gtx.Dp(1))).Op(gtx.Ops))
		paint.FillShape(gtx.Ops, accentThemes[i].swatch, clip.Ellipse(b.Inset(gtx.Dp(3))).Op(gtx.Ops))
		area.Pop()
		offset.Pop()
	}
	return layout.Dimensions{Size: image.Pt(gtx.Dp(140), gtx.Dp(25))}
}

type settingOption struct {
	id    string
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
			items = append(items, ui.Choice{ID: opt.id, Label: lang.Text(opt.label)})
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
		p.focusConfirmation = true
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
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(10)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = 0
					if p.prefs.busy {
						gtx = gtx.Disabled()
					}
					return view.Button(&p.reset, view.Language.Text(i18n.SettingsReset))(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					message := view.Message(view.Language.Text(status), ink)
					actions := func(gtx layout.Context) layout.Dimensions { return p.actions(gtx, view, canSave, dirty) }
					if gtx.Constraints.Max.X < gtx.Dp(510) {
						return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, layout.Rigid(message), layout.Rigid(actions))
					}
					return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(12)}.Layout(gtx, layout.Flexed(1, message), layout.Rigid(actions))
				}),
			)
		})
	})
}

func (p *settingsPage) actions(gtx layout.Context, view ui.View, canSave, dirty bool) layout.Dimensions {
	return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
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

func (p *settingsPage) cancelConfirmation(gtx layout.Context) {
	p.confirming = false
	p.returnFocus = true
	gtx.Execute(op.InvalidateCmd{})
}

// layoutConfirmation is called by the window after drawing its disabled content.
func (p *settingsPage) layoutConfirmation(gtx layout.Context, view ui.View) {
	paint.Fill(gtx.Ops, color.NRGBA{A: 0x68})
	if p.focusConfirmation {
		p.focusConfirmation = false
		gtx.Execute(key.FocusCmd{Tag: &p.cancelReset})
	}
	for _, tag := range []event.Tag{&p.cancelReset, &p.confirmReset} {
		for _, e := range ui.PressedKeys(gtx, tag, key.NameEscape, key.NameTab) {
			if e.Name == key.NameEscape && !p.prefs.busy {
				p.cancelConfirmation(gtx)
				return
			}
			if e.Name == key.NameTab {
				target := &p.cancelReset
				if tag == &p.cancelReset {
					target = &p.confirmReset
				}
				gtx.Execute(key.FocusCmd{Tag: target})
			}
		}
	}
	if !p.prefs.busy && p.cancelReset.Clicked(gtx) {
		p.cancelConfirmation(gtx)
		return
	}
	if !p.prefs.busy && p.confirmReset.Clicked(gtx) {
		p.prefs.reset(func() { clear(p.edits); p.notice = i18n.SettingsRestored; p.confirming = false; p.returnFocus = true })
	}
	layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx = ui.LimitWidth(gtx, 420)
		return view.Panel(gtx, view.Colors.Divider, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(22).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if p.prefs.busy {
					gtx = gtx.Disabled()
				}
				return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(14)}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return view.Label(gtx, view.Language.Text(i18n.SettingsResetTitle), 17, font.SemiBold, view.Colors.Ink)
					}),
					layout.Rigid(view.Message(view.Language.Text(i18n.SettingsResetBody), view.Colors.SecondaryInk)),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if p.prefs.err == nil {
							return layout.Dimensions{}
						}
						return view.Message(view.Language.Text(i18n.SettingsSaveError), view.Colors.ErrorInk)(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := i18n.SettingsReset
						if p.prefs.busy {
							label = i18n.SettingsPending
						}
						return layout.Flex{Gap: gtx.Dp(12)}.Layout(gtx,
							layout.Rigid(view.Button(&p.cancelReset, view.Language.Text(i18n.SettingsCancel))),
							layout.Rigid(view.PrimaryButton(&p.confirmReset, view.Language.Text(label))),
						)
					}),
				)
			})
		})
	})
}
