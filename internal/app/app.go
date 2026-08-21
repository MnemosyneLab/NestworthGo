package app

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"github.com/waltwang/nestworth-go/internal/ui"
)

// App owns the desktop application lifecycle and its main window.
type App struct {
	fyneApp fyne.App
	window  fyne.Window
}

// New creates the Nestworth application with a stable application ID.
func New() *App {
	fyneApp := app.NewWithID("com.nestworth.app")

	window := fyneApp.NewWindow("Nestworth")
	window.Resize(fyne.NewSize(1100, 720))
	window.SetContent(ui.NewDashboard())

	return &App{
		fyneApp: fyneApp,
		window:  window,
	}
}

// Run starts the application event loop.
func (a *App) Run() {
	a.window.ShowAndRun()
}
