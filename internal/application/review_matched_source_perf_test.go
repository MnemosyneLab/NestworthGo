//go:build perf

package application

import (
	"encoding/json"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Only unpublished synthetic source facts cross separate test binaries. Each
// candidate creates a fresh genesis DB and earns its own same-handle authority.
func matchedReviewSourceFixture(t *testing.T, config reviewPerfConfig) reviewFullPipelineFixture {
	dir := os.Getenv("NW_D2_MATCHED_SOURCE_DIR")
	if dir == "" {
		return newReviewPerfFixture(t, config)
	}
	dir = filepath.Clean(dir)
	if !strings.HasPrefix(dir, "/tmp/") {
		t.Fatal("matched synthetic source must be under /tmp")
	}
	type manifest struct {
		Marker      string
		Config      reviewPerfConfig
		Household   domain.HouseholdID
		Now, Start  time.Time
		Instruments []domain.Instrument
	}
	metaPath := filepath.Join(dir, "synthetic-source.json")
	dbPath := filepath.Join(dir, "synthetic-source.db")
	raw, err := os.ReadFile(metaPath)
	if os.IsNotExist(err) {
		if err = os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		source := newReviewPerfFixture(t, config)
		if _, err = source.db.SQL.Exec(`VACUUM INTO ?`, dbPath); err != nil {
			t.Fatal(err)
		}
		raw, err = json.Marshal(manifest{"nestworth-synthetic-D2-clock-fixture-v1", config, source.householdID, *source.now, source.start, source.instruments})
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(metaPath, raw, 0600); err != nil {
			t.Fatal(err)
		}
		return source
	}
	if err != nil {
		t.Fatal(err)
	}
	var m manifest
	if err = json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.Marker != "nestworth-synthetic-D2-clock-fixture-v1" || m.Config != config {
		t.Fatal("matched synthetic fixture marker/config mismatch")
	}
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	service := NewService(sqlite.NewRepository(db))
	service.setClock(func() time.Time { return m.Now })
	return reviewFullPipelineFixture{service: service, db: db, now: &m.Now, householdID: m.Household, dbPath: dbPath, start: m.Start, instruments: m.Instruments}
}
