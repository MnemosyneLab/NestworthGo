package assistant

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
	"time"
)

func TestReturnTrendClosedCivilRangeUsesHouseholdTimezone(t *testing.T) {
	origin := &domain.HistoryOrigin{Timezone: "America/Los_Angeles", StartedAt: time.Date(2026, 1, 2, 1, 0, 0, 0, time.UTC)}
	now := time.Date(2026, 11, 5, 1, 0, 0, 0, time.UTC) // Household is still Nov 4.
	action := ReturnTrendAction{From: "2026-01-01", To: "2026-11-03", Valuation: "base", Display: "cumulative_amount"}
	if !action.WithinHistory(origin, now) {
		t.Fatal("valid household-local endpoints refused")
	}
	for _, pair := range [][2]string{{"2025-12-31", "2026-11-03"}, {"2026-01-01", "2026-11-04"}, {"2026-02-30", "2026-03-01"}, {"2026-03-01", "2026-02-28"}} {
		action.From, action.To = pair[0], pair[1]
		if action.WithinHistory(origin, now) {
			t.Fatal("invalid range was silently clamped", pair)
		}
	}
	if action.WithinHistory(nil, now) {
		t.Fatal("missing history accepted")
	}
}
