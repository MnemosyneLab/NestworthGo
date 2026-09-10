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
	if usdCnyRate(35) != usdCnyRate(36) {
		t.Fatalf("USD/CNY must be flat on days 35/36: %v/%v", usdCnyRate(35), usdCnyRate(36))
	}
	for _, base := range []string{"AUD", "USD", "CNY"} {
		for _, p := range fxPairsForBase(base) {
			if p.Rate(35) != p.Rate(36) {
				t.Fatalf("%s pair %s/%s not flat on 35/36: %v/%v", base, p.Base, p.Quote, p.Rate(35), p.Rate(36))
			}
		}
	}
}

func TestParseV3ScenarioBaseCurrency(t *testing.T) {
	t.Setenv("NESTWORTH_QA_BASE_CURRENCY", "")
	cases := []struct {
		name, family, base string
		ok                 bool
	}{
		{"complete", "complete", "AUD", true},
		{"complete-usd", "complete", "USD", true},
		{"complete-cny", "complete", "CNY", true},
		{"missing-price", "missing-price", "AUD", true},
		{"missing-fx", "missing-fx", "AUD", true},
		{"missing-both", "missing-both", "AUD", true},
		{"loan-fc07", "", "", false},
	}
	for _, tc := range cases {
		cfg, ok := parseV3Scenario(tc.name)
		if ok != tc.ok {
			t.Fatalf("%s ok=%v want %v", tc.name, ok, tc.ok)
		}
		if !ok {
			continue
		}
		if cfg.Family != tc.family || cfg.Base != tc.base {
			t.Fatalf("%s family=%s base=%s want family=%s base=%s", tc.name, cfg.Family, cfg.Base, tc.family, tc.base)
		}
	}
}

func TestParseV3ScenarioEnvOverridesComplete(t *testing.T) {
	t.Setenv("NESTWORTH_QA_BASE_CURRENCY", "USD")
	cfg, ok := parseV3Scenario("complete")
	if !ok || cfg.Family != "complete" || cfg.Base != "USD" {
		t.Fatalf("complete + env USD: %+v ok=%v", cfg, ok)
	}
}

func TestParseV3ScenarioEnvRejectedOnMissing(t *testing.T) {
	t.Setenv("NESTWORTH_QA_BASE_CURRENCY", "USD")
	defer func() {
		if recover() == nil {
			t.Fatal("missing-price + USD base must panic")
		}
	}()
	parseV3Scenario("missing-price")
}

func TestFXPairsForUSDAndCNYQuoteAgainstHouseholdBase(t *testing.T) {
	usd := fxPairsForBase("USD")
	if len(usd) != 2 || usd[0].Quote != "USD" || usd[1].Quote != "USD" {
		t.Fatalf("USD pairs must quote USD, got %+v", usd)
	}
	cny := fxPairsForBase("CNY")
	if len(cny) != 3 {
		t.Fatalf("CNY needs USD/AUD/SGD vs CNY, got %d", len(cny))
	}
	for _, p := range cny {
		if p.Quote != "CNY" {
			t.Fatalf("CNY pair quote=%s want CNY", p.Quote)
		}
	}
	aud := fxPairsForBase("AUD")
	if len(aud) != 2 || aud[0].Quote != "AUD" || aud[1].Quote != "AUD" {
		t.Fatalf("AUD pairs must stay USD/AUD and SGD/AUD, got %+v", aud)
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
