package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReviewCertifiedFXEvidenceAndOracleIsolation(t *testing.T) {
	spec := syntheticHistoricalReviewSpec()
	spec.Predicate.All[1].Leaf.FXMode = "convert_on_day"
	f, r := certifiedConfiguredFixture(t, true, spec)
	f.day(t, "2026-10-13")
	p := certifiedStage(t, f, r)
	if p.CertifiedFullOracle == nil || domain.ValidateReviewCertifiedOutput(p) != "window_oracle_matched" {
		t.Fatal("FX candidate could not prove bounded/full equivalence")
	}
	full := domain.ReviewFullPublication(p)
	if err := compareReviewFullOracle(p, full, f.s.FXProviderKey()); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, cached := range p.Capture.ReusableDailyEvidence {
		for _, selected := range cached.Input.SelectedFX {
			found = true
			if selected.Quote.Rate.String() != "2" {
				t.Fatal("private exact FX scalar was lost in persisted evidence", selected.Quote.Rate.String())
			}
		}
	}
	if !found {
		t.Fatal("fixture did not require selected FX evidence")
	}
	if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
		t.Fatal(err)
	}
	*f.now = f.now.Add(time.Nanosecond)
	got, err := f.s.ReconcileInvestmentReviewsV2(t.Context())
	if err != nil || got.DayCount != 0 {
		t.Fatal("FX publication lost certification", got, err)
	}
}

func TestReviewCertifiedFullOracleDoesNotAliasFrozenOutput(t *testing.T) {
	f, r := certifiedFixture(t)
	f.day(t, "2026-10-13")
	p := certifiedStage(t, f, r)
	if p.CertifiedFullOracle == nil {
		t.Fatal("fixture not bounded")
	}
	p.Rules[0].TriggerResult.Truth = domain.ReviewFalse
	if p.CertifiedFullOracle.Rules[0].TriggerResult.Truth != domain.ReviewTrue {
		t.Fatal("full oracle aliased the bounded frozen result")
	}
	if domain.ValidateReviewCertifiedOutput(p) != "window_oracle_lifecycle_changed" {
		t.Fatal("in-place bounded output mutation escaped comparison")
	}
}
