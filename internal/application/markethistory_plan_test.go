package application

import (
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestPlanInstrumentRepairNeedUsesLookbackAndOpeningAnchor(t *testing.T) {
	before, err := planInstrumentRepairNeed(domain.InstrumentHistoryCoverage{
		ProviderKey:    TiingoProviderKey,
		ProviderSymbol: "AAPL",
		Market:         "US",
	}, "2026-09-06", "2026-09-08")
	if err != nil {
		t.Fatal(err)
	}
	if !before.OpeningAnchorMissing || before.OpeningAnchorDate != "" {
		t.Fatalf("opening anchor before persist = %+v", before)
	}
	if before.FetchRange.Start != "2025-09-06" || before.FetchRange.End != "2026-09-08" {
		t.Fatalf("fetch range = %+v, want 2025-09-06..2026-09-08 (365d lookback from origin through last finalized US date)", before.FetchRange)
	}
	if len(before.MissingRanges) == 0 {
		t.Fatal("expected coverage gaps before persist")
	}

	after, err := planInstrumentRepairNeed(domain.InstrumentHistoryCoverage{
		ProviderKey:        TiingoProviderKey,
		ProviderSymbol:     "AAPL",
		Market:             "US",
		CloseMarketDates:   []string{"2026-09-04", "2026-09-07", "2026-09-08"},
		NoObservationDates: []string{"2026-09-05", "2026-09-06"},
	}, "2026-09-06", "2026-09-08")
	if err != nil {
		t.Fatal(err)
	}
	if after.OpeningAnchorMissing || after.OpeningAnchorDate != "2026-09-04" {
		t.Fatalf("opening anchor after persist = %+v", after)
	}
	covered := map[string]struct{}{"2026-09-04": {}, "2026-09-05": {}, "2026-09-06": {}, "2026-09-07": {}, "2026-09-08": {}}
	for _, rng := range after.MissingRanges {
		for _, date := range mustInclusiveDates(t, rng) {
			if _, ok := covered[string(date)]; ok {
				t.Fatalf("persisted close or no-observation still listed as missing: %s", date)
			}
		}
	}
}

func mustInclusiveDates(t *testing.T, rng DateRange) []MarketDate {
	t.Helper()
	dates, err := InclusiveMarketDates(rng)
	if err != nil {
		t.Fatal(err)
	}
	return dates
}
