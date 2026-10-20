package application

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"strings"
	"sync"
	"testing"
)

// Physical suffix SQL rows distinguish private memo hits from a vacuous
// successful publication without retaining diagnostic state in signed captures.
type memoSQLTracker struct {
	mu   sync.Mutex
	rows int
}
type memoSQLObservation struct{ reusedDays, sqlRows int }

func trackMemoSQL(faults *reviewDriverFaults) *memoSQLTracker {
	tracker := &memoSQLTracker{}
	faults.afterRows = func(query string, rows int) {
		if strings.Contains(query, "FROM requested i CROSS JOIN investment_review_daily_inputs d") {
			tracker.mu.Lock()
			tracker.rows += rows
			tracker.mu.Unlock()
		}
	}
	return tracker
}
func memoFixture(t *testing.T, policy domain.ReviewTriggerPolicy) (seriesZeroFixture, *sqlite.Repository, *memoSQLTracker) {
	t.Helper()
	db, faults := openReviewFaultDatabase(t)
	f, r, _ := suffixFixtureWithDB(t, policy, db)
	return f, r, trackMemoSQL(faults)
}
func assertMemoOracle(t *testing.T, f seriesZeroFixture, r *sqlite.Repository, tracker *memoSQLTracker, want bool) (domain.ReviewPublication, memoSQLObservation) {
	t.Helper()
	tracker.mu.Lock()
	tracker.rows = 0
	tracker.mu.Unlock()
	p := assertSuffixOracle(t, f, r, want)
	tracker.mu.Lock()
	rows := tracker.rows
	tracker.mu.Unlock()
	observation := memoSQLObservation{sqlRows: rows}
	if p.Capture.Suffix != nil {
		observation.reusedDays = max(len(p.Capture.Suffix.Prefix)-rows, 0)
	}
	return p, observation
}
