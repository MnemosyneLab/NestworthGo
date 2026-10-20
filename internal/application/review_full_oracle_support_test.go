package application

import (
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
	s := NewService(sqlite.NewRepository(db).WithFullReviewRecompute())
	s.fxProviderKey = f.s.FXProviderKey()
	s.setClock(func() time.Time { return *f.now })
	if _, err := s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	return db
}
