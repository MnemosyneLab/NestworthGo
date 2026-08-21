package ui

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

const uiDateLayout = "2006-01-02"

func newDateEntry(placeholder string) *widget.DateEntry {
	entry := widget.NewDateEntry()
	entry.PlaceHolder = placeholder
	return entry
}

func setDateEntryISO(entry *widget.DateEntry, value string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return
	}
	parsed, err := time.Parse(uiDateLayout, trimmed)
	if err != nil {
		entry.SetText(trimmed)
		return
	}
	entry.SetDate(&parsed)
}

func dateEntryValue(entry *widget.DateEntry) string {
	if entry.Date != nil {
		return entry.Date.Format(uiDateLayout)
	}
	return strings.TrimSpace(entry.Text)
}

// DateEntry validates the display format chosen by Fyne's system locale. A
// blank optional date should still be accepted by the account form, so wrap
// the widget in a non-validating container and let the domain layer validate
// non-empty text on submit.
func dateFormField(entry *widget.DateEntry) fyne.CanvasObject {
	return container.NewMax(entry)
}
