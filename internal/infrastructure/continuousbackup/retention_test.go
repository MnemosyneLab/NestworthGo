package continuousbackup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type cleanupFault struct {
	fileBackend
	mu            sync.Mutex
	deleted       []string
	failAt        int
	afterDelete   func()
	inventoryGate chan struct{}
	entered       chan struct{}
}

func (b *cleanupFault) inventory(ctx context.Context, owner string) ([]storedObject, error) {
	if b.inventoryGate != nil {
		select {
		case b.entered <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-b.inventoryGate:
		}
	}
	return b.fileBackend.inventory(ctx, owner)
}
func (b *cleanupFault) deleteObject(ctx context.Context, o storedObject) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failAt > 0 && len(b.deleted)+1 == b.failAt {
		return errors.New("provider echoed local-test-secret and local-test-access")
	}
	if err := b.fileBackend.deleteObject(ctx, o); err != nil {
		return err
	}
	b.deleted = append(b.deleted, o.Key)
	if b.afterDelete != nil {
		b.afterDelete()
	}
	return nil
}
func activateCleanup(t *testing.T, m *Manager) {
	t.Helper()
	if _, err := m.ConfigureRetention(RetentionUpdate{Enabled: true, Days: 30, Acknowledged: true}); err != nil {
		t.Fatal(err)
	}
}
func runCleanup(t *testing.T, m *Manager) (RetentionStatus, error) {
	t.Helper()
	p, err := m.PreviewRetention(context.Background(), 30)
	if err != nil {
		t.Fatal(err)
	}
	return m.ExecuteRetention(context.Background(), CleanupRequest{Token: p.Token, Acknowledged: true})
}
func TestCleanupDeletesWholeStreamsAndRetainedStreamsStillRestore(t *testing.T) {
	m, c, root, streams := sealedFixture(t, 5)
	foreign := newIdentity(uuid.NewString() + "/" + uuid.NewString())
	if err := (fileBackend{root}).ensureIdentity(context.Background(), foreign); err != nil {
		t.Fatal(err)
	}
	legacy := c.BackupID + "/" + uuid.NewString()
	if err := (fileBackend{root}).ensureIdentity(context.Background(), newIdentity(legacy)); err != nil {
		t.Fatal(err)
	}
	crash := newIdentity(c.BackupID + "/" + uuid.NewString())
	if err := m.store.createStream(c, crash); err != nil {
		t.Fatal(err)
	}
	if err := (fileBackend{root}).ensureIdentity(context.Background(), crash); err != nil {
		t.Fatal(err)
	}
	if err := m.Configure(context.Background(), testUpdate(true)); err != nil {
		t.Fatal(err)
	}
	live := waitState(t, m, "recent-backup-confirmed")
	b := &cleanupFault{fileBackend: fileBackend{root}}
	m.factory = func(config) backend { return b }
	activateCleanup(t, m)
	before, _ := m.View()
	r, err := runCleanup(t, m)
	if err != nil || r.Result != "completed" {
		t.Fatalf("cleanup %+v %v", r, err)
	}
	after, _ := m.View()
	if after.LastSuccessfulBackup != before.LastSuccessfulBackup || after.ErrorSummary != before.ErrorSummary {
		t.Fatal("cleanup changed backup evidence")
	}
	for _, stream := range streams[:3] {
		files, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(stream)))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if !f.IsDir() {
				t.Fatal("stream metadata remains")
			}
		}
	}
	for _, stream := range []string{foreign.StreamID, legacy, crash.StreamID, live.StreamID} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(stream), identityFile)); err != nil {
			t.Fatalf("protected stream deleted %s", stream)
		}
	}
	// Restore retained replicas after cleanup with the real Litestream reader and
	// the existing schema/integrity/foreign-key verifier, not metadata promises.
	records, _ := m.store.streams(c)
	for i, stream := range streams[3:] {
		dest := filepath.Join(t.TempDir(), "restored.db")
		if err := restorePoint(context.Background(), b, RecoveryPoint{StreamID: stream, TXID: records[stream].FinalTXID}, dest); err != nil {
			t.Fatal(err)
		}
		db, err := sqlite.OpenReadOnlyForVerify(dest)
		if err != nil {
			t.Fatal(err)
		}
		var value string
		if err := db.SQL.QueryRow(`SELECT value FROM app_configuration WHERE key='retention-fixture'`).Scan(&value); err != nil || value != fmt.Sprint(i+3) {
			t.Fatal("retained business snapshot changed", value, err)
		}
		db.Close()
	}
	for _, stream := range streams[:3] {
		last := ""
		for _, key := range b.deleted {
			if strings.HasPrefix(key, remotePrefix+stream+"/") {
				last = key
			}
		}
		if last != remotePrefix+stream+"/"+identityFile {
			t.Fatal("metadata not last")
		}
	}
}
func TestPartialDeleteRestartResumesIdempotently(t *testing.T) {
	m, c, root, streams := sealedFixture(t, 3)
	b := &cleanupFault{fileBackend: fileBackend{root}, failAt: 2}
	m.factory = func(config) backend { return b }
	activateCleanup(t, m)
	r, err := runCleanup(t, m)
	if !errors.Is(err, ErrRetention) || r.Result != "stopped" || strings.Contains(r.ErrorSummary, "local-test-") {
		t.Fatalf("unsafe error %+v %v", r, err)
	}
	records, _ := m.store.streams(c)
	if records[streams[0]].State != "deleting" || records[streams[0]].DeletedObjects != 1 {
		t.Fatal("partial outcome not durable")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(streams[0]), identityFile)); err != nil {
		t.Fatal("metadata removed before data")
	}
	if err := m.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	next, err := New(m.path, m.database)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close(context.Background())
	next.factory = func(config) backend { return fileBackend{root} }
	r, err = runCleanup(t, next)
	if err != nil || r.Result != "completed" {
		t.Fatalf("resume %+v %v", r, err)
	}
	records, _ = next.store.streams(c)
	if records[streams[0]].State != "deleted" {
		t.Fatal("resume not complete")
	}
	r, err = runCleanup(t, next)
	if err != nil {
		t.Fatal("repeat not idempotent", err)
	}
}
func TestStalePreviewAndUnknownObjectsAbortBeforeDeletion(t *testing.T) {
	// n=3 keeps one expired stream beside the two verified survivors. One
	// replica build is reused; each mode undoes its own persistent mutation
	// so a later rejection still sees that eligible stream.
	m, c, root, streams := sealedFixture(t, 3)
	b := &cleanupFault{fileBackend: fileBackend{root}}
	m.factory = func(config) backend { return b }
	activateCleanup(t, m)
	records, err := m.store.streams(c)
	if err != nil {
		t.Fatal(err)
	}
	originalSeal := records[streams[0]].SealedAt
	unknown := filepath.Join(root, filepath.FromSlash(streams[0]), "unexpected.txt")
	for _, mode := range []string{"expired", "unknown", "changed", "config", "pin", "disabled", "unacknowledged"} {
		t.Run(mode, func(t *testing.T) {
			p, err := m.PreviewRetention(context.Background(), 30)
			if err != nil {
				t.Fatal(err)
			}
			request := CleanupRequest{Token: p.Token, Acknowledged: true}
			switch mode {
			case "expired":
				m.retentionPreview.View.ScannedAt = time.Now().Add(-time.Hour).Format(time.RFC3339Nano)
			case "unknown":
				if err = os.WriteFile(unknown, []byte("test"), 0600); err != nil {
					t.Fatal(err)
				}
			case "changed":
				records, _ := m.store.streams(c)
				r := records[streams[0]]
				r.SealedAt = time.Now().Format(time.RFC3339Nano)
				if err = m.store.writeStream(c, r); err != nil {
					t.Fatal(err)
				}
			case "config":
				u := testUpdate(false)
				u.Bucket = "other-target"
				if err = m.Configure(context.Background(), u); err != nil {
					t.Fatal(err)
				}
			case "pin":
				m.previewPins[streams[0]] = true
			case "disabled":
				if _, err = m.ConfigureRetention(RetentionUpdate{Days: 30}); err != nil {
					t.Fatal(err)
				}
			case "unacknowledged":
				request.Acknowledged = false
			}
			if _, err = m.ExecuteRetention(context.Background(), request); err == nil {
				t.Fatal("stale or unsafe request accepted")
			}
			if len(b.deleted) != 0 {
				t.Fatal("objects deleted before revalidation")
			}
			switch mode {
			case "unknown":
				if err = os.Remove(unknown); err != nil {
					t.Fatal(err)
				}
			case "changed":
				records, _ = m.store.streams(c)
				r := records[streams[0]]
				r.SealedAt = originalSeal
				if err = m.store.writeStream(c, r); err != nil {
					t.Fatal(err)
				}
			case "config":
				if err = m.Configure(context.Background(), testUpdate(false)); err != nil {
					t.Fatal(err)
				}
			case "pin":
				delete(m.previewPins, streams[0])
			case "disabled":
				activateCleanup(t, m)
			}
		})
	}
}
func TestConcurrentRestoreAndConfigCancelCleanup(t *testing.T) {
	for _, mode := range []string{"restore", "config", "close"} {
		t.Run(mode, func(t *testing.T) {
			m, _, root, _ := sealedFixture(t, 3)
			activateCleanup(t, m)
			p, err := m.PreviewRetention(context.Background(), 30)
			if err != nil {
				t.Fatal(err)
			}
			b := &cleanupFault{fileBackend: fileBackend{root}, inventoryGate: make(chan struct{}), entered: make(chan struct{}, 1)}
			m.factory = func(config) backend { return b }
			done := make(chan error, 1)
			go func() {
				_, err := m.ExecuteRetention(context.Background(), CleanupRequest{Token: p.Token, Acknowledged: true})
				done <- err
			}()
			select {
			case <-b.entered:
			case <-time.After(time.Second):
				t.Fatal("not started")
			}
			switch mode {
			case "restore":
				err = m.PauseForRestore(context.Background())
			case "config":
				err = m.Configure(context.Background(), testUpdate(false))
			case "close":
				err = m.Close(context.Background())
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case err = <-done:
				if err == nil {
					t.Fatal("concurrent change did not cancel")
				}
			case <-time.After(time.Second):
				t.Fatal("cleanup not joined")
			}
			if len(b.deleted) != 0 {
				t.Fatal("deleted during target/restore change")
			}
		})
	}
}
func TestBackgroundCleanupIsOffByDefaultAndAtMostDaily(t *testing.T) {
	m, c, root, _ := sealedFixture(t, 3)
	b := &cleanupFault{fileBackend: fileBackend{root}}
	m.factory = func(config) backend { return b }
	m.backgroundCleanup()
	if len(b.deleted) != 0 {
		t.Fatal("default cleanup enabled")
	}
	activateCleanup(t, m)
	m.backgroundCleanup()
	if len(b.deleted) == 0 {
		t.Fatal("background did not clean")
	}
	first, _ := m.store.retention(c)
	m.backgroundCleanup()
	second, _ := m.store.retention(c)
	if first.LastAttempt != second.LastAttempt {
		t.Fatal("background repeated within day")
	}
}
func TestMissingMetadataAfterInterruptedDeletionFinishesSafely(t *testing.T) {
	m, c, root, streams := sealedFixture(t, 3)
	activateCleanup(t, m)
	p, err := m.PreviewRetention(context.Background(), 30)
	if err != nil {
		t.Fatal(err)
	}
	r := m.retentionPreview.Records[streams[0]]
	r.State = "deleting"
	r.Objects = m.retentionPreview.Objects[streams[0]]
	if err = m.store.writeStream(c, r); err != nil {
		t.Fatal(err)
	}
	for _, o := range r.Objects {
		if err = (fileBackend{root}).deleteObject(context.Background(), o); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = m.ExecuteRetention(context.Background(), CleanupRequest{Token: p.Token, Acknowledged: true}); err == nil {
		t.Fatal("pre-crash plan accepted")
	}
	if _, err = runCleanup(t, m); err != nil {
		t.Fatal(err)
	}
	records, _ := m.store.streams(c)
	if records[streams[0]].State != "deleted" {
		t.Fatal("lost metadata response not resumed")
	}
}

// hookedRetentionStorage is in-memory inventory with the file backend's unused
// client surface, so executePlanLocked can fail a sealed-stream checkpoint
// without standing up a Litestream replica.
type hookedRetentionStorage struct {
	fileBackend
	inner       *largeStreamStorage
	afterDelete func()
}

func (b *hookedRetentionStorage) inventory(ctx context.Context, owner string) ([]storedObject, error) {
	return b.inner.inventory(ctx, owner)
}
func (b *hookedRetentionStorage) deleteObject(ctx context.Context, o storedObject) error {
	if err := b.inner.deleteObject(ctx, o); err != nil {
		return err
	}
	if b.afterDelete != nil {
		b.afterDelete()
	}
	return nil
}

func syntheticSealedStream(t *testing.T) (*Manager, config, string, *hookedRetentionStorage) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "backup_config.db")
	store, err := openConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{store: store, previewPins: map[string]bool{}}
	t.Cleanup(func() { m.store.db.Close() })
	c, _, err := store.load()
	if err != nil {
		t.Fatal(err)
	}
	c.AccountID = strings.Repeat("a", 32)
	c.Bucket = "synthetic-sealed-stream"
	identity := newIdentity(c.BackupID + "/" + uuid.NewString())
	if err := store.createStream(c, identity); err != nil {
		t.Fatal(err)
	}
	r := streamRecord{Identity: identity, State: "sealed", FinalTXID: "0000000000000001", SealedAt: time.Now().UTC().AddDate(0, 0, -100).Format(time.RFC3339Nano)}
	ltxObject := storedObject{Key: fmt.Sprintf("%s%s/ltx/0/%016x-%016x.ltx", remotePrefix, identity.StreamID, 1, 1), Size: 100, ETag: "1"}
	metadata := storedObject{Key: remotePrefix + identity.StreamID + "/" + identityFile, Size: 200, ETag: "metadata"}
	storage := &hookedRetentionStorage{inner: &largeStreamStorage{objects: map[string]storedObject{
		ltxObject.Key: ltxObject,
		metadata.Key:  metadata,
	}}}
	if err := store.writeStream(c, r); err != nil {
		t.Fatal(err)
	}
	m.factory = func(config) backend { return storage }
	return m, c, identity.StreamID, storage
}

