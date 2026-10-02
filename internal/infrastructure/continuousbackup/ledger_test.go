package continuousbackup

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestStreamLedgerSealsOnlyAfterSuccessfulDrain(t *testing.T) {
	m, db, _ := fixture(t)
	if err := m.Configure(context.Background(), testUpdate(true)); err != nil {
		t.Fatal(err)
	}
	v := waitState(t, m, "recent-backup-confirmed")
	c, _, _ := m.store.load()
	records, err := m.store.streams(c)
	if err != nil {
		t.Fatal(err)
	}
	if records[v.StreamID].State != "open" {
		t.Fatal("live stream sealed")
	}
	conn, err := db.SQL.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	err = m.Configure(ctx, testUpdate(false))
	cancel()
	conn.Close()
	if err == nil {
		t.Fatal("drain unexpectedly succeeded")
	}
	records, _ = m.store.streams(c)
	if records[v.StreamID].State != "open" {
		t.Fatal("failed drain sealed")
	}
	if err = m.Configure(context.Background(), testUpdate(false)); err != nil {
		t.Fatal(err)
	}
	records, _ = m.store.streams(c)
	r := records[v.StreamID]
	if r.State != "sealed" || r.SealedAt == "" || r.FinalTXID == "" {
		t.Fatalf("missing seal: %+v", r)
	}
	if r.SealedAt == r.Identity.StartedAt {
		t.Fatal("age derived from start")
	}
	c.Bucket = "other-bucket"
	records, _ = m.store.streams(c)
	if len(records) != 0 {
		t.Fatal("ownership crossed targets")
	}
}
func TestLedgerRestartNeverPromotesAbandonedStream(t *testing.T) {
	p := filepath.Join(t.TempDir(), "backup_config.db")
	s, err := openConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	c, _, _ := s.load()
	c.AccountID = testUpdate(false).AccountID
	c.Bucket = "test-bucket"
	info := newIdentity(c.BackupID + "/" + uuid.NewString())
	if err = s.createStream(c, info); err != nil {
		t.Fatal(err)
	}
	s.db.Close()
	s, err = openConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	records, err := s.streams(c)
	if err != nil {
		t.Fatal(err)
	}
	if records[info.StreamID].State != "open" || records[info.StreamID].SealedAt != "" {
		t.Fatal("abandoned stream promoted")
	}
	if err = s.createStream(c, info); err == nil {
		t.Fatal("ownership overwritten")
	}
}
func TestUnconfirmedShutdownLeavesStreamUnsealed(t *testing.T) {
	m, _, _ := fixture(t)
	if err := m.Configure(context.Background(), testUpdate(true)); err != nil {
		t.Fatal(err)
	}
	v := waitState(t, m, "recent-backup-confirmed")
	c, _, _ := m.store.load()
	m.op.Lock()
	m.pauseWorkerLocked()
	// Simulate canceled sync but allow the synchronous handle drain.
	m.initialized = false
	if err := m.stopLocked(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	m.op.Unlock()
	records, _ := m.store.streams(c)
	if records[v.StreamID].State != "open" {
		t.Fatal("unconfirmed closure sealed")
	}
}
