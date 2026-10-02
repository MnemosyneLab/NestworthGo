package continuousbackup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"
)

var ErrRetention = errors.New("cleanup stopped safely; refresh the preview and check storage permissions")
var ErrStalePreview = errors.New("cleanup preview has changed or expired; preview again")

type RetentionStatus struct {
	Running       bool   `json:"running"`
	Enabled       bool   `json:"enabled"`
	Days          int    `json:"days"`
	ScannedAt     string `json:"scannedAt"`
	ScannedBytes  int64  `json:"scannedBytes"`
	EligibleBytes int64  `json:"eligibleBytes"`
	LastAttempt   string `json:"lastAttempt"`
	LastCleanup   string `json:"lastCleanup"`
	Result        string `json:"result"`
	ErrorSummary  string `json:"errorSummary"`
}
type RetentionUpdate struct {
	Enabled      bool `json:"enabled"`
	Days         int  `json:"days"`
	Acknowledged bool `json:"acknowledged"`
}
type CleanupRequest struct {
	Token        string `json:"token"`
	Acknowledged bool   `json:"acknowledged"`
}

func (s *configStore) retention(c config) (RetentionStatus, error) {
	var data string
	var r RetentionStatus
	err := s.db.QueryRow(`SELECT record FROM backup_retention WHERE target=?`, targetKey(c)).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return RetentionStatus{Days: 30}, nil
	}
	if err != nil || json.Unmarshal([]byte(data), &r) != nil {
		return r, ErrRetention
	}
	return r, nil
}
func (s *configStore) writeRetention(c config, r RetentionStatus) error {
	r.Running = false // Running is process state, never a persisted success claim.
	data, _ := json.Marshal(r)
	_, err := s.db.Exec(`INSERT INTO backup_retention(target,record) VALUES(?,?) ON CONFLICT(target) DO UPDATE SET record=excluded.record`, targetKey(c), string(data))
	if err != nil {
		return ErrRetention
	}
	return nil
}
func (m *Manager) RetentionStatus() (RetentionStatus, error) {
	// Reads must remain available while op serializes remote cleanup/restore.
	// The control DB is independent of remote I/O; Close produces a safe error.
	c, _, err := m.store.load()
	if err != nil {
		return RetentionStatus{}, err
	}
	r, err := m.store.retention(c)
	if err != nil {
		return r, err
	}
	m.cleanupControl.Lock()
	r.Running = m.cleanupCancel != nil && m.cleanupTarget == targetKey(c)
	m.cleanupControl.Unlock()
	return r, nil
}
func (m *Manager) ConfigureRetention(u RetentionUpdate) (RetentionStatus, error) {
	m.CancelCleanup()
	m.op.Lock()
	defer m.op.Unlock()
	if m.closed || m.restorePaused {
		return RetentionStatus{}, ErrDisabled
	}
	if u.Days != 30 && u.Days != 90 || u.Enabled && !u.Acknowledged {
		return RetentionStatus{}, ErrConfiguration
	}
	c, _, err := m.store.load()
	if err != nil {
		return RetentionStatus{}, err
	}
	if u.Enabled && (c.AccountID == "" || c.Bucket == "" || c.AccessKeyID == "" || c.SecretAccessKey == "") {
		return RetentionStatus{}, ErrConfiguration
	}
	r, err := m.store.retention(c)
	if err != nil {
		return r, err
	}
	r.Enabled = u.Enabled
	r.Days = u.Days
	if err = m.store.writeRetention(c, r); err != nil {
		return r, err
	}
	m.retentionPreview = nil
	m.startCleanupSchedulerLocked()
	return r, nil
}

