package continuousbackup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/benbjohnson/litestream"
	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// Manager serializes configuration changes and owns every replication call.
// Built-in monitors/retention are off: no library goroutine can outlive a
// canceled operation or close a DB after a caller's timeout has returned.
type Manager struct {
	op                sync.Mutex
	store             *configStore
	database          *sqlite.DB
	path              string
	factory           func(config) backend
	workerCancel      context.CancelFunc
	workerDone        chan struct{}
	replicaDB         *litestream.DB
	replicaStore      *litestream.Store
	initialized       bool
	lastSyncSucceeded bool
	identity          streamIdentity
	replicaBackend    backend
	replicaConfig     config
	statusMu          sync.Mutex
	status            status
	interval          time.Duration
	closed            bool
	restorePaused     bool
}

func New(path string, database *sqlite.DB) (*Manager, error) {
	s, err := openConfig(filepath.Join(filepath.Dir(path), "backup_config.db"))
	if err != nil {
		return nil, err
	}
	_, st, err := s.load()
	if err != nil {
		s.db.Close()
		return nil, err
	}
	m := &Manager{store: s, database: database, path: path, status: st, interval: 5 * time.Second, factory: func(c config) backend { return r2Backend{c: c} }}
	// Persisted status is historical evidence, never a live success indication.
	m.status.State = "disabled"
	m.status.ErrorSummary = ""
	m.status.StreamID = ""
	m.status.RestoreState = ""
	if err := s.writeStatus(m.status); err != nil {
		s.db.Close()
		return nil, err
	}
	return m, nil
}
func (m *Manager) View() (View, error) {
	c, st, err := m.store.load()
	if err != nil {
		return View{}, err
	}
	return View{Enabled: c.Enabled, AccountID: c.AccountID, Bucket: c.Bucket, CredentialsConfigured: c.AccessKeyID != "" && c.SecretAccessKey != "", BackupID: c.BackupID, State: st.State, StreamID: st.StreamID, LastAttempt: st.LastAttempt, LastSuccessfulBackup: st.LastSuccessfulBackup, ErrorSummary: st.ErrorSummary, RestoreState: st.RestoreState}, nil
}
func (m *Manager) setStatus(fn func(*status)) error {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	fn(&m.status)
	return m.store.writeStatus(m.status)
}
func (m *Manager) Start(ctx context.Context) error {
	m.op.Lock()
	defer m.op.Unlock()
	if m.closed || m.restorePaused {
		return ErrDisabled
	}
	c, _, err := m.store.load()
	if err != nil {
		return err
	}
	if !c.Enabled {
		return nil
	}
	return m.startLocked(ctx, c)
}
func (m *Manager) startLocked(ctx context.Context, c config) error {
	if m.workerDone != nil {
		return nil
	}
	if m.database == nil || m.database.SQL == nil {
		m.setStatus(func(s *status) {
			s.State = "error"
			s.ErrorSummary = "The local database is unavailable. Restore explicitly from a backup."
		})
		return ErrUnavailable
	}
	stream := c.BackupID + "/" + uuid.NewString()
	d := litestream.NewDB(m.path)
	d.SetMetaPath(filepath.Join(filepath.Dir(m.path), ".nestworth-replication", stringsForPath(stream)))
	d.MonitorInterval = 0
	m.replicaBackend = m.factory(c)
	m.identity = newIdentity(stream)
	m.replicaConfig = c
	if err := m.store.createStream(c, m.identity); err != nil {
		return err
	}
	client := m.replicaBackend.client(stream)
	client.SetLogger(quietLogger)
	d.Replica = litestream.NewReplicaWithClient(d, client)
	d.Replica.MonitorEnabled = false
	store := litestream.NewStore([]*litestream.DB{d}, litestream.CompactionLevels{{Level: 0}, {Level: 1, Interval: time.Hour}})
	store.Logger = quietLogger
	d.SetLogger(quietLogger)
	store.CompactionMonitorEnabled = false
	store.L0Retention = 0
	store.L0RetentionCheckInterval = 0
	store.SetRetentionEnabled(false)
	store.SetShutdownSyncTimeout(0)
	openCtx, cancel := contextTimeout(ctx)
	defer cancel()
	if err := store.Open(openCtx); err != nil {
		m.setStatus(func(s *status) { s.State = "error"; s.ErrorSummary = ErrUnavailable.Error() })
		return ErrUnavailable
	}
	m.replicaDB = d
	m.replicaStore = store
	m.setStatus(func(s *status) { s.State = "preparing"; s.StreamID = stream; s.ErrorSummary = "" })
	m.initialized = false
	m.launchWorkerLocked(0)
	return nil
}

