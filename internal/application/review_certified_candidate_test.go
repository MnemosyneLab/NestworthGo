package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func certifiedFixture(t *testing.T) (seriesZeroFixture, *sqlite.Repository) {
	t.Helper()
	return certifiedConfiguredFixture(t, false, syntheticHistoricalReviewSpec())
}
func certifiedConfiguredFixture(t *testing.T, fx bool, spec domain.ReviewRuleSpec) (seriesZeroFixture, *sqlite.Repository) {
	t.Helper()
	return certifiedQuantityFixture(t, fx, spec, "1")
}
func certifiedQuantityFixture(t *testing.T, fx bool, spec domain.ReviewRuleSpec, quantity string) (seriesZeroFixture, *sqlite.Repository) {
	t.Helper()
	f := newSeriesZeroFixture(t, fx, "UTC", "2026-10-01", quantity)
	f.day(t, "2026-10-12")
	saveSyntheticReviewRule(t, f, "Synthetic certified review", spec)
	r := sqlite.NewRepository(f.db).WithCertifiedReviewCandidate()
	fake := newRefreshFakeProvider()
	fake.key = "synthetic-pinned"
	f.s = NewService(r, NewMarketDataRegistry(fake, &refreshAliasProvider{key: "synthetic-other", target: fake}))
	if err := f.s.SetFXProvider("synthetic-pinned"); err != nil {
		t.Fatal(err)
	}
	f.s.setClock(func() time.Time { return *f.now })
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	return f, r
}

func TestReviewCertifiedFreshSessionNoopAndBoundedPublication(t *testing.T) {
	f, r := certifiedFixture(t)
	var mac string
	if err := f.db.SQL.QueryRow(`SELECT json_extract(certificate_json,'$.CertifiedMAC') FROM review_temporal_checkpoints`).Scan(&mac); err != nil || mac == "" {
		t.Fatal("fresh publication did not establish integrity authority", mac, err)
	}
	*f.now = f.now.Add(time.Nanosecond)
	result, err := f.s.ReconcileInvestmentReviewsV2(t.Context())
	if err != nil || result.DayCount != 0 {
		t.Fatal("certified no-op failed", result, err)
	}
	f.day(t, "2026-10-13")
	cap, err := r.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if cap.Certified == nil || !cap.Certified.MayReuse || len(cap.ReusableDailyEvidence) != 1 {
		t.Fatalf("capture failed integrity reuse gate %+v evidence=%d", cap.Certified, len(cap.ReusableDailyEvidence))
	}
	result, err = f.s.ReconcileInvestmentReviewsV2(t.Context())
	if err != nil || result.DayCount != 1 {
		t.Fatal("certified bounded publication failed", result, err)
	}
	*f.now = f.now.Add(time.Nanosecond)
	result, err = f.s.ReconcileInvestmentReviewsV2(t.Context())
	if err != nil || result.DayCount != 0 {
		t.Fatal("bounded publication lost certification", result, err)
	}
	noChangeFullOracle(t, f)
	if sqlite.NewRepository(f.db).CertifiedReviewEnabled() || r.WithFullReviewRecompute().CertifiedReviewEnabled() {
		t.Fatal("candidate activated default or forced-full path")
	}
}
