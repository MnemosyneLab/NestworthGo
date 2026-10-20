package application

import (
	"context"
	"database/sql/driver"
	"errors"
	"github.com/waltwang/nestworth-go/internal/testsupport/reviewsource"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	modernsqlite "modernc.org/sqlite"
)

type reviewStaticOnlyProjectionPort struct {
	Repository
	delegate               *sqlite.Repository
	captures, publications int
}

func (p *reviewStaticOnlyProjectionPort) CaptureReviewProjection(ctx context.Context, id domain.HouseholdID, at time.Time) (domain.ReviewProjectionCapture, error) {
	p.captures++
	return p.delegate.CaptureReviewProjection(ctx, id, at)
}

func (p *reviewStaticOnlyProjectionPort) PublishReviewProjection(ctx context.Context, publication domain.ReviewPublication, at time.Time) error {
	p.publications++
	return p.delegate.PublishReviewProjection(ctx, publication, at)
}

func TestReviewTemporalRequiresFinalClockAwarePublicationPort(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	f.day(t, "2026-10-03")
	repo := sqlite.NewRepository(f.db)
	port := &reviewStaticOnlyProjectionPort{Repository: repo, delegate: repo}
	service := NewService(port)
	service.setClock(func() time.Time { return *f.now })
	_, err := service.ReconcileInvestmentReviewsV2(t.Context())
	var unavailable *domain.Error
	if !errors.As(err, &unavailable) || unavailable.Code != domain.ErrUnavailable || port.captures != 0 || port.publications != 0 {
		t.Fatal("service fell back to a time sampled before publication rechecks")
	}
}

