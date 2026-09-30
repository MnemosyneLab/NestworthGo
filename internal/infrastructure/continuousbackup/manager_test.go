package continuousbackup

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benbjohnson/litestream"
	"github.com/superfly/ltx"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func fixture(t *testing.T) (*Manager, *sqlite.DB, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "business.db")
	db, err := sqlite.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(p, db)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	m.factory = func(config) backend { return fileBackend{root: root} }
	m.interval = time.Hour
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := m.Close(ctx); err != nil {
			t.Error(err)
		}
		db.Close()
	})
	return m, db, root
}
func testUpdate(enabled bool) Update {
	return Update{Enabled: enabled, AccountID: strings.Repeat("a", 32), Bucket: "test-bucket", AccessKeyID: "local-test-access", SecretAccessKey: "local-test-secret"}
}
func waitState(t *testing.T, m *Manager, state string) View {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		v, err := m.View()
		if err != nil {
			t.Fatal(err)
		}
		if v.State == state {
			return v
		}
		time.Sleep(time.Millisecond * 5)
	}
	v, _ := m.View()
	t.Fatalf("state=%s, want %s (%s)", v.State, state, v.ErrorSummary)
	return v
}
func TestDefaultOffAtomicConfigurationAndSecretBoundary(t *testing.T) {
	m, db, _ := fixture(t)
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	v, _ := m.View()
	if v.Enabled || v.State != "disabled" || v.LastSuccessfulBackup != "" {
		t.Fatalf("default %+v", v)
	}
	if err := m.Configure(context.Background(), testUpdate(false)); err != nil {
		t.Fatal(err)
	}
	old, _, _ := m.store.load()
	bad := testUpdate(false)
	bad.SecretAccessKey = ""
	if !errors.Is(m.Configure(context.Background(), bad), ErrConfiguration) {
		t.Fatal("accepted partial pair")
	}
	got, _, _ := m.store.load()
	if got != old {
		t.Fatal("failed pair changed credentials")
	}
	keep := Update{AccountID: old.AccountID, Bucket: old.Bucket}
	if err := m.Configure(context.Background(), keep); err != nil {
		t.Fatal(err)
	}
	v, _ = m.View()
	data, _ := json.Marshal(v)
	if strings.Contains(string(data), old.AccessKeyID) || strings.Contains(string(data), old.SecretAccessKey) {
		t.Fatal("secret in outgoing DTO")
	}
	if err := m.TestConnection(context.Background()); err != nil {
		t.Fatal(err)
	}
	v, _ = m.View()
	if v.LastSuccessfulBackup != "" {
		t.Fatal("connection test claimed backup success")
	}
	var tables int
	if err := db.SQL.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name='backup_local'").Scan(&tables); err != nil || tables != 0 {
		t.Fatal("configuration in business DB")
	}
	info, err := os.Stat(filepath.Join(filepath.Dir(m.path), "backup_config.db"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("config not private")
	}
}
func TestFileBackupRestoreDisableAndIsolatedRestart(t *testing.T) {
	m, db, root := fixture(t)
	if _, err := db.SQL.Exec(`INSERT INTO app_configuration(key,value) VALUES('test-provider-secret','test-value')`); err != nil {
		t.Fatal(err)
	}
	if err := m.Configure(context.Background(), testUpdate(true)); err != nil {
		t.Fatal(err)
	}
	first := waitState(t, m, "recent-backup-confirmed")
	if first.LastAttempt == "" || first.LastSuccessfulBackup == "" || !validStream(first.StreamID) {
		t.Fatalf("no evidence %+v", first)
	}
	if _, err := db.SQL.Exec(`UPDATE app_configuration SET value='new-value' WHERE key='test-provider-secret'`); err != nil {
		t.Fatal(err)
	}
	if err := m.BackupNow(context.Background()); err != nil {
		t.Fatal(err)
	}
	points, err := m.RecoveryPoints(context.Background())
	if err != nil || len(points) < 2 {
		t.Fatalf("points %d %v", len(points), err)
	}
	staged, err := m.Stage(context.Background(), points[0])
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(filepath.Dir(staged))
	verified, err := sqlite.OpenReadOnlyForVerify(staged)
	if err != nil {
		t.Fatal(err)
	}
	var value string
	err = verified.SQL.QueryRow(`SELECT value FROM app_configuration WHERE key='test-provider-secret'`).Scan(&value)
	verified.Close()
	if err != nil || value != "new-value" {
		t.Fatalf("restore %q %v", value, err)
	}
	v, _ := m.View()
	success := v.LastSuccessfulBackup
	disabled := testUpdate(false)
	disabled.AccessKeyID = ""
	disabled.SecretAccessKey = ""
	if err := m.Configure(context.Background(), disabled); err != nil {
		t.Fatal(err)
	}
	v, _ = m.View()
	if v.State != "disabled" || v.LastSuccessfulBackup != success {
		t.Fatal("disabled lost historical evidence")
	}
	var busy, n, cp int
	if err := db.SQL.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &n, &cp); err != nil || busy != 0 {
		t.Fatalf("retained read lock %d %v", busy, err)
	}
	if err := m.Configure(context.Background(), testUpdate(true)); err != nil {
		t.Fatal(err)
	}
	next := waitState(t, m, "recent-backup-confirmed")
	if first.StreamID == next.StreamID || first.BackupID != next.BackupID {
		t.Fatal("stream identity unsafe")
	}
	streams, err := (fileBackend{root: root}).streams(context.Background())
	if err != nil || len(streams) != 2 {
		t.Fatalf("old history removed: %v %v", streams, err)
	}
	if err := m.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(m.path, db)
	if err != nil {
		t.Fatal(err)
	}
	restarted.factory = m.factory
	restarted.interval = time.Hour
	defer restarted.Close(context.Background())
	if err := restarted.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	third := waitState(t, restarted, "recent-backup-confirmed")
	if third.StreamID == next.StreamID || third.BackupID != next.BackupID {
		t.Fatal("restart reused active stream")
	}
}

