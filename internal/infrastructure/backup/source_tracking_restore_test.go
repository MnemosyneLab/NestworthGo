package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestSourceTrackingRestoreRotatesLiveEpochBeforeResume(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "synthetic-source.db")
	db, err := sqlite.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.SQL.Exec(`INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000001','Synthetic','CNY','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	var epoch string
	var revision int64
	if err = db.SQL.QueryRow(`SELECT tracking_epoch,revision FROM financial_source_revision`).Scan(&epoch, &revision); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "complete.db")
	if err = db.SnapshotTo(t.Context(), archive); err != nil {
		t.Fatal(err)
	}
	db.Close()
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	pkg := Package{Manifest: NewManifest(time.Now().UTC(), data, []byte(`{}`), sqlite.EntityCounts{}), Database: data, Settings: []byte(`{}`)}
	live := filepath.Join(dir, "synthetic-restored.db")
	if err = InstallRestore(context.Background(), live, pkg, nil, RestoreClasses{}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	// Simulates startup after a crash at replacement-installed. Rotation must
	// happen even when no settings store/session hooks were supplied.
	if err = ReconcileOnStartup(live, nil); err != nil {
		t.Fatal(err)
	}
	current, err := sqlite.Open(live)
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	var got string
	var floor, count int64
	if err = current.SQL.QueryRow(`SELECT tracking_epoch,journal_floor FROM financial_source_revision`).Scan(&got, &floor); err != nil || got == epoch || floor != revision {
		t.Fatalf("stale restore token: %s %d %v", got, floor, err)
	}
	if err = current.SQL.QueryRow(`SELECT count(*) FROM financial_source_changes`).Scan(&count); err != nil || count != 0 {
		t.Fatal("restored log floor not applied")
	}
	if err = current.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Completed startup is idempotent. Normal restart does not rotate again.
	if err = ReconcileOnStartup(live, nil); err != nil {
		t.Fatal(err)
	}
	current.Close()
	again, err := sqlite.Open(live)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	var stable string
	if err = again.SQL.QueryRow(`SELECT tracking_epoch FROM financial_source_revision`).Scan(&stable); err != nil || stable != got {
		t.Fatal("normal restart changed restored epoch")
	}
	original, err := sqlite.OpenReadOnlyForVerify(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	var retained string
	if err = original.SQL.QueryRow(`SELECT tracking_epoch FROM financial_source_revision`).Scan(&retained); err != nil || retained != epoch {
		t.Fatal("restore mutated backup archive")
	}
}
