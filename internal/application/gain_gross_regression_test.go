package application

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
	"time"
)

func TestAccountingLargeSellUsesPersistedGross(t *testing.T) {
	t.Parallel()
	f := newAccountingGainFixture(t, "100000000", "1")
	if _, err := f.service.RecordChange(t.Context(), domain.TradeInput{HouseholdID: f.boot.Household.ID, Side: domain.TradeSell, SettlementAccountID: f.accounts[0].Account.ID, HoldingID: f.holdings[0].ID, InstrumentID: f.instrument.ID, Quantity: mustQuantity(t, "100000000"), Gross: mustMoney(t, "100000000.49", "CNY"), EffectiveAt: f.now}); err != nil {
		t.Fatal(err)
	}
	gain, err := f.service.HoldingGain(t.Context(), f.holdings[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if gain.RealizedGain.Amount != "0.49" {
		t.Fatalf("realized gain=%s, want 0.49", gain.RealizedGain.Amount)
	}
}

func TestAccountingLargePartialSalesFeesLossAndPeriodAggregate(t *testing.T) {
	t.Parallel()
	f := newAccountingGainFixture(t, "100000000", "1")
	fee := mustMoney(t, "0.1", "CNY")
	sell := domain.TradeInput{HouseholdID: f.boot.Household.ID, Side: domain.TradeSell, SettlementAccountID: f.accounts[0].Account.ID, HoldingID: f.holdings[0].ID, InstrumentID: f.instrument.ID, Quantity: mustQuantity(t, "50000000"), Gross: mustMoney(t, "50000000.24", "CNY"), Fee: &fee, EffectiveAt: f.now}
	if _, err := f.service.RecordChange(t.Context(), sell); err != nil {
		t.Fatal(err)
	}
	gain, err := f.service.HoldingGain(t.Context(), f.holdings[0].ID)
	if err != nil || gain.RealizedGain.Amount != "0.14" {
		t.Fatalf("partial gain=%+v err=%v", gain, err)
	}
	f.now = f.now.Add(time.Minute)
	sell.Gross, sell.EffectiveAt = mustMoney(t, "49999999.76", "CNY"), f.now
	if _, err := f.service.RecordChange(t.Context(), sell); err != nil {
		t.Fatal(err)
	}
	gain, err = f.service.HoldingGain(t.Context(), f.holdings[0].ID)
	if err != nil || gain.RealizedGain.Amount != "-0.2" {
		t.Fatalf("full gain=%+v err=%v", gain, err)
	}
	period, err := f.service.RealizedGainInRange(t.Context(), domain.GainScope{}, "2026-09-25", "2026-09-25")
	if err != nil {
		t.Fatal(err)
	}
	if !period.Available || period.Total == nil || period.Total.Amount != "-0.2" || len(period.ByAccount) != 1 || period.ByAccount[0].Gain.Amount != "-0.2" || len(period.ByInstrument) != 1 || period.ByInstrument[0].Gain.Amount != "-0.2" {
		t.Fatalf("period aggregate=%+v", period)
	}
}

func TestAccountingBuyUsesGrossPlusFeeBeforeRounding(t *testing.T) {
	t.Parallel()
	f := newAccountingGainFixture(t, "0", "1")
	fee := mustMoney(t, "0.02", "CNY")
	buy := domain.TradeInput{HouseholdID: f.boot.Household.ID, Side: domain.TradeBuy, SettlementAccountID: f.accounts[0].Account.ID, HoldingID: f.holdings[0].ID, InstrumentID: f.instrument.ID, Quantity: mustQuantity(t, "100000000"), Gross: mustMoney(t, "100000000.49", "CNY"), Fee: &fee, EffectiveAt: f.now}
	posted, err := f.service.RecordChange(t.Context(), buy)
	if err != nil {
		t.Fatal(err)
	}
	f.requireCost(t, 0, "100000000", "100000001")
	events, err := f.repo.ListCostBasisEvents(t.Context(), f.holdings[0].ID, domain.CostBasisReadFilter{})
	if err != nil || len(events) != 1 || events[0].Gross == nil || events[0].Gross.CanonicalAmount() != "100000000.49" {
		t.Fatalf("gross events=%+v err=%v", events, err)
	}
	// Replay is read-only; the original gross and rounded price remain ledger facts.
	var gross, price string
	if err := f.db.SQL.QueryRowContext(t.Context(), `SELECT gross_amount,unit_price FROM activity_trade_details WHERE activity_id=?`, posted.Activity.ID.String()).Scan(&gross, &price); err != nil {
		t.Fatal(err)
	}
	if gross != "100000000.49" || price != "1" {
		t.Fatalf("ledger gross=%s price=%s", gross, price)
	}
	snapshot, err := f.repo.ReadGainSnapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	foundCash := false
	for _, cash := range snapshot.Portfolio.CashValues {
		if cash.AccountID == f.accounts[0].Account.ID {
			foundCash = true
			if cash.Amount.CanonicalAmount() != "899999999.49" {
				t.Fatalf("buy cash=%+v", cash)
			}
		}
	}

	if !foundCash {
		t.Fatal("buy cash missing from snapshot")
	}
}
