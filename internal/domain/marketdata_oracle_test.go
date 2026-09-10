package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFirstVerticalSliceOracleHasIndependentExpectedAmounts(t *testing.T) {
	scenario, err := FirstVerticalSliceScenario()
	if err != nil {
		t.Fatal(err)
	}
	slice, err := EvaluateMarketDataOracle(scenario)
	if err != nil {
		t.Fatal(err)
	}
	if slice.ScenarioID != FirstVerticalSliceScenarioID || slice.ResolverPolicy != MarketDataResolverPolicy {
		t.Fatalf("identity = %+v", slice)
	}
	if slice.HouseholdTimezone != "Asia/Singapore" || slice.TodayLocal != "2026-09-10" {
		t.Fatalf("clock = tz=%s today=%s", slice.HouseholdTimezone, slice.TodayLocal)
	}
	if slice.OpeningAnchorDate != "2026-09-04" || slice.OpeningAnchorMissing {
		t.Fatalf("opening anchor = %s missing=%v", slice.OpeningAnchorDate, slice.OpeningAnchorMissing)
	}
	if slice.LiveEligibleMarketDate != "2026-09-08" || !slice.LiveCarriedForward {
		t.Fatalf("live = date=%s carried=%v", slice.LiveEligibleMarketDate, slice.LiveCarriedForward)
	}
	if got := snapshotByDate(t, slice, "2026-09-06"); got.EligibleMarketDate != "2026-09-04" || got.EligibleClose != "185.25" || !got.CarriedForward || got.AssetsRounded != "3850.875" {
		t.Fatalf("Sunday snapshot = %+v", got)
	}
	if got := snapshotByDate(t, slice, "2026-09-07"); got.EligibleMarketDate != "2026-09-04" || got.AssetsRounded != "3850.875" {
		t.Fatalf("Monday snapshot used a later US close: %+v", got)
	}
	if got := snapshotByDate(t, slice, "2026-09-08"); got.EligibleMarketDate != "2026-09-07" || got.EligibleClose != "186" || got.AssetsRounded != "3861" {
		t.Fatalf("Tuesday snapshot = %+v", got)
	}
	wednesday := snapshotByDate(t, slice, "2026-09-09")
	if wednesday.EligibleMarketDate != "2026-09-08" || wednesday.EligibleRevision != 2 || wednesday.EligibleClose != "185.2500037" {
		t.Fatalf("Wednesday snapshot consumed an in-progress US close: %+v", wednesday)
	}
	if wednesday.AssetsRounded != "3850.875" {
		t.Fatalf("Wednesday rounded amount = %s", wednesday.AssetsRounded)
	}
	if wednesday.NativeHoldingExact == "1852.5" {
		t.Fatal("correction did not change the exact native holding")
	}
	if !slice.Correction.RoundedAmountUnchanged || !slice.Correction.ExactHoldingChanged || !slice.Correction.NewRevisionRequired {
		t.Fatalf("correction = %+v", slice.Correction)
	}
	if slice.Correction.OriginalRounded != slice.Correction.CorrectedRounded {
		t.Fatalf("rounded amounts diverged: %s vs %s", slice.Correction.OriginalRounded, slice.Correction.CorrectedRounded)
	}
	if len(slice.PendingMarketDates) != 1 || slice.PendingMarketDates[0] != "2026-09-09" {
		t.Fatalf("pending = %v", slice.PendingMarketDates)
	}
	sep9Close, err := ResolveEquitySessionClose("2026-09-09", "US", SessionEvidence{Kind: SessionKindRegular, Timezone: USEquitySessionTimezone, CloseClock: USEquityRegularCloseClock})
	if err != nil {
		t.Fatal(err)
	}
	if ObservationEligible(sep9Close.CloseInstant, wednesday.CutoffAt) {
		t.Fatal("closed Singapore Sep 9 snapshot is eligible for the later US close")
	}
}

