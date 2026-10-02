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
	data, _ := json.Marshal(r)
	_, err := s.db.Exec(`INSERT INTO backup_retention(target,record) VALUES(?,?) ON CONFLICT(target) DO UPDATE SET record=excluded.record`, targetKey(c), string(data))
	if err != nil {
		return ErrRetention
	}
	return nil
}
func (m *Manager) RetentionStatus() (RetentionStatus, error) {
	m.op.Lock()
	defer m.op.Unlock()
	if m.closed {
		return RetentionStatus{}, ErrDisabled
	}
	c, _, err := m.store.load()
	if err != nil {
		return RetentionStatus{}, err
	}
	return m.store.retention(c)
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
	if m.cleanupCancel != nil {
		m.cleanupCancel()
	}
}
func (m *Manager) cleanupContext(ctx context.Context) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(ctx, cleanupTimeout)
	m.cleanupControl.Lock()
	m.cleanupCancel = cancel
	m.cleanupControl.Unlock()
	return ctx, func() { cancel(); m.cleanupControl.Lock(); m.cleanupCancel = nil; m.cleanupControl.Unlock() }
}
func (m *Manager) ExecuteRetention(ctx context.Context, u CleanupRequest) (RetentionStatus, error) {
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
	runCtx, stop := m.cleanupContext(ctx)
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
func (m *Manager) deleteStreamLocked(ctx context.Context, c config, storage retentionBackend, r streamRecord, present []storedObject) error {
	expected := map[string]storedObject{}
	for _, o := range r.Objects {
		stream, err := objectStream(c.BackupID, o.Key)
		if err != nil || stream != r.Identity.StreamID || expected[o.Key].Key != "" {
			return ErrRetention
		}
		expected[o.Key] = o
	}
	if len(expected) == 0 {
		return ErrRetention
	}
	remaining := map[string]storedObject{}
	for _, o := range present {
		if expected[o.Key] != o {
			return ErrRetention
		}
		remaining[o.Key] = o
	}
	// Process L0 objects first, then stream.json. Missing objects are checkpoints
	// from a previous interrupted request, including a lost delete response.
	ordered := []storedObject{}
	var metadata storedObject
	for _, o := range r.Objects {
		if strings.HasSuffix(o.Key, "/"+identityFile) {
			metadata = o
		} else {
			ordered = append(ordered, o)
		}
	}
	if metadata.Key == "" {
		return ErrRetention
	}
	ordered = append(ordered, metadata)
	r.DeletedObjects = 0
	for _, o := range ordered {
		if ctx.Err() != nil {
			return ErrRetention
		}
		if _, exists := remaining[o.Key]; exists {
			// Before removing metadata, list the exact target again. New or changed
			// objects abort, preserving the identity for inspection and safe retries.
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
		}
		r.DeletedObjects++
		if err := m.store.writeStream(c, r); err != nil {
			return ErrRetention
		}
	}
	// Confirm absence before declaring completion; interrupted verification is
	// resumed from the durable deleting row and original object snapshot.
	all, err := storage.inventory(ctx, c.BackupID)
	if err != nil {
		return ErrRetention
	}
	groups, err := groupInventory(c.BackupID, all)
	if err != nil || len(groups[r.Identity.StreamID]) != 0 {
		return ErrRetention
	}
	r.State = "deleted"
	r.Objects = nil // the tombstone remains; the finished object manifest is no longer needed
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
	ctx, stop := m.cleanupContext(m.cleanupBackground)
	defer stop()
	p, err := m.scanRetentionLocked(ctx, c, r.Days)
	if err == nil {
		err = m.executePlanLocked(ctx, c, p)
	}
	_, _ = m.finishCleanup(c, r, p.View, err)
	m.retentionPreview = nil
}
