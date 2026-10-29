package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// A and B own the same USD instrument at different native costs and FX rates.
// An internal move must carry acquisition cost, irrespective of share price.
func newMixedAcquisitionCostFixture(t *testing.T) *accountingGainFixture {
	t.Helper()
	f := newAccountingGainFixtureCurrency(t, "10", "10", "USD")
	for _, quote := range []struct {
		rate string
		at   time.Time
	}{{"7", f.now.Add(-time.Hour)}, {"8", f.now}} {
		if _, err := f.service.AppendManualFXQuote(t.Context(), "USD", "CNY", quote.rate, quote.at.Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.service.RecordChange(t.Context(), domain.TradeInput{
		HouseholdID: f.boot.Household.ID, Side: domain.TradeBuy,
		SettlementAccountID: f.accounts[1].Account.ID, HoldingID: f.holdings[1].ID,
		InstrumentID: f.instrument.ID, Quantity: mustQuantity(t, "10"),
		Gross: mustMoney(t, "200", "USD"), EffectiveAt: f.now,
	}); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	return f
}

func requireAcquisitionCost(t *testing.T, f *accountingGainFixture, index int, quantity, native, base string) domain.HoldingGainView {
	t.Helper()
	view, err := f.service.HoldingGain(t.Context(), f.holdings[index].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !view.Available || view.Quantity != quantity || view.TotalCost.Amount != native || view.TotalCostBase == nil || view.TotalCostBase.Amount != base {
		t.Fatalf("holding %d acquisition cost=%+v; want quantity=%s native=%s base=%s", index, view, quantity, native, base)
	}
	return view
}

func TestGainTransferConservesMixedAcquisitionCosts(t *testing.T) {
	t.Parallel()
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "full merge", true: "partial multihop round trip"}[partial], func(t *testing.T) {
			f := newMixedAcquisitionCostFixture(t)
			requireAcquisitionCost(t, f, 0, "10", "100", "700")
			requireAcquisitionCost(t, f, 1, "10", "200", "1600")
			effective := f.now
			if partial {
				f.transfer(t, 0, 1, "4", effective)
				requireAcquisitionCost(t, f, 0, "6", "60", "420")
				requireAcquisitionCost(t, f, 1, "14", "240", "1880")
				f.now = f.now.Add(time.Minute)
				f.transfer(t, 1, 2, "7", effective)
				requireAcquisitionCost(t, f, 1, "7", "120", "940")
				requireAcquisitionCost(t, f, 2, "7", "120", "940")
				f.now = f.now.Add(time.Minute)
				f.transfer(t, 2, 0, "7", effective)
				requireAcquisitionCost(t, f, 0, "13", "180", "1360")
				f.now = f.now.Add(time.Minute)
				f.transfer(t, 0, 1, "13", effective)
			} else {
				f.transfer(t, 0, 1, "10", effective)
			}
			requireAcquisitionCost(t, f, 0, "0", "0", "0")
			requireAcquisitionCost(t, f, 1, "20", "300", "2300")
			requireAcquisitionCost(t, f, 2, "0", "0", "0")
		})
	}
}

func TestGainTransferCostCorrectionAndSalesPreserveAcquisitionRatio(t *testing.T) {
	t.Parallel()
	f := newMixedAcquisitionCostFixture(t)
	f.transfer(t, 0, 1, "10", f.now)
	f.now = f.now.Add(time.Minute)
	if _, err := f.service.RecordChange(t.Context(), domain.PositionCostAdjustmentInput{
		HouseholdID: f.boot.Household.ID, HoldingID: f.holdings[1].ID,
		UnitCost: reconciliationUnitCost(t, "18"), EffectiveAt: f.now,
	}); err != nil {
		t.Fatal(err)
	}
	// A native cost correction preserves 2300/300 acquisition FX provenance.
	requireAcquisitionCost(t, f, 1, "20", "360", "2760")
	f.now = f.now.Add(time.Minute)
	f.transfer(t, 1, 2, "10", f.now)
	requireAcquisitionCost(t, f, 1, "10", "180", "1380")
	requireAcquisitionCost(t, f, 2, "10", "180", "1380")
	f.now = f.now.Add(time.Minute)
	fee := mustMoney(t, "2", "USD")
	sale := domain.TradeInput{HouseholdID: f.boot.Household.ID, Side: domain.TradeSell,
		SettlementAccountID: f.accounts[2].Account.ID, HoldingID: f.holdings[2].ID,
		InstrumentID: f.instrument.ID, Quantity: mustQuantity(t, "4"),
		Gross: mustMoney(t, "80", "USD"), Fee: &fee, EffectiveAt: f.now}
	if _, err := f.service.RecordChange(t.Context(), sale); err != nil {
		t.Fatal(err)
	}
	if view := requireAcquisitionCost(t, f, 2, "6", "108", "828"); view.RealizedGain.Amount != "6" {
		t.Fatalf("partial-sale realized gain=%+v; want 6 USD", view.RealizedGain)
	}
	f.now = f.now.Add(time.Minute)
	f.transfer(t, 2, 1, "6", f.now)
	requireAcquisitionCost(t, f, 1, "16", "288", "2208")
	f.now = f.now.Add(time.Minute)
	sale.SettlementAccountID, sale.HoldingID = f.accounts[1].Account.ID, f.holdings[1].ID
	sale.Quantity, sale.Gross, sale.Fee, sale.EffectiveAt = mustQuantity(t, "16"), mustMoney(t, "320", "USD"), nil, f.now
	if _, err := f.service.RecordChange(t.Context(), sale); err != nil {
		t.Fatal(err)
	}
	if view := requireAcquisitionCost(t, f, 1, "0", "0", "0"); view.RealizedGain.Amount != "32" {
		t.Fatalf("full-sale realized gain=%+v; want 32 USD", view.RealizedGain)
	}
}

