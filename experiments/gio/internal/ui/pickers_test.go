package ui

import (
	"image"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
)

func TestTemporalValueRules(t *testing.T) {
	options := DateOptions{Today: "2024-02-28", Min: "2024-02-10"}
	if options.permits("2024-02-29") || options.permits("2024-02-09") || !options.permits("2024-02-10") {
		t.Fatal("wrong implicit upper or explicit lower limit")
	}
	options.Max = "2024-03-03"
	if !options.permits("2024-02-29") || !options.permits("2024-03-03") || options.permits("2024-03-04") ||
		options.permits("2023-02-29") || options.permits("2024-2-29") || options.permits("0000-01-01") {
		t.Fatal("failed civil day validation or explicit maximum")
	}
	options.Max, options.AllowFuture = "", true
	if !options.permits("2025-03-04") {
		t.Fatal("AllowFuture did not lift the default upper limit")
	}
	february, _ := ParseCivilDay("2024-02-01")
	if leadingDays(february, time.Monday) != 3 || leadingDays(february, time.Sunday) != 4 {
		t.Fatal("weekday offset did not respect week start")
	}
	leap, _ := ParseCivilDay("2024-02-29")
	for _, sample := range []struct {
		language   i18n.Language
		day, month string
	}{
		{i18n.English, "Feb 29, 2024", "February 2024"},
		{i18n.SimplifiedChinese, "2024年2月29日", "2024年2月"},
		{i18n.TraditionalChinese, "2024年2月29日", "2024年2月"},
		{i18n.Japanese, "2024年2月29日", "2024年2月"},
	} {
		if got := displayCivil(leap, sample.language); got != sample.day {
			t.Errorf("%s displayed leap day as %q, want %q", sample.language, got, sample.day)
		}
		if got := leap.Format(sample.language.Text(i18n.DateMonthLayout)); got != sample.month {
			t.Errorf("%s calendar heading was %q, want %q", sample.language, got, sample.month)
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
		{i18n.Japanese, "12:05", "午後12:05"},
		{i18n.SimplifiedChinese, "00:00", "上午12:00"},
		{i18n.SimplifiedChinese, "23:59", "下午11:59"},
		{i18n.TraditionalChinese, "00:00", "上午12:00"},
		{i18n.TraditionalChinese, "23:59", "下午11:59"},
	} {
		if got := displayClock(View{Language: sample.language}, sample.value, true); got != sample.want {
			t.Errorf("%s displayed %s as %q, want %q", sample.language, sample.value, got, sample.want)
		}
	}
}

func TestPopoverPlacementPrefersBelowWhenItFits(t *testing.T) {
	safe := image.Rect(8, 8, 692, 452)
	anchor := image.Rect(636, 32, 670, 64)
	size := image.Pt(300, 96)
	got := placePopover(safe, anchor, size, 6, 382, 18)
	if want := image.Rect(392, 70, 692, 166); got != want {
		t.Fatalf("got panel %v, want %v", got, want)
	}
}
