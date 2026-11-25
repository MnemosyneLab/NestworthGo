package desktop

import (
	"fmt"
	"image"
	"io"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/preferences"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/version"
)

var marketProviders = []settingOption{{settings.FXProviderFrankfurter, i18n.SettingsFrankfurter}}
var quoteCacheDurations = []settingOption{
	{settings.QuoteCacheTTL1h, i18n.SettingsQuoteCache1h},
	{settings.QuoteCacheTTL3h, i18n.SettingsQuoteCache3h},
	{settings.QuoteCacheTTL12h, i18n.SettingsQuoteCache12h},
	{settings.QuoteCacheTTL24h, i18n.SettingsQuoteCache24h},
}
var diagnosticsLevels = []settingOption{
	{ID: "off", label: i18n.SettingsLogOff}, {ID: "error", label: i18n.SettingsLogError}, {ID: "warn", label: i18n.SettingsLogWarn},
	{ID: "info", label: i18n.SettingsLogInfo}, {ID: "debug", label: i18n.SettingsLogDebug},
}

func (p *settingsPage) marketCard(gtx layout.Context, view ui.View, draft preferences.Values) layout.Dimensions {
	l := view.Language
	grid := ui.Grid{MinColumn: settingsMinColumn, Gap: settingsGap, MaxColumns: 2}
	return settingsCard(gtx, view, l.Text(i18n.SettingsMarket), func(gtx layout.Context, view ui.View) layout.Dimensions {
		width := gtx.Constraints.Max.X
		fields := grid.Layout(gtx, view, 2, func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
			if i == 0 {
				field := ui.PickerField{Label: i18n.SettingsFXProvider, Previous: &p.tabs[settingsMarket], Next: &p.quoteTTL.Trigger}
				return p.sectionPicker(gtx, view, &p.fxProvider, field, draft.FXProvider, marketProviders, i18n.SettingsFXProviderHelp)
			}
			field := ui.PickerField{Label: i18n.SettingsQuoteCacheTTL, Previous: &p.fxProvider.Trigger, Next: p.credential.nextFocus(draft.CoinGeckoConfigured)}
			return p.sectionPicker(gtx, view, &p.quoteTTL, field, draft.QuoteCacheTTL, quoteCacheDurations, i18n.SettingsQuoteCacheHelp)
		})
		y := fields.Size.Y + gtx.Dp(18)
		help := ui.Place(gtx, view, image.Pt(0, y), width, func(gtx layout.Context, view ui.View) layout.Dimensions {
			return view.Body(ui.TextCaption, l.Text(i18n.SettingsCredentialsHelp), view.Colors.SecondaryInk)(gtx)
		})
		y += help.Size.Y + gtx.Dp(10)
		credential := ui.Place(gtx, view, image.Pt(0, y), width, func(gtx layout.Context, view ui.View) layout.Dimensions {
			return p.credential.layout(gtx, view, p.prefs)
		})
		return layout.Dimensions{Size: image.Pt(width, y+credential.Size.Y)}
	})
}

func (p *settingsPage) diagnosticsCard(gtx layout.Context, view ui.View, draft preferences.Values) layout.Dimensions {
	l := view.Language
	if draft.LogPath != "" && p.copyLogPath.Clicked(gtx) {
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(draft.LogPath))})
		p.logPathCopied = true
	}
	return settingsCard(gtx, view, l.Text(i18n.SettingsDiagnosticsTab), func(gtx layout.Context, view ui.View) layout.Dimensions {
		width := gtx.Constraints.Max.X
		field := ui.PickerField{Label: i18n.SettingsLogLevel, Previous: &p.tabs[settingsDiagnostics], Next: &p.copyLogPath}
		level := p.sectionPicker(ui.LimitWidth(gtx, 520), view, &p.log, field, draft.LogLevel, diagnosticsLevels, i18n.SettingsLogHelp)
		if draft.LogPath == "" {
			return level
		}
		y := level.Size.Y + gtx.Dp(16)
		path := ui.Place(gtx, view, image.Pt(0, y), width, func(gtx layout.Context, view ui.View) layout.Dimensions {
			label := i18n.SettingsLogCopyPath
			if p.logPathCopied {
				label = i18n.SettingsLogPathCopied
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(6)}.Layout(gtx,
				layout.Rigid(view.Body(ui.TextCaptionStrong, l.Text(i18n.SettingsLogPath), view.Colors.Ink)),
				layout.Rigid(view.Body(ui.TextCaption, draft.LogPath, view.Colors.SecondaryInk)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = 0
					return view.ButtonOf(&p.copyLogPath, ui.ButtonSpec{Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: l.Text(label)})(gtx)
				}),
			)
		})
		return layout.Dimensions{Size: image.Pt(width, y+path.Size.Y)}
	})
}

