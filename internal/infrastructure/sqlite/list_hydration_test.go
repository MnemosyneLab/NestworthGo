package sqlite

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestListActivityPageHydrationQueryCountIsBounded(t *testing.T) {
	database, repository, household, account, _ := seedPortfolioRepository(t)
	ctx := context.Background()
	const rows = 50
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < rows; i++ {
		activityID := domain.NewActivityID()
		effectID := domain.NewActivityEffectID()
		when := now.Add(time.Duration(i) * time.Hour)
		if _, err := database.SQL.ExecContext(ctx, `INSERT INTO activities(id, household_id, kind, reason, effective_at, effective_local_date, created_at) VALUES(?, ?, 'cash_in', 'contribution', ?, ?, ?)`,
			activityID.String(), household.ID.String(), formatTimestamp(when), when.Format("2006-01-02"), formatTimestamp(when)); err != nil {
			t.Fatalf("insert activity %d: %v", i, err)
		}
		if _, err := database.SQL.ExecContext(ctx, `INSERT INTO activity_effects(id, activity_id, sequence, role, direction, target, classification, account_id, amount, currency) VALUES(?, ?, 1, 'principal', 'added', 'account_cash', 'income', ?, '10', 'CNY')`,
			effectID.String(), activityID.String(), account.ID.String()); err != nil {
			t.Fatalf("insert effect %d: %v", i, err)
		}
	}

	counter := &countingQueryer{queryer: database.SQL}
	page, err := listActivityPageQuery(ctx, counter, household.ID, domain.ActivityQuery{Limit: rows})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Activities) != rows {
		t.Fatalf("page size = %d, want %d", len(page.Activities), rows)
	}
	if counter.queries > 5 {
		t.Fatalf("ListActivityPage query count = %d, want at most 1 parent + 4 batched child queries", counter.queries)
	}
	if page.Activities[0].Effects == nil || page.Activities[rows-1].Effects == nil {
		t.Fatalf("effects were not hydrated: first=%+v last=%+v", page.Activities[0].Effects, page.Activities[rows-1].Effects)
	}
	if page.Activities[0].ID.String() == page.Activities[rows-1].ID.String() {
		t.Fatal("page collapsed to a single activity")
	}
	_ = repository
}

func TestListActivityPageKeysetDoesNotSkipIdenticalTimestamps(t *testing.T) {
	database, _, household, account, _ := seedPortfolioRepository(t)
	ctx := context.Background()
	const total = 51
	when := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	wantIDs := make(map[string]struct{}, total)
	for i := 0; i < total; i++ {
		activityID := domain.NewActivityID()
		wantIDs[activityID.String()] = struct{}{}
		if _, err := database.SQL.ExecContext(ctx, `INSERT INTO activities(id, household_id, kind, reason, effective_at, effective_local_date, created_at) VALUES(?, ?, 'cash_in', 'contribution', ?, '2026-01-01', ?)`,
			activityID.String(), household.ID.String(), formatTimestamp(when), formatTimestamp(when)); err != nil {
			t.Fatalf("insert activity %d: %v", i, err)
		}
		if _, err := database.SQL.ExecContext(ctx, `INSERT INTO activity_effects(id, activity_id, sequence, role, direction, target, classification, account_id, amount, currency) VALUES(?, ?, 1, 'principal', 'added', 'account_cash', 'income', ?, '10', 'CNY')`,
			domain.NewActivityEffectID().String(), activityID.String(), account.ID.String()); err != nil {
			t.Fatalf("insert effect %d: %v", i, err)
		}
	}

	first, err := listActivityPageQuery(ctx, database.SQL, household.ID, domain.ActivityQuery{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Activities) != 50 || !first.HasMore || first.Next == nil {
		t.Fatalf("first page = len=%d hasMore=%v next=%v", len(first.Activities), first.HasMore, first.Next)
	}
	second, err := listActivityPageQuery(ctx, database.SQL, household.ID, domain.ActivityQuery{Limit: 50, After: first.Next})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Activities) != 1 || second.HasMore || second.Next != nil {
		t.Fatalf("second page = len=%d hasMore=%v next=%v", len(second.Activities), second.HasMore, second.Next)
	}

	seen := make(map[string]struct{}, total)
	order := make([]string, 0, total)
	for _, activity := range append(append([]domain.Activity{}, first.Activities...), second.Activities...) {
		id := activity.ID.String()
		if _, duplicate := seen[id]; duplicate {
			t.Fatalf("duplicate activity %s across pages", id)
		}
		if !activity.EffectiveAt.Equal(when) || !activity.CreatedAt.Equal(when) {
			t.Fatalf("activity %s timestamps drifted: effective=%v created=%v", id, activity.EffectiveAt, activity.CreatedAt)
		}
		seen[id] = struct{}{}
		order = append(order, id)
	}
	if len(seen) != total {
		t.Fatalf("paged %d unique activities, want %d (skipped %d)", len(seen), total, total-len(seen))
	}
	for id := range wantIDs {
		if _, ok := seen[id]; !ok {
			t.Fatalf("skipped activity %s", id)
		}
	}
	for index := 1; index < len(order); index++ {
		if order[index-1] <= order[index] {
			t.Fatalf("keyset order is not id DESC at %d: %s then %s", index, order[index-1], order[index])
		}
	}
}

