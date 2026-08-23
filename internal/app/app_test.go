package app

import (
	"path/filepath"
	"testing"
)

func TestDefaultDatabasePathHonorsIsolatedDatabasePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "isolated", "nestworth.db")
	t.Setenv("NESTWORTH_DATABASE_PATH", path)
	if got := defaultDatabasePath(); got != path {
		t.Fatalf("defaultDatabasePath() = %q, want %q", got, path)
	}
}
