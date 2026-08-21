package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestDateEntryUsesISOValueForDomain(t *testing.T) {
	application := test.NewTempApp(t)
	defer application.Quit()

	entry := newDateEntry("Select a date")
	setDateEntryISO(entry, "2026-08-21")

	if got := dateEntryValue(entry); got != "2026-08-21" {
		t.Fatalf("date entry value = %q, want 2026-08-21", got)
	}
}

func TestDateFormFieldDoesNotMakeOptionalDateRequired(t *testing.T) {
	entry := newDateEntry("Select a date")
	field := dateFormField(entry)
	if _, validatable := field.(fyne.Validatable); validatable {
		t.Fatalf("date form field = %T, should not expose DateEntry validation to the form", field)
	}
}
