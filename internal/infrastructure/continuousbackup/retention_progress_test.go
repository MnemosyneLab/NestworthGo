package continuousbackup

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// In-memory object storage keeps this regression about durable checkpoint work,
// not filesystem throughput. All keys and manifests are synthetic.
type largeStreamStorage struct {
	objects      map[string]storedObject
	deleted      int
	calls        int
	budget       int
	cancel       context.CancelFunc
	lostResponse bool
	metadataLast bool
}

func (b *largeStreamStorage) inventory(ctx context.Context, _ string) ([]storedObject, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	result := make([]storedObject, 0, len(b.objects))
	for _, o := range b.objects {
		result = append(result, o)
	}
	return result, nil
}
func (b *largeStreamStorage) deleteObject(ctx context.Context, o storedObject) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if b.objects[o.Key] != o {
		return ErrRetention
	}
	if strings.HasSuffix(o.Key, "/"+identityFile) {
		b.metadataLast = len(b.objects) == 1
		if !b.metadataLast {
			return ErrRetention
		}
	}
	delete(b.objects, o.Key)
	b.deleted++
	b.calls++
	if b.budget > 0 && b.calls >= b.budget {
		b.cancel()
	}
	if b.lostResponse {
		b.lostResponse = false
		return ErrUnavailable
	}
	return nil
}
func largeDeletingStream(t *testing.T, count int) (*Manager, config, streamRecord, *largeStreamStorage, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "backup_config.db")
	store, err := openConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{store: store}
	t.Cleanup(func() { m.store.db.Close() })
	c, _, err := store.load()
	if err != nil {
		t.Fatal(err)
	}
	c.AccountID = strings.Repeat("a", 32)
	c.Bucket = "synthetic-large-stream"
	identity := newIdentity(c.BackupID + "/" + uuid.NewString())
	if err := store.createStream(c, identity); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TABLE checkpoint_audit(kind TEXT, bytes INTEGER);
 CREATE TRIGGER manifest_updates AFTER UPDATE ON backup_streams BEGIN INSERT INTO checkpoint_audit VALUES('manifest',length(NEW.record)); END;
 CREATE TRIGGER progress_inserts AFTER INSERT ON backup_stream_progress BEGIN INSERT INTO checkpoint_audit VALUES('progress',length(NEW.target)+length(NEW.stream)+8); END;
 CREATE TRIGGER progress_updates AFTER UPDATE ON backup_stream_progress BEGIN INSERT INTO checkpoint_audit VALUES('progress',length(NEW.target)+length(NEW.stream)+8); END;`); err != nil {
		t.Fatal(err)
	}
	r := streamRecord{Identity: identity, State: "deleting", FinalTXID: "0000000000000001", SealedAt: time.Now().AddDate(0, 0, -100).Format(time.RFC3339Nano)}
	b := &largeStreamStorage{objects: map[string]storedObject{}}
	for i := 1; i <= count; i++ {
		o := storedObject{Key: fmt.Sprintf("%s%s/ltx/0/%016x-%016x.ltx", remotePrefix, identity.StreamID, i, i), Size: 100, ETag: fmt.Sprint(i)}
		r.Objects = append(r.Objects, o)
		b.objects[o.Key] = o
	}
	metadata := storedObject{Key: remotePrefix + identity.StreamID + "/" + identityFile, Size: 200, ETag: "metadata"}
	r.Objects = append(r.Objects, metadata)
	b.objects[metadata.Key] = metadata
	if err := store.writeStream(c, r); err != nil {
		t.Fatal(err)
	}
	return m, c, r, b, path
}
func progressAudit(t *testing.T, m *Manager) (manifests, checkpoints, bytes int) {
	t.Helper()
	if err := m.store.db.QueryRow(`SELECT COUNT(*),COALESCE(SUM(bytes),0) FROM checkpoint_audit WHERE kind='manifest'`).Scan(&manifests, &bytes); err != nil {
		t.Fatal(err)
	}
	if err := m.store.db.QueryRow(`SELECT COUNT(*) FROM checkpoint_audit WHERE kind='progress'`).Scan(&checkpoints); err != nil {
		t.Fatal(err)
	}
	return
}
func deletingRecord(t *testing.T, m *Manager, c config, stream string) streamRecord {
	t.Helper()
	rows, err := m.store.streams(c)
	if err != nil {
		t.Fatal(err)
	}
	return rows[stream]
}
func TestSingleLargeStreamHasBoundedCheckpointWrites(t *testing.T) {
	const count = 72000
	m, c, r, b, _ := largeDeletingStream(t, count)
	present, err := b.inventory(context.Background(), c.BackupID)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.deleteStreamLocked(context.Background(), c, b, r, present); err != nil {
		t.Fatal(err)
	}
	manifests, checkpoints, bytes := progressAudit(t, m)
	// Exactly one immutable manifest write, then one small completion tombstone.
	// A larger stream must not multiply its manifest bytes by its object count.
	if manifests != 2 || bytes > len(r.Objects)*512 {
		t.Fatalf("manifest rewrites: %d writes, %d bytes for %d objects", manifests, bytes, len(r.Objects))
	}
	if checkpoints > (count+1+deletionCheckpointBatch-1)/deletionCheckpointBatch+2 {
		t.Fatalf("unbounded scalar checkpoints: %d", checkpoints)
	}
	if len(b.objects) != 0 || !b.metadataLast || b.deleted != count+1 {
		t.Fatal("incomplete or unsafe deletion")
	}
	if got := deletingRecord(t, m, c, r.Identity.StreamID); got.State != "deleted" || got.DeletedObjects != count+1 || len(got.Objects) != 0 {
		t.Fatal("bad final tombstone")
	}
	t.Logf("single stream: %d objects, %d manifest writes / %d bytes, %d scalar checkpoints", count+1, manifests, bytes, checkpoints)
}
func TestTightBudgetResumesMakeMonotonicProgressWithoutManifestRewrites(t *testing.T) {
	const count = 4096
	const budget = 73 // Smaller than a checkpoint batch: every attempt is interrupted.
	m, c, r, b, path := largeDeletingStream(t, count)
	var original string
	if err := m.store.db.QueryRow(`SELECT record FROM backup_streams WHERE stream=?`, r.Identity.StreamID).Scan(&original); err != nil {
		t.Fatal(err)
	}
	previous := 0
	attempts := 0
	for r.State != "deleted" {
		attempts++
		if attempts > 2*((count+1+budget-1)/budget)+2 {
			t.Fatal("resume exhausted bounded attempts")
		}
		before := len(b.objects)
		// A deterministic small I/O budget exercises the same cancellation boundary
		// as the production deadline without widening timeouts for slow race builds.
		ctx, cancel := context.WithCancel(context.Background())
		b.calls = 0
		b.budget = budget
		b.cancel = cancel
		b.lostResponse = attempts%7 == 0
		present, err := b.inventory(ctx, c.BackupID)
		if err != nil {
			t.Fatal(err)
		}
		r = deletingRecord(t, m, c, r.Identity.StreamID)
		err = m.deleteStreamLocked(ctx, c, b, r, present)
		cancel()
		if err != nil && !errors.Is(err, ErrRetention) {
			t.Fatal(err)
		}
		if before > 0 && len(b.objects) >= before {
			t.Fatalf("attempt %d made no new progress", attempts)
		}
		r = deletingRecord(t, m, c, r.Identity.StreamID)
		if r.DeletedObjects < previous || r.DeletedObjects > b.deleted {
			t.Fatalf("nonmonotonic/false checkpoint: %d -> %d, actual %d", previous, r.DeletedObjects, b.deleted)
		}
		previous = r.DeletedObjects
		if r.State == "deleting" {
			var saved string
			if err := m.store.db.QueryRow(`SELECT record FROM backup_streams WHERE stream=?`, r.Identity.StreamID).Scan(&saved); err != nil {
				t.Fatal(err)
			}
			if saved != original {
				t.Fatal("resume rewrote immutable manifest")
			}
		}
		// Reopen the control DB periodically; no in-memory cursor can hide replay.
		if attempts%3 == 0 {
			m.store.db.Close()
			var err error
			m.store, err = openConfig(path)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	manifests, checkpoints, _ := progressAudit(t, m)
	if manifests != 2 || checkpoints > attempts || !b.metadataLast {
		t.Fatalf("work grew with completed history: manifests=%d checkpoints=%d attempts=%d", manifests, checkpoints, attempts)
	}
	if previous != count+1 {
		t.Fatal("lost responses were not reconciled")
	}
	t.Logf("%d interrupted attempts, %d scalar checkpoints, %d manifest writes", attempts, checkpoints, manifests)
}
func TestAlreadyAbsentObjectsDoNotWriteCheckpoints(t *testing.T) {
	m, c, r, b, _ := largeDeletingStream(t, 20000)
	// Simulate remote success followed by a crash before any progress commit.
	b.objects = map[string]storedObject{}
	if err := m.deleteStreamLocked(context.Background(), c, b, r, nil); err != nil {
		t.Fatal(err)
	}
	manifests, checkpoints, _ := progressAudit(t, m)
	if manifests != 2 || checkpoints != 0 || b.deleted != 0 {
		t.Fatalf("replayed absent history: %d %d %d", manifests, checkpoints, b.deleted)
	}
}
func TestRetentionStatusAndCancelRemainAvailableDuringBackgroundCleanup(t *testing.T) {
	m, _, root, _ := sealedFixture(t, 3)
	activateCleanup(t, m)
	b := &cleanupFault{fileBackend: fileBackend{root}, inventoryGate: make(chan struct{}), entered: make(chan struct{}, 1)}
	m.factory = func(config) backend { return b }
	done := make(chan struct{})
	go func() { defer close(done); m.backgroundCleanup() }()
	select {
	case <-b.entered:
	case <-time.After(time.Second):
		t.Fatal("background did not start")
	}
	status := make(chan RetentionStatus, 1)
	errs := make(chan error, 1)
	go func() { r, err := m.RetentionStatus(); status <- r; errs <- err }()
	select {
	case r := <-status:
		if err := <-errs; err != nil || !r.Running || r.Result != "running" {
			t.Fatalf("not observable: %+v %v", r, err)
		}
	case <-time.After(time.Second):
		m.CancelCleanup()
		<-done
		t.Fatal("poll blocked behind cleanup operation lock")
	}
	m.CancelCleanup()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("background cancellation blocked")
	}
	r, err := m.RetentionStatus()
	if err != nil || r.Running || r.Result != "stopped" {
		t.Fatalf("stale running state: %+v %v", r, err)
	}
	if len(b.deleted) != 0 {
		t.Fatal("mutated blocked storage")
	}
}
