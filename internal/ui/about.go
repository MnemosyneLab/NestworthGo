package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/waltwang/nestworth-go/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/version"
)

// NewMainMenu creates the desktop menu shared by the application window.
func NewMainMenu(application fyne.App, parent fyne.Window, icon fyne.Resource, translator *i18n.Translator) *fyne.MainMenu {
	if translator == nil {
		translator = i18n.New(settings.LanguageEnglish)
	}
	about := fyne.NewMenuItem(translator.T("menu.about"), func() {
		ShowAbout(application, parent, icon, translator)
	})

	return fyne.NewMainMenu(
		fyne.NewMenu(translator.T("menu.file"), about),
	)
}

// ShowAbout displays the application identity and release metadata.
func ShowAbout(application fyne.App, parent fyne.Window, icon fyne.Resource, translator *i18n.Translator) {
	if translator == nil {
		translator = i18n.New(settings.LanguageEnglish)
	}
	name := version.Name
	appVersion := version.Version
	build := version.Build

	metadata := application.Metadata()
	if metadata.Name != "" {
		name = metadata.Name
	}
	if metadata.Version != "" && metadata.Version != "0.0.1" {
		appVersion = "v" + metadata.Version
	}
	if metadata.Build > 0 && metadata.Build != 1 {
		build = fmt.Sprintf("%d", metadata.Build)
	}

	iconView := canvas.NewImageFromResource(icon)
	iconView.FillMode = canvas.ImageFillContain
	iconView.SetMinSize(fyne.NewSize(112, 112))

	title := widget.NewLabelWithStyle(name, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	description := widget.NewLabel(translator.T("about.description"))
	description.Wrapping = fyne.TextWrapWord
	versionLabel := widget.NewLabelWithStyle(
		fmt.Sprintf(translator.T("about.version"), appVersion, build),
		fyne.TextAlignCenter,
		fyne.TextStyle{},
	)
	license := widget.NewLabelWithStyle(translator.T("about.license"), fyne.TextAlignCenter, fyne.TextStyle{})

	content := container.NewVBox(
		container.NewCenter(iconView),
		title,
		description,
		layout.NewSpacer(),
		versionLabel,
		license,
	)

	scroll := container.NewVScroll(content)
	scroll.SetMinSize(fyne.NewSize(0, 280))
	about := dialog.NewCustom(translator.T("about.title"), translator.T("common.close"), container.NewPadded(scroll), parent)
	about.SetIcon(icon)
	about.Resize(fittedModalSize(parent, fyne.NewSize(560, 430), fyne.NewSize(420, 330)))
	about.Show()
}
