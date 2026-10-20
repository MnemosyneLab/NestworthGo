package application

import (
	"crypto/sha256"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUpcomingActionsActualReadOnlyDatabase(t *testing.T) {
	s, ctx, _, _ := openRegressionProduct(t)
	path := filepath.Join(t.TempDir(), "synthetic-readonly.db")
	if err := s.SnapshotTo(ctx, path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.OpenReadOnlyForVerify(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := sqlite.NewRepository(db)
	snapshot, err := repo.ReadLiquiditySnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	factsBefore, _ := jsoncodec.Marshal(snapshot)
	reader := NewService(repo) // No provider registry is installed.
	for i := 0; i < 35; i++ {
		date := time.Date(2026, 12, 19+i, 23, 59, 59, 0, time.UTC)
		reader.SetClock(func() time.Time { return date })
		if _, err := reader.UpcomingActions(ctx); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err = repo.ReadLiquiditySnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	factsAfter, _ := jsoncodec.Marshal(snapshot)
	if string(factsBefore) != string(factsAfter) {
		t.Fatal("business facts changed during read")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("read-only database bytes changed")
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(path + suffix); !os.IsNotExist(err) {
			t.Fatalf("unexpected read sidecar %s", suffix)
		}
	}
}
