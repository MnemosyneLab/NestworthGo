package application

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestReviewCertifiedProviderLeaseAndStaleRead(t *testing.T) {
	f, r := certifiedFixture(t)
	f.day(t, "2026-10-13")
	p := certifiedStage(t, f, r)
	started, done := make(chan struct{}), make(chan error, 1)
	clock := func() time.Time {
		if f.s.reviewProviderMu.TryLock() {
			f.s.reviewProviderMu.Unlock()
			t.Error("provider lease was not held at the final fence")
		}
		go func() { close(started); done <- f.s.SetFXProvider("synthetic-other") }()
		<-started
		select {
		case err := <-done:
			t.Errorf("provider setter escaped publication: %v", err)
			done <- err
		default:
		}
		return *f.now
	}
	if err := certifiedPublish(t.Context(), f, r, p, clock); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	page, err := f.s.ListReviewCoverageV2(t.Context(), domain.ReviewCoverageFilter{}, "", 10)
	if err != nil || page.ProjectionStatus != "stale" {
		t.Fatal("provider change did not expire the old publication", page.ProjectionStatus, err)
	}
	got, err := f.s.ReconcileInvestmentReviewsV2(t.Context())
	if err != nil || got.DayCount != 12 {
		t.Fatal("provider change did not rebuild", got, err)
	}
	*f.now = f.now.Add(time.Nanosecond)
	got, err = f.s.ReconcileInvestmentReviewsV2(t.Context())
	if err != nil || got.DayCount != 0 {
		t.Fatal("provider rebuild failed certification", got, err)
	}
}

func TestReviewCertifiedProviderExhaustionAndDefault(t *testing.T) {
	for _, kind := range []string{"default", "exhausted", "registry"} {
		t.Run(kind, func(t *testing.T) {
			f, r := certifiedFixture(t)
			switch kind {
			case "default":
				f.s.reviewProviderMu.Lock()
				f.s.stateMu.Lock()
				f.s.fxProviderKey = ""
				f.s.stateMu.Unlock()
				f.s.reviewProviderMu.Unlock()
			case "exhausted":
				f.s.reviewProviderMu.Lock()
				f.s.reviewProviderRevision = ^uint64(0)
				f.s.reviewProviderMu.Unlock()
				if err := f.s.SetFXProvider("synthetic-pinned"); err != nil {
					t.Fatal(err)
				}
			case "registry":
				old := f.s.reviewProviderSnapshot()
				f.s.SetMarketDataRegistry(f.s.MarketDataRegistry())
				if err := f.s.withReviewProviderLease(old, func() error { return nil }); err == nil {
					t.Fatal("registry replacement retained old lease")
				}
				return
			}
			f.day(t, "2026-10-13")
			p := certifiedStage(t, f, r)
			if p.Capture.Certified.Provider.Pinned || p.CertifiedFullOracle != nil {
				t.Fatal("unfenced selection acquired bounded authority")
			}
			if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
				t.Fatal(err)
			}
			assertNoCertifiedMAC(t, f)
		})
	}
}

func TestReviewCertifiedLegacyRestartAndTrackingFallback(t *testing.T) {
	for _, kind := range []string{"legacy", "restart", "rollback", "epoch", "tracking-policy", "temporary-schema"} {
		t.Run(kind, func(t *testing.T) {
			var f seriesZeroFixture
			var r *sqlite.Repository
			if kind == "legacy" {
				f, _ = noChangeFixture(t)
				r = sqlite.NewRepository(f.db).WithCertifiedReviewCandidate()
				f.s = NewService(r)
				f.s.fxProviderKey = "synthetic-pinned"
				f.s.setClock(func() time.Time { return *f.now })
			} else {
				f, r = certifiedFixture(t)
			}
			switch kind {
			case "restart":
				db, err := sqlite.Open(f.db.Path)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				f.db = db
				r = sqlite.NewRepository(db).WithCertifiedReviewCandidate()
				f.s = NewService(r)
				f.s.fxProviderKey = "synthetic-pinned"
				f.s.setClock(func() time.Time { return *f.now })
			case "rollback":
				r.ObserveReviewClock(f.now.Add(time.Hour))
				*f.now = f.now.Add(30 * time.Minute)
			case "epoch":
				if err := f.db.ResetFinancialSourceTracking(t.Context()); err != nil {
					t.Fatal(err)
				}
			case "tracking-policy":
				if _, err := f.db.SQL.Exec(`UPDATE financial_source_revision SET tracking_version='future'`); err != nil {
					t.Fatal(err)
				}
			case "temporary-schema":
				if _, err := f.db.SQL.Exec(`CREATE TEMP TABLE synthetic_untrusted_scope(value TEXT)`); err != nil {
					t.Fatal(err)
				}
			}
			p := certifiedStage(t, f, r)
			if p.CertifiedFullOracle != nil || len(p.Capture.ReusableDailyEvidence) != 0 {
				t.Fatal("unsupported context used bounded publication")
			}
			if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
				t.Fatal(err)
			}
			if kind != "rollback" {
				assertNoCertifiedMAC(t, f)
			} else {
				*f.now = f.now.Add(time.Nanosecond)
				result, err := f.s.ReconcileInvestmentReviewsV2(t.Context())
				if err != nil || result.DayCount != 0 {
					t.Fatal("same-session full rollback recovery failed", result, err)
				}
			}
		})
	}
}

