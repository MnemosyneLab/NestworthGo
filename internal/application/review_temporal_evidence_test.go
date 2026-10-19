package application

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// The identical test-only harness also runs against C2a. Financial IDs/facts
// come from one immutable schema20 synthetic snapshot, copied to separate paths
// before either binary opens it. C2b may perform its normal additive upgrade.
func TestReviewTemporalPerformanceEvidence(t *testing.T) {
	path := os.Getenv("NW_TEMPORAL_EVIDENCE_DB")
	if path == "" {
		t.Skip("opt-in temporal distribution evidence")
	}
	if filepath.Base(path) != "synthetic-temporal-evidence.db" {
		t.Fatal("explicit synthetic evidence filename required")
	}
	config := reviewPerfConfig{Accounts: 1, Instruments: 1, Holdings: 1, Rules: 4, HistoryDays: 30, Quotes: 30}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		seed := newReviewPerfFixture(t, config)
		if err := seed.db.SnapshotTo(t.Context(), path); err != nil {
			t.Fatal(err)
		}
	}
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := sqlite.NewRepository(db)
	household, err := repo.Household(t.Context())
	if err != nil || household == nil || household.Name != "Synthetic review pipeline" {
		t.Fatal("synthetic source marker required")
	}
	origin, err := repo.HistoryOrigin(t.Context(), household.ID)
	if err != nil || origin == nil {
		t.Fatal("synthetic history required")
	}
	instruments, err := repo.ListInstruments(t.Context(), household.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	now := origin.StartedAt.AddDate(0, 0, 30)
	service := NewService(repo)
	service.setClock(func() time.Time { return now })
	source := reviewFullPipelineFixture{service: service, db: db, now: &now, householdID: household.ID, dbPath: path, start: origin.StartedAt, instruments: instruments}
	logReviewPerfManifest(t, source, config)
	if _, err := service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	for sample := 1; sample <= 30; sample++ {
		for _, scenario := range []string{"no-op", "activation", "append", "correction"} {
			t.Run(scenario+"-"+strconv.Itoa(sample), func(t *testing.T) {
				run := cloneReviewPerfFixture(t, source)
				at := run.now.Add(time.Hour)
				if scenario == "activation" {
					seedReviewTemporalEvidenceQuantity(t, run, at)
					*run.now = at.Add(-time.Nanosecond)
				}
				// Establish a warm publication in this same DB/process session;
				// restart invalidation and initial full work are outside this timer.
				if _, err := run.service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
					t.Fatal(err)
				}
				switch scenario {
				case "activation":
					*run.now = at
				case "append":
					*run.now = run.now.AddDate(0, 0, 1)
					if err := appendReviewPerfDailyMarketData(t.Context(), run, run.now.AddDate(0, 0, -1)); err != nil {
						t.Fatal(err)
					}
				case "correction":
					if err := correctReviewPerfHistoricalQuote(t.Context(), run, 0); err != nil {
						t.Fatal(err)
					}
				}
				var before, after runtime.MemStats
				runtime.ReadMemStats(&before)
				started := time.Now()
				result, err := run.service.ReconcileInvestmentReviewsV2(t.Context())
				elapsed := time.Since(started)
				runtime.ReadMemStats(&after)
				if err != nil {
					t.Fatal(err)
				}
				capture, err := sqlite.NewRepository(run.db).CaptureReviewProjection(t.Context(), run.householdID, *run.now)
				if err != nil {
					t.Fatal(err)
				}
				publication, err := PrepareReviewPublication(t.Context(), capture, *run.now, run.service.FXProviderKey())
				if err != nil {
					t.Fatal(err)
				}
				assertReviewFullOracle(t, run, publication) // Outside the timed call.
				t.Logf("temporal scenario=%s sample=%d elapsed_ns=%d alloc_bytes=%d allocations=%d days=%d fingerprint=%s", scenario, sample, elapsed.Nanoseconds(), after.TotalAlloc-before.TotalAlloc, after.Mallocs-before.Mallocs, result.DayCount, result.SourceFingerprint)
			})
		}
	}
}

// Exact same future fact IDs/values on both versions and every independent
// clone. This is untimed synthetic seeding; public-command lifecycle coverage
// is kept separately in the temporal correctness tests.
func seedReviewTemporalEvidenceQuantity(t *testing.T, f reviewFullPipelineFixture, at time.Time) {
	t.Helper()
	var holding, instrument, quantity string
	if err := f.db.SQL.QueryRow(`SELECT id,instrument_id,quantity FROM holdings ORDER BY id LIMIT 1`).Scan(&holding, &instrument, &quantity); err != nil {
		t.Fatal(err)
	}
	stamp := at.UTC().Format("2006-01-02T15:04:05.000Z")
	const activity = "00000000-0000-4000-801a-000000000001"
	const effect = "00000000-0000-4000-801a-000000000002"
	const value = "00000000-0000-4000-801a-000000000003"
	if err := f.db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO activities(id,household_id,kind,reason,effective_at,effective_local_date,created_at) VALUES(?,?,'value_update','other',?,?,?)`, activity, f.householdID.String(), stamp, at.UTC().Format(time.DateOnly), stamp); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO activity_effects(id,activity_id,sequence,role,direction,target,classification,holding_id,instrument_id,quantity,cost_unit_price) VALUES(?,?,1,'quantity','removed','holding_quantity','adjustment',?,?,?,'10')`, effect, activity, holding, instrument, quantity); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO holding_quantity_values(id,holding_id,quantity,effective_at,created_at,activity_effect_id,projection_kind) VALUES(?,?,'0',?,?,?,'event')`, value, holding, stamp, stamp, effect); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE holdings SET quantity='0',updated_at=? WHERE id=?`, stamp, holding)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
}
