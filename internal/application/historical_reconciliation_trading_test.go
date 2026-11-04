package application

import (
	"slices"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// All fixtures create isolated synthetic SQLite ledgers through supported
// application commands. No real household ledger is opened or copied.
func TestHistoricalReconciliationTradingLedgers(t *testing.T) {
	for _, tc := range []struct {
		name, quantity, openingPrice, closingPrice, currency string
		buys, sells                                          []string // quantity, gross pairs
		fees                                                 []string
		adjustment                                           string
		newPosition, newInstrument                           bool
		want, price, fee, adjustmentAmount, fx               string
	}{
		{name: "gold_one_gram_keeps_whole_position", quantity: "100", openingPrice: "500", closingPrice: "550", currency: "CNY", buys: []string{"1", "540"}, fees: []string{"1"}, want: "5009", price: "5010", fee: "-1"},
		{name: "fund_recurring_buys_fees_once", quantity: "100", openingPrice: "1", closingPrice: "1.1", currency: "CNY", buys: []string{"10", "10", "20", "21"}, fees: []string{"0.1", "0.2"}, want: "11.7", price: "12", fee: "-0.3"},
		{name: "money_fund_share_remeasurement", quantity: "100", openingPrice: "1", closingPrice: "1", currency: "CNY", adjustment: "2", want: "2", adjustmentAmount: "2"},
		{name: "sell_keeps_execution_spread", quantity: "10", openingPrice: "10", closingPrice: "12", currency: "CNY", sells: []string{"4", "44"}, fees: []string{"1"}, want: "15", price: "16", fee: "-1"},
		{name: "liquidation", quantity: "10", openingPrice: "10", closingPrice: "12", currency: "CNY", sells: []string{"10", "110"}, want: "10", price: "10"},
		{name: "round_trip_keeps_execution_spread", quantity: "10", openingPrice: "10", closingPrice: "12", currency: "CNY", buys: []string{"1", "11"}, sells: []string{"1", "13"}, fees: []string{"0.1", "0.2"}, want: "21.7", price: "22", fee: "-0.3"},
		{name: "foreign_liquidation", quantity: "10", openingPrice: "10", closingPrice: "12", currency: "USD", sells: []string{"10", "110"}, want: "10180", price: "80", fx: "10100"},
		{name: "new_instrument_and_position", quantity: "0", openingPrice: "10", closingPrice: "12", currency: "CNY", buys: []string{"2", "21"}, newPosition: true, newInstrument: true, want: "3", price: "3"},
		{name: "new_position", quantity: "0", openingPrice: "10", closingPrice: "12", currency: "CNY", buys: []string{"2", "21"}, newPosition: true, want: "3", price: "3"},
		{name: "foreign_trade_opening_fx", quantity: "10", openingPrice: "10", closingPrice: "12", currency: "USD", buys: []string{"1", "11"}, fees: []string{"0.5"}, want: "10264", price: "168", fee: "-4", fx: "10100"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			s, db, owner, now := overviewFixture(t)
			broker := overviewAccount(t, s, owner, "Synthetic brokerage", "brokerage", "asset", "holdings", tc.currency, "")
			if _, err := s.AppendAccountCashValue(ctx, broker.Account.ID, "10000", tc.currency, ""); err != nil {
				t.Fatal(err)
			}
			instrumentType := "mutual_fund"
			if tc.name == "gold_one_gram_keeps_whole_position" {
				instrumentType = "precious_metal"
			}
			var instrument domain.Instrument
			var err error
			createInstrument := func() {
				t.Helper()
				instrument, err = s.CreateInstrument(ctx, InstrumentInput{Name: tc.name, Type: instrumentType, QuoteCurrency: tc.currency, QuoteSource: "manual"})
				if err != nil {
					t.Fatal(err)
				}
				if _, err = s.AppendManualInstrumentQuote(ctx, instrument.ID, tc.openingPrice, "", false); err != nil {
					t.Fatal(err)
				}
			}
			if !tc.newInstrument {
				createInstrument()
			}
			var holding domain.Holding
			costs := map[domain.HoldingID]string{}
			if !tc.newPosition {
				holding, err = s.CreateHolding(ctx, HoldingInput{AccountID: broker.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: tc.quantity})
				if err != nil {
					t.Fatal(err)
				}
				costs[holding.ID] = tc.openingPrice
			}
			if tc.currency == "USD" {
				if _, err = s.SetFXPreference(ctx, "USD", "CNY", "manual"); err != nil {
					t.Fatal(err)
				}
				if _, err = s.AppendManualFXQuote(ctx, "USD", "CNY", "7", ""); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = s.StartHistoryWithCosts(ctx, "UTC", costs); err != nil {
				t.Fatal(err)
			}
			*now = now.AddDate(0, 0, 1)
			if tc.newInstrument {
				createInstrument()
			}
			index := 0
			for _, side := range []domain.TradeSide{domain.TradeBuy, domain.TradeSell} {
				trades := tc.buys
				if side == domain.TradeSell {
					trades = tc.sells
				}
				for i := 0; i < len(trades); i += 2 {
					var fee *domain.Money
					if index < len(tc.fees) {
						m := mustMoney(t, tc.fees[index], tc.currency)
						fee = &m
					}
					index++
					change, err := s.RecordChange(ctx, domain.TradeInput{HouseholdID: broker.Account.HouseholdID, Side: side, SettlementAccountID: broker.Account.ID, HoldingID: holding.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, trades[i]), Gross: mustMoney(t, trades[i+1], tc.currency), Fee: fee, EffectiveAt: *now})
					if err != nil {
						t.Fatal(err)
					}
					holding.ID = change.Activity.TradeDetail.HoldingID
				}
			}
			if tc.adjustment != "" {
				unitCost, _ := domain.ParseUnitPrice(tc.openingPrice)
				if _, err = s.RecordChange(ctx, domain.PositionAdjustmentInput{HouseholdID: broker.Account.HouseholdID, HoldingID: holding.ID, Quantity: mustQuantity(t, tc.adjustment), Added: true, UnitCost: &unitCost, EffectiveAt: *now}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = s.AppendManualInstrumentQuote(ctx, instrument.ID, tc.closingPrice, "", false); err != nil {
				t.Fatal(err)
			}
			if tc.currency == "USD" {
				if _, err = s.AppendManualFXQuote(ctx, "USD", "CNY", "8", ""); err != nil {
					t.Fatal(err)
				}
			}
			*now = now.AddDate(0, 0, 1)
			if _, err = db.SQL.Exec("PRAGMA query_only=ON"); err != nil {
				t.Fatal(err)
			}
			o, err := s.HistoricalOverview(ctx, "2026-08-01", "2026-08-02")
			if err != nil {
				t.Fatal(err)
			}
			r := assertReconciliation(t, o, tc.want, "0")
			if r.Status != "reconciled" {
				t.Fatalf("unexpected gaps: %+v", r)
			}
			for key, want := range map[string]string{"price_change": tc.price, "fee": tc.fee, "adjustment": tc.adjustmentAmount, "fx_impact": tc.fx, "income": "0", "external_flow": "0"} {
				if want == "" {
					want = "0"
				}
				if got := reconciliationBucket(r, key); got != want {
					t.Fatalf("%s=%s want %s; %+v", key, got, want, r)
				}
			}
			assertReconciliationDetails(t, r)
			for _, d := range r.Details {
				if d.HoldingID == holding.ID.String() {
					wantOverviewAmount(t, d.QuantityResidual, "0")
					if len(d.Records) == 0 {
						t.Fatal("lost holding records")
					}
				}
			}
			reversed, err := s.HistoricalOverview(ctx, "2026-08-02", "2026-08-01")
			if err != nil {
				t.Fatal(err)
			}
			rr := assertReconciliation(t, reversed, decimal.RequireFromString(tc.want).Neg().String(), "0")
			assertReconciliationDetails(t, rr)
			for _, c := range r.Contributions {
				if got := reconciliationBucket(rr, c.Key); got != decimal.RequireFromString(c.Amount).Neg().String() {
					t.Fatalf("reverse %s=%s", c.Key, got)
				}
			}
			series, err := s.HistoricalOverviewSeries(ctx, "2026-08-01", "2026-08-02")
			if err != nil {
				t.Fatal(err)
			}
			assertReconciliation(t, series.Overview, tc.want, "0")
		})
	}
}

func assertReconciliationDetails(t *testing.T, r *HistoricalReconciliation) {
	t.Helper()
	total, supported, principal, residual := decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero
	for _, d := range r.Details {
		if d.Total == nil || d.Residual == nil {
			t.Fatalf("unexpected unknown detail: %+v", d)
		}
		a := decimal.RequireFromString(*d.Total)
		s := decimal.RequireFromString(d.Supported)
		p := decimal.RequireFromString(d.Principal)
		z := decimal.RequireFromString(*d.Residual)
		if !s.Add(p).Add(z).Equal(a) {
			t.Fatalf("leaf identity: %+v", d)
		}
		cs := decimal.Zero
		for _, c := range d.Contributions {
			cs = cs.Add(decimal.RequireFromString(c.Amount))
		}
		if !cs.Equal(s) {
			t.Fatal("detail contributions", d)
		}
		total = total.Add(a)
		supported = supported.Add(s)
		principal = principal.Add(p)
		residual = residual.Add(z)
	}
	if !total.Equal(decimal.RequireFromString(*r.Total)) || !supported.Equal(decimal.RequireFromString(*r.Supported)) || !principal.IsZero() || !residual.Equal(decimal.RequireFromString(*r.Residual)) {
		t.Fatalf("detail/household identity: %+v", r)
	}
}

func TestHistoricalReconciliationSplitAndQuantityMismatch(t *testing.T) {
	t.Parallel()
	s, _, owner, now := overviewFixture(t)
	ctx := t.Context()
	a := overviewAccount(t, s, owner, "Synthetic split", "brokerage", "asset", "holdings", "CNY", "")
	i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic split", Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.CreateHolding(ctx, HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "100"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendManualInstrumentQuote(ctx, i.ID, "10", "", false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{h.ID: "10"}); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	if _, err = s.RecordChange(ctx, domain.PositionAdjustmentInput{HouseholdID: a.Account.HouseholdID, HoldingID: h.ID, Quantity: mustQuantity(t, "50"), Added: false, EffectiveAt: *now}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendManualInstrumentQuote(ctx, i.ID, "20", "", false); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	o, err := s.HistoricalOverview(ctx, "2026-08-01", "2026-08-02")
	if err != nil {
		t.Fatal(err)
	}
	r := assertReconciliation(t, o, "0", "0")
	assertReconciliationDetails(t, r)
	if r.Status != "partial" || !slices.Contains(r.Reasons, "remeasurement_basis") || reconciliationBucket(r, "income") != "0" || reconciliationBucket(r, "adjustment") != "0" {
		t.Fatal(r)
	}
	// A quoted quantity change without an accounting record stays residual.
	row := HistoricalOverviewRow{Key: "h", Kind: "holding", Role: "asset", Left: bridgeCell("10", "10", "CNY", ""), Right: bridgeCell("20", "20", "CNY", "")}
	row.Left.Quantity = historicalString("1")
	row.Right.Quantity = historicalString("2")
	row.Left.Price = &HistoricalOverviewEvidence{Currency: "CNY", Source: "manual", Value: historicalString("10")}
	row.Right.Price = row.Left.Price
	r = runBridge(t, domain.HistoricalSnapshotBatch{}, bridgeOverview([]HistoricalOverviewRow{row}, "10"), "0", "10")
	if !slices.Contains(r.Reasons, "quantity_mismatch") {
		t.Fatal(r)
	}
}

func TestHistoricalReconciliationTradingCorrectionUndoAndSeparateIncome(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s, _, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Synthetic corrections", "brokerage", "asset", "holdings", "CNY", "")
	if _, err := s.AppendAccountCashValue(ctx, a.Account.ID, "1000", "CNY", ""); err != nil {
		t.Fatal(err)
	}
	i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic fund", Type: "mutual_fund", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.CreateHolding(ctx, HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "10"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendManualInstrumentQuote(ctx, i.ID, "10", "", false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{h.ID: "10"}); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	fee := mustMoney(t, "1", "CNY")
	input := domain.TradeInput{HouseholdID: a.Account.HouseholdID, Side: domain.TradeBuy, SettlementAccountID: a.Account.ID, HoldingID: h.ID, InstrumentID: i.ID, Quantity: mustQuantity(t, "1"), Gross: mustMoney(t, "11", "CNY"), Fee: &fee, EffectiveAt: *now}
	change, err := s.RecordChange(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordChange(ctx, domain.CashDividendInput{HouseholdID: a.Account.HouseholdID, HoldingID: h.ID, Amount: mustMoney(t, "2", "CNY"), EffectiveAt: *now}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: a.Account.HouseholdID, AccountID: a.Account.ID, Amount: mustMoney(t, "3", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: *now}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: a.Account.HouseholdID, AccountID: a.Account.ID, Amount: mustMoney(t, "4", "CNY"), Reason: domain.ReasonReconciliation, EffectiveAt: *now}); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	input.Gross = mustMoney(t, "12", "CNY")
	input.EffectiveAt = *now
	fix, err := s.FixChange(ctx, change.Activity.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	if _, err = s.UndoChange(ctx, fix.Activity.ID); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	for _, tc := range []struct{ a, z, supported, price, fee string }{
		{"2026-08-01", "2026-08-02", "6", "-2", "-1"},
		{"2026-08-02", "2026-08-04", "3", "2", "1"},
		{"2026-08-01", "2026-08-04", "9", "0", "0"},
	} {
		o, err := s.HistoricalOverview(ctx, tc.a, tc.z)
		if err != nil {
			t.Fatal(err)
		}
		r := assertReconciliation(t, o, tc.supported, "0")
		assertReconciliationDetails(t, r)
		if reconciliationBucket(r, "price_change") != tc.price || reconciliationBucket(r, "fee") != tc.fee {
			t.Fatal(r)
		}
		if tc.a == "2026-08-01" && (reconciliationBucket(r, "dividend_interest") != "2" || reconciliationBucket(r, "income") != "3" || reconciliationBucket(r, "adjustment") != "4") {
			t.Fatal("double-counted categories", r)
		}
	}
}
