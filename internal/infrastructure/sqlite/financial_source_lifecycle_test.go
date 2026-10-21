package sqlite

import (
	"path/filepath"
	"testing"
)

// Current-schema regressions relocated from the retired pre21 migration suite.
func TestFinancialSourceSnapshotReadonlyResetAndIntegrity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.SQL.Exec(`INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000001','Synthetic','CNY','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	var epoch string
	if err = db.SQL.QueryRow(`SELECT tracking_epoch FROM financial_source_revision`).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var reopenedEpoch string
	if err := reopened.SQL.QueryRow(`SELECT tracking_epoch FROM financial_source_revision`).Scan(&reopenedEpoch); err != nil || reopenedEpoch != epoch {
		t.Fatal("normal reopen rotated source epoch")
	}
	reopened.Close()
	snapshot := filepath.Join(t.TempDir(), "complete.db")
	if err = db.SnapshotTo(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	ro, err := OpenReadOnlyForVerify(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var copyEpoch string
	if err = ro.SQL.QueryRow(`SELECT tracking_epoch FROM financial_source_revision`).Scan(&copyEpoch); err != nil || copyEpoch != epoch {
		t.Fatal("backup did not retain metadata")
	}
	if err = ro.ResetFinancialSourceTracking(t.Context()); err == nil {
		t.Fatal("readonly reset succeeded")
	}
	ro.Close()
	live, err := Open(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	before := sourceRevision(t, live.SQL)
	if err = live.ResetFinancialSourceTracking(t.Context()); err != nil {
		t.Fatal(err)
	}
	var fresh string
	var floor int64
	if err = live.SQL.QueryRow(`SELECT tracking_epoch,journal_floor FROM financial_source_revision`).Scan(&fresh, &floor); err != nil || fresh == epoch || floor != before {
		t.Fatalf("reset failed: %s %d %v", fresh, floor, err)
	}
	if err = live.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err = live.SQL.Exec(`UPDATE households SET name='Synthetic after restore'`); err != nil {
		t.Fatal(err)
	}
	if sourceRevision(t, live.SQL) != before+1 {
		t.Fatal("post-reset write untracked")
	}
	if _, err = live.SQL.Exec(`DELETE FROM financial_source_changes`); err != nil {
		t.Fatal(err)
	}
	if err = live.Verify(t.Context()); err == nil {
		t.Fatal("missing journal entry accepted")
	}
}

func TestFinancialSourcePruneAndFinanceOnlyImportEpoch(t *testing.T) {
	const household = "00000000-0000-4000-8000-000000000001"
	original, err := Open(filepath.Join(t.TempDir(), "synthetic-source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	financeSQL := `INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000001','Synthetic','CNY','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`
	if _, err = original.SQL.Exec(financeSQL); err != nil {
		t.Fatal(err)
	}
	var oldEpoch string
	if err = original.SQL.QueryRow(`SELECT tracking_epoch FROM financial_source_revision`).Scan(&oldEpoch); err != nil {
		t.Fatal(err)
	}
	if _, err = original.SQL.Exec(`UPDATE households SET name='Synthetic updated'`); err != nil {
		t.Fatal(err)
	}
	if err = original.PruneFinancialSourceChanges(t.Context(), household, 1); err != nil {
		t.Fatal(err)
	}
	if err = original.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = original.PruneFinancialSourceChanges(t.Context(), household, 3); err == nil {
		t.Fatal("pruned beyond current revision")
	}
	if err = original.PruneFinancialSourceChanges(t.Context(), household, 0); err == nil {
		t.Fatal("floor moved backwards")
	}
	// JSON has no importer in the current product. Its finance-only import
	// contract is tested at the storage boundary: importing into a fresh DB
	// never copies the source journal/epoch, even when financial IDs are equal.
	imported, err := Open(filepath.Join(t.TempDir(), "synthetic-import.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer imported.Close()
	if _, err = imported.SQL.Exec(financeSQL); err != nil {
		t.Fatal(err)
	}
	var newEpoch string
	if err = imported.SQL.QueryRow(`SELECT tracking_epoch FROM financial_source_revision`).Scan(&newEpoch); err != nil || newEpoch == oldEpoch {
		t.Fatal("import reused source epoch")
	}
	if err = imported.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
}
