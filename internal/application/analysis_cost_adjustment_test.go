package application

import (
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestCostOnlyEffectDoesNotBecomeAssetChange(t *testing.T) {
	t.Parallel()
	accountID := domain.NewAccountID()
	holdingID := domain.NewHoldingID()
	instrumentID := domain.NewInstrumentID()
	unitCost := reconciliationUnitCost(t, "30")
	effect := domain.ActivityEffect{Target: domain.EffectTargetHoldingCost, HoldingID: &holdingID, InstrumentID: &instrumentID, CostUnitPrice: &unitCost, Classification: domain.ClassificationRemeasurement, Direction: domain.EffectAdded}
	accounts := map[domain.AccountID]domain.Account{accountID: {ID: accountID}}
	holdings := map[domain.HoldingID]domain.Holding{holdingID: {ID: holdingID, AccountID: accountID, InstrumentID: instrumentID}}
	instruments := map[domain.InstrumentID]domain.Instrument{instrumentID: {ID: instrumentID, Type: domain.InstrumentETF, QuoteCurrency: "USD"}}
	component, ok := componentForEffect(effect, accounts, holdings, instruments)
	if !ok {
		t.Fatal("fixture cost effect does not map to its Holding component")
	}
	universe := analysisUniverse{accounts: accounts, holdings: holdings, instruments: instruments, componentKeys: map[string]struct{}{component.Key(): {}}, investmentComponentKeys: map[string]struct{}{component.Key(): {}}}
	classified, err := universe.classifyActivity(domain.Activity{Kind: domain.ActivityCostAdjustment, Effects: []domain.ActivityEffect{effect}}, domain.DailyValuationSnapshot{}, nil, AnalysisInputs{}, domain.AnalysisQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(classified) != 0 {
		t.Fatalf("cost correction created asset flows: %+v", classified)
	}
}
