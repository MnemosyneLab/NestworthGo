package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestFinancialContextAdmissionBeforeAllocation(t *testing.T) {
	db, _, _, _, _ := seedPortfolioRepository(t)
	for _, test := range []struct {
		name, query string
		rows, bytes int
	}{{"rows", `WITH RECURSIVE x(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM x WHERE n<10000) SELECT n FROM x`, 3, 100000}, {"bytes", `SELECT printf('%01000000d',1) AS oversized`, 100, 1024}} {
		t.Run(test.name, func(t *testing.T) {
			tx, err := db.SQL.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			q := &contextBudgetQuery{tx: tx, rows: test.rows, bytes: test.bytes}
			rows, err := q.QueryContext(t.Context(), test.query)
			if rows != nil {
				rows.Close()
				t.Fatal("oversized query opened allocation rows")
			}
			var e *domain.Error
			if !errors.As(err, &e) || string(e.Code) != "too_large" {
				t.Fatal(err)
			}
		})
	}
	tx, err := db.SQL.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	q := &contextBudgetQuery{tx: tx, rows: 10, bytes: 512}
	var value string
	err = q.QueryRowContext(t.Context(), `SELECT printf('%01000000d',1) AS oversized`).Scan(&value)
	if err == nil || q.err == nil || value != "" {
		t.Fatal("single-row allocation bypassed budget", err, q.err)
	}
}
func TestFinancialContextRepositoryReadSnapshotAndCancellation(t *testing.T) {
	db, repo, _, account, _ := seedPortfolioRepository(t)
	tx, err := db.SQL.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	q := &contextBudgetQuery{tx: tx, rows: contextInputRows, bytes: contextInputBytes}
	var name string
	if err = q.QueryRowContext(t.Context(), `SELECT name FROM accounts WHERE id=?`, account.ID.String()).Scan(&name); err != nil {
		t.Fatal(err)
	}
	writer, err := sql.Open("sqlite", db.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if _, err = writer.Exec(`UPDATE accounts SET name='new snapshot name' WHERE id=?`, account.ID.String()); err != nil {
		t.Fatal(err)
	}
	p, err := readPortfolioSnapshotQuery(t.Context(), q, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	if p.Accounts[0].Account.Name != name {
		t.Fatal("escaped read snapshot")
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err = db.SQL.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	inputs, err := repo.ReadFinancialContextInputs(t.Context(), false, []domain.AccountID{account.ID}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if inputs.Portfolio.Accounts[0].Account.Name != "new snapshot name" {
		t.Fatal("missed new committed state")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = repo.ReadFinancialContextInputs(ctx, false, nil, time.Now()); err == nil {
		t.Fatal("cancelled read accepted")
	}
}
func TestFinancialContextRejectsOversizedPersistedText(t *testing.T) {
	db, repo, _, account, _ := seedPortfolioRepository(t)
	if _, err := db.SQL.Exec(`UPDATE accounts SET note=? WHERE id=?`, strings.Repeat("x", contextInputBytes+1), account.ID.String()); err != nil {
		t.Fatal(err)
	}
	_, err := repo.ReadFinancialContextInputs(t.Context(), false, nil, time.Now())
	var e *domain.Error
	if !errors.As(err, &e) || string(e.Code) != "too_large" {
		t.Fatal(err)
	}
}

func BenchmarkFinancialContextInputs(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(benchmarkContextName(n), func(b *testing.B) {
			db, err := Open(b.TempDir() + "/bench.db")
			if err != nil {
				b.Fatal(err)
			}
			defer db.Close()
			repo := NewRepository(db)
			if _, err = db.SQL.Exec(`INSERT INTO households(id,singleton_key,name,base_currency,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000001',1,'Synthetic','USD','2026-09-01T00:00:00Z','2026-09-01T00:00:00Z')`); err != nil {
				b.Fatal(err)
			}
			if _, err = db.SQL.Exec(`WITH RECURSIVE x(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM x WHERE n<?) INSERT INTO instruments(id,household_id,name,instrument_type,quote_currency,quote_source,icon_key,created_at,updated_at) SELECT printf('00000000-0000-4000-8000-%012d',n+1),'00000000-0000-4000-8000-000000000001','Synthetic','crypto','USD','manual','crypto-generic','2026-09-01T00:00:00Z','2026-09-01T00:00:00Z' FROM x`, n); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err = repo.ReadFinancialContextInputs(context.Background(), false, nil, time.Now()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
func benchmarkContextName(n int) string {
	if n == 1000 {
		return "1000_instruments"
	}
	return "10000_instruments"
}
