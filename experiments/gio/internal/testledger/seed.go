// Package testledger seeds ordinary Wails-format SQLite data for Gio tests.
package testledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

// Seed writes the ordinary synthetic household for directory and activity tests.
func Seed(ctx context.Context, database, preferences string) (result error) {
	db, err := sqlite.Open(database)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, db.Close()) }()
	service := application.NewService(sqlite.NewRepository(db))
	service.SetClock(HistoryStart)
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Harbor Sample Household",
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
	if err := settings.NewStore(preferences).Save(settings.Default()); err != nil {
		return err
	}
	return db.CheckpointWAL(ctx)
}
