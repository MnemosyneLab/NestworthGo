//go:build perf

package application

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Embedding preserves every optional repository capability. Only perf builds
// add these timers/CPU labels; production reconciliation is unchanged.
type reviewProfileRepository struct {
	*sqlite.Repository
	phases map[string]time.Duration
}

func (r *reviewProfileRepository) phase(ctx context.Context, name string, fn func(context.Context) error) error {
	start := time.Now()
	var err error
	pprof.Do(ctx, pprof.Labels("review_phase", name), func(ctx context.Context) { err = fn(ctx) })
	r.phases[name] += time.Since(start)
	return err
}

func (r *reviewProfileRepository) CaptureReviewProjection(ctx context.Context, id domain.HouseholdID, at time.Time) (out domain.ReviewProjectionCapture, err error) {
	err = r.phase(ctx, "capture", func(ctx context.Context) error {
		out, err = r.Repository.CaptureReviewProjection(ctx, id, at)
		return err
	})
	return
}
func (r *reviewProfileRepository) PublishReviewProjectionWithClock(ctx context.Context, p domain.ReviewPublication, clock func() time.Time) error {
	return r.phase(ctx, "publish", func(ctx context.Context) error { return r.Repository.PublishReviewProjectionWithClock(ctx, p, clock) })
}
func (r *reviewProfileRepository) PublishCertifiedReviewProjection(ctx context.Context, p domain.ReviewPublication, clock func() time.Time, guard func(func() error) error) error {
	return r.phase(ctx, "publish", func(ctx context.Context) error {
		return r.Repository.PublishCertifiedReviewProjection(ctx, p, clock, guard)
	})
}
func (r *reviewProfileRepository) ConfirmCertifiedReviewNoChange(ctx context.Context, id domain.HouseholdID, at time.Time, clock func() time.Time, selection domain.ReviewProviderSelection) (out domain.ReviewNoChangeResult, ok bool, err error) {
	err = r.phase(ctx, "confirm", func(ctx context.Context) error {
		out, ok, err = r.Repository.ConfirmCertifiedReviewNoChange(ctx, id, at, clock, selection)
		return err
	})
	return
}

func writeReviewAllocProfile(t *testing.T, path string) {
	t.Helper()
	// Allocation records may lag two GC cycles. The sampling rate is set once
	// by -memprofilerate; before/after subtraction removes setup/oracle stacks.
	runtime.GC()
	runtime.GC()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = pprof.Lookup("allocs").WriteTo(f, 0)
	if closeErr := f.Close(); err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
}

