//go:build perf

package application

import (
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func c3RefreshConfig(size string) reviewPerfConfig {
	if size == "large" {
		return referenceReviewPerfConfig(30)
	}
	return reviewPerfConfig{Accounts: 1, Instruments: 10, Holdings: 10, Rules: 1, HistoryDays: 30, Quotes: 310, CashActivities: 100}
}

func TestReviewNoChangeFallbackPerformanceEvidence(t *testing.T) {
	if os.Getenv("NW_C3_PERF_EVIDENCE") != "1" {
		t.Skip("opt-in corrected C3 evidence")
	}
	samples := 3
	if raw := os.Getenv("NW_C3_FALLBACK_SAMPLES"); raw != "" {
		var err error
		samples, err = strconv.Atoi(raw)
		if err != nil || samples < 1 {
			t.Fatal("invalid NW_C3_FALLBACK_SAMPLES")
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
			for _, scenario := range []string{"bootstrap", "clock-only", "proof-repair"} {
				t.Run(scenario, func(t *testing.T) {
					timings := map[string][]time.Duration{"baseline": {}, "candidate": {}}
					for sample := range samples {
						baseline, candidate := cloneReviewPerfFixture(t, source), cloneReviewPerfFixture(t, source)
						for _, f := range []reviewFullPipelineFixture{baseline, candidate} {
							enableReviewSQLMetrics(t, f)
						}
						reference := "C2-bounded"
						repository := sqlite.NewRepository(baseline.db).WithBoundedReviewWindow()
						if scenario != "clock-only" {
							repository = repository.WithFullReviewRecompute()
							reference = "forced-full"
						}
						baseline.service.repository = repository
						candidate.service.repository = sqlite.NewRepository(candidate.db).WithBoundedReviewWindow().WithReviewNoChangeCandidate()
						for _, f := range []reviewFullPipelineFixture{baseline, candidate} {
							if scenario == "bootstrap" {
								continue
							}
							if _, err := f.service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
								t.Fatal(err)
							}
							if scenario == "clock-only" {
								*f.now = f.now.AddDate(0, 0, 1)
							} else {
								// Destroy an old slot beyond every retained window, with no source write.
								for _, table := range []string{"investment_review_daily_inputs", "investment_review_daily_results"} {
									if _, err := f.db.SQL.Exec(`DELETE FROM ` + table + ` WHERE on_date=(SELECT min(on_date) FROM ` + table + `)`); err != nil {
										t.Fatal(err)
									}
								}
								if _, err := f.db.SQL.Exec(`UPDATE review_temporal_checkpoints SET certificate_json=json_remove(certificate_json,'$.NoChangeReviewFingerprint')`); err != nil {
									t.Fatal(err)
								}
							}
						}
						order := []string{"baseline", "candidate"}
						if sample%2 == 1 {
							order[0], order[1] = order[1], order[0]
						}
						for _, arm := range order {
							f := baseline
							if arm == "candidate" {
								f = candidate
							}
							reviewPerfStats.reset()
							started := time.Now()
							result, err := f.service.ReconcileInvestmentReviewsV2(t.Context())
							elapsed := time.Since(started)
							if err != nil || result.DayCount == 0 {
								t.Fatal("fallback failed to prepare", result, err)
							}
							timings[arm] = append(timings[arm], elapsed)
							t.Logf("fallback size=%s scenario=%s reference=%s arm=%s n=%d elapsed=%s pairDays=%d %s", size, scenario, reference, arm, sample+1, elapsed, result.DayCount, reviewPerfStats.snapshot(0).summary())
						}
						if !reflect.DeepEqual(noChangeProjectionRows(t, baseline.db), noChangeProjectionRows(t, candidate.db)) {
							t.Fatal("paired fallback projection differs")
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
						if scenario == "proof-repair" {
							var count int
							if err := candidate.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_daily_inputs WHERE on_date=?`, source.start.Format(time.DateOnly)).Scan(&count); err != nil || count != config.Rules*config.Instruments {
								t.Fatal("old slots not repaired", count, err)
							}
						}
						if err := baseline.db.Close(); err != nil {
							t.Fatal(err)
						}
						if err := candidate.db.Close(); err != nil {
							t.Fatal(err)
						}
						for _, f := range []reviewFullPipelineFixture{baseline, candidate} {
							if err := os.Remove(f.dbPath); err != nil {
								t.Fatal(err)
							}
						}
					}
					for _, arm := range []string{"baseline", "candidate"} {
						t.Logf("fallback-distribution size=%s scenario=%s arm=%s n=%d median=%s raw=%v (small-n diagnostic; no p95 estimate)", size, scenario, arm, samples, durationPercentile(timings[arm], .5), timings[arm])
					}
				})
			}
		})
	}
}
