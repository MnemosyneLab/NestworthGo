package sqlite

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

const reviewPrefixMemoVersion = "owned-lossless-prefix-v1"
const reviewPrefixMemoMaxEntries = 65536
const reviewPrefixMemoMaxPayload = 32 << 20

// Explicit experiment opt-in. A new handle never imports a memo or authority.
func (r *Repository) WithReviewPrefixMemoExperiment(enabled ...bool) *Repository {
	copy := *r.WithReviewDependencySuffix()
	copy.reviewPrefixMemoEnabled = len(enabled) == 0 || enabled[0]
	copy.reviewPrefixMemoObserve = true
	return &copy
}

type reviewMemoReadTarget struct {
	entries     map[domain.ReviewDailyEvidenceKey]reviewMemoEntry
	observation *domain.ReviewPrefixMemoObservation
	payload     int
	disabled    bool
}

func (t *reviewMemoReadTarget) add(e reviewMemoEntry) {
	if t.disabled {
		return
	}
	n := len(e.Input) + len(e.Coverage) + len(e.Result) + len(e.Fingerprint)
	if len(t.entries) >= reviewPrefixMemoMaxEntries || t.payload+n > reviewPrefixMemoMaxPayload {
		t.entries = nil
		t.disabled = true
		return
	}
	if _, exists := t.entries[e.Key]; exists {
		return
	}
	t.entries[e.Key] = e
	t.payload += n
}

type reviewMemoEntry struct {
	Key                                  domain.ReviewDailyEvidenceKey
	Input, Coverage, Result, Fingerprint string
	Prior                                int
}
type reviewMemoBinding struct {
	Household, Predecessor, Seal, Source, Fingerprint, Provider, Clock, Version string
}
type reviewMemoSnapshot struct {
	binding             reviewMemoBinding
	entries             []reviewMemoEntry // immutable; strings only, never exposed
	digest, mac         string
	generation, receipt uint64
}
type reviewPrefixMemoState struct {
	mu                 sync.Mutex
	generation, next   uint64
	committed, pending *reviewMemoSnapshot
}

func reviewMemoBind(c domain.ReviewProjectionCapture) reviewMemoBinding {
	source, _ := json.Marshal(c.SourceRevision)
	provider, _ := json.Marshal(c.PublishedTemporal.CertifiedProvider)
	return reviewMemoBinding{c.HouseholdID.String(), reviewTemporalDigest(c.PublishedTemporal), c.PublishedTemporal.CertifiedProjectionSeal, string(source), c.HistoricalFingerprint, string(provider), c.Temporal.ClockEpoch, reviewPrefixMemoVersion + "/" + domain.ReviewCertifiedCompletenessVersion + "/" + domain.ReviewTemporalPolicyVersion}
}
func reviewMemoDigest(entries []reviewMemoEntry) string {
	h := sha256.New()
	var size [8]byte
	write := func(s string) {
		binary.BigEndian.PutUint64(size[:], uint64(len(s)))
		h.Write(size[:])
		h.Write([]byte(s))
	}
	for _, e := range entries {
		key, _ := json.Marshal(e.Key)
		write(string(key))
		write(e.Input)
		write(e.Coverage)
		write(e.Result)
		write(e.Fingerprint)
		write(fmt.Sprint(e.Prior))
	}
	return hex.EncodeToString(h.Sum(nil))
}
func (r *Repository) memoMAC(s *reviewMemoSnapshot) (string, error) {
	return r.reviewMAC(struct {
		Binding             reviewMemoBinding
		Digest              string
		Generation, Receipt uint64
	}{s.binding, s.digest, s.generation, s.receipt})
}
func (r *Repository) clearReviewMemo() {
	m := &r.database.reviewPrefixMemo
	m.mu.Lock()
	m.committed = nil
	m.pending = nil
	m.mu.Unlock()
}
func (r *Repository) readReviewMemo(c domain.ReviewProjectionCapture) map[domain.ReviewDailyEvidenceKey]reviewMemoEntry {
	out := map[domain.ReviewDailyEvidenceKey]reviewMemoEntry{}
	m := &r.database.reviewPrefixMemo
	m.mu.Lock()
	s := m.committed
	m.mu.Unlock()
	if s == nil {
		return out
	}
	// Immutable private snapshots can be authenticated without holding the mutex.
	mac, err := r.memoMAC(s)
	if err != nil || s.binding != reviewMemoBind(c) || s.digest != reviewMemoDigest(s.entries) || mac != s.mac {
		r.clearReviewMemo()
		return out
	}
	for _, e := range s.entries {
		if _, exists := out[e.Key]; exists {
			r.clearReviewMemo()
			return map[domain.ReviewDailyEvidenceKey]reviewMemoEntry{}
		}
		out[e.Key] = e
	}
	return out
}
func (e reviewMemoEntry) decode() (domain.ReviewReusableDailyEvidence, error) {
	var v domain.ReviewReusableDailyEvidence
	if err := decodeReviewInput([]byte(e.Input), &v.Input); err != nil {
		return v, err
	}
	if err := json.Unmarshal([]byte(e.Coverage), &v.CoverageMissing); err != nil {
		return v, err
	}
	if err := json.Unmarshal([]byte(e.Result), &v.Result); err != nil {
		return v, err
	}
	if v.Input.On != e.Key.On || v.Input.InstrumentID != e.Key.InstrumentID || v.Result.On != e.Key.On {
		return v, fmt.Errorf("memo typed identity mismatch")
	}
	v.HasResult = true
	v.InputFingerprint = e.Fingerprint
	v.PriorConsecutiveCount = e.Prior
	return v, nil
}

