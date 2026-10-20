package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"sync"
	"testing"

	modernsqlite "modernc.org/sqlite"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReviewCertifiedUnknownConnectionKeepsFullPublication(t *testing.T) {
	for _, unavailable := range []string{"identity", "commit-counter", "driver-capability"} {
		t.Run(unavailable, func(t *testing.T) { testReviewCertifiedUnavailableProof(t, unavailable) })
	}
}

func testReviewCertifiedUnavailableProof(t *testing.T, unavailable string) {
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
	r := NewRepository(db).WithDefaultReviewOptimizations()
	switch unavailable {
	case "identity":
		db.reviewCertificateKey.connectionID = ^uint64(0)
	case "commit-counter":
		conn, _, err := r.certifiedConnection(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		conn.Close()
		db.reviewCertificateKey.commits.Store(^uint64(0))
	case "driver-capability":
		registerNoCommitHookDriver.Do(func() { sql.Register("review-no-commit-hook", noCommitHookDriver{}) })
		if err := db.SQL.Close(); err != nil {
			t.Fatal(err)
		}
		pool, err := sql.Open("review-no-commit-hook", db.Path+"?_txlock=immediate&_pragma=busy_timeout%3d5000&_pragma=foreign_keys%3d1&_pragma=recursive_triggers%3d1")
		if err != nil {
			t.Fatal(err)
		}
		pool.SetMaxOpenConns(1)
		db.SQL = pool
	}
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	c, err := r.CaptureReviewProjection(t.Context(), id, now)
	if err != nil {
		t.Fatal(err)
	}
	if c.Certified == nil || validCertifiedConnection(c.Certified.Connection) || c.Certified.MayCertify {
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

var registerNoCommitHookDriver sync.Once

type noCommitHookDriver struct{}

func (noCommitHookDriver) Open(name string) (driver.Conn, error) {
	c, err := (&modernsqlite.Driver{}).Open(name)
	if err != nil {
		return nil, err
	}
	return &noCommitHookConn{Conn: c}, nil
}

// Embedding driver.Conn deliberately hides the concrete driver's hook method.
type noCommitHookConn struct{ driver.Conn }

func (c *noCommitHookConn) BeginTx(ctx context.Context, options driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, options)
}
