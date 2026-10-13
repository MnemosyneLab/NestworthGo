package application

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestHistoricalSnapshotsUseCivilLabelsAcrossMidnightTransitions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ zone, first, last, cutoff string }{
		{"America/Santiago", "2026-09-05", "2026-09-06", "2026-09-06T03:59:59.999Z"},
		{"America/Havana", "2026-10-31", "2026-11-01", "2026-11-01T03:59:59.999Z"},
		{"Pacific/Apia", "2011-12-29", "2011-12-31", "2011-12-30T09:59:59.999Z"},
		{"UTC", "2026-09-05", "2026-09-06", "2026-09-05T23:59:59.999Z"},
	} {
		t.Run(tc.zone, func(t *testing.T) {
			ctx := context.Background()
			s, db, owner, now := overviewFixture(t)
			first, _ := time.Parse("2006-01-02", tc.first)
			last, _ := time.Parse("2006-01-02", tc.last)
			*now = first.AddDate(0, 0, -2).Add(12 * time.Hour)
			a := overviewAccount(t, s, owner, "Synthetic cash", "bank_account", "asset", "balance", "CNY", "100")
			if _, err := s.StartHistory(ctx, tc.zone); err != nil {
				t.Fatal(err)
			}
			location, _ := time.LoadLocation(tc.zone)
			*now = time.Date(first.Year(), first.Month(), first.Day(), 12, 0, 0, 0, location)
			if _, err := s.AppendAccountValue(ctx, a.Account.ID, "120", ""); err != nil {
				t.Fatal(err)
			}
			*now = time.Date(last.Year(), last.Month(), last.Day(), 12, 0, 0, 0, location)
			if _, err := s.AppendAccountValue(ctx, a.Account.ID, "130", ""); err != nil {
				t.Fatal(err)
			}
			*now = last.AddDate(0, 0, 3).Add(12 * time.Hour)
			panorama, err := s.HistoricalOverview(ctx, tc.first, tc.last)
			if err != nil {
				t.Fatal(err)
			}
			for _, day := range []struct{ date, amount, cutoff string }{{tc.first, "120", tc.cutoff}, {tc.last, "130", panorama.Right.CutoffAt}} {
				snapshot, appended, err := s.BuildDailyValuationSnapshot(ctx, day.date)
				if err != nil || !appended {
					t.Fatalf("%s appended=%v err=%v", day.date, appended, err)
				}
				if snapshot.CutoffAt.UTC().Format(time.RFC3339Nano) != day.cutoff || snapshot.NetWorthAmount == nil || snapshot.NetWorthAmount.CanonicalAmount() != day.amount {
					t.Errorf("%s cutoff=%s net=%v; want %s / %s", day.date, snapshot.CutoffAt.UTC().Format(time.RFC3339Nano), snapshot.NetWorthAmount, day.cutoff, day.amount)
				}
				if len(snapshot.Items) != 1 || snapshot.Items[0].BaseAmountExact != day.amount {
					t.Errorf("exact stored items=%+v", snapshot.Items)
				}
			}
			count, err := s.RebuildHistoricalSnapshots(ctx, tc.first, tc.last)
			if err != nil || count != 0 {
				t.Fatalf("idempotent rebuild count=%d err=%v", count, err)
			}
			var changesBefore, changesAfter int
			if err := db.SQL.QueryRow(`SELECT total_changes()`).Scan(&changesBefore); err != nil {
				t.Fatal(err)
			}
			today := now.In(location).Format("2006-01-02")
			if _, _, err := s.BuildDailyValuationSnapshot(ctx, today); err == nil {
				t.Fatal("current day snapshot accepted")
			}
			if _, err := s.RebuildHistoricalSnapshots(ctx, tc.first, today); err == nil {
				t.Fatal("range through current day accepted")
			}
			if err := db.SQL.QueryRow(`SELECT total_changes()`).Scan(&changesAfter); err != nil {
				t.Fatal(err)
			}
			if changesBefore != changesAfter {
				t.Fatal("invalid range partially published")
			}
			if tc.zone == "Pacific/Apia" {
				if _, _, err := s.BuildDailyValuationSnapshot(ctx, "2011-12-30"); err == nil {
					t.Fatal("skipped civil label accepted")
				}
			}
			var storedCutoff, exact string
			if err := db.SQL.QueryRow(`SELECT cutoff_at, base_amount FROM daily_valuation_snapshots s JOIN daily_valuation_snapshot_items i ON i.snapshot_id = s.id WHERE local_date = ?`, tc.first).Scan(&storedCutoff, &exact); err != nil {
				t.Fatal(err)
			}
			if storedCutoff != tc.cutoff || exact != "120" {
				t.Fatalf("stored cutoff=%s exact=%s", storedCutoff, exact)
			}
			trend, err := s.NetWorthTrend(ctx, domain.TrendAllTime)
			if err != nil {
				t.Fatal(err)
			}
			seen := map[string]int{}
			for _, p := range trend.Points {
				seen[p.LocalDate]++
			}
			if seen[tc.first] != 1 || seen[tc.last] != 1 {
				t.Fatalf("civil labels duplicated or omitted: %v", seen)
			}
			if tc.zone == "Pacific/Apia" && seen["2011-12-30"] != 1 {
				t.Fatal("skipped date gap omitted")
			}
			if _, err := s.RebuildDerivedData(ctx); err != nil {
				t.Fatalf("full derived snapshot staging: %v", err)
			}
		})
	}
}

