package marketdata

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestTiingoCompleteFixtureMapsSessionCloseNotUTCMidnight(t *testing.T) {
	meta, body := mustLoadVNext(t, "providers/tiingo/aapl-eod-complete.json")
	outcome, err := QualifyTiingoHistory(meta, body)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != application.MappingMapped {
		t.Fatalf("status = %s reason=%s", outcome.Status, outcome.Reason)
	}
	if len(outcome.Batch.Observations) != 3 {
		t.Fatalf("observations = %d", len(outcome.Batch.Observations))
	}
	first := outcome.Batch.Observations[0]
	if first.MarketDate != "2026-09-04" || first.Value != "185.25" || first.PriceBasis != application.PriceBasisTiingoRawClose {
		t.Fatalf("first observation = %+v", first)
	}
	if first.ValueEffectiveAt.UTC().Format(time.RFC3339) != "2026-09-04T20:00:00Z" {
		t.Fatalf("value_effective_at = %s", first.ValueEffectiveAt.UTC())
	}
	if first.ProviderTimestamp.UTC().Format(time.RFC3339) != "2026-09-04T00:00:00Z" {
		t.Fatalf("provider timestamp = %s", first.ProviderTimestamp.UTC())
	}
	if first.ValueEffectiveAt.Equal(first.ProviderTimestamp) {
		t.Fatal("UTC midnight date label was used as the economic close")
	}
	if len(outcome.Batch.VerifiedRanges) != 1 || outcome.Batch.VerifiedRanges[0] != (application.DateRange{Start: "2026-09-04", End: "2026-09-08"}) {
		t.Fatalf("verified = %+v", outcome.Batch.VerifiedRanges)
	}
	if len(outcome.Batch.PendingRanges) != 1 || outcome.Batch.PendingRanges[0] != (application.DateRange{Start: "2026-09-09", End: "2026-09-09"}) {
		t.Fatalf("pending = %+v", outcome.Batch.PendingRanges)
	}
}

func TestTiingoCorrectionPreservesRawCloseLexeme(t *testing.T) {
	meta, body := mustLoadVNext(t, "providers/tiingo/aapl-eod-correction.json")
	outcome, err := QualifyTiingoHistory(meta, body)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != application.MappingMapped || len(outcome.Batch.Observations) != 1 {
		t.Fatalf("outcome = %+v", outcome)
	}
	if outcome.Batch.Observations[0].Value != "185.2500037" {
		t.Fatalf("corrected close = %s", outcome.Batch.Observations[0].Value)
	}
}

func TestTiingoUnsupportedAndUncertainInputsAreExplicit(t *testing.T) {
	cases := []struct {
		path   string
		status application.MappingStatus
		reason string
	}{
		{path: "providers/tiingo/cn-equity-unsupported.json", status: application.MappingUnsupported, reason: "tiingo_us_listed_only"},
		{path: "providers/tiingo/aapl-eod-adjclose-only.json", status: application.MappingUnsupported, reason: "unsupported_price_basis"},
		{path: "providers/tiingo/aapl-eod-malformed.json", status: application.MappingInvalid, reason: "malformed_response"},
		{path: "providers/tiingo/aapl-eod-empty-pending.json", status: application.MappingPending, reason: "publication_not_ready"},
		{path: "providers/tiingo/aapl-eod-truncated.json", status: application.MappingUncertain, reason: "truncated_response"},
		{path: "providers/tiingo/aapl-eod-early-close-unknown.json", status: application.MappingUncertain, reason: "session_timing_unknown"},
	}
	for _, testCase := range cases {
		t.Run(testCase.path, func(t *testing.T) {
			meta, body := mustLoadVNext(t, testCase.path)
			outcome, err := QualifyTiingoHistory(meta, body)
			if err != nil {
				t.Fatal(err)
			}
			if outcome.Status != testCase.status || outcome.Reason != testCase.reason {
				t.Fatalf("status=%s reason=%s observations=%d", outcome.Status, outcome.Reason, len(outcome.Batch.Observations))
			}
			if outcome.Status != application.MappingMapped && outcome.Status != application.MappingUncertain {
				if testCase.status == application.MappingInvalid && len(outcome.Batch.Observations) != 0 {
					t.Fatal("invalid batch persisted observations")
				}
			}
			if testCase.status == application.MappingUnsupported && len(outcome.Batch.Observations) != 0 {
				t.Fatal("unsupported input produced a guessed price")
			}
		})
	}
}

