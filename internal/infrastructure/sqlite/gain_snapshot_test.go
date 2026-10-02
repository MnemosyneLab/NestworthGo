package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type gainCountingQuery struct {
	queryer
	count       int
	beforeCosts func()
}

func (q *gainCountingQuery) QueryRowContext(ctx context.Context, s string, args ...any) *sql.Row {
	q.count++
	return q.queryer.QueryRowContext(ctx, s, args...)
}
func (q *gainCountingQuery) QueryContext(ctx context.Context, s string, args ...any) (*sql.Rows, error) {
	q.count++
	if q.beforeCosts != nil && strings.Contains(s, "SELECT e.holding_id, e.activity_id") {
		fn := q.beforeCosts
		q.beforeCosts = nil
		fn()
	}
	return q.queryer.QueryContext(ctx, s, args...)
}

func TestGainSnapshotQueryCountDoesNotGrowWithHoldings(t *testing.T) {
	db := seedCurrentGainFixture(t, filepath.Join(t.TempDir(), "gain.db"))
	ctx := context.Background()
	count := func() (int, int) {
		tx, e := db.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback()
		q := &gainCountingQuery{queryer: tx}
		snapshot, e := readGainSnapshotQuery(ctx, q)
		if e != nil {
			t.Fatal(e)
		}
		return q.count, len(snapshot.Portfolio.Holdings)
	}
	before, n := count()
	for i := 0; i < 500; i++ {
		instrument := fmt.Sprintf("10000000-0000-4000-8000-%012d", i)
		holding := fmt.Sprintf("20000000-0000-4000-8000-%012d", i)
		_, e := db.SQL.Exec(`INSERT INTO instruments(id,household_id,name,instrument_type,quote_currency,quote_source,icon_key,created_at,updated_at) SELECT ?,household_id,?,'stock','USD','manual','',created_at,updated_at FROM instruments LIMIT 1`, instrument, fmt.Sprintf("Synthetic %d", i))
		if e != nil {
			t.Fatal(e)
		}
		_, e = db.SQL.Exec(`INSERT INTO holdings(id,account_id,instrument_id,quantity,created_at,updated_at) SELECT ?,account_id,?,'0',created_at,updated_at FROM holdings LIMIT 1`, holding, instrument)
		if e != nil {
			t.Fatal(e)
		}
	}
	start := time.Now()
	after, m := count()
	t.Logf("gain snapshot queries: %d holdings=%d; %d holdings=%d; bulk read=%s", before, n, after, m, time.Since(start))
	if m != n+500 || after != before {
		t.Fatalf("query count grew with holdings: before=%d after=%d", before, after)
	}
}

func TestGainSnapshotSingleConnectionLetsWaitingWriterFinish(t *testing.T) {
	db := seedCurrentGainFixture(t, filepath.Join(t.TempDir(), "gain.db"))
	db.SQL.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, e := db.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	done := make(chan error, 1)
	waits := db.SQL.Stats().WaitCount
	q := &gainCountingQuery{queryer: tx, beforeCosts: func() {
		started := make(chan struct{})
		go func() {
			close(started)
			_, e := db.SQL.ExecContext(ctx, `UPDATE households SET name='Writer committed'`)
			done <- e
		}()
		<-started
		// Wait for the database pool, never for a commit while retaining its sole connection.
		for db.SQL.Stats().WaitCount == waits {
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			default:
				runtime.Gosched()
			}
		}
	}}
	inputs, e := readGainSnapshotQuery(ctx, q)
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	select {
	case e = <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-ctx.Done():
		t.Fatal("writer blocked after gain snapshot transaction released")
	}
	if inputs.Portfolio.Household.Name == "Writer committed" {
		t.Fatal("snapshot mixed in later write")
	}
	var name string
	if e = db.SQL.QueryRowContext(ctx, `SELECT name FROM households`).Scan(&name); e != nil || name != "Writer committed" {
		t.Fatalf("writer result=%s err=%v", name, e)
	}
}
