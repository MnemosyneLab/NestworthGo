package ui

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/waltwang/nestworth-go/internal/settings"
)

const uiDateLayout = "2006-01-02"

type dateEntry struct {
	*widget.Entry
	date   *time.Time
	layout string
}

func newDateEntry(placeholder string, preferences ...settings.Settings) *dateEntry {
	pref := settings.Default()
	if len(preferences) > 0 {
		pref = preferences[0]
	}
	entry := &dateEntry{Entry: widget.NewEntry(), layout: displayDateLayout(pref)}
	entry.PlaceHolder = placeholder
	entry.OnChanged = func(value string) {
		parsed, err := time.Parse(entry.layout, strings.TrimSpace(value))
		if err == nil {
			entry.date = &parsed
		} else {
			entry.date = nil
		}
	}
	return entry
}

func displayDateLayout(preference settings.Settings) string {
	switch preference.DateFormat {
	case settings.DateFormatDayFirst:
		return "02/01/2006"
	case settings.DateFormatMonthFirst:
		return "01/02/2006"
	case settings.DateFormatLocalized:
		return "02 Jan 2006"
	default:
		return uiDateLayout
	}
}

func datePlaceholder(preference settings.Settings) string {
	switch preference.DateFormat {
	case settings.DateFormatDayFirst:
		return "DD/MM/YYYY"
	case settings.DateFormatMonthFirst:
		return "MM/DD/YYYY"
	case settings.DateFormatLocalized:
		return "DD Mon YYYY"
	default:
		return "YYYY-MM-DD"
	}
}

func timePlaceholder(preference settings.Settings) string {
	if preference.TimeFormat == settings.TimeFormat12 {
		return "HH:MM AM/PM"
	}
	return "HH:MM"
}

func displayClock(value time.Time, preference settings.Settings) string {
	if preference.TimeFormat == settings.TimeFormat12 {
		return value.Format("03:04 PM")
	}
	return value.Format("15:04")
}

func normalizeDateInput(value string, preference settings.Settings) (string, error) {
	trimmed := strings.TrimSpace(value)
	if parsed, err := time.Parse(displayDateLayout(preference), trimmed); err == nil {
		return parsed.Format(uiDateLayout), nil
	}
	parsed, err := time.Parse(uiDateLayout, trimmed)
	if err != nil {
		return "", err
	}
	return parsed.Format(uiDateLayout), nil
}

func normalizeClockInput(value string, preference settings.Settings) (string, error) {
	trimmed := strings.TrimSpace(value)
	if preference.TimeFormat == settings.TimeFormat12 {
		parsed, err := time.Parse("03:04 PM", strings.ToUpper(trimmed))
		if err == nil {
			return parsed.Format("15:04"), nil
		}
		parsed, err = time.Parse("15:04", trimmed)
		if err != nil {
			return "", err
		}
		return parsed.Format("15:04"), nil
	}
	parsed, err := time.Parse("15:04", trimmed)
	if err != nil {
		return "", err
	}
	return parsed.Format("15:04"), nil
}

func (entry *dateEntry) SetDate(value *time.Time) {
	if value == nil {
		entry.date = nil
		entry.SetText("")
		return
	}
	copy := *value
	entry.date = &copy
	entry.SetText(copy.Format(entry.layout))
}

func setDateEntryISO(entry *dateEntry, value string) {
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

func dateEntryValue(entry *dateEntry) string {
	if entry.date != nil {
		return entry.date.Format(uiDateLayout)
	}
	trimmed := strings.TrimSpace(entry.Text)
	if trimmed == "" {
		return ""
	}
	if parsed, err := time.Parse(entry.layout, trimmed); err == nil {
		return parsed.Format(uiDateLayout)
	}
	return trimmed
}

// DateEntry validates the display format chosen by Fyne's system locale. A
// blank optional date should still be accepted by the account form, so wrap
// the widget in a non-validating container and let the domain layer validate
// non-empty text on submit.
func dateFormField(entry *dateEntry) fyne.CanvasObject {
	return container.NewMax(entry)
}
