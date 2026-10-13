package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func (f *accountingGainFixture) requireFXCost(t *testing.T, index int, nativeCost, baseCost, currencyMovement string) {
	t.Helper()
	view, err := f.service.HoldingGain(t.Context(), f.holdings[index].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !view.Available || view.TotalCost.Currency != "USD" || view.TotalCost.Amount != nativeCost || view.TotalCostBase == nil || view.TotalCostBase.Currency != "CNY" || view.TotalCostBase.Amount != baseCost || view.InstrumentMovement == nil || view.InstrumentMovement.Amount != "0" || view.CurrencyMovement == nil || view.CurrencyMovement.Amount != currencyMovement || view.UnrealizedGainBase == nil || view.UnrealizedGainBase.Amount != currencyMovement {
		t.Fatalf("holding %d FX cost/decomposition=%+v", index, view)
	}
}

func TestAccountingForeignFullTransferChainAndRoundTripPreservesAcquisitionFX(t *testing.T) {
	t.Parallel()
	f := newAccountingGainFixtureCurrency(t, "3", "10", "USD")
	effective := f.now
	if _, err := f.service.AppendManualFXQuote(t.Context(), "USD", "CNY", "7", effective.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	f.transfer(t, 0, 1, "3", effective)
	f.requireFXCost(t, 1, "30", "180", "30")
	f.now = f.now.Add(time.Minute)
	f.transfer(t, 1, 2, "3", effective)
	f.requireFXCost(t, 2, "30", "180", "30")
	f.now = f.now.Add(time.Minute)
	f.transfer(t, 2, 0, "3", effective)
	f.requireFXCost(t, 0, "30", "180", "30")
	f.requireInstrumentCost(t, "3", "30")
}

func TestAccountingForeignTransferExcludesLaterSameTimeBuyFromAcquisitionFX(t *testing.T) {
	t.Parallel()
	f := newAccountingGainFixtureCurrency(t, "3", "10", "USD")
	effective := f.now
	if _, err := f.service.AppendManualFXQuote(t.Context(), "USD", "CNY", "7", effective.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	f.transfer(t, 0, 1, "1", effective)
	f.requireFXCost(t, 1, "10", "60", "10")
	f.now = f.now.Add(time.Minute)
	buy := domain.TradeInput{HouseholdID: f.boot.Household.ID, Side: domain.TradeBuy, SettlementAccountID: f.accounts[0].Account.ID, HoldingID: f.holdings[0].ID, InstrumentID: f.instrument.ID, Quantity: mustQuantity(t, "1"), Gross: mustMoney(t, "10", "USD"), EffectiveAt: effective}
	if _, err := f.service.RecordChange(t.Context(), buy); err != nil {
		t.Fatal(err)
	}
	f.requireFXCost(t, 1, "10", "60", "10")
	f.requireFXCost(t, 0, "30", "190", "20")
}

func TestAccountingEqualEffectiveAndCreatedTimesUseCorrectionRootID(t *testing.T) {
	t.Parallel()
	f := newAccountingGainFixture(t, "0", "10")
	effective := f.now
	buy := domain.TradeInput{HouseholdID: f.boot.Household.ID, Side: domain.TradeBuy, SettlementAccountID: f.accounts[0].Account.ID, HoldingID: f.holdings[0].ID, InstrumentID: f.instrument.ID, Quantity: mustQuantity(t, "3"), Gross: mustMoney(t, "30", "CNY"), EffectiveAt: effective}
	bought, err := f.service.RecordChange(t.Context(), buy)
	if err != nil {
		t.Fatal(err)
	}
	// Leave the synthetic audit clock fixed: SQL must use the UUID tie-break.
	transfer := f.transfer(t, 0, 1, "1", effective)
	adjustment := domain.PositionCostAdjustmentInput{HouseholdID: f.boot.Household.ID, HoldingID: f.holdings[0].ID, UnitCost: reconciliationUnitCost(t, "20"), EffectiveAt: effective}
	adjusted, err := f.service.RecordChange(t.Context(), adjustment)
	if err != nil {
		t.Fatal(err)
	}
	if !bought.Activity.CreatedAt.Equal(transfer.Activity.CreatedAt) || !transfer.Activity.CreatedAt.Equal(adjusted.Activity.CreatedAt) || bought.Activity.ID.String() >= transfer.Activity.ID.String() || transfer.Activity.ID.String() >= adjusted.Activity.ID.String() {
		t.Fatal("fixture does not exercise equal-time root ID ordering")
	}
	f.requireCost(t, 1, "1", "10")
	f.requireCost(t, 0, "2", "40")
	f.now = f.now.Add(time.Minute)
	buy.Gross = mustMoney(t, "36", "CNY")
	fixed, err := f.service.FixChange(t.Context(), bought.Activity.ID, buy)
	if err != nil {
		t.Fatal(err)
	}
	events, err := f.repo.ListCostBasisEvents(t.Context(), f.holdings[0].ID, domain.CostBasisReadFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].ActivityID != fixed.Activity.ID || events[0].OrderingID != bought.Activity.ID || !events[0].CreatedAt.Equal(events[1].CreatedAt) || !events[1].CreatedAt.Equal(events[2].CreatedAt) {
		t.Fatalf("SQL correction-aware tie order=%+v", events)
	}
	f.requireCost(t, 1, "1", "12")
	f.requireCost(t, 0, "2", "40")
}
