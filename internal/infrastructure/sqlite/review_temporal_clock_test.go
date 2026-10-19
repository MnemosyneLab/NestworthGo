package sqlite

import (
	"testing"
	"time"
)

func TestReviewTemporalClockStoresWallInstantsAndRotatesOnRollback(t *testing.T) {
	var clock reviewTemporalClock
	observed := time.Now()
	epoch, rollback := clock.observe(observed)
	if rollback || epoch == "" || clock.last != observed.UTC() {
		t.Fatal("clock retained a monotonic/local representation instead of the exact UTC wall instant")
	}
	later := observed.Add(time.Hour)
	if current, rollback := clock.observe(later); rollback || current != epoch {
		t.Fatal("forward clock invalidated current generation")
	}
	// Roll back while remaining later than the original published time.
	back := observed.Add(30 * time.Minute)
	current, rollback := clock.observe(back)
	if !rollback || current == epoch || clock.last != back.UTC() {
		t.Fatal("rollback above publication was hidden or clamped")
	}
	if next, rollback := clock.observe(back.Add(time.Nanosecond)); rollback || next != current {
		t.Fatal("fresh forward observation reused an older generation")
	}
}