// launchWorkerLocked resumes an existing stream without recreating its handles.
func (m *Manager) launchWorkerLocked(delay time.Duration) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	m.workerCancel, m.workerDone = cancel, done
	go func() {
		defer close(done)
		timer := time.NewTimer(delay)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				m.sync(ctx)
				timer.Reset(m.interval)
			}
		}
	}()
}
func stringsForPath(stream string) string { return filepath.FromSlash(stream) }
func (m *Manager) sync(ctx context.Context) error {
	m.lastSyncSucceeded = false
	syncCtx, cancel := contextTimeout(ctx)
	defer cancel()
	m.setStatus(func(s *status) { s.State = "backing-up"; s.LastAttempt = time.Now().UTC().Format(time.RFC3339Nano) })
	// Lazy init can close SQLite descriptors on any failure, including remote
	// listing errors after acquireReadLock. Drain before every initialization
	// attempt; if Sync fails, close/reset the library handles before allowing
	// the application pool to reopen. The callback never borrows app SQL.
	var err error
	if !m.initialized {
		err = m.replicaBackend.ensureIdentity(syncCtx, m.identity)
		if err == nil {
			err = m.database.DrainForReplication(syncCtx, func() error {
				if err := m.replicaDB.Sync(syncCtx); err != nil {
					cleanupCtx, stop := context.WithCancel(context.Background())
					stop()
					_ = m.replicaDB.Close(cleanupCtx)
					if openErr := m.replicaDB.Open(); openErr != nil {
						return openErr
					}
					return err
				}
				m.initialized = true
				return nil
			})
		}
	}
	if err == nil {
		err = m.replicaDB.SyncAndWait(syncCtx)
	}
	if err == nil {
		st, checkErr := m.replicaDB.SyncStatus(syncCtx)
		if checkErr != nil || !st.InSync {
			err = ErrUnavailable
		}
	}
	if err != nil {
		m.setStatus(func(s *status) { s.State = "retrying"; s.ErrorSummary = ErrUnavailable.Error() })
		return ErrUnavailable
	}
	if err := m.setStatus(func(s *status) {
		s.State = "recent-backup-confirmed"
		s.LastSuccessfulBackup = time.Now().UTC().Format(time.RFC3339Nano)
		s.ErrorSummary = ""
	}); err != nil {
		return err
	}
	m.lastSyncSucceeded = true
	return nil
}
func (m *Manager) pauseWorkerLocked() {
	if m.workerDone != nil {
		m.workerCancel()
		<-m.workerDone
		m.workerDone = nil
		m.workerCancel = nil
	}
}
func (m *Manager) stopLocked(ctx context.Context, resume bool) error {
	m.pauseWorkerLocked()
	if m.replicaStore == nil {
		return nil
	}
	// Cleanup is synchronous. A timed-out goroutine is never left accessing
	// SQLite. All operations are stopped before reserving/closing the pool slot.
	closedHandles := false
	var confirmed time.Time
	var finalTXID string
	closeStore := func() error {
		closedHandles = true
		finalCtx, stopFinal := context.WithTimeout(ctx, 500*time.Millisecond)
		defer stopFinal()
		// Application queries are drained, and the replication worker is joined.
		// Confirm the final snapshot without borrowing the reserved app pool slot.
		if m.initialized && m.lastSyncSucceeded && ctx.Err() == nil {
			if err := m.replicaDB.SyncAndWait(finalCtx); err == nil {
				if st, err := m.replicaDB.SyncStatus(finalCtx); err == nil && st.InSync {
					confirmed, finalTXID = time.Now().UTC(), st.RemoteTXID.String()
				}
			}
		}
		closeCtx, cancel := context.WithCancel(finalCtx)
		if confirmed.IsZero() {
			cancel()
		}
		err := m.replicaStore.Close(closeCtx)
		cancel()
		if err != nil || ctx.Err() != nil {
			confirmed = time.Time{}
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		return err
	}
	if err := m.database.DrainForReplication(ctx, closeStore); err != nil {
		m.setStatus(func(s *status) { s.State = "retrying"; s.ErrorSummary = ErrUnavailable.Error() })
		if !closedHandles && resume {
			// A canceled drain has not touched the library handles. Keep the old
			// enabled configuration running after the failed change/restore pause.
			m.launchWorkerLocked(m.interval)
		} else if closedHandles {
			m.replicaDB, m.replicaStore = nil, nil
			m.initialized = false
			c, _, loadErr := m.store.load()
			if resume && loadErr == nil && c.Enabled && !m.closed && !m.restorePaused {
				retryCtx, cancel := contextTimeout(context.Background())
				_ = m.startLocked(retryCtx, c)
				cancel()
			}
		}
		return ErrUnavailable
	}
	// Closure errors (even cancellation) never create a seal. A failed ledger
	// write also leaves the stream protected; it must not break shutdown.
	if !confirmed.IsZero() {
		_ = m.store.seal(m.replicaConfig, m.identity, finalTXID, confirmed)
	}
	m.replicaDB = nil
	m.replicaStore = nil
	m.initialized = false
	return nil
}
func (m *Manager) Configure(ctx context.Context, u Update) error {
	m.op.Lock()
	defer m.op.Unlock()
	if m.closed || m.restorePaused {
		return ErrDisabled
	}
	old, oldStatus, err := m.store.load()
	if err != nil {
		return err
	}
	c, err := validateUpdate(old, u)
	if err != nil {
		return err
	}
	if c.Enabled && (m.database == nil || m.database.SQL == nil) {
		return ErrUnavailable
	}
	if err := m.stopLocked(ctx, true); err != nil {
		return err
	}
	m.statusMu.Lock()
	m.status.State = "disabled"
	m.status.StreamID = ""
	m.status.ErrorSummary = ""
	if old.AccountID != c.AccountID || old.Bucket != c.Bucket {
		m.status.LastAttempt = ""
		m.status.LastSuccessfulBackup = ""
	}
	err = m.store.save(c, m.status)
	m.statusMu.Unlock()
	if err != nil {
		m.statusMu.Lock()
		m.status = oldStatus
		m.statusMu.Unlock()
		if old.Enabled {
			retryCtx, cancel := contextTimeout(context.Background())
			_ = m.startLocked(retryCtx, old)
			cancel()
		}
		return err
	}
	if c.Enabled {
		if err := m.startLocked(ctx, c); err != nil {
			m.statusMu.Lock()
			m.status = oldStatus
			m.status.State, m.status.StreamID = "disabled", ""
			rollbackErr := m.store.save(old, m.status)
			m.statusMu.Unlock()
			if rollbackErr != nil {
				return ErrUnavailable
			}
			if old.Enabled {
				retryCtx, cancel := contextTimeout(context.Background())
				_ = m.startLocked(retryCtx, old)
				cancel()
			}
			return err
		}
	}
	return nil
}
func (m *Manager) BackupNow(ctx context.Context) error {
	m.op.Lock()
	defer m.op.Unlock()
	if m.closed || m.restorePaused || m.replicaDB == nil {
		return ErrDisabled
	}
	// Avoid overlap with the scheduled call; keep the same stream open.
	m.pauseWorkerLocked()
	err := m.sync(ctx)
	m.launchWorkerLocked(m.interval)
	return err
}
func (m *Manager) TestConnection(ctx context.Context) error {
	m.op.Lock()
	defer m.op.Unlock()
	c, _, err := m.store.load()
	if err != nil {
		return err
	}
	if c.AccountID == "" || c.Bucket == "" || c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return ErrConfiguration
	}
	checkCtx, cancel := contextTimeout(ctx)
	defer cancel()
	_, err = m.factory(c).streams(checkCtx)
	if err != nil {
		return ErrUnavailable
	}
	return nil
}
func (m *Manager) RecoveryPoints(ctx context.Context) ([]RecoveryPoint, error) {
	m.op.Lock()
	defer m.op.Unlock()
	c, _, err := m.store.load()
	if err != nil {
		return nil, err
	}
	if c.AccountID == "" || c.Bucket == "" || c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return nil, ErrConfiguration
	}
	listCtx, cancel := contextTimeout(ctx)
	defer cancel()
	return recoveryPoints(listCtx, m.factory(c))
}
func (m *Manager) Stage(ctx context.Context, p RecoveryPoint) (Candidate, error) {
	m.op.Lock()
	defer m.op.Unlock()
	c, _, err := m.store.load()
	if err != nil {
		return Candidate{}, err
	}
	if c.AccountID == "" || c.Bucket == "" || c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return Candidate{}, ErrConfiguration
	}
	m.setStatus(func(s *status) { s.RestoreState = "preparing" })
	dir, err := os.MkdirTemp(filepath.Dir(m.path), ".nestworth-cloud-restore-")
	if err != nil {
		return Candidate{}, ErrUnavailable
	}
	destination := filepath.Join(dir, "candidate.db")
	stageCtx, cancel := contextTimeout(ctx)
	defer cancel()
	info, identityErr := m.factory(c).identity(stageCtx, p.StreamID)
	if identityErr != nil {
		os.RemoveAll(dir)
		m.setStatus(func(s *status) { s.RestoreState = "error" })
		return Candidate{}, ErrUnavailable
	}
	if err := restorePoint(stageCtx, m.factory(c), p, destination); err != nil {
		os.RemoveAll(dir)
		m.setStatus(func(s *status) { s.RestoreState = "error" })
		return Candidate{}, err
	}
	verified, err := sqlite.OpenReadOnlyForVerify(destination)
	if err != nil {
		os.RemoveAll(dir)
		m.setStatus(func(s *status) { s.RestoreState = "error" })
		return Candidate{}, err
	}
	verified.Close()
	m.setStatus(func(s *status) { s.RestoreState = "preview" })
	return Candidate{Path: destination, AppVersion: info.AppVersion, AppBuild: info.AppBuild}, nil
}