func TestGainCorrectedTransferUsesOriginalAcquisitionBoundary(t *testing.T) {
	t.Parallel()
	f := newMixedAcquisitionCostFixture(t)
	effective := f.now
	transfer := f.transfer(t, 0, 1, "4", effective)
	f.now = f.now.Add(time.Minute)
	if _, err := f.service.RecordChange(t.Context(), domain.PositionCostAdjustmentInput{
		HouseholdID: f.boot.Household.ID, HoldingID: f.holdings[0].ID,
		UnitCost: reconciliationUnitCost(t, "20"), EffectiveAt: effective,
	}); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	if _, err := f.service.FixChange(t.Context(), transfer.Activity.ID, domain.PositionTransferInput{
		HouseholdID: f.boot.Household.ID, FromHoldingID: f.holdings[0].ID,
		ToHoldingID: f.holdings[1].ID, Quantity: mustQuantity(t, "6"), EffectiveAt: effective,
	}); err != nil {
		t.Fatal(err)
	}
	requireAcquisitionCost(t, f, 0, "4", "80", "560")
	requireAcquisitionCost(t, f, 1, "16", "260", "2020")
}

func TestGainTransferAcquisitionIncludesFeesAndResetsAfterFullSale(t *testing.T) {
	t.Parallel()
	f := newAccountingGainFixtureCurrency(t, "0", "10", "USD")
	buy := func(index int, quantity, gross, feeAmount, rate string) {
		t.Helper()
		f.now = f.now.Add(time.Minute)
		if _, err := f.service.AppendManualFXQuote(t.Context(), "USD", "CNY", rate, f.now.Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
		fee := mustMoney(t, feeAmount, "USD")
		if _, err := f.service.RecordChange(t.Context(), domain.TradeInput{
			HouseholdID: f.boot.Household.ID, Side: domain.TradeBuy,
			SettlementAccountID: f.accounts[index].Account.ID, HoldingID: f.holdings[index].ID,
			InstrumentID: f.instrument.ID, Quantity: mustQuantity(t, quantity),
			Gross: mustMoney(t, gross, "USD"), Fee: &fee, EffectiveAt: f.now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	buy(0, "10", "90", "10", "7")
	buy(0, "10", "200", "0", "8")
	requireAcquisitionCost(t, f, 0, "20", "300", "2300")
	f.now = f.now.Add(time.Minute)
	f.transfer(t, 0, 1, "20", f.now)
	f.now = f.now.Add(time.Minute)
	if _, err := f.service.RecordChange(t.Context(), domain.TradeInput{
		HouseholdID: f.boot.Household.ID, Side: domain.TradeSell,
		SettlementAccountID: f.accounts[1].Account.ID, HoldingID: f.holdings[1].ID,
		InstrumentID: f.instrument.ID, Quantity: mustQuantity(t, "20"),
		Gross: mustMoney(t, "300", "USD"), EffectiveAt: f.now,
	}); err != nil {
		t.Fatal(err)
	}
	requireAcquisitionCost(t, f, 1, "0", "0", "0")
	buy(1, "2", "40", "2", "9")
	requireAcquisitionCost(t, f, 1, "2", "42", "378")
}

func TestGainZeroCostCorrectionKeepsAcquisitionFXThroughTransfer(t *testing.T) {
	t.Parallel()
	f := newAccountingGainFixtureCurrency(t, "10", "0", "USD")
	if _, err := f.service.AppendManualFXQuote(t.Context(), "USD", "CNY", "7", f.now.Add(-time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.AppendManualFXQuote(t.Context(), "USD", "CNY", "8", f.now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	requireAcquisitionCost(t, f, 0, "10", "0", "0")
	f.transfer(t, 0, 1, "5", f.now)
	f.now = f.now.Add(time.Minute)
	if _, err := f.service.RecordChange(t.Context(), domain.PositionCostAdjustmentInput{
		HouseholdID: f.boot.Household.ID, HoldingID: f.holdings[1].ID,
		UnitCost: reconciliationUnitCost(t, "10"), EffectiveAt: f.now,
	}); err != nil {
		t.Fatal(err)
	}
	requireAcquisitionCost(t, f, 1, "5", "50", "350")
}
