package application

import (
	"context"
	"database/sql"
	"maps"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type lineageObservations struct {
	sync.Mutex
	items []domain.ReviewCompletenessLineageObservation
}

func (o *lineageObservations) observe(v domain.ReviewCompletenessLineageObservation) {
	o.Lock()
	defer o.Unlock()
	o.items = append(o.items, v)
}

func (o *lineageObservations) last() domain.ReviewCompletenessLineageObservation {
	o.Lock()
	defer o.Unlock()
	return o.items[len(o.items)-1]
}

func lineageFixture(t *testing.T, policy domain.ReviewTriggerPolicy) (seriesZeroFixture, *sqlite.Repository, *lineageObservations) {
	t.Helper()
	spec := syntheticHistoricalReviewSpec()
	spec.TriggerPolicy = policy
	return lineageSpecFixture(t, spec)
}

func lineageSpecFixture(t *testing.T, spec domain.ReviewRuleSpec) (seriesZeroFixture, *sqlite.Repository, *lineageObservations) {
	t.Helper()
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	if len(spec.Predicate.All) > 1 && spec.Predicate.All[1].Leaf != nil && spec.Predicate.All[1].Leaf.Metric == "amount" {
		requireDailyReviewPrices(t, &f)
		allowDailyReviewCrypto(&spec.Predicate)
	}
	f.day(t, "2026-10-12")
	saveSyntheticReviewRule(t, f, "Synthetic completeness lineage", spec)
	f.s, _ = noChangeService(f)
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	o := &lineageObservations{}
	r := sqlite.NewRepository(f.db).WithBoundedReviewWindow().WithReviewCompletenessLineageShadow(o.observe)
	f.day(t, "2026-10-13")
	return f, r, o
}

func TestReviewCompletenessLineageStoredTruth(t *testing.T) {
	for _, truth := range []domain.ReviewTruth{domain.ReviewTrue, domain.ReviewFalse, domain.ReviewUnknown} {
		name := string(truth)
		if truth == domain.ReviewUnknown {
			name = "unknown"
		}
		t.Run(name, func(t *testing.T) {
			spec := syntheticHistoricalReviewSpec()
			if truth == domain.ReviewFalse {
				spec.Predicate.All[1].Leaf.Operator = ">"
			}
			if truth == domain.ReviewUnknown {
				// Agent-priced daily instruments require an eligible quote.
				spec.Predicate.All[1].Leaf.Metric = "amount"
			}
			f, r, o := lineageSpecFixture(t, spec)
			p := lineagePublication(t, f, r)
			if p.Rules[0].Days[0].Result.Truth != truth {
				t.Fatal("fixture did not produce expected truth", truth)
			}
			if got := domain.ValidateReviewCompletenessPrefix(p); got != "prefix_validated" {
				t.Fatal("stored truth encoding declined", name, got)
			}
			if err := r.PublishReviewProjection(t.Context(), p, *f.now); err != nil {
				t.Fatal(err)
			}
			if got := o.last().Verdict; got != "lineage_validated_shadow" {
				t.Fatal(got)
			}
			noChangeFullOracle(t, f)
		})
	}
}

func lineagePublication(t *testing.T, f seriesZeroFixture, r *sqlite.Repository) domain.ReviewPublication {
	t.Helper()
	capture, err := r.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if !capture.TemporalFullRecompute || len(capture.ReusableDailyEvidence) != 0 {
		t.Fatal("shadow consumed reusable evidence instead of the full oracle")
	}
	p, err := PrepareReviewPublication(t.Context(), capture, *f.now, f.s.FXProviderKey())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReviewCompletenessLineageShadowFullOracle(t *testing.T) {
	for _, policy := range []domain.ReviewTriggerPolicy{{Kind: "single"}, {Kind: "consecutive", Count: 3}, {Kind: "rolling", WindowDays: 3, MinHits: 2}} {
		t.Run(policy.Kind, func(t *testing.T) {
			f, r, o := lineageFixture(t, policy)
			if sqlite.NewRepository(f.db).ReviewCompletenessLineageShadowEnabled() || r.ReviewNoChangeEnabled() || r.WithFullReviewRecompute().ReviewCompletenessLineageShadowEnabled() {
				t.Fatal("shadow changed default activation or forced-full escape hatch")
			}
			before := noChangeProjectionRows(t, f.db)
			p := lineagePublication(t, f, r)
			l := p.Capture.CompletenessLineage
			if !l.Candidate || len(l.Inputs) != 11 || len(l.Results) != 11 || len(l.Window) == 0 || len(p.Rules[0].Days) != 12 || domain.ValidateReviewCompletenessPrefix(p) != "prefix_validated" {
				t.Fatalf("full oracle did not validate old prefix: %+v verdict=%s", l, domain.ValidateReviewCompletenessPrefix(p))
			}
			if !reflect.DeepEqual(before, noChangeProjectionRows(t, f.db)) {
				t.Fatal("shadow capture wrote derived state")
			}
			hypothetical := p.Capture
			hypothetical.CompletenessLineage = nil
			hypothetical.TemporalFullRecompute = false
			hypothetical.ReusableDailyEvidence = l.Window
			bounded, err := PrepareReviewPublication(t.Context(), hypothetical, *f.now, f.s.FXProviderKey())
			if err != nil || len(bounded.Rules[0].Days) != 1 {
				t.Fatalf("hypothetical bounded window: %v %+v", err, bounded.Rules)
			}
			if err := compareReviewFullOracle(bounded, p, f.s.FXProviderKey()); err != nil {
				t.Fatal(err)
			}
			if err := r.PublishReviewProjection(t.Context(), p, *f.now); err != nil {
				t.Fatal(err)
			}
			if got := o.last(); got.Phase != "publication" || got.Verdict != "lineage_validated_shadow" {
				t.Fatalf("publication observation: %+v", got)
			}
			noChangeFullOracle(t, f)
		})
	}
}

func TestReviewCompletenessLineageRejectsIncompletePrefix(t *testing.T) {
	for _, scenario := range []string{"missing-input", "same-count-key-substitution", "old-input-fingerprint", "old-result-truth", "window-payload", "window-streak", "unreadable-window"} {
		t.Run(scenario, func(t *testing.T) {
			f, r, _ := lineageFixture(t, domain.ReviewTriggerPolicy{Kind: "consecutive", Count: 3})
			statement, want := "", ""
			switch scenario {
			case "missing-input":
				statement, want = `DELETE FROM investment_review_daily_inputs WHERE on_date='2026-10-01'`, "counts_changed"
			case "same-count-key-substitution":
				statement, want = `UPDATE investment_review_daily_inputs SET on_date='2026-09-30' WHERE on_date='2026-10-01'`, "input_prefix_missing_or_changed"
			case "old-input-fingerprint":
				statement, want = `UPDATE investment_review_daily_inputs SET input_fingerprint='synthetic-damage' WHERE on_date='2026-10-01'`, "input_prefix_missing_or_changed"
			case "old-result-truth":
				statement, want = `UPDATE investment_review_daily_results SET predicate_truth='false' WHERE on_date='2026-10-01'`, "result_prefix_missing_or_changed"
			case "window-payload":
				statement, want = `UPDATE investment_review_daily_inputs SET evidence_json=json_set(evidence_json,'$.Quantity','999') WHERE on_date='2026-10-11'`, "window_payload_changed"
			case "window-streak":
				statement, want = `UPDATE investment_review_evaluation_state SET consecutive_count=999`, "window_policy_state_changed"
			case "unreadable-window":
				statement, want = `UPDATE investment_review_daily_inputs SET evidence_json='{}' WHERE on_date='2026-10-11'`, "window_unreadable"
			}
			if _, err := f.db.SQL.Exec(statement); err != nil {
				t.Fatal(err)
			}
			p := lineagePublication(t, f, r)
			if got := domain.ValidateReviewCompletenessPrefix(p); got != want {
				t.Fatalf("damaged prefix: got %s want %s", got, want)
			}
			// Missing history is reconstructed by the actual full publication.
			// A substituted extra key is deliberately not claimed to be repaired.
			if scenario == "missing-input" || scenario == "old-input-fingerprint" || scenario == "old-result-truth" {
				if err := r.PublishReviewProjection(t.Context(), p, *f.now); err != nil {
					t.Fatal(err)
				}
				noChangeFullOracle(t, f)
			}
		})
	}
}

func TestReviewCompletenessLineageRequiresFullOracle(t *testing.T) {
	f, r, _ := lineageFixture(t, domain.ReviewTriggerPolicy{Kind: "single"})
	p := lineagePublication(t, f, r)
	for _, scenario := range []string{"missing", "unknown-version", "missing-binding", "incremental-oracle", "provider", "duplicate-day", "extra-key"} {
		t.Run(scenario, func(t *testing.T) {
			candidate := cloneReviewOraclePublication(p)
			lineage := *p.Capture.CompletenessLineage
			lineage.Inputs = maps.Clone(lineage.Inputs)
			candidate.Capture.CompletenessLineage = &lineage
			want := ""
			switch scenario {
			case "missing":
				candidate.Capture.CompletenessLineage, want = nil, "not_observed"
			case "unknown-version":
				lineage.Version, want = "future-version", "untrusted_lineage"
			case "missing-binding":
				lineage.BindingDigest, want = "", "untrusted_lineage"
			case "incremental-oracle":
				candidate.Capture.TemporalFullRecompute, want = false, "full_oracle_required"
			case "provider":
				candidate.FXProviderKey, want = "synthetic-new-provider", "provider_changed"
			case "duplicate-day":
				candidate.Rules[0].Days[1], want = candidate.Rules[0].Days[0], "duplicate_oracle_key"
			case "extra-key":
				for key, value := range lineage.Inputs {
					key.On = "2026-09-30"
					lineage.Inputs[key] = value
					break
				}
				want = "unexpected_daily_key"
			}
			if got := domain.ValidateReviewCompletenessPrefix(candidate); got != want {
				t.Fatalf("got %s want %s", got, want)
			}
		})
	}
	for _, mode := range []*sqlite.Repository{sqlite.NewRepository(f.db), r.WithFullReviewRecompute()} {
		capture, err := mode.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
		if err != nil || capture.CompletenessLineage != nil {
			t.Fatal("default/full mode captured shadow", err)
		}
	}
}

func TestReviewCompletenessLineageFailsClosed(t *testing.T) {
	for _, scenario := range []string{"missing-proof", "unknown-proof", "temporal-policy", "tracking-policy", "rule-namespace", "rule-revision", "rule-metadata", "handling", "source", "floor-gap", "tracking-damage", "foreign-keys", "epoch", "rollback", "restart", "provider"} {
		t.Run(scenario, func(t *testing.T) {
			f, r, _ := lineageFixture(t, domain.ReviewTriggerPolicy{Kind: "single"})
			statement, want := "", ""
			switch scenario {
			case "missing-proof":
				statement, want = `UPDATE review_temporal_checkpoints SET certificate_json=json_remove(certificate_json,'$.NoChangeReviewFingerprint')`, "untrusted_checkpoint"
			case "unknown-proof":
				statement, want = `UPDATE review_temporal_checkpoints SET certificate_json=json_set(certificate_json,'$.NoChangeCompletenessVersion','future-version')`, "untrusted_checkpoint"
			case "temporal-policy":
				statement, want = `UPDATE review_temporal_checkpoints SET certificate_json=json_set(certificate_json,'$.PolicyVersion','future-version')`, "clock_or_policy_changed"
			case "tracking-policy":
				statement, want = `UPDATE financial_source_revision SET tracking_version='future-version'`, "source_changed"
			case "rule-namespace":
				statement, want = `INSERT INTO investment_review_rule_versions SELECT rule_id,condition_version+1,spec_version,canonical_spec_json,semantic_hash,created_at FROM investment_review_rule_versions; UPDATE investment_review_rules SET current_condition_version=current_condition_version+1,row_revision=row_revision+1`, "unsupported_or_changed_rule"
			case "rule-revision":
				statement, want = `UPDATE investment_review_rules SET row_revision=row_revision+1`, "unsupported_or_changed_rule"
			case "rule-metadata":
				statement, want = `UPDATE investment_review_rules SET name='Synthetic new name'`, "review_changed"
			case "handling":
				statement, want = `UPDATE investment_review_control SET review_state_revision=review_state_revision+1`, "review_changed"
			case "source":
				statement, want = `UPDATE instrument_quotes SET unit_price='101'`, "source_changed"
			case "floor-gap":
				statement, want = `DELETE FROM financial_source_changes WHERE revision=(SELECT max(revision) FROM financial_source_changes)`, "tracking_unverified"
			case "tracking-damage":
				statement, want = `DROP TRIGGER financial_source_instrument_quotes_update`, "tracking_unverified"
			case "foreign-keys":
				statement, want = `PRAGMA foreign_keys=0`, "tracking_unverified"
			case "epoch":
				if err := f.db.ResetFinancialSourceTracking(t.Context()); err != nil {
					t.Fatal(err)
				}
				want = "untrusted_checkpoint"
			case "rollback":
				r.ObserveReviewClock(f.now.Add(time.Hour))
				want = "clock_or_policy_changed"
			case "restart":
				db, err := sqlite.Open(f.db.Path)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				r = sqlite.NewRepository(db).WithReviewCompletenessLineageShadow(nil)
				want = "clock_or_policy_changed"
			case "provider":
				f.s.fxProviderKey = "synthetic-other-provider"
				want = "provider_changed"
			}
			if statement != "" {
				if _, err := f.db.SQL.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			p := lineagePublication(t, f, r)
			if got := domain.ValidateReviewCompletenessPrefix(p); got != want {
				t.Fatalf("unsafe lineage %s: got %s want %s", scenario, got, want)
			}
		})
	}
}

func TestReviewCompletenessLineagePublicationReservation(t *testing.T) {
	for _, scenario := range []string{"count-change", "same-count-change", "window-change", "lifecycle-change", "floor-change", "source-change", "handling-change", "cancel", "cancel-at-final-clock", "final-clock", "write-abort", "writer-reservation"} {
		t.Run(scenario, func(t *testing.T) {
			f, r, o := lineageFixture(t, domain.ReviewTriggerPolicy{Kind: "single"})
			p := lineagePublication(t, f, r)
			if got := domain.ValidateReviewCompletenessPrefix(p); got != "prefix_validated" {
				t.Fatal(got)
			}
			statement, want, conflict := "", "", false
			ctx := t.Context()
			clock := func() time.Time { return *f.now }
			switch scenario {
			case "count-change":
				statement, want = `DELETE FROM investment_review_daily_inputs WHERE on_date='2026-10-01'`, "counts_changed"
			case "same-count-change":
				statement, want = `UPDATE investment_review_daily_inputs SET input_fingerprint='synthetic-concurrent-change' WHERE on_date='2026-10-01'`, "projection_changed"
			case "window-change":
				statement, want = `UPDATE investment_review_daily_inputs SET evidence_json=json_set(evidence_json,'$.Quantity','999') WHERE on_date='2026-10-11'`, "window_changed"
			case "lifecycle-change":
				statement, want = `UPDATE investment_review_evaluation_state SET consecutive_count=999`, "window_changed"
			case "floor-change":
				if err := f.db.PruneFinancialSourceChanges(ctx, f.account.Account.HouseholdID.String(), p.Capture.SourceRevision.Token.Revision); err != nil {
					t.Fatal(err)
				}
				want = "binding_changed"
			case "source-change":
				statement, want, conflict = `UPDATE instrument_quotes SET unit_price='101'`, "authoritative_guard_rejected", true
			case "handling-change":
				statement, want, conflict = `UPDATE investment_review_control SET review_state_revision=review_state_revision+1`, "authoritative_guard_rejected", true
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				want, conflict = "authoritative_guard_rejected", true
			case "cancel-at-final-clock":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				defer cancel()
				clock = func() time.Time { cancel(); return *f.now }
				want, conflict = "authoritative_guard_rejected", true
			case "final-clock":
				clock = func() time.Time { return p.Capture.Temporal.NextTransitionAt }
				want, conflict = "authoritative_guard_rejected", true
			case "write-abort":
				statement = `CREATE TRIGGER synthetic_lineage_abort BEFORE UPDATE ON investment_review_control BEGIN SELECT RAISE(ABORT,'synthetic publication failure'); END`
				want, conflict = "authoritative_guard_rejected", true
			case "writer-reservation":
				writer, err := sql.Open("sqlite", f.db.Path+"?_txlock=immediate&_pragma=foreign_keys%3d1&_pragma=recursive_triggers%3d1&_pragma=busy_timeout%3d0")
				if err != nil {
					t.Fatal(err)
				}
				defer writer.Close()
				clock = func() time.Time {
					if _, err := writer.Exec(`UPDATE instrument_quotes SET unit_price='101'`); err == nil || !strings.Contains(err.Error(), "locked") {
						t.Errorf("writer escaped publication reservation: %v", err)
					}
					return *f.now
				}
				want = "lineage_validated_shadow"
			}
			if statement != "" {
				if _, err := f.db.SQL.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			before := noChangeProjectionRows(t, f.db)
			var temporalBefore string
			if err := f.db.SQL.QueryRow(`SELECT certificate_json FROM review_temporal_checkpoints`).Scan(&temporalBefore); err != nil {
				t.Fatal(err)
			}
			err := r.PublishReviewProjectionWithClock(ctx, p, clock)
			if conflict {
				if err == nil {
					t.Fatal("publication unexpectedly succeeded")
				}
				if !reflect.DeepEqual(before, noChangeProjectionRows(t, f.db)) {
					t.Fatal("rejected publication partially wrote projections")
				}
				var temporalAfter string
				if err := f.db.SQL.QueryRow(`SELECT certificate_json FROM review_temporal_checkpoints`).Scan(&temporalAfter); err != nil || temporalBefore != temporalAfter {
					t.Fatal("rejected publication wrote certificate", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				noChangeFullOracle(t, f)
			}
			if got := o.last(); got.Phase != "publication" || got.Verdict != want {
				t.Fatalf("wrong publication observation %+v want %s", got, want)
			}
		})
	}
}

func TestReviewCompletenessLineageFutureActivation(t *testing.T) {
	for _, kind := range []string{"cash", "quantity"} {
		t.Run(kind, func(t *testing.T) {
			f, r, _ := lineageFixture(t, domain.ReviewTriggerPolicy{Kind: "single"})
			before, at := *f.now, f.now.Add(time.Hour)
			if kind == "quantity" {
				*f.now = at
				f.quantity(t, false)
				*f.now = before
			} else {
				id := domain.NewActivityID().String()
				if _, err := f.db.SQL.Exec(`INSERT INTO activities(id,household_id,kind,reason,effective_at,effective_local_date,created_at) VALUES(?,?,'cash_in','reconciliation',?,?,?)`, id, f.account.Account.HouseholdID.String(), at.Format(time.RFC3339Nano), at.Format(time.DateOnly), before.Format(time.RFC3339Nano)); err != nil {
					t.Fatal(err)
				}
				if _, err := f.db.SQL.Exec(`INSERT INTO activity_effects(id,activity_id,sequence,role,direction,target,classification,account_id,amount,currency) VALUES(?,?,1,'cash','added','account_cash','remeasurement',?,'1','CNY')`, domain.NewActivityID().String(), id, f.account.Account.ID.String()); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
				t.Fatal(err)
			}
			*f.now = at.Add(-time.Nanosecond)
			p := lineagePublication(t, f, r)
			if got := domain.ValidateReviewCompletenessPrefix(p); got != "prefix_validated" {
				t.Fatalf("before activation: %s", got)
			}
			for _, now := range []time.Time{at, at.Add(time.Nanosecond)} {
				*f.now = now
				p := lineagePublication(t, f, r)
				if got := domain.ValidateReviewCompletenessPrefix(p); got == "prefix_validated" || got == "lineage_validated_shadow" {
					t.Fatalf("activation admitted: %s", got)
				}
				if err := r.PublishReviewProjection(t.Context(), p, *f.now); err != nil {
					t.Fatal(err)
				}
				noChangeFullOracle(t, f)
			}
		})
	}
}
