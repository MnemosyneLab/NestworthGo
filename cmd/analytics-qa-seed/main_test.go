package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestFixtureVersionIsV3(t *testing.T) {
	if fixtureVersion != "analytics-linux-qa-v3" {
		t.Fatalf("fixtureVersion = %q, want analytics-linux-qa-v3", fixtureVersion)
	}
}

func TestParseAnchorUTC(t *testing.T) {
	got := parseAnchor(defaultQAAnchor)
	want := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("parseAnchor(%s) = %s, want %s", defaultQAAnchor, got, want)
	}
}

func TestPathWithin(t *testing.T) {
	base := filepath.Join(t.TempDir(), "qa")
	inside := filepath.Join(base, "data", "nestworth.db")
	if !pathWithin(base, inside) {
		t.Fatalf("pathWithin(%s, %s) = false, want true", base, inside)
	}
	if pathWithin(base, filepath.Join(base, "..", "other.db")) {
		t.Fatal("pathWithin allowed a path outside the QA output dir")
	}
}

func TestTrueZeroScheduleIsFlat(t *testing.T) {
	if aaplPrice(35) != aaplPrice(36) || qqqPrice(35) != qqqPrice(36) || es3Price(35) != es3Price(36) {
		t.Fatalf("instrument prices must be flat on days 35/36: aapl=%v/%v qqq=%v/%v es3=%v/%v",
			aaplPrice(35), aaplPrice(36), qqqPrice(35), qqqPrice(36), es3Price(35), es3Price(36))
	}
	if usdAudRate(35) != usdAudRate(36) || sgdAudRate(35) != sgdAudRate(36) {
		t.Fatalf("FX must be flat on days 35/36: usd=%v/%v sgd=%v/%v",
			usdAudRate(35), usdAudRate(36), sgdAudRate(35), sgdAudRate(36))
	}
}

func TestExpectedGapQuoteCount(t *testing.T) {
	if expectedQuoteDays != 45 {
		t.Fatalf("expectedQuoteDays = %d, want 45", expectedQuoteDays)
	}
	if expectedGapQuotes != 22 {
		t.Fatalf("expectedGapQuotes = %d, want 22 (days 23..44)", expectedGapQuotes)
	}
}
