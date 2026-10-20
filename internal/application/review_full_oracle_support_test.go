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
}
type fullReviewTestRepository struct{ reviewTestProjectionRepository }

func fullReviewOracle(r reviewTestProjectionRepository) *fullReviewTestRepository {
	return &fullReviewTestRepository{reviewTestProjectionRepository: r}
}
func (r *fullReviewTestRepository) CertifiedReviewEnabled() bool { return false }
func (r *fullReviewTestRepository) CaptureReviewProjection(ctx context.Context, id domain.HouseholdID, at time.Time) (domain.ReviewProjectionCapture, error) {
	c, err := r.reviewTestProjectionRepository.CaptureReviewProjection(ctx, id, at)
	if err != nil {
		return c, err
	}
	c.TemporalFullRecompute = true
	c.ReusableDailyEvidence = nil
	c.Suffix = nil
	c.Certified = nil
	return c, nil
}