func TestFirstVerticalSliceFixtureMatchesOracleFacts(t *testing.T) {
	scenario, err := FirstVerticalSliceScenario()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "market-data", "vnext", "scenarios", "e2e-tiingo-us-manual-fx-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		ID                 string   `json:"id"`
		Clock              string   `json:"clock"`
		HouseholdTimezone  string   `json:"householdTimezone"`
		BaseCurrency       string   `json:"baseCurrency"`
		StartingPointLocal string   `json:"startingPointLocal"`
		PendingMarketDates []string `json:"pendingMarketDates"`
		Holding            struct {
			Quantity      string `json:"quantity"`
			QuoteCurrency string `json:"quoteCurrency"`
		} `json:"holding"`
		Cash struct {
			Amount   string `json:"amount"`
			Currency string `json:"currency"`
		} `json:"cash"`
		ManualFX struct {
			Base  string `json:"base"`
			Quote string `json:"quote"`
			Rate  string `json:"rate"`
		} `json:"manualFx"`
		InstrumentCloses []struct {
			MarketDate         string `json:"marketDate"`
			Close              string `json:"close"`
			SessionTimezone    string `json:"sessionTimezone"`
			CloseClock         string `json:"closeClock"`
			SessionKind        string `json:"sessionKind"`
			Revision           int    `json:"revision"`
			SupersedesRevision int    `json:"supersedesRevision"`
		} `json:"instrumentCloses"`
	}
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.ID != scenario.ID || fixture.HouseholdTimezone != scenario.Clock.HouseholdTimezone || fixture.BaseCurrency != scenario.BaseCurrency.String() {
		t.Fatalf("fixture identity drifted from oracle: %+v", fixture)
	}
	clock, err := time.Parse(time.RFC3339, fixture.Clock)
	if err != nil || !clock.Equal(scenario.Clock.Now.In(clock.Location())) {
		t.Fatalf("fixture clock = %s oracle = %s err=%v", fixture.Clock, scenario.Clock.Now, err)
	}
	start, err := time.Parse(time.RFC3339, fixture.StartingPointLocal)
	if err != nil || !start.Equal(scenario.StartingPointLocal) {
		t.Fatalf("starting point = %s oracle = %s err=%v", fixture.StartingPointLocal, scenario.StartingPointLocal, err)
	}
	if fixture.Holding.Quantity != scenario.Holding.Quantity || fixture.Holding.QuoteCurrency != scenario.Holding.QuoteCurrency.String() {
		t.Fatalf("holding drifted: %+v vs %+v", fixture.Holding, scenario.Holding)
	}
	if fixture.Cash.Amount != scenario.Cash.Amount || fixture.Cash.Currency != scenario.Cash.Currency.String() {
		t.Fatalf("cash drifted: %+v vs %+v", fixture.Cash, scenario.Cash)
	}
	if fixture.ManualFX.Base != scenario.FX.BaseCurrency.String() || fixture.ManualFX.Quote != scenario.FX.QuoteCurrency.String() || fixture.ManualFX.Rate != scenario.FX.Rate {
		t.Fatalf("fx drifted: %+v vs %+v", fixture.ManualFX, scenario.FX)
	}
	if len(fixture.InstrumentCloses) != len(scenario.Closes) || len(fixture.PendingMarketDates) != len(scenario.PendingMarketDates) {
		t.Fatalf("close/pending counts json=%d/%d go=%d/%d", len(fixture.InstrumentCloses), len(fixture.PendingMarketDates), len(scenario.Closes), len(scenario.PendingMarketDates))
	}
	for i, close := range fixture.InstrumentCloses {
		want := scenario.Closes[i]
		if close.MarketDate != want.MarketDate || close.Close != want.Close || close.SessionTimezone != want.SessionTimezone || close.CloseClock != want.CloseClock || close.SessionKind != string(want.SessionKind) || close.Revision != want.Revision || close.SupersedesRevision != want.SupersedesRevision {
			t.Fatalf("close[%d] json=%+v go=%+v", i, close, want)
		}
	}
	for i, pending := range fixture.PendingMarketDates {
		if pending != scenario.PendingMarketDates[i] {
			t.Fatalf("pending[%d] json=%s go=%s", i, pending, scenario.PendingMarketDates[i])
		}
	}
}

func snapshotByDate(t *testing.T, slice OracleSlice, localDate string) OracleSnapshot {
	t.Helper()
	for _, snapshot := range slice.Snapshots {
		if snapshot.LocalDate == localDate {
			return snapshot
		}
	}
	t.Fatalf("snapshot %s missing from %v", localDate, slice.ClosedLocalDates)
	return OracleSnapshot{}
}
