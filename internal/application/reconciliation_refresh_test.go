package application

import (
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReconciliationRefreshOnlyChangedEndpoints(t *testing.T) {
	for _, scenario := range []string{"cash", "buy", "sell", "dividend", "fee", "cost", "fx", "cash_transfer", "position_transfer", "buy_then_undo", "new_holding", "metadata_and_quote"} {
		t.Run(scenario, func(t *testing.T) {
			s, db, _ := reconciliationFixture(t)
			ctx := t.Context()
			one := seedHoldingsCash(t, s, ctx, "1000")
			two := seedHoldingsCash(t, s, ctx, "500")
			if _, err := s.AppendAccountCashValue(ctx, one, "500", "EUR", ""); err != nil {
				t.Fatal(err)
			}
			first, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic first ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			second, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic second ETF", Type: "etf", QuoteCurrency: "EUR", QuoteSource: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			h1, err := s.CreateHolding(ctx, HoldingInput{AccountID: one.String(), InstrumentID: first.ID.String(), Quantity: "10", UnitCost: "10"})
			if err != nil {
				t.Fatal(err)
			}
			h2, err := s.CreateHolding(ctx, HoldingInput{AccountID: one.String(), InstrumentID: second.ID.String(), Quantity: "20", UnitCost: "10"})
			if err != nil {
				t.Fatal(err)
			}
			other, err := s.CreateHolding(ctx, HoldingInput{AccountID: two.String(), InstrumentID: first.ID.String(), Quantity: "5", UnitCost: "10"})
			if err != nil {
				t.Fatal(err)
			}
			session := recAction(t, s, ReconciliationAction{Action: "start", AccountIDs: []string{one.String(), two.String()}})
			for _, account := range session.Accounts {
				for _, item := range account.Items {
					session = recAction(t, s, ReconciliationAction{Action: "check", SessionID: session.ID, ExpectedRevision: session.Revision, AccountID: account.AccountID, ItemID: item.ID, FinancialRevision: account.FinancialRevision, Outcome: "confirmed", Source: domain.ReconciliationSource{Reference: "Synthetic endpoint review", AsOfDate: "2026-09-01"}})
				}
			}
			old := map[string]domain.ReconciliationItem{}
			key := func(account, id string) string { return account + "/" + id }
			for _, account := range session.Accounts {
				for _, item := range account.Items {
					old[key(account.AccountID, item.ID)] = item
				}
			}
			plan, err := s.PreviewReconciliationSession(ctx, ReconciliationAction{Action: "finish", SessionID: session.ID, ExpectedRevision: session.Revision})
			if err != nil {
				t.Fatal(err)
			}
			household, _ := s.requireHousehold(ctx)
			s.SetClock(func() time.Time { return time.Date(2026, 9, 1, 12, 1, 0, 0, time.UTC) })
			want := map[string]bool{}
			mark := func(account domain.AccountID, ids ...string) {
				for _, id := range ids {
					want[key(account.String(), id)] = true
				}
			}
			record := func(command any) domain.ChangePreview {
				t.Helper()
				preview, err := s.RecordChange(ctx, command)
				if err != nil {
					t.Fatal(err)
				}
				return preview
			}
			switch scenario {
			case "cash":
				record(domain.MoneyAddedInput{HouseholdID: household.ID, AccountID: one, Amount: mustMoney(t, "1", "USD"), EffectiveAt: s.clock()})
				mark(one, "cash:USD")
			case "buy", "sell", "buy_then_undo":
				side := domain.TradeBuy
				if scenario == "sell" {
					side = domain.TradeSell
				}
				fee := mustMoney(t, "1", "USD")
				preview := record(domain.TradeInput{HouseholdID: household.ID, Side: side, SettlementAccountID: one, HoldingID: h1.ID, InstrumentID: first.ID, Quantity: mustQuantity(t, "1"), Gross: mustMoney(t, "10", "USD"), Fee: &fee, EffectiveAt: s.clock()})
				if scenario == "buy_then_undo" {
					s.SetClock(func() time.Time { return time.Date(2026, 9, 1, 12, 2, 0, 0, time.UTC) })
					if _, err := s.UndoChange(ctx, preview.Activity.ID); err != nil {
						t.Fatal(err)
					}
				}
				mark(one, "cash:USD", "holding:"+h1.ID.String())
			case "dividend":
				record(domain.CashDividendInput{HouseholdID: household.ID, HoldingID: h1.ID, Amount: mustMoney(t, "5", "USD"), EffectiveAt: s.clock()})
				mark(one, "cash:USD")
			case "fee":
				record(domain.MoneyRemovedInput{HouseholdID: household.ID, AccountID: one, Amount: mustMoney(t, "1", "EUR"), Reason: domain.ReasonFee, EffectiveAt: s.clock()})
				mark(one, "cash:EUR")
			case "cost":
				record(domain.PositionCostAdjustmentInput{HouseholdID: household.ID, HoldingID: h1.ID, UnitCost: mustUnitPrice(t, "11"), EffectiveAt: s.clock()})
				mark(one, "holding:"+h1.ID.String())
			case "fx":
				record(domain.FXConversionInput{HouseholdID: household.ID, AccountID: one, Sold: mustMoney(t, "10", "USD"), Bought: mustMoney(t, "9", "EUR"), EffectiveAt: s.clock()})
				mark(one, "cash:USD", "cash:EUR")
			case "cash_transfer":
				record(domain.CashTransferInput{HouseholdID: household.ID, FromAccountID: one, ToAccountID: two, Sent: mustMoney(t, "10", "USD"), Received: mustMoney(t, "10", "USD"), EffectiveAt: s.clock()})
				mark(one, "cash:USD")
				mark(two, "cash:USD")
			case "position_transfer":
				record(domain.PositionTransferInput{HouseholdID: household.ID, FromHoldingID: h1.ID, ToHoldingID: other.ID, Quantity: mustQuantity(t, "1"), EffectiveAt: s.clock()})
				mark(one, "holding:"+h1.ID.String())
				mark(two, "holding:"+other.ID.String())
			case "new_holding":
				added, err := s.CreateHolding(ctx, HoldingInput{AccountID: two.String(), InstrumentID: second.ID.String(), Quantity: "0"})
				if err != nil {
					t.Fatal(err)
				}
				mark(two, "holding:"+added.ID.String(), "scope")
			case "metadata_and_quote":
				if _, err := db.SQL.Exec("UPDATE instruments SET name='Renamed synthetic ETF' WHERE id=?", second.ID.String()); err != nil {
					t.Fatal(err)
				}
				if _, err := s.AppendManualInstrumentQuote(ctx, first.ID, "30", "", false); err != nil {
					t.Fatal(err)
				}
			}
			if len(want) > 0 {
				if _, err := s.CommitReconciliationSession(ctx, plan); err == nil {
					t.Fatal("account CAS accepted stale finish")
				}
			}
			check := func(value domain.ReconciliationSession) {
				t.Helper()
				for _, account := range value.Accounts {
					for _, item := range account.Items {
						k := key(account.AccountID, item.ID)
						prior, exists := old[k]
						if want[k] {
							if item.Outcome != "pending" || !item.RecheckRequired || item.CheckedAt != "" {
								t.Fatalf("changed endpoint %s not pending: %+v", k, item)
							}
							if exists && !reflect.DeepEqual(item.Attempts, prior.Attempts) {
								t.Fatal("changed endpoint lost frozen attempts")
							}
							continue
						}
						if !exists {
							t.Fatalf("unexpected endpoint %s", k)
						}
						// Promoting an unchanged working point to the latest account CAS must
						// not fabricate another check or alter its frozen confirmation evidence.
						item.FinancialRevision = prior.FinancialRevision
						if !reflect.DeepEqual(item, prior) {
							t.Fatalf("unaffected %s lost confirmation: before=%+v after=%+v", k, prior, item)
						}
					}
				}
			}
			current, err := s.GetReconciliationSession(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			check(current)
			current = recAction(t, s, ReconciliationAction{Action: "pause", SessionID: current.ID, ExpectedRevision: current.Revision})
			check(current)
			current, err = s.GetReconciliationSession(ctx, current.ID)
			if err != nil {
				t.Fatal(err)
			}
			check(current)
			_ = h2 // Both holdings and both cash currencies are confirmed in the baseline.
		})
	}
}

func TestReconciliationRefreshUnknownEvidenceRetainsConservativeFallback(t *testing.T) {
	before := domain.ReconciliationAccount{FinancialRevision: 1, Items: []domain.ReconciliationItem{{ID: "scope"}, {ID: "cash:USD"}, {ID: "holding:synthetic", HoldingID: "synthetic"}}}
	fresh := before
	fresh.FinancialRevision = 2
	for _, family := range []string{"future_family", "account_cash_values", "product_policy", "product_contracts", "holdings"} {
		touched, err := (&Service{}).reconciliationTouchedItems(t.Context(), "", before, fresh, []domain.ReconciliationFinancialChange{{Family: family, Evidence: []byte("{}")}})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range before.Items {
			if !touched[item.ID] {
				t.Fatalf("%s assumed unknown evidence was unrelated to %s", family, item.ID)
			}
		}
	}
	touched, err := (&Service{}).reconciliationTouchedItems(t.Context(), "", before, fresh, nil)
	if err != nil || len(touched) != len(before.Items) {
		t.Fatal("missing legacy change log did not retain conservative fallback", touched, err)
	}
}
