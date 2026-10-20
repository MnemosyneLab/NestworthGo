package domain_test

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/testsupport/reviewsource"
	"strings"
	"testing"
	"time"
)

func TestReviewSourceCheckpointRequiresFixedCutoffAndCompleteCheckpoint(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 0, 0, 1, time.UTC)
	base := domain.ReviewProjectionCapture{SourceCutoff: at, HistoricalFingerprint: "synthetic", SourceRevision: &domain.ReviewSourceCheckpoint{Token: domain.SourceRevisionToken{HouseholdID: "synthetic", Epoch: strings.Repeat("a", 32), Revision: 10, TrackingVersion: "synthetic-tracking", ResolverPolicyVersion: "synthetic-resolver", EvaluatorVersion: domain.ReviewEvaluator, MetricPolicyVersion: domain.ReviewMetricPolicyVersion}}}
	for _, tc := range []struct {
		name   string
		change func(*domain.ReviewProjectionCapture)
		want   reviewsource.Verdict
	}{
		{"same", func(*domain.ReviewProjectionCapture) {}, reviewsource.Same},
		{"scalar-without-revision", func(c *domain.ReviewProjectionCapture) { c.HistoricalFingerprint = "corrected" }, reviewsource.Untracked},
		{"revision-and-scalar", func(c *domain.ReviewProjectionCapture) {
			c.SourceRevision.Token.Revision++
			c.HistoricalFingerprint = "corrected"
		}, reviewsource.Changed},
		{"conservative", func(c *domain.ReviewProjectionCapture) { c.SourceRevision.Token.Revision++ }, reviewsource.Conservative},
		{"new-epoch-same-number", func(c *domain.ReviewProjectionCapture) { c.SourceRevision.Token.Epoch = strings.Repeat("b", 32) }, reviewsource.Conservative},
		{"policy", func(c *domain.ReviewProjectionCapture) { c.SourceRevision.Token.ResolverPolicyVersion = "next" }, reviewsource.Conservative},
		{"evaluator", func(c *domain.ReviewProjectionCapture) { c.SourceRevision.Token.EvaluatorVersion = "next" }, reviewsource.Conservative},
		{"metric", func(c *domain.ReviewProjectionCapture) { c.SourceRevision.Token.MetricPolicyVersion = "next" }, reviewsource.Conservative},
		{"tracking", func(c *domain.ReviewProjectionCapture) { c.SourceRevision.Token.TrackingVersion = "next" }, reviewsource.Conservative},
		{"nanosecond-activation", func(c *domain.ReviewProjectionCapture) {
			c.SourceCutoff = c.SourceCutoff.Add(time.Nanosecond)
			c.HistoricalFingerprint = "active"
		}, reviewsource.CutoffChanged},
		{"clock-rollback", func(c *domain.ReviewProjectionCapture) { c.SourceCutoff = c.SourceCutoff.Add(-time.Nanosecond) }, reviewsource.ClockRollback},
		{"no-checkpoint", func(c *domain.ReviewProjectionCapture) { c.SourceRevision = nil }, reviewsource.Unavailable},
		{"invalid-epoch", func(c *domain.ReviewProjectionCapture) { c.SourceRevision.Token.Epoch = strings.Repeat("g", 32) }, reviewsource.Unavailable},
		{"invalid-floor", func(c *domain.ReviewProjectionCapture) { c.SourceRevision.JournalFloor = 11 }, reviewsource.Unavailable},
		{"missing-policy", func(c *domain.ReviewProjectionCapture) { c.SourceRevision.Token.ResolverPolicyVersion = "" }, reviewsource.Unavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := base
			checkpoint := *base.SourceRevision
			candidate.SourceRevision = &checkpoint
			tc.change(&candidate)
			if got := reviewsource.Compare(base, candidate); got.Verdict != tc.want {
				t.Fatalf("verdict=%+v want=%s", got, tc.want)
			}
		})
	}
	pruned := base
	checkpoint := *base.SourceRevision
	pruned.SourceRevision = &checkpoint
	pruned.SourceRevision.JournalFloor = 10
	if o := reviewsource.Compare(base, pruned); o.Verdict != reviewsource.Same || o.BelowJournalFloor {
		t.Fatalf("floor changed token identity: %+v", o)
	}
	pruned.SourceRevision.Token.Revision = 11
	pruned.SourceRevision.JournalFloor = 11
	if o := reviewsource.Compare(base, pruned); !o.BelowJournalFloor {
		t.Fatal("stale journal consumer not reported")
	}
	pruned.SourceRevision.Token.HouseholdID = "other-synthetic-household"
	if o := reviewsource.Compare(base, pruned); o.BelowJournalFloor {
		t.Fatal("journal floors compared across household namespaces")
	}
}
