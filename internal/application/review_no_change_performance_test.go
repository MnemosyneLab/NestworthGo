package application

import (
	"encoding/json"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// Opt-in, serial paired measurements. Setup, initial publication, session
// refresh, oracle and diagnostic reads are outside every timed sample. Both
// arms use identical authoritative source IDs/rows and the bounded C2 window;
// the baseline keeps full capture/recheck, the candidate confirms unchanged.
func TestReviewNoChangePerformanceEvidence(t *testing.T) {
	if os.Getenv("NW_C3_PERF_EVIDENCE") != "1" {
		t.Skip("opt-in synthetic C3 evidence")
	}
	for _, size := range []string{"small", "large"} {
		t.Run(size, func(t *testing.T) {
			config := reviewPerfConfig{Accounts: 1, Instruments: 10, Holdings: 10, Rules: 1, HistoryDays: 30, Quotes: 310, CashActivities: 100}
			if size == "large" {
				config = referenceReviewPerfConfig(30)
			}
			source := newReviewPerfFixture(t, config)
			logReviewPerfManifest(t, source, config)
			coldStarted := time.Now()
			if _, err := source.service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
				t.Fatal(err)
			}
			t.Logf("fresh-projection size=%s n=1 elapsed=%s (warm process/pages; setup excluded)", size, time.Since(coldStarted))
			baseline, candidate := cloneReviewPerfFixture(t, source), cloneReviewPerfFixture(t, source)
			for _, f := range []reviewFullPipelineFixture{baseline, candidate} {
				enableReviewSQLMetrics(t, f)
			}
			baseline.service.repository = sqlite.NewRepository(baseline.db).WithBoundedReviewWindow()
			candidate.service.repository = sqlite.NewRepository(candidate.db).WithBoundedReviewWindow().WithReviewNoChangeCandidate()
			for _, f := range []reviewFullPipelineFixture{baseline, candidate} {
				if _, err := f.service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			if !reflect.DeepEqual(noChangeProjectionRows(t, baseline.db), noChangeProjectionRows(t, candidate.db)) {
				t.Fatal("paired initial projection differs")
			}
			timings := map[string][]time.Duration{"baseline": {}, "candidate": {}}
			bytes := map[string][]uint64{"baseline": {}, "candidate": {}}
			allocs := map[string][]uint64{"baseline": {}, "candidate": {}}
			for sample := range 30 {
				order := []string{"baseline", "candidate"}
				if sample%2 == 1 {
					order[0], order[1] = order[1], order[0]
				}
				for _, arm := range order {
					f := baseline
					if arm == "candidate" {
						f = candidate
					}
					*f.now = source.now.Add(time.Duration(sample+1) * time.Nanosecond)
					reviewPerfStats.reset()
					var before, after runtime.MemStats
					runtime.ReadMemStats(&before)
					started := time.Now()
					result, err := f.service.ReconcileInvestmentReviewsV2(t.Context())
					elapsed := time.Since(started)
					runtime.ReadMemStats(&after)
					if err != nil {
						t.Fatal(err)
					}
					timings[arm] = append(timings[arm], elapsed)
					bytes[arm] = append(bytes[arm], after.TotalAlloc-before.TotalAlloc)
					allocs[arm] = append(allocs[arm], after.Mallocs-before.Mallocs)
					stats := reviewPerfStats.snapshot(0)
					if arm == "candidate" {
						if result.DayCount != 0 {
							t.Fatal("no-change fell back")
						}
						for _, statement := range stats.statements {
							q := strings.ToLower(statement.query)
							if strings.Contains(q, "from instrument_quotes") || strings.Contains(q, "from fx_quotes") || strings.Contains(q, "d.evidence_json") {
								t.Fatal("candidate loaded full source/evidence", q)
							}
						}
					}
					t.Logf("sample size=%s arm=%s n=%d elapsed=%s allocatedBytes=%d allocations=%d pairDays=%d %s", size, arm, sample+1, elapsed, bytes[arm][sample], allocs[arm][sample], result.DayCount, stats.summary())
					if sample == 0 {
						t.Logf("%s first SQL consumers: %s", arm, stats.slowest())
					}
				}
			}
			for _, arm := range []string{"baseline", "candidate"} {
				raw, _ := json.Marshal(struct {
					Timings       []time.Duration
					Bytes, Allocs []uint64
				}{timings[arm], bytes[arm], allocs[arm]})
				t.Logf("distribution size=%s arm=%s n=30 median=%s p95=%s rawNanoseconds=%s", size, arm, durationPercentile(timings[arm], .5), durationPercentile(timings[arm], .95), raw)
			}
			if !reflect.DeepEqual(noChangeProjectionRows(t, baseline.db), noChangeProjectionRows(t, candidate.db)) {
				t.Fatal("paired final projection differs")
			}
			capture, err := sqlite.NewRepository(candidate.db).CaptureReviewProjection(t.Context(), candidate.householdID, *candidate.now)
			if err != nil {
				t.Fatal(err)
			}
			stage, err := PrepareReviewPublication(t.Context(), capture, *candidate.now, candidate.service.FXProviderKey())
			if err != nil {
				t.Fatal(err)
			}
			assertReviewFullOracle(t, candidate, stage)
			logReviewFullPipelineStorage(t, candidate, 30)
			for _, scenario := range []string{"clock-only", "quote-correction", "market-append"} {
				f := cloneReviewPerfFixture(t, source)
				enableReviewSQLMetrics(t, f)
				f.service.repository = sqlite.NewRepository(f.db).WithBoundedReviewWindow().WithReviewNoChangeCandidate()
				if _, err := f.service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
					t.Fatal(err)
				}
				switch scenario {
				case "clock-only":
					*f.now = f.now.AddDate(0, 0, 1)
				case "quote-correction":
					if _, err := f.db.SQL.Exec(`UPDATE instrument_quotes SET unit_price='101' WHERE id=(SELECT id FROM instrument_quotes ORDER BY quoted_at,id LIMIT 1)`); err != nil {
						t.Fatal(err)
					}
				case "market-append":
					*f.now = f.now.AddDate(0, 0, 1)
					if err := appendReviewPerfDailyMarketData(t.Context(), f, *source.now); err != nil {
						t.Fatal(err)
					}
				}
				reviewPerfStats.reset()
				started := time.Now()
				result, err := f.service.ReconcileInvestmentReviewsV2(t.Context())
				elapsed := time.Since(started)
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("changed-workload size=%s scenario=%s n=1 elapsed=%s pairDays=%d %s", size, scenario, elapsed, result.DayCount, reviewPerfStats.snapshot(0).summary())
				capture, err := sqlite.NewRepository(f.db).CaptureReviewProjection(t.Context(), f.householdID, *f.now)
				if err != nil {
					t.Fatal(err)
				}
				stage, err := PrepareReviewPublication(t.Context(), capture, *f.now, f.service.FXProviderKey())
				if err != nil {
					t.Fatal(err)
				}
				assertReviewFullOracle(t, f, stage)
			}
		})
	}
}
