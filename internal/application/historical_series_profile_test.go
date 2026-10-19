package application

import (
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// Explicit opt-in; the profile excludes synthetic fixture construction. The
// production runtime and input budgets remain enabled for the measured call.
func TestHistoricalSeriesCPUProfileEvidence(t *testing.T) {
	dir := os.Getenv("NW_SERIES_CPU_EVIDENCE_DIR")
	if dir == "" {
		t.Skip("opt-in CPU profiling")
	}
	var s *Service
	fixture := os.Getenv("NW_SERIES_EVIDENCE_DB")
	if fixture != "" {
		if _, err := os.Stat(fixture); os.IsNotExist(err) {
			_, db := seriesBenchmarkFixture(t, true)
			if _, err := db.SQL.Exec(`PRAGMA query_only=OFF`); err != nil {
				t.Fatal(err)
			}
			if err := db.SnapshotTo(t.Context(), fixture); err != nil {
				t.Fatal(err)
			}
		}
		db, err := sqlite.OpenReadOnlyForVerify(fixture)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		var marker string
		if err := db.SQL.QueryRow(`SELECT name FROM households WHERE singleton_key=1`).Scan(&marker); err != nil || marker != "Synthetic series benchmark" {
			t.Fatalf("isolated synthetic marker required: %q %v", marker, err)
		}
		s = NewService(sqlite.NewRepository(db))
		s.setClock(func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) })
	} else {
		s, _ = seriesBenchmarkFixture(t, true)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	profile := os.Getenv("NW_SERIES_CPU_PROFILE") == "1"
	if profile {
		f, err := os.Create(filepath.Join(dir, "series.cpu"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			t.Fatal(err)
		}
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	out, err := s.HistoricalOverviewSeries(t.Context(), "2026-08-01", "2026-08-31")
	elapsed := time.Since(start)
	if profile {
		pprof.StopCPUProfile()
	}
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("dense series elapsed_ns=%d alloc_bytes=%d allocs=%d facts=%d bytes=%d leaves=%d queries=%d points=%d", elapsed.Nanoseconds(), after.TotalAlloc-before.TotalAlloc, after.Mallocs-before.Mallocs, out.Work.CandidateFacts, out.Work.CandidateBytes, out.Work.Leaves, out.Work.Queries, len(out.Points))
}
