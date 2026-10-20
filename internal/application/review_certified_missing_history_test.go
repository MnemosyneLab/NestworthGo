package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestReviewCertifiedDeletedHistoryCannotBootstrap(t *testing.T) {
	for _, kind := range []string{"open-ack", "resolved-ack", "empty-namespace-with-ack-receipt"} {
		t.Run(kind, func(t *testing.T) {
			f, _ := certifiedFixture(t)
			var episode string
			var revision int
			if err := f.db.SQL.QueryRow(`SELECT id,revision FROM investment_review_episodes WHERE resolved_at IS NULL`).Scan(&episode, &revision); err != nil {
				t.Fatal(err)
			}
			mutation := string(domain.NewMutationID())
			if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), episode, mutation, revision); err != nil {
				t.Fatal(err)
			}
			if kind == "resolved-ack" {
				f.day(t, "2026-10-13")
				f.quantity(t, false)
			}
			if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
				t.Fatal(err)
			}
			// Save the independent historical expectation BEFORE corruption.
			// Two full captures after deletion would share the same missing input.
			var ack, receipt, checkpoint string
			if err := f.db.SQL.QueryRow(`SELECT acknowledged_at FROM investment_review_episodes WHERE id=?`, episode).Scan(&ack); err != nil || ack == "" {
				t.Fatal("fixture lacks acknowledgement", err)
			}
			if err := f.db.SQL.QueryRow(`SELECT result_json FROM investment_review_mutation_receipts WHERE mutation_id=?`, mutation).Scan(&receipt); err != nil {
				t.Fatal(err)
			}
			if err := f.db.SQL.QueryRow(`SELECT certificate_json FROM review_temporal_checkpoints`).Scan(&checkpoint); err != nil {
				t.Fatal(err)
			}
			var published domain.ReviewTemporalCertificate
			if err := published.UnmarshalJSON([]byte(checkpoint)); err != nil || len(published.NoChangeProjectionCounts) != 6 || published.NoChangeProjectionCounts[4] != 1 || published.NoChangeProjectionCounts[5] < 2 {
				t.Fatal("checkpoint does not record prior lifecycle history", err)
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
			tx, err := db.SQL.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			statements := []string{`UPDATE investment_review_evaluation_state SET episode_id=NULL`, `DELETE FROM investment_review_episode_events`, `DELETE FROM investment_review_episodes`}
			if kind == "empty-namespace-with-ack-receipt" {
				statements = append(statements, `DELETE FROM investment_review_evaluation_state`, `DELETE FROM investment_review_daily_results`, `DELETE FROM investment_review_daily_inputs`, `DELETE FROM investment_review_position_epochs`, `DELETE FROM review_temporal_checkpoints`, `UPDATE investment_review_control SET input_generation=0,review_state_revision=0,projection_status='pending',published_on=NULL,published_input_fingerprint=NULL`)
			}
			for _, q := range statements {
				if _, err := tx.ExecContext(t.Context(), q); err != nil {
					tx.Rollback()
					t.Fatal(err)
				}
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			var retainedReceipt, retainedCheckpoint string
			if err := db.SQL.QueryRow(`SELECT result_json FROM investment_review_mutation_receipts WHERE mutation_id=?`, mutation).Scan(&retainedReceipt); err != nil || receipt != retainedReceipt {
				t.Fatal("fixture changed the independent acknowledgement receipt", err)
			}
			if kind != "empty-namespace-with-ack-receipt" {
				if err := db.SQL.QueryRow(`SELECT certificate_json FROM review_temporal_checkpoints`).Scan(&retainedCheckpoint); err != nil || checkpoint != retainedCheckpoint {
					t.Fatal("fixture changed prior checkpoint evidence", err)
				}
			}
			p := certifiedStage(t, f, r)
			if p.Capture.Certified.MayCertify {
				t.Error("deleted lifecycle/ack history acquired bootstrap eligibility")
			}
			if kind != "resolved-ack" {
				current := p.Rules[0].CurrentEpisode
				if current == nil || current.ID == episode || current.AcknowledgedAt != nil {
					t.Fatal("fixture no longer exposes replay's loss of prior acknowledgement")
				}
				t.Logf("pre-corruption acknowledged identity %s at %s differs from newly planned unacknowledged identity %s", episode, ack, current.ID)
			}
			if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
				t.Fatal(err)
			}
			assertNoCertifiedMAC(t, f)
			_, ok, err := r.ConfirmCertifiedReviewNoChange(t.Context(), f.account.Account.HouseholdID, *f.now, func() time.Time { return *f.now }, f.s.reviewProviderSnapshot())
			if err != nil || ok {
				t.Fatal("lost history became a certified no-op", ok, err)
			}
		})
	}
}
