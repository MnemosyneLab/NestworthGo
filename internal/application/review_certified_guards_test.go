package application

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func certifiedStage(t *testing.T, f seriesZeroFixture, r *sqlite.Repository) domain.ReviewPublication {
	t.Helper()
	selection := f.s.reviewProviderSnapshot()
	c, err := r.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	c.Certified.Provider = selection
	p, err := PrepareReviewPublication(t.Context(), c, *f.now, selection.Key)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func certifiedPublish(ctx context.Context, f seriesZeroFixture, r *sqlite.Repository, p domain.ReviewPublication, clock func() time.Time) error {
	return r.PublishCertifiedReviewProjection(ctx, p, clock, func(fn func() error) error { return f.s.withReviewProviderLease(p.Capture.Certified.Provider, fn) })
}
func assertNoCertifiedMAC(t *testing.T, f seriesZeroFixture) {
	t.Helper()
	var mac sql.NullString
	if err := f.db.SQL.QueryRow(`SELECT json_extract(certificate_json,'$.CertifiedMAC') FROM review_temporal_checkpoints`).Scan(&mac); err != nil {
		t.Fatal(err)
	}
	if mac.Valid && mac.String != "" {
		t.Fatal("unknown or damaged state acquired integrity authority")
	}
}

func TestReviewCertifiedCorruptionCannotBeRecertified(t *testing.T) {
	for _, scenario := range []string{"old-input-json", "old-input-fingerprint", "unreadable-old-input", "unreadable-old-result", "old-result-json", "frozen-json", "frozen-json-and-fingerprint", "event-json", "evaluation", "control-without-revision", "old-version-json", "missing-day", "same-count-key", "forged-certificate", "malformed-authority"} {
		t.Run(scenario, func(t *testing.T) {
			f, r := certifiedFixture(t)
			statement := ""
			switch scenario {
			case "old-input-fingerprint":
				statement = `UPDATE investment_review_daily_inputs SET input_fingerprint='synthetic-forged' WHERE on_date='2026-10-01'`
			case "unreadable-old-input":
				statement = `UPDATE investment_review_daily_inputs SET evidence_json='[]' WHERE on_date='2026-10-01'`
			case "unreadable-old-result":
				statement = `UPDATE investment_review_daily_results SET evidence_json='[]' WHERE on_date='2026-10-01'`
			case "old-input-json":
				statement = `UPDATE investment_review_daily_inputs SET evidence_json=json_set(evidence_json,'$.Quantity','999') WHERE on_date='2026-10-01'`
			case "old-result-json":
				statement = `UPDATE investment_review_daily_results SET evidence_json=json_set(evidence_json,'$.truth','false') WHERE on_date='2026-10-01'`
			case "frozen-json":
				statement = `UPDATE investment_review_episodes SET evidence_json=json_set(evidence_json,'$.TriggerResult.truth','false')`
			case "frozen-json-and-fingerprint":
				statement = `UPDATE investment_review_episodes SET evidence_json=json_set(evidence_json,'$.TriggerResult.truth','false'),evidence_fingerprint='synthetic-forged'`
			case "event-json":
				statement = `UPDATE investment_review_episode_events SET payload_json='{}'`
			case "evaluation":
				statement = `UPDATE investment_review_evaluation_state SET consecutive_count=999`
			case "control-without-revision":
				statement = `UPDATE investment_review_control SET enabled=0`
			case "old-version-json":
				statement = `UPDATE investment_review_rule_versions SET semantic_hash='synthetic-forged'`
			case "missing-day":
				statement = `DELETE FROM investment_review_daily_inputs WHERE on_date='2026-10-01'`
			case "same-count-key":
				statement = `UPDATE investment_review_daily_inputs SET on_date='2026-09-30' WHERE on_date='2026-10-01'`
			case "malformed-authority":
				statement = `UPDATE review_temporal_checkpoints SET certificate_json=json_set(certificate_json,'$.CertifiedMAC',123)`
			case "forged-certificate":
				statement = `UPDATE review_temporal_checkpoints SET certificate_json=json_set(certificate_json,'$.CertifiedProjectionSeal','synthetic-forged','$.CertifiedMAC','synthetic-forged')`
			}
			if _, err := f.db.SQL.Exec(statement); err != nil {
				t.Fatal(err)
			}
			f.day(t, "2026-10-13")
			p := certifiedStage(t, f, r)
			if p.Capture.Certified.MayCertify || len(p.Rules[0].Days) != 12 {
				t.Fatal("corruption bypassed full oracle fallback")
			}
			if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
				t.Fatal(err)
			}
			assertNoCertifiedMAC(t, f)
			*f.now = f.now.Add(time.Nanosecond)
			got, err := f.s.ReconcileInvestmentReviewsV2(t.Context())
			if err != nil || got.DayCount == 0 {
				t.Fatal("fallback laundered unknown history into a no-op", got, err)
			}
		})
	}
}

func TestReviewCertifiedLegitimateHandlingPreservesChain(t *testing.T) {
	f, r := certifiedFixture(t)
	var id string
	var revision int
	if err := f.db.SQL.QueryRow(`SELECT id,revision FROM investment_review_episodes WHERE resolved_at IS NULL`).Scan(&id, &revision); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), id, string(domain.NewMutationID()), revision); err != nil {
		t.Fatal(err)
	}
	reference := windowOracleReference(t, f) // authorized ack, before publication
	got, err := f.s.ReconcileInvestmentReviewsV2(t.Context())
	if err != nil || got.DayCount != 11 {
		t.Fatal("ack did not select full fallback", got, err)
	}
	if !reflect.DeepEqual(noChangeProjectionRows(t, f.db), noChangeProjectionRows(t, reference)) {
		t.Fatal("ack/full-oracle divergence")
	}
	*f.now = f.now.Add(time.Nanosecond)
	got, err = f.s.ReconcileInvestmentReviewsV2(t.Context())
	if err != nil || got.DayCount != 0 {
		t.Fatal("authorized handling lost certification", got, err)
	}
	f.day(t, "2026-10-13")
	p := certifiedStage(t, f, r)
	if p.Rules[0].CurrentEpisode.AcknowledgedAt == nil {
		t.Fatal("full publication lost acknowledgement")
	}
}

