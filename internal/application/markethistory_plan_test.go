package application

import (
	"testing"
	"time"

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

func TestPlanInstrumentHistorySyncForceRecheckAndRouting(t *testing.T) {
	now := time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC)
	covered, err := planInstrumentRepairNeed(domain.InstrumentHistoryCoverage{
		ProviderKey:        TiingoProviderKey,
		ProviderSymbol:     "AAPL",
		Market:             "US",
		CloseMarketDates:   []string{"2026-09-04", "2026-09-07", "2026-09-08"},
		NoObservationDates: []string{"2026-09-05", "2026-09-06"},
		CloseFetchedAt: map[string]time.Time{
			"2026-09-04": now.Add(-48 * time.Hour),
			"2026-09-07": now.Add(-2 * time.Hour),
			"2026-09-08": now.Add(-2 * time.Hour),
		},
		NoObservationExpiresAt: map[string]time.Time{
			"2026-09-05": now.Add(time.Hour),
			"2026-09-06": now.Add(time.Hour),
		},
		NoObservationCheckedAt: map[string]time.Time{
			"2026-09-05": now.Add(-time.Hour),
			"2026-09-06": now.Add(-time.Hour),
		},
	}, "2026-09-06", "2026-09-08")
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := applyHistorySyncPolicy(covered, domain.InstrumentHistoryCoverage{
		ProviderKey:        TiingoProviderKey,
		ProviderSymbol:     "AAPL",
		Market:             "US",
		CloseMarketDates:   []string{"2026-09-04", "2026-09-07", "2026-09-08"},
		NoObservationDates: []string{"2026-09-05", "2026-09-06"},
		CloseFetchedAt: map[string]time.Time{
			"2026-09-04": now.Add(-48 * time.Hour),
			"2026-09-07": now.Add(-2 * time.Hour),
			"2026-09-08": now.Add(-2 * time.Hour),
		},
		NoObservationExpiresAt: map[string]time.Time{
			"2026-09-05": now.Add(time.Hour),
			"2026-09-06": now.Add(time.Hour),
		},
		NoObservationCheckedAt: map[string]time.Time{
			"2026-09-05": now.Add(-time.Hour),
			"2026-09-06": now.Add(-time.Hour),
		},
	}, "2026-09-08", now, false)
	if err != nil {
		t.Fatal(err)
	}
	if ordinary.RouteStatus != domain.InstrumentRouteOK {
		t.Fatalf("route = %s", ordinary.RouteStatus)
	}
	fetched := map[string]struct{}{}
	for _, rng := range ordinary.FetchRanges {
		for _, date := range mustInclusiveDates(t, rng) {
			fetched[string(date)] = struct{}{}
		}
	}
	if _, ok := fetched["2026-09-04"]; ok {
		t.Fatal("ordinary sync refetched an old close outside the last-3 window")
	}
	if _, ok := fetched["2026-09-05"]; ok {
		t.Fatal("ordinary sync refetched unexpired no-observation")
	}

	forced, err := applyHistorySyncPolicy(covered, domain.InstrumentHistoryCoverage{
		ProviderKey:        TiingoProviderKey,
		ProviderSymbol:     "AAPL",
		Market:             "US",
		CloseMarketDates:   []string{"2026-09-04", "2026-09-07", "2026-09-08"},
		NoObservationDates: []string{"2026-09-05", "2026-09-06"},
	}, "2026-09-08", now, true)
	if err != nil {
		t.Fatal(err)
	}
	forcedDates := map[string]struct{}{}
	for _, rng := range forced.FetchRanges {
		for _, date := range mustInclusiveDates(t, rng) {
			forcedDates[string(date)] = struct{}{}
		}
	}
	for _, date := range []string{"2026-09-04", "2026-09-05", "2026-09-08"} {
		if _, ok := forcedDates[date]; !ok {
			t.Fatalf("Force Recheck omitted %s", date)
		}
	}

	cnTiingo, err := applyHistorySyncPolicy(InstrumentRepairNeed{FetchRange: DateRange{Start: "2026-09-04", End: "2026-09-08"}}, domain.InstrumentHistoryCoverage{
		ProviderKey:      TiingoProviderKey,
		ProviderSymbol:   "000001.SS",
		Market:           "CN",
		CloseMarketDates: []string{"2026-09-04"},
	}, "2026-09-08", now, true)
	if err != nil {
		t.Fatal(err)
	}
	if cnTiingo.RouteStatus != domain.InstrumentRouteUnsupported || len(cnTiingo.FetchRanges) != 0 {
		t.Fatalf("CN Tiingo should not fetch or fall back: %+v", cnTiingo)
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
