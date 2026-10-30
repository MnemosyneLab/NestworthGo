package application

import (
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Exercise the real per-holding SQL read, bulk gain snapshot and GainService.
// The base currency is CNY; acquisitions and sales are in USD or EUR.
func TestGainReversedAndCorrectedAcquisitionsKeepValidLaterFX(t *testing.T) {
	t.Parallel()
	for _, currency := range []domain.CurrencyCode{"USD", "EUR"} {
		t.Run(string(currency), func(t *testing.T) {
			f := newAccountingGainFixtureCurrency(t, "0", "10", string(currency))
			ctx := t.Context()
			day := f.now
			buy := func(quantity, gross string) domain.TradeInput {
				return domain.TradeInput{HouseholdID: f.boot.Household.ID, Side: domain.TradeBuy, SettlementAccountID: f.accounts[0].Account.ID, HoldingID: f.holdings[0].ID, InstrumentID: f.instrument.ID, Quantity: mustQuantity(t, quantity), Gross: mustMoney(t, gross, string(currency)), EffectiveAt: f.now}
			}
			addFX := func(rate string) {
				t.Helper()
				if _, err := f.service.AppendManualFXQuote(ctx, string(currency), "CNY", rate, f.now.Format(time.RFC3339)); err != nil {
					t.Fatal(err)
				}
			}
			reversed, err := f.service.RecordChange(ctx, buy("9", "90"))
			if err != nil {
				t.Fatal(err)
			}
			f.now = day.Add(24 * time.Hour)
			if _, err = f.service.UndoChange(ctx, reversed.Activity.ID); err != nil {
				t.Fatal(err)
			}
			f.now = day.Add(48 * time.Hour)
			addFX("7")
			correctedCommand := buy("2", "20")
			original, err := f.service.RecordChange(ctx, correctedCommand)
			if err != nil {
				t.Fatal(err)
			}
			f.now = day.Add(72 * time.Hour)
			addFX("8")
			later, err := f.service.RecordChange(ctx, buy("2", "40"))
			if err != nil {
				t.Fatal(err)
			}
			f.now = day.Add(96 * time.Hour)
			addFX("9")
			correctedCommand.Gross = mustMoney(t, "24", string(currency))
			correctedCommand.EffectiveAt = f.now // Correction retains acquisition's economic time (rate 7).
			fixed, err := f.service.FixChange(ctx, original.Activity.ID, correctedCommand)
			if err != nil {
				t.Fatal(err)
			}

			checkEvents := func(want []domain.ActivityID) {
				t.Helper()
				events, err := f.repo.ListCostBasisEvents(ctx, f.holdings[0].ID, domain.CostBasisReadFilter{})
				if err != nil {
					t.Fatal(err)
				}
				snapshot, err := f.repo.ReadGainSnapshot(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(events, snapshot.CostEvents[f.holdings[0].ID]) {
					t.Fatal("per-holding and bulk cost events differ")
				}
				ids := make([]domain.ActivityID, 0, len(events))
				for _, event := range events {
					ids = append(ids, event.ActivityID)
				}
				if !reflect.DeepEqual(ids, want) {
					t.Fatalf("cost event IDs=%v want %v", ids, want)
				}
			}
			checkEvents([]domain.ActivityID{fixed.Activity.ID, later.Activity.ID})
			gain, err := f.service.HoldingGain(ctx, f.holdings[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			// Native cost=24+40=64; base cost=24*7+40*8=488. Acquisition FX
			// is cost-weighted: 488/64=7.625, preserving both cost amounts.
			// Current value=4*10*9=360; instrument effect=(40-64)*7.625=-183;
			// FX effect=40*(9-7.625)=55; unrealized base gain=360-488=-128.
			if !gain.Available || gain.Quantity != "4" || gain.TotalCost.Amount != "64" || gain.TotalCostBase == nil || gain.TotalCostBase.Amount != "488" || gain.UnrealizedGainBase == nil || gain.UnrealizedGainBase.Amount != "-128" || gain.InstrumentMovement == nil || gain.InstrumentMovement.Amount != "-183" || gain.CurrencyMovement == nil || gain.CurrencyMovement.Amount != "55" {
				t.Fatalf("acquisition gain=%+v", gain)
			}
			sale := buy("1", "30")
			sale.Side = domain.TradeSell
			sold, err := f.service.RecordChange(ctx, sale)
			if err != nil {
				t.Fatal(err)
			}
			checkEvents([]domain.ActivityID{fixed.Activity.ID, later.Activity.ID, sold.Activity.ID})
			gain, err = f.service.HoldingGain(ctx, f.holdings[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			// Selling 1/4 leaves native cost 48 and acquisition base cost 366.
			// Realized native gain=30-16=14; sale-time conversion=14*9=126 CNY.
			if !gain.Available || gain.Quantity != "3" || gain.TotalCost.Amount != "48" || gain.TotalCostBase == nil || gain.TotalCostBase.Amount != "366" || gain.RealizedGain.Amount != "14" {
				t.Fatalf("post-sale gain=%+v", gain)
			}
			account, err := f.service.AccountGain(ctx, f.accounts[0].Account.ID)
			if err != nil {
				t.Fatal(err)
			}
			assertSignedGain(t, account.RealizedGain, "126", "CNY")
			for _, scope := range []domain.GainScope{{}, {AccountID: &f.accounts[0].Account.ID}, {InstrumentID: &f.instrument.ID}, {AccountID: &f.accounts[0].Account.ID, InstrumentID: &f.instrument.ID}} {
				period, err := f.service.RealizedGainInRange(ctx, scope, "2026-09-25", "2026-10-01")
				if err != nil || !period.Available {
					t.Fatalf("period=%+v err=%v", period, err)
				}
				assertSignedGain(t, period.Total, "126", "CNY")
			}
		})
	}
}