// PauseForRestore is called inside the exclusive restore operation before
// checkpoint/close. Persist disabled before installation, including crash paths.
func (m *Manager) PauseForRestore(ctx context.Context) error {
	m.op.Lock()
	defer m.op.Unlock()
	if m.closed {
		return ErrDisabled
	}
	c, previous, err := m.store.load()
	if err != nil {
		return err
	}
	if err := m.stopLocked(ctx, true); err != nil {
		return err
	}
	old := c
	c.Enabled = false
	m.statusMu.Lock()
	m.status.State = "disabled"
	m.status.RestoreState = "installing"
	m.status.StreamID = ""
	err = m.store.save(c, m.status)
	if err != nil {
		m.status = previous
	}
	m.statusMu.Unlock()
	if err != nil {
		if old.Enabled {
			retryCtx, cancel := contextTimeout(context.Background())
			_ = m.startLocked(retryCtx, old)
			cancel()
		}
		return err
	}
	m.restorePaused = true
	return nil
}
func (m *Manager) Close(ctx context.Context) error {
	m.op.Lock()
	defer m.op.Unlock()
	if m.closed {
		return nil
	}
	m.pauseWorkerLocked()
	if m.replicaDB != nil && !m.restorePaused {
		_ = m.sync(ctx)
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := m.stopLocked(cleanupCtx, false)
	// If draining fails, retain handles/config and let process teardown close
	// them. Never close the DB behind an in-flight operation.
	if err != nil {
		return err
	}
	m.closed = true
	return m.store.db.Close()
}
