package application

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestReviewPerfConfiguration(t *testing.T) {
	if validateReviewPerfMode("typo") == nil {
		t.Fatal("unknown evidence mode accepted")
	}
	for _, c := range []reviewPerfConfig{{}, {Accounts: 1, Instruments: 1, Rules: 1, HistoryDays: 1, Quotes: -1}} {
		if c.validate() == nil {
			t.Fatalf("invalid config accepted: %+v", c)
		}
	}
	if durationPercentile([]time.Duration{3, 1, 2}, .95) != 3 {
		t.Fatal("nearest-rank percentile")
	}
}

// Each measured mutation starts from this identical, checkpointed synthetic
// snapshot. VACUUM INTO includes committed WAL contents without copying a live
// database file. UUIDs are generated once in the source and shared by clones.
func cloneReviewPerfFixture(t *testing.T, source reviewFullPipelineFixture) reviewFullPipelineFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic-clone.db")
	if _, err := source.db.SQL.Exec(`VACUUM INTO ?`, path); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	now := *source.now
	service := NewService(sqlite.NewRepository(db))
	service.setClock(func() time.Time { return now })
	return reviewFullPipelineFixture{service: service, db: db, now: &now, householdID: source.householdID, dbPath: path, start: source.start, instruments: source.instruments}
}

func reviewPerfEnvironmentConfig(t *testing.T) reviewPerfConfig {
	t.Helper()
	c := referenceReviewPerfConfig(30)
	for _, field := range []struct {
		name string
		into *int
	}{
		{"ACCOUNTS", &c.Accounts}, {"INSTRUMENTS", &c.Instruments}, {"RULES", &c.Rules},
		{"DAYS", &c.HistoryDays}, {"QUOTES", &c.Quotes}, {"ACTIVITIES", &c.CashActivities},
	} {
		if value, exists := os.LookupEnv("NW_REVIEW_PERF_" + field.name); exists {
			parsed, err := strconv.Atoi(value)
			if err != nil {
				t.Fatal(err)
			}
			*field.into = parsed
		}
	}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	return c
}

func logReviewPerfManifest(t *testing.T, f reviewFullPipelineFixture, c reviewPerfConfig) {
	t.Helper()
	var version string
	if err := f.db.SQL.QueryRow(`SELECT sqlite_version()`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("manifest=%s; deterministic arithmetic source generation (no PRNG); clock=%s schema=%d Go=%s SQLite=%s GOOS=%s GOARCH=%s GOMAXPROCS=%d; UUIDs generated once then cloned; synthetic household=%s", manifest, f.now.Format(time.RFC3339Nano), sqlite.CurrentSchemaVersion, runtime.Version(), version, runtime.GOOS, runtime.GOARCH, runtime.GOMAXPROCS(0), f.householdID)
}

// Full recomputation is captured independently from the same authoritative DB,
// retaining handling history but loading no reusable daily projection inputs.
// Compare every emitted day's exact typed financial evidence and the complete
// lifecycle/coverage/window summary. Only freshly generated episode IDs are
// mapped; persisted IDs and acknowledgement times must remain identical.
func assertReviewFullOracle(t *testing.T, f reviewFullPipelineFixture, candidate domain.ReviewPublication) {
	t.Helper()
	capture, err := sqlite.NewRepository(f.db).WithFullReviewRecompute().CaptureReviewProjection(t.Context(), f.householdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if len(capture.ReusableDailyEvidence) != 0 {
		t.Fatal("full oracle reused evidence")
	}
	full, err := PrepareReviewPublication(t.Context(), capture, *f.now, f.service.FXProviderKey())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidate.Rules) != len(full.Rules) {
		t.Fatal("oracle rule count differs")
	}
	for i, got := range candidate.Rules {
		want := full.Rules[i]
		byDay := map[string]domain.ReviewPublicationDay{}
		for _, day := range want.Days {
			byDay[day.On] = day
		}
		for _, day := range got.Days {
			if !reflect.DeepEqual(day, byDay[day.On]) {
				t.Fatalf("oracle day differs rule=%s instrument=%s on=%s\ngot=%+v\nwant=%+v", got.RuleID, got.InstrumentID, day.On, day, byDay[day.On])
			}
		}
		got.Days, want.Days = nil, nil
		if got.OpenedEpisode && want.OpenedEpisode && got.CurrentEpisode != nil && want.CurrentEpisode != nil {
			copied := *got.CurrentEpisode
			copied.ID = want.CurrentEpisode.ID
			got.CurrentEpisode = &copied
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("oracle lifecycle differs rule=%s instrument=%s\ngot=%+v\nwant=%+v", got.RuleID, got.InstrumentID, got, want)
		}
	}
}

func TestReviewPerformanceFullOracle(t *testing.T) {
	runReviewPerfPairs(t, reviewPerfConfig{Accounts: 2, Instruments: 3, Rules: 4, HistoryDays: 10, Quotes: 30}, 1)
}

// Opt-in distribution evidence: samples are independent clones, paths alternate
// order, and all correctness/oracle work occurs outside reported timings.
func TestReviewPairedPerformanceEvidence(t *testing.T) {
	if os.Getenv("NW_REVIEW_PERF_EVIDENCE") != "1" {
		t.Skip("set NW_REVIEW_PERF_EVIDENCE=1")
	}
	samples := 30
	if value := os.Getenv("NW_REVIEW_PERF_SAMPLES"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			t.Fatal("invalid NW_REVIEW_PERF_SAMPLES")
		}
		samples = parsed
	}
	runReviewPerfPairs(t, reviewPerfEnvironmentConfig(t), samples)
}

