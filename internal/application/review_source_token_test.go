package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestReviewHistoricalSourceTokenDetectsQuoteDeletionWithoutGenerationBump(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
	repo := sqlite.NewRepository(f.db)
	cutoff := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	before, err := repo.LoadReviewHistoricalBatch(t.Context(), f.account.Account.HouseholdID, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	first, err := domain.ReviewHistoricalFingerprint(before)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SQL.Exec(`INSERT INTO investment_review_control(household_id) VALUES(?) ON CONFLICT(household_id) DO NOTHING`, f.account.Account.HouseholdID.String()); err != nil {
		t.Fatal(err)
	}
	var generation int
	if err := f.db.SQL.QueryRow(`SELECT input_generation FROM investment_review_control WHERE household_id=?`, f.account.Account.HouseholdID.String()).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SQL.Exec(`DELETE FROM instrument_quotes WHERE instrument_id=?`, f.instrument.ID.String()); err != nil {
		t.Fatal(err)
	}
	after, err := repo.LoadReviewHistoricalBatch(t.Context(), f.account.Account.HouseholdID, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	second, err := domain.ReviewHistoricalFingerprint(after)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("financial quote deletion remained invisible to source token")
	}
	var currentGeneration int
	if err := f.db.SQL.QueryRow(`SELECT input_generation FROM investment_review_control WHERE household_id=?`, f.account.Account.HouseholdID.String()).Scan(&currentGeneration); err != nil || currentGeneration != generation {
		t.Fatalf("test no longer isolates a source change without review generation bump: %d %d %v", generation, currentGeneration, err)
	}
}
