//go:build perf

package application

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"testing"
)

// Profiles only the actual enabled reconcile call, excluding setup and oracles.
// Opt-in diagnostic; never selected by normal initialization or timing evidence.
func beginReviewDependencyProfile(t *testing.T, enabled bool, arm, scenario string, sample int) func() {
	t.Helper()
	dir := os.Getenv("NW_D2_PROFILE_DIR")
	if dir == "" || !enabled || arm != "certified" {
		return func() {}
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	stem := filepath.Join(dir, fmt.Sprintf("%s-%d", scenario, sample+1))
	write := func(name string) {
		f, err := os.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if err = pprof.Lookup("allocs").WriteTo(f, 0); err != nil {
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	write(stem + "-alloc-before.pprof")
	f, err := os.Create(stem + "-cpu.pprof")
	if err != nil {
		t.Fatal(err)
	}
	if err = pprof.StartCPUProfile(f); err != nil {
		t.Fatal(err)
	}
	return func() {
		pprof.StopCPUProfile()
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		write(stem + "-alloc-after.pprof")
	}
}
