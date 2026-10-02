package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Keep the previous query as an independent selection oracle and benchmark
// baseline. Both paths use the production scanner and exact decimal parsing.
const legacyLatestValuesQuery = `SELECT av.id, av.account_id, av.value_kind, av.amount, av.currency, av.effective_at, av.created_at FROM account_values av JOIN accounts a ON a.id = av.account_id WHERE a.household_id = ? AND NOT EXISTS (SELECT 1 FROM account_values newer WHERE newer.account_id = av.account_id AND (newer.effective_at > av.effective_at OR (newer.effective_at = av.effective_at AND newer.created_at > av.created_at) OR (newer.effective_at = av.effective_at AND newer.created_at = av.created_at AND newer.rowid > av.rowid)))`

type legacyLatestValuesQueryer struct{ queryer }

func (q legacyLatestValuesQueryer) QueryContext(ctx context.Context, statement string, args ...any) (*sql.Rows, error) {
	if statement == latestAccountValuesQuery {
		statement = legacyLatestValuesQuery
	}
	return q.queryer.QueryContext(ctx, statement, args...)
}

func TestLatestValuesMatchLegacySelection(t *testing.T) {
	for _, tied := range []bool{false, true} {
		t.Run(fmt.Sprintf("tied=%t", tied), func(t *testing.T) {
			database, householdID, accountIDs := seedLatestValues(t, 6, 40, tied)
			ctx := context.Background()
			got, err := loadLatestValues(ctx, database.SQL, householdID)
			if err != nil {
				t.Fatal(err)
			}
			want, err := loadLatestValues(ctx, legacyLatestValuesQueryer{database.SQL}, householdID)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 6 || !reflect.DeepEqual(got, want) {
				t.Fatalf("latest values differ: got %+v, want %+v", got, want)
			}
			if _, exists := got[accountIDs[6]]; exists {
				t.Fatal("account without observations received a value")
			}
			other, err := loadLatestValues(ctx, database.SQL, domain.NewHouseholdID())
			if err != nil || len(other) != 0 {
				t.Fatalf("unrelated household: values=%v err=%v", other, err)
			}

			// Exercise hydration, archive filtering and the transaction-backed read
			// models, not only the SQL helper. The last valued account is archived.
			repository := NewRepository(database)
			for _, includeArchived := range []bool{false, true} {
				filter := domain.AccountFilter{IncludeArchived: includeArchived}
				records, err := repository.ListAccountRecords(ctx, householdID, filter)
				if err != nil {
					t.Fatal(err)
				}
				wantCount := 6
				if includeArchived {
					wantCount++
				}
				if len(records) != wantCount {
					t.Fatalf("record count=%d, want %d", len(records), wantCount)
				}
				for _, record := range records {
					if !reflect.DeepEqual(record.LatestValue, want[record.Account.ID]) {
						t.Fatalf("account %s value differs", record.Account.ID)
					}
				}
				snapshot, err := repository.ReadSnapshot(ctx, filter)
				if err != nil || !reflect.DeepEqual(snapshot.Accounts, records) {
					t.Fatalf("read snapshot differs: %v", err)
				}
				portfolio, err := repository.ReadPortfolioSnapshot(ctx, filter)
				if err != nil || !reflect.DeepEqual(portfolio.Accounts, records) {
					t.Fatalf("portfolio snapshot differs: %v", err)
				}
			}
		})
	}
}

func TestLatestValuesTimestampAndInsertionPrecedence(t *testing.T) {
	database, householdID, accounts := seedLatestValues(t, 1, 0, false)
	ctx := context.Background()
	// IDs run backwards so ordering by id would select the wrong observation.
	for index, observation := range []struct{ effective, created, amount string }{
		{"2026-01-02T00:00:00Z", "2026-01-02T00:00:00Z", "1"},
		{"2026-01-02T00:00:00Z", "2026-01-03T00:00:00Z", "2"},
		{"2026-01-02T00:00:00Z", "2026-01-03T00:00:00Z", "123.4567"},
		{"2026-01-02T00:00:00Z", "2026-01-01T00:00:00Z", "4"},
		{"2026-01-01T00:00:00Z", "2026-01-04T00:00:00Z", "5"},
	} {
		id := fmt.Sprintf("00000000-0000-4000-8000-%012d", 100-index)
		_, err := database.SQL.ExecContext(ctx, `INSERT INTO account_values(id, account_id, value_kind, amount, currency, effective_at, created_at) VALUES(?, ?, 'balance', ?, 'CNY', ?, ?)`, id, accounts[0].String(), observation.amount, observation.effective, observation.created)
		if err != nil {
			t.Fatal(err)
		}
	}
	record, err := NewRepository(database).AccountRecord(ctx, householdID, accounts[0])
	if err != nil {
		t.Fatal(err)
	}
	if record.LatestValue == nil || record.LatestValue.ID.String() != "00000000-0000-4000-8000-000000000098" || record.LatestValue.Amount.CanonicalAmount() != "123.4567" {
		t.Fatalf("wrong latest value: %+v", record.LatestValue)
	}
	assertPlanUsesIndex(t, database, "EXPLAIN QUERY PLAN "+latestAccountValuesQuery, []any{householdID.String()}, "idx_account_values_latest")
}

