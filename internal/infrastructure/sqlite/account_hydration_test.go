package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type hydrationQueryEvidence struct {
	queryer
	calls, returned int
	full            bool
}

func (q *hydrationQueryEvidence) QueryContext(ctx context.Context, statement string, args ...any) (*sql.Rows, error) {
	if strings.Contains(statement, "SELECT ao.account_id") || strings.HasPrefix(statement, "SELECT av.id") {
		if q.full {
			if start := strings.Index(statement, " AND a.id IN ("); start >= 0 {
				end := start + strings.Index(statement[start:], ")") + 1
				statement = statement[:start] + statement[end:]
				args = args[:1]
			}
		}
		q.calls++
		rows, err := q.queryer.QueryContext(ctx, "SELECT COUNT(*) FROM ("+statement+")", args...)
		if err != nil {
			return nil, err
		}
		var count int
		if !rows.Next() {
			rows.Close()
			return nil, fmt.Errorf("missing count")
		}
		if err := rows.Scan(&count); err != nil {
			rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		q.returned += count
	}
	return q.queryer.QueryContext(ctx, statement, args...)
}

func TestSelectedAccountHydration(t *testing.T) {
	ctx := context.Background()
	db, household, ids := seedLatestValues(t, 12, 8, true)
	// Make a selected subset with shared ownership, including an archived member.
	member, err := domain.NewMember(household, "Second owner", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.ExecContext(ctx, `INSERT INTO members(id, household_id, name, icon_key, created_at, updated_at, archived_at) SELECT ?, household_id, ?, icon_key, created_at, updated_at, created_at FROM members LIMIT 1`, member.ID.String(), member.Name); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids[:2] {
		if _, err := db.SQL.ExecContext(ctx, `UPDATE account_ownership SET share_bps=6000 WHERE account_id=?`, id.String()); err != nil {
			t.Fatal(err)
		}
		if _, err := db.SQL.ExecContext(ctx, `INSERT INTO account_ownership VALUES(?, ?, 4000)`, id.String(), member.ID.String()); err != nil {
			t.Fatal(err)
		}
	}
	missing := domain.NewMemberID()
	for _, tc := range []struct {
		name        string
		filter      domain.AccountFilter
		count, rows int
	}{
		{"household", domain.AccountFilter{IncludeArchived: true}, 13, 27},
		{"active", domain.AccountFilter{}, 12, 25},
		{"subset", domain.AccountFilter{MemberID: &member.ID}, 2, 6},
		{"shared", domain.AccountFilter{OwnershipScope: domain.OwnershipShared}, 2, 6},
		{"sole", domain.AccountFilter{OwnershipScope: domain.OwnershipSole}, 10, 19},
		{"empty", domain.AccountFilter{MemberID: &missing}, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selected := &hydrationQueryEvidence{queryer: db.SQL}
			got, err := listAccountRecords(ctx, selected, household, tc.filter)
			if err != nil {
				t.Fatal(err)
			}
			baseline := &hydrationQueryEvidence{queryer: db.SQL, full: true}
			want, err := listAccountRecords(ctx, baseline, household, tc.filter)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) || len(got) != tc.count {
				t.Fatalf("selected records differ from household oracle")
			}
			if selected.returned != tc.rows {
				t.Fatalf("rows=%d want %d", selected.returned, tc.rows)
			}
			if tc.count == 0 && selected.calls != 0 {
				t.Fatal("empty selection hydrated")
			}
			t.Logf("accounts=%d hydration queries=%d rows consumed/returned=%d baseline=%d", len(got), selected.calls, selected.returned, baseline.returned)
			snapshot, err := NewRepository(db).ReadSnapshot(ctx, tc.filter)
			if err != nil || !reflect.DeepEqual(snapshot.Accounts, got) {
				t.Fatalf("read snapshot differs: %v", err)
			}
			portfolio, err := NewRepository(db).ReadPortfolioSnapshot(ctx, tc.filter)
			if err != nil || !reflect.DeepEqual(portfolio.Accounts, got) {
				t.Fatalf("portfolio snapshot differs: %v", err)
			}
			for _, record := range got {
				detail, err := NewRepository(db).AccountRecord(ctx, household, record.Account.ID)
				if err != nil || !reflect.DeepEqual(detail, record) {
					t.Fatalf("detail differs: %v", err)
				}
			}
		})
	}
	if _, err := NewRepository(db).AccountRecord(ctx, domain.NewHouseholdID(), ids[0]); err == nil {
		t.Fatal("cross-household detail allowed")
	}
	owners, err := loadOwnership(ctx, db.SQL, domain.NewHouseholdID(), ids)
	if err != nil || len(owners) != 0 {
		t.Fatalf("cross-household owners: %v", err)
	}
	values, err := loadLatestValues(ctx, db.SQL, domain.NewHouseholdID(), ids)
	if err != nil || len(values) != 0 {
		t.Fatalf("cross-household values: %v", err)
	}
	for _, id := range ids[:1] {
		q := &hydrationQueryEvidence{queryer: db.SQL}
		if _, err := loadOwnership(ctx, q, household, []domain.AccountID{id}); err != nil {
			t.Fatal(err)
		}
		if _, err := loadLatestValues(ctx, q, household, []domain.AccountID{id}); err != nil {
			t.Fatal(err)
		}
		if q.returned != 3 {
			t.Fatalf("single-account rows=%d", q.returned)
		}
		t.Logf("single account: hydration rows=3 versus household=27")
	}
	assertPlanUsesIndex(t, db, "EXPLAIN QUERY PLAN "+latestAccountValuesQuery+" AND a.id IN (?)", []any{household.String(), ids[0].String()}, "idx_account_values_latest")
}

