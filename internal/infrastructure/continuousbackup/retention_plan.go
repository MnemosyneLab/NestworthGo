package continuousbackup

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

const cleanupTimeout = 5 * time.Minute
const previewLifetime = 15 * time.Minute

type RetentionItem struct {
	StreamID string `json:"streamID"`
	SealedAt string `json:"sealedAt"`
	Bytes    int64  `json:"bytes"`
	Eligible bool   `json:"eligible"`
	Reason   string `json:"reason"`
}
type RetentionPreview struct {
	Token         string          `json:"token"`
	ScannedAt     string          `json:"scannedAt"`
	ScannedBytes  int64           `json:"scannedBytes"`
	EligibleBytes int64           `json:"eligibleBytes"`
	Items         []RetentionItem `json:"items"`
	Ready         bool            `json:"ready"`
}
type retentionPlan struct {
	View        RetentionPreview
	Config      config
	Days        int
	Objects     map[string][]storedObject
	Records     map[string]streamRecord
	Fingerprint string
}

// planRetention is pure: all network discovery and actual restore verification
// happen before it is called. No clock, status or StartedAt implies verification.
func planRetention(now time.Time, days int, current string, records map[string]streamRecord, objects map[string][]storedObject, verified, pinned map[string]bool) RetentionPreview {
	p := RetentionPreview{ScannedAt: now.UTC().Format(time.RFC3339Nano), Items: []RetentionItem{}}
	survivors := 0
	for stream, ok := range verified {
		if ok && records[stream].State == "sealed" && stream != current {
			survivors++
		}
	}
	p.Ready = survivors >= 2 && (days == 30 || days == 90)
	streams := map[string]bool{}
	for stream := range objects {
		streams[stream] = true
	}
	for stream := range records {
		streams[stream] = true
	}
	for stream := range streams {
		r, owned := records[stream]
		item := RetentionItem{StreamID: stream, SealedAt: r.SealedAt}
		for _, o := range objects[stream] {
			item.Bytes += o.Size
		}
		p.ScannedBytes += item.Bytes
		sealed, err := time.Parse(time.RFC3339Nano, r.SealedAt)
		switch {
		case stream == current:
			item.Reason = "current"
		case pinned[stream]:
			item.Reason = "restore-preview"
		case !owned:
			item.Reason = "unknown-owner"
		case r.State == "open":
			item.Reason = "unsealed"
		case r.State == "deleted":
			item.Reason = "deleted"
		case r.State != "sealed" && r.State != "deleting":
			item.Reason = "unsealed"
		case verified[stream]:
			item.Reason = "verified-survivor"
		case err != nil || sealed.After(now) || r.FinalTXID == "":
			item.Reason = "unreliable-seal"
		case len(objects[stream]) == 0 && r.State != "deleting":
			item.Reason = "missing-data"
		case !p.Ready:
			item.Reason = "insufficient-survivors"
		case sealed.After(now.AddDate(0, 0, -days)):
			item.Reason = "within-retention"
		default:
			item.Eligible = true
			item.Reason = "expired-sealed"
			if r.State == "deleting" {
				item.Reason = "resume-deletion"
			}
			p.EligibleBytes += item.Bytes
		}
		p.Items = append(p.Items, item)
	}
	sort.Slice(p.Items, func(i, j int) bool { return p.Items[i].StreamID < p.Items[j].StreamID })
	return p
}
func newestSealed(records map[string]streamRecord, current string) []string {
	result := []string{}
	for stream, r := range records {
		if r.State == "sealed" && stream != current {
			result = append(result, stream)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		a, _ := time.Parse(time.RFC3339Nano, records[result[i]].SealedAt)
		b, _ := time.Parse(time.RFC3339Nano, records[result[j]].SealedAt)
		if a.Equal(b) {
			return result[i] > result[j]
		}
		return a.After(b)
	})
	if len(result) > 2 {
		result = result[:2]
	}
	return result
}
func (m *Manager) verifySurvivor(ctx context.Context, b backend, r streamRecord) error {
	dir, err := os.MkdirTemp(filepath.Dir(m.path), ".nestworth-retention-verify-")
	if err != nil {
		return ErrUnavailable
	}
	defer os.RemoveAll(dir)
	destination := filepath.Join(dir, "candidate.db")
	if err = restorePoint(ctx, b, RecoveryPoint{StreamID: r.Identity.StreamID, TXID: r.FinalTXID}, destination); err != nil {
		return ErrUnavailable
	}
	db, err := sqlite.OpenReadOnlyForVerifyContext(ctx, destination)
	if err != nil {
		return ErrUnavailable
	}
	if err = db.Close(); err != nil {
		return ErrUnavailable
	}
	return nil
}
func inventoryFingerprint(current string, objects map[string][]storedObject, records map[string]streamRecord) string {
	stable := map[string][]storedObject{}
	for stream, items := range objects {
		if stream != current {
			stable[stream] = items
		}
	}
	data, _ := json.Marshal(struct {
		Objects map[string][]storedObject
		Records map[string]streamRecord
	}{stable, records})
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
func (m *Manager) scanRetentionLocked(ctx context.Context, c config, days int) (retentionPlan, error) {
	b := m.factory(c)
	storage, ok := b.(retentionBackend)
	if !ok {
		return retentionPlan{}, ErrUnavailable
	}
	all, err := storage.inventory(ctx, c.BackupID)
	if err != nil {
		return retentionPlan{}, ErrUnavailable
	}
	objects, err := groupInventory(c.BackupID, all)
	if err != nil {
		return retentionPlan{}, err
	}
	records, err := m.store.streams(c)
	if err != nil {
		return retentionPlan{}, err
	}
	current := ""
	if m.replicaDB != nil {
		current = m.identity.StreamID
	}
	for stream, items := range objects {
		r, owned := records[stream]
		if !owned {
			continue
		}
		if r.State == "deleted" {
			return retentionPlan{}, ErrUnavailable
		}
		hasIdentity := false
		for _, o := range items {
			if strings.HasSuffix(o.Key, "/"+identityFile) {
				hasIdentity = true
			}
		}
		// An interrupted metadata-last delete may already have removed the identity.
		if !hasIdentity && r.State == "deleting" {
			continue
		}
		if !hasIdentity {
			if r.State == "open" {
				continue
			}
			return retentionPlan{}, ErrUnavailable
		}
		info, err := b.identity(ctx, stream)
		if err != nil || info != r.Identity {
			return retentionPlan{}, ErrUnavailable
		}
	}
	verified := map[string]bool{}
	for _, stream := range newestSealed(records, current) {
		// A failed survivor restore stops eligibility for the whole run.
		if m.verifySurvivor(ctx, b, records[stream]) != nil {
			continue
		}
		verified[stream] = true
	}
	pinned := map[string]bool{}
	for stream := range m.previewPins {
		pinned[stream] = true
	}
	view := planRetention(time.Now().UTC(), days, current, records, objects, verified, pinned)
	view.Token = uuid.NewString()
	return retentionPlan{View: view, Config: c, Days: days, Objects: objects, Records: records, Fingerprint: inventoryFingerprint(current, objects, records)}, nil
}
func (m *Manager) PreviewRetention(ctx context.Context, days int) (RetentionPreview, error) {
	generation := m.cleanupEpoch()
	m.op.Lock()
	defer m.op.Unlock()
	if m.closed || m.restorePaused {
		return RetentionPreview{}, ErrDisabled
	}
	if days != 30 && days != 90 {
		return RetentionPreview{}, ErrConfiguration
	}
	c, _, err := m.store.load()
	if err != nil {
		return RetentionPreview{}, err
	}
	if c.AccountID == "" || c.Bucket == "" || c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return RetentionPreview{}, ErrConfiguration
	}
	scanCtx, cancel := m.cleanupContext(ctx, generation, targetKey(c))
	defer cancel()
	plan, err := m.scanRetentionLocked(scanCtx, c, days)
	if err != nil {
		return RetentionPreview{}, ErrUnavailable
	}
	r, err := m.store.retention(c)
	if err != nil {
		return RetentionPreview{}, err
	}
	r.ScannedAt = plan.View.ScannedAt
	r.ScannedBytes = plan.View.ScannedBytes
	r.EligibleBytes = plan.View.EligibleBytes
	if err = m.store.writeRetention(c, r); err != nil {
		return RetentionPreview{}, err
	}
	m.retentionPreview = &plan
	return plan.View, nil
}

// Hide durable tombstones before identity reads or LTX discovery. The same
// check in Stage blocks callers holding an old recovery-point DTO.
