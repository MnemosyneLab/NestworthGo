package application

import (
	"strings"
	"testing"
	"time"
)

// Provider invalidation requires full preparation after a confirmation miss;
// the source CAS guards publication against changes after that preparation.
func TestReviewCertifiedProviderInvalidationFullDispatch(t *testing.T) {
	db, faults := openReviewFaultDatabase(t)
	f, r := certifiedPublicationFixture(t, true, syntheticHistoricalReviewSpec(), "1", db)
	f.day(t, "2026-10-13")
	if err := f.s.SetFXProvider("synthetic-pinned"); err != nil {
		t.Fatal(err)
	}
	p := certifiedStage(t, f, r)
	if !p.Capture.TemporalFullRecompute || len(p.Capture.ReusableDailyEvidence) != 0 {
		t.Fatal("ordinary C3 selected redundant bounded calculation")
	}
	if len(p.Rules) != 1 || len(p.Rules[0].Days) != 12 {
		t.Fatal("full preparation omitted historical days")
	}
	assertReviewFullOracle(t, reviewFullPipelineFixture{db: f.db, service: f.s, householdID: f.account.Account.HouseholdID, now: f.now}, p)
	fullReads := 0
	faults.afterQuery = func(query string) {
		if strings.Contains(query, "FROM history_origins") {
			fullReads++
		}
	}
	if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
		t.Fatal(err)
	}
	if fullReads != 0 {
		t.Fatal("authenticated unchanged source CAS reloaded financial history", fullReads)
	}
	*f.now = f.now.Add(time.Nanosecond)
	result, err := f.s.ReconcileInvestmentReviewsV2(t.Context())
	if err != nil || result.DayCount != 0 {
		t.Fatal("full publication lost same-day confirmation", result, err)
	}
	noChangeFullOracle(t, f)
}
