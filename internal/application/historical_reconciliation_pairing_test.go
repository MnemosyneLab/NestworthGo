package application

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestHistoricalReconciliationTradingDivisionLeavesExactRemainder(t *testing.T) {
	t.Parallel()
	b, o := syntheticBridgeTrade(t, true)
	for _, r := range o.Rows {
		r.Left.FX.Operation = "divide"
		r.Left.FX.Value = historicalString("3")
		r.Left.BaseAmount = historicalString("33.3333333333333333")
		r.Right.FX.Operation = "divide"
		r.Right.FX.Value = historicalString("6")
	}
	o.Rows[0].Right.BaseAmount = historicalString("14.8333333333333333")
	o.Rows[1].Right.BaseAmount = historicalString("22")
	o.Change.NetWorth = historicalString("-29.8333333333333333")
	r := runBridge(t, b, o, "-29.8333333333333332", "-0.0000000000000001")
	assertReconciliationDetails(t, r)
	if r.Status != "partial" || reconciliationBucket(r, "price_change") != "3.5" {
		t.Fatal(r)
	}
}

func TestHistoricalReconciliationMissingTotalKeepsKnownLeafEvidence(t *testing.T) {
	t.Parallel()
	b, o := syntheticBridgeTrade(t, true)
	o.Rows[0].Right.FX = nil
	o.Rows[0].Right.BaseAmount = nil
	o.Rows[0].Right.Complete = false
	o.Change.NetWorth = nil
	o.Right.Complete = false
	r, err := historicalReconciliation(context.Background(), &b, o)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "unavailable" || r.Total != nil || r.Supported != nil || r.Residual != nil || len(r.Details) != 2 || len(r.Contributions) != 0 {
		t.Fatal(r)
	}
	if r.Details[0].Total != nil || r.Details[0].Residual != nil || len(r.Details[0].Records) != 1 || !slices.Contains(r.Details[0].Reasons, "missing_evidence") {
		t.Fatal(r.Details)
	}
	wantOverviewAmount(t, r.Details[1].Total, "356")
}

func syntheticBridgeTrade(t *testing.T, foreign bool) (domain.HistoricalSnapshotBatch, HistoricalOverviewResult) {
	t.Helper()
	id, h, i := domain.NewAccountID(), domain.NewHoldingID(), domain.NewInstrumentID()
	currency, openingFX, closingFX := "CNY", "", ""
	openingCash, closingCash, openingHolding, closingHolding, total := "100", "89", "100", "132", "21"
	if foreign {
		currency, openingFX, closingFX = "USD", "7", "8"
		openingCash, closingCash, openingHolding, closingHolding, total = "700", "712", "700", "1056", "368"
	}
	rows := []HistoricalOverviewRow{
		{Key: id.String() + ":cash:" + currency, AccountID: id.String(), Kind: "cash", Role: "asset", Left: bridgeCell("100", openingCash, currency, openingFX), Right: bridgeCell("89", closingCash, currency, closingFX)},
		{Key: "position", AccountID: id.String(), HoldingID: h.String(), InstrumentID: i.String(), Kind: "holding", Role: "asset", Left: bridgeCell("100", openingHolding, currency, openingFX), Right: bridgeCell("132", closingHolding, currency, closingFX)},
	}
	rows[1].Left.Quantity = historicalString("10")
	rows[1].Right.Quantity = historicalString("11")
	rows[1].Left.Price = &HistoricalOverviewEvidence{Source: "manual", Currency: currency, Value: historicalString("10")}
	rows[1].Right.Price = &HistoricalOverviewEvidence{Source: "manual", Currency: currency, Value: historicalString("12")}
	q := mustQuantity(t, "1")
	gross := mustMoney(t, "11", currency)
	unit, _ := domain.ParseUnitPrice("11")
	activity := domain.Activity{ID: domain.NewActivityID(), Kind: domain.ActivityBuy, Reason: domain.ReasonPrincipal, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), TradeDetail: &domain.TradeDetail{HoldingID: h, InstrumentID: i, Quantity: q, Gross: gross, UnitPrice: unit, Side: domain.TradeBuy}, Effects: []domain.ActivityEffect{
		{HoldingID: &h, InstrumentID: &i, Target: domain.EffectTargetHoldingQuantity, Quantity: &q, Direction: domain.EffectAdded, Classification: domain.ClassificationTradePrincipal},
		{AccountID: &id, Target: domain.EffectTargetAccountCash, Money: &gross, Direction: domain.EffectRemoved, Classification: domain.ClassificationTradePrincipal},
	}}
	batch := domain.HistoricalSnapshotBatch{Portfolio: domain.PortfolioSnapshot{Accounts: []domain.AccountRecord{{Account: domain.Account{ID: id, TrackingMode: domain.TrackingHoldings}}}}, Activities: []domain.Activity{activity}}
	return batch, bridgeOverview(rows, total)
}

