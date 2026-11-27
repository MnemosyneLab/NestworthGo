package sqlite

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReviewSourceCheckpointMissingPolicyKeepsFullOracleAvailable(t *testing.T) {
	db := financialConflictDB(t)
	if _, err := db.SQL.Exec(`UPDATE history_origins SET id='00000000-0000-4000-8000-000000000004' WHERE household_id='00000000-0000-4000-8000-000000000001'`); err != nil {
		t.Fatal(err)
	}
	capture, err := NewRepository(db).CaptureReviewProjection(t.Context(), domain.HouseholdID("00000000-0000-4000-8000-000000000001"), time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if capture.SourceRevision != nil || capture.HistoricalFingerprint == "" {
		t.Fatal("missing resolver policy treated as a valid revision certificate or blocked full capture")
	}
}
