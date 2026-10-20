//go:build perf

package application

import (
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Measures the application projection, including SQL, snapshot maintenance,
// kernel and DTO. A fresh analysis memo is used on both clones; the second
// sample starts with durable closed snapshots but no in-memory memo.
func TestAnalysisEndToEndPerformanceEvidence(t *testing.T) {
	c := reviewPerfConfig{Accounts: 2, Instruments: 10, Holdings: 10, Rules: 1, HistoryDays: 30, Quotes: 310, CashActivities: 100, MixedActivities: 8}
	fixture := newReviewPerfFixture(t, c)
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, Basis: domain.ReturnBasisInvestment, From: domain.LocalDate(fixture.start.Format(time.DateOnly)), To: domain.LocalDate(fixture.now.AddDate(0, 0, -1).Format(time.DateOnly)), Valuation: domain.ValuationBase}
	var previous ReturnCalendarResult
	for sample := 0; sample < 2; sample++ {
		run := cloneReviewPerfFixture(t, fixture)
		metrics := enableReviewSQLMetrics(t, run)
		started := time.Now()
		result, err := run.service.ReturnCalendar(t.Context(), query, "", "day")
		elapsed := time.Since(started)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("analysis SQL -> ensure snapshots -> kernel -> calendar DTO: sample=%d elapsed=%s %s (driver query duration includes consumption)", sample, elapsed, metrics.snapshot(8).summary())
		if sample == 1 && !reflect.DeepEqual(previous, result) {
			t.Fatal("analysis cold/warm durable snapshot results differ")
		}
		previous = result
		// Subsequent clone contains snapshots but a new Service/memo.
		fixture = run
	}
}
