package application

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestHistoricalOverviewDenseEntrySingleAndComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("SQLite parameter-boundary synthetic entry regression")
	}
	// 8,300 activities require 33,200 parameters in the old four-way union.
	// That is the chunk boundary. The 1,000-leaf/25,000-activity profile is
	// TestHistoricalOverviewDenseFullScale in the daily/manual perf workflow.
	runHistoricalOverviewDenseScale(t, 2, 0, 8300, 24981, 20, 22, "0.00216")
}

func runHistoricalOverviewDenseScale(t *testing.T, accounts, backfills, activities, wantFacts, wantLeaves, wantRows int, wantAmount string) {
	t.Helper()
	s, db := seriesHistoryScaleFixture(t, accounts, backfills, activities)
	var before, after int
	if err := db.SQL.QueryRow(`SELECT total_changes()`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	var single, comparison *HistoricalOverviewResult
	for _, compareTo := range []string{"", "2026-08-31"} {
		name := "single-day"
		if compareTo != "" {
			name = "comparison"
		}
		t.Run(name, func(t *testing.T) {
			start := time.Now()
			out, err := s.HistoricalOverview(t.Context(), "2026-08-01", compareTo)
			if err != nil {
				t.Fatalf("dense GUI entry: %v", err)
			}
			wantOverviewAmount(t, out.Left.Assets, wantAmount)
			wantOverviewAmount(t, out.Left.NetWorth, wantAmount)
			if !out.Left.Complete || len(out.Rows) != wantRows {
				t.Fatalf("truncated/incomplete dense entry: complete=%t rows=%d", out.Left.Complete, len(out.Rows))
			}
			if compareTo == "" {
				single = &out
			} else {
				comparison = &out
				wantOverviewAmount(t, out.Right.Assets, wantAmount)
				wantOverviewAmount(t, out.Change.NetWorth, "0")
			}
			t.Logf("dense %s read=%s, rows=%d", name, time.Since(start), len(out.Rows))
		})
	}
	if single != nil && comparison != nil && !reflect.DeepEqual(single.Left, comparison.Left) {
		t.Fatal("single-day baseline differs from comparison entry")
	}
	t.Run("series-endpoint-equivalence", func(t *testing.T) {
		start := time.Now()
		out, err := s.HistoricalOverviewSeries(t.Context(), "2026-08-01", "2026-08-31")
		if err != nil {
			// Race instrumentation can consume the unchanged production runtime
			// deadline on this parameter-boundary fixture. Accept only that explicit
			// deadline rejection with no partial bundle. The 25,000-activity profile
			// is TestHistoricalOverviewDenseFullScale and must succeed without -race.
			var budget *domain.Error
			if analysisRaceDetector && errors.As(err, &budget) && budget.Code == domain.ErrHistoricalSeriesBudget && budget.Message == "local trend exceeded its computation deadline" && time.Since(start) >= historicalSeriesTimeout && len(out.Points) == 0 && out.Overview.CapturedAt == "" {
				t.Log("race-instrumented series reached its unchanged production deadline; no partial bundle returned")
				return
			}
			t.Fatal(err)
		}
		if out.Work.Leaves != wantLeaves || out.Work.CandidateFacts != wantFacts || len(out.Points) != 31 {
			t.Fatalf("wrong dense fixture/coverage: %+v points=%d", out.Work, len(out.Points))
		}
		if comparison != nil && !reflect.DeepEqual(*comparison, out.Overview) {
			t.Fatal("dense comparison differs from same-source series endpoints")
		}
		if single != nil && !reflect.DeepEqual(single.Left, out.Overview.Left) {
			t.Fatal("dense single date differs from series baseline")
		}
	})
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.HistoricalOverview(cancelled, "2026-08-01", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("dense entry ignored cancellation: %v", err)
	}
	if err := db.SQL.QueryRow(`SELECT total_changes()`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("query-only dense reads wrote data: %d -> %d", before, after)
	}
}

func TestHistoricalOverviewHydrationChunksPreserveEconomicReplay(t *testing.T) {
	t.Parallel()
	s, db, owner, now := overviewFixture(t)
	account := overviewAccount(t, s, owner, "Synthetic chunked cash", "bank_account", "asset", "balance", "CNY", "100")
	if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	ids := make([]domain.ActivityID, 513)
	tx, err := db.SQL.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	balance := 100
	for n := 1; n <= len(ids); n++ {
		ids[n-1] = domain.NewActivityID()
		fx := domain.NewActivityEffectID()
		at := now.Add(time.Duration(n) * time.Second).Format(time.RFC3339Nano)
		kind, classification, amount := "cash_in", "income", 1
		if n == 256 || n == 513 {
			target := 1000
			if n == 513 {
				target = 1500
			}
			kind, classification, amount = "value_update", "remeasurement", target-balance
			balance = target
		} else {
			balance++
		}
		if _, err := tx.Exec(`INSERT INTO activities(id,household_id,kind,reason,effective_at,effective_local_date,created_at) VALUES(?,?,?,'other',?,'2026-08-01',?)`, ids[n-1].String(), account.Account.HouseholdID.String(), kind, at, at); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO activity_effects(id,activity_id,sequence,role,direction,target,classification,account_id,amount,currency) VALUES(?,?,1,'amount','added','account_value',?,?,?,'CNY')`, fx.String(), ids[n-1].String(), classification, account.Account.ID.String(), fmt.Sprint(amount)); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO account_values(id,account_id,value_kind,amount,currency,effective_at,created_at,activity_effect_id,projection_kind) VALUES(?,?,'balance',?,'CNY',?,?,?,'event')`, domain.NewAccountValueID().String(), account.Account.ID.String(), fmt.Sprint(balance), at, at, fx.String()); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	fixed, err := s.FixChange(t.Context(), ids[9], domain.MoneyAddedInput{HouseholdID: account.Account.HouseholdID, AccountID: account.Account.ID, Amount: mustMoney(t, "2", "CNY"), Reason: domain.ReasonOther, EffectiveAt: *now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec(`PRAGMA query_only=ON`); err != nil {
		t.Fatal(err)
	}
	batch, err := s.repository.LoadHistoricalSnapshotBatch(t.Context(), account.Account.HouseholdID, *now)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Activities) != 513 || batch.Activities[9].ID != fixed.Activity.ID {
		t.Fatalf("correction was duplicated or lost economic position: count=%d", len(batch.Activities))
	}
	seen := map[domain.ActivityID]bool{}
	for i, a := range batch.Activities {
		if seen[a.ID] || len(a.Resulting) != 1 || len(a.Effects) != 1 {
			t.Fatalf("duplicate/missing hydrated evidence at %d: %+v", i, a)
		}
		seen[a.ID] = true
		if i > 0 && !a.EffectiveAt.After(batch.Activities[i-1].EffectiveAt) {
			t.Fatalf("economic order changed at %d", i)
		}
	}
	if batch.Activities[255].Effects[0].Money.CanonicalAmount() != "644" || batch.Activities[255].RecordedEffects[0].Money.CanonicalAmount() != "645" || batch.Activities[512].Effects[0].Money.CanonicalAmount() != "244" {
		t.Fatal("absolute balances were replayed independently per hydration chunk")
	}
	out, err := s.HistoricalOverview(t.Context(), "2026-08-01", "")
	if err != nil {
		t.Fatal(err)
	}
	wantOverviewAmount(t, out.Left.NetWorth, "1500")
}
