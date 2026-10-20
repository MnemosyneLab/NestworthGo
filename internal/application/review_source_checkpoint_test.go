package application

import (
	"errors"
	"github.com/waltwang/nestworth-go/internal/testsupport/reviewsource"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestReviewSourceCheckpointChecksPublicationAlongsideFullFingerprint(t *testing.T) {
	for _, scenario := range []string{"unchanged", "configuration", "quote", "untracked-quote"} {
		t.Run(scenario, func(t *testing.T) {
			f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
			f.day(t, "2026-10-03")
			saveSyntheticReviewRule(t, f, "Synthetic shadow", syntheticHistoricalReviewSpec())
			repo := sqlite.NewRepository(f.db)
			capture, err := repo.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
			if err != nil {
				t.Fatal(err)
			}
			if capture.SourceRevision == nil || !capture.SourceRevision.Valid() {
				t.Fatal("capture lacks complete source checkpoint")
			}
			var revision, floor int64
			var epoch, version string
			if err = f.db.SQL.QueryRow(`SELECT revision,journal_floor,tracking_epoch,tracking_version FROM financial_source_revision`).Scan(&revision, &floor, &epoch, &version); err != nil {
				t.Fatal(err)
			}
			if c := capture.SourceRevision; c.Token.Revision != revision || c.JournalFloor != floor || c.Token.Epoch != epoch || c.Token.TrackingVersion != version || c.Token.ResolverPolicyVersion != capture.Historical.History.ResolverPolicyVersion {
				t.Fatal("checkpoint differs from captured source metadata")
			}
			stage, err := PrepareReviewPublication(t.Context(), capture, *f.now, "")
			if err != nil {
				t.Fatal(err)
			}
			want := reviewsource.Same
			switch scenario {
			case "configuration":
				if err = sqlite.NewConfigurationRepository(f.db).SaveConfiguration("synthetic-shadow", []byte("synthetic-only")); err != nil {
					t.Fatal(err)
				}
				want = reviewsource.Conservative
			case "untracked-quote":
				if _, err = f.db.SQL.Exec(`DROP TRIGGER financial_source_instrument_quotes_update`); err != nil {
					t.Fatal(err)
				}
				fallthrough
			case "quote":
				if _, err = f.db.SQL.Exec(`UPDATE instrument_quotes SET unit_price='100.0001' WHERE instrument_id=?`, f.instrument.ID.String()); err != nil {
					t.Fatal(err)
				}
				want = reviewsource.Changed
				if scenario == "untracked-quote" {
					want = reviewsource.Untracked
				}
			}
			current, captureErr := repo.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
			if captureErr != nil {
				t.Fatal(captureErr)
			}
			observation := reviewsource.Compare(capture, current)
			err = repo.PublishReviewProjection(t.Context(), stage, *f.now)
			if observation.Verdict != want || !observation.SameCutoff {
				t.Fatalf("observation=%+v want=%s", observation, want)
			}
			if scenario == "quote" || scenario == "untracked-quote" || scenario == "configuration" {
				var conflict *domain.Error
				if !errors.As(err, &conflict) || conflict.Code != domain.ErrConflict {
					t.Fatalf("source checks allowed stale stage: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if scenario == "configuration" {
				if _, err = f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
					t.Fatalf("conservative conflict did not allow fresh full capture/publication: %v", err)
				}
			}
		})
	}
}

func TestReviewSourceCheckpointPublicationHoldsWriterReservationBeforeComparison(t *testing.T) {
	db, faults := openReviewFaultDatabase(t)
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1", db)
	f.day(t, "2026-10-03")
	writer, err := sqlite.Open(f.db.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if _, err = writer.SQL.Exec(`PRAGMA busy_timeout=0`); err != nil {
		t.Fatal(err)
	}
	observed := false
	repo := sqlite.NewRepository(f.db)
	checkReservation := func() {
		observed = true
		_, writeErr := writer.SQL.Exec(`UPDATE households SET name='Synthetic concurrent writer'`)
		var coded interface{ Code() int }
		if !errors.As(writeErr, &coded) || coded.Code()&255 != 5 {
			t.Fatalf("token/fingerprint comparison lacked write reservation: %v", writeErr)
		}
	}
	capture, err := repo.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := PrepareReviewPublication(t.Context(), capture, *f.now, "")
	if err != nil {
		t.Fatal(err)
	}
	faults.afterQuery = func(query string) {
		if strings.HasPrefix(query, "SELECT revision,journal_floor,tracking_epoch,tracking_version") {
			checkReservation()
		}
	}
	if err = repo.PublishReviewProjection(t.Context(), stage, *f.now); err != nil {
		t.Fatal(err)
	}
	faults.afterQuery = nil
	if !observed {
		t.Fatal("shadow comparison not observed")
	}
	if _, err = writer.SQL.Exec(`UPDATE households SET name='Synthetic after publication'`); err != nil {
		t.Fatal(err)
	}
	after, err := repo.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if after.SourceRevision.Token.Revision <= capture.SourceRevision.Token.Revision || after.HistoricalFingerprint == capture.HistoricalFingerprint {
		t.Fatal("committed external writer missing from next coherent capture")
	}
}

func TestReviewSourceCheckpointFutureActivationAndClockRollbackRemainIncomparable(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	f.day(t, "2026-10-03")
	at := f.now.Add(time.Hour)
	*f.now = at
	f.quantity(t, false)
	repo := sqlite.NewRepository(f.db)
	id := f.account.Account.HouseholdID
	before, err := repo.CaptureReviewProjection(t.Context(), id, at.Add(-time.Nanosecond))
	if err != nil {
		t.Fatal(err)
	}
	after, err := repo.CaptureReviewProjection(t.Context(), id, at)
	if err != nil {
		t.Fatal(err)
	}
	o := reviewsource.Compare(before, after)
	if o.Verdict != reviewsource.CutoffChanged || !o.SameToken || o.SameFingerprint {
		t.Fatalf("future activation treated as revision proof: %+v", o)
	}
	back, err := repo.CaptureReviewProjection(t.Context(), id, at.Add(-time.Nanosecond))
	if err != nil {
		t.Fatal(err)
	}
	o = reviewsource.Compare(after, back)
	if o.Verdict != reviewsource.ClockRollback || !o.SameToken || back.HistoricalFingerprint != before.HistoricalFingerprint {
		t.Fatalf("rollback reused current temporal result: %+v", o)
	}
	if _, err = f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestReviewSourceCheckpointFloorResetAndReopenNeverCertifyMissingHistory(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	f.day(t, "2026-10-03")
	repo := sqlite.NewRepository(f.db)
	id := f.account.Account.HouseholdID
	before, err := repo.CaptureReviewProjection(t.Context(), id, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.SQL.Exec(`UPDATE instrument_quotes SET unit_price='100.0001' WHERE instrument_id=?`, f.instrument.ID.String()); err != nil {
		t.Fatal(err)
	}
	var revision int64
	if err = f.db.SQL.QueryRow(`SELECT revision FROM financial_source_revision`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if err = f.db.PruneFinancialSourceChanges(t.Context(), id.String(), revision); err != nil {
		t.Fatal(err)
	}
	after, err := repo.CaptureReviewProjection(t.Context(), id, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if o := reviewsource.Compare(before, after); !o.BelowJournalFloor || o.Verdict != reviewsource.Changed {
		t.Fatalf("lost journal accepted: %+v", o)
	}
	if err = f.db.ResetFinancialSourceTracking(t.Context()); err != nil {
		t.Fatal(err)
	}
	reset, err := repo.CaptureReviewProjection(t.Context(), id, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if reset.SourceRevision.Token.Revision != after.SourceRevision.Token.Revision || reset.SourceRevision.Token.Epoch == after.SourceRevision.Token.Epoch {
		t.Fatal("restore reset did not invalidate token identity")
	}
	if o := reviewsource.Compare(after, reset); o.Verdict != reviewsource.Conservative || o.SameToken {
		t.Fatalf("new epoch reused old token: %+v", o)
	}
	reopened, err := sqlite.Open(f.db.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	stable, err := sqlite.NewRepository(reopened).CaptureReviewProjection(t.Context(), id, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if o := reviewsource.Compare(reset, stable); o.Verdict != reviewsource.Same {
		t.Fatalf("normal reopen changed checkpoint: %+v", o)
	}
	path := filepath.Join(t.TempDir(), "synthetic-shadow-snapshot.db")
	if err = f.db.SnapshotTo(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	readonly, err := sqlite.OpenReadOnlyForVerify(path)
	if err != nil {
		t.Fatal(err)
	}
	defer readonly.Close()
	copyCapture, err := sqlite.NewRepository(readonly).CaptureReviewProjection(t.Context(), id, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if o := reviewsource.Compare(stable, copyCapture); o.Verdict != reviewsource.Same {
		t.Fatalf("readonly complete snapshot changed checkpoint: %+v", o)
	}
}
