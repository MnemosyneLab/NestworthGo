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
	// Preserve the scale axes and keep absolute cash observations isolated from
	// holdings and funded mixed activity. These checks stay outside query timers.
	for _, check := range []struct {
		name, query string
		want        int
	}{
		{"brokerage accounts", `SELECT count(*) FROM accounts WHERE account_type='brokerage'`, c.Accounts},
		{"cash-only account", `SELECT count(*) FROM accounts WHERE name='Synthetic cash observations'`, 1},
		{"holdings", `SELECT count(*) FROM holdings`, c.Holdings},
		{"bulk cash observations", `SELECT count(*) FROM activity_effects e JOIN accounts a ON a.id=e.account_id WHERE a.name='Synthetic cash observations' AND e.role='observation' AND e.amount='0'`, c.CashActivities},
		{"bulk cash holding overlap", `SELECT count(*) FROM holdings h JOIN accounts a ON a.id=h.account_id WHERE a.name='Synthetic cash observations'`, 0},
		{"bulk cash movement overlap", `SELECT count(*) FROM activity_effects e JOIN accounts a ON a.id=e.account_id WHERE a.name='Synthetic cash observations' AND e.role<>'observation'`, 0},
	} {
		var got int
		if err := fixture.db.SQL.QueryRowContext(t.Context(), check.query).Scan(&got); err != nil {
			t.Fatalf("%s: %v", check.name, err)
		}
		if got != check.want {
			t.Fatalf("%s: got %d, want %d", check.name, got, check.want)
		}
	}
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
