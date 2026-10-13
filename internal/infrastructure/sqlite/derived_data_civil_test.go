package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestDerivedSnapshotCoverageAllowsOnlyNonexistentCivilGaps(t *testing.T) {
	db, repo, household, _, _ := seedPortfolioRepository(t)
	ctx := context.Background()
	location, err := time.LoadLocation("Pacific/Apia")
	if err != nil {
		t.Fatal(err)
	}
	started := time.Date(2011, 12, 29, 12, 0, 0, 0, location)
	origin, err := domain.NewHistoryOrigin(household.ID, location.String(), started, started)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.StartHistory(ctx, domain.HistoryOriginData{Origin: origin}); err != nil {
		t.Fatal(err)
	}
	tx, err := db.SQL.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, tc := range []struct {
		name  string
		dates []string
		valid bool
	}{
		{"skipped day", []string{"2011-12-29", "2011-12-31"}, true},
		{"existing day omitted", []string{"2011-12-31"}, false},
		{"invented skipped day", []string{"2011-12-29", "2011-12-30", "2011-12-31"}, false},
		{"duplicate", []string{"2011-12-29", "2011-12-31", "2011-12-31"}, false},
		{"reordered", []string{"2011-12-31", "2011-12-29"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rebuild := domain.DerivedDataRebuild{HouseholdID: household.ID, AsOf: time.Date(2012, 1, 1, 12, 0, 0, 0, location), ClosedThrough: "2011-12-31"}
			for _, date := range tc.dates {
				rebuild.Snapshots = append(rebuild.Snapshots, domain.DailyValuationSnapshot{HouseholdID: household.ID, LocalDate: date})
			}
			if err := validateRebuildDatesTx(ctx, tx, rebuild); (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}
