package desktop

import (
	"image"
	"testing"
	"time"

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
	if p.accountID != "" || p.account.list.highlight != "cash" {
		t.Fatalf("moving through options changed the value or missed cash: %q, %q", p.accountID, p.account.list.highlight)
	}
	h.press(key.NameDownArrow)
	if p.account.list.highlight != "brokerage" {
		t.Fatal("keyboard highlight did not skip the disabled option")
	}
	h.click("Credit card", false)
	if p.accountID != "" || h.shell.overlay.trigger == nil {
		t.Fatal("a disabled row committed or closed the select")
	}
	h.press(key.NameReturn)
	if p.accountID != "brokerage" || h.preview.submissions != 0 {
		t.Fatalf("unexpected committed value or form submission: %q, %d", p.accountID, h.preview.submissions)
	}
	h.focused(&p.account.trigger)

	h.press(key.NameReturn)
	h.press(key.NameEscape)
	h.focused(&p.account.trigger)
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
	h.focused(&p.institution.search.field.editor)
	if got := h.shell.overlay.panel; got.Empty() || !got.In(image.Rectangle{Max: h.size}) {
		t.Fatalf("the long-list overlay does not fit: %v", got)
	}
	h.router.Queue(key.EditEvent{Text: "West"})
	h.frame()
	h.press(key.NameReturn)
	if p.institutionID != "" || h.preview.submissions != 0 || h.shell.overlay.trigger == nil {
		t.Fatal("Return in the search editor committed an option or submitted the form")
	}
	h.router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press, Modifiers: key.ModAlt})
	h.frame()
	h.focused(&p.institution)
	h.router.Queue(key.Event{Name: key.NameUpArrow, State: key.Press, Modifiers: key.ModAlt})
	h.frame()
	h.focused(&p.institution.search.field.editor)
	if p.institution.query != "West" {
		t.Fatal("moving back to the editor lost the current search")
	}
	h.router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press, Modifiers: key.ModAlt})
	h.frame()
	h.press(key.NameReturn)
	if p.institutionID != "westlake" {
		t.Fatalf("keyboard selection committed %q", p.institutionID)
	}
	h.focused(&p.institution.trigger)

	h.press(key.NameReturn)
	if p.institution.query != "" || p.institution.search.field.text() != "" {
		t.Fatal("reopening did not start a new search")
	}
	h.router.Queue(key.EditEvent{Text: "no such institution"})
	h.frame()
	if p.institution.list.active {
		t.Fatal("a search without matches left a committable highlight")
	}
	h.router.Queue(input.SystemEvent{Event: key.Event{Name: key.NameTab, State: key.Press}})
	h.frame()
	h.focused(&p.date.trigger)
	if p.institutionID != "westlake" || h.shell.overlay.trigger != nil || p.institution.query != "" {
		t.Fatal("Tab changed the committed value or left the overlay open")
	}

	h.click("Choose institution", true)
	h.router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press, Modifiers: key.ModAlt})
	h.frame()
	for range 12 {
		h.press(key.NameDownArrow)
	}
	if p.institution.list.scroll.Position.First == 0 || p.institutionID != "westlake" {
		t.Fatal("moving through a long list did not scroll or mutated the committed value")
	}
	h.press(key.NameEscape)
	h.focused(&p.institution.trigger)
}

func TestPreviewCalendarAndClock(t *testing.T) {
	h := newPreviewHarness(t)
	p := &h.preview.pickers
	h.click("Effective date", true)
	h.focused(&p.date)
	h.click("Feb 9, 2024", true)
	if p.day != "" || h.shell.overlay.trigger == nil {
		t.Fatal("a day before the lower bound was committed")
	}
	h.press(key.NameRightArrow)
	if p.day != "" || p.date.day.Format(civilLayout) != "2024-02-29" {
		t.Fatal("moving to leap day incorrectly committed or skipped it")
	}
	h.press(key.NameReturn)
	if p.day != "2024-02-29" {
		t.Fatalf("expected leap day, got %q", p.day)
	}
	h.focused(&p.date.trigger)
	h.click("Monday first", false)
	if !p.sundayFirst || p.day != "2024-02-29" {
		t.Fatal("changing the start of week changed the date")
	}
	h.click("Effective date", true)
	h.router.Queue(key.Event{Name: key.NamePageDown, State: key.Press, Modifiers: key.ModShift})
	h.frame()
	h.press(key.NamePageDown)
	if p.date.month.Format("2006-01") != "2025-03" || p.day != "2024-02-29" {
		t.Fatal("navigating month and year overwrote the committed date")
	}
	h.click("Mar 4, 2025", true)
	if p.day != "2024-02-29" || h.shell.overlay.trigger == nil {
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
	h.click("繁體中文", false)
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

func TestTemporalValueRules(t *testing.T) {
	options := dateOptions{today: "2024-02-28", min: "2024-02-10"}
	if options.permits("2024-02-29") || options.permits("2024-02-09") || !options.permits("2024-02-10") {
		t.Fatal("wrong implicit upper or explicit lower limit")
	}
	options.max = "2024-03-03"
	if !options.permits("2024-02-29") || !options.permits("2024-03-03") || options.permits("2024-03-04") ||
		options.permits("2023-02-29") || options.permits("2024-2-29") || options.permits("0000-01-01") {
		t.Fatal("failed civil day validation or explicit maximum")
	}
	options.max, options.allowFuture = "", true
	if !options.permits("2025-03-04") {
		t.Fatal("allowFuture did not lift the default upper limit")
	}
	february, _ := parseCivilDay("2024-02-01")
	if leadingDays(february, time.Monday) != 3 || leadingDays(february, time.Sunday) != 4 {
		t.Fatal("weekday offset did not respect week start")
	}
	leap, _ := parseCivilDay("2024-02-29")
	for language, want := range map[i18n.Language]string{
		i18n.English: "Feb 29, 2024", i18n.SimplifiedChinese: "2024年2月29日", i18n.TraditionalChinese: "2024年2月29日",
	} {
		if got := displayCivil(leap, language); got != want {
			t.Errorf("%s displayed leap day as %q, want %q", language, got, want)
		}
	}
	if chooseClockPart("", true, 23) != "00:23" || chooseClockPart("", false, 7) != "07:00" ||
		chooseClockPart("23:59", false, 0) != "00:59" {
		t.Fatal("changing a time part overwrote its partner or missed the initial zero")
	}
	for _, value := range []string{"", "24:00", "03:60", "0:00", "00:0"} {
		if _, _, ok := clockParts(value); ok {
			t.Errorf("%q was parsed as a canonical time", value)
		}
	}
	for _, sample := range []struct {
		language    i18n.Language
		value, want string
	}{
		{i18n.English, "00:00", "12:00 AM"},
		{i18n.English, "12:05", "12:05 PM"},
		{i18n.SimplifiedChinese, "00:00", "上午12:00"},
		{i18n.SimplifiedChinese, "23:59", "下午11:59"},
		{i18n.TraditionalChinese, "00:00", "上午12:00"},
		{i18n.TraditionalChinese, "23:59", "下午11:59"},
	} {
		if got := displayClock(presentation{language: sample.language}, sample.value, true); got != sample.want {
			t.Errorf("%s displayed %s as %q, want %q", sample.language, sample.value, got, sample.want)
		}
	}
}
