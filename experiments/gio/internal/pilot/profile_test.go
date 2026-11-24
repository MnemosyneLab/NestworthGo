package pilot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
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
			finished <- Ensure(ctx, paths)
		}()
	}
	close(start)
	for range 2 {
		if err := <-finished; err != nil {
			t.Fatal(err)
		}
	}
	db, unlock, err := OpenSession(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	if err := Ensure(ctx, paths); err != nil {
		t.Fatalf("launcher could not verify the live profile: %v", err)
	}
	if second, release, err := OpenSession(ctx, paths); !errors.Is(err, backup.ErrAlreadyRunning) {
		if second != nil {
			second.Close()
			release()
		}
		t.Fatalf("second desktop process lock = %v", err)
	}
	if err := errors.Join(db.Close(), unlock()); err != nil {
		t.Fatal(err)
	}
	reopened, release, err := OpenSession(ctx, paths)
	if err != nil {
		t.Fatalf("previous session retained the lock: %v", err)
	}
	if err := errors.Join(reopened.Close(), release()); err != nil {
		t.Fatal(err)
	}
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
