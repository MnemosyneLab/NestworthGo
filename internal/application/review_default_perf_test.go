//go:build perf

package application

import (
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"reflect"
	"testing"
	"time"
)

// Fixed synthetic default-path samples. Fixture creation and independent oracle
// checks are outside each timer. These few samples do not establish p95/RSS or
// a global latency target, and write no evidence/profile artifacts.
func TestReviewDefaultPerformance(t *testing.T) {
	fixture := newReviewPerfFixture(t, reviewPerfConfig{Accounts: 2, Instruments: 4, Holdings: 4, Rules: 3, HistoryDays: 30, Quotes: 120, MixedActivities: 8})
	repository := sqlite.NewRepository(fixture.db).WithDefaultReviewOptimizations()
	provider := newRefreshFakeProvider()
	provider.key = "synthetic-pinned"
	fixture.service = NewService(repository, NewMarketDataRegistry(provider))
	if err := fixture.service.SetFXProvider(provider.key); err != nil {
		t.Fatal(err)
	}
	fixture.service.setClock(func() time.Time { return *fixture.now })
	f := seriesZeroFixture{s: fixture.service, db: fixture.db, now: fixture.now}
	f.account.Account.HouseholdID = fixture.householdID
	for _, scenario := range []string{"initial", "same-day", "next-day", "correction"} {
		t.Run(scenario, func(t *testing.T) {
			if scenario == "next-day" {
				*fixture.now = fixture.now.AddDate(0, 0, 1)
			}
			if scenario == "correction" {
				if err := correctReviewPerfHistoricalQuote(t.Context(), fixture, 0); err != nil {
					t.Fatal(err)
				}
			}
			started := time.Now()
			result, err := fixture.service.ReconcileInvestmentReviewsV2(t.Context())
			elapsed := time.Since(started)
			if err != nil {
				t.Fatal(err)
			}
			if result.RuleInstrumentCount == 0 {
				t.Fatal("vacuous default reconciliation")
			}
			if scenario == "initial" && result.DayCount == 0 {
				t.Fatal("initial preparation skipped")
			}
			if scenario == "same-day" && result.DayCount != 0 {
				t.Fatal("default certified confirmation missed")
			}
			reference := suffixFullReference(t, f)
			if !reflect.DeepEqual(noChangeProjectionRows(t, fixture.db), noChangeProjectionRows(t, reference)) || !reflect.DeepEqual(publicationDailyTimes(t, fixture.db), publicationDailyTimes(t, reference)) {
				t.Fatal("default path differs from independent full durable oracle")
			}
			stage := certifiedStage(t, f, repository)
			assertReviewFullOracle(t, fixture, stage)
			requireCertifiedNoop(t, f, repository)
			t.Logf("default review scenario=%s elapsed=%s emittedDays=%d ruleInstrumentPairs=%d", scenario, elapsed, result.DayCount, result.RuleInstrumentCount)
		})
	}
}
