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
	if before.FetchRange.Start != "2026-08-30" || before.FetchRange.End != "2026-09-08" {
		t.Fatalf("fetch range = %+v, want 2026-08-30..2026-09-08 (7d opening-anchor window through last finalized US date)", before.FetchRange)
	}
	if before.LastFinalizedMarketDate != "2026-09-08" {
		t.Fatalf("last finalized market date = %s, want 2026-09-08", before.LastFinalizedMarketDate)
	}
	if before.OpeningAnchorWindowDays != 7 || before.OpeningAnchorExhausted {
		t.Fatalf("opening-anchor search state = %+v, want first 7d window", before)
	}
	firstSync, err := applyHistorySyncPolicy(before, domain.InstrumentHistoryCoverage{
		ProviderKey:    TiingoProviderKey,
		ProviderSymbol: "AAPL",
		Market:         "US",
	}, "2026-09-08", time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstSync.FetchRanges) != 1 || firstSync.FetchRanges[0].Start != "2026-08-30" || firstSync.FetchRanges[0].End != "2026-09-08" {
		t.Fatalf("first staged fetch = %+v, want only 7d anchor window plus required interval", firstSync.FetchRanges)
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

func TestInstrumentHistoryTasksPreserveQuoteCurrencyAndMarket(t *testing.T) {
	service := &Service{historyRequestMaxDays: 366}
	need := InstrumentRepairNeed{
		InstrumentID:            domain.NewInstrumentID(),
		ProviderKey:             domain.YahooFinanceProviderKey,
		ProviderSymbol:          "600519.SS",
		Market:                  "CN",
		QuoteCurrency:           "CNY",
		LastFinalizedMarketDate: "2026-09-09",
		RouteStatus:             domain.InstrumentRouteOK,
		FetchRanges:             []DateRange{{Start: "2026-09-09", End: "2026-09-09"}},
	}
	tasks := service.instrumentHistoryTasksFromNeeds([]InstrumentRepairNeed{need})
	if len(tasks) != 1 {
		t.Fatalf("history tasks = %d, want one", len(tasks))
	}
	if tasks[0].identity.ProviderSymbol != "600519.SS" || tasks[0].identity.QuoteCurrency != "CNY" || tasks[0].identity.Market != "CN" {
		t.Fatalf("history task identity = %+v", tasks[0].identity)
	}
}

func TestPlanInstrumentRepairNeedWidensOpeningAnchorSearchAndStops(t *testing.T) {
	coverage := domain.InstrumentHistoryCoverage{
		ProviderKey:    TiingoProviderKey,
		ProviderSymbol: "AAPL",
		Market:         "US",
	}
	appendCoverageDates(t, &coverage.NoObservationDates, "2026-08-30", "2026-09-05")

	widened, err := planInstrumentRepairNeed(coverage, "2026-09-06", "2026-09-08")
	if err != nil {
		t.Fatal(err)
	}
	if widened.OpeningAnchorWindowDays != 30 || widened.OpeningAnchorExhausted {
		t.Fatalf("opening-anchor search state = %+v, want second 30d window", widened)
	}
	if widened.FetchRange.Start != "2026-08-07" {
		t.Fatalf("widened fetch start = %s, want 2026-08-07", widened.FetchRange.Start)
	}

	appendCoverageDates(t, &coverage.NoObservationDates, "2026-08-07", "2026-08-29")
	exhausted, err := planInstrumentRepairNeed(coverage, "2026-09-06", "2026-09-08")
	if err != nil {
		t.Fatal(err)
	}
	if exhausted.OpeningAnchorWindowDays != 365 || exhausted.OpeningAnchorExhausted {
		t.Fatalf("opening-anchor search state = %+v, want third 365d window", exhausted)
	}
	appendCoverageDates(t, &coverage.NoObservationDates, "2025-09-06", "2026-08-06")
	exhausted, err = planInstrumentRepairNeed(coverage, "2026-09-06", "2026-09-08")
	if err != nil {
		t.Fatal(err)
	}
	if exhausted.OpeningAnchorWindowDays != 365 || !exhausted.OpeningAnchorExhausted {
		t.Fatalf("opening-anchor search state = %+v, want exhausted 365d window", exhausted)
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

func appendCoverageDates(t *testing.T, target *[]string, start, end string) {
	t.Helper()
	dates, err := domain.InclusiveMarketDates(start, end)
	if err != nil {
		t.Fatal(err)
	}
	for _, date := range dates {
		*target = append(*target, date)
	}
}
