package sqlite

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReviewSourceShadowLegacyReadonlyKeepsFullCaptureWithoutCheckpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-v19.db")
	old := syntheticV19(t, path)
	if _, err := old.Exec(`INSERT INTO history_origins(id,household_id,timezone,started_at,created_at) VALUES('00000000-0000-4000-8000-000000000002','00000000-0000-4000-8000-000000000001','UTC','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`INSERT INTO history_snapshot_state(household_id,updated_at,resolver_policy_version) VALUES('00000000-0000-4000-8000-000000000001','2026-10-01T00:00:00Z',?)`, domain.MarketDataResolverPolicy); err != nil {
		t.Fatal(err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	ro, err := OpenReadOnlyForVerify(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	before := int64(0)
	if err = ro.SQL.QueryRow(`SELECT total_changes()`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	cutoff := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	repo := NewRepository(ro)
	capture, err := repo.CaptureReviewProjection(t.Context(), domain.HouseholdID("00000000-0000-4000-8000-000000000001"), cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if capture.SourceRevision != nil || capture.HistoricalFingerprint == "" {
		t.Fatal("legacy read claimed source checkpoint or lost full fingerprint")
	}
	var after int64
	var version int
	if err = ro.SQL.QueryRow(`SELECT total_changes()`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if err = ro.SQL.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if after != before || version != 19 {
		t.Fatal("legacy capture wrote or migrated readonly database")
	}
	if err = repo.PublishReviewProjection(t.Context(), domain.ReviewPublication{Capture: capture, At: cutoff}, cutoff); err == nil {
		t.Fatal("readonly legacy capture published")
	}
}

func TestReviewSourceShadowMissingPolicyKeepsFullOracleAvailable(t *testing.T) {
	db := financialConflictDB(t)
	if _, err := db.SQL.Exec(`INSERT INTO history_origins(id,household_id,timezone,started_at,created_at) VALUES('00000000-0000-4000-8000-000000000004','00000000-0000-4000-8000-000000000001','UTC','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	capture, err := NewRepository(db).WithFullReviewRecompute().CaptureReviewProjection(t.Context(), domain.HouseholdID("00000000-0000-4000-8000-000000000001"), time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if capture.SourceRevision != nil || capture.HistoricalFingerprint == "" {
		t.Fatal("missing resolver policy treated as a valid revision certificate or blocked full capture")
	}
}