func runReviewPerfPairs(t *testing.T, config reviewPerfConfig, samples int) {
	t.Helper()
	source := newReviewPerfFixture(t, config)
	logReviewPerfManifest(t, source, config)
	started := time.Now()
	if _, err := source.service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Logf("cold initial projection (single sample, includes capture through atomic publish; excludes fixture generation)=%s", time.Since(started))
	for _, scenario := range []string{"no-op", "clock-only", "append", "correction"} {
		t.Run(scenario, func(t *testing.T) {
			durations := [2][]time.Duration{}
			for sample := 0; sample < samples; sample++ {
				// Subtest cleanup releases each clone before the next sample (including
				// large capacity runs) rather than retaining 30 full database copies.
				t.Run(fmt.Sprintf("sample-%02d", sample), func(t *testing.T) {
					fixture := cloneReviewPerfFixture(t, source)
					switch scenario {
					case "clock-only":
						*fixture.now = fixture.now.AddDate(0, 0, 1)
					case "append":
						*fixture.now = fixture.now.AddDate(0, 0, 1)
						if err := appendReviewPerfDailyMarketData(t.Context(), fixture, fixture.now.AddDate(0, 0, -1)); err != nil {
							t.Fatal(err)
						}
					case "correction":
						if err := correctReviewPerfHistoricalQuote(t.Context(), fixture, 0); err != nil {
							t.Fatal(err)
						}
					}
					repositories := [2]*sqlite.Repository{sqlite.NewRepository(fixture.db).WithReviewWindowBaseline(), sqlite.NewRepository(fixture.db)}
					var captures [2]domain.ReviewProjectionCapture
					for offset := 0; offset < 2; offset++ {
						path := (sample + offset) % 2
						started := time.Now()
						capture, err := repositories[path].CaptureReviewProjection(t.Context(), fixture.householdID, *fixture.now)
						elapsed := time.Since(started)
						if err != nil {
							t.Fatal(err)
						}
						durations[path] = append(durations[path], elapsed)
						captures[path] = capture
						t.Logf("capture path=%d sample=%d elapsed=%s reusableRows=%d (includes source fingerprint and Rows consumption)", path, sample, elapsed, len(capture.ReusableDailyEvidence))
					}
					if !reflect.DeepEqual(captures[0], captures[1]) {
						t.Fatal("baseline/candidate captures differ")
					}
					publication, err := PrepareReviewPublication(t.Context(), captures[1], *fixture.now, fixture.service.FXProviderKey())
					if err != nil {
						t.Fatal(err)
					}
					assertReviewFullOracle(t, fixture, publication)
					for _, path := range []int{0, 1} {
						// Reconcile/publish on separate identical scenario clones, never publish
						// the baseline before measuring the candidate on the same DB.
						run := cloneReviewPerfFixture(t, fixture)
						repo := sqlite.NewRepository(run.db)
						if path == 0 {
							repo = repo.WithReviewWindowBaseline()
						}
						service := NewService(repo)
						service.setClock(func() time.Time { return *run.now })
						started := time.Now()
						result, err := service.ReconcileInvestmentReviewsV2(t.Context())
						if err != nil {
							t.Fatal(err)
						}
						t.Logf("full reconcile path=%d sample=%d elapsed=%s pairDays=%d", path, sample, time.Since(started), result.DayCount)
					}
				})
			}
			for path, values := range durations {
				t.Logf("capture distribution scenario=%s path=%d n=%d median=%s nearest-rank-p95=%s raw=%v", scenario, path, len(values), durationPercentile(values, .5), durationPercentile(values, .95), values)
			}
		})
	}
}
