package application

import (
	"database/sql"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func unpublishedCertifiedFixture(t *testing.T) (seriesZeroFixture, *sqlite.Repository) {
	t.Helper()
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	f.day(t, "2026-10-12")
	saveSyntheticReviewRule(t, f, "Synthetic genesis", syntheticHistoricalReviewSpec())
	r := sqlite.NewRepository(f.db).WithCertifiedReviewCandidate()
	f.s = NewService(r)
	f.s.fxProviderKey = "synthetic-pinned"
	f.s.setClock(func() time.Time { return *f.now })
	return f, r
}

func eraseReviewNamespace(t *testing.T, f seriesZeroFixture) {
	t.Helper()
	tx, err := f.db.SQL.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, q := range []string{`DELETE FROM investment_review_evaluation_state`, `DELETE FROM investment_review_episode_events`, `DELETE FROM investment_review_episodes`, `DELETE FROM investment_review_daily_results`, `DELETE FROM investment_review_daily_inputs`, `DELETE FROM investment_review_position_epochs`, `DELETE FROM review_temporal_checkpoints`, `DELETE FROM investment_review_mutation_receipts`, `UPDATE investment_review_control SET input_generation=0,review_state_revision=0,projection_status='pending',published_on=NULL,published_input_fingerprint=NULL`} {
		if _, err := tx.ExecContext(t.Context(), q); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestReviewCertifiedGenesisRequiresPrivateLifecycleProof(t *testing.T) {
	for _, kind := range []string{"new", "reopened-unpublished", "external-commit", "pool-replaced", "restore-boundary", "c2-history-erased"} {
		t.Run(kind, func(t *testing.T) {
			f, r := unpublishedCertifiedFixture(t)
			switch kind {
			case "reopened-unpublished":
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
			case "external-commit":
				other, err := sqlite.Open(f.db.Path)
				if err != nil {
					t.Fatal(err)
				}
				defer other.Close()
				if _, err := other.SQL.Exec(`UPDATE investment_review_rules SET name=name||' externally changed'`); err != nil {
					t.Fatal(err)
				}
			case "pool-replaced":
				if err := f.db.SQL.Close(); err != nil {
					t.Fatal(err)
				}
				pool, err := sql.Open("sqlite", f.db.Path+"?_txlock=immediate&_pragma=foreign_keys%3d1&_pragma=recursive_triggers%3d1")
				if err != nil {
					t.Fatal(err)
				}
				pool.SetMaxOpenConns(1)
				f.db.SQL = pool
			case "restore-boundary":
				if err := f.db.ResetFinancialSourceTracking(t.Context()); err != nil {
					t.Fatal(err)
				}
			case "c2-history-erased":
				c2 := NewService(sqlite.NewRepository(f.db))
				c2.fxProviderKey = "synthetic-pinned"
				c2.setClock(func() time.Time { return *f.now })
				if _, err := c2.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
					t.Fatal(err)
				}
				var episode string
				var revision int
				if err := f.db.SQL.QueryRow(`SELECT id,revision FROM investment_review_episodes`).Scan(&episode, &revision); err != nil {
					t.Fatal(err)
				}
				if _, err := c2.AcknowledgeReviewEpisodeV2(t.Context(), episode, string(domain.NewMutationID()), revision); err != nil {
					t.Fatal(err)
				}
				eraseReviewNamespace(t, f)
			}
			p := certifiedStage(t, f, r)
			want := kind == "new"
			if p.Capture.Certified.InitialPublication != want || p.Capture.Certified.MayCertify != want || p.Capture.Certified.MayBootstrap {
				t.Fatal("wrong genesis eligibility", kind, p.Capture.Certified.InitialPublication, p.Capture.Certified.MayCertify)
			}
			if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
				t.Fatal(err)
			}
			if want {
				requireCertifiedNoop(t, f, r)
			} else {
				assertNoCertifiedMAC(t, f)
			}
		})
	}
}

func TestReviewCertifiedCapturedGenesisCannotSurvivePublicationAndErasure(t *testing.T) {
	f, r := unpublishedCertifiedFixture(t)
	p := certifiedStage(t, f, r)
	if !p.Capture.Certified.InitialPublication {
		t.Fatal("fixture did not capture genesis")
	}
	c2 := NewService(sqlite.NewRepository(f.db))
	c2.fxProviderKey = "synthetic-pinned"
	c2.setClock(func() time.Time { return *f.now })
	if _, err := c2.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	eraseReviewNamespace(t, f)
	if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
		t.Fatal(err)
	}
	assertNoCertifiedMAC(t, f)
}
