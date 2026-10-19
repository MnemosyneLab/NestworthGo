package sqlite

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReviewCertifiedUnknownConnectionKeepsFullPublication(t *testing.T) {
	db := financialConflictDB(t)
	id := domain.HouseholdID("00000000-0000-4000-8000-000000000001")
	if _, err := db.SQL.Exec(`INSERT INTO history_origins(id,household_id,timezone,started_at,created_at) VALUES('00000000-0000-4000-8000-000000000004',?,'UTC','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`, id.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec(`INSERT INTO history_snapshot_state(household_id,updated_at,resolver_policy_version) VALUES(?,'2026-10-01T00:00:00Z',?)`, id.String(), domain.MarketDataResolverPolicy); err != nil {
		t.Fatal(err)
	}
	// A saturated physical-connection namespace represents unavailable identity;
	// it must select full computation, without making refresh impossible.
	db.reviewCertificateKey.connectionID = ^uint64(0)
	r := NewRepository(db).WithCertifiedReviewCandidate()
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	c, err := r.CaptureReviewProjection(t.Context(), id, now)
	if err != nil {
		t.Fatal(err)
	}
	if c.Certified == nil || c.Certified.Connection.Connection != 0 || c.Certified.MayCertify || c.Certified.MayReuse {
		t.Fatal("unknown connection acquired authority")
	}
	c.Certified.Provider = domain.ReviewProviderSelection{Key: "synthetic-pinned", Pinned: true}
	p := domain.ReviewPublication{Capture: c, At: now, FXProviderKey: "synthetic-pinned"}
	if err := r.PublishCertifiedReviewProjection(t.Context(), p, func() time.Time { return now }, func(fn func() error) error { return fn() }); err != nil {
		t.Fatal("unknown connection blocked full publication", err)
	}
	saved, err := readReviewTemporal(t.Context(), db.SQL, id)
	if err != nil {
		t.Fatal(err)
	}
	if saved == nil || saved.CertifiedMAC != "" || saved.NoChangeCompletenessVersion != "" {
		t.Fatal("fallback minted authority")
	}
}
