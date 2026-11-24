package pilot

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

const (
	householdName = "Gio Institutions QA"
	markerKey     = "gio.institutions.fixture.v1"
)

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

func verifyFixture(ctx context.Context, paths Paths) (os.FileInfo, error) {
	if err := requirePrivateDirectory(paths.RootPath()); err != nil {
		return nil, err
	}
	if err := requirePrivateDirectory(paths.data); err != nil {
		return nil, err
	}
	dbInfo, err := verifyFile(paths.database)
	if err != nil {
		return nil, fmt.Errorf("verify isolated database file: %w", err)
	}
	if _, err := verifyFile(paths.settings); err != nil {
		return nil, fmt.Errorf("verify isolated settings file: %w", err)
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal", ".instance-lock"} {
		if _, err := verifyFile(paths.database + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
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
	checkErr := verifyHousehold(ctx, probe)
	if err := errors.Join(checkErr, probe.Close()); err != nil {
		return nil, err
	}
	return dbInfo, nil
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

// Verify checks the fixture read-only before launching the desktop.
func Verify(ctx context.Context, paths Paths) error {
	_, err := verifyFixture(ctx, paths)
	return err
}

// Open verifies the fixture and returns its writable database handle.
func Open(ctx context.Context, paths Paths) (*sqlite.DB, error) {
	verified, err := verifyFixture(ctx, paths)
	if err != nil {
		return nil, err
	}
	return openVerified(ctx, paths, verified)
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
	if err := verifyHousehold(ctx, db); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return db, nil
}
