package application

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
)

func TestReviewPrefixMemoColdWarmOwnedAndNoop(t *testing.T) {
	f, r, tracker := memoFixture(t, domain.ReviewTriggerPolicy{Kind: "rolling", WindowDays: 5, MinHits: 3})
	r = r.WithDefaultReviewOptimizations()
	f.s.repository = r
	f.day(t, "2026-10-13")
	cold, obs := assertMemoOracle(t, f, r, tracker, true)
	if obs.reusedDays != 0 || obs.sqlRows != 22 || cold.Capture.Certified.SuffixMemoReceipt == 0 {
		t.Fatalf("cold observation %+v", obs)
	}
	// Returned captures belong to the caller; modifying their maps/pointers after
	// commit cannot change the private immutable encoding used by later capture.
	*cold.Capture.Suffix.Prefix[0].Evidence.Input.Quantity = "999"
	cold.Capture.Suffix.Prefix[0].Evidence.Result.Truth = domain.ReviewUnknown
	requireCertifiedNoop(t, f, r)
	f.day(t, "2026-10-14")
	_, obs = assertMemoOracle(t, f, r, tracker, true)
	if obs.reusedDays != 22 || obs.sqlRows != 2 {
		t.Fatalf("warm observation %+v", obs)
	}
	f.day(t, "2026-10-15")
	_, obs = assertMemoOracle(t, f, r, tracker, true)
	if obs.reusedDays != 24 || obs.sqlRows != 2 {
		t.Fatalf("next observation %+v", obs)
	}
}