func TestTiingoSplitAndDividendKeepRawClose(t *testing.T) {
	splitMeta, splitBody := mustLoadVNext(t, "providers/tiingo/aapl-eod-split.json")
	split, err := QualifyTiingoHistory(splitMeta, splitBody)
	if err != nil {
		t.Fatal(err)
	}
	if split.Status != application.MappingMapped || split.Batch.Observations[0].Value != "50" || split.Batch.Observations[0].SplitFactor != "4.0" {
		t.Fatalf("split = %+v", split.Batch.Observations)
	}
	divMeta, divBody := mustLoadVNext(t, "providers/tiingo/aapl-eod-dividend.json")
	div, err := QualifyTiingoHistory(divMeta, divBody)
	if err != nil {
		t.Fatal(err)
	}
	if div.Batch.Observations[0].Value != "185.25" || div.Batch.Observations[0].DividendCash != "0.25" {
		t.Fatalf("dividend used adjClose: %+v", div.Batch.Observations)
	}
}

func TestTiingoDSTFixtureKeepsDistinctCloseInstants(t *testing.T) {
	meta, body := mustLoadVNext(t, "providers/tiingo/aapl-eod-ny-dst.json")
	outcome, err := QualifyTiingoHistory(meta, body)
	if err != nil {
		t.Fatal(err)
	}
	if len(outcome.Batch.Observations) != 2 {
		t.Fatalf("observations = %d", len(outcome.Batch.Observations))
	}
	if outcome.Batch.Observations[0].ValueEffectiveAt.UTC().Format(time.RFC3339) != "2026-03-06T21:00:00Z" {
		t.Fatalf("EST = %s", outcome.Batch.Observations[0].ValueEffectiveAt.UTC())
	}
	if outcome.Batch.Observations[1].ValueEffectiveAt.UTC().Format(time.RFC3339) != "2026-03-09T20:00:00Z" {
		t.Fatalf("EDT = %s", outcome.Batch.Observations[1].ValueEffectiveAt.UTC())
	}
}

func TestYahooHistoryQualificationStaysExplicit(t *testing.T) {
	cases := []struct {
		path   string
		status application.MappingStatus
		reason string
	}{
		{path: "providers/yahoo/aapl-history-split.json", status: application.MappingUnsupported, reason: "unsupported_price_basis"},
		{path: "providers/yahoo/aapl-history-adjclose-only.json", status: application.MappingUnsupported, reason: "unsupported_price_basis"},
		{path: "providers/yahoo/aapl-history-missing-timezone.json", status: application.MappingUncertain, reason: "session_timezone_unknown"},
	}
	for _, testCase := range cases {
		t.Run(testCase.path, func(t *testing.T) {
			meta, body := mustLoadVNext(t, testCase.path)
			outcome, err := QualifyYahooHistory(meta, body)
			if err != nil {
				t.Fatal(err)
			}
			if outcome.Status != testCase.status || outcome.Reason != testCase.reason {
				t.Fatalf("status=%s reason=%s", outcome.Status, outcome.Reason)
			}
			if len(outcome.Batch.Observations) != 0 {
				t.Fatal("Yahoo history produced a guessed price")
			}
		})
	}
}