// Simulate a direct SQL writer rewinding an otherwise structurally valid C1
// revision/journal after DML. SQLite connection counters must still fence it.
func rewindCertifiedRevision(t *testing.T, writer *sql.DB, checkpoint *domain.ReviewSourceCheckpoint) {
	t.Helper()
	if _, err := writer.Exec(`UPDATE instrument_quotes SET unit_price='101'`); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(`DELETE FROM financial_source_changes WHERE household_id=? AND revision>?`, checkpoint.Token.HouseholdID.String(), checkpoint.Token.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(`UPDATE financial_source_revision SET revision=?,journal_floor=? WHERE household_id=?`, checkpoint.Token.Revision, checkpoint.JournalFloor, checkpoint.Token.HouseholdID.String()); err != nil {
		t.Fatal(err)
	}
}

func TestReviewCertifiedRewoundSourceMetadata(t *testing.T) {
	for _, connection := range []string{"same", "other"} {
		t.Run(connection, func(t *testing.T) {
			f, r := certifiedFixture(t)
			checkpoint, err := r.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
			if err != nil {
				t.Fatal(err)
			}
			writer := f.db.SQL
			if connection == "other" {
				writer, err = sql.Open("sqlite", f.db.Path+"?_txlock=immediate&_pragma=foreign_keys%3d1&_pragma=recursive_triggers%3d1")
				if err != nil {
					t.Fatal(err)
				}
				defer writer.Close()
			}
			rewindCertifiedRevision(t, writer, checkpoint.SourceRevision)
			page, err := f.s.ListReviewCoverageV2(t.Context(), domain.ReviewCoverageFilter{}, "", 10)
			if err != nil || page.ProjectionStatus != "stale" {
				t.Fatal("rewound tracking metadata hid a write", page.ProjectionStatus, err)
			}
			result, err := f.s.ReconcileInvestmentReviewsV2(t.Context())
			if err != nil || result.DayCount != 11 {
				t.Fatal("rewound token authorized no-op", result, err)
			}
			noChangeFullOracle(t, f)
			// Also cover a source edit between prepare and publish, with token equal.
			f.day(t, "2026-10-13")
			p := certifiedStage(t, f, r)
			if _, err := writer.Exec(`UPDATE instrument_quotes SET unit_price='102'`); err != nil {
				t.Fatal(err)
			}
			if _, err := writer.Exec(`DELETE FROM financial_source_changes WHERE household_id=? AND revision>?`, p.Capture.HouseholdID.String(), p.Capture.SourceRevision.Token.Revision); err != nil {
				t.Fatal(err)
			}
			if _, err := writer.Exec(`UPDATE financial_source_revision SET revision=? WHERE household_id=?`, p.Capture.SourceRevision.Token.Revision, p.Capture.HouseholdID.String()); err != nil {
				t.Fatal(err)
			}
			before := noChangeProjectionRows(t, f.db)
			if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err == nil {
				t.Fatal("source fingerprint fallback was bypassed")
			}
			if !reflect.DeepEqual(before, noChangeProjectionRows(t, f.db)) {
				t.Fatal("rejected rewound source wrote partial data")
			}
		})
	}
}

func TestReviewCertifiedCancelledPreparation(t *testing.T) {
	f, r := certifiedFixture(t)
	f.day(t, "2026-10-13")
	c, err := r.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	c.Certified.Provider = f.s.reviewProviderSnapshot()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := PrepareReviewPublication(ctx, c, *f.now, c.Certified.Provider.Key); err == nil {
		t.Fatal("cancelled candidate produced a stage")
	}
}

func TestReviewCertifiedHoldsSQLiteWriteReservation(t *testing.T) {
	for _, operation := range []string{"no-op", "publication"} {
		t.Run(operation, func(t *testing.T) {
			f, r := certifiedFixture(t)
			writer, err := sql.Open("sqlite", f.db.Path+"?_txlock=immediate&_pragma=busy_timeout%3d0&_pragma=foreign_keys%3d1&_pragma=recursive_triggers%3d1")
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Close()
			reached := false
			clock := func() time.Time {
				reached = true
				_, err := writer.Exec(`UPDATE instrument_quotes SET unit_price='999'`)
				var coded interface{ Code() int }
				if !errors.As(err, &coded) || coded.Code()&255 != 5 {
					t.Errorf("external writer escaped SQLite reservation: %v", err)
				}
				return *f.now
			}
			if operation == "publication" {
				f.day(t, "2026-10-13")
				p := certifiedStage(t, f, r)
				err = certifiedPublish(t.Context(), f, r, p, clock)
			} else {
				var ok bool
				_, ok, err = r.ConfirmCertifiedReviewNoChange(t.Context(), f.account.Account.HouseholdID, *f.now, clock, f.s.reviewProviderSnapshot())
				if !ok {
					t.Error("valid no-op was not confirmed")
				}
			}
			if err != nil || !reached {
				t.Fatal("reservation probe failed", reached, err)
			}
		})
	}
}

func TestReviewCertifiedRolledBackWritesInvalidateConnectionProof(t *testing.T) {
	f, r := certifiedFixture(t)
	f.day(t, "2026-10-13")
	p := certifiedStage(t, f, r)
	reference := windowOracleReference(t, f)
	tx, err := f.db.SQL.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(t.Context(), `UPDATE instrument_quotes SET unit_price='999'`); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var verdict string
	r = r.WithReviewCompletenessLineageShadow(func(o domain.ReviewCompletenessLineageObservation) {
		if o.Phase == "certified_publication" {
			verdict = o.Verdict
		}
	})
	if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
		t.Fatal(err)
	}
	if verdict != "full_source_recheck" {
		t.Fatal("rolled-back DML retained counter authority", verdict)
	}
	if !reflect.DeepEqual(noChangeProjectionRows(t, f.db), noChangeProjectionRows(t, reference)) {
		t.Fatal("rollback fallback differs from pre-publication full oracle")
	}
}
