package application

import (
	"fmt"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// DDL does not increment total_changes(). Rewinding the writable schema cookie
// removes that independent hint too. These are single-connection transactions;
// PRAGMA data_version does not advance for the connection's own commits.
func certifiedOwnDDL(t *testing.T, f seriesZeroFixture, dropEvents, rollback bool) {
	t.Helper()
	var beforeChanges, beforeData, cookie int64
	if err := f.db.SQL.QueryRow(`SELECT total_changes()`).Scan(&beforeChanges); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SQL.QueryRow(`PRAGMA data_version`).Scan(&beforeData); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SQL.QueryRow(`PRAGMA schema_version`).Scan(&cookie); err != nil {
		t.Fatal(err)
	}
	statements := []string{`CREATE TABLE synthetic_ddl_probe(value TEXT)`, `DROP TABLE synthetic_ddl_probe`}
	if dropEvents {
		var ddl string
		if err := f.db.SQL.QueryRow(`SELECT sql FROM sqlite_master WHERE name='investment_review_episode_events'`).Scan(&ddl); err != nil {
			t.Fatal(err)
		}
		statements = []string{`DROP TABLE investment_review_episode_events`, ddl}
		rows, err := f.db.SQL.Query(`SELECT sql FROM sqlite_master WHERE type='index' AND tbl_name='investment_review_episode_events' AND sql IS NOT NULL ORDER BY name`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var indexDDL string
			if err := rows.Scan(&indexDDL); err != nil {
				t.Fatal(err)
			}
			statements = append(statements, indexDDL)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
	tx, err := f.db.SQL.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, q := range statements {
		if _, err := tx.ExecContext(t.Context(), q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.ExecContext(t.Context(), fmt.Sprintf("PRAGMA schema_version=%d", cookie)); err != nil {
		t.Fatal(err)
	}
	if rollback {
		err = tx.Rollback()
	} else {
		err = tx.Commit()
	}
	if err != nil {
		t.Fatal(err)
	}
	var afterChanges, afterData, afterCookie int64
	if err := f.db.SQL.QueryRow(`SELECT total_changes()`).Scan(&afterChanges); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SQL.QueryRow(`PRAGMA data_version`).Scan(&afterData); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SQL.QueryRow(`PRAGMA schema_version`).Scan(&afterCookie); err != nil {
		t.Fatal(err)
	}
	if beforeChanges != afterChanges || beforeData != afterData || cookie != afterCookie {
		t.Fatal("fixture did not reproduce the old counters' DDL blind spot")
	}
}

func TestReviewCertifiedOwnDDLCommitRejectsNoop(t *testing.T) {
	f, r := certifiedFixture(t)
	certifiedOwnDDL(t, f, false, false)
	_, ok, err := r.ConfirmCertifiedReviewNoChange(t.Context(), f.account.Account.HouseholdID, *f.now, func() time.Time { return *f.now }, f.s.reviewProviderSnapshot())
	if err != nil || ok {
		t.Fatal("same-connection DDL escaped continuity fence", ok, err)
	}
}

func TestReviewCertifiedOwnDDLCommitSelectsFullSourceRecheck(t *testing.T) {
	f, r := certifiedFixture(t)
	p := certifiedStage(t, f, r)
	certifiedOwnDDL(t, f, false, false)
	verdict := "missing"
	r = r.WithReviewCompletenessLineageShadow(func(o domain.ReviewCompletenessLineageObservation) {
		if o.Phase == "certified_publication" {
			verdict = o.Verdict
		}
	})
	if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
		t.Fatal(err)
	}
	if verdict != "full_source_recheck" {
		t.Fatal("DDL used counter-only source CAS", verdict)
	}
}

func TestReviewCertifiedOwnDDLDoesNotLaunderLostFrozenHistory(t *testing.T) {
	f, r := certifiedFixture(t)
	f.day(t, "2026-10-13")
	f.quantity(t, false)
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var resolved, events int
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_episodes WHERE resolved_at IS NOT NULL`).Scan(&resolved); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_episode_events`).Scan(&events); err != nil || resolved != 1 || events == 0 {
		t.Fatal("fixture lacks frozen history", resolved, events, err)
	}
	certifiedOwnDDL(t, f, true, false)
	p := certifiedStage(t, f, r)
	if p.Capture.Certified.MayCertify || p.Capture.Certified.MayBootstrap || p.Capture.Certified.MayReuse {
		t.Error("lost frozen event history was treated as an authentic unchanged projection")
	}
	if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
		t.Fatal(err)
	}
	assertNoCertifiedMAC(t, f)
}

func TestReviewCertifiedOwnDDLRollbackRetainsUnchangedAuthority(t *testing.T) {
	f, r := certifiedFixture(t)
	certifiedOwnDDL(t, f, false, true)
	requireCertifiedNoop(t, f, r)
}
