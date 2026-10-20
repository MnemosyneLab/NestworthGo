package application

import (
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func seriesAgentSelectionFixture(t *testing.T, kind string, days ...string) (*Service, *sqlite.DB, domain.Instrument, AgentMarketDataResult) {
	t.Helper()
	ctx := t.Context()
	s, db, owner, now := overviewFixture(t)
	broker := overviewAccount(t, s, owner, "Synthetic broker", "brokerage", "asset", "holdings", "CNY", "")
	instrumentType := "stock"
	if kind == "nav" {
		instrumentType = "mutual_fund"
	}
	instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic US instrument", Type: instrumentType, MarketCode: "US", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := s.CreateHolding(ctx, HoldingInput{AccountID: broker.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendManualInstrumentQuote(ctx, instrument.ID, "100", "2026-08-01", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{holding.ID: "100"}); err != nil {
		t.Fatal(err)
	}
	// Daily coverage assertions apply after an explicit switch out of manual
	// carry-forward. Keep the manual origin for source/basis transition tests.
	*now = time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	instrument, err = s.UpdateInstrument(ctx, instrument.ID, InstrumentInput{QuoteSource: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	for _, day := range days {
		if day > "2026-08-05" {
			*now = time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
		}
	}
	items := make([]AgentMarketDataItem, 0, len(days))
	for _, day := range days {
		items = append(items, AgentMarketDataItem{InstrumentID: instrument.ID.String(), Currency: "CNY", Kind: kind, Date: day, Timezone: "UTC", Value: "100", SourceTitle: "Synthetic source"})
	}
	imported, err := s.ImportAgentMarketData(ctx, domain.NewActivityID().String(), AgentMarketDataInput{Items: items})
	if err != nil {
		t.Fatal(err)
	}
	return s, db, instrument, imported
}

func TestHistoricalSeriesAgentBoundedClosureSelection(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s, db, _, _ := seriesAgentSelectionFixture(t, "close", "2026-08-03", "2026-08-05")
	if _, err := db.SQL.Exec(`PRAGMA query_only=ON`); err != nil {
		t.Fatal(err)
	}
	panorama, err := s.HistoricalOverview(ctx, "2026-08-04", "2026-08-04")
	if err != nil {
		t.Fatal(err)
	}
	if !panorama.Left.Complete {
		t.Fatalf("bounded Agent closure did not complete panorama: %+v", panorama.Left)
	}
	wantOverviewAmount(t, panorama.Left.Assets, "100")
	series, err := s.HistoricalOverviewSeries(ctx, "2026-08-04", "2026-08-04")
	if err != nil {
		t.Fatal(err)
	}
	if !series.Points[0].AssetsComplete || !reflect.DeepEqual(panorama, series.Overview) {
		t.Fatalf("bounded Agent closure changed after series selection: point=%+v, panorama=%+v, series=%+v", series.Points[0], panorama.Left, series.Overview.Left)
	}
	wantOverviewAmount(t, series.Points[0].Assets, "100")
}

// Public entry checks use query-only SQLite and verify the entire endpoint
// bundle, including selected source IDs, completeness and missing details.
func checkSeriesAgentSelection(t *testing.T, s *Service, db *sqlite.DB, from, to string, complete []bool, amount string) HistoricalOverviewSeriesResult {
	t.Helper()
	ctx := t.Context()
	if _, err := db.SQL.Exec(`PRAGMA query_only=ON`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.SQL.Exec(`PRAGMA query_only=OFF`); err != nil {
			t.Fatal(err)
		}
	}()
	var before, after int
	if err := db.SQL.QueryRow(`SELECT total_changes()`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	panorama, err := s.HistoricalOverview(ctx, from, to)
	if err != nil {
		t.Fatal(err)
	}
	series, err := s.HistoricalOverviewSeries(ctx, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(panorama, series.Overview) {
		t.Fatalf("endpoint selection differs: panorama=%+v, series=%+v", panorama, series.Overview)
	}
	if len(series.Points) != len(complete) {
		t.Fatalf("point count=%d, want %d", len(series.Points), len(complete))
	}
	for i, point := range series.Points {
		day, err := s.HistoricalOverview(ctx, point.Date, "")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(point.Assets, day.Left.Assets) || !reflect.DeepEqual(point.NetWorth, day.Left.NetWorth) || point.AssetsComplete != day.Left.Complete || point.AssetsComplete != complete[i] {
			t.Fatalf("%s point completeness/value differs: point=%+v, panorama=%+v, complete=%v", point.Date, point, day.Left, complete[i])
		}
		wantOverviewAmount(t, point.KnownAssets, amount)
		if complete[i] {
			wantOverviewAmount(t, point.Assets, amount)
			wantOverviewAmount(t, point.NetWorth, amount)
			if point.AssetCoverage.Missing != 0 || len(point.Missing) != 0 {
				t.Fatalf("complete point retained missing coverage: %+v", point)
			}
		} else if point.Assets != nil || point.NetWorth != nil || point.AssetCoverage.Missing != 1 || len(point.Missing) != 1 || !reflect.DeepEqual(point.Missing[0].Reasons, []string{string(domain.MissingHistoryCoverage)}) {
			t.Fatalf("incomplete carry was promoted to complete: %+v", point)
		}
	}
	if err := db.SQL.QueryRow(`SELECT total_changes()`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("capture wrote data: changes=%d -> %d", before, after)
	}
	return series
}

func TestHistoricalSeriesAgentSelectionCoverageBoundaries(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, kind, from, to string
		days                 []string
		complete             []bool
	}{
		{name: "dated Agent", kind: "close", from: "2026-08-03", to: "2026-08-03", days: []string{"2026-08-03"}, complete: []bool{true}},
		{name: "bounded weekday closure", kind: "close", from: "2026-08-03", to: "2026-08-05", days: []string{"2026-08-03", "2026-08-05"}, complete: []bool{true, true, true}},
		{name: "unbounded weekday tail", kind: "close", from: "2026-08-03", to: "2026-08-05", days: []string{"2026-08-03"}, complete: []bool{true, false, false}},
		{name: "equity weekend", kind: "close", from: "2026-08-07", to: "2026-08-09", days: []string{"2026-08-07"}, complete: []bool{true, true, true}},
		{name: "NAV weekday carry", kind: "nav", from: "2026-08-03", to: "2026-08-05", days: []string{"2026-08-03", "2026-08-05"}, complete: []bool{true, false, true}},
		{name: "NAV weekend carry", kind: "nav", from: "2026-08-07", to: "2026-08-09", days: []string{"2026-08-07"}, complete: []bool{true, false, false}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, db, _, _ := seriesAgentSelectionFixture(t, test.kind, test.days...)
			checkSeriesAgentSelection(t, s, db, test.from, test.to, test.complete, "100")
		})
	}
}

func TestHistoricalSeriesAgentSelectionCorrectionRetractionRecapture(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s, db, instrument, imported := seriesAgentSelectionFixture(t, "close", "2026-08-03", "2026-08-05")
	initial := checkSeriesAgentSelection(t, s, db, "2026-08-04", "2026-08-04", []bool{true}, "100")
	corrected, err := s.ImportAgentMarketData(ctx, domain.NewActivityID().String(), AgentMarketDataInput{Items: []AgentMarketDataItem{{
		Operation: "correct", TargetQuoteID: imported.QuoteIDs[0], InstrumentID: instrument.ID.String(), Currency: "CNY", Kind: "close", Date: "2026-08-03", Timezone: "UTC", Value: "120", SourceTitle: "Synthetic correction",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	updated := checkSeriesAgentSelection(t, s, db, "2026-08-04", "2026-08-04", []bool{true}, "120")
	if initial.Overview.InputGeneration == updated.Overview.InputGeneration {
		t.Fatal("correction did not advance capture generation")
	}
	for _, row := range updated.Overview.Rows {
		if row.Kind == "holding" && (row.Left.Price == nil || row.Left.Price.ID != corrected.QuoteIDs[0]) {
			t.Fatalf("correction identity lost: %+v", row.Left.Price)
		}
	}
	if _, err := s.ImportAgentMarketData(ctx, domain.NewActivityID().String(), AgentMarketDataInput{Items: []AgentMarketDataItem{{
		Operation: "retract", TargetQuoteID: imported.QuoteIDs[1], SourceTitle: "Synthetic right-hand close withdrawal",
	}}}); err != nil {
		t.Fatal(err)
	}
	withdrawn := checkSeriesAgentSelection(t, s, db, "2026-08-04", "2026-08-04", []bool{false}, "120")
	if updated.Overview.InputGeneration == withdrawn.Overview.InputGeneration {
		t.Fatal("retraction did not advance capture generation")
	}
	// Previously returned captures keep their original amounts and coverage.
	wantOverviewAmount(t, initial.Points[0].Assets, "100")
	wantOverviewAmount(t, updated.Points[0].Assets, "120")
}