func TestHistoricalReconciliationUnreliablePairingAndQuoteAuthorities(t *testing.T) {
	for _, name := range []string{"multi_leg", "missing_detail", "missing_cash_fx", "price_source_changed", "fx_source_changed", "native_value_mismatch", "cash_currency_mismatch"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			b, o := syntheticBridgeTrade(t, true)
			supported, residual, reason := "0", "368", "quantity_activity"
			switch name {
			case "multi_leg":
				// Two holdings and one cash leg must never each receive the
				// entire recorded gross amount.
				h := domain.NewHoldingID()
				r := o.Rows[1]
				r.Key = "second_position"
				r.HoldingID = h.String()
				o.Rows = append(o.Rows, r)
				e := b.Activities[0].Effects[0]
				e.HoldingID = &h
				b.Activities[0].Effects = append(b.Activities[0].Effects, e)
				o.Change.NetWorth = historicalString("724")
				supported, residual = "100", "624"
			case "missing_detail":
				b.Activities[0].TradeDetail = nil
				supported, residual = "100", "268"
			case "missing_cash_fx":
				o.Rows[0].Right.FX = nil
				reason = "missing_evidence"
			case "price_source_changed":
				o.Rows[1].Right.Price.Source = "agent"
				supported, residual, reason = "100", "268", "basis_changed"
			case "fx_source_changed":
				o.Rows[0].Right.FX.Source = "agent"
				o.Rows[1].Right.FX.Source = "agent"
				supported, residual, reason = "168", "200", "basis_changed"
			case "native_value_mismatch":
				o.Rows[1].Right.NativeAmount = historicalString("132.000000000001")
				supported, residual, reason = "100", "268", "valuation_mismatch"
			case "cash_currency_mismatch":
				o.Rows[0].Right.Currency = "CNY"
				supported, residual, reason = "0", "368", "basis_changed"
			}
			r := runBridge(t, b, o, supported, residual)
			if r.Status != "partial" || !slices.Contains(r.Reasons, reason) || reconciliationBucket(r, "income") != "0" {
				t.Fatal(r)
			}
			assertReconciliationDetails(t, r)
			if name == "multi_leg" || name == "missing_detail" {
				for _, d := range r.Details {
					if d.Principal != "0" {
						t.Fatal("guessed principal allocation", d)
					}
				}
			}
		})
	}
}

func TestHistoricalReconciliationNewPositionDoesNotHidePreferenceChanges(t *testing.T) {
	for _, change := range []bool{false, true} {
		t.Run(map[bool]string{false: "initial_preference", true: "same_instant_second_preference"}[change], func(t *testing.T) {
			t.Parallel()
			b, o := syntheticBridgeTrade(t, false)
			at := b.Activities[0].EffectiveAt
			id := domain.InstrumentID(o.Rows[1].InstrumentID)
			hid := domain.HoldingID(o.Rows[1].HoldingID)
			b.Portfolio.Instruments = []domain.Instrument{{ID: id, CreatedAt: at}}
			b.Portfolio.Holdings = []domain.Holding{{ID: hid, CreatedAt: at}}
			b.InstrumentPreferenceFacts = []domain.InstrumentPreferenceObservation{{InstrumentID: id, EffectiveAt: at}}
			if change {
				b.InstrumentPreferenceFacts = append(b.InstrumentPreferenceFacts, domain.InstrumentPreferenceObservation{InstrumentID: id, EffectiveAt: at})
			}
			o.Rows[1].Left = nil
			o.Rows[1].Right.Quantity = historicalString("1")
			o.Rows[1].Right.NativeAmount = historicalString("12")
			o.Rows[1].Right.BaseAmount = historicalString("12")
			o.Rows = append(o.Rows, HistoricalOverviewRow{Key: o.Rows[0].AccountID, Kind: "account", Left: bridgeCell("100", "100", "CNY", ""), Right: bridgeCell("101", "101", "CNY", "")})
			o.Change.NetWorth = historicalString("1")
			supported, residual := "1", "0"
			if change {
				supported, residual = "0", "1"
			}
			r := runBridge(t, b, o, supported, residual)
			assertReconciliationDetails(t, r)
			if slices.Contains(r.Reasons, "basis_changed") != change {
				t.Fatal(r)
			}
		})
	}
}

