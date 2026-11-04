package domain

import (
	"testing"
	"time"
)

func TestMarketReadCivilWindow(t *testing.T) {
	ptr := func(s string) *string { return &s }
	for _, r := range []MarketReadRequest{
		{FromDate: ptr("2026-03-08")}, {ToDate: ptr("2026-03-08")},
		{FromDate: ptr(""), ToDate: ptr("")},
		{FromDate: ptr("2026-02-29"), ToDate: ptr("2026-03-01")},
		{FromDate: ptr("2026-03-09"), ToDate: ptr("2026-03-08")},
		{FromDate: ptr(" 2026-03-08"), ToDate: ptr("2026-03-08")},
		{Range: "all", FromDate: ptr("2026-03-08"), ToDate: ptr("2026-03-08")},
	} {
		if _, err := r.TrendRange(); err == nil {
			t.Fatalf("accepted invalid dates: %+v", r)
		}
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		date  string
		hours time.Duration
	}{{"2026-03-08", 23}, {"2026-11-01", 25}, {"2024-02-29", 24}} {
		r, err := (MarketReadRequest{FromDate: ptr(tc.date), ToDate: ptr(tc.date)}).TrendRange()
		if err != nil {
			t.Fatal(err)
		}
		w := NewQuoteReadWindow(r, QuoteSourceFilter("all"), time.Now(), loc, "", "")
		if w.FromDate != tc.date || w.ToDate != tc.date || w.To.Sub(*w.From)+time.Nanosecond != tc.hours*time.Hour {
			t.Fatalf("civil/DST bounds: %+v", w)
		}
	}
	// A missing origin uses UTC, including the end of the requested day.
	r, _ := ParseTrendRange("2026-03-08:2026-03-08")
	w := NewQuoteReadWindow(r, QuoteSourceFilter("all"), time.Now(), nil, "", "")
	if w.From.Location() != time.UTC || w.To.Sub(*w.From)+time.Nanosecond != 24*time.Hour {
		t.Fatal(w)
	}
}
