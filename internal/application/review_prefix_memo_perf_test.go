//go:build perf

package application

import (
	"context"
	"os"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type memoPerfRepository struct {
	*reviewProfileRepository
	last domain.ReviewPrefixMemoObservation
}

func (r *memoPerfRepository) CaptureReviewProjection(ctx context.Context, id domain.HouseholdID, at time.Time) (domain.ReviewProjectionCapture, error) {
	c, err := r.reviewProfileRepository.CaptureReviewProjection(ctx, id, at)
	if c.Certified != nil && c.Certified.SuffixMemoObservation != nil {
		r.last = *c.Certified.SuffixMemoObservation
	}
	return c, err
}
func TestReviewPrefixMemoPerformance(t *testing.T) {
	arm := os.Getenv("NW_D2_MEMO_ARM")
	if arm != "baseline" && arm != "memo" {
		t.Skip("opt-in isolated single-arm memo experiment")
	}
	config := c3RefreshConfig("small")
	if os.Getenv("NW_D2_MEMO_SIZE") == "medium" {
		config = reviewPerfConfig{Accounts: 1, Instruments: 50, Holdings: 50, Rules: 3, HistoryDays: 30, Quotes: 1550, CashActivities: 500}
	}
	t.Logf("memo arm=%s source config=%+v", arm, config)
	source := matchedReviewSourceFixture(t, config)
	f := cloneReviewGenesisPerfFixture(t, source)
	r := sqlite.NewRepository(f.db).WithReviewPrefixMemoExperiment(arm == "memo")
	prof := &memoPerfRepository{reviewProfileRepository: &reviewProfileRepository{Repository: r, phases: map[string]time.Duration{}}}
	f.service = NewService(prof)
	f.service.fxProviderKey = "synthetic-pinned"
	f.service.setClock(func() time.Time { return *f.now })
	if _, err := f.service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	z := seriesZeroFixture{s: f.service, db: f.db, now: f.now}
	z.account.Account.HouseholdID = f.householdID
	for step := 0; step < 3; step++ {
		*f.now = f.now.AddDate(0, 0, 1)
		preserved := suffixExistingIdentities(t, f.db)
		ref := suffixFullReference(t, z)
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
		stage = domain.ReviewPublication{}
		c = domain.ReviewProjectionCapture{}
		prof.phases = map[string]time.Duration{}
		prof.last = domain.ReviewPrefixMemoObservation{}
		reviewPerfStats.reset()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		start := time.Now()
		_, err = f.service.ReconcileInvestmentReviewsV2(t.Context())
		elapsed := time.Since(start)
		runtime.ReadMemStats(&after)
		sqlStats := reviewPerfStats.snapshot(0)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(suffixDurableRows(t, f.db, preserved), suffixDurableRows(t, ref, preserved)) || !reflect.DeepEqual(publicationDailyTimes(t, f.db), publicationDailyTimes(t, ref)) {
			t.Fatal("full durable/timestamp mismatch")
		}
		_ = ref.Close()
		runtime.GC()
		runtime.GC()
		debug.FreeOSMemory()
		var retained runtime.MemStats
		runtime.ReadMemStats(&retained)
		raw, _ := os.ReadFile("/proc/self/status")
		rss := "unavailable"
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "VmRSS:") {
				rss = line
			}
		}
		t.Logf("memo arm=%s step=%d elapsed=%s allocatedBytes=%d allocs=%d phases=%v observed=%+v retainedHeap=%d RSS=%s %s", arm, step, elapsed, after.TotalAlloc-before.TotalAlloc, after.Mallocs-before.Mallocs, prof.phases, prof.last, retained.HeapAlloc, rss, sqlStats.summary())
		if step == 0 {
			// Existing certified noop updates its checkpoint; memo rebind happens only
			// after that successful CAS commit, preserving the same content digest.
			if got, err := f.service.ReconcileInvestmentReviewsV2(t.Context()); err != nil || got.DayCount != 0 {
				t.Fatal("noop", got, err)
			}
		}
	}
}
