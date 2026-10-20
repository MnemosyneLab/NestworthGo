package application

import (
	"context"
	"database/sql/driver"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type certifiedConfirmationCounts struct {
	*sqlite.Repository
	confirmations, captures, publications int
}

func (r *certifiedConfirmationCounts) ConfirmCertifiedReviewNoChange(ctx context.Context, id domain.HouseholdID, now time.Time, clock func() time.Time, selection domain.ReviewProviderSelection) (domain.ReviewNoChangeResult, bool, error) {
	r.confirmations++
	return r.Repository.ConfirmCertifiedReviewNoChange(ctx, id, now, clock, selection)
}
func (r *certifiedConfirmationCounts) CaptureReviewProjection(ctx context.Context, id domain.HouseholdID, now time.Time) (domain.ReviewProjectionCapture, error) {
	r.captures++
	return r.Repository.CaptureReviewProjection(ctx, id, now)
}
func (r *certifiedConfirmationCounts) PublishCertifiedReviewProjection(ctx context.Context, p domain.ReviewPublication, clock func() time.Time, guard func(func() error) error) error {
	r.publications++
	return r.Repository.PublishCertifiedReviewProjection(ctx, p, clock, guard)
}

// Driver faults exercise the real authenticated no-change CAS with the exact
// compiled schema intact. A synthetic trigger would disable that authority and
// accidentally test full fallback instead of the intended confirmation path.
func TestReviewCertifiedCASFailureAndBoundedRetry(t *testing.T) {
	for _, scenario := range []string{"control-ignore", "checkpoint-ignore", "checkpoint-abort"} {
		t.Run(scenario, func(t *testing.T) {
			db, faults := openReviewFaultDatabase(t)
			f, repo := certifiedPublicationFixture(t, false, syntheticHistoricalReviewSpec(), "1", false, db)
			counts := &certifiedConfirmationCounts{Repository: repo}
			f.s.repository = counts
			before := noChangeProjectionRows(t, db)
			var certificate string
			if err := db.SQL.QueryRow(`SELECT certificate_json FROM review_temporal_checkpoints`).Scan(&certificate); err != nil {
				t.Fatal(err)
			}
			injected := 0
			faults.beforeExec = func(query string, _ []driver.NamedValue) (driver.Result, error, bool) {
				target := "UPDATE review_temporal_checkpoints SET evaluated_at="
				if scenario == "control-ignore" {
					target = "UPDATE investment_review_control SET input_generation=input_generation"
				}
				if !strings.HasPrefix(query, target) {
					return nil, nil, false
				}
				injected++
				if scenario == "checkpoint-abort" {
					return nil, errors.New("synthetic checkpoint failure"), true
				}
				return driver.RowsAffected(0), nil, true
			}
			*f.now = f.now.Add(time.Nanosecond)
			_, err := f.s.ReconcileInvestmentReviewsV2(t.Context())
			if err == nil {
				t.Fatal("failed CAS accepted")
			}
			attempts := 3
			if scenario == "checkpoint-abort" {
				attempts = 1
			}
			// A checkpoint fault follows the control self-update. SQLite's own
			// change counter advances even on rollback, so later attempts must
			// discard counter-only confirmation and run complete preparation.
			expectedFallbacks := 0
			if scenario == "checkpoint-ignore" {
				expectedFallbacks = 2
			}
			if injected != attempts || counts.confirmations != attempts || counts.captures != expectedFallbacks || counts.publications != expectedFallbacks {
				t.Fatalf("wrong confirmation/retry path: injected=%d confirmations=%d captures=%d publications=%d", injected, counts.confirmations, counts.captures, counts.publications)
			}
			var after string
			if err := db.SQL.QueryRow(`SELECT certificate_json FROM review_temporal_checkpoints`).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if after != certificate || !reflect.DeepEqual(before, noChangeProjectionRows(t, db)) {
				t.Fatal("failed CAS partially committed")
			}
			faults.beforeExec = nil
			recovered, err := f.s.ReconcileInvestmentReviewsV2(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if scenario != "control-ignore" && recovered.DayCount == 0 {
				t.Fatal("rolled-back DML did not require full recovery")
			}
			noChangeFullOracle(t, f)
			requireCertifiedNoop(t, f, repo)
		})
	}
}
