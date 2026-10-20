package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestReviewTemporalCalendarNanosecondBoundaries(t *testing.T) {
	for _, tc := range []struct{ zone, now, next string }{
		{"UTC", "2026-10-19T23:59:59.999999999Z", "2026-10-20T00:00:00Z"},
		{"Asia/Kathmandu", "2026-10-19T12:00:00Z", "2026-10-19T18:15:00Z"},
		{"America/New_York", "2026-03-08T06:59:59.999999999Z", "2026-03-08T07:00:00Z"},
		{"America/New_York", "2026-03-08T07:00:00Z", "2026-03-09T04:00:00Z"},
		{"America/New_York", "2026-11-01T05:59:59.999999999Z", "2026-11-01T06:00:00Z"},
		{"America/New_York", "2026-11-01T06:00:00Z", "2026-11-02T05:00:00Z"},
		{"Pacific/Apia", "2011-12-30T09:59:59.999999999Z", "2011-12-30T10:00:00Z"},
		{"Pacific/Apia", "2011-12-30T10:00:00Z", "2011-12-31T10:00:00Z"},
	} {
		t.Run(tc.zone+tc.now, func(t *testing.T) {
			now, _ := time.Parse(time.RFC3339Nano, tc.now)
			want, _ := time.Parse(time.RFC3339Nano, tc.next)
			got, err := ReviewNextCalendarBoundary(now, tc.zone)
			if err != nil || !got.Equal(want) {
				t.Fatalf("next=%s want=%s err=%v", got, want, err)
			}
		})
	}
}

func TestReviewTemporalCertificateNeverCertifiesUnknownOrExpiredInputs(t *testing.T) {
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	base := ReviewTemporalCertificate{EvaluatedAt: at, NextTransitionAt: at.Add(time.Hour), PolicyVersion: ReviewTemporalPolicyVersion, DependencyScope: ReviewTemporalDependencyScope, ClockEpoch: "synthetic-session/0", Source: &ReviewSourceCheckpoint{Token: SourceRevisionToken{HouseholdID: "synthetic", Epoch: strings.Repeat("a", 32), Revision: 10, TrackingVersion: "synthetic", ResolverPolicyVersion: "synthetic", EvaluatorVersion: ReviewEvaluator, MetricPolicyVersion: ReviewMetricPolicyVersion}}, ConfigRevision: 1, StateRevision: 1, RuleRevisions: map[ReviewRuleID]int{"synthetic-rule": 1}}
	for _, tc := range []struct {
		name   string
		mutate func(*ReviewTemporalCertificate)
	}{
		{"restart-or-calendar", func(c *ReviewTemporalCertificate) { c.ClockEpoch = "new-session/0" }},
		{"rollback-above-published", func(c *ReviewTemporalCertificate) { c.ClockEpoch = "synthetic-session/1" }},
		{"missing-source", func(c *ReviewTemporalCertificate) { c.Source = nil }},
		{"unknown-current-policy", func(c *ReviewTemporalCertificate) { c.PolicyVersion = "synthetic-unknown" }},
		{"unknown-current-scope", func(c *ReviewTemporalCertificate) { c.DependencyScope = "synthetic-unknown" }},
		{"new-revision", func(c *ReviewTemporalCertificate) { c.Source.Token.Revision++ }},
		{"restore", func(c *ReviewTemporalCertificate) { c.Source.Token.Epoch = strings.Repeat("b", 32) }},
		{"rule", func(c *ReviewTemporalCertificate) { c.RuleRevisions["synthetic-rule"]++ }},
		{"config", func(c *ReviewTemporalCertificate) { c.ConfigRevision++ }},
		{"handling", func(c *ReviewTemporalCertificate) { c.StateRevision++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := jsoncodec.Marshal(base)
			var current ReviewTemporalCertificate
			if err := jsoncodec.Unmarshal(raw, &current); err != nil {
				t.Fatal(err)
			}
			tc.mutate(&current)
			if base.Eligible(at, &current) {
				t.Fatal("incompatible certificate accepted")
			}
		})
	}
	if !base.Eligible(base.NextTransitionAt.Add(-time.Nanosecond), &base) || base.Eligible(base.NextTransitionAt, &base) || base.Eligible(at.Add(-time.Nanosecond), &base) {
		t.Fatal("certificate is not the exact half-open interval")
	}
	for _, version := range []string{"", "unknown-policy"} {
		c := base
		c.PolicyVersion = version
		if c.Covers(at) {
			t.Fatal("unknown temporal policy certified")
		}
	}
	c := base
	c.NextTransitionAt = time.Time{}
	if c.Covers(at) {
		t.Fatal("missing boundary interpreted as infinity")
	}
}
