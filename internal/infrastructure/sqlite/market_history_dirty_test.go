package sqlite

import (
	"testing"
	"time"
)

func TestInstrumentHistoryDirtyBoundsUseMarketDateNotUTCCalendar(t *testing.T) {
	// US 2026-09-08 16:00 ET is 2026-09-08T20:00Z. A later Pacific close can
	// land on the next UTC calendar day while remaining the same market date.
	effective := time.Date(2026, 9, 9, 4, 0, 0, 0, time.UTC)
	from, to := instrumentHistoryDirtyBounds([]InstrumentHistoryObservation{
		{MarketDate: "2026-09-08", ValueEffectiveAt: effective},
		{MarketDate: "2026-09-04", ValueEffectiveAt: time.Date(2026, 9, 4, 20, 0, 0, 0, time.UTC)},
	})
	if from != "2026-09-04" || to != "2026-09-08" {
		t.Fatalf("bounds from=%s to=%s, want market dates 2026-09-04..2026-09-08 not UTC %s", from, to, effective.Format("2006-01-02"))
	}
}

func TestExpandDirtyToUsesLastClosedHouseholdDay(t *testing.T) {
	singapore, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC)
	started := time.Date(2026, 9, 6, 0, 0, 0, 0, singapore).Format(time.RFC3339Nano)
	got := expandDirtyToLocalBound("2026-09-08", "Asia/Singapore", started, at)
	if got != "2026-09-09" {
		t.Fatalf("dirty_to = %s, want 2026-09-09 last closed Singapore day", got)
	}
}
