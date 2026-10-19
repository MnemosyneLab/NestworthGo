//go:build perf

package application

import (
	"encoding/json"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// Clone authoritative facts before any projection exists: a new DB handle
// cannot import another handle's certification. A shared full preparation
// supplies identical bootstrap episode identities. Every clone captures and
// signs its own source proof; no key/certificate is exported between handles.
func TestReviewCertifiedPerformanceEvidence(t *testing.T) {
	if os.Getenv("NW_C3_CERTIFIED_EVIDENCE") != "1" {
		t.Skip("opt-in synthetic certified evidence")
	}
	samples := reviewPerfSampleCount(t, 3)
	for _, size := range c3PerfSizes(t) {
		t.Run(size, func(t *testing.T) {
			config := c3RefreshConfig(size)
			source := newReviewPerfFixture(t, config)
			logReviewPerfManifest(t, source, config)
			source.service.fxProviderKey = "synthetic-pinned"
			baseCapture, err := sqlite.NewRepository(source.db).WithFullReviewRecompute().CaptureReviewProjection(t.Context(), source.householdID, *source.now)
			if err != nil {
				t.Fatal(err)
			}
			bootstrap, err := PrepareReviewPublication(t.Context(), baseCapture, *source.now, "synthetic-pinned")
			if err != nil {
				t.Fatal(err)
			}
			for _, scenario := range []string{"warm", "clock-only", "append", "correction"} {
				t.Run(scenario, func(t *testing.T) {
					times := map[string][]time.Duration{"C2": {}, "certified": {}}
					for sample := range samples {
						fixtures := map[string]reviewFullPipelineFixture{}
						for _, arm := range []string{"C2", "certified"} {
							f := cloneReviewPerfFixture(t, source)
							enableReviewSQLMetrics(t, f)
							r := sqlite.NewRepository(f.db).WithBoundedReviewWindow()
							if arm == "certified" {
								r = r.WithCertifiedReviewCandidate()
							}
							f.service = NewService(r)
							f.service.fxProviderKey = "synthetic-pinned" // Fixed synthetic selection, no network provider calls.
							f.service.setClock(func() time.Time { return *f.now })
							c, err := r.CaptureReviewProjection(t.Context(), f.householdID, *f.now)
							if err != nil {
								t.Fatal(err)
							}
							if c.HistoricalFingerprint != baseCapture.HistoricalFingerprint {
								t.Fatal("bootstrap source differs")
							}
							p := bootstrap
							p.Rules = cloneReviewOracleRules(bootstrap.Rules)
							// Give both arms the complete exact-FX codec at bootstrap. This favors
							// C2 and prevents its older JSON-private-rate loss from skewing results.
							if c.Certified == nil {
								c.Certified = &domain.ReviewCertifiedCapture{}
							}
							c.Certified.Provider = f.service.reviewProviderSnapshot()
							p.Capture = c
							if arm == "certified" {
								err = r.PublishCertifiedReviewProjection(t.Context(), p, func() time.Time { return *f.now }, func(fn func() error) error { return f.service.withReviewProviderLease(c.Certified.Provider, fn) })
							} else {
								err = r.PublishReviewProjection(t.Context(), p, *f.now)
							}
							if err != nil {
								t.Fatal(err)
							}
							switch scenario {
							case "warm":
								*f.now = f.now.Add(time.Nanosecond)
							case "clock-only":
								*f.now = f.now.AddDate(0, 0, 1)
							case "append":
								if err := appendReviewPerfDailyMarketData(t.Context(), f, *f.now); err != nil {
									t.Fatal(err)
								}
								*f.now = f.now.AddDate(0, 0, 1)
							case "correction":
								if err := correctReviewPerfHistoricalQuote(t.Context(), f, sample); err != nil {
									t.Fatal(err)
								}
							}
							// A separate pre-publication full capture is the oracle, never a clone
							// created after the candidate has already written its answers.
							if arm == "certified" && scenario != "warm" {
								c, err := r.CaptureReviewProjection(t.Context(), f.householdID, *f.now)
								if err != nil {
									t.Fatal(err)
								}
								c.Certified.Provider = f.service.reviewProviderSnapshot()
								stage, err := PrepareReviewPublication(t.Context(), c, *f.now, "synthetic-pinned")
								if err != nil {
									t.Fatal(err)
								}
								assertReviewFullOracle(t, f, stage)
								audited := countReviewPublicationDays(stage)
								if stage.CertifiedFullOracle != nil {
									audited = 0
									for _, rule := range stage.CertifiedFullOracle.Rules {
										audited += len(rule.Days)
									}
								}
								t.Logf("audit size=%s scenario=%s sample=%d actualPublicationDays=%d independentFullAuditDays=%d", size, scenario, sample+1, countReviewPublicationDays(stage), audited)
							}
							fixtures[arm] = f
						}
						order := []string{"C2", "certified"}
						if sample%2 == 1 {
							order[0], order[1] = order[1], order[0]
						}
						for _, arm := range order {
							f := fixtures[arm]
							verdict := "none"
							r := f.service.repository.(*sqlite.Repository)
							if arm == "certified" {
								f.service.repository = r.WithReviewCompletenessLineageShadow(func(o domain.ReviewCompletenessLineageObservation) {
									if o.Phase == "certified_publication" {
										verdict = o.Verdict
									}
								})
							}
							runtime.GC()
							var before, after runtime.MemStats
							runtime.ReadMemStats(&before)
							reviewPerfStats.reset()
							started := time.Now()
							got, err := f.service.ReconcileInvestmentReviewsV2(t.Context())
							elapsed := time.Since(started)
							runtime.ReadMemStats(&after)
							metrics := reviewPerfStats.snapshot(0)
							if err != nil {
								t.Fatal(err)
							}
							if arm == "certified" && scenario == "warm" {
								if got.DayCount != 0 {
									t.Fatal("certified warm did computation", got)
								}
								for _, q := range metrics.statements {
									normalized := strings.ToLower(q.query)
									if q.queries > 0 && (strings.Contains(normalized, "instrument_quotes") || strings.Contains(normalized, "fx_quotes") || strings.Contains(normalized, "evidence_json") || strings.Contains(normalized, "payload_json")) && !strings.Contains(normalized, "sqlite_master") {
										t.Fatal("warm read financial/evidence JSON", q.query)
									}
								}
							} else if arm == "certified" && verdict != "verified_source_cas" {
								t.Fatal("expected source CAS", verdict)
							}
							times[arm] = append(times[arm], elapsed)
							t.Logf("certified size=%s scenario=%s arm=%s n=%d elapsed=%s allocatedBytes=%d allocs=%d publishedPairDays=%d publication=%s %s", size, scenario, arm, sample+1, elapsed, after.TotalAlloc-before.TotalAlloc, after.Mallocs-before.Mallocs, got.DayCount, verdict, metrics.summary())
						}
						// Financial/lifecycle rows must match across arms, apart from the
						// candidate's additive exact-rate codec. Typed oracle above checks exact
						// rates independently; no monetary/rule/window fields are normalized.
						normalize := func(f reviewFullPipelineFixture) map[string][]string { return certifiedPerfRows(t, f) }
						if !reflect.DeepEqual(normalize(fixtures["C2"]), normalize(fixtures["certified"])) {
							t.Fatal("paired persisted output differs")
						}
						for _, f := range fixtures {
							if err := f.db.Close(); err != nil {
								t.Fatal(err)
							}
							if err := os.Remove(f.dbPath); err != nil {
								t.Fatal(err)
							}
						}
					}
					for _, arm := range []string{"C2", "certified"} {
						t.Logf("certified-distribution size=%s scenario=%s arm=%s n=%d median=%s raw=%v (diagnostic; no p95/RSS acceptance)", size, scenario, arm, samples, durationPercentile(times[arm], .5), times[arm])
					}
				})
			}
		})
	}
}

func certifiedPerfRows(t *testing.T, f reviewFullPipelineFixture) map[string][]string {
	result := noChangeProjectionRows(t, f.db)
	// ExactSelectedFXRates is an additive certified codec field. Both arms
	// still compare every other persisted column and JSON field byte-for-byte.
	table := "investment_review_daily_inputs"
	for i, raw := range result[table] {
		var values []any
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			t.Fatal(err)
		}
		for j, v := range values {
			text, ok := v.(string)
			if !ok || !strings.Contains(text, "ExactSelectedFXRates") {
				continue
			}
			var obj map[string]json.RawMessage
			if err := json.Unmarshal([]byte(text), &obj); err != nil {
				t.Fatal(err)
			}
			delete(obj, "ExactSelectedFXRates")
			// Marshal both forms via maps so key ordering is identical.
			clean, err := json.Marshal(obj)
			if err != nil {
				t.Fatal(err)
			}
			values[j] = string(clean)
		}
		// Canonicalize ordinary JSON too; no fields removed here.
		for j, v := range values {
			if text, ok := v.(string); ok && strings.HasPrefix(text, "{") {
				var obj map[string]json.RawMessage
				if json.Unmarshal([]byte(text), &obj) == nil {
					clean, _ := json.Marshal(obj)
					values[j] = string(clean)
				}
			}
		}
		clean, err := json.Marshal(values)
		if err != nil {
			t.Fatal(err)
		}
		result[table][i] = string(clean)
	}

	sort.Strings(result[table])
	// All event UUIDs were allocated independently during the fresh bootstrap;
	// normalize only that identity through its unique event key. Episode IDs,
	// payloads, event keys and every other field remain exact.
	table = "investment_review_episode_events"
	for i, raw := range result[table] {
		var values []any
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			t.Fatal(err)
		}
		values[0] = "new-event:" + values[2].(string)
		clean, err := json.Marshal(values)
		if err != nil {
			t.Fatal(err)
		}
		result[table][i] = string(clean)
	}
	sort.Strings(result[table])
	return result
}
