package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestHoldingCostAvailabilityPreservesMarketValue(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, quantity, inputCost, historyCost string
		history, known                         bool
	}{
		{name: "prehistory_unknown", quantity: "10"},
		{name: "prehistory_unit_cost_is_not_saved", quantity: "10", inputCost: "0"},
		{name: "empty_position", quantity: "0", known: true},
		{name: "explicit_zero_cost", quantity: "10", history: true, historyCost: "0", known: true},
		{name: "positive_cost", quantity: "10", history: true, historyCost: "25", known: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, s, _, _, boot, _ := historicalInsertionFixture(t)
			a, err := s.CreateAccount(ctx, AccountInput{Name: "Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
			if err != nil {
				t.Fatal(err)
			}
			i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Fund", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			h, err := s.CreateHolding(ctx, HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: tc.quantity, UnitCost: tc.inputCost})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.AppendManualInstrumentQuote(ctx, i.ID, "100", s.clock().Format(time.RFC3339Nano), false); err != nil {
				t.Fatal(err)
			}
			if tc.history {
				if _, err := s.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{h.ID: tc.historyCost}); err != nil {
					t.Fatal(err)
				}
			}
			gain, err := s.HoldingGain(ctx, h.ID)
			if err != nil {
				t.Fatal(err)
			}
			value := "1000"
			if tc.quantity == "0" {
				value = "0"
			}
			if gain.CurrentValue == nil || gain.CurrentValue.Amount != value || gain.CurrentValueBase == nil || gain.CurrentValueBase.Amount != value {
				t.Fatalf("valid market value lost: %+v", gain)
			}
			if gain.Available != tc.known || (gain.UnrealizedGain != nil) != tc.known {
				t.Fatalf("cost availability=%+v want known=%v", gain, tc.known)
			}
			if tc.known {
				wantCost, wantGain := "0", value
				if tc.historyCost == "25" {
					wantCost, wantGain = "250", "750"
				}
				if gain.TotalCost == nil || gain.TotalCost.Amount != wantCost || gain.AverageCost == nil || gain.UnrealizedGain.Amount != wantGain {
					t.Fatalf("known cost=%+v", gain)
				}
			} else if gain.TotalCost != nil || gain.AverageCost != nil || gain.TotalCostBase != nil || gain.UnrealizedGainBase != nil || gain.InstrumentMovement != nil || gain.CurrencyMovement != nil || gain.CostMissingReason == "" {
				t.Fatalf("unknown cost became numeric: %+v", gain)
			}
			account, err := s.AccountGain(ctx, a.Account.ID)
			if err != nil {
				t.Fatal(err)
			}
			if account.CurrentValue == nil || account.CurrentValue.Amount != value || (account.TotalCost != nil) != tc.known || (account.UnrealizedGain != nil) != tc.known {
				t.Fatalf("account availability=%+v", account)
			}
			groups, err := s.InstrumentHoldingGroups(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(groups) != 1 || len(groups[0].Instruments) != 1 {
				t.Fatalf("groups=%+v", groups)
			}
			for _, amounts := range []domain.HoldingAmounts{groups[0].Instruments[0].Amounts, groups[0].Instruments[0].Holdings[0].Amounts} {
				if amounts.CurrentValue == nil || amounts.CurrentValue.Amount != value || (amounts.TotalCost != nil) != tc.known || (amounts.UnrealizedGain != nil) != tc.known {
					t.Fatalf("group availability=%+v", amounts)
				}
				if !tc.known && (amounts.CostMissingReason == "" || amounts.GainMissingReason == "") {
					t.Fatalf("missing reasons lost: %+v", amounts)
				}
			}
			if tc.quantity != "0" {
				if len(groups[0].Subtotals) != 1 || (groups[0].Subtotals[0].Amounts.TotalCost != nil) != tc.known || groups[0].Subtotals[0].Amounts.CurrentValue.Amount != value {
					t.Fatalf("subtotal=%+v", groups[0].Subtotals)
				}
			}
		})
	}
}

