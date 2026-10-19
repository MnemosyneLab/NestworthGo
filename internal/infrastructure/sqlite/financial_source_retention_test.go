package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
)

func sourceRetentionState(t *testing.T, db *DB) (revision, floor, count int64) {
	t.Helper()
	if err := db.SQL.QueryRow(`SELECT revision,journal_floor,(SELECT count(*) FROM financial_source_changes) FROM financial_source_revision`).Scan(&revision, &floor, &count); err != nil {
		t.Fatal(err)
	}
	if count != revision-floor || count > financialJournalMaxRetained {
		t.Fatalf("unbounded/discontinuous journal: revision=%d floor=%d count=%d", revision, floor, count)
	}
	return
}
func advanceSourceTo(t *testing.T, db *DB, through int64) {
	t.Helper()
	from := sourceRevision(t, db.SQL)
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		for n := from + 1; n <= through; n++ {
			if _, err := tx.Exec(`UPDATE households SET name=?`, fmt.Sprintf("Synthetic %d", n)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
func TestFinancialSourceBoundedRetentionContinuityReopenAndStaleConsumer(t *testing.T) {
	for _, recursive := range []int{0, 1} {
		t.Run(fmt.Sprintf("recursive-%d", recursive), func(t *testing.T) {
			db := financialConflictDB(t)
			if _, err := db.SQL.Exec(fmt.Sprintf(`PRAGMA recursive_triggers=%d`, recursive)); err != nil {
				t.Fatal(err)
			}
			// A hypothetical consumer at this revision becomes stale after pruning.
			staleRevision := sourceRevision(t, db.SQL)
			advanceSourceTo(t, db, financialJournalMaxRetained)
			_, floor, count := sourceRetentionState(t, db)
			if floor != 0 || count != financialJournalMaxRetained {
				t.Fatal("pruned early or failed to reach bounded slack")
			}
			advanceSourceTo(t, db, financialJournalMaxRetained+1)
			_, floor, count = sourceRetentionState(t, db)
			if floor != financialJournalPruneEvery || count != financialJournalRetain || staleRevision >= floor {
				t.Fatalf("prune/fallback contract: floor=%d count=%d stale=%d", floor, count, staleRevision)
			}
			var min, max int64
			if err := db.SQL.QueryRow(`SELECT min(revision),max(revision) FROM financial_source_changes`).Scan(&min, &max); err != nil || min != floor+1 || max != financialJournalMaxRetained+1 {
				t.Fatalf("retained interval %d..%d err=%v", min, max, err)
			}
			// Caller rollback must undo the floor advance and all prefix deletions too.
			advanceSourceTo(t, db, financialJournalMaxRetained+financialJournalPruneEvery)
			before, beforeFloor, beforeCount := sourceRetentionState(t, db)
			err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
				if _, err := tx.Exec(`UPDATE households SET name='Synthetic rolled back pruning boundary'`); err != nil {
					return err
				}
				return fmt.Errorf("synthetic rollback")
			})
			if err == nil {
				t.Fatal("rollback missing")
			}
			after, afterFloor, afterCount := sourceRetentionState(t, db)
			if before != after || beforeFloor != afterFloor || beforeCount != afterCount {
				t.Fatal("rollback leaked retention state")
			}
			advanceSourceTo(t, db, 2*financialJournalRetain)
			revision, floor, count := sourceRetentionState(t, db)
			if floor != financialJournalRetain || count != financialJournalRetain {
				t.Fatalf("second retention window: %d/%d/%d", revision, floor, count)
			}
			if _, err := db.SQL.Exec(`PRAGMA recursive_triggers=ON`); err != nil {
				t.Fatal(err)
			}
			if err := db.Verify(t.Context()); err != nil {
				t.Fatal(err)
			}
			var epoch string
			if err := db.SQL.QueryRow(`SELECT tracking_epoch FROM financial_source_revision`).Scan(&epoch); err != nil {
				t.Fatal(err)
			}
			path := db.Path
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			gotRevision, gotFloor, gotCount := sourceRetentionState(t, reopened)
			var gotEpoch string
			if err = reopened.SQL.QueryRow(`SELECT tracking_epoch FROM financial_source_revision`).Scan(&gotEpoch); err != nil {
				t.Fatal(err)
			}
			if gotRevision != revision || gotFloor != floor || gotCount != count || gotEpoch != epoch {
				t.Fatal("reopen changed bounded retention checkpoint")
			}
		})
	}
}
func TestFinancialSourceRetentionFailureIsAtomic(t *testing.T) {
	for _, failure := range []struct{ name, trigger string }{
		{"delete-abort", `CREATE TRIGGER synthetic_prune_failure BEFORE DELETE ON financial_source_changes BEGIN SELECT RAISE(ABORT,'synthetic prune failure'); END`},
		{"delete-ignore", `CREATE TRIGGER synthetic_prune_failure BEFORE DELETE ON financial_source_changes BEGIN SELECT RAISE(IGNORE); END`},
		{"floor-ignore", `CREATE TRIGGER synthetic_prune_failure BEFORE UPDATE OF journal_floor ON financial_source_revision BEGIN SELECT RAISE(IGNORE); END`},
	} {
		t.Run(failure.name, func(t *testing.T) {
			db := financialConflictDB(t)
			advanceSourceTo(t, db, financialJournalMaxRetained)
			before, floor, count := sourceRetentionState(t, db)
			var name string
			if err := db.SQL.QueryRow(`SELECT name FROM households`).Scan(&name); err != nil {
				t.Fatal(err)
			}
			if _, err := db.SQL.Exec(failure.trigger); err != nil {
				t.Fatal(err)
			}
			if _, err := db.SQL.Exec(`UPDATE households SET name='Synthetic failed retention boundary'`); err == nil {
				t.Fatal("failed retention allowed source mutation")
			}
			after, afterFloor, afterCount := sourceRetentionState(t, db)
			var afterName string
			if err := db.SQL.QueryRow(`SELECT name FROM households`).Scan(&afterName); err != nil {
				t.Fatal(err)
			}
			if after != before || afterFloor != floor || afterCount != count || afterName != name {
				t.Fatal("failed prune leaked source/revision/floor/journal")
			}
			if _, err := db.SQL.Exec(`DROP TRIGGER synthetic_prune_failure`); err != nil {
				t.Fatal(err)
			}
			advanceSourceTo(t, db, before+1)
			if err := db.Verify(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestFinancialSourceIgnoredJournalPublicationAbortsSource(t *testing.T) {
	db := financialConflictDB(t)
	before := sourceRevision(t, db.SQL)
	if _, err := db.SQL.Exec(`CREATE TRIGGER synthetic_skip_journal BEFORE INSERT ON financial_source_changes BEGIN SELECT RAISE(IGNORE); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec(`INSERT OR IGNORE INTO app_configuration VALUES('synthetic','1')`); err == nil || !strings.Contains(err.Error(), "journal not published") {
		t.Fatalf("skipped journal not rejected: %v", err)
	}
	var count int
	if err := db.SQL.QueryRow(`SELECT count(*) FROM app_configuration`).Scan(&count); err != nil || count != 0 || sourceRevision(t, db.SQL) != before {
		t.Fatal("skipped journal leaked source or revision")
	}
	if _, err := db.SQL.Exec(`DROP TRIGGER synthetic_skip_journal`); err != nil {
		t.Fatal(err)
	}
	if err := db.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
}
