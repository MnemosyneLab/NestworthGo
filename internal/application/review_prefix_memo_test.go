package application

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
)

func TestReviewPrefixMemoColdWarmOwnedAndNoop(t *testing.T) {
	f, r, _ := suffixFixture(t, domain.ReviewTriggerPolicy{Kind: "rolling", WindowDays: 5, MinHits: 3})
	r = r.WithReviewPrefixMemoExperiment()
	f.s.repository = r
	f.day(t, "2026-10-13")
	cold := assertSuffixOracle(t, f, r, true)
	obs := cold.Capture.Certified.SuffixMemoObservation
	if obs == nil || obs.ReusedDays != 0 || obs.SQLDecodedDays != 22 || obs.ArgumentEncodes != 22 || cold.Capture.Certified.SuffixMemoReceipt == 0 {
		t.Fatalf("cold observation %+v", obs)
	}
	// Returned captures belong to the caller; modifying their maps/pointers after
	// commit cannot change the private immutable encoding used by later capture.
	*cold.Capture.Suffix.Prefix[0].Evidence.Input.Quantity = "999"
	cold.Capture.Suffix.Prefix[0].Evidence.Result.Truth = domain.ReviewUnknown
	requireCertifiedNoop(t, f, r)
	f.day(t, "2026-10-14")
	warm := assertSuffixOracle(t, f, r, true)
	obs = warm.Capture.Certified.SuffixMemoObservation
	if obs == nil || obs.ReusedDays != 22 || obs.SQLDecodedDays != 2 || obs.ArgumentEncodes != 2 {
		t.Fatalf("warm observation %+v", obs)
	}
	f.day(t, "2026-10-15")
	next := assertSuffixOracle(t, f, r, true)
	obs = next.Capture.Certified.SuffixMemoObservation
	if obs == nil || obs.ReusedDays != 24 || obs.SQLDecodedDays != 2 || obs.ArgumentEncodes != 2 {
		t.Fatalf("next observation %+v", obs)
	}
}
