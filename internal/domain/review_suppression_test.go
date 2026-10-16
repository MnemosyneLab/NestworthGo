package domain

import (
	"testing"
	"time"
)

func TestReviewSuppressionCivilExpiryAndIdempotentAbsoluteDate(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	setAt := time.Date(2026, 10, 31, 15, 0, 0, 0, loc)
	until, err := ReviewSuppressionUntil(setAt, loc.String(), 7, "")
	if err != nil || until != "2026-11-07" {
		t.Fatalf("DST changed seven civil days: %q %v", until, err)
	}
	id := NewInstrumentID()
	entry := ReviewSuppression{ID: "suppression", InstrumentID: id, Mode: "until_date", UntilOn: until, Timezone: loc.String()}
	for _, tc := range []struct {
		at     time.Time
		active bool
	}{
		{time.Date(2026, 11, 6, 23, 59, 0, 0, loc), true},
		{time.Date(2026, 11, 7, 0, 0, 0, 0, loc), false},
	} {
		found, err := ActiveReviewSuppression([]ReviewSuppression{entry}, id, "rule-a", tc.at)
		if err != nil || (found != nil) != tc.active {
			t.Fatalf("at %s active=%v err=%v", tc.at, found, err)
		}
	}
	if _, err := ReviewSuppressionUntil(setAt, "Pacific/Apia", 0, "2011-12-30"); err == nil {
		t.Fatal("accepted skipped civil date")
	}
}

func TestReviewSuppressionScopesAndAcknowledgementRemainIndependent(t *testing.T) {
	id := NewInstrumentID()
	now := time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
	entries := []ReviewSuppression{
		{ID: "one-rule", InstrumentID: id, RuleID: "yield", Mode: "until_date", UntilOn: "2026-10-30", Timezone: "UTC"},
		{ID: "all-rules", InstrumentID: id, Mode: "permanent", Timezone: "UTC"},
	}
	selected, err := ActiveReviewSuppression(entries, id, "loss", now)
	if err != nil || selected == nil || selected.ID != "all-rules" {
		t.Fatalf("future rule not covered by permanent ignore: %+v %v", selected, err)
	}
	selected, err = ActiveReviewSuppression(entries[:1], id, "loss", now)
	if err != nil || selected != nil {
		t.Fatalf("rule-only ignore covered another rule: %+v %v", selected, err)
	}
	if !ReviewEpisodeActionable(true, true, true, false, ReviewTrue, nil) || ReviewEpisodeActionable(true, true, true, true, ReviewTrue, nil) || ReviewEpisodeActionable(true, true, true, false, ReviewUnknown, nil) || ReviewEpisodeActionable(true, true, true, false, ReviewTrue, &entries[0]) {
		t.Fatal("acknowledgement, unknown and ignore must independently suppress actionable status")
	}
}
