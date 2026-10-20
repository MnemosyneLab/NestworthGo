//go:build perf

package application

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

type c3RSSConfig struct {
	Database  string
	Household domain.HouseholdID
	Now       time.Time
	Arm       string
}

type c3RSSResult struct {
	Arm                                 string
	PID                                 int
	Times                               []time.Duration
	ReadyRSSKiB, ReadyHWMKiB, EndHWMKiB int64
	SampledPeakRSSKiB                   int64
	Polls                               int
	SQL                                 string
}

func c3ProcessRSS(pid int) (rss, hwm int64, err error) {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, 0, err
	}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "VmRSS:":
			rss, err = strconv.ParseInt(fields[1], 10, 64)
		case "VmHWM:":
			hwm, err = strconv.ParseInt(fields[1], 10, 64)
		}
		if err != nil {
			return 0, 0, err
		}
	}
	if rss == 0 || hwm == 0 {
		return 0, 0, fmt.Errorf("Linux RSS counters unavailable")
	}
	return rss, hwm, nil
}

func c3WaitFile(ctx context.Context, path string) error {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func TestReviewNoChangeRSSChild(t *testing.T) {
	if os.Getenv("NW_C3_RSS_CHILD") != "1" {
		t.Skip("isolated synthetic child only")
	}
	dir := os.Getenv("NW_C3_RSS_DIR")
	marker, err := os.ReadFile(filepath.Join(dir, "synthetic.marker"))
	if err != nil || string(marker) != "Nestworth synthetic C3 RSS fixture" {
		t.Fatal("synthetic fixture marker required", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config c3RSSConfig
	if err := jsoncodec.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(config.Database) != "synthetic-clone.db" || (config.Arm != "baseline" && config.Arm != "candidate") {
		t.Fatal("invalid synthetic RSS configuration")
	}
	db, err := sqlite.Open(config.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err := db.SQL.QueryRow(`SELECT name FROM households WHERE id=?`, config.Household.String()).Scan(&name); err != nil || name != "Synthetic review pipeline" {
		t.Fatal("synthetic household required", name, err)
	}
	f := reviewFullPipelineFixture{db: db, dbPath: config.Database, householdID: config.Household, now: &config.Now}
	enableReviewSQLMetrics(t, f)
	repository := sqlite.NewRepository(db).WithBoundedReviewWindow()
	if config.Arm == "candidate" {
		repository = repository.WithReviewNoChangeCandidate()
	}
	f.service = NewService(repository)
	f.service.setClock(func() time.Time { return *f.now })
	// Required same-session bootstrap is deliberately outside the sampled region.
	if _, err := f.service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	debug.FreeOSMemory()
	result := c3RSSResult{Arm: config.Arm, PID: os.Getpid()}
	result.ReadyRSSKiB, result.ReadyHWMKiB, err = c3ProcessRSS(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ready"), []byte("ready"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	if err := c3WaitFile(ctx, filepath.Join(dir, "start")); err != nil {
		t.Fatal(err)
	}
	reviewPerfStats.reset()
	for range 3 {
		*f.now = f.now.Add(time.Nanosecond)
		started := time.Now()
		refresh, err := f.service.ReconcileInvestmentReviewsV2(ctx)
		result.Times = append(result.Times, time.Since(started))
		if err != nil || refresh.DayCount != 0 {
			t.Fatal("RSS warm arm recomputed", refresh, err)
		}
	}
	// End sampling before snapshot formatting/JSON reporting allocations.
	if err := os.WriteFile(filepath.Join(dir, "region-done"), []byte("done"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := c3WaitFile(ctx, filepath.Join(dir, "sampled")); err != nil {
		t.Fatal(err)
	}
	stats := reviewPerfStats.snapshot(0)
	if config.Arm == "candidate" && stats.writes != 6 {
		t.Fatal("candidate did not use three metadata-only confirmations", stats.summary())
	}
	result.SQL = stats.summary()
	_, result.EndHWMKiB, err = c3ProcessRSS(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	raw, err = jsoncodec.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "done.tmp"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "done.tmp"), filepath.Join(dir, "done.json")); err != nil {
		t.Fatal(err)
	}
	if err := c3WaitFile(ctx, filepath.Join(dir, "ack")); err != nil {
		t.Fatal(err)
	}
}

func c3MeasureRSSChild(t *testing.T, f reviewFullPipelineFixture, arm string) c3RSSResult {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "synthetic.marker"), []byte("Nestworth synthetic C3 RSS fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	raw, err := jsoncodec.Marshal(c3RSSConfig{Database: f.dbPath, Household: f.householdID, Now: *f.now, Arm: arm})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestReviewNoChangeRSSChild$", "-test.v")
	child.Env = append(os.Environ(), "NW_C3_RSS_CHILD=1", "NW_C3_RSS_DIR="+dir)
	var output bytes.Buffer
	child.Stdout, child.Stderr = &output, &output
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	defer func() {
		if !waited {
			_ = child.Process.Kill()
			<-done
		}
	}()
	readyTicker := time.NewTicker(time.Millisecond)
	defer readyTicker.Stop()
	waitFor := func(name string) {
		for {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				return
			} else if !os.IsNotExist(err) {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				waited = true
				t.Fatalf("RSS child exited before %s: %v\n%s", name, err, output.String())
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-readyTicker.C:
			}
		}
	}
	waitFor("ready")
	peak, _, err := c3ProcessRSS(child.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "start"), []byte("start"), 0600); err != nil {
		t.Fatal(err)
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	polls := 0
	for {
		rss, _, err := c3ProcessRSS(child.Process.Pid)
		if err != nil {
			t.Fatal(err)
		}
		polls++
		if rss > peak {
			peak = rss
		}
		if _, err := os.Stat(filepath.Join(dir, "region-done")); err == nil {
			break
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			waited = true
			t.Fatalf("RSS child exited before done: %v\n%s", err, output.String())
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "sampled"), []byte("sampled"), 0600); err != nil {
		t.Fatal(err)
	}
	waitFor("done.json")
	raw, err = os.ReadFile(filepath.Join(dir, "done.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result c3RSSResult
	if err := jsoncodec.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	result.SampledPeakRSSKiB, result.Polls = peak, polls
	if err := os.WriteFile(filepath.Join(dir, "ack"), []byte("ack"), 0600); err != nil {
		t.Fatal(err)
	}
	err = <-done
	waited = true
	if err != nil {
		t.Fatalf("RSS child failed: %v\n%s", err, output.String())
	}
	return result
}

func TestReviewNoChangeRSSPerformanceEvidence(t *testing.T) {
	if os.Getenv("NW_C3_PERF_EVIDENCE") != "1" {
		t.Skip("opt-in synthetic RSS evidence")
	}
	if runtime.GOOS != "linux" {
		t.Skip("sampled RSS uses Linux /proc; no native RSS claim")
	}
	for _, size := range c3PerfSizes(t) {
		t.Run(size, func(t *testing.T) {
			source := newReviewPerfFixture(t, c3RefreshConfig(size))
			logReviewPerfManifest(t, source, c3RefreshConfig(size))
			if _, err := source.service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
				t.Fatal(err)
			}
			for sample := 0; sample < 3; sample++ {
				arms := []string{"baseline", "candidate"}
				if sample%2 == 1 {
					arms[0], arms[1] = arms[1], arms[0]
				}
				projections := map[string]map[string][]string{}
				for _, arm := range arms {
					f := cloneReviewPerfFixture(t, source)
					if err := f.db.Close(); err != nil {
						t.Fatal(err)
					}
					result := c3MeasureRSSChild(t, f, arm)
					raw, _ := jsoncodec.Marshal(result)
					t.Logf("rss size=%s pair=%d result=%s (sampled post-setup VmRSS, 1ms polling; lifetime HWM includes bootstrap; child timings are not main latency samples)", size, sample+1, raw)
					db, err := sqlite.Open(f.dbPath)
					if err != nil {
						t.Fatal(err)
					}
					f.db = db
					*f.now = f.now.Add(3 * time.Nanosecond)
					f.service = NewService(sqlite.NewRepository(db))
					f.service.setClock(func() time.Time { return *f.now })
					projections[arm] = noChangeProjectionRows(t, db)
					capture, err := sqlite.NewRepository(db).CaptureReviewProjection(t.Context(), f.householdID, *f.now)
					if err != nil {
						t.Fatal(err)
					}
					stage, err := PrepareReviewPublication(t.Context(), capture, *f.now, f.service.FXProviderKey())
					if err != nil {
						t.Fatal(err)
					}
					assertReviewFullOracle(t, f, stage)
					if err := db.Close(); err != nil {
						t.Fatal(err)
					}
					if err := os.Remove(f.dbPath); err != nil {
						t.Fatal(err)
					}
				}
				if !reflect.DeepEqual(projections["baseline"], projections["candidate"]) {
					t.Fatal("RSS arms differ")
				}
			}
		})
	}
}