func TestCivilSnapshotPolicyInvalidationRebuildIsAppendOnlyAndIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, db, owner, now := overviewFixture(t)
	*now = time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	a := overviewAccount(t, s, owner, "Synthetic cash", "bank_account", "asset", "balance", "CNY", "100")
	origin, err := s.StartHistory(ctx, "America/Santiago")
	if err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 9, 5, 15, 0, 0, 0, time.UTC)
	if _, err := s.AppendAccountValue(ctx, a.Account.ID, "120", ""); err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	// Recreate the exact derived payload emitted by v3 on September 5.
	legacy, err := s.valueHistoricalSnapshot(ctx, &origin, time.Date(2026, 9, 5, 3, 59, 59, 999000000, time.UTC), "2026-09-05")
	if err != nil {
		t.Fatal(err)
	}
	legacy.ContentHash = "v3:synthetic-legacy-cutoff"
	legacy.ResolverPolicyVersion = "market-date-daily-summary-v3"
	if _, err := s.repository.SaveDailyValuationSnapshotAndMarkCompleted(ctx, legacy, *now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec(`UPDATE history_snapshot_state SET resolver_policy_version = 'market-date-daily-summary-v3', input_generation = 4, dirty_from = NULL, dirty_to = NULL`); err != nil {
		t.Fatal(err)
	}
	var factsBefore int
	if err := db.SQL.QueryRow(`SELECT (SELECT COUNT(*) FROM activities) + (SELECT COUNT(*) FROM account_values)`).Scan(&factsBefore); err != nil {
		t.Fatal(err)
	}
	path := db.Path
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	s = NewService(sqlite.NewRepository(reopened))
	s.setClock(func() time.Time { return *now })
	state, err := s.repository.DailySnapshotState(ctx, origin.HouseholdID)
	if err != nil || state.InputGeneration != 5 || state.DirtyFrom == nil || *state.DirtyFrom != "2026-09-04" {
		t.Fatalf("invalidation=%+v err=%v", state, err)
	}
	old, err := s.repository.ListDailyValuationSnapshots(ctx, origin.HouseholdID, time.Time{}, time.Time{})
	if err != nil || len(old) != 1 || old[0].Complete || old[0].ContentHash != legacy.ContentHash || old[0].NetWorthAmount.CanonicalAmount() != "100" {
		t.Fatalf("invalidated payload=%+v err=%v", old, err)
	}
	if err := s.ensureSnapshotCoverage(ctx, origin.HouseholdID, "2026-09-04", "2026-09-07"); err != nil {
		t.Fatal(err)
	}
	if err := s.ensureSnapshotCoverage(ctx, origin.HouseholdID, "2026-09-04", "2026-09-07"); err != nil {
		t.Fatal(err)
	}
	var revisions, factsAfter int
	if err := reopened.SQL.QueryRow(`SELECT COUNT(*) FROM daily_valuation_snapshots WHERE local_date='2026-09-05'`).Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if revisions != 2 {
		t.Fatalf("revisions=%d want 2", revisions)
	}
	if err := reopened.SQL.QueryRow(`SELECT (SELECT COUNT(*) FROM activities) + (SELECT COUNT(*) FROM account_values)`).Scan(&factsAfter); err != nil {
		t.Fatal(err)
	}
	if factsBefore != factsAfter {
		t.Fatalf("facts changed %d -> %d", factsBefore, factsAfter)
	}
	listed, err := s.repository.ListDailyValuationSnapshots(ctx, origin.HouseholdID, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, snapshot := range listed {
		if snapshot.LocalDate == "2026-09-05" && (!snapshot.Complete || snapshot.NetWorthAmount.CanonicalAmount() != "120" || snapshot.InputGeneration != 5 || snapshot.SupersedesID == nil || *snapshot.SupersedesID != legacy.ID) {
			t.Fatalf("rebuilt=%+v", snapshot)
		}
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	state, err = sqlite.NewRepository(again).DailySnapshotState(ctx, origin.HouseholdID)
	// The synthetic clock precedes real migration time: its future dirty suffix
	// remains pending. Reopening must not increment generation or move the prefix.
	if err != nil || state.InputGeneration != 5 || state.DirtyFrom == nil || *state.DirtyFrom != "2026-09-08" {
		t.Fatalf("reopen repeated migration=%+v err=%v", state, err)
	}
}
