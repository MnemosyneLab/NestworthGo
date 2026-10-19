package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestReviewTemporalPublicationPersistsIntervalIntersection(t *testing.T) {
	for _, scenario := range []string{"earlier-untracked", "later-untracked", "expired-untracked", "earlier-tracked", "quantity-untracked"} {
		t.Run(scenario, func(t *testing.T) {
			f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
			f.day(t, "2026-10-03")
			saveSyntheticReviewRule(t, f, "Synthetic publication interval", syntheticHistoricalReviewSpec())
			before := *f.now
			t1, t2 := before.Add(time.Hour), before.Add(2*time.Hour)
			original, changed := t2, t1
			if scenario == "later-untracked" {
				original, changed = t1, t2
			}
			var activityID string
			if scenario == "quantity-untracked" {
				*f.now = original
				// A quantity-only removal (no holding_cost effect) still appears
				// in the independent, uncut cost-event fingerprint.
				if _, err := f.s.RecordChange(t.Context(), domain.PositionAdjustmentInput{HouseholdID: f.account.Account.HouseholdID, HoldingID: f.holding.ID, Quantity: mustQuantity(t, "1"), Added: false, EffectiveAt: original}); err != nil {
					t.Fatal(err)
				}
				if err := f.db.SQL.QueryRow(`SELECT activity_id FROM activity_effects WHERE holding_id=? AND target='holding_quantity'`, f.holding.ID.String()).Scan(&activityID); err != nil {
					t.Fatal(err)
				}
				*f.now = before
			} else {
				// Cash facts are excluded from the review quantity/cost closure,
				// but the conservative uncut frontier scans every activity.
				activityID = domain.NewActivityID().String()
				if _, err := f.db.SQL.Exec(`INSERT INTO activities(id,household_id,kind,reason,effective_at,effective_local_date,created_at) VALUES(?,?,'cash_in','reconciliation',?,?,?)`, activityID, f.account.Account.HouseholdID.String(), original.Format(time.RFC3339Nano), original.Format("2006-01-02"), before.Format(time.RFC3339Nano)); err != nil {
					t.Fatal(err)
				}
				if _, err := f.db.SQL.Exec(`INSERT INTO activity_effects(id,activity_id,sequence,role,direction,target,classification,account_id,amount,currency) VALUES(?,?,1,'cash','added','account_cash','remeasurement',?,'1','CNY')`, domain.NewActivityID().String(), activityID, f.account.Account.ID.String()); err != nil {
					t.Fatal(err)
				}
			}
			capture := temporalCapture(t, f)
			if !capture.Temporal.NextTransitionAt.Equal(original) || len(capture.Historical.History.Activities) != 0 {
				t.Fatal("fixture did not isolate a future frontier")
			}
			stage := temporalPublicationOracle(t, f, capture)
			if scenario != "earlier-tracked" {
				if _, err := f.db.SQL.Exec(`DROP TRIGGER financial_source_activities_update`); err != nil {
					t.Fatal(err)
				}
			}
			result, err := f.db.SQL.Exec(`UPDATE activities SET effective_at=? WHERE id=?`, changed.Format(time.RFC3339Nano), activityID)
			if err != nil {
				t.Fatal(err)
			}
			if n, err := result.RowsAffected(); err != nil || n != 1 {
				t.Fatal("future activity was not moved")
			}
			current := temporalCapture(t, f)
			if !current.Temporal.NextTransitionAt.Equal(changed) || len(current.Historical.History.Activities) != 0 {
				t.Fatal("recheck did not isolate the changed frontier")
			}
			sameToken := current.SourceRevision.Token == capture.SourceRevision.Token
			sameFingerprint := current.HistoricalFingerprint == capture.HistoricalFingerprint
			if sameToken != (scenario != "earlier-tracked") || sameFingerprint != (scenario != "quantity-untracked") {
				t.Fatalf("unexpected source evidence: same token=%v, same fingerprint=%v", sameToken, sameFingerprint)
			}
			// Full recomputation remains the oracle after the adversarial edit.
			temporalPublicationOracle(t, f, current)
			if scenario == "expired-untracked" {
				*f.now = t1
			}
			err = sqlite.NewRepository(f.db).PublishReviewProjectionWithClock(t.Context(), stage, func() time.Time { return *f.now })
			if scenario == "earlier-tracked" || scenario == "quantity-untracked" || scenario == "expired-untracked" {
				temporalConflict(t, err)
				for _, table := range []string{"investment_review_daily_inputs", "investment_review_daily_results", "investment_review_episodes", "review_temporal_checkpoints"} {
					var count int
					if err := f.db.SQL.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
						t.Fatalf("rejected stage wrote %s: %d %v", table, count, err)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			// Both directions must retain the earlier proof boundary, and
			// publication must not mutate the caller's captured certificate.
			if !capture.Temporal.NextTransitionAt.Equal(original) {
				t.Fatal("publication mutated captured certificate")
			}
			var saved string
			if err := f.db.SQL.QueryRow(`SELECT next_transition_at FROM review_temporal_checkpoints`).Scan(&saved); err != nil {
				t.Fatal(err)
			}
			bound, err := time.Parse(time.RFC3339Nano, saved)
			if err != nil {
				t.Fatal(err)
			}
			for i, at := range []time.Time{t1.Add(-time.Nanosecond), t1} {
				*f.now = at
				page, err := f.s.ListReviewAlertGroupsV2(t.Context(), domain.ReviewAlertFilter{}, "", 10)
				want := "complete"
				if i == 1 {
					want = "stale"
				}
				if err != nil || page.ProjectionStatus != want {
					t.Errorf("at %s: status=%s want=%s err=%v", at, page.ProjectionStatus, want, err)
				}
			}
			if !bound.Equal(t1) {
				t.Fatalf("persisted %s; intersection must end at %s", bound, t1)
			}
		})
	}
}
