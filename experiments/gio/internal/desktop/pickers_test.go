package desktop

import (
	"image"
	"testing"

	"gioui.org/io/input"
	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
)

func TestPreviewSelectSeparatesHighlightAndCommit(t *testing.T) {
	h := newPreviewHarness(t)
	p := &h.preview.pickers
	h.click("Account type", true)
	h.focused(&p.account)
	h.press(key.NameDownArrow)
	if p.accountID != "" {
		t.Fatalf("moving through options committed %q", p.accountID)
	}
	h.press(key.NameReturn)
	if p.accountID != "cash" {
		t.Fatalf("first enabled account choice committed %q", p.accountID)
	}
	h.focused(&p.account.Trigger)
	h.press(key.NameReturn)
	h.press(key.NameDownArrow)
	h.click("Credit card", false)
	if p.accountID != "cash" || h.shell.overlay.Trigger == nil {
		t.Fatal("a disabled row committed or closed the select")
	}
	h.press(key.NameReturn)
	if p.accountID != "brokerage" || h.preview.submissions != 0 {
		t.Fatalf("unexpected committed value or form submission: %q, %d", p.accountID, h.preview.submissions)
	}
	h.focused(&p.account.Trigger)

	h.press(key.NameReturn)
	h.press(key.NameEscape)
	h.focused(&p.account.Trigger)
	if p.accountID != "brokerage" {
		t.Fatal("cancel changed the committed value")
	}
	h.press(key.NameReturn)
	h.click("None", false)
	if p.accountID != "" {
		t.Fatal("the explicit empty row did not clear the value")
	}
}

func TestPreviewSearchResetsAndReturnsToTraversal(t *testing.T) {
	h := newPreviewHarness(t)
	p := &h.preview.pickers
	h.size = image.Pt(700, 460)
	h.frame()
	h.click("Choose institution", true)
	if got := h.shell.overlay.Panel; got.Empty() || !got.In(image.Rectangle{Max: h.size}) {
		t.Fatalf("the long-list overlay does not fit: %v", got)
	}
	h.router.Queue(key.EditEvent{Text: "West"})
	h.frame()
	semanticBounds(t, &h.router, "Westlake Savings", false)
	h.press(key.NameReturn)
	if p.institutionID != "" || h.preview.submissions != 0 || h.shell.overlay.Trigger == nil {
		t.Fatal("Return in the search editor committed an option or submitted the form")
	}
	h.router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press, Modifiers: key.ModAlt})
	h.frame()
	h.focused(&p.institution)
	h.router.Queue(key.Event{Name: key.NameUpArrow, State: key.Press, Modifiers: key.ModAlt})
	h.frame()
	h.router.Queue(key.SnippetEvent{End: 4})
	h.frame()
	if got := h.router.EditorState().Snippet.Text; got != "West" {
		t.Fatalf("moving back to the editor returned a search draft of %q", got)
	}
	h.router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press, Modifiers: key.ModAlt})
	h.frame()
	h.press(key.NameReturn)
	if p.institutionID != "westlake" {
		t.Fatalf("keyboard selection committed %q", p.institutionID)
	}
	h.focused(&p.institution.Trigger)

	h.press(key.NameReturn)
	semanticBounds(t, &h.router, "Harbor Branch 01", false)
	h.router.Queue(key.EditEvent{Text: "no such institution"})
	h.frame()
	assertPageLabel(t, &h.router, "No matches")
	h.router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press, Modifiers: key.ModAlt})
	h.frame()
	h.press(key.NameReturn)
	if p.institutionID != "westlake" || h.shell.overlay.Trigger == nil {
		t.Fatal("a search without matches committed or closed the popup")
	}
	h.router.Queue(input.SystemEvent{Event: key.Event{Name: key.NameTab, State: key.Press}})
	h.frame()
	h.focused(&p.date.Trigger)
	if p.institutionID != "westlake" || h.shell.overlay.Trigger != nil {
		t.Fatal("Tab changed the committed value or left the overlay open")
	}

	h.click("Choose institution", true)
	h.router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press, Modifiers: key.ModAlt})
	h.frame()
	for range 12 {
		h.press(key.NameDownArrow)
	}
	row := semanticBounds(t, &h.router, "Harbor Branch 12", false)
	if !row.Min.Add(row.Max).Div(2).In(h.shell.overlay.Panel) || p.institutionID != "westlake" {
		t.Fatal("moving through a long list did not scroll or mutated the committed value")
	}
	h.press(key.NameEscape)
	h.focused(&p.institution.Trigger)
}

func TestPreviewCalendarAndClock(t *testing.T) {
	h := newPreviewHarness(t)
	p := &h.preview.pickers
	h.click("Effective date", true)
	h.focused(&p.date)
	h.click("Feb 9, 2024", true)
	if p.day != "" || h.shell.overlay.Trigger == nil {
		t.Fatal("a day before the lower bound was committed")
	}
	h.press(key.NameRightArrow)
	if p.day != "" {
		t.Fatal("moving to leap day committed it before Return")
	}
	h.press(key.NameReturn)
	if p.day != "2024-02-29" {
		t.Fatalf("expected leap day, got %q", p.day)
	}
	h.focused(&p.date.Trigger)
	h.click("Monday first", false)
	if !p.sundayFirst || p.day != "2024-02-29" {
		t.Fatal("changing the start of week changed the date")
	}
	h.click("Effective date", true)
	h.router.Queue(key.Event{Name: key.NamePageDown, State: key.Press, Modifiers: key.ModShift})
	h.frame()
	h.press(key.NamePageDown)
	assertPageLabel(t, &h.router, "March 2025")
	if p.day != "2024-02-29" {
		t.Fatal("navigating month and year overwrote the committed date")
	}
	h.click("Mar 4, 2025", true)
	if p.day != "2024-02-29" || h.shell.overlay.Trigger == nil {
		t.Fatal("the calendar accepted a date after the upper bound")
	}
	h.press(key.NameEscape)

	h.click("Effective time", true)
	h.focused(&p.clock)
	h.press(key.NameReturn)
	if p.timeValue != "00:00" {
		t.Fatalf("midnight was not preserved: %q", p.timeValue)
	}
	h.press(key.NameRightArrow)
	h.press(key.NameDownArrow)
	h.press(key.NameReturn)
	if p.timeValue != "00:01" {
		t.Fatalf("choosing minutes changed the hour: %q", p.timeValue)
	}
	h.press(key.NameEscape)
	h.click("24-hour time", false)
	h.selectSidebarLanguage(i18n.SettingsLanguageZhTW)
	if p.timeValue != "00:01" || p.day != "2024-02-29" || !p.twelveHour {
		t.Fatal("changing language or time display rewrote canonical picker values")
	}
	h.click("生效時間", true)
	h.click("清除選擇", false)
	if p.timeValue != "" {
		t.Fatal("time clear did not reset the value")
	}
	h.click("生效日期", true)
	h.click("清除選擇", false)
	if p.day != "" || h.preview.submissions != 0 {
		t.Fatal("date clear did not reset the value or changed the submit count")
	}
}
