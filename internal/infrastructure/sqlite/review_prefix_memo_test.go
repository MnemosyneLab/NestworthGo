package sqlite

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
)

// Pure coverage selection, without certificates, SQL mutation or authority.
func TestReviewMemoCoverageStopsAtMissingOrUnattestedEntry(t *testing.T) {
	key := domain.ReviewDailyEvidenceKey{RuleID: domain.NewReviewRuleID(), InstrumentID: domain.NewInstrumentID(), ConditionVersion: 1, TimezoneVersion: "UTC/v1"}
	origins := map[string]string{reviewMemoScope(key): "2026-10-01"}
	prefix := []domain.ReviewSuffixDay{}
	entries := map[domain.ReviewDailyEvidenceKey]reviewMemoEntry{}
	for _, on := range []string{"2026-10-01", "2026-10-02", "2026-10-03"} {
		k := key
		k.On = on
		prefix = append(prefix, domain.ReviewSuffixDay{Key: k, OutputAttested: true})
		entries[k] = reviewMemoEntry{Key: k}
	}
	if n := len(contiguousReviewMemoEntries(prefix, entries, origins)); n != 3 {
		t.Fatal(n)
	}
	delete(entries, prefix[1].Key)
	if n := len(contiguousReviewMemoEntries(prefix, entries, origins)); n != 1 {
		t.Fatal("crossed missing coverage", n)
	}
	entries[prefix[1].Key] = reviewMemoEntry{Key: prefix[1].Key}
	prefix[0].OutputAttested = false
	if n := len(contiguousReviewMemoEntries(prefix, entries, origins)); n != 0 {
		t.Fatal("crossed unattested origin", n)
	}
}

// A delayed postcommit callback may arrive after a newer transaction's hook.
// Pure state fencing, without minting or modifying any certificate.
func TestReviewMemoStaleCommitHookLeavesNewerState(t *testing.T) {
	db := &DB{}
	db.reviewPrefixMemo.active.Store(true)
	r := NewRepository(db)
	newer := &reviewMemoSnapshot{receipt: 9}
	db.reviewPrefixMemo.generation = 4
	db.reviewPrefixMemo.committed = newer
	r.clearReviewMemoGeneration(3)
	if len(r.readReviewMemo(domain.ReviewProjectionCapture{}, 3)) != 0 {
		t.Fatal("stale capture read memo")
	}
	r.commitReviewMemo(domain.ReviewProjectionCapture{}, domain.ReviewTemporalCertificate{}, 3)
	r.commitReviewMemoNoop(nil, domain.ReviewTemporalCertificate{}, 3)
	if db.reviewPrefixMemo.generation != 4 || db.reviewPrefixMemo.committed != newer {
		t.Fatal("stale hook changed newer memo")
	}
}

func TestDefaultReviewOptimizations(t *testing.T) {
	db := &DB{}
	r := NewRepository(db).WithDefaultReviewOptimizations()
	if !r.reviewPrefixMemoEnabled || !r.reviewDependencySuffix || !r.CertifiedReviewEnabled() || r.reviewFullRecompute || !db.reviewPrefixMemo.active.Load() {
		t.Fatal("incorrect startup configuration")
	}
	// Bare repositories retain the supported C2 compatibility behavior.
	bare := NewRepository(&DB{})
	if bare.CertifiedReviewEnabled() || !bare.reviewWindowBaseline {
		t.Fatal("bare repository initialization changed")
	}
}