func settingsAboutCard(gtx layout.Context, view ui.View) layout.Dimensions {
	l := view.Language
	return settingsCard(gtx, view, pilot.DisplayName, func(gtx layout.Context, view ui.View) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(16)}.Layout(gtx,
			layout.Rigid(view.Message(l.Text(i18n.SettingsAboutDescription), view.Colors.SecondaryInk)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(4)}.Layout(gtx,
					layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.SettingsAboutVersion), version.Version, version.Build), view.Colors.SecondaryInk)),
					layout.Rigid(view.Message(l.Text(i18n.SettingsAboutLicense), view.Colors.SecondaryInk)),
					layout.Rigid(view.Message(pilot.AppID, view.Colors.SecondaryInk)),
				)
			}),
		)
	})
}

func settingsCard(gtx layout.Context, view ui.View, title string, body func(layout.Context, ui.View) layout.Dimensions) layout.Dimensions {
	return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
		pad := gtx.Dp(20)
		width := gtx.Constraints.Max.X
		heading := ui.Place(gtx, view, image.Pt(pad, pad), width-2*pad, func(gtx layout.Context, view ui.View) layout.Dimensions {
			return view.Line(gtx, ui.TextHeading, title, view.Colors.Ink)
		})
		y := pad + heading.Size.Y + gtx.Dp(12)
		content := ui.Place(gtx, view, image.Pt(pad, y), width-2*pad, body)
		return layout.Dimensions{Size: image.Pt(width, y+content.Size.Y+pad)}
	})
}

func (p *settingsPage) sectionPicker(gtx layout.Context, view ui.View, selectField *ui.Select, field ui.PickerField, value string, options []settingOption, help i18n.MessageID) layout.Dimensions {
	width := gtx.Constraints.Max.X
	field.Origin, field.Disabled = view.Origin, p.prefs.busy || !gtx.Enabled()
	choices := make([]ui.Choice, len(options))
	for i, option := range options {
		choices[i] = ui.Choice{ID: option.ID, Label: view.Language.Text(option.label)}
	}
	selectField.Layout(gtx, view, field, ui.SelectOptions{Items: choices}, value, func(value string) { p.edit(field.Label, value) })
	y := gtx.Dp(ui.PickerHeight + 6)
	caption := ui.Place(gtx, view, image.Pt(0, y), width, func(gtx layout.Context, view ui.View) layout.Dimensions {
		return view.Body(ui.TextCaption, view.Language.Text(help), view.Colors.SecondaryInk)(gtx)
	})
	return layout.Dimensions{Size: image.Pt(width, y+caption.Size.Y)}
}

type settingsCredential struct {
	input                         ui.TextField
	replace, remove, show, cancel widget.Clickable
	save                          widget.Clickable
	editing, visible, focusInput  bool
	notice                        i18n.MessageID
}

func (c *settingsCredential) clearInput() {
	c.input.Editor.SetText("")
	c.editing, c.visible, c.focusInput = false, false, false
}

func (c *settingsCredential) nextFocus(configured bool) event.Tag {
	if configured && !c.editing {
		return &c.replace
	}
	return &c.input.Editor
}

func (c *settingsCredential) write(prefs *preferenceState, value string) {
	c.notice = i18n.NoMessage
	removing := value == ""
	prefs.setCoinGeckoKey(value, func() {
		c.clearInput()
		c.notice = i18n.SettingsCoinGeckoSaved
		if removing {
			c.notice = i18n.NoMessage
		}
	}, func() { c.notice = i18n.SettingsCoinGeckoError })
}