func BenchmarkLatestAccountValues(b *testing.B) {
	for _, scenario := range []struct {
		values int
		tied   bool
	}{{1, false}, {365, false}, {3650, false}, {365, true}} {
		b.Run(fmt.Sprintf("20_accounts/%d_values/tied=%t", scenario.values, scenario.tied), func(b *testing.B) {
			database, householdID, _ := seedLatestValues(b, 20, scenario.values, scenario.tied)
			for _, legacy := range []bool{true, false} {
				name := "indexed"
				var query queryer = database.SQL
				if legacy {
					name, query = "legacy", legacyLatestValuesQueryer{database.SQL}
				}
				b.Run(name, func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						values, err := loadLatestValues(context.Background(), query, householdID)
						if err != nil || len(values) != 20 {
							b.Fatalf("latest values: count=%d err=%v", len(values), err)
						}
					}
				})
			}
		})
	}
}

// Use an isolated current-schema database, including an empty account and an
// archived account. Backfilled values, timestamp ties, both tracking modes and
// all projection kinds exercise the existing selection contract.
func seedLatestValues(t testing.TB, accountCount, valueCount int, tied bool) (*DB, domain.HouseholdID, []domain.AccountID) {
	t.Helper()
	database, err := Open(filepath.Join(t.TempDir(), "latest-values.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	household, err := domain.NewHousehold("Latest values", "CNY", now)
	if err != nil {
		t.Fatal(err)
	}
	member, err := domain.NewMember(household.ID, "Owner", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewRepository(database).CreateOnboarding(ctx, household, []domain.Member{member}); err != nil {
		t.Fatal(err)
	}
	accounts := make([]domain.AccountID, accountCount+1)
	err = database.WithTx(ctx, func(tx *sql.Tx) error {
		for index := range accounts {
			accounts[index] = domain.NewAccountID()
			mode := "balance"
			if index%2 != 0 {
				mode = "manual_value"
			}
			var archived any
			if index == accountCount-1 {
				archived = formatTimestamp(now)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO accounts(id, household_id, name, account_type, balance_sheet_role, tracking_mode, default_currency, icon_key, created_at, updated_at, archived_at) VALUES(?, ?, ?, 'other', 'asset', ?, 'CNY', 'wallet', ?, ?, ?)`, accounts[index].String(), household.ID.String(), fmt.Sprintf("Account %02d", index), mode, formatTimestamp(now), formatTimestamp(now), archived); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO account_ownership(account_id, member_id, share_bps) VALUES(?, ?, 10000)`, accounts[index].String(), member.ID.String()); err != nil {
				return err
			}
			if index == accountCount {
				continue
			}
			for value := 0; value < valueCount; value++ {
				effective := now.AddDate(0, 0, (valueCount-value-1)/4)
				created := now.Add(time.Duration(value%4/2) * time.Minute)
				if tied {
					effective, created = now, now
				}
				id := fmt.Sprintf("00000000-0000-4000-8000-%012d", (index+1)*(valueCount+1)-value)
				projection := []string{"baseline", "event", "replay"}[value%3]
				if _, err := tx.ExecContext(ctx, `INSERT INTO account_values(id, account_id, value_kind, amount, currency, effective_at, created_at, projection_kind) VALUES(?, ?, ?, ?, 'CNY', ?, ?, ?)`, id, accounts[index].String(), mode, fmt.Sprintf("%d.1234", value), formatTimestamp(effective), formatTimestamp(created), projection); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return database, household.ID, accounts
}
