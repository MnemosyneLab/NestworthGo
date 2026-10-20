package sqlite

import (
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Capture proofs stay process-local; the entire current proof shape is bound
// by the same producer/verifier function, including eligibility and ownership.
func TestReviewCaptureMACBindsEligibilityAndMemoReceipt(t *testing.T) {
	r := NewRepository(&DB{})
	original := domain.ReviewProjectionCapture{Certified: &domain.ReviewCertifiedCapture{MayCertify: true, InitialPublication: true, SuffixMemoReceipt: 7}}
	want, err := r.sourceCaptureMAC(original)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*domain.ReviewCertifiedCapture){
		func(p *domain.ReviewCertifiedCapture) { p.MayCertify = false },
		func(p *domain.ReviewCertifiedCapture) { p.InitialPublication = false },
		func(p *domain.ReviewCertifiedCapture) { p.SuffixMemoReceipt++ },
	} {
		proof := *original.Certified
		mutate(&proof)
		changed := original
		changed.Certified = &proof
		got, err := r.sourceCaptureMAC(changed)
		if err != nil || got == want {
			t.Fatalf("proof mutation was not authenticated: %v", err)
		}
	}
}
