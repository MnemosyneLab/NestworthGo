package pilot

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestProfilePathsFromEnv(t *testing.T) {
	base, isolated := testPaths(t)
	t.Setenv("HOME", base)
	t.Setenv("XDG_CONFIG_HOME", base)
	t.Setenv("NESTWORTH_GIO_RUN_DIR", "")
	t.Setenv("NESTWORTH_DATABASE_PATH", "")
	t.Setenv("NESTWORTH_SETTINGS_PATH", "")

	config, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(config, "com.nestworth.gio")
	profile, err := PathsFromEnv()
	if err != nil || profile.RootPath() != want || profile != dataPaths(filepath.Join(want, "data")) {
		t.Fatalf("default Gio profile = %+v, %v", profile, err)
	}
	if _, err := os.Lstat(want); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("resolving the profile created its directory: %v", err)
	}

	t.Setenv("NESTWORTH_DATABASE_PATH", isolated.database)
	t.Setenv("NESTWORTH_SETTINGS_PATH", isolated.settings)
	if _, err := PathsFromEnv(); err == nil {
		t.Fatal("accepted isolated files without a run root")
	}
	t.Setenv("NESTWORTH_GIO_RUN_DIR", base)
	if paths, err := PathsFromEnv(); err != nil || paths != isolated {
		t.Fatalf("explicit isolated run = %+v, %v", paths, err)
	}
	t.Setenv("NESTWORTH_DATABASE_PATH", "")
	if _, err := PathsFromEnv(); err == nil {
		t.Fatal("accepted a partial override")
	}
	t.Setenv("NESTWORTH_SETTINGS_PATH", "")
	if paths, err := PathsFromEnv(); err != nil || paths != isolated {
		t.Fatalf("isolated root alone = %+v, %v", paths, err)
	}
}

