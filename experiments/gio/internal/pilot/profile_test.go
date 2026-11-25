package pilot

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestProfilePathsFromEnv(t *testing.T) {
	base := t.TempDir()
	t.Setenv("HOME", base)
	t.Setenv("XDG_CONFIG_HOME", base)
	t.Setenv("NESTWORTH_DATABASE_PATH", "")
	t.Setenv("NESTWORTH_SETTINGS_PATH", "")
	config, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	expected := dataPaths(filepath.Join(config, AppID, "data"))
	p, err := PathsFromEnv()
	if err != nil || p != expected {
		t.Fatalf("default Gio profile = %+v, %v", p, err)
	}
	if _, err := os.Lstat(expected.data); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("resolving the profile created its directory: %v", err)
	}
	customDB := filepath.Join(base, "independent", "test.db")
	customSettings := filepath.Join(base, "prefs", "test.json")
	t.Setenv("NESTWORTH_DATABASE_PATH", customDB)
	p, err = PathsFromEnv()
	if err != nil || p.database != customDB || p.settings != expected.settings {
		t.Fatalf("independent database override = %+v, %v", p, err)
	}
	t.Setenv("NESTWORTH_DATABASE_PATH", "")
	t.Setenv("NESTWORTH_SETTINGS_PATH", customSettings)
	p, err = PathsFromEnv()
	if err != nil || p.database != expected.database || p.settings != customSettings {
		t.Fatalf("independent settings override = %+v, %v", p, err)
	}
}

func TestFirstLaunchStartsOnboardingAndUsesCanonicalInstanceLock(t *testing.T) {
	base, paths := pathsForDefaultProfile(t)
	// A normal application directory does not need the old pilot permissions.
	if err := os.Chmod(paths.data, 0o755); err != nil {
		t.Fatal(err)
	}
	session, err := OpenSession(t.Context(), paths)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	core, err := application.NewService(sqlite.NewRepository(session.DB)).Bootstrap(t.Context())
	if err != nil || core.Household != nil || len(core.Members) != 0 {
		t.Fatalf("a fresh profile should require onboarding: %+v, %v", core, err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(paths.data, alias); err != nil {
		t.Fatal(err)
	}
	throughAlias := dataPaths(alias)
	if other, err := OpenSession(t.Context(), throughAlias); !errors.Is(err, backup.ErrAlreadyRunning) {
		if other != nil {
			_ = other.Close()
		}
		t.Fatalf("the same ledger through a symlink escaped the Wails lock: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSession(t.Context(), throughAlias)
	if err != nil {
		t.Fatalf("released profile could not be opened through its alias: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
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
