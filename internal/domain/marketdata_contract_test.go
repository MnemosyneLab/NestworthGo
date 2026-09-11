package domain

import (
	"testing"
	"time"
)

func TestHouseholdDayCutoffUsesNextLocalMidnight(t *testing.T) {
	cutoff, err := HouseholdDayCutoff("2026-09-09", "Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	want, err := time.Parse(time.RFC3339Nano, "2026-09-09T15:59:59.999Z")
	if err != nil {
		t.Fatal(err)
	}
	if !cutoff.Equal(want) {
		t.Fatalf("cutoff = %s, want %s", cutoff.UTC().Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
}

func TestUSEquitySessionCloseFollowsNewYorkDST(t *testing.T) {
	before, err := ResolveEquitySessionClose("2026-03-06", "US", SessionEvidence{Kind: SessionKindRegular, Timezone: USEquitySessionTimezone, CloseClock: USEquityRegularCloseClock})
	if err != nil || before.Status != "mapped" {
		t.Fatalf("before DST: %#v err=%v", before, err)
	}
	after, err := ResolveEquitySessionClose("2026-03-09", "US", SessionEvidence{Kind: SessionKindRegular, Timezone: USEquitySessionTimezone, CloseClock: USEquityRegularCloseClock})
	if err != nil || after.Status != "mapped" {
		t.Fatalf("after DST: %#v err=%v", after, err)
	}
	if before.CloseInstant.UTC().Format(time.RFC3339) != "2026-03-06T21:00:00Z" {
		t.Fatalf("EST close = %s", before.CloseInstant.UTC())
	}
	if after.CloseInstant.UTC().Format(time.RFC3339) != "2026-03-09T20:00:00Z" {
		t.Fatalf("EDT close = %s", after.CloseInstant.UTC())
	}
	if ObservationEligible(after.CloseInstant, time.Date(2026, 3, 9, 15, 59, 59, 999000000, time.UTC)) {
		t.Fatal("Singapore Sep-equivalent cutoff consumed a later US close")
	}
}

func TestUnknownSessionTimingIsUncertain(t *testing.T) {
	resolved, err := ResolveEquitySessionClose("2026-11-27", "US", SessionEvidence{Kind: SessionKindUnknown})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != "uncertain" || resolved.Reason != "session_timing_unknown" {
		t.Fatalf("unknown session = %#v", resolved)
	}
	if !resolved.CloseInstant.IsZero() {
		t.Fatal("invented a close timestamp")
	}
}

func TestChineseMarketUsesShanghaiSession(t *testing.T) {
	resolved, err := ResolveEquitySessionClose("2026-09-04", "CN", SessionEvidence{Kind: SessionKindRegular})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != "mapped" || resolved.Policy != CNEquityRegularClosePolicy {
		t.Fatalf("CN market = %#v", resolved)
	}
	if resolved.CloseInstant.UTC().Format(time.RFC3339) != "2026-09-04T07:00:00Z" {
		t.Fatalf("CN close = %s, want Shanghai 15:00 close", resolved.CloseInstant.UTC())
	}
}

func TestUTCMidnightIsNotACloseInstant(t *testing.T) {
	resolved, err := ResolveEquitySessionClose("2026-09-04", "US", SessionEvidence{Kind: SessionKindRegular, Timezone: USEquitySessionTimezone, CloseClock: USEquityRegularCloseClock})
	if err != nil {
		t.Fatal(err)
	}
	midnight := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	if resolved.CloseInstant.Equal(midnight) {
		t.Fatal("session close collapsed to UTC midnight")
	}
}

func TestFXMappingIdentityReciprocalAndUnsupported(t *testing.T) {
	identity, err := MapFXRate("USD", "USD", "USD", "SGD", "1.35")
	if err != nil || identity.Status != FXMappingIdentity || identity.Rate.Canonical() != "1" || identity.Derived {
		t.Fatalf("identity = %#v err=%v", identity, err)
	}
	direct, err := MapFXRate("USD", "SGD", "USD", "SGD", "1.35")
	if err != nil || direct.Status != FXMappingDirect || direct.Derived {
		t.Fatalf("direct = %#v err=%v", direct, err)
	}
	reciprocal, err := MapFXRate("SGD", "USD", "USD", "SGD", "1.35")
	if err != nil || reciprocal.Status != FXMappingReciprocal || !reciprocal.Derived {
		t.Fatalf("reciprocal = %#v err=%v", reciprocal, err)
	}
	if reciprocal.Rate.Canonical() != "0.740740740741" {
		t.Fatalf("reciprocal rate = %s", reciprocal.Rate.Canonical())
	}
	unsupported, err := MapFXRate("USD", "XXX", "USD", "SGD", "1.35")
	if err != nil || unsupported.Status != FXMappingUnsupported {
		t.Fatalf("unsupported = %#v err=%v", unsupported, err)
	}
}

func TestHistoryCompletenessDoesNotTreatHTTPSuccessAsVerified(t *testing.T) {
	pending, err := ClassifyHistoryCompleteness("2026-09-09", "2026-09-09", "", []string{"2026-09-09"}, false, false)
	if err != nil || pending.Status != "pending" || len(pending.PendingRanges) != 1 {
		t.Fatalf("pending = %#v err=%v", pending, err)
	}
	truncated, err := ClassifyHistoryCompleteness("2026-09-04", "2026-09-09", "2026-09-08", nil, true, false)
	if err != nil || truncated.Status != "uncertain" || len(truncated.VerifiedRanges) != 0 {
		t.Fatalf("truncated = %#v err=%v", truncated, err)
	}
	malformed, err := ClassifyHistoryCompleteness("2026-09-04", "2026-09-04", "2026-09-04", nil, false, true)
	if err != nil || malformed.Status != "invalid" {
		t.Fatalf("malformed = %#v err=%v", malformed, err)
	}
	complete, err := ClassifyHistoryCompleteness("2026-09-04", "2026-09-09", "2026-09-08", []string{"2026-09-09"}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(complete.VerifiedRanges) != 1 || complete.VerifiedRanges[0] != (InclusiveDateRange{Start: "2026-09-04", End: "2026-09-08"}) {
		t.Fatalf("verified = %#v", complete.VerifiedRanges)
	}
	if len(complete.PendingRanges) != 1 || complete.PendingRanges[0] != (InclusiveDateRange{Start: "2026-09-09", End: "2026-09-09"}) {
		t.Fatalf("pending ranges = %#v", complete.PendingRanges)
	}
}

func TestOpeningAnchorLookbackExhaustionIsExplicit(t *testing.T) {
	anchor, missing := FindOpeningAnchor("2026-09-06", nil)
	if !missing || anchor != "" {
		t.Fatalf("missing anchor = %q missing=%v", anchor, missing)
	}
	anchor, missing = FindOpeningAnchor("2026-09-06", []OracleClose{{MarketDate: "2026-09-04"}})
	if missing || anchor != "2026-09-04" {
		t.Fatalf("Friday anchor = %q missing=%v", anchor, missing)
	}
}
