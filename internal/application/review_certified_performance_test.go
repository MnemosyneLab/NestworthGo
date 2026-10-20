//go:build perf

package application

import (
	"encoding/json"
	"fmt"
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
	runReviewCertifiedPerformanceEvidence(t, false)
}

func TestReviewDependencySuffixPerformanceEvidence(t *testing.T) {
	if os.Getenv("NW_D2_SUFFIX_EVIDENCE") != "1" {
		t.Skip("opt-in synthetic D1/D2 evidence")
	}
	runReviewCertifiedPerformanceEvidence(t, true)
}

func runReviewCertifiedPerformanceEvidence(t *testing.T, suffix bool) {
	t.Logf("dependencySuffix=%v (same C2 control; isolated synthetic data)", suffix)
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
			scenarios := []string{"warm", "clock-only", "append", "correction"}
			if suffix {
				scenarios = append(scenarios, "quote-append")
			}
			for _, scenario := range scenarios {
				t.Run(scenario, func(t *testing.T) {
					times := map[string][]time.Duration{"C2": {}, "certified": {}}
					for sample := range samples {
						fixtures := map[string]reviewFullPipelineFixture{}
						for _, arm := range []string{"C2", "certified"} {
							var f reviewFullPipelineFixture
							if arm == "certified" {
								f = cloneReviewGenesisPerfFixture(t, source)
							} else {
								f = cloneReviewPerfFixture(t, source)
								enableReviewSQLMetrics(t, f)
							}
							r := sqlite.NewRepository(f.db).WithBoundedReviewWindow()
							if arm == "certified" {
								r = r.WithCertifiedReviewCandidate()
								if suffix {
									r = r.WithReviewDependencySuffix()
								}
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
							case "quote-append":
								when := *f.now
								for index, instrument := range f.instruments {
									at := time.Date(when.Year(), when.Month(), when.Day(), 17, 0, 0, 0, time.UTC)
									_, err := f.db.SQL.Exec(`INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at,observation_kind,effective_date,value_effective_at,binding_revision,price_basis,timestamp_basis,revision) VALUES(?,?, '101',?,'agent','agent',?,?,'close',?,?,0,'agent_raw_close_v1','date_label',1)`, fmt.Sprintf("00000000-0000-4000-8007-%012x", index+1), instrument.ID.String(), instrument.QuoteCurrency.String(), formatReviewPerfTime(at), formatReviewPerfTime(at), when.Format(time.DateOnly), formatReviewPerfTime(at))
									if err != nil {
										t.Fatal(err)
									}
								}
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
								if suffix {
									wantSuffix := scenario != "append"
									if (stage.Capture.Suffix != nil) != wantSuffix {
										t.Fatalf("scenario=%s suffix selected=%v want=%v", scenario, stage.Capture.Suffix != nil, wantSuffix)
									}
									reused := 0
									if stage.Capture.Suffix != nil {
										reused = len(stage.Capture.Suffix.Prefix)
									}
									t.Logf("D2 size=%s scenario=%s recomputedPairDays=%d reusedPairDays=%d", size, scenario, countReviewPublicationDays(stage)-reused, reused)
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
								f.service.repository = r.WithReviewLineageObserver(func(o domain.ReviewCompletenessLineageObservation) {
									if o.Phase == "certified_publication" {
										verdict = o.Verdict
									}
								})
							}
							var phaseProfile *reviewProfileRepository
							if suffix && arm == "certified" && os.Getenv("NW_D2_PROFILE_DIR") != "" {
								phaseProfile = &reviewProfileRepository{Repository: f.service.repository.(*sqlite.Repository), phases: map[string]time.Duration{}}
								f.service.repository = phaseProfile
							}
							runtime.GC()
							finishProfile := beginReviewDependencyProfile(t, suffix, arm, scenario, sample)
							var before, after runtime.MemStats
							runtime.ReadMemStats(&before)
							reviewPerfStats.reset()
							started := time.Now()
							got, err := f.service.ReconcileInvestmentReviewsV2(t.Context())
							elapsed := time.Since(started)
							runtime.ReadMemStats(&after)
							finishProfile()
							if phaseProfile != nil {
								summed := time.Duration(0)
								for _, duration := range phaseProfile.phases {
									summed += duration
								}
								t.Logf("D2 phase wall scenario=%s sample=%d phases=%v prepare-and-orchestration-residual=%s (disjoint API intervals; residual includes planning/locks/provider orchestration)", scenario, sample+1, phaseProfile.phases, elapsed-summed)
							}
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

// Bootstrap and intra-day activation are reported separately from stable warm
// and clock-only measurements. Each activation starts with the future event
// already present, so its source token stays constant across T.
func TestReviewCertifiedBootstrapActivationEvidence(t *testing.T) {
	if os.Getenv("NW_C3_CERTIFIED_EVIDENCE") != "1" {
		t.Skip("opt-in synthetic certified evidence")
	}
	for _, size := range c3PerfSizes(t) {
		t.Run(size, func(t *testing.T) {
			config := c3RefreshConfig(size)
			source := newReviewPerfFixture(t, config)
			logReviewPerfManifest(t, source, config)
			before := *source.now
			at := before.Add(time.Hour)
			var holding domain.HoldingID
			if err := source.db.SQL.QueryRow(`SELECT id FROM holdings WHERE instrument_id=? ORDER BY id LIMIT 1`, source.instruments[0].ID.String()).Scan(&holding); err != nil {
				t.Fatal(err)
			}
			*source.now = at
			_, err := source.service.RecordChange(t.Context(), domain.PositionAdjustmentInput{HouseholdID: source.householdID, HoldingID: holding, Quantity: mustQuantity(t, "2"), Added: false, EffectiveAt: at})
			if err != nil {
				t.Fatal(err)
			}
			*source.now = before
			for sample := range reviewPerfSampleCount(t, 3) {
				order := []string{"C2-full", "certified"}
				if sample%2 == 1 {
					order[0], order[1] = order[1], order[0]
				}
				for _, arm := range order {
					var f reviewFullPipelineFixture
					if arm == "certified" {
						f = cloneReviewGenesisPerfFixture(t, source)
					} else {
						f = cloneReviewPerfFixture(t, source)
						enableReviewSQLMetrics(t, f)
					}
					r := sqlite.NewRepository(f.db).WithBoundedReviewWindow()
					if arm == "certified" {
						r = r.WithCertifiedReviewCandidate()
					} else {
						r = r.WithFullReviewRecompute()
					}
					f.service = NewService(r)
					f.service.fxProviderKey = "synthetic-pinned"
					f.service.setClock(func() time.Time { return *f.now })
					for _, scenario := range []string{"bootstrap", "activation"} {
						if scenario == "activation" {
							*f.now = at
						}
						c, err := r.CaptureReviewProjection(t.Context(), f.householdID, *f.now)
						if err != nil {
							t.Fatal(err)
						}
						if c.Certified != nil {
							c.Certified.Provider = f.service.reviewProviderSnapshot()
						}
						p, err := PrepareReviewPublication(t.Context(), c, *f.now, "synthetic-pinned")
						if err != nil {
							t.Fatal(err)
						}
						assertReviewFullOracle(t, f, p)
						if scenario == "activation" {
							if c.PublishedTemporal == nil || c.PublishedTemporal.Source.Token != c.SourceRevision.Token {
								t.Fatal("activation changed the token")
							}
							if p.CertifiedFullOracle != nil {
								t.Fatal("intra-day activation reused bounded authority")
							}
						}
						runtime.GC()
						var b, a runtime.MemStats
						runtime.ReadMemStats(&b)
						reviewPerfStats.reset()
						started := time.Now()
						got, err := f.service.ReconcileInvestmentReviewsV2(t.Context())
						elapsed := time.Since(started)
						runtime.ReadMemStats(&a)
						if err != nil || got.DayCount == 0 {
							t.Fatal("expected full reconciliation", got, err)
						}
						metrics := reviewPerfStats.snapshot(0)
						t.Logf("certified-lifecycle size=%s scenario=%s arm=%s n=%d elapsed=%s allocatedBytes=%d allocs=%d publishedPairDays=%d %s", size, scenario, arm, sample+1, elapsed, a.TotalAlloc-b.TotalAlloc, a.Mallocs-b.Mallocs, got.DayCount, metrics.summary())
					}
					if err := f.db.Close(); err != nil {
						t.Fatal(err)
					}
					if err := os.Remove(f.dbPath); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}
