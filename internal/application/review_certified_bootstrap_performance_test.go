//go:build perf

package application

import (
	"encoding/json"
	"os"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// Reopen an existing C2 ledger whose complete derived namespace has no episode
// history. Both arms must first prepare from full sources; the candidate may
// establish its new private authority only after replacing every derived field.
func TestReviewCertifiedHistoryFreeBootstrapEvidence(t *testing.T) {
	if os.Getenv("NW_C3_CERTIFIED_EVIDENCE") != "1" {
		t.Skip("opt-in synthetic bootstrap evidence")
	}
	for _, size := range c3PerfSizes(t) {
		t.Run(size, func(t *testing.T) {
			config := c3RefreshConfig(size)
			source := newReviewPerfFixture(t, config)
			logReviewPerfManifest(t, source, config)
			spec := syntheticHistoricalReviewSpec()
			spec.Predicate.All[1].Leaf.Value = "0"
			spec, err := spec.Normalize()
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(spec)
			if err != nil {
				t.Fatal(err)
			}
			hash, err := spec.SemanticHash()
			if err != nil {
				t.Fatal(err)
			}
			// Synthetic fixture construction before any daily projection exists.
			if _, err := source.db.SQL.Exec(`UPDATE investment_review_rule_versions SET canonical_spec_json=?,semantic_hash=?`, string(raw), hash); err != nil {
				t.Fatal(err)
			}
			source.service = NewService(sqlite.NewRepository(source.db).WithFullReviewRecompute())
			source.service.fxProviderKey = "synthetic-pinned"
			source.service.setClock(func() time.Time { return *source.now })
			if _, err := source.service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
				t.Fatal(err)
			}
			var episodes int
			if err := source.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_episodes`).Scan(&episodes); err != nil || episodes != 0 {
				t.Fatal("fixture has lifecycle history", episodes, err)
			}
			for sample := range reviewPerfSampleCount(t, 1) {
				rowsByArm := map[string]map[string][]string{}
				order := []string{"C2", "certified"}
				if sample%2 == 1 {
					order[0], order[1] = order[1], order[0]
				}
				for _, arm := range order {
					f := cloneReviewPerfFixture(t, source)
					enableReviewSQLMetrics(t, f)
					r := sqlite.NewRepository(f.db).WithBoundedReviewWindow()
					if arm == "certified" {
						r = r.WithCertifiedReviewCandidate()
					}
					f.service = NewService(r)
					f.service.fxProviderKey = "synthetic-pinned"
					f.service.setClock(func() time.Time { return *f.now })
					c, err := r.CaptureReviewProjection(t.Context(), f.householdID, *f.now)
					if err != nil {
						t.Fatal(err)
					}
					if arm == "certified" {
						if !c.Certified.MayBootstrap || c.Certified.MayCertify || c.Certified.MayReuse {
							t.Fatal("invalid bootstrap authority")
						}
						c.Certified.Provider = f.service.reviewProviderSnapshot()
					}
					p, err := PrepareReviewPublication(t.Context(), c, *f.now, "synthetic-pinned")
					if err != nil {
						t.Fatal(err)
					}
					assertReviewFullOracle(t, f, p)
					runtime.GC()
					var b, a runtime.MemStats
					runtime.ReadMemStats(&b)
					reviewPerfStats.reset()
					started := time.Now()
					got, err := f.service.ReconcileInvestmentReviewsV2(t.Context())
					elapsed := time.Since(started)
					runtime.ReadMemStats(&a)
					if err != nil || got.DayCount == 0 {
						t.Fatal("bootstrap did not fully recompute", got, err)
					}
					t.Logf("history-free-bootstrap size=%s arm=%s n=%d elapsed=%s allocatedBytes=%d allocs=%d publishedPairDays=%d %s", size, arm, sample+1, elapsed, a.TotalAlloc-b.TotalAlloc, a.Mallocs-b.Mallocs, got.DayCount, reviewPerfStats.snapshot(0).summary())
					if arm == "certified" {
						_, ok, err := r.ConfirmCertifiedReviewNoChange(t.Context(), f.householdID, *f.now, func() time.Time { return *f.now }, f.service.reviewProviderSnapshot())
						if err != nil || !ok {
							t.Fatal("full replacement did not certify", ok, err)
						}
					}
					rows := certifiedPerfRows(t, f)
					// The candidate deliberately replaces all evaluation state rows;
					// only this opaque publication counter differs (1 -> 2).
					for i, raw := range rows["investment_review_evaluation_state"] {
						var values []any
						if err := json.Unmarshal([]byte(raw), &values); err != nil {
							t.Fatal(err)
						}
						want := float64(1)
						if arm == "certified" {
							want = 2
						}
						if values[len(values)-1] != want {
							t.Fatal("unexpected publication counter", values[len(values)-1], want)
						}
						values[len(values)-1] = nil
						clean, err := json.Marshal(values)
						if err != nil {
							t.Fatal(err)
						}
						rows["investment_review_evaluation_state"][i] = string(clean)
					}
					rowsByArm[arm] = rows
					if err := f.db.Close(); err != nil {
						t.Fatal(err)
					}
					if err := os.Remove(f.dbPath); err != nil {
						t.Fatal(err)
					}
				}
				if !reflect.DeepEqual(rowsByArm["C2"], rowsByArm["certified"]) {
					t.Fatal("bootstrap durable financial/lifecycle output differs")
				}
			}
		})
	}
}