func TestListDailyValuationSnapshotsHydrationQueryCountIsBounded(t *testing.T) {
	database, repository, household, account, _ := seedPortfolioRepository(t)
	ctx := context.Background()
	const days = 365
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < days; i++ {
		localDate := start.AddDate(0, 0, i).Format("2006-01-02")
		snapshotID := domain.NewDailyValuationSnapshotID()
		itemID := domain.NewDailyValuationSnapshotItemID()
		cutoff := start.AddDate(0, 0, i).Add(24*time.Hour - time.Nanosecond)
		if _, err := database.SQL.ExecContext(ctx, `INSERT INTO daily_valuation_snapshots(id, household_id, local_date, cutoff_at, revision, content_hash, assets_amount, liabilities_amount, net_worth_amount, currency, complete, component_count, missing_count, generation_reason, created_at) VALUES(?, ?, ?, ?, 1, 'v2:test', '1', '0', '1', 'CNY', 1, 1, 0, 'test', ?)`,
			snapshotID.String(), household.ID.String(), localDate, formatTimestamp(cutoff), formatTimestamp(cutoff)); err != nil {
			t.Fatalf("insert snapshot %s: %v", localDate, err)
		}
		if _, err := database.SQL.ExecContext(ctx, `INSERT INTO daily_valuation_snapshot_items(id, snapshot_id, account_id, native_amount, native_currency, base_amount, base_currency, complete) VALUES(?, ?, ?, '1', 'CNY', '1', 'CNY', 1)`,
			itemID.String(), snapshotID.String(), account.ID.String()); err != nil {
			t.Fatalf("insert snapshot item %s: %v", localDate, err)
		}
	}

	counter := &countingQueryer{queryer: database.SQL}
	snapshots, err := listDailyValuationSnapshotsQuery(ctx, counter, household.ID, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != days {
		t.Fatalf("snapshot count = %d, want %d", len(snapshots), days)
	}
	if counter.queries > 2 {
		t.Fatalf("ListDailyValuationSnapshots query count = %d, want at most 1 parent + 1 batched items query", counter.queries)
	}
	if len(snapshots[0].Items) != 1 || len(snapshots[days-1].Items) != 1 {
		t.Fatalf("items were not hydrated: first=%d last=%d", len(snapshots[0].Items), len(snapshots[days-1].Items))
	}
	if snapshots[0].LocalDate != "2025-01-01" || snapshots[days-1].LocalDate != "2025-12-31" {
		t.Fatalf("snapshot order = %s ... %s", snapshots[0].LocalDate, snapshots[days-1].LocalDate)
	}
	_ = repository
}

func TestBatchedChildLookupsUseExistingIndexes(t *testing.T) {
	database, _, household, account, _ := seedPortfolioRepository(t)
	ctx := context.Background()
	activityID := domain.NewActivityID()
	if _, err := database.SQL.ExecContext(ctx, `INSERT INTO activities(id, household_id, kind, reason, effective_at, effective_local_date, created_at) VALUES(?, ?, 'cash_in', 'contribution', ?, '2026-01-01', ?)`,
		activityID.String(), household.ID.String(), formatTimestamp(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)), formatTimestamp(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SQL.ExecContext(ctx, `INSERT INTO activity_effects(id, activity_id, sequence, role, direction, target, classification, account_id, amount, currency) VALUES(?, ?, 1, 'principal', 'added', 'account_cash', 'income', ?, '10', 'CNY')`,
		domain.NewActivityEffectID().String(), activityID.String(), account.ID.String()); err != nil {
		t.Fatal(err)
	}
	snapshotID := domain.NewDailyValuationSnapshotID()
	if _, err := database.SQL.ExecContext(ctx, `INSERT INTO daily_valuation_snapshots(id, household_id, local_date, cutoff_at, revision, content_hash, currency, complete, component_count, missing_count, generation_reason, created_at) VALUES(?, ?, '2026-01-01', ?, 1, 'v2:test', 'CNY', 1, 0, 0, 'test', ?)`,
		snapshotID.String(), household.ID.String(), formatTimestamp(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)), formatTimestamp(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))); err != nil {
		t.Fatal(err)
	}

	assertPlanUsesIndex(t, database, `EXPLAIN QUERY PLAN SELECT id FROM activity_effects WHERE activity_id IN (?, ?)`, []any{activityID.String(), activityID.String()}, "idx_activity_effects_activity")
	assertPlanUsesIndex(t, database, `EXPLAIN QUERY PLAN SELECT id FROM daily_valuation_snapshot_items WHERE snapshot_id IN (?, ?)`, []any{snapshotID.String(), snapshotID.String()}, "idx_daily_valuation_items_snapshot")
}

func assertPlanUsesIndex(t *testing.T, database *DB, statement string, args []any, index string) {
	t.Helper()
	rows, err := database.SQL.Query(statement, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var selectID, order, from int
		var detail string
		if err := rows.Scan(&selectID, &order, &from, &detail); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&plan, "%s\n", detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.String(), index) {
		t.Fatalf("query plan %q does not use %s", plan.String(), index)
	}
}
