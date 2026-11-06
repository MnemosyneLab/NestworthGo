package aiusage

import (
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
