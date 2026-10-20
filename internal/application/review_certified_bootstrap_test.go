package application

import (
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func historyFreeLegacyReview(t *testing.T) (seriesZeroFixture, *sqlite.Repository) {
	t.Helper()
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	f.day(t, "2026-10-12")
	spec := syntheticHistoricalReviewSpec()
	spec.Predicate.All[1].Leaf.Value = "0" // No trigger history, including closed episodes.
	saveSyntheticReviewRule(t, f, "Synthetic history-free restart", spec)
	f.s = NewService(sqlite.NewRepository(f.db).WithFullReviewRecompute())
	f.s.fxProviderKey = "synthetic-pinned"
	f.s.setClock(func() time.Time { return *f.now })
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_episodes`).Scan(&count); err != nil || count != 0 {
		t.Fatal("fixture has trigger history", count, err)
	}
	r := sqlite.NewRepository(f.db).WithCertifiedReviewCandidate()
	f.s = NewService(r)
	f.s.fxProviderKey = "synthetic-pinned"
	f.s.setClock(func() time.Time { return *f.now })
	return f, r
}

func requireCertifiedNoop(t *testing.T, f seriesZeroFixture, r *sqlite.Repository) {
	t.Helper()
	_, ok, err := r.ConfirmCertifiedReviewNoChange(t.Context(), f.account.Account.HouseholdID, *f.now, func() time.Time { return *f.now }, f.s.reviewProviderSnapshot())
	if err != nil || !ok {
		t.Fatal("full rebuild did not establish no-op authority", ok, err)
	}
}

func TestReviewCertifiedHistoryFreeLegacyAndRestartRemainUntrusted(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "restart"}[restart], func(t *testing.T) {
			var f seriesZeroFixture
			var r *sqlite.Repository
			if restart {
				spec := syntheticHistoricalReviewSpec()
				spec.Predicate.All[1].Leaf.Value = "0"
				f, r = certifiedConfiguredFixture(t, false, spec)
				// A valid prior-session signature is not authority after key loss.
				requireCertifiedNoop(t, f, r)
				path := f.db.Path
				if err := f.db.Close(); err != nil {
					t.Fatal(err)
				}
				db, err := sqlite.Open(path)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = db.Close() })
				f.db = db
				r = sqlite.NewRepository(db).WithCertifiedReviewCandidate()
				f.s = NewService(r)
				f.s.fxProviderKey = "synthetic-pinned"
				f.s.setClock(func() time.Time { return *f.now })
			} else {
				f, r = historyFreeLegacyReview(t)
			}
			_, ok, err := r.ConfirmCertifiedReviewNoChange(t.Context(), f.account.Account.HouseholdID, *f.now, func() time.Time { return *f.now }, f.s.reviewProviderSnapshot())
			if err != nil || ok {
				t.Fatal("new handle trusted an old certificate", ok, err)
			}
			p := certifiedStage(t, f, r)
			if p.Capture.Certified.MayCertify || len(p.Rules[0].Days) != 11 {
				t.Fatal("legacy/restart escaped full untrusted fallback")
			}
			// Separate source-only full preparation predates candidate publication.
			c, err := r.WithFullReviewRecompute().CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
			if err != nil {
				t.Fatal(err)
			}
			oracle, err := PrepareReviewPublication(t.Context(), c, *f.now, "synthetic-pinned")
			if err != nil {
				t.Fatal(err)
			}
			if err := compareReviewFullOracle(p, oracle, "synthetic-pinned"); err != nil {
				t.Fatal(err)
			}
			if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
				t.Fatal(err)
			}
			assertNoCertifiedMAC(t, f)
			for range 2 {
				_, ok, err := r.ConfirmCertifiedReviewNoChange(t.Context(), f.account.Account.HouseholdID, *f.now, func() time.Time { return *f.now }, f.s.reviewProviderSnapshot())
				if err != nil || ok {
					t.Fatal("unproven absence acquired no-op authority", ok, err)
				}
			}
			noChangeFullOracle(t, f)
		})
	}
}

func TestReviewCertifiedRecomputeDoesNotProveHistoricalProvenance(t *testing.T) {
	f, r := historyFreeLegacyReview(t)
	for _, q := range []string{
		`UPDATE investment_review_daily_inputs SET quantity='999',amount_exact='999',cost_exact='999',instrument_type='fund',native_currency='USD',evidence_json='{}',coverage_json='["bad"]'`,
		`UPDATE investment_review_daily_results SET predicate_truth='unknown',reason='bad',evidence_json='{}'`,
		`UPDATE investment_review_position_epochs SET opened_at=NULL,opening_activity_id=NULL,start_unknown=1-start_unknown,source_fingerprint='bad'`,
		`UPDATE investment_review_evaluation_state SET consecutive_count=999,dirty_from='bad',coverage_json='["bad"]'`,
	} {
		if _, err := f.db.SQL.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	p := certifiedStage(t, f, r)
	if p.Capture.Certified.MayCertify {
		t.Fatal("recomputation was mistaken for historical provenance")
	}
	if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
		t.Fatal(err)
	}
	assertNoCertifiedMAC(t, f)
	// Full fallback is not claimed to repair every retained scalar or frozen
	// historical payload. In particular, a checksum after it grants no trust.
	_, ok, err := r.ConfirmCertifiedReviewNoChange(t.Context(), f.account.Account.HouseholdID, *f.now, func() time.Time { return *f.now }, f.s.reviewProviderSnapshot())
	if err != nil || ok {
		t.Fatal("unknown projection acquired no-op authority", ok, err)
	}

}

func TestReviewCertifiedBootstrapCannotBlessUnknownNamespace(t *testing.T) {
	for _, kind := range []string{"input-policy", "result-timezone", "epoch"} {
		t.Run(kind, func(t *testing.T) {
			f, r := historyFreeLegacyReview(t)
			q := map[string]string{
				"input-policy":    `UPDATE investment_review_daily_inputs SET input_policy='unknown' WHERE on_date='2026-10-01'`,
				"result-timezone": `UPDATE investment_review_daily_results SET timezone_version='unknown' WHERE on_date='2026-10-01'`,
				"epoch":           `INSERT INTO investment_review_position_epochs SELECT 'unknown',household_id,instrument_id,opened_at,opening_activity_id,closed_at,closing_activity_id,start_unknown,source_fingerprint FROM investment_review_position_epochs LIMIT 1`,
			}[kind]
			if _, err := f.db.SQL.Exec(q); err != nil {
				t.Fatal(err)
			}
			p := certifiedStage(t, f, r)
			if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
				t.Fatal(err)
			}
			assertNoCertifiedMAC(t, f)
		})
	}
}

func TestReviewCertifiedBootstrapRetainsUnprovenHistory(t *testing.T) {
	f, _ := certifiedFixture(t)
	f.day(t, "2026-10-13")
	f.quantity(t, false)
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var resolved int
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_episodes WHERE resolved_at IS NOT NULL`).Scan(&resolved); err != nil || resolved != 1 {
		t.Fatal("missing retained history", resolved, err)
	}
	if _, err := f.db.SQL.Exec(`UPDATE investment_review_episode_events SET payload_json='{}' WHERE kind='opened'`); err != nil {
		t.Fatal(err)
	}
	path := f.db.Path
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	f.db = db
	r := sqlite.NewRepository(db).WithCertifiedReviewCandidate()
	f.s = NewService(r)
	f.s.fxProviderKey = "synthetic-pinned"
	f.s.setClock(func() time.Time { return *f.now })
	p := certifiedStage(t, f, r)
	if p.Capture.Certified.MayCertify {
		t.Fatal("retained resolved history acquired provenance")
	}
	if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
		t.Fatal(err)
	}
	assertNoCertifiedMAC(t, f)
	var event string
	if err := db.SQL.QueryRow(`SELECT payload_json FROM investment_review_episode_events WHERE kind='opened'`).Scan(&event); err != nil || event != "{}" {
		t.Fatal("fallback rewrote frozen history", event, err)
	}
}

