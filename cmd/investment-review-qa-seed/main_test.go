package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSeedRefusesExistingPaths(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "do-not-change")
	if err := os.WriteFile(marker, []byte("synthetic marker"), 0600); err != nil {
		t.Fatal(err)
	}
	if seed(dir, "2026-10-13") == nil {
		t.Fatal("existing directory accepted")
	}
	raw, err := os.ReadFile(marker)
	if err != nil || string(raw) != "synthetic marker" {
		t.Fatal("existing data changed")
	}
	if seed("", "2026-10-13") == nil {
		t.Fatal("empty directory accepted")
	}
}