func TestConcurrentFirstLaunchAndSessionLock(t *testing.T) {
	base, _ := testPaths(t)
	paths, err := resolvePaths(filepath.Join(base, "new-profile"), "", "")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	start := make(chan struct{})
	finished := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			finished <- ensureFixture(ctx, paths, true, seedFixture)
		}()
	}
	close(start)
	for range 2 {
		if err := <-finished; err != nil {
			t.Fatal(err)
		}
	}
	session, err := OpenSession(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	before := fixtureContents(t, paths)
	if err := Verify(ctx, paths); !errors.Is(err, backup.ErrAlreadyRunning) {
		t.Fatalf("verification did not honor the live profile lock: %v", err)
	}
	if second, err := OpenSession(ctx, paths); !errors.Is(err, backup.ErrAlreadyRunning) {
		if second != nil {
			_ = second.Close()
		}
		t.Fatalf("second desktop process lock = %v", err)
	}
	if !maps.Equal(before, fixtureContents(t, paths)) {
		t.Fatal("blocked launches changed the active fixture")
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSession(ctx, paths)
	if err != nil {
		t.Fatalf("previous session retained the lock: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPendingRestoreBlocksWithoutReconciliationAndReleasesLock(t *testing.T) {
	_, paths := testPaths(t)
	ctx := t.Context()
	if err := Prepare(ctx, paths); err != nil {
		t.Fatal(err)
	}
	if err := backup.WriteJournal(paths.database, backup.Journal{State: backup.StatePrepared}); err != nil {
		t.Fatal(err)
	}
	before := fixtureContents(t, paths)
	if session, err := OpenSession(ctx, paths); !errors.Is(err, ErrRecoveryPending) {
		if session != nil {
			_ = session.Close()
		}
		t.Fatalf("pending restore did not block startup: %v", err)
	}
	if !maps.Equal(before, fixtureContents(t, paths)) {
		t.Fatal("startup modified a fixture with a pending restore")
	}
	safetyPath := paths.database + ".pre-restore"
	if err := os.Rename(paths.database, safetyPath); err != nil {
		t.Fatal(err)
	}
	before = fixtureContents(t, paths)
	if err := Verify(ctx, paths); !errors.Is(err, ErrRecoveryPending) {
		t.Fatalf("an interrupted rename was not reported as a pending restore: %v", err)
	}
	if !maps.Equal(before, fixtureContents(t, paths)) {
		t.Fatal("verification changed a fixture with an interrupted rename")
	}
	if err := os.Rename(safetyPath, paths.database); err != nil {
		t.Fatal(err)
	}
	if err := backup.RemoveJournal(paths.database); err != nil {
		t.Fatal(err)
	}
	session, err := OpenSession(ctx, paths)
	if err != nil {
		t.Fatalf("blocked startup retained ownership: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRejectsUnmarkedLegacyProfileWithoutCreatingSQLiteOrLockSidecars(t *testing.T) {
	_, paths := testPaths(t)
	ctx := t.Context()
	if err := Prepare(ctx, paths); err != nil {
		t.Fatal(err)
	}
	session, err := OpenSession(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	err = sqlite.NewConfigurationRepository(session.DB).SaveConfiguration(markerKey, []byte("unrecognized fixture"))
	if err := errors.Join(err, session.Close()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(paths.database + ".instance-lock"); err != nil {
		t.Fatal(err)
	}
	before := fixtureContents(t, paths)
	if session, err := OpenSession(ctx, paths); err == nil {
		_ = session.Close()
		t.Fatal("opened an unmarked legacy profile")
	}
	if !maps.Equal(before, fixtureContents(t, paths)) {
		t.Fatal("refusing an unmarked profile changed its files")
	}
}

func TestVerificationReadsTheMarkerFromACommittedWAL(t *testing.T) {
	_, source := testPaths(t)
	_, candidate := testPaths(t)
	ctx := t.Context()
	session, err := OpenSession(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if err := session.DB.CheckpointWAL(ctx); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.NewConfigurationRepository(session.DB).SaveConfiguration(markerKey, []byte("invalid marker in WAL")); err != nil {
		t.Fatal(err)
	}
	// Model a stopped writer whose committed change remains in the WAL.
	files := fixtureContents(t, source)
	if files["nestworth.db-wal"] == "" {
		t.Fatal("the fixture mutation did not reach the WAL")
	}
	if err := os.Mkdir(candidate.data, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"nestworth.db", "nestworth.db-wal", "settings.json"} {
		if err := os.WriteFile(filepath.Join(candidate.data, name), []byte(files[name]), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before := fixtureContents(t, candidate)
	if err := Verify(ctx, candidate); err == nil || !strings.Contains(err.Error(), "fixture marker does not match") {
		t.Fatalf("verification ignored the committed WAL marker: %v", err)
	}
	if !maps.Equal(before, fixtureContents(t, candidate)) {
		t.Fatal("verification changed the fixture or its WAL")
	}
}

func fixtureContents(t *testing.T, paths Paths) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(paths.data)
	if err != nil {
		t.Fatal(err)
	}
	contents := make(map[string]string, len(entries))
	for _, entry := range entries {
		raw, err := os.ReadFile(filepath.Join(paths.data, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		contents[entry.Name()] = string(raw)
	}
	return contents
}

func TestEnsurePreservesUnknownOrIncompleteData(t *testing.T) {
	_, paths := testPaths(t)
	if err := os.Mkdir(paths.data, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := Ensure(ctx, paths); err == nil {
		t.Fatal("seeded an existing data directory with missing files")
	}
	if _, err := os.Lstat(paths.database); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected database after failed verification: %v", err)
	}
	const original = "unrecognized local database"
	if err := os.WriteFile(paths.database, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := settings.NewStore(paths.settings).Save(settings.Default()); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(ctx, paths); err == nil {
		t.Fatal("reused a corrupt database")
	}
	if content, err := os.ReadFile(paths.database); err != nil || string(content) != original {
		t.Fatalf("overwrote the existing database: %q, %v", content, err)
	}
}

func TestFailedSeedCanRetryWithoutRepairingExistingData(t *testing.T) {
	_, paths := testPaths(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Ensure(ctx, paths); err == nil {
		t.Fatal("published a canceled seed")
	}
	if _, err := os.Lstat(paths.data); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed seed left a partial data directory: %v", err)
	}
	if err := Ensure(context.Background(), paths); err != nil {
		t.Fatalf("retry after failed initial seed: %v", err)
	}
}
