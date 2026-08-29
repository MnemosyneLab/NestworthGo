package domain

import (
	"testing"
	"time"
)

func TestQuoteFreshnessRejectsExtremeProviderTimes(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	if got := QuoteFreshness(QuoteSourceProvider, false, now.Add(QuoteClockSkewTolerance), now, 12*time.Hour); got != FreshnessFresh {
		t.Fatalf("small tolerated skew freshness = %q, want fresh", got)
	}
	if got := QuoteFreshness(QuoteSourceProvider, false, now.Add(QuoteClockSkewTolerance+time.Nanosecond), now, 12*time.Hour); got != FreshnessUnavailable {
		t.Fatalf("future provider freshness = %q, want unavailable", got)
	}
	if got := QuoteFreshness(QuoteSourceProvider, false, ProviderObservationEarliest().Add(-time.Nanosecond), now, 12*time.Hour); got != FreshnessUnavailable {
		t.Fatalf("old provider freshness = %q, want unavailable", got)
	}
	if got := QuoteFreshness(QuoteSourceManual, false, time.Time{}, now, 12*time.Hour); got != FreshnessManual {
		t.Fatalf("manual freshness = %q, want manual", got)
	}
	if got := QuoteFreshness(QuoteSourceProvider, false, now.Add(-12*time.Hour), now, 12*time.Hour); got != FreshnessStale {
		t.Fatalf("ttl-aged provider freshness = %q, want stale", got)
	}
}
