package application

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"path/filepath"
	"testing"
	"time"
)

func windowOracleReference(t *testing.T, f seriesZeroFixture) *sqlite.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic-pre-publication.db")
	if _, err := f.db.SQL.Exec(`VACUUM INTO ?`, path); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := NewService(fullReviewOracle(sqlite.NewRepository(db)))
	s.fxProviderKey = f.s.FXProviderKey()
	s.setClock(func() time.Time { return *f.now })
	if _, err := s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	return db
}

// Test-owned full preparation strips every captured reuse authority. The real
// repository still performs all source/revision/clock/publication guards; no
// production constructor or test boolean chooses an alternative algorithm.
type reviewTestProjectionRepository interface {
	Repository
	ReviewProjectionRepository
	CertifiedReviewEnabled() bool
	WithDefaultReviewOptimizations() *sqlite.Repository
}
type fullReviewTestRepository struct{ reviewTestProjectionRepository }

func fullReviewOracle(r reviewTestProjectionRepository) *fullReviewTestRepository {
	return &fullReviewTestRepository{reviewTestProjectionRepository: r}
}
func (r *fullReviewTestRepository) CertifiedReviewEnabled() bool { return false }
func (r *fullReviewTestRepository) CaptureReviewProjection(ctx context.Context, id domain.HouseholdID, at time.Time) (domain.ReviewProjectionCapture, error) {
	// The supported certified capture always bypasses C2 window decoding. Its
	// suffix reader treats malformed derived rows as a full-source fallback;
	// discard even valid suffix results below. This keeps the oracle independent
	// of corrupted cache bytes without a production force-full control.
	c, err := r.reviewTestProjectionRepository.WithDefaultReviewOptimizations().CaptureReviewProjection(ctx, id, at)
	if err != nil {
		return c, err
	}
	c.TemporalFullRecompute = true
	c.ReusableDailyEvidence = nil
	c.Suffix = nil
	c.Certified = nil
	return c, nil
}

func independentReviewFullStage(t *testing.T, f seriesZeroFixture, r reviewTestProjectionRepository) domain.ReviewPublication {
	t.Helper()
	c, err := fullReviewOracle(r).CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	p, err := PrepareReviewPublication(t.Context(), c, *f.now, f.s.FXProviderKey())
	if err != nil {
		t.Fatal(err)
	}
	return p
}
