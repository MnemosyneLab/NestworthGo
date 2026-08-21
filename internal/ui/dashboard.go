package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// NewDashboard builds the initial application shell. Domain data and charts
// will be connected here after the storage and application layers are added.
func NewDashboard() fyne.CanvasObject {
	navigation := container.NewVBox(
		widget.NewLabelWithStyle("NESTWORTH", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		widget.NewButton("Dashboard", nil),
		widget.NewButton("Accounts", nil),
		widget.NewButton("Activities", nil),
		widget.NewButton("Analytics", nil),
		layout.NewSpacer(),
		widget.NewButton("Settings", nil),
	)

	welcome := widget.NewLabelWithStyle(
		"Your financial picture, in one place.",
		fyne.TextAlignLeading,
		fyne.TextStyle{Bold: true},
	)

	metrics := container.NewGridWithColumns(
		3,
		widget.NewCard("Net worth", "Current total", widget.NewLabel("—")),
		widget.NewCard("This month", "Net change", widget.NewLabel("—")),
		widget.NewCard("Accounts", "Tracked accounts", widget.NewLabel("—")),
	)

	content := container.NewVBox(
		welcome,
		widget.NewLabel("Dashboard is ready for the domain and persistence layers."),
		metrics,
		widget.NewSeparator(),
		widget.NewCard("Activity", "Recent transactions will appear here.", widget.NewLabel("No activities yet")),
	)

	return container.NewBorder(nil, nil, navigation, nil, container.NewPadded(content))
}
