package domain

import (
	"strings"
	"testing"
	"time"
)

func TestReviewSourceShadowRequiresFixedCutoffAndCompleteCheckpoint(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 0, 0, 1, time.UTC)
	base := ReviewProjectionCapture{SourceCutoff: at, HistoricalFingerprint: "synthetic", SourceRevision: &ReviewSourceCheckpoint{Token: SourceRevisionToken{HouseholdID: "synthetic", Epoch: strings.Repeat("a", 32), Revision: 10, TrackingVersion: "synthetic-tracking", ResolverPolicyVersion: "synthetic-resolver", EvaluatorVersion: ReviewEvaluator, MetricPolicyVersion: ReviewMetricPolicyVersion}}}
	for _, tc := range []struct {
		name   string
		change func(*ReviewProjectionCapture)
		want   ReviewSourceShadowVerdict
	}{
		{"same", func(*ReviewProjectionCapture) {}, ReviewSourceSame},
		{"scalar-without-revision", func(c *ReviewProjectionCapture) { c.HistoricalFingerprint = "corrected" }, ReviewSourceUntracked},
		{"revision-and-scalar", func(c *ReviewProjectionCapture) {
			c.SourceRevision.Token.Revision++
			c.HistoricalFingerprint = "corrected"
		}, ReviewSourceChanged},
		{"conservative", func(c *ReviewProjectionCapture) { c.SourceRevision.Token.Revision++ }, ReviewSourceConservative},
		{"new-epoch-same-number", func(c *ReviewProjectionCapture) { c.SourceRevision.Token.Epoch = strings.Repeat("b", 32) }, ReviewSourceConservative},
		{"policy", func(c *ReviewProjectionCapture) { c.SourceRevision.Token.ResolverPolicyVersion = "next" }, ReviewSourceConservative},
		{"evaluator", func(c *ReviewProjectionCapture) { c.SourceRevision.Token.EvaluatorVersion = "next" }, ReviewSourceConservative},
		{"metric", func(c *ReviewProjectionCapture) { c.SourceRevision.Token.MetricPolicyVersion = "next" }, ReviewSourceConservative},
		{"tracking", func(c *ReviewProjectionCapture) { c.SourceRevision.Token.TrackingVersion = "next" }, ReviewSourceConservative},
		{"nanosecond-activation", func(c *ReviewProjectionCapture) {
			c.SourceCutoff = c.SourceCutoff.Add(time.Nanosecond)
			c.HistoricalFingerprint = "active"
		}, ReviewSourceCutoffChanged},
		{"clock-rollback", func(c *ReviewProjectionCapture) { c.SourceCutoff = c.SourceCutoff.Add(-time.Nanosecond) }, ReviewSourceClockRollback},
		{"no-checkpoint", func(c *ReviewProjectionCapture) { c.SourceRevision = nil }, ReviewSourceUnavailable},
		{"invalid-epoch", func(c *ReviewProjectionCapture) { c.SourceRevision.Token.Epoch = strings.Repeat("g", 32) }, ReviewSourceUnavailable},
		{"invalid-floor", func(c *ReviewProjectionCapture) { c.SourceRevision.JournalFloor = 11 }, ReviewSourceUnavailable},
		{"missing-policy", func(c *ReviewProjectionCapture) { c.SourceRevision.Token.ResolverPolicyVersion = "" }, ReviewSourceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := base
			checkpoint := *base.SourceRevision
			candidate.SourceRevision = &checkpoint
			tc.change(&candidate)
			if got := CompareReviewSourceCheckpoints(base, candidate); got.Verdict != tc.want {
				t.Fatalf("verdict=%+v want=%s", got, tc.want)
			}
		})
	}
	pruned := base
	checkpoint := *base.SourceRevision
	pruned.SourceRevision = &checkpoint
	pruned.SourceRevision.JournalFloor = 10
	if o := CompareReviewSourceCheckpoints(base, pruned); o.Verdict != ReviewSourceSame || o.BelowJournalFloor {
		t.Fatalf("floor changed token identity: %+v", o)
	}
	pruned.SourceRevision.Token.Revision = 11
	pruned.SourceRevision.JournalFloor = 11
	if o := CompareReviewSourceCheckpoints(base, pruned); !o.BelowJournalFloor {
		t.Fatal("stale journal consumer not reported")
	}
	pruned.SourceRevision.Token.HouseholdID = "other-synthetic-household"
	if o := CompareReviewSourceCheckpoints(base, pruned); o.BelowJournalFloor {
		t.Fatal("journal floors compared across household namespaces")
	}
}