func syntheticRetentionPlan(t *testing.T, m *Manager, c config, storage *hookedRetentionStorage) retentionPlan {
	t.Helper()
	all, err := storage.inventory(context.Background(), c.BackupID)
	if err != nil {
		t.Fatal(err)
	}
	objects, err := groupInventory(c.BackupID, all)
	if err != nil {
		t.Fatal(err)
	}
	records, err := m.store.streams(c)
	if err != nil {
		t.Fatal(err)
	}
	items := make([]RetentionItem, 0, len(records))
	for stream, r := range records {
		items = append(items, RetentionItem{StreamID: stream, SealedAt: r.SealedAt, Eligible: r.State == "sealed" || r.State == "deleting"})
	}
	return retentionPlan{
		View:        RetentionPreview{Ready: true, Items: items},
		Records:     records,
		Objects:     objects,
		Fingerprint: inventoryFingerprint("", objects, records),
	}
}

func TestDurableDeletingCheckpointFailureNeverLosesOwnership(t *testing.T) {
	for _, when := range []string{"before", "after"} {
		t.Run(when, func(t *testing.T) {
			m, c, stream, storage := syntheticSealedStream(t)
			trigger := `CREATE TRIGGER reject_deleting BEFORE UPDATE ON backup_streams WHEN json_extract(NEW.record,'$.State')='deleting' BEGIN SELECT RAISE(FAIL,'simulated full disk'); END`
			if when == "before" {
				if _, err := m.store.db.Exec(trigger); err != nil {
					t.Fatal(err)
				}
			} else {
				storage.afterDelete = func() {
					_, _ = m.store.db.Exec(`CREATE TRIGGER reject_deleting BEFORE INSERT ON backup_stream_progress BEGIN SELECT RAISE(FAIL,'simulated full disk'); END`)
				}
			}
			if err := m.executePlanLocked(context.Background(), c, syntheticRetentionPlan(t, m, c, storage)); err == nil {
				t.Fatal("disk failure ignored")
			}
			if when == "before" && storage.inner.deleted != 0 {
				t.Fatal("deleted before durable intent")
			}
			records, _ := m.store.streams(c)
			if when == "after" && records[stream].State != "deleting" {
				t.Fatal("intent lost")
			}
			_, _ = m.store.db.Exec(`DROP TRIGGER reject_deleting`)
			storage.afterDelete = nil
			if err := m.executePlanLocked(context.Background(), c, syntheticRetentionPlan(t, m, c, storage)); err != nil {
				t.Fatal("checkpoint resume failed", err)
			}
		})
	}
}
func TestSurvivorVerificationHonorsCancellation(t *testing.T) {
	m, _, _, _ := sealedFixture(t, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if db, err := sqlite.OpenReadOnlyForVerifyContext(ctx, m.path); err == nil {
		db.Close()
		t.Fatal("canceled validation succeeded")
	}
	if err := m.database.Verify(context.Background()); err != nil {
		t.Fatal("verification changed source", err)
	}
}

func TestBackgroundDefersWhileCleanupPreviewIsOpen(t *testing.T) {
	m, _, root, _ := sealedFixture(t, 3)
	b := &cleanupFault{fileBackend: fileBackend{root}}
	m.factory = func(config) backend { return b }
	activateCleanup(t, m)
	if _, err := m.PreviewRetention(context.Background(), 30); err != nil {
		t.Fatal(err)
	}
	m.backgroundCleanup()
	if len(b.deleted) != 0 {
		t.Fatal("background deleted preview candidates")
	}
	m.retentionPreview.View.ScannedAt = time.Now().Add(-time.Hour).Format(time.RFC3339Nano)
	m.backgroundCleanup()
	if len(b.deleted) == 0 {
		t.Fatal("expired preview prevented next automatic run")
	}
}

func TestCancellationDuringCleanupSetupIsNotLost(t *testing.T) {
	m, _, root, _ := sealedFixture(t, 3)
	activateCleanup(t, m)
	p, err := m.PreviewRetention(context.Background(), 30)
	if err != nil {
		t.Fatal(err)
	}
	b := &cleanupFault{fileBackend: fileBackend{root}}
	m.factory = func(config) backend { return b }
	// Pause configuration loading after Execute acquires op but before it can
	// register its I/O cancellation context. No timing-dependent remote fake.
	m.store.mu.Lock()
	done := make(chan error, 1)
	go func() {
		_, err := m.ExecuteRetention(context.Background(), CleanupRequest{Token: p.Token, Acknowledged: true})
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for m.op.TryLock() {
		m.op.Unlock()
		if time.Now().After(deadline) {
			m.store.mu.Unlock()
			t.Fatal("cleanup did not enter")
		}
		time.Sleep(time.Millisecond)
	}
	m.CancelCleanup()
	m.store.mu.Unlock()
	select {
	case err = <-done:
		if err == nil {
			t.Fatal("setup cancellation was lost")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled setup did not return")
	}
	if len(b.deleted) != 0 {
		t.Fatal("deleted after stop requested during setup")
	}
}