// Only private entries supplied by the coherent reader are considered. A gap
// stops that scope; successful UPSERTs and caller-owned typed values never seed.
func (r *Repository) stageReviewMemo(c *domain.ReviewProjectionCapture, entries map[domain.ReviewDailyEvidenceKey]reviewMemoEntry, origins map[string]string) error {
	if len(entries) == 0 {
		return nil
	}
	list := contiguousReviewMemoEntries(c.Suffix.Prefix, entries, origins)
	if len(list) == 0 || len(list) > reviewPrefixMemoMaxEntries {
		return nil
	}
	payload := 0
	for _, e := range list {
		payload += len(e.Input) + len(e.Coverage) + len(e.Result) + len(e.Fingerprint)
	}
	if payload > reviewPrefixMemoMaxPayload {
		return nil
	}
	m := &r.database.reviewPrefixMemo
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next++
	s := &reviewMemoSnapshot{binding: reviewMemoBind(*c), entries: list, digest: reviewMemoDigest(list), generation: m.generation, receipt: m.next}
	var err error
	s.mac, err = r.memoMAC(s)
	if err != nil {
		return err
	}
	m.pending = s
	c.Certified.SuffixMemoReceipt = s.receipt
	return nil
}
func reviewMemoScope(k domain.ReviewDailyEvidenceKey) string {
	k.On = ""
	raw, _ := json.Marshal(k)
	return string(raw)
}
func reviewMemoNextDay(on string) string {
	t, _ := time.Parse(time.DateOnly, on)
	return t.AddDate(0, 0, 1).Format("2006-01-02")
}

// The generation is observed inside the SQL transaction. A delayed postcommit
// hook cannot replace newer state. No mutex is held across SQLite COMMIT.
func (r *Repository) commitReviewMemo(c domain.ReviewProjectionCapture, out domain.ReviewTemporalCertificate, expected uint64) {
	m := &r.database.reviewPrefixMemo
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.generation != expected {
		return
	}
	p := m.pending
	m.pending = nil
	valid := r.reviewPrefixMemoEnabled && c.Certified != nil && c.Suffix != nil && out.CertifiedMAC != "" && p != nil && p.receipt == c.Certified.SuffixMemoReceipt && p.generation == m.generation && p.binding == reviewMemoBind(c)
	m.generation++
	m.committed = nil
	if !valid {
		return
	}
	// These exact private entries were attested before capture exposure and were
	// omitted by the authenticated unique publication partition, hence unchanged.
	q := *p
	// Pending storage is already private and immutable; share its header.
	q.entries = p.entries
	q.generation = m.generation
	q.binding.Predecessor = reviewTemporalDigest(&out)
	q.binding.Seal = out.CertifiedProjectionSeal
	q.digest = p.digest
	var err error
	q.mac, err = r.memoMAC(&q)
	if err == nil {
		m.committed = &q
	}
}

// Certified noop writes only existing control/checkpoint CAS metadata. Preserve
// the old authenticated content digest; never attest or add daily evidence here.
func (r *Repository) commitReviewMemoNoop(oldJSON []byte, out domain.ReviewTemporalCertificate, expected uint64) {
	m := &r.database.reviewPrefixMemo
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.generation != expected {
		return
	}
	s := m.committed
	m.committed = nil
	m.pending = nil
	m.generation++
	if !r.reviewPrefixMemoEnabled || s == nil || out.CertifiedMAC == "" || out.Source == nil {
		return
	}
	oldHash := sha256.Sum256(oldJSON)
	source, _ := json.Marshal(out.Source)
	provider, _ := json.Marshal(out.CertifiedProvider)
	if s.binding.Predecessor != hex.EncodeToString(oldHash[:]) || s.binding.Seal != out.CertifiedProjectionSeal || s.binding.Source != string(source) || s.binding.Provider != string(provider) || s.binding.Clock != out.ClockEpoch {
		return
	}
	q := *s
	q.generation = m.generation
	q.binding.Predecessor = reviewTemporalDigest(&out)
	var err error
	q.mac, err = r.memoMAC(&q)
	if err == nil {
		m.committed = &q
	}
}

// Eviction changes only optional process-private data, never a certificate.
func (r *Repository) EvictReviewPrefixMemo() { r.clearReviewMemo() }

func contiguousReviewMemoEntries(prefix []domain.ReviewSuffixDay, entries map[domain.ReviewDailyEvidenceKey]reviewMemoEntry, origins map[string]string) []reviewMemoEntry {
	list := make([]reviewMemoEntry, 0, len(entries))
	stopped := map[string]bool{}
	last := map[string]string{}
	for _, p := range prefix {
		scope := reviewMemoScope(p.Key)
		if stopped[scope] {
			continue
		}
		e, ok := entries[p.Key]
		expected := origins[scope]
		if previous := last[scope]; previous != "" {
			expected = reviewMemoNextDay(previous)
		}
		if !ok || !p.OutputAttested || e.Key.On != expected {
			stopped[scope] = true
			continue
		}
		list = append(list, e)
		last[scope] = e.Key.On
	}
	return list
}

func (r *Repository) reviewMemoGeneration() uint64 {
	m := &r.database.reviewPrefixMemo
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.generation
}
