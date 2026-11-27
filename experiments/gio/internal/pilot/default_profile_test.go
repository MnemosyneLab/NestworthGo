package pilot

import (
	"bytes"
	"database/sql"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/testfixture"
)

func pathsForDefaultProfile(t *testing.T) (string, Paths) {
	t.Helper()
	base, _ := testPaths(t)
	t.Setenv("HOME", base)
	t.Setenv("XDG_CONFIG_HOME", base)
	t.Setenv("NESTWORTH_DATABASE_PATH", "")
	t.Setenv("NESTWORTH_SETTINGS_PATH", "")
	paths, err := PathsFromEnv()
	if err != nil {
		t.Fatalf("resolve the default Gio profile: %v", err)
	}
	if err := os.MkdirAll(paths.data, 0o700); err != nil {
		t.Fatal(err)
	}
	return base, paths
}

func TestDefaultProfileOpensCopiedWailsLedger(t *testing.T) {
	base, paths := pathsForDefaultProfile(t)
	ctx := t.Context()
	source := dataPaths(filepath.Join(base, "wails-source"))
	db, err := sqlite.Open(source.database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	app := testfixture.NewService(&testfixture.BaselineRepository{Repository: testfixture.Repository(db)})
	now := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	testfixture.SetClock(app, func() time.Time { return now })
	if err := app.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Pine Street Household", BaseCurrency: "AUD", MemberNames: []string{"Alex"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := testfixture.CreateAccount(app, ctx, application.AccountInput{Name: "Pine Street checking", AccountType: "bank_account", BalanceSheetRole: "asset",
		TrackingMode: "balance", DefaultCurrency: "AUD", InitialAmount: "123.45", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testfixture.CaptureSyntheticOrigin(app, ctx, "Australia/Sydney"); err != nil {
		t.Fatal(err)
	}
	store := settings.NewStore(source.settings)
	if err := store.Attach(sqlite.NewConfigurationRepository(db)); err != nil {
		t.Fatal(err)
	}
	want, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	want.Appearance = settings.AppearanceDark
	want.DateFormat = settings.DateFormatDayFirst
	want.QuoteCacheTTL = settings.QuoteCacheTTL3h
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(db.CheckpointWAL(ctx), db.Close()); err != nil {
		t.Fatal(err)
	}
	original := fixtureContents(t, source)
	for name, data := range original {
		if name == filepath.Base(source.database) || name == filepath.Base(source.settings) {
			if err := os.WriteFile(filepath.Join(paths.data, name), []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	// A receipt can remain when a tester replaces a previously used fixture.
	stale := []byte("obsolete data from an earlier preview")
	receipt := filepath.Join(paths.data, "onboarding.fixture")
	if err := os.WriteFile(receipt, stale, 0o600); err != nil {
		t.Fatal(err)
	}
	open := func() (*Session, *application.Service) {
		t.Helper()
		s, err := OpenSession(ctx, paths)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = s.Close() })
		return s, testfixture.NewService(&testfixture.BaselineRepository{Repository: testfixture.Repository(s.DB)})
	}
	s, copied := open()
	if second, err := OpenSession(ctx, paths); !errors.Is(err, backup.ErrAlreadyRunning) {
		if second != nil {
			_ = second.Close()
		}
		t.Fatalf("copied profile did not use the shared instance lock: %v", err)
	}
	check := func(s *Session, copied *application.Service) {
		t.Helper()
		boot, err := copied.Bootstrap(ctx)
		if err != nil || boot.Household == nil || boot.Household.ID != bootstrap.Household.ID || boot.Household.Name != "Pine Street Household" {
			t.Fatalf("ordinary household did not survive the copy: %v", err)
		}
		accounts, err := copied.ListAccounts(ctx, domain.AccountFilter{})
		if err != nil || len(accounts) != 1 || accounts[0].Account.ID != account.Account.ID {
			t.Fatalf("ordinary account did not survive the copy: %v", err)
		}
		origin, err := copied.HistoryOrigin(ctx)
		if err != nil || origin == nil || origin.Timezone != "Australia/Sydney" {
			t.Fatalf("history origin did not survive the copy: %v", err)
		}
		store := settings.NewStore(paths.settings)
		if err := store.Attach(sqlite.NewConfigurationRepository(s.DB)); err != nil {
			t.Fatal(err)
		}
		if got, err := store.Load(); err != nil || got != want {
			t.Fatalf("presentation and financial settings changed: %v", err)
		}
	}
	check(s, copied)
	institution, err := copied.CreateInstitution(ctx, "Local Gio edit", domain.InstitutionBank)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, copied = open()
	check(s, copied)
	institutions, err := copied.ListInstitutions(ctx, false)
	if err != nil || !slices.ContainsFunc(institutions, func(value domain.Institution) bool {
		return value.ID == institution.ID && value.Name == institution.Name
	}) {
		t.Fatalf("the local write did not survive a restart: %v", err)
	}
	if !maps.Equal(original, fixtureContents(t, source)) {
		t.Fatal("the original Wails files changed")
	}
	if contents, err := os.ReadFile(receipt); err != nil || !bytes.Equal(contents, stale) {
		t.Fatalf("normal startup modified stale fixture metadata: %v", err)
	}
}

func TestDefaultProfileRefusesInvalidDatabaseWithoutReseeding(t *testing.T) {
	for _, scenario := range []string{"empty", "corrupt", "unsupported schema"} {
		t.Run(scenario, func(t *testing.T) {
			_, paths := pathsForDefaultProfile(t)
			switch scenario {
			case "empty":
				if err := os.WriteFile(paths.database, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				if err := os.WriteFile(paths.database, []byte("broken database"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "unsupported schema":
				other, err := sql.Open("sqlite", paths.database)
				if err != nil {
					t.Fatal(err)
				}
				_, err = other.Exec("CREATE TABLE unrelated(value TEXT)")
				if err := errors.Join(err, other.Close()); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(paths.database)
			if err != nil {
				t.Fatal(err)
			}
			if s, err := OpenSession(t.Context(), paths); err == nil {
				_ = s.Close()
				t.Fatal("startup accepted an invalid database")
			}
			after, err := os.ReadFile(paths.database)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("the rejected database was seeded or modified: %v", err)
			}
			unlock, err := backup.AcquireInstanceLock(paths.database)
			if err != nil {
				t.Fatalf("failed startup left its instance lock held: %v", err)
			}
			if err := unlock(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
