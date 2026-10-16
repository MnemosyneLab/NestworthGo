package application

import (
	"database/sql"
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
)

// Compatibility fixtures only: legacy databases can contain archives that the
// current transactional write guard correctly rejects. Keep that read coverage.
func seedLegacyInstrumentArchive(t *testing.T, s *Service, id domain.InstrumentID) error {
	t.Helper()
	return seedLegacyArchive(t, s, "instruments", id.String())
}
func seedLegacyHoldingArchive(t *testing.T, s *Service, id domain.HoldingID) error {
	t.Helper()
	return seedLegacyArchive(t, s, "holdings", id.String())
}
func seedLegacyArchive(t *testing.T, s *Service, table, id string) error {
	t.Helper()
	db, err := sql.Open("sqlite", s.repository.(interface{ Path() string }).Path()+"?_pragma=foreign_keys%3d1")
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec("UPDATE "+table+" SET archived_at=?,updated_at=? WHERE id=?", s.clock().UTC().Format("2006-01-02T15:04:05.000Z"), s.clock().UTC().Format("2006-01-02T15:04:05.000Z"), id)
	if err == nil {
		s.invalidateAnalysis()
	}
	return err
}
