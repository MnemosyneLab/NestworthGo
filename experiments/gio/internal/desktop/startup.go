package desktop

import (
	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/preferences"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

// ShowStartupBlocked uses only the system language and appearance. The
// rejected profile can be locked by another process or unfit to read.
func ShowStartupBlocked(reason i18n.MessageID) error {
	window := new(app.Window)
	window.Option(app.Title(pilot.DisplayName), app.Size(520, 250), app.MinSize(430, 230))
	prefs := sessionPreferences{language: i18n.Language(preferences.SystemLanguage()), dark: preferences.SystemDark()}
	colors := prefs.colors()
	theme := ui.NewTheme()
	theme.Palette = material.Palette{Bg: colors.PageBackground, Fg: colors.Ink, ContrastBg: colors.Accent, ContrastFg: colors.OnAccent}
	view := ui.View{Language: prefs.language, Colors: colors, Theme: theme}
	var ops op.Ops
	var closeButton widget.Clickable
	firstFrame := true
	for {
		switch event := window.Event().(type) {
		case app.DestroyEvent:
			return event.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, event)
			gtx.Locale.Language = string(view.Language)
			if closeButton.Clicked(gtx) {
				window.Perform(system.ActionClose)
			}
			if firstFrame {
				gtx.Execute(key.FocusCmd{Tag: &closeButton})
				firstFrame = false
			}
			paint.Fill(gtx.Ops, colors.PageBackground)
			layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(24).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(ui.LimitWidth(gtx, 440),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return view.Label(gtx, view.Language.Text(i18n.StartupBlocked), 24, font.Bold, colors.Ink)
						}),
						layout.Rigid(layout.Spacer{Height: 10}.Layout),
						layout.Rigid(view.Message(view.Language.Text(reason), colors.SecondaryInk)),
						layout.Rigid(layout.Spacer{Height: 20}.Layout),
						layout.Rigid(view.PrimaryButton(&closeButton, view.Language.Text(i18n.StartupQuit))),
					)
				})
			})
			event.Frame(gtx.Ops)
		}
	}
}
