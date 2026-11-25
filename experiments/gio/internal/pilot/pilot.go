package pilot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

const (
	householdName = "Gio Institutions QA"
	markerKey     = "gio.institutions.fixture.v1"
)

var ErrRecoveryPending = errors.New("the isolated preview profile has a pending restore journal")

// seedFixture writes only inside a fresh private staging directory.
func seedFixture(ctx context.Context, paths Paths) (result error) {
	db, err := sqlite.Open(paths.database)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, db.Close()) }()
	service := application.NewService(sqlite.NewRepository(db))
	service.SetClock(historyStart)
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: householdName,
		BaseCurrency:  "USD",
		MemberNames:   []string{"Sample member"},
	}); err != nil {
		return err
	}
	var bankID domain.InstitutionID
	for _, entry := range []struct {
		name string
		kind domain.InstitutionType
	}{
		{"Harbor Sample Bank", domain.InstitutionBank},
		{"Evergreen Sample Brokerage", domain.InstitutionBrokerage},
		{"Westlake Sample Insurer", domain.InstitutionInsurer},
	} {
		institution, err := service.CreateInstitution(ctx, entry.name, entry.kind)
		if err != nil {
			return err
		}
		if entry.kind == domain.InstitutionBank {
			bankID = institution.ID
		}
	}
	if err := seedHistory(ctx, service, bankID); err != nil {
		return fmt.Errorf("seed synthetic account history: %w", err)
	}
	if err := settings.NewStore(paths.settings).Save(settings.Default()); err != nil {
		return err
	}
	household, err := service.Household(ctx)
	if err != nil {
		return err
	}
	marker := sqlite.NewConfigurationRepository(db)
	if err := marker.SaveConfiguration(markerKey, []byte(household.ID.String())); err != nil {
		return err
	}
	return db.CheckpointWAL(ctx)
}

type fixtureFiles struct {
	database os.FileInfo
	journals []string
	lock     bool
}

func inspectFixture(paths Paths) (fixtureFiles, error) {
	var files fixtureFiles
	if err := requirePrivateDirectory(paths.RootPath()); err != nil {
		return files, err
	}
	if err := requirePrivateDirectory(paths.data); err != nil {
		return files, err
	}
	dbInfo, err := verifyFile(paths.database)
	if err != nil {
		// An interrupted restore can leave the live database temporarily absent.
		if errors.Is(err, os.ErrNotExist) {
			if err := checkRestoreJournal(paths); err != nil {
				return files, err
			}
		}
		return files, fmt.Errorf("verify isolated database file: %w", err)
	}
	files.database = dbInfo
	if _, err := verifyFile(paths.settings); err != nil {
		return files, fmt.Errorf("verify isolated settings file: %w", err)
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal", ".instance-lock"} {
		if _, err := verifyFile(paths.database + suffix); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return files, fmt.Errorf("verify isolated database sidecar: %w", err)
		}
		switch suffix {
		case ".instance-lock":
			files.lock = true
		case "-wal", "-journal":
			files.journals = append(files.journals, suffix)
		}
	}
	return files, nil
}

func checkRestoreJournal(paths Paths) error {
	_, err := os.Lstat(backup.JournalPath(paths.database))
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil
	case err == nil:
		return ErrRecoveryPending
	default:
		return fmt.Errorf("check for a restore journal: %w", err)
	}
}

func verifyFixture(ctx context.Context, paths Paths, files fixtureFiles) error {
	if err := checkRestoreJournal(paths); err != nil {
		return err
	}
	_, status, err := settings.NewStore(paths.settings).LoadWithStatus()
	if err != nil || status != settings.LoadStatusClean {
		return errors.New("the isolated preview settings file is invalid")
	}
	// SQLite can create or update WAL-index sidecars even in read-only mode.
	// Verify the main file and any journals on a private copy. SQLite rebuilds
	// its transient WAL index there, including after an interrupted session.
	dir, err := os.MkdirTemp("", "nestworth-gio-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	copyPath := filepath.Join(dir, filepath.Base(paths.database))
	for _, suffix := range append([]string{""}, files.journals...) {
		if err := copyFixtureFile(paths.database+suffix, copyPath+suffix); err != nil {
			return fmt.Errorf("copy the fixture for read-only verification: %w", err)
		}
	}
	probe, err := sqlite.OpenReadOnlyForVerifyContext(ctx, copyPath)
	if err != nil {
		return fmt.Errorf("verify the fixture read-only: %w", err)
	}
	return errors.Join(verifyIdentity(ctx, paths, probe), probe.Close())
}

func copyFixtureFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, input)
	return errors.Join(err, output.Close())
}

func verifyHousehold(ctx context.Context, db *sqlite.DB) error {
	household, err := sqlite.NewRepository(db).Household(ctx)
	if err != nil || household == nil || household.Name != householdName {
		return errors.New("the database does not contain the synthetic Institutions household")
	}
	marker, found, err := sqlite.NewConfigurationRepository(db).LoadConfiguration(markerKey)
	if err != nil || !found || string(marker) != household.ID.String() {
		return errors.New("the Institutions fixture marker does not match the household")
	}
	return nil
}

func openVerified(ctx context.Context, paths Paths, verified os.FileInfo) (*sqlite.DB, error) {
	current, err := verifyFile(paths.database)
	if err != nil || !os.SameFile(verified, current) {
		return nil, errors.New("the isolated database changed during verification")
	}
	db, err := sqlite.Open(paths.database)
	if err != nil {
		return nil, err
	}
	if err := verifyIdentity(ctx, paths, db); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return db, nil
}