func TestFrankfurterV2MappingAndUnsupportedPairs(t *testing.T) {
	meta, body := mustLoadVNext(t, "providers/frankfurter/usd-sgd-history.json")
	outcome, err := QualifyFrankfurterHistory(meta, body)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != application.MappingMapped || len(outcome.Batch.Observations) != 3 {
		t.Fatalf("history = %+v", outcome)
	}
	first := outcome.Batch.Observations[0]
	if first.TimestampBasis != application.TimestampBasisPolicyDerived || first.Kind != application.FXObservationDailyReference {
		t.Fatalf("first FX = %+v", first)
	}
	if first.ValueEffectiveAt.UTC().Format(time.RFC3339Nano) != "2026-09-04T23:59:59.999Z" {
		t.Fatalf("policy timestamp presented as publication time? %s", first.ValueEffectiveAt.UTC())
	}
	if first.Rate != "1.35" {
		t.Fatalf("rate = %s", first.Rate)
	}

	unsupportedMeta, unsupportedBody := mustLoadVNext(t, "providers/frankfurter/usd-xxx-unsupported.json")
	unsupported, err := QualifyFrankfurterHistory(unsupportedMeta, unsupportedBody)
	if err != nil {
		t.Fatal(err)
	}
	if unsupported.Status != application.MappingUnsupported || len(unsupported.Batch.Observations) != 0 {
		t.Fatalf("unsupported pair = %+v", unsupported)
	}

	malformedMeta, malformedBody := mustLoadVNext(t, "providers/frankfurter/usd-sgd-malformed.json")
	malformed, err := QualifyFrankfurterHistory(malformedMeta, malformedBody)
	if err != nil {
		t.Fatal(err)
	}
	if malformed.Status != application.MappingInvalid || len(malformed.Batch.Observations) != 0 {
		t.Fatalf("malformed = %+v", malformed)
	}

	truncatedMeta, truncatedBody := mustLoadVNext(t, "providers/frankfurter/usd-sgd-truncated.json")
	truncated, err := QualifyFrankfurterHistory(truncatedMeta, truncatedBody)
	if err != nil {
		t.Fatal(err)
	}
	if truncated.Status != application.MappingUncertain || len(truncated.Batch.VerifiedRanges) != 0 {
		t.Fatalf("truncated = %+v", truncated)
	}
}

func TestMappedTiingoClosesMatchIndependentOracleFacts(t *testing.T) {
	scenario, err := domain.FirstVerticalSliceScenario()
	if err != nil {
		t.Fatal(err)
	}
	slice, err := domain.EvaluateMarketDataOracle(scenario)
	if err != nil {
		t.Fatal(err)
	}
	meta, body := mustLoadVNext(t, "providers/tiingo/aapl-eod-complete.json")
	outcome, err := QualifyTiingoHistory(meta, body)
	if err != nil {
		t.Fatal(err)
	}
	byDate := map[application.MarketDate]application.InstrumentDailyObservation{}
	for _, observation := range outcome.Batch.Observations {
		byDate[observation.MarketDate] = observation
	}
	friday := snapshotObservation(t, slice, "2026-09-06")
	if byDate["2026-09-04"].Value != friday.EligibleClose {
		t.Fatalf("mapped Friday close %s != oracle %s", byDate["2026-09-04"].Value, friday.EligibleClose)
	}
	if !byDate["2026-09-04"].ValueEffectiveAt.Equal(friday.ValueEffectiveAt) {
		t.Fatalf("mapped Friday instant %s != oracle %s", byDate["2026-09-04"].ValueEffectiveAt, friday.ValueEffectiveAt)
	}
}

func snapshotObservation(t *testing.T, slice domain.OracleSlice, localDate string) domain.OracleSnapshot {
	t.Helper()
	for _, snapshot := range slice.Snapshots {
		if snapshot.LocalDate == localDate {
			return snapshot
		}
	}
	t.Fatalf("missing snapshot %s", localDate)
	return domain.OracleSnapshot{}
}

func mustLoadVNext(t *testing.T, relative string) (vnextFixtureMeta, []byte) {
	t.Helper()
	meta, body, err := loadVNextFixture(relative)
	if err != nil {
		t.Fatal(err)
	}
	return meta, body
}
