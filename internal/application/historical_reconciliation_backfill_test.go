package application

import (
	"slices"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// The audit date stays after both comparison endpoints. All writes target a
// fresh synthetic SQLite ledger; the overview and trend are read-only.
func TestHistoricalReconciliationBackfilledHoldingLedgers(t *testing.T) {
	for _, kind := range []string{"buy", "transfer", "zero_cost_adjustment"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			s, db, owner, now := overviewFixture(t)
			a := overviewAccount(t, s, owner, "Synthetic broker", "brokerage", "asset", "holdings", "CNY", "")
			if _, err := s.AppendAccountCashValue(ctx, a.Account.ID, "100", "CNY", ""); err != nil {
				t.Fatal(err)
			}
			i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic fund", Type: "mutual_fund", QuoteCurrency: "CNY", QuoteSource: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.AppendManualInstrumentQuote(ctx, i.ID, "12", "", false); err != nil {
				t.Fatal(err)
			}
			var source domain.Holding
			var target domain.AccountRecord
			costs := map[domain.HoldingID]string{}
			if kind == "transfer" {
				target = overviewAccount(t, s, owner, "Synthetic receiver", "brokerage", "asset", "holdings", "CNY", "")
				source, err = s.CreateHolding(ctx, HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "2"})
				if err != nil {
					t.Fatal(err)
				}
				costs[source.ID] = "10"
			}
			if _, err = s.StartHistoryWithCosts(ctx, "UTC", costs); err != nil {
				t.Fatal(err)
			}
			effective := now.AddDate(0, 0, 1)
			*now = now.AddDate(0, 0, 10)
			var change domain.ChangePreview
			switch kind {
			case "buy":
				change, err = s.RecordChange(ctx, domain.TradeInput{HouseholdID: a.Account.HouseholdID, Side: domain.TradeBuy, SettlementAccountID: a.Account.ID, InstrumentID: i.ID, Quantity: mustQuantity(t, "2"), Gross: mustMoney(t, "21", "CNY"), EffectiveAt: effective})
			case "transfer":
				change, err = s.RecordChange(ctx, domain.PositionTransferInput{HouseholdID: a.Account.HouseholdID, FromHoldingID: source.ID, ToAccountID: &target.Account.ID, Quantity: mustQuantity(t, "2"), EffectiveAt: effective})
			default:
				h, createErr := s.CreateHolding(ctx, HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "0"})
				if createErr != nil {
					t.Fatal(createErr)
				}
				zero, _ := domain.ParseUnitPrice("0")
				change, err = s.RecordChange(ctx, domain.PositionAdjustmentInput{HouseholdID: a.Account.HouseholdID, HoldingID: h.ID, Quantity: mustQuantity(t, "2"), Added: true, UnitCost: &zero, EffectiveAt: effective})
			}
			if err != nil {
				t.Fatal(err)
			}
			if !change.Activity.CreatedAt.Equal(*now) || !change.Activity.EffectiveAt.Equal(effective) {
				t.Fatal("lost audit/economic dates", change.Activity)
			}
			batch, err := s.repository.LoadHistoricalSnapshotBatch(ctx, a.Account.HouseholdID, *now)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, h := range batch.Portfolio.Holdings {
				if h.ID != source.ID {
					found = true
					if !h.CreatedAt.Equal(*now) {
						t.Fatal("audit creation time changed", h)
					}
				}
			}
			if !found {
				t.Fatal("new holding missing")
			}
			if _, err = db.SQL.Exec("PRAGMA query_only=ON"); err != nil {
				t.Fatal(err)
			}
			want, price, adjustment := "3", "3", "0"
			if kind == "transfer" {
				want, price = "0", "0"
			}
			if kind == "zero_cost_adjustment" {
				want, price, adjustment = "24", "0", "24"
			}
			for _, reverse := range []bool{false, true} {
				from, to := "2026-08-01", "2026-08-02"
				sign := decimal.NewFromInt(1)
				if reverse {
					from, to = to, from
					sign = sign.Neg()
				}
				o, e := s.HistoricalOverview(ctx, from, to)
				if e != nil {
					t.Fatal(e)
				}
				r := assertReconciliation(t, o, decimal.RequireFromString(want).Mul(sign).String(), "0")
				if r.Status != "reconciled" {
					t.Fatalf("lost backfilled holding: %+v", r)
				}
				assertReconciliationDetails(t, r)
				for key, v := range map[string]string{"price_change": price, "adjustment": adjustment} {
					if got := reconciliationBucket(r, key); got != decimal.RequireFromString(v).Mul(sign).String() {
						t.Fatalf("%s=%s; %+v", key, got, r)
					}
				}
			}
			trend, err := s.HistoricalOverviewSeries(ctx, "2026-08-01", "2026-08-02")
			if err != nil {
				t.Fatal(err)
			}
			r := assertReconciliation(t, trend.Overview, want, "0")
			if r.Status != "reconciled" {
				t.Fatal(r)
			}
			assertReconciliationDetails(t, r)
		})
	}
}

