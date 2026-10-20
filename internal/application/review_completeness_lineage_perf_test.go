//go:build perf

package application

import (
	"os"
	"reflect"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// Diagnostic cost only: both arms retain C3's authoritative full fallback.
// Reuse is simulated after measurement, checked, and never published.
func TestReviewCompletenessLineagePerformanceEvidence(t *testing.T) {
	if os.Getenv("NW_C3_LINEAGE_EVIDENCE") != "1" {
		t.Skip("opt-in synthetic lineage shadow evidence")
	}
	samples := 3
	if raw := os.Getenv("NW_C3_LINEAGE_SAMPLES"); raw != "" {
		var err error
		samples, err = strconv.Atoi(raw)
		if err != nil || samples < 1 {
			t.Fatal("invalid NW_C3_LINEAGE_SAMPLES")
		}
	}
	for _, size := range c3PerfSizes(t) {
		t.Run(size, func(t *testing.T) {
			config := c3RefreshConfig(size)
			source := newReviewPerfFixture(t, config)
			logReviewPerfManifest(t, source, config)
			if _, err := source.service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
				t.Fatal(err)
			}
			timings := map[string][]time.Duration{"full": {}, "shadow": {}}
			for sample := range samples {
				arms := []string{"full", "shadow"}
				if sample%2 == 1 {
					arms[0], arms[1] = arms[1], arms[0]
				}
				var outputs [2]map[string][]string
				for _, arm := range arms {
					f := cloneReviewPerfFixture(t, source)
					r := sqlite.NewRepository(f.db).WithReviewNoChangeCandidate()
					f.service.repository = r
					if _, err := f.service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
						t.Fatal(err)
					}
					*f.now = f.now.AddDate(0, 0, 1)
					o := &lineageObservations{}
					if arm == "shadow" {
						r = r.WithReviewCompletenessLineageShadow(o.observe)
					}
					enableReviewSQLMetrics(t, f)
					reviewPerfStats.reset()
					runtime.GC()
					var before, after runtime.MemStats
					runtime.ReadMemStats(&before)
					started := time.Now()
					capture, err := r.CaptureReviewProjection(t.Context(), f.householdID, *f.now)
					captured := time.Now()
					if err != nil {
						t.Fatal(err)
					}
					if len(capture.ReusableDailyEvidence) != 0 || !capture.TemporalFullRecompute {
						t.Fatal("measured arm reused history")
					}
					p, err := PrepareReviewPublication(t.Context(), capture, *f.now, f.service.FXProviderKey())
					prepared := time.Now()
					if err != nil {
						t.Fatal(err)
					}
					if err := r.PublishReviewProjection(t.Context(), p, *f.now); err != nil {
						t.Fatal(err)
					}
					finished := time.Now()
					runtime.ReadMemStats(&after)
					elapsed := finished.Sub(started)
					timings[arm] = append(timings[arm], elapsed)
					stats := reviewPerfStats.snapshot(0).summary()
					days := 0
					for _, rule := range p.Rules {
						days += len(rule.Days)
					}
					verdict, keys, window, hypotheticalDays := "off", 0, 0, 0
					if arm == "shadow" {
						verdict = o.last().Verdict
						if verdict != "lineage_validated_shadow" {
							t.Fatal("lineage shadow declined", verdict)
						}
						keys, window = len(capture.CompletenessLineage.Inputs), len(capture.CompletenessLineage.Window)
						hypothetical := capture
						hypothetical.CompletenessLineage = nil
						hypothetical.TemporalFullRecompute = false
						hypothetical.ReusableDailyEvidence = capture.CompletenessLineage.Window
						bounded, err := PrepareReviewPublication(t.Context(), hypothetical, *f.now, f.service.FXProviderKey())
						if err != nil {
							t.Fatal(err)
						}
						if err := compareReviewFullOracle(bounded, p, f.service.FXProviderKey()); err != nil {
							t.Fatal(err)
						}
						for _, rule := range bounded.Rules {
							hypotheticalDays += len(rule.Days)
						}
					}
					t.Logf("lineage size=%s arm=%s n=%d total=%s capture=%s prepare=%s publish=%s allocatedBytes=%d actualPairDays=%d prefixKeys=%d windowKeys=%d hypotheticalPairDays=%d verdict=%s %s", size, arm, sample+1, elapsed, captured.Sub(started), prepared.Sub(captured), finished.Sub(prepared), after.TotalAlloc-before.TotalAlloc, days, keys, window, hypotheticalDays, verdict, stats)
					index := 0
					if arm == "shadow" {
						index = 1
					}
					outputs[index] = noChangeProjectionRows(t, f.db)
					// Metadata-only C3 proofs do not affect this complete financial/lifecycle oracle.
					if err := f.db.Close(); err != nil {
						t.Fatal(err)
					}
					if err := os.Remove(f.dbPath); err != nil {
						t.Fatal(err)
					}
				}
				if !reflect.DeepEqual(outputs[0], outputs[1]) {
					t.Fatal("shadow changed full publication output")
				}
			}
			for _, arm := range []string{"full", "shadow"} {
				t.Logf("lineage-distribution size=%s arm=%s n=%d median=%s raw=%v (diagnostic overhead; no p95 or native performance claim)", size, arm, samples, durationPercentile(timings[arm], .5), timings[arm])
			}
		})
	}
}