func TestHistoricalReconciliationUnpricedSharesUseAdjustment(t *testing.T) {
	t.Parallel()
	b, o := syntheticBridgeTrade(t, false)
	o.Rows = o.Rows[1:]
	o.Rows[0].Left.NativeAmount = historicalString("10")
	o.Rows[0].Left.BaseAmount = historicalString("10")
	o.Rows[0].Left.Quantity = historicalString("10")
	o.Rows[0].Left.Price.Value = historicalString("1")
	o.Rows[0].Right.NativeAmount = historicalString("11")
	o.Rows[0].Right.BaseAmount = historicalString("11")
	o.Rows[0].Right.Price.Value = historicalString("1")
	a := &b.Activities[0]
	a.Kind = domain.ActivityPositionTransfer
	a.Reason = domain.ReasonReconciliation
	a.TradeDetail = nil
	a.Effects = a.Effects[:1]
	a.Effects[0].Classification = domain.ClassificationRemeasurement
	o.Change.NetWorth = historicalString("1")
	r := runBridge(t, b, o, "1", "0")
	assertReconciliationDetails(t, r)
	if r.Status != "reconciled" || reconciliationBucket(r, "adjustment") != "1" || reconciliationBucket(r, "income") != "0" {
		t.Fatal(r)
	}
}

func TestHistoricalReconciliationPositionTransferIsNeutral(t *testing.T) {
	for _, tc := range []struct {
		foreign  bool
		quantity string
	}{{false, "2"}, {true, "2"}, {false, "10"}, {true, "10"}} {
		foreign := tc.foreign
		t.Run(map[bool]string{false: "domestic", true: "foreign"}[foreign]+"_"+tc.quantity, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			s, db, owner, now := overviewFixture(t)
			currency := "CNY"
			if foreign {
				currency = "USD"
			}
			a := overviewAccount(t, s, owner, "Synthetic from", "brokerage", "asset", "holdings", currency, "")
			z := overviewAccount(t, s, owner, "Synthetic to", "brokerage", "asset", "holdings", currency, "")
			i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic transferred fund", Type: "etf", QuoteCurrency: currency, QuoteSource: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			h, err := s.CreateHolding(ctx, HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "10"})
			if err != nil {
				t.Fatal(err)
			}
			to, err := s.CreateHolding(ctx, HoldingInput{AccountID: z.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "0"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.AppendManualInstrumentQuote(ctx, i.ID, "10", "", false); err != nil {
				t.Fatal(err)
			}
			if foreign {
				if _, err = s.SetFXPreference(ctx, "USD", "CNY", "manual"); err != nil {
					t.Fatal(err)
				}
				if _, err = s.AppendManualFXQuote(ctx, "USD", "CNY", "7", ""); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = s.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{h.ID: "10"}); err != nil {
				t.Fatal(err)
			}
			*now = now.AddDate(0, 0, 1)
			if _, err = s.RecordChange(ctx, domain.PositionTransferInput{HouseholdID: a.Account.HouseholdID, FromHoldingID: h.ID, ToHoldingID: to.ID, Quantity: mustQuantity(t, tc.quantity), EffectiveAt: *now}); err != nil {
				t.Fatal(err)
			}
			if _, err = s.AppendManualInstrumentQuote(ctx, i.ID, "12", "", false); err != nil {
				t.Fatal(err)
			}
			if foreign {
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
			want, price, fx := "20", "20", "0"
			if foreign {
				want, price, fx = "260", "160", "100"
			}
			r := assertReconciliation(t, o, want, "0")
			assertReconciliationDetails(t, r)
			if r.Status != "reconciled" || reconciliationBucket(r, "price_change") != price || reconciliationBucket(r, "fx_impact") != fx || reconciliationBucket(r, "income") != "0" || reconciliationBucket(r, "external_flow") != "0" {
				t.Fatal(r)
			}
		})
	}
}