func TestReviewTemporalCrashPublicationChild(t *testing.T) {
	if os.Getenv("NW_TEMPORAL_CRASH_CHILD") != "1" {
		t.Skip("subprocess crash helper")
	}
	path := os.Getenv("NW_TEMPORAL_CRASH_DB")
	if filepath.Base(path) != "synthetic-temporal-crash.db" {
		t.Fatal("isolated synthetic path required")
	}
	if err := modernsqlite.RegisterScalarFunction("synthetic_temporal_crash", 0, func(*modernsqlite.FunctionContext, []driver.Value) (driver.Value, error) {
		os.Exit(93)
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var id domain.HouseholdID
	var marker string
	if err := db.SQL.QueryRow(`SELECT id,name FROM households WHERE singleton_key=1`).Scan(&id, &marker); err != nil || marker != "Synthetic temporal crash" {
		t.Fatal("synthetic marker required")
	}
	if _, err := db.SQL.Exec(`CREATE TRIGGER synthetic_temporal_crash AFTER INSERT ON review_temporal_checkpoints BEGIN SELECT synthetic_temporal_crash(); END`); err != nil {
		t.Fatal(err)
	}
	now, err := time.Parse(time.RFC3339Nano, os.Getenv("NW_TEMPORAL_CRASH_AT"))
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db)
	capture, err := repo.CaptureReviewProjection(t.Context(), id, now)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := PrepareReviewPublication(t.Context(), capture, now, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.PublishReviewProjection(t.Context(), stage, now); err != nil {
		t.Fatal(err)
	}
	t.Fatal("crash trigger was not reached")
}

func TestReviewTemporalCrashBeforeCommitRollsBackCompletePublication(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	f.day(t, "2026-10-03")
	saveSyntheticReviewRule(t, f, "Synthetic crash rule", syntheticHistoricalReviewSpec())
	if _, err := f.db.SQL.Exec(`UPDATE households SET name='Synthetic temporal crash'`); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "synthetic-temporal-crash.db")
	if err := f.db.SnapshotTo(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestReviewTemporalCrashPublicationChild$", "-test.count=1", "-test.timeout=1m")
	command.Env = append(os.Environ(), "NW_TEMPORAL_CRASH_CHILD=1", "NW_TEMPORAL_CRASH_DB="+path, "NW_TEMPORAL_CRASH_AT="+f.now.Format(time.RFC3339Nano))
	output, err := command.CombinedOutput()
	var exited *exec.ExitError
	if !errors.As(err, &exited) || exited.ExitCode() != 93 {
		t.Fatalf("child did not crash at final checkpoint write: %v\n%s", err, output)
	}
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range []string{"investment_review_daily_inputs", "investment_review_daily_results", "investment_review_position_epochs", "investment_review_evaluation_state", "investment_review_episodes", "investment_review_episode_events", "review_temporal_checkpoints"} {
		var count int
		if err := db.SQL.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("crash committed partial %s: %d %v", table, count, err)
		}
	}
	if _, err := db.SQL.Exec(`DROP TRIGGER synthetic_temporal_crash`); err != nil {
		t.Fatal(err)
	}
	run := f
	run.db = db
	run.s = NewService(sqlite.NewRepository(db))
	run.s.setClock(func() time.Time { return *run.now })
	capture := temporalCapture(t, run)
	if !capture.TemporalFullRecompute {
		t.Fatal("crash restart reused absent certificate")
	}
	temporalPublicationOracle(t, run, capture)
	if _, err := run.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := db.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestReviewTemporalMetadataFailureIsAtomicAndUnknownPolicyUsesOracle(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	f.day(t, "2026-10-03")
	saveSyntheticReviewRule(t, f, "Synthetic checkpoint failure", syntheticHistoricalReviewSpec())
	if _, err := f.db.SQL.Exec(`CREATE TRIGGER synthetic_temporal_failure BEFORE INSERT ON review_temporal_checkpoints BEGIN SELECT RAISE(ABORT,'synthetic checkpoint failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err == nil {
		t.Fatal("checkpoint failure committed publication")
	}
	for _, table := range []string{"investment_review_daily_results", "investment_review_episodes", "investment_review_episode_events", "review_temporal_checkpoints"} {
		var count int
		if err := f.db.SQL.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("checkpoint failure leaked %s: %d %v", table, count, err)
		}
	}
	if _, err := f.db.SQL.Exec(`DROP TRIGGER synthetic_temporal_failure`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SQL.Exec(`UPDATE review_temporal_checkpoints SET certificate_json=json_set(certificate_json,'$.PolicyVersion','synthetic-unknown-policy')`); err != nil {
		t.Fatal(err)
	}
	capture := temporalCapture(t, f)
	if !capture.TemporalFullRecompute || len(capture.ReusableDailyEvidence) != 0 || capture.PublishedTemporal.Eligible(*f.now, capture.Temporal) {
		t.Fatal("unknown policy reused certificate/history")
	}
	temporalPublicationOracle(t, f, capture)
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestReviewTemporalFutureSourceEditsRetainFullOracleFallback(t *testing.T) {
	for _, mode := range []string{"unknown-source", "untracked-future"} {
		t.Run(mode, func(t *testing.T) {
			f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
			f.day(t, "2026-10-03")
			saveSyntheticReviewRule(t, f, "Synthetic unknown namespace", syntheticHistoricalReviewSpec())
			if mode == "unknown-source" {
				if _, err := f.db.SQL.Exec(`UPDATE history_snapshot_state SET resolver_policy_version=''`); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := f.db.SQL.Exec(`DROP TRIGGER financial_source_instrument_quotes_insert`); err != nil {
					t.Fatal(err)
				}
			}
			capture := temporalCapture(t, f)
			if mode == "unknown-source" && (capture.SourceRevision != nil || capture.Temporal.Source != nil) {
				t.Fatal("unknown resolver certified")
			}
			stage := temporalPublicationOracle(t, f, capture)
			at := f.now.Add(time.Hour)
			// Quote capture is already uncut on latest main. Even a future edit
			// remains detectable by the retained full fingerprint when its source
			// checkpoint is unavailable or a trigger is deliberately damaged.
			if _, err := f.db.SQL.Exec(`INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at,delayed) VALUES(?,?,'100','CNY','manual','manual',?,?,0)`, domain.NewInstrumentQuoteID().String(), f.instrument.ID.String(), at.Format("2006-01-02T15:04:05.000Z"), at.Format("2006-01-02T15:04:05.000Z")); err != nil {
				t.Fatal(err)
			}
			check := temporalCapture(t, f)
			if check.HistoricalFingerprint == capture.HistoricalFingerprint || mode == "untracked-future" && check.SourceRevision.Token != capture.SourceRevision.Token {
				t.Fatal("future-source edit escaped full oracle or damaged-trigger setup")
			}
			*f.now = at
			observation := reviewsource.Compare(capture, check)
			repo := sqlite.NewRepository(f.db)
			temporalConflict(t, repo.PublishReviewProjectionWithClock(t.Context(), stage, func() time.Time { return *f.now }))
			if mode == "untracked-future" && observation.Verdict != reviewsource.Untracked {
				t.Fatal("damaged future quote trigger was not exposed by full oracle")
			}
		})
	}
}

func TestReviewTemporalLogicalAsOfDoesNotExtendValidityPastCommit(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	f.day(t, "2026-10-03")
	at := f.now.Add(time.Hour)
	*f.now = at
	f.quantity(t, false)
	before := at.Add(-time.Nanosecond)
	*f.now = before
	saveSyntheticReviewRule(t, f, "Synthetic as-of", syntheticHistoricalReviewSpec())
	capture := temporalCapture(t, f)
	stage := temporalPublicationOracle(t, f, capture)
	if err := sqlite.NewRepository(f.db).PublishReviewProjectionWithClock(t.Context(), stage, func() time.Time { *f.now = at; return before }); err != nil {
		t.Fatal(err)
	}
	var evaluated string
	if err := f.db.SQL.QueryRow(`SELECT evaluated_at FROM review_temporal_checkpoints`).Scan(&evaluated); err != nil || evaluated != before.Format(time.RFC3339Nano) {
		t.Fatal("final logical as-of was not saved exactly")
	}
	page, err := f.s.ListReviewAlertGroupsV2(t.Context(), domain.ReviewAlertFilter{}, "", 10)
	if err != nil || page.ProjectionStatus != "stale" {
		t.Fatal("post-check clock advancement extended certificate validity")
	}
}

func TestReviewTemporalPreviewObservationInvalidatesRollbackAbovePublication(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	f.day(t, "2026-10-03")
	spec := syntheticHistoricalReviewSpec()
	saveSyntheticReviewRule(t, f, "Synthetic preview clock", spec)
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	published := *f.now
	*f.now = published.Add(time.Hour)
	if _, err := f.s.PreviewReviewRuleV2(t.Context(), spec, f.instrument.ID.String(), "", ""); err != nil {
		t.Fatal(err)
	}
	*f.now = published.Add(30 * time.Minute)
	capture := temporalCapture(t, f)
	if !capture.TemporalFullRecompute || len(capture.ReusableDailyEvidence) != 0 || capture.PublishedTemporal.Eligible(*f.now, capture.Temporal) {
		t.Fatal("preview-observed rollback above evaluated_at reused certificate")
	}
	temporalPublicationOracle(t, f, capture)
}