func (c *settingsCredential) layout(gtx layout.Context, view ui.View, prefs *preferenceState) layout.Dimensions {
	l := view.Language
	configured := prefs.saved.CoinGeckoConfigured
	if prefs.busy {
		gtx = gtx.Disabled()
	}
	if c.replace.Clicked(gtx) {
		c.editing, c.focusInput = true, true
		c.notice = i18n.NoMessage
	}
	if c.cancel.Clicked(gtx) {
		c.clearInput()
		c.notice = i18n.NoMessage
		gtx.Execute(key.FocusCmd{Tag: &c.replace})
	}
	if configured && c.remove.Clicked(gtx) {
		c.write(prefs, "")
		gtx = gtx.Disabled()
	}
	return view.Surface(gtx, ui.SurfaceSpec{Fill: view.Colors.PageBackground}, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(14).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			status, ink := i18n.SettingsCoinGeckoMissing, view.Colors.SecondaryInk
			if configured {
				status, ink = i18n.SettingsCoinGeckoConfigured, view.Colors.Success
			}
			if c.notice != i18n.NoMessage {
				status = c.notice
				if c.notice == i18n.SettingsCoinGeckoError {
					ink = view.Colors.ErrorInk
				}
			}
			rows := []layout.FlexChild{
				layout.Rigid(view.Body(ui.TextLabel, l.Text(i18n.SettingsCoinGecko), view.Colors.Ink)),
				layout.Rigid(view.Body(ui.TextCaption, l.Text(status), ink)),
			}
			if configured && !c.editing {
				rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return (ui.Wrap{Gap: 8}).Layout(gtx, 2, func(gtx layout.Context, i int) layout.Dimensions {
						if i == 0 {
							return view.Button(&c.replace, l.Text(i18n.SettingsSecretReplace))(gtx)
						}
						return view.Button(&c.remove, l.Text(i18n.SettingsCoinGeckoRemove))(gtx)
					})
				}))
			} else {
				rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions { return c.editForm(gtx, view, prefs, configured) }))
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, rows...)
		})
	})
}

func (c *settingsCredential) editForm(gtx layout.Context, view ui.View, prefs *preferenceState, configured bool) layout.Dimensions {
	if c.show.Clicked(gtx) {
		c.visible = !c.visible
	}
	c.input.Editor.Mask = '•'
	visibility := i18n.SettingsSecretShow
	if c.visible {
		c.input.Editor.Mask = 0
		visibility = i18n.SettingsSecretHide
	}
	if c.focusInput && gtx.Enabled() {
		c.focusInput = false
		c.input.Focus(gtx)
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.input.Layout(ui.LimitWidth(gtx, 520), view, ui.FieldOptions{
				Label: i18n.SettingsCoinGeckoKey, Hint: i18n.SettingsCoinGeckoHint, Disabled: !gtx.Enabled(),
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := view.Language
			value := strings.TrimSpace(c.input.Text())
			if value != "" && c.save.Clicked(gtx) {
				c.write(prefs, value)
				gtx = gtx.Disabled()
			}
			buttons := []layout.Widget{
				func(gtx layout.Context) layout.Dimensions {
					if value == "" {
						gtx = gtx.Disabled()
					}
					return view.ButtonOf(&c.save, ui.ButtonSpec{Variant: ui.ButtonSoft, Title: l.Text(i18n.SettingsCoinGeckoSave)})(gtx)
				},
				view.Button(&c.show, l.Text(visibility)),
			}
			if configured {
				buttons = append(buttons, view.ButtonOf(&c.cancel, ui.ButtonSpec{Variant: ui.ButtonGhost, Title: l.Text(i18n.SettingsCancel)}))
			}
			return (ui.Wrap{Gap: 8}).Layout(gtx, len(buttons), func(gtx layout.Context, i int) layout.Dimensions { return buttons[i](gtx) })
		}),
	)
}