func TestReviewCertifiedBootstrapRejectsUnsupportedRules(t *testing.T) {
	for _, kind := range []string{"disabled", "archived", "old-condition"} {
		t.Run(kind, func(t *testing.T) {
			f, r := historyFreeLegacyReview(t)
			for _, q := range map[string][]string{
				"disabled":      {`UPDATE investment_review_rules SET enabled=0`},
				"archived":      {`UPDATE investment_review_rules SET archived_at=created_at`},
				"old-condition": {`INSERT INTO investment_review_rule_versions SELECT rule_id,2,spec_version,canonical_spec_json,semantic_hash,created_at FROM investment_review_rule_versions`, `UPDATE investment_review_rules SET current_condition_version=2`},
			}[kind] {
				if _, err := f.db.SQL.Exec(q); err != nil {
					t.Fatal(err)
				}
			}
			p := certifiedStage(t, f, r)
			if p.Capture.Certified.MayCertify {
				t.Fatal("unsupported rule history selected bootstrap")
			}
			if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
				t.Fatal(err)
			}
			assertNoCertifiedMAC(t, f)
		})
	}
}

func TestReviewCertifiedBootstrapConflictsAtomically(t *testing.T) {
	for _, kind := range []string{"source", "projection", "temporal-cas", "late-invalid-day"} {
		t.Run(kind, func(t *testing.T) {
			f, r := historyFreeLegacyReview(t)
			p := certifiedStage(t, f, r)
			q := map[string]string{
				"source":       `UPDATE instrument_quotes SET unit_price='101'`,
				"projection":   `UPDATE investment_review_daily_inputs SET quantity='999'`,
				"temporal-cas": `UPDATE review_temporal_checkpoints SET certificate_json=json_set(certificate_json,'$.CertifiedMAC','changed')`,
			}[kind]
			if kind == "late-invalid-day" {
				// The first date is written before the next date is rejected.
				p.Rules[0].Days[1].Input.On = "invalid"
			} else if _, err := f.db.SQL.Exec(q); err != nil {
				t.Fatal(err)
			}
			before := noChangeProjectionRows(t, f.db)
			var checkpoint string
			if err := f.db.SQL.QueryRow(`SELECT certificate_json FROM review_temporal_checkpoints`).Scan(&checkpoint); err != nil {
				t.Fatal(err)
			}
			if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err == nil || (kind != "late-invalid-day" && !hasDomainCode(err, domain.ErrConflict)) {
				t.Fatal("changed/invalid stage published a bootstrap", err)
			}
			if !reflect.DeepEqual(before, noChangeProjectionRows(t, f.db)) {
				t.Fatal("failed bootstrap partially published")
			}
			var after string
			if err := f.db.SQL.QueryRow(`SELECT certificate_json FROM review_temporal_checkpoints`).Scan(&after); err != nil || checkpoint != after {
				t.Fatal("failed bootstrap changed checkpoint", err)
			}
			_, ok, err := r.ConfirmCertifiedReviewNoChange(t.Context(), f.account.Account.HouseholdID, *f.now, func() time.Time { return *f.now }, f.s.reviewProviderSnapshot())
			if err != nil || ok {
				t.Fatal("failed bootstrap issued authority", ok, err)
			}
		})
	}
}
