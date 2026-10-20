package application

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestReviewSourceShadowChecksPublicationAlongsideFullFingerprint(t *testing.T) {
	for _, scenario := range []string{"unchanged", "configuration", "quote", "untracked-quote"} {
		t.Run(scenario, func(t *testing.T) {
			f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
			f.day(t, "2026-10-03")
			saveSyntheticReviewRule(t, f, "Synthetic shadow", syntheticHistoricalReviewSpec())
			var observations []domain.ReviewSourceShadowObservation
			repo := sqlite.NewRepository(f.db).WithReviewSourceShadowObserver(func(o domain.ReviewSourceShadowObservation) { observations = append(observations, o) })
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
			want := domain.ReviewSourceSame
			switch scenario {
			case "configuration":
				if err = sqlite.NewConfigurationRepository(f.db).SaveConfiguration("synthetic-shadow", []byte("synthetic-only")); err != nil {
					t.Fatal(err)
				}
				want = domain.ReviewSourceConservative
			case "untracked-quote":
				if _, err = f.db.SQL.Exec(`DROP TRIGGER financial_source_instrument_quotes_update`); err != nil {
					t.Fatal(err)
				}
				fallthrough
			case "quote":
				if _, err = f.db.SQL.Exec(`UPDATE instrument_quotes SET unit_price='100.0001' WHERE instrument_id=?`, f.instrument.ID.String()); err != nil {
					t.Fatal(err)
				}
				want = domain.ReviewSourceChanged
				if scenario == "untracked-quote" {
					want = domain.ReviewSourceUntracked
				}
			}
			err = repo.PublishReviewProjection(t.Context(), stage, *f.now)
			if len(observations) != 1 || observations[0].Verdict != want || !observations[0].SameCutoff {
				t.Fatalf("observation=%+v want=%s", observations, want)
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

func TestReviewSourceShadowPublicationHoldsWriterReservationBeforeComparison(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
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
	repo := sqlite.NewRepository(f.db).WithReviewSourceShadowObserver(func(o domain.ReviewSourceShadowObservation) {
		observed = true
		_, writeErr := writer.SQL.Exec(`UPDATE households SET name='Synthetic concurrent writer'`)
		var coded interface{ Code() int }
		if !errors.As(writeErr, &coded) || coded.Code()&255 != 5 {
			t.Fatalf("token/fingerprint comparison lacked write reservation: %v", writeErr)
		}
		if o.Verdict != domain.ReviewSourceSame {
			t.Fatalf("unchanged source: %+v", o)
		}
	})
	capture, err := repo.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := PrepareReviewPublication(t.Context(), capture, *f.now, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.PublishReviewProjection(t.Context(), stage, *f.now); err != nil {
		t.Fatal(err)
	}
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

func TestReviewSourceShadowFutureActivationAndClockRollbackRemainIncomparable(t *testing.T) {
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
	o := domain.CompareReviewSourceCheckpoints(before, after)
	if o.Verdict != domain.ReviewSourceCutoffChanged || !o.SameToken || o.SameFingerprint {
		t.Fatalf("future activation treated as revision proof: %+v", o)
	}
	back, err := repo.CaptureReviewProjection(t.Context(), id, at.Add(-time.Nanosecond))
	if err != nil {
		t.Fatal(err)
	}
	o = domain.CompareReviewSourceCheckpoints(after, back)
	if o.Verdict != domain.ReviewSourceClockRollback || !o.SameToken || back.HistoricalFingerprint != before.HistoricalFingerprint {
		t.Fatalf("rollback reused current temporal result: %+v", o)
	}
	if _, err = f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestReviewSourceShadowFloorResetAndReopenNeverCertifyMissingHistory(t *testing.T) {
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
	if o := domain.CompareReviewSourceCheckpoints(before, after); !o.BelowJournalFloor || o.Verdict != domain.ReviewSourceChanged {
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
	if o := domain.CompareReviewSourceCheckpoints(after, reset); o.Verdict != domain.ReviewSourceConservative || o.SameToken {
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
	if o := domain.CompareReviewSourceCheckpoints(reset, stable); o.Verdict != domain.ReviewSourceSame {
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
	if o := domain.CompareReviewSourceCheckpoints(stable, copyCapture); o.Verdict != domain.ReviewSourceSame {
		t.Fatalf("readonly complete snapshot changed checkpoint: %+v", o)
	}
}
