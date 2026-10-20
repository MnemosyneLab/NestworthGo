package sqlite

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func syntheticV20(t *testing.T, path string) *sql.DB {
	t.Helper()
	db := syntheticV19(t, path)
	if _, err := db.Exec(`PRAGMA recursive_triggers=1`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := applyV19Upgrade(t.Context(), tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestV20TemporalUpgradePreservesSourceAndVerifiedRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-v20.db")
	old := syntheticV20(t, path)
	var epoch string
	if err := old.QueryRow(`SELECT tracking_epoch FROM financial_source_revision`).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	ro, err := OpenReadOnlyForVerify(path)
	if err != nil {
		t.Fatal(err)
	}
	var version int
	if err := ro.SQL.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 20 {
		t.Fatal("readonly v20 migrated")
	}
	ro.Close()
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var got string
	var count, revision int
	if err := db.SQL.QueryRow(`SELECT tracking_epoch,revision FROM financial_source_revision`).Scan(&got, &revision); err != nil || got != epoch || revision != 0 {
		t.Fatal("temporal upgrade changed source identity/history")
	}
	if err := db.SQL.QueryRow(`SELECT count(*) FROM review_temporal_checkpoints`).Scan(&count); err != nil || count != 0 {
		t.Fatal("upgrade fabricated temporal history")
	}
	copies, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".nestworth-v20-recovery-*", "original-v20.sqlite"))
	if err != nil || len(copies) != 1 {
		t.Fatalf("recovery copies=%v err=%v", copies, err)
	}
	info, err := os.Stat(copies[0])
	if err != nil || info.Mode().Perm() != 0400 {
		t.Fatal("recovery was not frozen")
	}
	recovery, err := OpenReadOnlyForVerify(copies[0])
	if err != nil {
		t.Fatal(err)
	}
	defer recovery.Close()
	if err := recovery.SQL.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 20 {
		t.Fatal("recovery did not retain v20")
	}
	if _, err := Open(copies[0]); err == nil {
		t.Fatal("recovery opened writable")
	}
	if err := db.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestV20TemporalUpgradeFailureAndConcurrentWriterKeepOriginal(t *testing.T) {
	for _, mode := range []string{"flush-failure", "concurrent-write"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "synthetic-v20.db")
			db := syntheticV20(t, path)
			defer db.Close()
			flush := (*os.File).Sync
			if mode == "flush-failure" {
				flush = func(*os.File) error { return errors.New("synthetic flush failure") }
			}
			generation, err := prepareV20Recovery(t.Context(), db, path, flush, syncV20UpgradeDir)
			if mode == "flush-failure" {
				if err == nil {
					t.Fatal("unflushed recovery accepted")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				writer, err := sql.Open("sqlite", path+"?_pragma=foreign_keys%3d1&_pragma=recursive_triggers%3d1&_txlock=immediate")
				if err != nil {
					t.Fatal(err)
				}
				if _, err = writer.Exec(`UPDATE households SET name='Synthetic concurrent upgrade writer'`); err != nil {
					t.Fatal(err)
				}
				writer.Close()
				if err := commitV20Upgrade(t.Context(), db, generation); err == nil {
					t.Fatal("upgrade ignored concurrent commit")
				}
			}
			if err := verifySchemaVersion(t.Context(), db, 20); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='review_temporal_checkpoints'`).Scan(&count); err != nil || count != 0 {
				t.Fatal("failed upgrade left partial schema")
			}
		})
	}
}
