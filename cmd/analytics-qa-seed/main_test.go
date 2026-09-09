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

func TestParseLoanScenarioFamily(t *testing.T) {
	cases := []struct {
		name, currency, tz, week string
		ok                       bool
	}{
		{"loan-fc07", "AUD", "Asia/Singapore", "monday", true},
		{"loan-fc07-utc", "AUD", "UTC", "monday", true},
		{"loan-fc07-cny", "CNY", "Asia/Singapore", "monday", true},
		{"loan-fc07-usd", "USD", "Asia/Singapore", "monday", true},
		{"loan-fc07-week-sunday", "AUD", "Asia/Singapore", "sunday", true},
		{"complete", "", "", "", false},
		{"loan-fc07-mystery", "", "", "", false},
	}
	for _, tc := range cases {
		cfg, ok := parseLoanScenario(tc.name)
		if ok != tc.ok {
			t.Fatalf("%s ok=%v want %v", tc.name, ok, tc.ok)
		}
		if !ok {
			continue
		}
		if cfg.Currency != tc.currency || cfg.Timezone != tc.tz || cfg.WeekStart != tc.week {
			t.Fatalf("%s cfg=%+v want currency=%s tz=%s week=%s", tc.name, cfg, tc.currency, tc.tz, tc.week)
		}
		if cfg.WindowW != 1280 {
			t.Fatalf("%s default window: got %d", tc.name, cfg.WindowW)
		}
	}
}

func TestLoanFC07ScenarioConstants(t *testing.T) {
	if loanFC07Scenario != "loan-fc07" {
		t.Fatalf("loanFC07Scenario = %q, want loan-fc07", loanFC07Scenario)
	}
	if loanFixtureVersion != "analytics-linux-qa-loan-fc07" {
		t.Fatalf("loanFixtureVersion = %q, want analytics-linux-qa-loan-fc07", loanFixtureVersion)
	}
	if loanDrawDay >= loanRepayDay || loanRepayDay >= loanInterestDay || loanInterestDay >= loanQuoteHorizon {
		t.Fatalf("loan days must sit inside the rebuild window: draw=%d repay=%d interest=%d horizon=%d", loanDrawDay, loanRepayDay, loanInterestDay, loanQuoteHorizon)
	}
	if loanDrawPrincipal != "100000" || loanRepayPrincipal != "10000" || loanInterestFee != "500" {
		t.Fatalf("FC-07 oracles drifted: draw=%s repay=%s interest=%s", loanDrawPrincipal, loanRepayPrincipal, loanInterestFee)
	}
	if loanInterestPrincipal != "1000" {
		t.Fatalf("interest-day principal must match Case 17 (API rejects 0): got %s", loanInterestPrincipal)
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
