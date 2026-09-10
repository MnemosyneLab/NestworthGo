package domain

import (
	"testing"
	"time"
)

func TestLastFinalizedUSEquityMarketDateBeforeSingaporeThursdayOpen(t *testing.T) {
	sgt, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 10, 0, 5, 0, 0, sgt)
	got, err := LastFinalizedUSEquityMarketDate(now)
	if err != nil {
		t.Fatal(err)
	}
	if got != "2026-09-08" {
		t.Fatalf("LastFinalizedUSEquityMarketDate(%s) = %q, want 2026-09-08 (Tue US close; Wed close has not elapsed)", now.Format(time.RFC3339), got)
	}
}

func TestLastFinalizedUSEquityMarketDateAfterWednesdayUSClose(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 9, 16, 5, 0, 0, ny)
	got, err := LastFinalizedUSEquityMarketDate(now)
	if err != nil {
		t.Fatal(err)
	}
	if got != "2026-09-09" {
		t.Fatalf("LastFinalizedUSEquityMarketDate(%s) = %q, want 2026-09-09", now.Format(time.RFC3339), got)
	}
}
