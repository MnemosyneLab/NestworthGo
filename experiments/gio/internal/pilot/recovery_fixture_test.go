package pilot

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func makePendingRecovery(t *testing.T) Paths {
	t.Helper()
	root, paths := testPaths(t)
	t.Setenv("NESTWORTH_GIO_RUN_DIR", root)
	if err := PrepareRecovery(t.Context(), paths); err != nil {
		t.Fatal(err)
	}
	return paths
}

func TestDisposableRecoveryAgreesWithSharedStartup(t *testing.T) {
	ctx := t.Context()
	paths := makePendingRecovery(t)
	before := fixtureContents(t, paths)
	if err := Verify(ctx, paths); !errors.Is(err, ErrRecoveryPending) {
		t.Fatalf("read-only verification accepted an unfinished restore: %v", err)
	}
	if err := PrepareRecovery(ctx, paths); !errors.Is(err, os.ErrExist) || !maps.Equal(before, fixtureContents(t, paths)) {
		t.Fatalf("verification or a repeated prepare changed the pending restore: %v", err)
	}

	journal, err := backup.ReadJournal(paths.database)
	if err != nil {
		t.Fatal(err)
	}
	safetyBytes := before[journal.SafetyName]
	originalPath := filepath.Join(t.TempDir(), "original.db")
	if err := os.WriteFile(originalPath, []byte(safetyBytes), 0o600); err != nil {
		t.Fatal(err)
	}
	original, err := sqlite.OpenReadOnlyForVerifyContext(ctx, originalPath)
	if err != nil {
		t.Fatal(err)
	}
	if amount := recoveryReserve(t, original); amount != "99.5" {
		t.Fatalf("the safety copy did not preserve the pre-restore amount: %s", amount)
	}
	if err := original.Close(); err != nil {
		t.Fatal(err)
	}
	originalPrefs, err := settings.NewStore(paths.settings).Load()
	if err != nil || originalPrefs.Appearance != settings.AppearanceLight || originalPrefs.DateFormat != settings.DateFormatISO {
		t.Fatalf("fixture did not start with outgoing preferences: %+v, %v", originalPrefs, err)
	}

	_, reference := testPaths(t)
	if err := os.Mkdir(reference.data, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, contents := range before {
		if err := os.WriteFile(filepath.Join(reference.data, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Exercise the same lock, default reconciler, preference reload and open
	// boundary as cmd/nestworth/main.go on an independent synthetic copy.
	unlock, err := backup.AcquireInstanceLock(reference.database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unlock() })
	refStore := settings.NewStore(reference.settings)
	if err := backup.ReconcileOnStartup(reference.database, refStore); err != nil {
		t.Fatal(err)
	}
	refPrefs, err := refStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	refDB, err := sqlite.Open(reference.database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = refDB.Close() })
	if err := refStore.Attach(sqlite.NewConfigurationRepository(refDB)); err != nil {
		t.Fatal(err)
	}

	session, err := OpenSession(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	gotPrefs, err := settings.NewStore(paths.settings).Load()
	if err != nil || gotPrefs != refPrefs || gotPrefs.Appearance != settings.AppearanceDark || gotPrefs.Accent != settings.AccentMint || gotPrefs.DateFormat != settings.DateFormatDayFirst {
		t.Fatalf("restored preferences differ from the shared startup: %+v vs %+v, %v", gotPrefs, refPrefs, err)
	}
	if got, ref := recoveryReserve(t, session.DB), recoveryReserve(t, refDB); got != "42.25" || got != ref {
		t.Fatalf("restored financial result = %s; reference = %s", got, ref)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	after := fixtureContents(t, paths)
	if _, pending := after[backup.JournalFileName]; pending || after[journal.SafetyName] != safetyBytes ||
		after[recoveryBackup] != before[recoveryBackup] || after["nestworth.db"] == before["nestworth.db"] {
		t.Fatal("shared finalization did not clear the journal and preserve its backup and safety evidence")
	}
	if err := Verify(ctx, paths); err != nil {
		t.Fatalf("clean verify after recovery: %v", err)
	}
	restarted, err := OpenSession(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	if amount := recoveryReserve(t, restarted.DB); amount != "42.25" {
		t.Fatalf("restarted amount = %s", amount)
	}
}

func recoveryReserve(t *testing.T, db *sqlite.DB) string {
	t.Helper()
	svc := application.NewService(sqlite.NewRepository(db))
	records, err := svc.ListAccounts(t.Context(), domain.AccountFilter{})
	i := slices.IndexFunc(records, func(row domain.AccountRecord) bool { return row.Account.Name == "Recovery Sample Reserve" })
	if err != nil || i < 0 || records[i].LatestValue == nil {
		t.Fatalf("read recovery reserve: %v", err)
	}
	return records[i].LatestValue.Amount.CanonicalAmount()
}

func TestUnsafeDisposableRecoveryMakesNoFileChanges(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*testing.T, Paths, string)
	}{
		{"external journal path even when sealed", func(t *testing.T, p Paths, _ string) {
			resealJournal(t, p, func(plan *backup.Journal) { plan.StagingName = "../outside.db" })
		}},
		{"unsupported phase even when sealed", func(t *testing.T, p Paths, _ string) {
			resealJournal(t, p, func(plan *backup.Journal) { plan.State = backup.StateOriginalRenamed })
		}},
		{"symlink safety copy", func(t *testing.T, p Paths, outside string) {
			journal, err := backup.ReadJournal(p.database)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(p.data, journal.SafetyName)
			if err := errors.Join(os.Remove(path), os.Symlink(outside, path)); err != nil {
				t.Fatal(err)
			}
		}},
		{"hardlinked journal", func(t *testing.T, p Paths, outside string) {
			journal := backup.JournalPath(p.database)
			if err := errors.Join(os.Remove(journal), os.Link(outside, journal)); err != nil {
				t.Fatal(err)
			}
		}},
		{"staging symlink", func(t *testing.T, p Paths, outside string) {
			path := filepath.Join(p.data, backup.StagingName(p.database))
			if err := os.Symlink(outside, path); err != nil {
				t.Fatal(err)
			}
		}},
		{"unexpected SQLite sidecar", func(t *testing.T, p Paths, _ string) {
			if err := os.WriteFile(p.database+"-wal", []byte("synthetic rejected WAL"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"unsealed replacement", func(t *testing.T, p Paths, _ string) {
			if err := os.WriteFile(p.database, []byte("unrecognized synthetic candidate"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"different synthetic database even when checksummed", func(t *testing.T, p Paths, _ string) {
			_, other := testPaths(t)
			if err := Prepare(t.Context(), other); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(other.database)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p.database, data, 0o600); err != nil {
				t.Fatal(err)
			}
			resealJournal(t, p, func(plan *backup.Journal) { plan.BackupSHA256 = recoveryHash(data) })
		}},
		{"changed preferences", func(t *testing.T, p Paths, _ string) {
			prefs := settings.Default()
			prefs.Appearance = settings.AppearanceDark
			if err := settings.NewStore(p.settings).Save(prefs); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paths := makePendingRecovery(t)
			outside := filepath.Join(paths.RootPath(), "outside.db")
			if err := os.WriteFile(outside, []byte("outside synthetic sentinel"), 0o600); err != nil {
				t.Fatal(err)
			}
			tc.change(t, paths, outside)
			before := recoveryFileInventory(t, paths)
			open, err := OpenSession(t.Context(), paths)
			if open != nil {
				_ = open.Close()
			}
			if !errors.Is(err, ErrRecoveryPending) || !maps.Equal(before, recoveryFileInventory(t, paths)) {
				t.Fatalf("unsafe restore was accepted or touched fixture files: %v", err)
			}
			if data, err := os.ReadFile(outside); err != nil || string(data) != "outside synthetic sentinel" {
				t.Fatalf("unsafe restore changed the outside file: %v", err)
			}
			unlock, err := backup.AcquireInstanceLock(paths.database)
			if err != nil {
				t.Fatalf("rejected restore leaked its instance lock: %v", err)
			}
			if err := unlock(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func resealJournal(t *testing.T, p Paths, change func(*backup.Journal)) {
	t.Helper()
	journal, err := backup.ReadJournal(p.database)
	if err != nil {
		t.Fatal(err)
	}
	change(&journal)
	if err := backup.WriteJournal(p.database, journal); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(p.data, recoveryReceipt)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var seal recoverySeal
	if err := jsoncodec.Unmarshal(raw, &seal); err != nil {
		t.Fatal(err)
	}
	gotJournal, err := os.ReadFile(backup.JournalPath(p.database))
	if err != nil {
		t.Fatal(err)
	}
	seal.Journal = recoveryHash(gotJournal)
	raw, err = jsoncodec.Marshal(seal)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryReceiptIsBoundToOriginalRun(t *testing.T) {
	source := makePendingRecovery(t)
	root, clone := testPaths(t)
	t.Setenv("NESTWORTH_GIO_RUN_DIR", root)
	if err := os.Mkdir(clone.data, 0o700); err != nil {
		t.Fatal(err)
	}
	before := fixtureContents(t, source)
	for name, contents := range before {
		if err := os.WriteFile(filepath.Join(clone.data, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if session, err := OpenSession(t.Context(), clone); !errors.Is(err, ErrRecoveryPending) || !strings.Contains(err.Error(), "paired") {
		if session != nil {
			_ = session.Close()
		}
		t.Fatalf("a copied receipt was allowed to recover at a different root: %v", err)
	}
	if !maps.Equal(before, fixtureContents(t, clone)) {
		t.Fatal("rejecting a copied receipt changed its files")
	}
}

func recoveryFileInventory(t *testing.T, paths Paths) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(paths.data)
	if err != nil {
		t.Fatal(err)
	}
	values := make(map[string]string, len(entries))
	for _, entry := range entries {
		path := filepath.Join(paths.data, entry.Name())
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				t.Fatal(err)
			}
			values[entry.Name()] = "symlink:" + target
			continue
		}
		value, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		values[entry.Name()] = string(value)
	}
	return values
}

func TestPendingRecoveryCompetesThroughCanonicalLock(t *testing.T) {
	paths := makePendingRecovery(t)
	before := fixtureContents(t, paths)
	alias := filepath.Join(t.TempDir(), "fixture-alias")
	if err := os.Symlink(paths.RootPath(), alias); err != nil {
		t.Fatal(err)
	}
	if _, err := resolvePaths(alias, "", ""); err == nil {
		t.Fatal("Gio accepted the alias as a profile root")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRecoveryLockHolder$")
	child.Env = append(os.Environ(), "GIO_TEST_RECOVERY_LOCK="+filepath.Join(alias, "data", "nestworth.db"))
	in, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	child.Stderr = os.Stderr
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = in.Close(); _ = child.Wait() }()
	reader := bufio.NewReader(out)
	if line, err := reader.ReadString('\n'); err != nil || strings.TrimSpace(line) != "owns recovery lock" {
		t.Fatalf("alias process did not acquire the shared lock: %q, %v", line, err)
	}
	if session, err := OpenSession(t.Context(), paths); !errors.Is(err, backup.ErrAlreadyRunning) {
		if session != nil {
			_ = session.Close()
		}
		t.Fatalf("Gio crossed the alias process's ownership boundary: %v", err)
	}
	if !maps.Equal(before, fixtureContents(t, paths)) {
		t.Fatal("the blocked session modified pending recovery files")
	}
	if err := in.Close(); err != nil {
		t.Fatal(err)
	}
	// Drain the subprocess output before Wait closes its pipe.
	if _, err := io.Copy(io.Discard, reader); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err != nil {
		t.Fatal(err)
	}
	session, err := OpenSession(t.Context(), paths)
	if err != nil {
		t.Fatalf("restart after the alias owner released the lock: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryLockHolder(t *testing.T) {
	path := os.Getenv("GIO_TEST_RECOVERY_LOCK")
	if path == "" {
		t.Skip("subprocess helper")
	}
	unlock, err := backup.AcquireInstanceLock(path)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("owns recovery lock")
	if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
		t.Fatal(err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
}