// Cancellation uses its own small lock so a waiting configuration change or
// app shutdown can interrupt remote work without waiting for the operation lock.
func (m *Manager) CancelCleanup() {
	m.cleanupControl.Lock()
	defer m.cleanupControl.Unlock()
	m.cleanupGeneration++
	if m.cleanupCancel != nil {
		m.cleanupCancel()
	}
}
func (m *Manager) cleanupEpoch() uint64 {
	m.cleanupControl.Lock()
	defer m.cleanupControl.Unlock()
	return m.cleanupGeneration
}
func (m *Manager) cleanupContext(ctx context.Context, generation uint64, target string) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(ctx, cleanupTimeout)
	m.cleanupControl.Lock()
	m.cleanupCancel = cancel
	m.cleanupTarget = target
	if generation != m.cleanupGeneration {
		cancel()
	}
	m.cleanupControl.Unlock()
	stopShutdown := context.AfterFunc(m.cleanupBackground, cancel)
	if m.cleanupBackground.Err() != nil {
		cancel()
	}
	return ctx, func() {
		stopShutdown()
		cancel()
		m.cleanupControl.Lock()
		m.cleanupCancel = nil
		m.cleanupTarget = ""
		m.cleanupControl.Unlock()
	}
}
func (m *Manager) ExecuteRetention(ctx context.Context, u CleanupRequest) (RetentionStatus, error) {
	generation := m.cleanupEpoch()
	m.op.Lock()
	defer m.op.Unlock()
	if m.closed || m.restorePaused {
		return RetentionStatus{}, ErrDisabled
	}
	if !u.Acknowledged {
		return RetentionStatus{}, ErrConfiguration
	}
	c, _, err := m.store.load()
	if err != nil {
		return RetentionStatus{}, err
	}
	r, err := m.store.retention(c)
	if err != nil {
		return r, err
	}
	old := m.retentionPreview
	m.retentionPreview = nil // single use, including failures
	if old == nil || u.Token == "" || old.View.Token != u.Token || old.Config != c || old.Days != r.Days || !r.Enabled {
		return r, ErrStalePreview
	}
	scanned, err := time.Parse(time.RFC3339Nano, old.View.ScannedAt)
	if err != nil || time.Since(scanned) < 0 || time.Since(scanned) > previewLifetime {
		return r, ErrStalePreview
	}
	runCtx, stop := m.cleanupContext(ctx, generation, targetKey(c))
	defer stop()
	r.LastAttempt = time.Now().UTC().Format(time.RFC3339Nano)
	r.ErrorSummary = ""
	r.Result = "running"
	if err = m.store.writeRetention(c, r); err != nil {
		return r, err
	}
	// Re-run inventory, identity, survivor restores and pins with the same lock
	// held through deletion. Stale UI state is never authority to delete.
	fresh, err := m.scanRetentionLocked(runCtx, c, r.Days)
	if err == nil && (fresh.Fingerprint != old.Fingerprint || !sameCandidates(fresh.View, old.View)) {
		err = ErrStalePreview
	}
	if err == nil {
		err = m.executePlanLocked(runCtx, c, fresh)
	}
	return m.finishCleanup(c, r, fresh.View, err)
}
func sameCandidates(a, b RetentionPreview) bool {
	candidates := func(p RetentionPreview) []string {
		r := []string{}
		for _, i := range p.Items {
			if i.Eligible {
				r = append(r, i.StreamID)
			}
		}
		return r
	}
	return a.Ready == b.Ready && reflect.DeepEqual(candidates(a), candidates(b))
}
func (m *Manager) finishCleanup(c config, r RetentionStatus, p RetentionPreview, err error) (RetentionStatus, error) {
	if p.ScannedAt != "" {
		r.ScannedAt = p.ScannedAt
		r.ScannedBytes = p.ScannedBytes
		r.EligibleBytes = p.EligibleBytes
	}
	r.LastCleanup = time.Now().UTC().Format(time.RFC3339Nano)
	r.Result = "completed"
	if !p.Ready {
		r.Result = "skipped"
	}
	if err != nil {
		r.Result = "stopped"
		r.ErrorSummary = ErrRetention.Error()
		if errors.Is(err, ErrStalePreview) {
			r.ErrorSummary = ErrStalePreview.Error()
		} else {
			err = ErrRetention
		}
	}
	if saveErr := m.store.writeRetention(c, r); saveErr != nil {
		return r, saveErr
	}
	return r, err
}
func (m *Manager) executePlanLocked(ctx context.Context, c config, p retentionPlan) error {
	if !p.View.Ready {
		return nil
	}
	storage, ok := m.factory(c).(retentionBackend)
	if !ok {
		return ErrRetention
	}
	// Recheck the complete inventory after verification and before any mutation.
	all, err := storage.inventory(ctx, c.BackupID)
	if err != nil {
		return ErrRetention
	}
	objects, err := groupInventory(c.BackupID, all)
	if err != nil {
		return ErrRetention
	}
	current := ""
	if m.replicaDB != nil {
		current = m.identity.StreamID
	}
	if inventoryFingerprint(current, objects, p.Records) != p.Fingerprint {
		return ErrStalePreview
	}
	for _, item := range p.View.Items {
		if !item.Eligible {
			continue
		}
		if ctx.Err() != nil {
			return ErrRetention
		}
		r := p.Records[item.StreamID]
		if item.StreamID == current || m.previewPins[item.StreamID] || r.Identity.StreamID != item.StreamID || !strings.HasPrefix(item.StreamID, c.BackupID+"/") {
			return ErrRetention
		}
		if r.State == "sealed" {
			r.State = "deleting"
			r.Objects = objects[item.StreamID]
			r.DeletedObjects = 0
			// This commit must be durable before the first non-atomic remote delete.
			if err = m.store.writeStream(c, r); err != nil {
				return ErrRetention
			}
		} else if r.State != "deleting" {
			return ErrRetention
		}
		if err = m.deleteStreamLocked(ctx, c, storage, r, objects[item.StreamID]); err != nil {
			return err
		}
	}
	return nil
}

// Bounded scalar checkpoints avoid rewriting the immutable manifest per object.
// The final partial batch is flushed on cancellation/error; a process crash can
// lose that batch, which the next complete remote inventory reconciles safely.
const deletionCheckpointBatch = 128
const deletionCheckpointInterval = time.Second

