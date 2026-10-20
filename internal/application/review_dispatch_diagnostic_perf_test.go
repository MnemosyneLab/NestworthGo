//go:build perf

package application

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"os"
	"testing"
)

// Observe the supported selectors on synthetic data. No signed proof is
// fabricated or modified; the existing publication methods own the transition.
func TestReviewClockSelectorProvenanceDiagnostic(t *testing.T) {
	if os.Getenv("NW_D2_DISPATCH_EVIDENCE") != "1" {
		t.Skip("opt-in selector diagnostic")
	}
	f, d2, _ := suffixFixture(t, domain.ReviewTriggerPolicy{Kind: "single"})
	if !d2.WithBoundedReviewWindow().CertifiedReviewEnabled() {
		t.Fatal("bounded selector unexpectedly disables certified mode")
	}
	f.day(t, "2026-10-13")
	c2 := sqlite.NewRepository(f.db).WithBoundedReviewWindow()
	f.s.repository = c2
	c, err := c2.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if c.Certified != nil || c.TemporalFullRecompute || len(c.ReusableDailyEvidence) == 0 {
		t.Fatal("expected existing cheap C2 bounded capture")
	}
	stage, err := PrepareReviewPublication(t.Context(), c, *f.now, f.s.FXProviderKey())
	if err != nil {
		t.Fatal(err)
	}
	assertReviewFullOracle(t, reviewFullPipelineFixture{db: f.db, service: f.s, householdID: f.account.Account.HouseholdID, now: f.now}, stage)
	if err = c2.PublishReviewProjection(t.Context(), stage, *f.now); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err = f.db.SQL.QueryRow(`SELECT certificate_json FROM review_temporal_checkpoints`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var cert domain.ReviewTemporalCertificate
	if err = jsoncodec.Unmarshal([]byte(raw), &cert); err != nil {
		t.Fatal(err)
	}
	if cert.CertifiedMAC != "" {
		t.Fatal("ordinary C2 unexpectedly certified historical output")
	}
	c, err = d2.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if c.Suffix != nil || c.Certified == nil || c.Certified.MayCertify {
		t.Fatal("C2 transition unexpectedly preserved/reacquired certified reuse authority")
	}
	t.Log("existing bounded selector on D2 retains C3; fresh C2 selector is cheap but clears certified chain; returning to D2 falls back without historical authority")
}