func TestHoldingMissingStartingCostPreservesMarketValue(t *testing.T) {
	t.Parallel()
	for _, sale := range []bool{false, true} {
		name := "starting_quantity_only"
		if sale {
			name = "starting_quantity_with_sale"
		}
		t.Run(name, func(t *testing.T) {
			f := newGainReadModelFixture(t, "CNY")
			account := f.addAccount("Synthetic legacy account")
			quantity := "10"
			var events []domain.CostBasisEvent
			if sale {
				quantity = "20"
				events = append(events, f.sale("CNY", "2026-08-02", 10, "10", "100"))
			}
			holding := f.addHolding(account, "Synthetic legacy security", "CNY", quantity, "25", "100", events...)
			// A materialized legacy read can retain its starting quantity while
			// lacking acquisition cost. This does not assert current API reachability.
			f.inputs.StartingCosts[holding.ID] = nil
			f.inputs.CostEvents[holding.ID][0].UnitCost = nil
			g := f.service()
			gain, err := g.HoldingGain(t.Context(), holding.ID)
			if err != nil {
				t.Fatal(err)
			}
			if gain.CurrentValue == nil || gain.CurrentValue.Amount != "1000" || gain.CurrentValueBase == nil || gain.CurrentValueBase.Amount != "1000" {
				t.Fatalf("missing cost hid valid market value: %+v", gain)
			}
			if gain.Available || gain.AverageCost != nil || gain.TotalCost != nil || gain.TotalCostBase != nil || gain.UnrealizedGain != nil || gain.UnrealizedGainBase != nil || gain.InstrumentMovement != nil || gain.CurrencyMovement != nil || gain.RealizedGain != nil || gain.CostMissingReason == "" {
				t.Fatalf("failed cost replay became numeric: %+v", gain)
			}
			accountGain, err := g.AccountGain(t.Context(), account.ID)
			if err != nil {
				t.Fatal(err)
			}
			if accountGain.Available || accountGain.CurrentValue == nil || accountGain.CurrentValue.Amount != "1000" || accountGain.TotalCost != nil || accountGain.UnrealizedGain != nil || accountGain.RealizedGain != nil || accountGain.MissingReason != holdingCostMissingReason {
				t.Fatalf("account cost/value availability: %+v", accountGain)
			}
			groups, err := g.InstrumentHoldingGroups(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(groups) != 1 || len(groups[0].Instruments) != 1 || len(groups[0].Instruments[0].Holdings) != 1 || len(groups[0].Subtotals) != 1 {
				t.Fatalf("groups=%+v", groups)
			}
			for _, amounts := range []domain.HoldingAmounts{groups[0].Instruments[0].Amounts, groups[0].Instruments[0].Holdings[0].Amounts, groups[0].Subtotals[0].Amounts} {
				if amounts.CurrentValue == nil || amounts.CurrentValue.Amount != "1000" || amounts.TotalCost != nil || amounts.UnrealizedGain != nil || amounts.CostMissingReason == "" || amounts.GainMissingReason == "" {
					t.Fatalf("group cost/value availability: %+v", amounts)
				}
			}
		})
	}
}

func TestMixedHoldingCostAvailabilityAggregation(t *testing.T) {
	t.Parallel()
	for _, replayFails := range []bool{false, true} {
		name := "successful_empty_prehistory_replay"
		if replayFails {
			name = "missing_starting_cost_replay"
		}
		t.Run(name, func(t *testing.T) {
			f := newGainReadModelFixture(t, "CNY")
			account := f.addAccount("Synthetic mixed account")
			known := f.addHolding(account, "Known zero cost", "CNY", "5", "0", "100")
			unknown := f.addHolding(account, "Unknown cost", "CNY", "10", "25", "100")
			// Group both positions in the same instrument to exercise
			// instrument aggregation as well as account and currency subtotals.
			f.inputs.Portfolio.Holdings[1].InstrumentID = known.InstrumentID
			f.inputs.StartingCosts[unknown.ID] = nil
			if replayFails {
				f.inputs.CostEvents[unknown.ID][0].UnitCost = nil
			} else {
				f.inputs.CostEvents[unknown.ID] = nil
			}
			g := f.service()
			knownGain, err := g.HoldingGain(t.Context(), known.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !knownGain.Available || knownGain.TotalCost == nil || knownGain.TotalCost.Amount != "0" || knownGain.UnrealizedGain == nil || knownGain.UnrealizedGain.Amount != "500" || knownGain.RealizedGain == nil || knownGain.RealizedGain.Amount != "0" {
				t.Fatalf("explicit zero cost lost: %+v", knownGain)
			}
			unknownGain, err := g.HoldingGain(t.Context(), unknown.ID)
			if err != nil {
				t.Fatal(err)
			}
			if replayFails {
				if unknownGain.RealizedGain != nil {
					t.Fatalf("unknown realized gain became zero: %+v", unknownGain)
				}
			} else if unknownGain.RealizedGain == nil || unknownGain.RealizedGain.Amount != "0" {
				t.Fatalf("successful empty replay lost known realized zero: %+v", unknownGain)
			}
			accountGain, err := g.AccountGain(t.Context(), account.ID)
			if err != nil {
				t.Fatal(err)
			}
			if accountGain.CurrentValue == nil || accountGain.CurrentValue.Amount != "1500" || accountGain.TotalCost != nil || accountGain.UnrealizedGain != nil || accountGain.Available || accountGain.MissingReason != holdingCostMissingReason {
				t.Fatalf("mixed account availability: %+v", accountGain)
			}
			if (accountGain.RealizedGain == nil) != replayFails || (!replayFails && accountGain.RealizedGain.Amount != "0") {
				t.Fatalf("mixed realized availability: %+v", accountGain)
			}
			groups, err := g.InstrumentHoldingGroups(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(groups) != 1 || len(groups[0].Instruments) != 1 || len(groups[0].Instruments[0].Holdings) != 2 || len(groups[0].Subtotals) != 1 {
				t.Fatalf("groups=%+v", groups)
			}
			for _, amounts := range []domain.HoldingAmounts{groups[0].Instruments[0].Amounts, groups[0].Subtotals[0].Amounts} {
				if amounts.CurrentValue == nil || amounts.CurrentValue.Amount != "1500" || amounts.TotalCost != nil || amounts.UnrealizedGain != nil || amounts.CostMissingReason == "" || amounts.GainMissingReason == "" {
					t.Fatalf("mixed aggregate availability: %+v", amounts)
				}
			}
		})
	}
}