func TestHistoricalReconciliationFirstVisibilityGuards(t *testing.T) {
	for _, name := range []string{"backfill", "start_boundary", "before_start", "end_boundary", "after_end", "earlier_created", "zero_created", "origin", "unknown_opening", "account_absent", "account_excluded", "account_changed", "holding_archived", "missing_quote", "quantity_mismatch", "unpaired_principal", "earlier_cost_effect"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			b, o := syntheticBridgeTrade(t, false)
			h := domain.HoldingID(o.Rows[1].HoldingID)
			at := b.Activities[0].EffectiveAt
			b.Portfolio.Holdings = []domain.Holding{{ID: h, CreatedAt: at.AddDate(0, 0, 10)}}
			o.Rows[1].Left = nil
			o.Rows[1].Right.Quantity = historicalString("1")
			o.Rows[1].Right.NativeAmount = historicalString("12")
			o.Rows[1].Right.BaseAmount = historicalString("12")
			o.Rows = append(o.Rows, HistoricalOverviewRow{Key: o.Rows[0].AccountID, Kind: "account", Left: bridgeCell("100", "100", "CNY", ""), Right: bridgeCell("101", "101", "CNY", "")})
			o.Change.NetWorth = historicalString("1")
			start, _ := time.Parse(time.RFC3339Nano, o.Left.CutoffAt)
			end, _ := time.Parse(time.RFC3339Nano, o.Right.CutoffAt)
			good := name == "backfill" || name == "end_boundary"
			switch name {
			case "start_boundary":
				b.Activities[0].EffectiveAt = start
			case "before_start":
				b.Activities[0].EffectiveAt = start.Add(-time.Nanosecond)
			case "end_boundary":
				b.Activities[0].EffectiveAt = end
			case "after_end":
				b.Activities[0].EffectiveAt = end.Add(time.Nanosecond)
			case "earlier_created":
				b.Portfolio.Holdings[0].CreatedAt = start
			case "zero_created":
				b.Portfolio.Holdings[0].CreatedAt = time.Time{}
			case "origin":
				b.OriginData.Components = []domain.HistoryOriginComponent{{HoldingID: &h}}
			case "unknown_opening":
				o.Rows[1].Left = &HistoricalOverviewCell{Included: true, Currency: "CNY"}
			case "account_absent":
				o.Rows[2].Left = nil
			case "account_excluded":
				o.Rows[2].Left.Included = false
			case "account_changed":
				b.AccountStateObservations = []domain.AccountStateObservation{{AccountID: domain.AccountID(o.Rows[0].AccountID), EffectiveAt: at}}
			case "holding_archived":
				b.HoldingStateObservations = []domain.HoldingStateObservation{{HoldingID: h, EffectiveAt: at, ArchivedAt: &at}}
			case "missing_quote":
				o.Rows[1].Right.Price = nil
			case "quantity_mismatch":
				o.Rows[1].Right.Quantity = historicalString("2")
			case "unpaired_principal":
				b.Activities[0].TradeDetail = nil
			case "earlier_cost_effect":
				b.Activities = append(b.Activities, domain.Activity{ID: domain.NewActivityID(), EffectiveAt: start, Effects: []domain.ActivityEffect{{HoldingID: &h, Target: domain.EffectTargetHoldingCost}}})
			}
			r, err := historicalReconciliation(t.Context(), &b, o)
			if err != nil {
				t.Fatal(err)
			}
			if good {
				if r.Status != "reconciled" {
					t.Fatalf("valid first appearance rejected: %+v", r)
				}
				wantOverviewAmount(t, r.Residual, "0")
				assertReconciliationDetails(t, r)
			} else if r.Status != "partial" && r.Status != "unavailable" {
				t.Fatalf("guard lost: %+v", r)
			}
			if name == "quantity_mismatch" && !slices.Contains(r.Reasons, "quantity_mismatch") {
				t.Fatal(r)
			}
			if name == "unknown_opening" && r.Details[1].OpeningQuantity != nil {
				t.Fatal("unknown replaced by zero", r)
			}
		})
	}
}

func TestHistoricalReconciliationBackfillEffectBudget(t *testing.T) {
	t.Parallel()
	b, o := syntheticBridgeTrade(t, false)
	h := domain.HoldingID(o.Rows[1].HoldingID)
	b.Portfolio.Holdings = []domain.Holding{{ID: h, CreatedAt: b.Activities[0].EffectiveAt.AddDate(0, 0, 10)}}
	// Even a single activity can exhaust the existing fact/effect budget. The
	// visibility index must not turn that failure into a fabricated residual.
	b.Activities[0].Effects = make([]domain.ActivityEffect, historicalReconciliationMaxWork)
	r, err := historicalReconciliation(t.Context(), &b, o)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "unavailable" || !slices.Contains(r.Reasons, "budget_exceeded") || r.Residual != nil || r.Supported != nil || len(r.Details) != 0 {
		t.Fatal(r)
	}
}