func TestReviewCertifiedPublicationGuardsAndAtomicity(t *testing.T) {
	for _, scenario := range []string{"source", "projection", "review-without-revision", "provider", "staged-provider", "provider-aba", "floor", "clock", "cancel", "control-ignore", "checkpoint-ignore", "checkpoint-abort", "source-proof-forged"} {
		t.Run(scenario, func(t *testing.T) {
			f, r := certifiedPublicationFixture(t, false, syntheticHistoricalReviewSpec(), "1")
			f.day(t, "2026-10-13")
			p := certifiedStage(t, f, r)
			statement := ""
			clock := func() time.Time { return *f.now }
			ctx := t.Context()
			switch scenario {
			case "source":
				statement = `UPDATE instrument_quotes SET unit_price='101'`
			case "projection":
				statement = `UPDATE investment_review_daily_inputs SET evidence_json='{}' WHERE on_date='2026-10-01'`
			case "review-without-revision":
				statement = `UPDATE investment_review_rules SET name='Synthetic race'`
			case "staged-provider":
				p.FXProviderKey = "synthetic-other"
			case "provider":
				if err := f.s.SetFXProvider("synthetic-other"); err != nil {
					t.Fatal(err)
				}
			case "provider-aba":
				for _, key := range []string{"synthetic-other", "synthetic-pinned"} {
					if err := f.s.SetFXProvider(key); err != nil {
						t.Fatal(err)
					}
				}
			case "floor":
				if err := f.db.PruneFinancialSourceChanges(ctx, f.account.Account.HouseholdID.String(), p.Capture.SourceRevision.Token.Revision); err != nil {
					t.Fatal(err)
				}
			case "clock":
				clock = func() time.Time { return p.Capture.Temporal.NextTransitionAt }
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				defer cancel()
				clock = func() time.Time { cancel(); return *f.now }
			case "control-ignore":
				statement = `CREATE TRIGGER synthetic_cert_ignore BEFORE UPDATE ON investment_review_control BEGIN SELECT RAISE(IGNORE); END`
			case "checkpoint-ignore":
				statement = `CREATE TRIGGER synthetic_cert_ignore BEFORE UPDATE ON review_temporal_checkpoints BEGIN SELECT RAISE(IGNORE); END`
			case "checkpoint-abort":
				statement = `CREATE TRIGGER synthetic_cert_ignore BEFORE UPDATE ON review_temporal_checkpoints BEGIN SELECT RAISE(ABORT,'synthetic'); END`
			case "source-proof-forged":
				p.Capture.Certified.SourceMAC = "synthetic-forged"
				statement = `UPDATE instrument_quotes SET unit_price='101'`
			}
			if statement != "" {
				if _, err := f.db.SQL.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			before := noChangeProjectionRows(t, f.db)
			var certificate string
			if err := f.db.SQL.QueryRow(`SELECT certificate_json FROM review_temporal_checkpoints`).Scan(&certificate); err != nil {
				t.Fatal(err)
			}
			err := certifiedPublish(ctx, f, r, p, clock)
			if err == nil {
				t.Fatal("stale/failed stage published")
			}
			if !reflect.DeepEqual(before, noChangeProjectionRows(t, f.db)) {
				t.Fatal("failed publication partially changed data")
			}
			var after string
			if err := f.db.SQL.QueryRow(`SELECT certificate_json FROM review_temporal_checkpoints`).Scan(&after); err != nil || after != certificate {
				t.Fatal("failed publication changed certificate", err)
			}
		})
	}
}

// Direct unleased callers publish all days with full source checks and never
// gain certification. Unexpected reusable C2 evidence cannot enter this path.
func TestReviewCertifiedDirectCallerFallback(t *testing.T) {
	for _, scenario := range []string{"unleased", "unexpected-reuse"} {
		t.Run(scenario, func(t *testing.T) {
			f, r := certifiedFixture(t)
			f.day(t, "2026-10-13")
			reference := windowOracleReference(t, f)
			p := certifiedStage(t, f, r)
			if len(p.Rules[0].Days) != 12 {
				t.Fatal("direct caller fixture is not complete")
			}
			if scenario == "unexpected-reuse" {
				p.Capture.ReusableDailyEvidence = map[domain.ReviewDailyEvidenceKey]domain.ReviewReusableDailyEvidence{{}: {}}
				before := noChangeProjectionRows(t, f.db)
				if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err == nil {
					t.Fatal("unexpected reusable evidence accepted")
				}
				if !reflect.DeepEqual(before, noChangeProjectionRows(t, f.db)) {
					t.Fatal("rejected direct stage wrote data")
				}
				return
			}
			if err := r.PublishReviewProjection(t.Context(), p, *f.now); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(noChangeProjectionRows(t, f.db), noChangeProjectionRows(t, reference)) {
				t.Fatal("direct full-source publication diverged")
			}
			assertNoCertifiedMAC(t, f)
		})
	}
}
