package continuousbackup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func localLTX(t *testing.T, m *Manager) (int, int64) {
	t.Helper()
	var count int
	var size int64
	err := filepath.WalkDir(filepath.Join(filepath.Dir(m.path), ".nestworth-replication"), func(path string, d fs.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".ltx") {
			info, err := d.Info()
			if err != nil {
				return err
			}
			count++
			size += info.Size()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return count, size
}

func TestConfirmedStreamMetadataReclaimedAcrossDisableCycles(t *testing.T) {
	m, db, _ := fixture(t)
	ctx := context.Background()
	for cycle := 1; cycle <= 3; cycle++ {
		value := fmt.Sprintf("cycle-%d", cycle)
		if _, err := db.SQL.Exec(`INSERT OR REPLACE INTO app_configuration(key,value) VALUES('cycle',?)`, value); err != nil {
			t.Fatal(err)
		}
		if err := m.Configure(ctx, testUpdate(true)); err != nil {
			t.Fatal(err)
		}
		waitState(t, m, "recent-backup-confirmed")
		if err := m.Configure(ctx, testUpdate(false)); err != nil {
			t.Fatal(err)
		}
		count, size := localLTX(t, m)
		t.Logf("cycle %d: local LTX count=%d bytes=%d", cycle, count, size)
		if count != 0 {
			t.Errorf("confirmed closed stream retained %d LTX files", count)
		}
		points, err := m.RecoveryPoints(ctx)
		if err != nil || len(points) < cycle {
			t.Fatalf("remote history lost: %v %v", points, err)
		}
		candidate, err := m.Stage(ctx, points[0])
		if err != nil {
			t.Fatal(err)
		}
		read, err := sqlite.OpenReadOnlyForVerify(candidate.Path)
		if err != nil {
			t.Fatal(err)
		}
		var got string
		err = read.SQL.QueryRow(`SELECT value FROM app_configuration WHERE key='cycle'`).Scan(&got)
		read.Close()
		os.RemoveAll(filepath.Dir(candidate.Path))
		if err != nil || got != value {
			t.Fatalf("remote restore=%q want %q: %v", got, value, err)
		}
		if _, err := db.SQL.Exec(`UPDATE app_configuration SET value='live-after-disable' WHERE key='cycle'`); err != nil {
			t.Fatal("live query did not resume", err)
		}
	}
	if err := m.Close(ctx); err != nil {
		t.Fatal(err)
	}
	count, size := localLTX(t, m)
	t.Logf("after Close: local LTX count=%d bytes=%d", count, size)
	if count != 0 {
		t.Errorf("Close retained %d confirmed LTX files", count)
	}
}

func TestFinalCloseReclaimsConfirmedMetadataAndPreservesRemoteRestore(t *testing.T) {
	m, db, _ := fixture(t)
	ctx := context.Background()
	if err := m.Configure(ctx, testUpdate(true)); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, "recent-backup-confirmed")
	if _, err := db.SQL.Exec(`INSERT INTO app_configuration(key,value) VALUES('final-close','last-write')`); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if count, size := localLTX(t, m); count != 0 || size != 0 {
		t.Fatalf("confirmed Close retained %d files / %d bytes", count, size)
	}
	reader, err := New(m.path, db)
	if err != nil {
		t.Fatal(err)
	}
	reader.factory = m.factory
	defer reader.Close(ctx)
	points, err := reader.RecoveryPoints(ctx)
	if err != nil || len(points) == 0 {
		t.Fatalf("Close lost remote history: %v %v", points, err)
	}
	candidate, err := reader.Stage(ctx, points[0])
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(filepath.Dir(candidate.Path))
	read, err := sqlite.OpenReadOnlyForVerify(candidate.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	var value string
	if err := read.SQL.QueryRow(`SELECT value FROM app_configuration WHERE key='final-close'`).Scan(&value); err != nil || value != "last-write" {
		t.Fatalf("final remote restore=%q %v", value, err)
	}
	if err := db.Verify(ctx); err != nil {
		t.Fatal("live query did not resume after Close", err)
	}
}

type failedStoreClose struct {
	store interface{ Close(context.Context) error }
	err   error
}

func (s failedStoreClose) Close(ctx context.Context) error {
	if err := s.store.Close(ctx); err != nil {
		return err
	}
	return s.err
}

func TestStreamMetadataPreservedAfterFailedClose(t *testing.T) {
	for _, closeErr := range []error{errors.New("synthetic close failure"), context.Canceled, context.DeadlineExceeded} {
		t.Run(closeErr.Error(), func(t *testing.T) {
			m, db, _ := fixture(t)
			if err := m.Configure(context.Background(), testUpdate(true)); err != nil {
				t.Fatal(err)
			}
			v := waitState(t, m, "recent-backup-confirmed")
			m.op.Lock()
			m.pauseWorkerLocked()
			path := m.replicaDB.MetaPath()
			m.replicaStore = failedStoreClose{m.replicaStore, closeErr}
			m.op.Unlock()
			_ = m.Configure(context.Background(), testUpdate(false))
			if _, err := os.Stat(path); err != nil {
				t.Fatal("failed close removed evidence", err)
			}
			c, _, _ := m.store.load()
			records, err := m.store.streams(c)
			if err != nil || records[v.StreamID].State != "open" {
				t.Fatal("failed close sealed stream", err)
			}
			if _, err := db.SQL.Exec(`INSERT INTO app_configuration(key,value) VALUES('after-close-error','intact')`); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStreamMetadataPreservedAfterFailedDrainAndUnsentBackup(t *testing.T) {
	for _, failure := range []string{"drain", "unsent", "seal"} {
		t.Run(failure, func(t *testing.T) {
			m, db, root := fixture(t)
			var fail, hang atomic.Bool
			m.factory = func(config) backend { return faultBackend{backend: fileBackend{root}, fail: &fail, hang: &hang} }
			if err := m.Configure(context.Background(), testUpdate(true)); err != nil {
				t.Fatal(err)
			}
			waitState(t, m, "recent-backup-confirmed")
			m.op.Lock()
			m.pauseWorkerLocked()
			path := m.replicaDB.MetaPath()
			m.op.Unlock()
			ctx := context.Background()
			switch failure {
			case "drain":
				conn, err := db.SQL.Conn(ctx)
				if err != nil {
					t.Fatal(err)
				}
				short, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
				err = m.Configure(short, testUpdate(false))
				cancel()
				conn.Close()
				if err == nil {
					t.Fatal("occupied pool did not fail drain")
				}
			case "unsent":
				if _, err := db.SQL.Exec(`INSERT INTO app_configuration(key,value) VALUES('unsent','evidence')`); err != nil {
					t.Fatal(err)
				}
				fail.Store(true)
				if err := m.BackupNow(ctx); err == nil {
					t.Fatal("upload unexpectedly succeeded")
				}
				if err := m.Configure(ctx, testUpdate(false)); err != nil {
					t.Fatal(err)
				}
			case "seal":
				if _, err := m.store.db.Exec(`CREATE TRIGGER reject_seal BEFORE UPDATE ON backup_streams BEGIN SELECT RAISE(FAIL,'synthetic seal failure'); END`); err != nil {
					t.Fatal(err)
				}
				if err := m.Configure(ctx, testUpdate(false)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatal("uncertain stop removed local evidence", err)
			}
			if _, err := db.SQL.Exec(`INSERT INTO app_configuration(key,value) VALUES('after-failed-stop-metadata','intact')`); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMetadataDeletionFailureDoesNotBreakDisable(t *testing.T) {
	m, db, _ := fixture(t)
	if err := m.Configure(context.Background(), testUpdate(true)); err != nil {
		t.Fatal(err)
	}
	v := waitState(t, m, "recent-backup-confirmed")
	path := filepath.Join(filepath.Dir(m.path), ".nestworth-replication", filepath.FromSlash(v.StreamID))
	parent := filepath.Dir(path)
	if err := os.Chmod(parent, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(parent, 0700)
	if err := m.Configure(context.Background(), testUpdate(false)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("deletion failure lost retained directory", err)
	}
	v, _ = m.View()
	if v.Enabled || v.State != "disabled" {
		t.Fatalf("failed reclamation blocked disable: %+v", v)
	}
	if err := db.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestMetadataCleanupPreservesNeighborsAndRejectsSymlinkAncestors(t *testing.T) {
	for _, link := range []string{"none", "metadata", "group", "stream"} {
		t.Run(link, func(t *testing.T) {
			m, _, _ := fixture(t)
			if err := m.Configure(context.Background(), testUpdate(true)); err != nil {
				t.Fatal(err)
			}
			v := waitState(t, m, "recent-backup-confirmed")
			// Close first while leaving evidence protected, then test filesystem
			// ownership against disposable directories with no live handles.
			m.op.Lock()
			m.pauseWorkerLocked()
			path := m.replicaDB.MetaPath()
			m.lastSyncSucceeded = false
			err := m.stopLocked(context.Background(), false)
			m.op.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			neighbor := filepath.Join(filepath.Dir(path), uuid.NewString(), "orphan.ltx")
			if err := os.MkdirAll(filepath.Dir(neighbor), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(neighbor, []byte("orphan evidence"), 0600); err != nil {
				t.Fatal(err)
			}
			safety := filepath.Join(filepath.Dir(m.path), "synthetic-safety.db")
			if err := os.WriteFile(safety, []byte("safety evidence"), 0600); err != nil {
				t.Fatal(err)
			}
			if link != "none" {
				source := path
				if link == "group" {
					source = filepath.Dir(path)
				}
				if link == "metadata" {
					source = filepath.Dir(filepath.Dir(path))
				}
				moved := source + "-retained"
				if err := os.Rename(source, moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(moved, source); err != nil {
					t.Fatal(err)
				}
			}
			err = m.removeStreamMetadata(path)
			if (err != nil) != (link != "none") {
				t.Fatalf("link=%s cleanup error=%v", link, err)
			}
			if link != "none" {
				if _, err := os.Stat(path); err != nil {
					t.Fatal("symlink target removed", err)
				}
			}
			if data, err := os.ReadFile(neighbor); err != nil || string(data) != "orphan evidence" {
				t.Fatal("neighbor changed", err)
			}
			if data, err := os.ReadFile(safety); err != nil || string(data) != "safety evidence" {
				t.Fatal("safety copy changed", err)
			}
			if err := m.removeStreamMetadata(safety); err == nil {
				t.Fatal("accepted business-directory path")
			}
			m.identity.StreamID = v.BackupID + "/../escape"
			if err := m.removeStreamMetadata(path); err == nil {
				t.Fatal("accepted non-UUID stream")
			}
		})
	}
}
