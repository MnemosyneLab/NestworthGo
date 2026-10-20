package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Ordinary C3 accelerates confirmation and source CAS. Missing a confirmation
// does not implicitly opt the call into full + bounded runtime experiments.
func TestReviewCertifiedFullDispatchAndObserver(t *testing.T) {
	f, r := certifiedPublicationFixture(t, true, syntheticHistoricalReviewSpec(), "1", false)
	observations := []domain.ReviewCompletenessLineageObservation{}
	r = r.WithReviewLineageObserver(func(o domain.ReviewCompletenessLineageObservation) {
		observations = append(observations, o)
	})
	f.s.repository = r
	if r.ReviewCompletenessLineageShadowEnabled() {
		t.Fatal("observer enabled the window experiment")
	}
	f.day(t, "2026-10-13")
	p := certifiedStage(t, f, r)
	if !p.Capture.TemporalFullRecompute || p.Capture.Certified.MayReuse || len(p.Capture.ReusableDailyEvidence) != 0 || p.Capture.CompletenessLineage != nil || p.WindowOracle != nil || p.CertifiedFullOracle != nil {
		t.Fatal("ordinary C3 selected redundant bounded calculation")
	}
	if len(p.Rules) != 1 || len(p.Rules[0].Days) != 12 {
		t.Fatal("full preparation omitted historical days")
	}
	assertReviewFullOracle(t, reviewFullPipelineFixture{db: f.db, service: f.s, householdID: f.account.Account.HouseholdID, now: f.now}, p)
	if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
		t.Fatal(err)
	}
	if len(observations) != 1 || observations[0].Phase != "certified_publication" || observations[0].Verdict != "verified_source_cas" {
		t.Fatal("full dispatch lost certified source publication", observations)
	}
	*f.now = f.now.Add(time.Nanosecond)
	result, err := f.s.ReconcileInvestmentReviewsV2(t.Context())
	if err != nil || result.DayCount != 0 {
		t.Fatal("full publication lost same-day confirmation", result, err)
	}
	noChangeFullOracle(t, f)
}
