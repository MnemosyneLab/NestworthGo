package application

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestCostCorrectionPreservesAcquisitionFXThroughTransfer(t *testing.T) {
	t.Parallel()
	acquired := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	corrected := acquired.Add(24 * time.Hour)
	transferred := corrected.Add(24 * time.Hour)
	householdID := domain.NewHouseholdID()
	sourceID := domain.NewHoldingID()
	household := &domain.Household{ID: householdID, BaseCurrency: "CNY"}
	snapshot := domain.PortfolioSnapshot{Household: household, FXPreferences: []domain.FXPreference{{HouseholdID: householdID, CurrencyA: "CNY", CurrencyB: "USD", SourceKind: domain.QuoteSourceManual}}}
	rate7, err := domain.ParseFxRate("7")
	if err != nil {
		t.Fatal(err)
	}
	rate8, err := domain.ParseFxRate("8")
	if err != nil {
		t.Fatal(err)
	}
	quotes := []domain.FXQuote{
		{HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: rate7, SourceKind: domain.QuoteSourceManual, QuotedAt: acquired},
		{HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: rate8, SourceKind: domain.QuoteSourceManual, QuotedAt: corrected},
	}
	quantity10 := reconciliationQuantity(t, "10")
	quantity2 := reconciliationQuantity(t, "2")
	zero := reconciliationQuantity(t, "0")
	initialCost := reconciliationUnitCost(t, "25")
	correctedCost := reconciliationUnitCost(t, "30")
	sourceEvents := []domain.CostBasisEvent{
		{Kind: domain.CostBasisStartingPoint, Quantity: quantity10, EffectiveAt: acquired},
		{Kind: domain.CostBasisCostAdjustment, Quantity: zero, UnitCost: &correctedCost, EffectiveAt: corrected},
	}
	sourceLot, err := domain.ReplayCostBasis(&initialCost, sourceEvents)
	if err != nil || sourceLot.Current.AverageUnitCost.Canonical() != "30" {
		t.Fatalf("corrected native cost=%+v err=%v", sourceLot, err)
	}
	replay := &costBasisReplayContext{events: map[domain.HoldingID][]domain.CostBasisEvent{sourceID: sourceEvents}, starting: map[domain.HoldingID]*domain.UnitPrice{sourceID: &initialCost}}
	gain := NewGainService(nil)
	sourceFX, ok := gain.acquisitionFXRate(context.Background(), snapshot, "USD", &initialCost, sourceEvents, replay, quotes, nil)
	if !ok || sourceFX.String() != "7" {
		t.Fatalf("source acquisition FX=%s available=%v, want 7", sourceFX, ok)
	}
	destinationEvents := []domain.CostBasisEvent{{Kind: domain.CostBasisTransferIn, Quantity: quantity2, UnitCost: &correctedCost, SourceHoldingID: &sourceID, EffectiveAt: transferred}}
	destinationFX, ok := gain.acquisitionFXRate(context.Background(), snapshot, "USD", nil, destinationEvents, replay, quotes, nil)
	if !ok || destinationFX.String() != "7" {
		t.Fatalf("transferred acquisition FX=%s available=%v, want 7", destinationFX, ok)
	}
}
