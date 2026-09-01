package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenReadOnlyForVerifyRejectsMissingFile(t *testing.T) {
	_, err := OpenReadOnlyForVerify(filepath.Join(t.TempDir(), "missing.db"))
	if err == nil {
		t.Fatal("expected missing file to fail")
	}
}

func TestOpenReadOnlyForVerifyDoesNotCreateOrRepair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "live.db")
	live, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := live.Close(); err != nil {
		t.Fatal(err)
	}
	verified, err := OpenReadOnlyForVerify(path)
	if err != nil {
		t.Fatal(err)
	}
	defer verified.Close()
	counts, err := verified.EntityCounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if counts.Households != 0 {
		t.Fatalf("households = %d", counts.Households)
	}
}

func TestSnapshotToRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "live.db")
	live, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	dest := filepath.Join(dir, "snap.db")
	if err := live.SnapshotTo(context.Background(), dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest + "-wal"); !os.IsNotExist(err) {
		t.Fatalf("snapshot unexpectedly has wal: %v", err)
	}
	verified, err := OpenReadOnlyForVerify(dest)
	if err != nil {
		t.Fatal(err)
	}
	_ = verified.Close()
}

func TestOpenReadOnlyForVerifyDoesNotCreateParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-dir", "db.sqlite")
	_, err := OpenReadOnlyForVerify(path)
	if err == nil {
		t.Fatal("expected failure")
	}
	if _, statErr := os.Stat(filepath.Dir(path)); !os.IsNotExist(statErr) {
		t.Fatal("OpenReadOnlyForVerify created a parent directory")
	}
}