func TestReviewClockOnlyCPUAllocationProfile(t *testing.T) {
	dir := os.Getenv("NW_C3_PROFILE_DIR")
	if dir == "" {
		t.Skip("opt-in synthetic clock-only profiling")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, size := range c3PerfSizes(t) {
		t.Run(size, func(t *testing.T) {
			source := newReviewPerfFixture(t, c3RefreshConfig(size))
			logReviewPerfManifest(t, source, c3RefreshConfig(size))
			source.service.fxProviderKey = "synthetic-pinned"
			base, err := fullReviewOracle(sqlite.NewRepository(source.db)).CaptureReviewProjection(t.Context(), source.householdID, *source.now)
			if err != nil {
				t.Fatal(err)
			}
			bootstrap, err := PrepareReviewPublication(t.Context(), base, *source.now, "synthetic-pinned")
			if err != nil {
				t.Fatal(err)
			}
			outputs := map[string]map[string][]string{}
			for _, arm := range []string{"C2", "certified"} {
				var f reviewFullPipelineFixture
				if arm == "certified" {
					f = cloneReviewGenesisPerfFixture(t, source)
				} else {
					f = cloneReviewPerfFixture(t, source)
					enableReviewSQLMetrics(t, f)
				}
				r := sqlite.NewRepository(f.db)
				if arm == "certified" {
					r = r.WithCertifiedReviewCandidate()
				}
				f.service = NewService(r)
				f.service.fxProviderKey = "synthetic-pinned"
				f.service.setClock(func() time.Time { return *f.now })
				capture, err := r.CaptureReviewProjection(t.Context(), f.householdID, *f.now)
				if err != nil || capture.HistoricalFingerprint != base.HistoricalFingerprint {
					t.Fatal("bootstrap source differs", err)
				}
				p := bootstrap
				p.Rules = cloneReviewOracleRules(bootstrap.Rules)
				if capture.Certified == nil {
					capture.Certified = &domain.ReviewCertifiedCapture{}
				}
				capture.Certified.Provider = f.service.reviewProviderSnapshot()
				p.Capture = capture
				if arm == "certified" {
					err = r.PublishCertifiedReviewProjection(t.Context(), p, func() time.Time { return *f.now }, func(fn func() error) error { return f.service.withReviewProviderLease(capture.Certified.Provider, fn) })
				} else {
					err = r.PublishReviewProjection(t.Context(), p, *f.now)
				}
				if err != nil {
					t.Fatal(err)
				}
				*f.now = f.now.AddDate(0, 0, 1)
				// Oracle work remains outside both profiles and measured call.
				capture, err = r.CaptureReviewProjection(t.Context(), f.householdID, *f.now)
				if err != nil {
					t.Fatal(err)
				}
				if capture.Certified != nil {
					capture.Certified.Provider = f.service.reviewProviderSnapshot()
				}
				stage, err := PrepareReviewPublication(t.Context(), capture, *f.now, "synthetic-pinned")
				if err != nil {
					t.Fatal(err)
				}
				assertReviewFullOracle(t, f, stage)
				observed := &reviewProfileRepository{Repository: r, phases: map[string]time.Duration{}}
				f.service.repository = observed
				prefix := filepath.Join(dir, size+"-"+arm)
				writeReviewAllocProfile(t, prefix+"-alloc-before.pb.gz")
				cpu, err := os.Create(prefix + "-cpu.pb.gz")
				if err != nil {
					t.Fatal(err)
				}
				if err := pprof.StartCPUProfile(cpu); err != nil {
					cpu.Close()
					t.Fatal(err)
				}
				reviewPerfStats.reset()
				var before, after runtime.MemStats
				runtime.ReadMemStats(&before)
				start := time.Now()
				got, reconcileErr := f.service.ReconcileInvestmentReviewsV2(t.Context())
				elapsed := time.Since(start)
				runtime.ReadMemStats(&after)
				pprof.StopCPUProfile()
				if err := cpu.Close(); err != nil {
					t.Fatal(err)
				}
				if reconcileErr != nil {
					t.Fatal(reconcileErr)
				}
				metrics := reviewPerfStats.snapshot(0)
				writeReviewAllocProfile(t, prefix+"-alloc-after.pb.gz")
				residual := elapsed
				for _, duration := range observed.phases {
					residual -= duration
				}
				manifest := map[string]any{"arm": arm, "size": size, "elapsed": elapsed.String(), "repositoryPhases": observed.phases, "outsideRepository": residual.String(), "allocatedBytes": after.TotalAlloc - before.TotalAlloc, "allocs": after.Mallocs - before.Mallocs, "pairDays": got.DayCount, "memProfileRate": runtime.MemProfileRate, "profiled": true, "cpuScope": "one actual reconcile only", "allocationScope": "after minus before cumulative alloc_space; GC twice at both boundaries", "outsideRepositoryIsNotPurePrepare": true}
				raw, _ := jsoncodec.MarshalIndent(manifest, "", "  ")
				if err := os.WriteFile(prefix+"-manifest.json", append(raw, '\n'), 0o600); err != nil {
					t.Fatal(err)
				}
				t.Logf("profile arm=%s elapsed=%s phases=%v outsideRepository=%s allocatedBytes=%d allocs=%d pairDays=%d %s", arm, elapsed, observed.phases, residual, after.TotalAlloc-before.TotalAlloc, after.Mallocs-before.Mallocs, got.DayCount, metrics.summary())
				outputs[arm] = certifiedPerfRows(t, f)
				if err := f.db.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if !reflect.DeepEqual(outputs["C2"], outputs["certified"]) {
				t.Fatal("profiled paired durable output differs")
			}
		})
	}
}

func c3RefreshConfig(size string) reviewPerfConfig {
	if size == "large" {
		return referenceReviewPerfConfig(30)
	}
	return reviewPerfConfig{Accounts: 1, Instruments: 10, Holdings: 10, Rules: 1, HistoryDays: 30, Quotes: 310, CashActivities: 100}
}
