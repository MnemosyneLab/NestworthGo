package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReviewPositionEpochsUseAtomicQuantityEffects(t *testing.T) {
	origin := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	instrument := domain.NewInstrumentID()
	a, b := domain.NewHoldingID(), domain.NewHoldingID()
	quantity := func(value string) *domain.Quantity {
		parsed, err := domain.ParseQuantity(value)
		if err != nil {
			t.Fatal(err)
		}
		return &parsed
	}
	effect := func(id domain.HoldingID, direction domain.EffectDirection) *domain.ActivityEffect {
		return &domain.ActivityEffect{HoldingID: &id, InstrumentID: &instrument, Direction: direction, Quantity: quantity("2")}
	}
	activity := func(day int, kind domain.ActivityKind, effects ...*domain.ActivityEffect) domain.Activity {
		at := time.Date(2026, 10, day, 12, 0, 0, 0, time.UTC)
		result := domain.Activity{ID: domain.NewActivityID(), Kind: kind, EffectiveAt: at, CreatedAt: at}
		for _, e := range effects {
			result.Effects = append(result.Effects, *e)
		}
		return result
	}
	buy := activity(2, domain.ActivityBuy, effect(a, domain.EffectAdded))
	transfer := activity(3, domain.ActivityPositionTransfer, effect(a, domain.EffectRemoved), effect(b, domain.EffectAdded))
	closeOneLeg := activity(4, domain.ActivityPositionTransfer, effect(b, domain.EffectRemoved))
	reopen := activity(5, domain.ActivityBuy, effect(b, domain.EffectAdded))
	batch := domain.HistoricalSnapshotBatch{
		Origin: domain.HistoryOrigin{StartedAt: origin},
		Portfolio: domain.PortfolioSnapshot{Holdings: []domain.Holding{
			{ID: a, InstrumentID: instrument, CreatedAt: origin.Add(time.Hour), Quantity: *quantity("0")},
			{ID: b, InstrumentID: instrument, CreatedAt: origin.Add(time.Hour), Quantity: *quantity("2")},
		}},
		Activities: []domain.Activity{reopen, closeOneLeg, transfer, buy}, // deliberately unsorted
	}
	for _, tc := range []struct {
		day        int
		quantity   string
		epochCount int
		closed     bool
	}{
		{3, "2", 1, false}, {4, "2", 1, false}, {5, "0", 1, true}, {6, "2", 2, false},
	} {
		cutoff := time.Date(2026, 10, tc.day, 0, 0, 0, 0, time.UTC)
		got, err := ProjectReviewPositionTimeline(t.Context(), batch, instrument, cutoff)
		if err != nil || !got.Known || got.Quantity != tc.quantity || len(got.Epochs) != tc.epochCount || (got.Epochs[0].ClosedAt != nil) != tc.closed && tc.epochCount == 1 {
			t.Fatalf("day %d: %+v %v", tc.day, got, err)
		}
		if tc.day == 4 && got.Epochs[0].ClosedAt != nil {
			t.Fatal("two-legged transfer closed epoch")
		}
		if tc.day == 5 && (got.Epochs[0].ClosedBy == nil || *got.Epochs[0].ClosedBy != closeOneLeg.ID) {
			t.Fatal("one-legged transfer failed to close epoch")
		}
		if tc.day == 6 && (got.Epochs[1].OpenedBy == nil || *got.Epochs[1].OpenedBy != reopen.ID) {
			t.Fatal("re-purchase did not open a fresh epoch")
		}
	}
}

func TestReviewPositionEpochCannotInventOriginQuantity(t *testing.T) {
	origin := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	instrument := domain.NewInstrumentID()
	id := domain.NewHoldingID()
	q, err := domain.ParseQuantity("2")
	if err != nil {
		t.Fatal(err)
	}
	batch := domain.HistoricalSnapshotBatch{Origin: domain.HistoryOrigin{StartedAt: origin}, Portfolio: domain.PortfolioSnapshot{Holdings: []domain.Holding{{ID: id, InstrumentID: instrument, Quantity: q, CreatedAt: origin.Add(-time.Hour)}}}}
	got, err := ProjectReviewPositionTimeline(t.Context(), batch, instrument, origin.AddDate(0, 0, 1))
	if err != nil || got.Known || got.Reason != "historical_quantity" {
		t.Fatalf("missing origin baseline: %+v %v", got, err)
	}
}
