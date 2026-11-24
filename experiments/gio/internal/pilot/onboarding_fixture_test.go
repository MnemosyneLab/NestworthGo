package pilot

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestOnboardingFixtureIsExplicitAndFencesTheSession(t *testing.T) {
	root, paths := testPaths(t)
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("NESTWORTH_GIO_RUN_DIR", "")
	t.Setenv("NESTWORTH_DATABASE_PATH", "")
	t.Setenv("NESTWORTH_SETTINGS_PATH", "")
	if err := PrepareOnboarding(t.Context(), paths); err == nil {
		t.Fatal("prepared without an explicit run")
	}
	defaultPath, err := PathsFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("NESTWORTH_GIO_RUN_DIR", defaultPath.RootPath())
	if err := PrepareOnboarding(t.Context(), defaultPath); err == nil {
		t.Fatal("prepared onboarding in the default location")
	}
	if _, err := os.Lstat(defaultPath.RootPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("onboarding preparation changed the default location: %v", err)
	}
	t.Setenv("NESTWORTH_GIO_RUN_DIR", root)
	if err := PrepareOnboarding(t.Context(), paths); err != nil {
		t.Fatal(err)
	}
	if err := PrepareOnboarding(t.Context(), paths); !errors.Is(err, os.ErrExist) {
		t.Fatalf("prepared over an existing fixture: %v", err)
	}
	if err := Ensure(t.Context(), paths); err != nil {
		t.Fatal("default launcher did not recognize the explicit fixture:", err)
	}
	session, err := OpenSession(t.Context(), paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	before := fixtureContents(t, paths)
	if second, err := OpenSession(t.Context(), paths); !errors.Is(err, backup.ErrAlreadyRunning) {
		if second != nil {
			_ = second.Close()
		}
		t.Fatalf("owner did not fence the empty fixture: %v", err)
	}
	if !maps.Equal(before, fixtureContents(t, paths)) {
		t.Fatal("blocked launch changed the data directory")
	}
	service := application.NewService(sqlite.NewRepository(session.DB))
	core, err := service.Bootstrap(t.Context())
	if err != nil || core.Household != nil {
		t.Fatalf("fresh fixture was not empty: %+v, %v", core, err)
	}
	if err := service.CompleteOnboarding(t.Context(), application.OnboardingInput{
		HouseholdName: "Synthetic test family", BaseCurrency: "SGD", MemberNames: []string{"Sample member"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSession(t.Context(), paths)
	if err != nil {
		t.Fatal("initialized fixture could not reopen:", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	stored, err := application.NewService(sqlite.NewRepository(reopened.DB)).Bootstrap(t.Context())
	if err != nil || stored.Household == nil || stored.Household.Name != "Synthetic test family" || len(stored.Members) != 1 {
		t.Fatalf("reopened profile did not contain the named household: %+v %v", stored, err)
	}
}

func TestOnboardingGuardRejectsUnmarkedAndCopiedLedgersWithoutWrites(t *testing.T) {
	assertRejected := func(t *testing.T, paths Paths) {
		t.Helper()
		before := fixtureContents(t, paths)
		if session, err := OpenSession(t.Context(), paths); err == nil {
			_ = session.Close()
			t.Fatal("opened a ledger without a matching onboarding proof")
		}
		if !maps.Equal(before, fixtureContents(t, paths)) {
			t.Fatal("rejected launch changed the data directory")
		}
	}
	root, genuine := testPaths(t)
	t.Setenv("NESTWORTH_GIO_RUN_DIR", root)
	if err := PrepareOnboarding(t.Context(), genuine); err != nil {
		t.Fatal(err)
	}
	t.Run("copied to another path", func(t *testing.T) {
		_, copied := testPaths(t)
		if err := os.Mkdir(copied.data, 0o700); err != nil {
			t.Fatal(err)
		}
		for file, contents := range fixtureContents(t, genuine) {
			if err := os.WriteFile(filepath.Join(copied.data, file), []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		assertRejected(t, copied)
	})
	t.Run("ordinary database replacement", func(t *testing.T) {
		for _, withHousehold := range []bool{false, true} {
			root, candidate := testPaths(t)
			t.Setenv("NESTWORTH_GIO_RUN_DIR", root)
			if err := os.Mkdir(candidate.data, 0o700); err != nil {
				t.Fatal(err)
			}
			db, err := sqlite.Open(candidate.database)
			if err != nil {
				t.Fatal(err)
			}
			if withHousehold {
				app := application.NewService(sqlite.NewRepository(db))
				err = app.CompleteOnboarding(t.Context(), application.OnboardingInput{
					HouseholdName: "Unmarked synthetic replacement", BaseCurrency: "USD", MemberNames: []string{"Sample member"},
				})
			}
			if err := errors.Join(err, db.CheckpointWAL(t.Context()), db.Close()); err != nil {
				t.Fatal(err)
			}
			if err := settings.NewStore(candidate.settings).Save(settings.Default()); err != nil {
				t.Fatal(err)
			}
			assertRejected(t, candidate)
			receipt := filepath.Join(genuine.data, onboardingReceipt)
			if err := copyFixtureFile(receipt, filepath.Join(candidate.data, onboardingReceipt)); err != nil {
				t.Fatal(err)
			}
			assertRejected(t, candidate)
			if err := PrepareOnboarding(t.Context(), candidate); !errors.Is(err, os.ErrExist) {
				t.Fatalf("prepare accepted a replacement ledger: %v", err)
			}
		}
	})
	t.Run("linked receipt", func(t *testing.T) {
		receipt := filepath.Join(genuine.data, onboardingReceipt)
		linked := filepath.Join(root, "linked-receipt")
		if err := os.Link(receipt, linked); err != nil {
			t.Fatal(err)
		}
		assertRejected(t, genuine)
	})
}
