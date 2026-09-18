package diagnostics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLevelChangesAndDisableAffectExistingLogger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "nestworth.log")
	sink := New(path)
	t.Cleanup(func() { _ = sink.Close() })
	logger := sink.Logger().With("component", "test")
	logger.Error("disabled")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("disabled logger created a file")
	}
	if err := sink.Configure("warn"); err != nil {
		t.Fatal(err)
	}
	logger.Info("filtered")
	logger.Warn("warning")
	if err := sink.Configure("debug"); err != nil {
		t.Fatal(err)
	}
	logger.Debug("diagnostic")
	if err := sink.Configure("off"); err != nil {
		t.Fatal(err)
	}
	logger.Error("disabled again")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "disabled") || strings.Contains(text, "filtered") || !strings.Contains(text, "warning") || !strings.Contains(text, "diagnostic") {
		t.Fatal(text)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("log permissions: %v, %v", info, err)
	}
}

func TestRotationRetainsTwoOlderFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nestworth.log")
	sink := New(path)
	t.Cleanup(func() { _ = sink.Close() })
	if err := sink.Configure("debug"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if _, err := sink.Write([]byte(strings.Repeat("x", maxBytes))); err != nil {
			t.Fatal(err)
		}
	}
	for _, suffix := range []string{"", ".1", ".2"} {
		info, err := os.Stat(path + suffix)
		if err != nil || info.Size() > maxBytes {
			t.Fatalf("rotation %s: %v", suffix, err)
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatal("unexpected fourth log file")
	}
}