func TestSelectedAccountHydrationChunks(t *testing.T) {
	db, household, ids := seedLatestValues(t, accountHydrationChunkSize+1, 1, false)
	ctx := context.Background()
	q := &hydrationQueryEvidence{queryer: db.SQL}
	owners, err := loadOwnership(ctx, q, household, ids)
	if err != nil {
		t.Fatal(err)
	}
	values, err := loadLatestValues(ctx, q, household, ids)
	if err != nil {
		t.Fatal(err)
	}
	wantOwners, err := loadAllOwnership(ctx, db.SQL, household)
	if err != nil {
		t.Fatal(err)
	}
	wantValues, err := loadAllLatestValues(ctx, db.SQL, household)
	if err != nil {
		t.Fatal(err)
	}
	if q.calls != 4 || !reflect.DeepEqual(owners, wantOwners) || !reflect.DeepEqual(values, wantValues) {
		t.Fatalf("chunk results differ; queries=%d", q.calls)
	}
	empty := &hydrationQueryEvidence{queryer: db.SQL}
	if _, err := loadOwnership(ctx, empty, household, []domain.AccountID{}); err != nil {
		t.Fatal(err)
	}
	if _, err := loadLatestValues(ctx, empty, household, []domain.AccountID{}); err != nil {
		t.Fatal(err)
	}
	if empty.calls != 0 {
		t.Fatal("empty scope queried")
	}
}

// Whole-household queries are independent test oracles; production always uses
// explicit selected account IDs, including an empty selection.
func loadAllOwnership(ctx context.Context, query queryer, id domain.HouseholdID) (map[domain.AccountID][]domain.OwnershipShare, error) {
	return scanOwnershipRows(query.QueryContext(ctx, accountOwnershipQuery, id.String()))
}
func loadAllLatestValues(ctx context.Context, query queryer, id domain.HouseholdID) (map[domain.AccountID]*domain.AccountValue, error) {
	return scanLatestValuesRows(query.QueryContext(ctx, latestAccountValuesQuery, id.String()))
}
