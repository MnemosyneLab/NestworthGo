//go:build perf

package sqlite

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestFinancialSourceWriteAmplificationEvidence(t *testing.T) {
	source, err := Open(filepath.Join(t.TempDir(), "synthetic-source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if _, err = source.SQL.Exec(`INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000001','Synthetic','CNY','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z'); INSERT INTO instruments(id,household_id,name,instrument_type,quote_currency,icon_key,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000002','00000000-0000-4000-8000-000000000001','Synthetic','stock','CNY','chart-line','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z'); INSERT INTO history_origins(id,household_id,timezone,started_at,created_at) VALUES('00000000-0000-4000-8000-000000000003','00000000-0000-4000-8000-000000000001','UTC','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z');`); err != nil {
		t.Fatal(err)
	}
	if _, err = source.SQL.Exec(`INSERT INTO history_snapshot_state(household_id,updated_at,resolver_policy_version) VALUES('00000000-0000-4000-8000-000000000001','2026-10-01T00:00:00Z',?)`, domain.MarketDataResolverPolicy); err != nil {
		t.Fatal(err)
	}
	var version string
	source.SQL.QueryRow(`SELECT sqlite_version()`).Scan(&version)
	t.Logf("synthetic primitive-write evidence n=30 each path/scenario; Go=%s SQLite=%s GOMAXPROCS=%d schema=%d; 1 household, 1 instrument, 100 writes/transaction; alternating order, immutable source clones; all setup/checkpoints/oracles excluded", runtime.Version(), version, runtime.GOMAXPROCS(0), CurrentSchemaVersion)
	for _, scenario := range []string{"quotes-100", "catalog-100"} {
		t.Run(scenario, func(t *testing.T) {
			durations := [2][]time.Duration{}
			for sample := 0; sample < 30; sample++ {
				t.Run(fmt.Sprintf("sample-%02d", sample), func(t *testing.T) {
					fingerprints := [2]string{}
					for offset := 0; offset < 2; offset++ {
						mode := (sample + offset) % 2
						t.Run(fmt.Sprintf("path-%d", mode), func(t *testing.T) {
							path := filepath.Join(t.TempDir(), "synthetic-clone.db")
							if _, err := source.SQL.Exec(`VACUUM INTO ?`, path); err != nil {
								t.Fatal(err)
							}
							db, err := Open(path)
							if err != nil {
								t.Fatal(err)
							}
							defer db.Close()
							// Isolated counterfactual: disable only C1 on this clone. All existing
							// reconciliation/schema/financial logic remains. Never used in production.
							if mode == 0 {
								for name := range financialTriggerStatements() {
									if _, err = db.SQL.Exec("DROP TRIGGER " + name); err != nil {
										t.Fatal(err)
									}
								}
							}
							if err = db.CheckpointWAL(t.Context()); err != nil {
								t.Fatal(err)
							}
							var before, after int64
							if err = db.SQL.QueryRow(`SELECT total_changes()`).Scan(&before); err != nil {
								t.Fatal(err)
							}
							var memBefore, memAfter runtime.MemStats
							runtime.ReadMemStats(&memBefore)
							started := time.Now()
							tx, err := db.SQL.BeginTx(t.Context(), nil)
							if err != nil {
								t.Fatal(err)
							}
							for i := 0; i < 100; i++ {
								if scenario == "quotes-100" {
									_, err = tx.ExecContext(t.Context(), `INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at) VALUES(?,'00000000-0000-4000-8000-000000000002','100.0001','CNY','manual','manual','2026-10-01T12:00:00Z','2026-10-01T12:00:00Z')`, fmt.Sprintf("00000000-0000-4000-8000-%012d", i+100))
								} else {
									_, err = tx.ExecContext(t.Context(), `UPDATE households SET name=?`, fmt.Sprintf("Synthetic %d", i))
								}
								if err != nil {
									tx.Rollback()
									t.Fatal(err)
								}
							}
							if err = tx.Commit(); err != nil {
								t.Fatal(err)
							}
							elapsed := time.Since(started)
							runtime.ReadMemStats(&memAfter)
							durations[mode] = append(durations[mode], elapsed)
							if err = db.SQL.QueryRow(`SELECT total_changes()`).Scan(&after); err != nil {
								t.Fatal(err)
							}
							var changes, bytes int64
							if err = db.SQL.QueryRow(`SELECT count(*),COALESCE(SUM(length(source_table)+length(source_id)+length(operation)+COALESCE(length(old_scope_json),0)+COALESCE(length(new_scope_json),0)+COALESCE(length(old_effective_at),0)+COALESCE(length(new_effective_at),0)),0) FROM financial_source_changes WHERE revision>4`).Scan(&changes, &bytes); err != nil {
								t.Fatal(err)
							}
							// Source fixture has four real inserts, not manufactured historic rows.
							if mode == 1 && (changes != 100 || after-before != 300) {
								t.Fatalf("unexpected write amplification: journal=%d totalChanges=%d", changes, after-before)
							}
							if mode == 0 && (changes != 0 || after-before != 100) {
								t.Fatal("counterfactual changed source bookkeeping")
							}
							walBytes := int64(0)
							if info, e := os.Stat(path + "-wal"); e == nil {
								walBytes = info.Size()
							}
							if err = db.CheckpointWAL(t.Context()); err != nil {
								t.Fatal(err)
							}
							info, err := os.Stat(path)
							if err != nil {
								t.Fatal(err)
							}
							batch, err := NewRepository(db).LoadReviewHistoricalBatch(t.Context(), domain.HouseholdID("00000000-0000-4000-8000-000000000001"), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
							if err != nil {
								t.Fatal(err)
							}
							fingerprints[mode], err = domain.ReviewHistoricalFingerprint(batch)
							if err != nil {
								t.Fatal(err)
							}
							t.Logf("write scenario=%s path=%d sample=%d elapsed_ns=%d sourceRows=100 totalChangedRows=%d journalRows=%d journalTextBytes=%d WALBeforeCheckpoint=%d DBBytesAfterCheckpoint=%d allocatedBytes=%d allocations=%d", scenario, mode, sample, elapsed.Nanoseconds(), after-before, changes, bytes, walBytes, info.Size(), memAfter.TotalAlloc-memBefore.TotalAlloc, memAfter.Mallocs-memBefore.Mallocs)
						})
					}
					if !reflect.DeepEqual(fingerprints[0], fingerprints[1]) {
						t.Fatal("tracking changed authoritative financial fingerprint")
					}
				})
			}
			for mode, values := range durations {
				sorted := append([]time.Duration{}, values...)
				sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
				t.Logf("write distribution scenario=%s path=%d n=%d nearest-rank-median=%s nearest-rank-p95=%s raw=%v", scenario, mode, len(values), sorted[14], sorted[28], values)
			}
		})
	}
}