func (m *Manager) deleteStreamLocked(ctx context.Context, c config, storage retentionBackend, r streamRecord, present []storedObject) (result error) {
	expected := make(map[string]storedObject, len(r.Objects))
	var metadata storedObject
	for _, o := range r.Objects {
		stream, err := objectStream(c.BackupID, o.Key)
		if err != nil || stream != r.Identity.StreamID || expected[o.Key].Key != "" {
			return ErrRetention
		}
		expected[o.Key] = o
		if strings.HasSuffix(o.Key, "/"+identityFile) {
			metadata = o
		}
	}
	if len(expected) == 0 || metadata.Key == "" {
		return ErrRetention
	}
	remaining := make(map[string]bool, len(present))
	ordered := make([]storedObject, 0, len(present))
	for _, o := range present {
		if expected[o.Key] != o || remaining[o.Key] {
			return ErrRetention
		}
		remaining[o.Key] = true
		if o.Key != metadata.Key {
			ordered = append(ordered, o)
		}
	}
	if remaining[metadata.Key] {
		ordered = append(ordered, metadata)
	}
	// Missing objects are already complete, including lost-response deletes. Do
	// not walk them again or write one checkpoint for each on every retry.
	r.DeletedObjects = len(expected) - len(present)
	pending := 0
	checkpointFailed := false
	lastCheckpoint := time.Now()
	checkpoint := func() error {
		if pending == 0 || checkpointFailed {
			return nil
		}
		if err := m.store.checkpointStream(c, r.Identity.StreamID, r.DeletedObjects); err != nil {
			checkpointFailed = true
			return ErrRetention
		}
		pending = 0
		lastCheckpoint = time.Now()
		return nil
	}
	defer func() {
		if err := checkpoint(); err != nil {
			result = err
		}
	}()
	for _, o := range ordered {
		if ctx.Err() != nil {
			return ErrRetention
		}
		if o.Key == metadata.Key {
			all, err := storage.inventory(ctx, c.BackupID)
			if err != nil {
				return ErrRetention
			}
			groups, err := groupInventory(c.BackupID, all)
			if err != nil {
				return ErrRetention
			}
			items := groups[r.Identity.StreamID]
			if len(items) != 1 || items[0] != metadata {
				return ErrRetention
			}
		}
		if err := storage.deleteObject(ctx, o); err != nil {
			return ErrRetention
		}
		r.DeletedObjects++
		pending++
		if pending >= deletionCheckpointBatch || time.Since(lastCheckpoint) >= deletionCheckpointInterval {
			if err := checkpoint(); err != nil {
				return err
			}
		}
	}
	if err := checkpoint(); err != nil {
		return err
	}
	// Confirm absence before the small final tombstone replaces the manifest.
	all, err := storage.inventory(ctx, c.BackupID)
	if err != nil {
		return ErrRetention
	}
	groups, err := groupInventory(c.BackupID, all)
	if err != nil || len(groups[r.Identity.StreamID]) != 0 {
		return ErrRetention
	}
	r.State = "deleted"
	r.Objects = nil
	return m.store.writeStream(c, r)
}
func (m *Manager) startCleanupSchedulerLocked() {
	if m.cleanupStarted || m.closed {
		return
	}
	m.cleanupStarted = true
	go func() {
		defer close(m.cleanupDone)
		timer := time.NewTimer(time.Minute)
		defer timer.Stop()
		for {
			select {
			case <-m.cleanupBackground.Done():
				return
			case <-timer.C:
				m.backgroundCleanup()
				timer.Reset(time.Hour)
			}
		}
	}()
}
func (m *Manager) backgroundCleanup() {
	generation := m.cleanupEpoch()
	m.op.Lock()
	defer m.op.Unlock()
	// Do not race a user reviewing an unexpired cleanup preview. The user
	// either consumes its token manually or the next scheduled scan waits.
	if m.retentionPreview != nil {
		scanned, err := time.Parse(time.RFC3339Nano, m.retentionPreview.View.ScannedAt)
		if err != nil || time.Since(scanned) < previewLifetime {
			return
		}
	}
	if m.closed || m.restorePaused || m.cleanupBackground.Err() != nil {
		return
	}
	c, _, err := m.store.load()
	if err != nil {
		return
	}
	r, err := m.store.retention(c)
	if err != nil || !r.Enabled || c.AccountID == "" || c.Bucket == "" || c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return
	}
	if r.LastAttempt != "" {
		last, err := time.Parse(time.RFC3339Nano, r.LastAttempt)
		if err != nil || time.Since(last) < 24*time.Hour {
			return
		}
	}
	// Persist the attempt before I/O: crashes cannot cause startup retry loops.
	r.LastAttempt = time.Now().UTC().Format(time.RFC3339Nano)
	r.Result = "running"
	r.ErrorSummary = ""
	if m.store.writeRetention(c, r) != nil {
		return
	}
	ctx, stop := m.cleanupContext(m.cleanupBackground, generation, targetKey(c))
	defer stop()
	p, err := m.scanRetentionLocked(ctx, c, r.Days)
	if err == nil {
		err = m.executePlanLocked(ctx, c, p)
	}
	_, _ = m.finishCleanup(c, r, p.View, err)
	m.retentionPreview = nil
}
