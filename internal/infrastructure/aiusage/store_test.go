package aiusage

import (
	"bytes"
	"context"
	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUsageSurvivesReopenAndUpserts(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := assistant.BeginUsage(context.Background(), "test-model")
	c.Kind = "conversation"
	c.ConversationID = uuid.NewString()
	if err = s.Record(c); err != nil {
		t.Fatal(err)
	}
	c.Status = "succeeded"
	now := time.Now().UTC()
	c.Finished = &now
	if err = s.Record(c); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	calls, err := s.Calls()
	if err != nil || len(calls) != 1 || calls[0].Status != "succeeded" || calls[0].Input != nil {
		t.Fatalf("%+v %v", calls, err)
	}
	info, _ := os.Stat(filepath.Join(dir, "usage.db"))
	if info.Mode().Perm() != 0600 {
		t.Fatal("unsafe mode")
	}
}
func TestUsageRejectsSymlinks(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	_ = os.Mkdir(dir, 0700)
	if err := os.Symlink(filepath.Join(t.TempDir(), "other"), filepath.Join(dir, "usage.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("accepted symlink")
	}
}

func TestUsageRejectsFutureBeforeWritableSQLiteOpen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	_ = os.Mkdir(dir, 0700)
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("PRAGMA user_version=99"); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	before, _ := os.ReadFile(filepath.Join(dir, "usage.db"))
	if _, err = Open(dir); err == nil {
		t.Fatal("accepted future version")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "usage.db"))
	if !bytes.Equal(before, after) {
		t.Fatal("modified unsupported database")
	}
}
func TestUsageFailureMarkerSurvivesRestart(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	_ = os.Mkdir(dir, 0700)
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Record(assistant.UsageCall{}) == nil || !s.Incomplete() {
		t.Fatal("hidden invalid record")
	}
	_ = s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.Incomplete() {
		t.Fatal("lost coverage marker")
	}
}

// A report holds a SQLite read snapshot while new attempts are recorded. This
// deterministically reproduces the former single-connection pool starvation.
func TestReportSnapshotDoesNotBlockAccounting(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := assistant.BeginUsage(context.Background(), "test-model")
	c.Kind = "conversation"
	if err = s.Record(c); err != nil {
		t.Fatal(err)
	}
	rows, err := s.reader.Query("SELECT payload FROM usage_calls")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("no snapshot")
	}
	for i := 0; i < 20; i++ {
		c := assistant.BeginUsage(context.Background(), "test-model")
		c.Kind = "conversation"
		if err = s.Record(c); err != nil {
			t.Fatalf("write during snapshot: %v", err)
		}
	}
	if err = rows.Close(); err != nil {
		t.Fatal(err)
	}
	calls, err := s.Calls()
	if err != nil || len(calls) != 21 || s.Incomplete() {
		t.Fatalf("calls=%d incomplete=%v error=%v", len(calls), s.Incomplete(), err)
	}
}
