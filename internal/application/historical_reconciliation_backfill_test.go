package application

import (
	"slices"
	"strings"
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

func assertBackfillComparisonAndTrend(t *testing.T, s *Service, from, to, total string, buckets map[string]string) {
	t.Helper()
	for _, reverse := range []bool{false, true} {
		a, z, sign := from, to, decimal.NewFromInt(1)
		if reverse {
			a, z, sign = to, from, sign.Neg()
		}
		overview, err := s.HistoricalOverview(t.Context(), a, z)
		if err != nil {
			t.Fatal(err)
		}
		r := assertReconciliation(t, overview, decimal.RequireFromString(total).Mul(sign).String(), "0")
		if r.Status != "reconciled" {
			t.Fatalf("backfill evidence lost: %+v", r)
		}
		assertReconciliationDetails(t, r)
		for key, value := range buckets {
			if got, want := reconciliationBucket(r, key), decimal.RequireFromString(value).Mul(sign).String(); got != want {
				t.Fatalf("%s=%s want %s; %+v", key, got, want, r)
			}
		}
		for _, detail := range r.Details {
			if detail.Kind == "holding" {
				wantOverviewAmount(t, detail.QuantityResidual, "0")
			}
		}
	}
	series, err := s.HistoricalOverviewSeries(t.Context(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	r := assertReconciliation(t, series.Overview, total, "0")
	if r.Status != "reconciled" {
		t.Fatal(r)
	}
	assertReconciliationDetails(t, r)
	for key, want := range buckets {
		if got := reconciliationBucket(r, key); got != want {
			t.Fatalf("trend %s=%s want %s", key, got, want)
		}
	}
}

func TestHistoricalReconciliationBackfilledFirstBuyCorrectionAndUndo(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s, db, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Synthetic corrected backfill", "brokerage", "asset", "holdings", "CNY", "")
	if _, err := s.AppendAccountCashValue(ctx, a.Account.ID, "100", "CNY", ""); err != nil {
		t.Fatal(err)
	}
	instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic corrected fund", Type: "mutual_fund", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendManualInstrumentQuote(ctx, instrument.ID, "12", "", false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	effective := now.AddDate(0, 0, 1)
	*now = now.AddDate(0, 0, 10)
	auditCreated := *now
	command := domain.TradeInput{HouseholdID: a.Account.HouseholdID, Side: domain.TradeBuy, SettlementAccountID: a.Account.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, "2"), Gross: mustMoney(t, "21", "CNY"), EffectiveAt: effective}
	buy, err := s.RecordChange(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	assertBackfillComparisonAndTrend(t, s, "2026-08-01", "2026-08-02", "3", map[string]string{"price_change": "3", "external_flow": "0"})

	// Correct both magnitude fields after the closed endpoints. The replacement
	// retains the original economic instant, while its audit date advances.
	*now = now.AddDate(0, 0, 1)
	command.HoldingID = buy.Activity.TradeDetail.HoldingID
	command.Quantity, command.Gross, command.EffectiveAt = mustQuantity(t, "3"), mustMoney(t, "32", "CNY"), *now
	fixed, err := s.FixChange(ctx, buy.Activity.ID, command)
	if err != nil {
		t.Fatal(err)
	}
	if !fixed.Activity.EffectiveAt.Equal(effective) || !fixed.Activity.CreatedAt.Equal(*now) {
		t.Fatal("correction changed economic/audit semantics", fixed.Activity)
	}
	batch, err := s.repository.LoadHistoricalSnapshotBatch(ctx, a.Account.HouseholdID, *now)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Activities) != 1 || batch.Activities[0].ID != fixed.Activity.ID || !batch.Activities[0].EffectiveAt.Equal(effective) {
		t.Fatal("capture did not normalize the first-buy correction", batch.Activities)
	}
	assertBackfillComparisonAndTrend(t, s, "2026-08-01", "2026-08-02", "4", map[string]string{"price_change": "4", "adjustment": "0", "external_flow": "0"})

	// Undo the corrected buy on the next economic day, still before every audit
	// date. Reversal directions must be consumed once from the retained capture.
	*now = now.AddDate(0, 0, 1)
	undoAt := effective.AddDate(0, 0, 1)
	_, token, err := s.PreviewUndoChangeGuardedAt(ctx, fixed.Activity.ID, undoAt)
	if err != nil {
		t.Fatal(err)
	}
	undone, err := s.RecordUndoChangeGuardedAt(ctx, fixed.Activity.ID, undoAt, domain.NewMutationID().String(), strings.Repeat("ab", 32), token)
	if err != nil {
		t.Fatal(err)
	}
	if !undone.Activity.EffectiveAt.Equal(undoAt) || !undone.Activity.CreatedAt.Equal(*now) {
		t.Fatal("undo changed economic/audit semantics", undone.Activity)
	}
	batch, err = s.repository.LoadHistoricalSnapshotBatch(ctx, a.Account.HouseholdID, *now)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Activities) != 2 || batch.Activities[0].ID != fixed.Activity.ID || batch.Activities[1].ID != undone.Activity.ID {
		t.Fatal("capture lost corrected buy or retained undo", batch.Activities)
	}
	if len(batch.Portfolio.Holdings) != 1 || !batch.Portfolio.Holdings[0].CreatedAt.Equal(auditCreated) {
		t.Fatal("holding audit date rewritten", batch.Portfolio.Holdings)
	}
	if _, err = db.SQL.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	assertBackfillComparisonAndTrend(t, s, "2026-08-01", "2026-08-02", "4", map[string]string{"price_change": "4", "external_flow": "0"})
	assertBackfillComparisonAndTrend(t, s, "2026-08-01", "2026-08-03", "0", map[string]string{"price_change": "0", "adjustment": "0", "external_flow": "0"})
	assertBackfillComparisonAndTrend(t, s, "2026-08-02", "2026-08-03", "-4", map[string]string{"price_change": "-4", "external_flow": "0"})
}

func TestHistoricalReconciliationForeignFirstBackfillAndLiquidation(t *testing.T) {
	for _, liquidate := range []bool{false, true} {
		t.Run(map[bool]string{false: "open_position", true: "liquidated_position"}[liquidate], func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			s, db, owner, now := overviewFixture(t)
			a := overviewAccount(t, s, owner, "Synthetic USD backfill", "brokerage", "asset", "holdings", "USD", "")
			if _, err := s.AppendAccountCashValue(ctx, a.Account.ID, "100", "USD", ""); err != nil {
				t.Fatal(err)
			}
			instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic USD fund", Type: "mutual_fund", QuoteCurrency: "USD", QuoteSource: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.AppendManualInstrumentQuote(ctx, instrument.ID, "12", "", false); err != nil {
				t.Fatal(err)
			}
			if _, err = s.SetFXPreference(ctx, "USD", "CNY", "manual"); err != nil {
				t.Fatal(err)
			}
			if _, err = s.AppendManualFXQuote(ctx, "USD", "CNY", "7", ""); err != nil {
				t.Fatal(err)
			}
			if _, err = s.StartHistory(ctx, "UTC"); err != nil {
				t.Fatal(err)
			}
			effective := now.AddDate(0, 0, 1)
			*now = now.AddDate(0, 0, 10)
			auditCreated := *now
			buy, err := s.RecordChange(ctx, domain.TradeInput{HouseholdID: a.Account.HouseholdID, Side: domain.TradeBuy, SettlementAccountID: a.Account.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, "2"), Gross: mustMoney(t, "21", "USD"), EffectiveAt: effective})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.AppendManualFXQuote(ctx, "USD", "CNY", "8", effective.Format(time.RFC3339)); err != nil {
				t.Fatal(err)
			}
			if liquidate {
				if _, err = s.RecordChange(ctx, domain.TradeInput{HouseholdID: a.Account.HouseholdID, Side: domain.TradeSell, SettlementAccountID: a.Account.ID, InstrumentID: instrument.ID, HoldingID: buy.Activity.TradeDetail.HoldingID, Quantity: mustQuantity(t, "2"), Gross: mustMoney(t, "25", "USD"), EffectiveAt: effective.AddDate(0, 0, 1)}); err != nil {
					t.Fatal(err)
				}
			}
			batch, err := s.repository.LoadHistoricalSnapshotBatch(ctx, a.Account.HouseholdID, *now)
			if err != nil {
				t.Fatal(err)
			}
			if len(batch.Portfolio.Holdings) != 1 || !batch.Portfolio.Holdings[0].CreatedAt.Equal(auditCreated) {
				t.Fatal("holding audit date rewritten", batch.Portfolio.Holdings)
			}
			if _, err = db.SQL.Exec("PRAGMA query_only=ON"); err != nil {
				t.Fatal(err)
			}
			// The buy uses later-endpoint FX: (24-21)*8 + 100*(8-7).
			assertBackfillComparisonAndTrend(t, s, "2026-08-01", "2026-08-02", "124", map[string]string{"price_change": "24", "fx_impact": "100", "external_flow": "0"})
			if liquidate {
				// A zero closing holding needs no adopted quote/FX. Paired settlement
				// cash supplies its later FX authority without inventing return.
				closed, err := s.HistoricalOverview(ctx, "2026-08-01", "2026-08-03")
				if err != nil {
					t.Fatal(err)
				}
				row := overviewRow(t, closed, a.Account.ID.String()+":holding:"+buy.Activity.TradeDetail.HoldingID.String())
				if row.Left != nil || row.Right == nil || row.Right.FX != nil || row.Right.Price != nil {
					t.Fatalf("fixture did not exercise absent opening and cleared closing evidence: %+v", row)
				}
				wantOverviewAmount(t, row.Right.Quantity, "0")
				assertBackfillComparisonAndTrend(t, s, "2026-08-01", "2026-08-03", "132", map[string]string{"price_change": "32", "fx_impact": "100", "external_flow": "0"})
				assertBackfillComparisonAndTrend(t, s, "2026-08-02", "2026-08-03", "8", map[string]string{"price_change": "8", "fx_impact": "0", "external_flow": "0"})
			}
		})
	}
}
