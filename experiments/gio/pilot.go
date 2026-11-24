package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

const (
	pilotHousehold = "Gio Institutions QA"
	pilotMarkerKey = "gio.institutions.fixture.v1"
)

type pilotPaths struct {
	data, database, settings string
}

func pilotPathsFromEnv() (pilotPaths, error) {
	return resolvePilotPaths(os.Getenv("NESTWORTH_GIO_RUN_DIR"),
		os.Getenv("NESTWORTH_DATABASE_PATH"), os.Getenv("NESTWORTH_SETTINGS_PATH"))
}

func resolvePilotPaths(runDir, databasePath, settingsPath string) (pilotPaths, error) {
	if !filepath.IsAbs(runDir) || filepath.Clean(runDir) != runDir {
		return pilotPaths{}, errors.New("NESTWORTH_GIO_RUN_DIR must be the absolute path of the isolated preview run")
	}
	data := filepath.Join(runDir, "data")
	expected := pilotPaths{data: data, database: filepath.Join(data, "nestworth.db"), settings: filepath.Join(data, "settings.json")}
	if databasePath != expected.database || settingsPath != expected.settings {
		return pilotPaths{}, errors.New("explicit database and settings paths must match the isolated preview run")
	}
	canonical, err := filepath.EvalSymlinks(runDir)
	if err != nil || canonical != runDir {
		return pilotPaths{}, errors.New("the preview run directory must exist at its canonical path")
	}
	if err := requirePrivateDirectory(runDir); err != nil {
		return pilotPaths{}, err
	}
	return expected, nil
}

func requirePrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || !isPrivateOwned(info) {
		return errors.New("the preview requires a private run and data directory owned by the current user")
	}
	return nil
}

func isPrivateOwned(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid() && info.Mode().Perm()&0o077 == 0
}

func pilotFile(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || !isPrivateOwned(info) || info.Sys().(*syscall.Stat_t).Nlink != 1 {
		return nil, errors.New("expected a private regular file without symbolic or hard links")
	}
	return info, nil
}

func preparePilotFixture(ctx context.Context, paths pilotPaths) (result error) {
	// A fresh data directory keeps the seeder off any existing ledger.
	if err := os.Mkdir(paths.data, 0o700); err != nil {
		return fmt.Errorf("create fresh fixture directory: %w", err)
	}
	db, err := sqlite.Open(paths.database)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, db.Close()) }()
	service := application.NewService(sqlite.NewRepository(db))
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: pilotHousehold,
		BaseCurrency:  "USD",
		MemberNames:   []string{"Sample member"},
	}); err != nil {
		return err
	}
	for _, entry := range []struct {
		name string
		kind domain.InstitutionType
	}{
		{"Harbor Sample Bank", domain.InstitutionBank},
		{"Evergreen Sample Brokerage", domain.InstitutionBrokerage},
		{"Westlake Sample Insurer", domain.InstitutionInsurer},
	} {
		if _, err := service.CreateInstitution(ctx, entry.name, entry.kind); err != nil {
			return err
		}
	}
	if err := settings.NewStore(paths.settings).Save(settings.Default()); err != nil {
		return err
	}
	household, err := service.Household(ctx)
	if err != nil {
		return err
	}
	marker := sqlite.NewConfigurationRepository(db)
	if err := marker.SaveConfiguration(pilotMarkerKey, []byte(household.ID.String())); err != nil {
		return err
	}
	return db.CheckpointWAL(ctx)
}

func verifyPilotFixture(ctx context.Context, paths pilotPaths) (os.FileInfo, error) {
	if err := requirePrivateDirectory(paths.data); err != nil {
		return nil, err
	}
	dbInfo, err := pilotFile(paths.database)
	if err != nil {
		return nil, fmt.Errorf("verify isolated database file: %w", err)
	}
	if _, err := pilotFile(paths.settings); err != nil {
		return nil, fmt.Errorf("verify isolated settings file: %w", err)
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := pilotFile(paths.database + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("verify isolated database sidecar: %w", err)
		}
	}
	_, status, err := settings.NewStore(paths.settings).LoadWithStatus()
	if err != nil || status != settings.LoadStatusClean {
		return nil, errors.New("the isolated preview settings file is invalid")
	}
	probe, err := sqlite.OpenReadOnlyForVerifyContext(ctx, paths.database)
	if err != nil {
		return nil, fmt.Errorf("verify the fixture read-only: %w", err)
	}
	checkErr := verifyPilotHousehold(ctx, probe)
	if err := errors.Join(checkErr, probe.Close()); err != nil {
		return nil, err
	}
	return dbInfo, nil
}

func verifyPilotHousehold(ctx context.Context, db *sqlite.DB) error {
	household, err := sqlite.NewRepository(db).Household(ctx)
	if err != nil || household == nil || household.Name != pilotHousehold {
		return errors.New("the database does not contain the synthetic Institutions household")
	}
	marker, found, err := sqlite.NewConfigurationRepository(db).LoadConfiguration(pilotMarkerKey)
	if err != nil || !found || string(marker) != household.ID.String() {
		return errors.New("the Institutions fixture marker does not match the household")
	}
	return nil
}

func openPilot(ctx context.Context, paths pilotPaths) (*sqlite.DB, error) {
	verified, err := verifyPilotFixture(ctx, paths)
	if err != nil {
		return nil, err
	}
	current, err := pilotFile(paths.database)
	if err != nil || !os.SameFile(verified, current) {
		return nil, errors.New("the isolated database changed during verification")
	}
	db, err := sqlite.Open(paths.database)
	if err != nil {
		return nil, err
	}
	if err := verifyPilotHousehold(ctx, db); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return db, nil
}
