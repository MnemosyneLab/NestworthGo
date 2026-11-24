package desktop

import (
	"errors"
	"image"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func generalSettingsHarness(t *testing.T) (*previewHarness, *settingsPage) {
	t.Helper()
	prefs := newTestPreferenceState(t)
	prefs.saved.Language = settings.LanguageEnglish
	prefs.saved.HouseholdCurrency = "USD"
	prefs.saved.HistoryTimezone = "Asia/Singapore"
	prefs.store.(*testPreferenceStore).value = prefs.saved
	h := newShellHarnessWithPages(t, ledgerPages{prefs: prefs})
	h.size = image.Pt(1120, 960)
	h.frame()
	h.click("Settings", false)
	return h, h.shell.settingsPage
}

func TestGeneralSettingsDraftSaveDiscardAndSeparators(t *testing.T) {
	h, p := generalSettingsHarness(t)
	assertPageLabel(t, &h.router, "Appearance & language")
	h.click("Decimal separator", true)
	h.press(key.NameDownArrow)
	h.press(key.NameReturn)
	assertPageLabel(t, &h.router, "Decimal and grouping separators must differ")
	var bounds image.Rectangle
	var inspect func(input.SemanticNode)
	inspect = func(node input.SemanticNode) {
		if node.Desc.Label == "Save changes" {
			bounds = node.Desc.Bounds
		}
		for _, child := range node.Children {
			inspect(child)
		}
	}
	for _, root := range h.router.AppendSemantics(nil) {
		inspect(root)
	}
	if bounds.Empty() {
		t.Fatal("disabled save label missing")
	}
	h.clickAt(bounds.Min.Add(bounds.Max).Div(2))
	if p.prefs.busy || p.prefs.saved.DecimalSeparator != "." {
		t.Fatal("saved a conflicting separator pair")
	}
	h.click("Grouping separator", true)
	h.press(key.NameDownArrow)
	h.press(key.NameReturn)
	if p.prefs.saved.DecimalSeparator != "." || p.draft().GroupingSeparator != "." {
		t.Fatal("editing the draft changed the saved preferences")
	}
	// A sidebar write should survive saving a different set of fields.
	p.prefs.saved.Accent = settings.AccentSky
	h.frame()
	h.click("Save changes", false)
	p.prefs.wait()
	h.frame()
	if p.prefs.saved.DecimalSeparator != "," || p.prefs.saved.GroupingSeparator != "." || p.prefs.saved.Accent != settings.AccentSky || len(p.edits) != 0 {
		t.Fatalf("bad saved settings: %+v, edits %v", p.prefs.saved, p.edits)
	}
	assertPageLabel(t, &h.router, "Settings saved")
	h.click("Date format", true)
	h.press(key.NameDownArrow)
	h.press(key.NameReturn)
	h.click("Discard changes", false)
	if p.draft().DateFormat != settings.DateFormatISO || len(p.edits) != 0 {
		t.Fatal("discard did not return to the saved date format")
	}
}

func TestGeneralSettingsTimezoneAndConfirmedReset(t *testing.T) {
	h, p := generalSettingsHarness(t)
	h.click("Timezone", true)
	h.router.Queue(key.EditEvent{Text: "not-a-timezone"})
	h.frame()
	assertPageLabel(t, &h.router, "No matches")
	h.press(key.NameEscape)
	h.click("Timezone", true)
	h.router.Queue(key.EditEvent{Text: "Tokyo"})
	h.frame()
	h.router.Queue(key.Event{Name: key.NameDownArrow, Modifiers: key.ModAlt, State: key.Press})
	h.frame()
	h.press(key.NameReturn)
	if p.draft().Timezone != "Asia/Tokyo" {
		t.Fatalf("timezone search selected %q", p.draft().Timezone)
	}
	assertPageLabel(t, &h.router, "Display timezone: Asia/Tokyo\nHistory was started in Asia/Singapore. Past ledger dates stay there. Ordinary times are now shown in Asia/Tokyo.")
	overview := semanticBounds(t, &h.router, "Overview", false)
	h.click("Restore display defaults", false)
	h.frame()
	h.focused(&p.confirmation.Cancel)
	h.clickAt(overview.Min.Add(overview.Max).Div(2))
	if !p.confirming || h.shell.activeRoute != "settings" {
		t.Fatal("reset confirmation allowed navigation behind it")
	}
	h.press(key.NameEscape)
	h.frame()
	if p.confirming || p.draft().Timezone != "Asia/Tokyo" {
		t.Fatal("cancelling reset changed the pending draft")
	}
	h.focused(&p.reset)
	h.click("Restore display defaults", false)
	h.frame()
	h.press(key.NameTab)
	h.focused(&p.confirmation.Confirm)
	h.press(key.NameReturn)
	p.prefs.wait()
	h.frame()
	if p.confirming || len(p.edits) != 0 || p.prefs.saved.Timezone != settings.TimezoneSystem {
		t.Fatal("confirmed reset did not restore defaults")
	}
}

func TestGeneralSettingsSaveErrorKeepsDraftForRetry(t *testing.T) {
	h, p := generalSettingsHarness(t)
	store := p.prefs.store.(*testPreferenceStore)
	store.err = errors.New("test write failed")
	h.click("Date format", true)
	h.press(key.NameDownArrow)
	h.press(key.NameReturn)
	h.click("Save changes", false)
	p.prefs.wait()
	h.frame()
	assertPageLabel(t, &h.router, "Settings could not be saved.")
	if p.draft().DateFormat != settings.DateFormatDayFirst || p.prefs.saved.DateFormat != settings.DateFormatISO {
		t.Fatal("failed save changed the draft or authoritative snapshot")
	}
	store.err = nil
	h.click("Save changes", false)
	p.prefs.wait()
	h.frame()
	if p.prefs.saved.DateFormat != settings.DateFormatDayFirst || len(p.edits) != 0 {
		t.Fatal("retry did not apply the retained draft")
	}
}

// A field picker inside a scrolled, one-column card still anchors to its
// visible trigger. Choosing the currently saved value releases the draft field.
func TestGeneralSettingsNarrowScrolledPicker(t *testing.T) {
	h, p := generalSettingsHarness(t)
	h.size = image.Pt(515, 490)
	h.frame()
	p.scroll.Position.Offset = 320
	h.frame()
	b := semanticBounds(t, &h.router, "Timezone", true)
	h.clickAt(b.Min.Add(b.Max).Div(2))
	if h.shell.overlay.Trigger == nil || h.shell.overlay.Anchor != b {
		t.Fatalf("accounted scroll bounds %v, want visible trigger %v", h.shell.overlay.Anchor, b)
	}
	h.press(key.NameEscape)
	p.edit(i18n.SettingsTimezone, "Asia/Tokyo")
	p.edit(i18n.SettingsTimezone, settings.TimezoneSystem)
	p.prefs.saved.Timezone = "UTC"
	if p.draft().Timezone != "UTC" {
		t.Fatal("reverted field overrode a newer saved preference")
	}
	// The action card must remain reachable at the end of the compact form.
	h.router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(350, 300), Scroll: f32.Pt(0, 3000)})
	h.frame()
	b = semanticBounds(t, &h.router, "Restore display defaults", false)
	if b.Min.Y < 0 || b.Max.Y > h.size.Y {
		t.Fatalf("footer bounds %v outside compact viewport %v", b, h.size)
	}
	h.clickAt(b.Min.Add(b.Max).Div(2))
	if !p.confirming {
		t.Fatal("compact card footer could not open its reset confirmation")
	}
}