type faultBackend struct {
	backend
	fail    *atomic.Bool
	hang    *atomic.Bool
	entered chan struct{}
}

func (b faultBackend) client(stream string) litestream.ReplicaClient {
	return &faultClient{ReplicaClient: b.backend.client(stream), fail: b.fail, hang: b.hang, entered: b.entered}
}

type faultClient struct {
	litestream.ReplicaClient
	fail, hang *atomic.Bool
	entered    chan struct{}
}

func (c *faultClient) WriteLTXFile(ctx context.Context, level int, min, max ltx.TXID, r io.Reader) (*ltx.FileInfo, error) {
	if c.hang.Load() {
		select {
		case c.entered <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if c.fail.Load() {
		return nil, errors.New("provider response containing a simulated credential")
	}
	return c.ReplicaClient.WriteLTXFile(ctx, level, min, max, r)
}
func TestOfflineRetryAndCanceledHangingBackendSafeClose(t *testing.T) {
	m, db, root := fixture(t)
	fail, hang := &atomic.Bool{}, &atomic.Bool{}
	fail.Store(true)
	entered := make(chan struct{}, 1)
	m.factory = func(config) backend {
		return faultBackend{backend: fileBackend{root: root}, fail: fail, hang: hang, entered: entered}
	}
	if err := m.Configure(context.Background(), testUpdate(true)); err != nil {
		t.Fatal(err)
	}
	v := waitState(t, m, "retrying")
	if v.LastSuccessfulBackup != "" || strings.Contains(v.ErrorSummary, "simulated credential") {
		t.Fatal("false success or leaked backend response")
	}
	if _, err := db.SQL.Exec(`INSERT INTO app_configuration(key,value) VALUES('offline-local-write','{}')`); err != nil {
		t.Fatal(err)
	}
	fail.Store(false)
	if err := m.BackupNow(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, "recent-backup-confirmed")
	if _, err := db.SQL.Exec(`UPDATE app_configuration SET value='changed' WHERE key='offline-local-write'`); err != nil {
		t.Fatal(err)
	}
	hang.Store(true)
	backupDone := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { backupDone <- m.BackupNow(ctx) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("backend not entered")
	}
	start := time.Now()
	cancel()
	if err := <-backupDone; err == nil {
		t.Fatal("canceled call claimed success")
	}
	if err := m.Configure(context.Background(), testUpdate(false)); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancel/stop exceeded bound")
	}
	if _, err := db.SQL.Exec(`UPDATE app_configuration SET value='still-valid' WHERE key='offline-local-write'`); err != nil {
		t.Fatal(err)
	}
}
func TestRestorePausePersistsDisabledAndNotFoundIsFailure(t *testing.T) {
	m, db, _ := fixture(t)
	if err := m.Configure(context.Background(), testUpdate(true)); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, "recent-backup-confirmed")
	if _, err := m.Stage(context.Background(), RecoveryPoint{StreamID: "../../escape", TXID: "0000000000000001"}); err == nil {
		t.Fatal("unsafe stream accepted")
	}
	v, _ := m.View()
	if _, err := m.Stage(context.Background(), RecoveryPoint{StreamID: v.BackupID + "/00000000-0000-0000-0000-000000000001", TXID: "0000000000000001"}); err == nil {
		t.Fatal("missing remote restored")
	}
	if err := m.PauseForRestore(context.Background()); err != nil {
		t.Fatal(err)
	}
	v, _ = m.View()
	if v.Enabled || v.RestoreState != "installing" {
		t.Fatal("restore did not pause")
	}
	if !errors.Is(m.Start(context.Background()), ErrDisabled) {
		t.Fatal("resumed during restore")
	}
	if err := db.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// This gate is reached by pinned Litestream init after acquireReadLock and
// before its descriptor-closing error defer. Application transactions must
// never overlap it, including failed initialization and subsequent retries.
type initFaultBackend struct {
	backend
	entered, release chan struct{}
	fail             *atomic.Bool
}

func (b initFaultBackend) client(stream string) litestream.ReplicaClient {
	return &initFaultClient{ReplicaClient: b.backend.client(stream), gate: b}
}

type initFaultClient struct {
	litestream.ReplicaClient
	gate initFaultBackend
}

func (c *initFaultClient) LTXFiles(ctx context.Context, level int, seek ltx.TXID, metadata bool) (ltx.FileIterator, error) {
	if c.gate.fail.Load() {
		select {
		case c.gate.entered <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-c.gate.release:
		}
		return nil, errors.New("offline or permission denied")
	}
	return c.ReplicaClient.LTXFiles(ctx, level, seek, metadata)
}
func TestInitializationFailureDrainsApplicationTransactionsAndRetries(t *testing.T) {
	m, db, root := fixture(t)
	fail := &atomic.Bool{}
	fail.Store(true)
	entered, release := make(chan struct{}, 1), make(chan struct{}, 1)
	m.factory = func(config) backend { return initFaultBackend{fileBackend{root}, entered, release, fail} }
	if err := m.Configure(context.Background(), testUpdate(true)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("init remote listing not reached")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if tx, err := db.SQL.BeginTx(ctx, nil); err == nil {
		tx.Rollback()
		t.Fatal("app transaction overlapped descriptor-closing initialization")
	}
	release <- struct{}{}
	v := waitState(t, m, "retrying")
	if v.LastSuccessfulBackup != "" {
		t.Fatal("failed init claimed success")
	}
	tx, err := db.SQL.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO app_configuration(key,value) VALUES('init-failure-write','committed')`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	fail.Store(false)
	if err := m.BackupNow(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, "recent-backup-confirmed")
	if err := db.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func TestFailedStopResumesOldWorkerWithoutFalseConfirmation(t *testing.T) {
	for _, operation := range []string{"configure", "restore-pause"} {
		t.Run(operation, func(t *testing.T) {
			m, db, _ := fixture(t)
			if err := m.Configure(context.Background(), testUpdate(true)); err != nil {
				t.Fatal(err)
			}
			initial := waitState(t, m, "recent-backup-confirmed")
			conn, err := db.SQL.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if operation == "configure" {
				err = m.Configure(ctx, testUpdate(false))
			} else {
				err = m.PauseForRestore(ctx)
			}
			if err == nil {
				conn.Close()
				t.Fatal("occupied pool did not fail drain")
			}
			v, _ := m.View()
			if !v.Enabled || v.State == "recent-backup-confirmed" || v.LastSuccessfulBackup != initial.LastSuccessfulBackup {
				t.Fatalf("false status after failed drain: %+v", v)
			}
			// Use the borrowed connection so the resumed worker cannot take the slot
			// before this write. Its next capture must include this committed change.
			if _, err := conn.ExecContext(context.Background(), `INSERT INTO app_configuration(key,value) VALUES('after-failed-stop','new')`); err != nil {
				t.Fatal(err)
			}
			conn.Close()
			m.op.Lock()
			resumed := m.workerDone != nil
			m.op.Unlock()
			if !resumed {
				t.Fatal("failed stop left enabled backup without worker")
			}
			// Sync may have captured before this transaction committed; a
			// confirmation only promises its captured LTX, not every later write.
			if err := m.BackupNow(context.Background()); err != nil {
				t.Fatal(err)
			}
			next := waitState(t, m, "recent-backup-confirmed")
			if next.StreamID != initial.StreamID || next.LastSuccessfulBackup == initial.LastSuccessfulBackup {
				t.Fatal("old stream did not resume")
			}
			points, err := m.RecoveryPoints(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			staged, err := m.Stage(context.Background(), points[0])
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(filepath.Dir(staged))
			candidate, err := sqlite.OpenReadOnlyForVerify(staged)
			if err != nil {
				t.Fatal(err)
			}
			defer candidate.Close()
			var value string
			if err := candidate.SQL.QueryRow(`SELECT value FROM app_configuration WHERE key='after-failed-stop'`).Scan(&value); err != nil || value != "new" {
				t.Fatalf("resumed stream missed write: %s %v", value, err)
			}
		})
	}
}
