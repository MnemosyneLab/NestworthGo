package application

import (
	"slices"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestHoldingAmountGapSurvivesSnapshotPersistence(t *testing.T) {
	t.Parallel()
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "2")
	f.day(t, "2026-08-02")
	// Supersede the fixture's noon manual quote within the same closed day.
	quote, err := f.s.AppendManualInstrumentQuote(t.Context(), f.instrument.ID, "999999999999", "2026-08-01T18:00:00Z", false)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _, err := f.s.BuildDailyValuationSnapshot(t.Context(), "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Complete {
		t.Fatal("out-of-range holding produced a complete snapshot")
	}
	stored, err := f.s.repository.ListDailyValuationSnapshots(t.Context(), f.account.Account.HouseholdID, time.Time{}, time.Time{})
	if err != nil || len(stored) != 1 {
		t.Fatalf("stored snapshots = %+v, err=%v", stored, err)
	}
	var found bool
	for _, item := range stored[0].Items {
		if item.HoldingID == nil || *item.HoldingID != f.holding.ID {
			continue
		}
		found = true
		if item.Complete || item.NativeAmount != "" || item.BaseAmount != nil || item.BaseAmountExact != "" || item.QuoteID == nil || *item.QuoteID != quote.ID.String() || item.MissingReason == nil || *item.MissingReason != "holding amount exceeds the supported range" {
			t.Fatalf("stored overflow lost its diagnosis or quote evidence: %+v", item)
		}
		component, missing, _ := snapshotDisplayItem(item, snapshotDisplayEvidence{prices: map[string]domain.QuoteEvidenceView{quote.ID.String(): {ObservationID: quote.ID.String(), Source: domain.QuoteSourceManual}}})
		if component.PriceEvidence == nil || len(missing) != 1 || missing[0].Kind != domain.MissingHoldingAmount {
			t.Fatalf("reloaded overflow = %+v, missing=%+v", component, missing)
		}
		var display displayAccumulator
		if err := display.add(component, missing, "2026-08-01", false); err != nil {
			t.Fatal(err)
		}
		amount, err := display.amount("CNY", false)
		if err != nil || amount.Total != nil || amount.KnownSubtotal != nil || amount.Quality.UnpricedCount != 1 || !slices.Contains(amount.Quality.Reasons, domain.ReasonEvidenceUnavailable) || slices.Contains(amount.Quality.Reasons, domain.ReasonPriceMissing) {
			t.Fatalf("reloaded display = %+v, err=%v", amount, err)
		}
	}
	if !found {
		t.Fatal("stored holding is missing")
	}
}

func TestHoldingAmountSnapshotReasonPreservesCoverageAndLegacyGaps(t *testing.T) {
	t.Parallel()
	id := domain.NewInstrumentID()
	component := domain.ValuationComponent{InstrumentID: &id}
	for _, kinds := range [][]domain.MissingInputKind{
		{domain.MissingHoldingAmount},
		{domain.MissingHistoryCoverage, domain.MissingHoldingAmount},
		{domain.MissingHoldingAmount, domain.MissingHistoryCoverage},
	} {
		missing := make([]domain.MissingInputView, len(kinds))
		for i, kind := range kinds {
			missing[i].Kind = kind
		}
		reason := missingReason(component, missing)
		_, decoded, _ := snapshotDisplayItem(domain.DailyValuationSnapshotItem{InstrumentID: &id, MissingReason: &reason}, snapshotDisplayEvidence{})
		if len(decoded) != len(missing) || decoded[0].Kind != domain.MissingHoldingAmount || (len(kinds) > 1 && decoded[1].Kind != domain.MissingHistoryCoverage) {
			t.Errorf("kinds=%v reason=%q decoded=%+v", kinds, reason, decoded)
		}
	}

	for _, tc := range []struct {
		reason string
		native string
		kind   domain.MissingInputKind
	}{
		{"missing instrument price or FX rate", "", domain.MissingInstrumentPrice},
		{"missing instrument price or FX rate", "2", domain.MissingFXRate},
		{"historical market-data coverage is unverified", "", domain.MissingInstrumentPrice},
	} {
		_, missing, _ := snapshotDisplayItem(domain.DailyValuationSnapshotItem{InstrumentID: &id, NativeAmount: tc.native, MissingReason: &tc.reason}, snapshotDisplayEvidence{})
		if len(missing) == 0 || missing[0].Kind != tc.kind {
			t.Errorf("legacy reason=%q native=%q decoded=%+v", tc.reason, tc.native, missing)
		}
	}
}
