package application

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestReconciliationCompletedCoverageIsDerivedAndReadOnly(t *testing.T) {
	for _, archived := range []string{"holding", "instrument", "both"} {
		for _, existed := range []bool{true, false} {
			name := archived + "/created-after-completion"
			if existed {
				name = archived + "/legacy-omission"
			}
			t.Run(name, func(t *testing.T) {
				s, db, _ := reconciliationFixture(t)
				ctx := t.Context()
				aid := seedHoldingsCash(t, s, ctx, "100")
				instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic coverage ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
				if err != nil {
					t.Fatal(err)
				}
				var holding domain.Holding
				create := func() {
					var err error
					holding, err = s.CreateHolding(ctx, HoldingInput{AccountID: aid.String(), InstrumentID: instrument.ID.String(), Quantity: "2", UnitCost: "10"})
					if err != nil {
						t.Fatal(err)
					}
				}
				if existed {
					create()
				}
				v := recAction(t, s, ReconciliationAction{Action: "start", AccountIDs: []string{aid.String()}, Note: "Original opening evidence"})
				for _, item := range v.Accounts[0].Items {
					v = recAction(t, s, ReconciliationAction{Action: "check", SessionID: v.ID, ExpectedRevision: v.Revision, AccountID: aid.String(), ItemID: item.ID, FinancialRevision: v.Accounts[0].FinancialRevision, Outcome: "confirmed", Source: domain.ReconciliationSource{Reference: "Synthetic review", AsOfDate: "2026-09-01"}})
				}
				v = recAction(t, s, ReconciliationAction{Action: "finish", SessionID: v.ID, ExpectedRevision: v.Revision, Note: "Original closing explanation"})
				s.SetClock(func() time.Time { return time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC) })
				if !existed {
					create()
				}
				// Synthetic legacy archive bypass; production guards remain unconditional.
				for _, table := range []string{"holdings", "instruments"} {
					if archived != "both" && archived != map[string]string{"holdings": "holding", "instruments": "instrument"}[table] {
						continue
					}
					id := holding.ID.String()
					if table == "instruments" {
						id = instrument.ID.String()
					}
					if _, err := db.SQL.Exec("UPDATE "+table+" SET archived_at=? WHERE id=?", s.clock().Format(time.RFC3339), id); err != nil {
						t.Fatal(err)
					}
				}
				if existed {
					// Emulate a completed old record that excluded the legacy position,
					// with no financial revision difference to hide the coverage problem.
					if _, err := db.SQL.Exec("DELETE FROM reconciliation_items WHERE session_id=? AND id=?", v.ID, "holding:"+holding.ID.String()); err != nil {
						t.Fatal(err)
					}
					revisions, err := sqlite.NewRepository(db).ReconciliationRevisions(ctx)
					if err != nil {
						t.Fatal(err)
					}
					a := v.Accounts[0]
					a.Items = nil
					a.Counts.Confirmed--
					a.FinancialRevision = revisions[aid.String()]
					raw, err := json.Marshal(a)
					if err != nil {
						t.Fatal(err)
					}
					if _, err = db.SQL.Exec("UPDATE reconciliation_accounts SET evidence=? WHERE session_id=? AND account_id=?", string(raw), v.ID, aid.String()); err != nil {
						t.Fatal(err)
					}
				}
				frozen, err := sqlite.NewRepository(db).ReconciliationSession(ctx, v.HouseholdID, v.ID)
				if err != nil {
					t.Fatal(err)
				}
				before, err := s.ExportJSONBytes(ctx)
				if err != nil {
					t.Fatal(err)
				}
				for read := 0; read < 3; read++ {
					current, err := s.GetReconciliationSession(ctx, v.ID)
					if err != nil {
						t.Fatal(err)
					}
					a := current.Accounts[0]
					if !a.InventoryCoverageGap || len(a.UncoveredArchivedHoldings) != 1 || a.UncoveredArchivedHoldings[0].HoldingID != holding.ID.String() || a.UncoveredArchivedHoldings[0].Quantity != "2" || a.FinancialRevisionChanged != !existed {
						t.Fatalf("incorrect independent current context: %+v", a)
					}
					if current.StartedAt != frozen.StartedAt || current.CompletedAt != frozen.CompletedAt || current.Revision != frozen.Revision || current.Status != frozen.Status || current.Counts != frozen.Counts || current.Partial != frozen.Partial || current.Note != frozen.Note || current.ClosingNote != frozen.ClosingNote || a.FinancialRevision != frozen.Accounts[0].FinancialRevision || a.Counts != frozen.Accounts[0].Counts || a.Outcome != frozen.Accounts[0].Outcome || !reflect.DeepEqual(a.Items, frozen.Accounts[0].Items) {
						t.Fatal("current context rewrote completed evidence", current, frozen)
					}
					last, err := s.ReconciliationLastSuccess(ctx, aid.String())
					if err != nil || last == nil || !last.Accounts[0].InventoryCoverageGap || last.CompletedAt != v.CompletedAt {
						t.Fatalf("account lost historical coverage context: %+v %v", last, err)
					}
				}
				after, err := s.ExportJSONBytes(ctx)
				if err != nil || string(before) != string(after) {
					t.Fatalf("coverage reads changed database: %v", err)
				}
				next := recAction(t, s, ReconciliationAction{Action: "start", AccountIDs: []string{aid.String()}})
				var target *domain.ReconciliationItem
				for i := range next.Accounts[0].Items {
					if next.Accounts[0].Items[i].HoldingID == holding.ID.String() {
						target = &next.Accounts[0].Items[i]
					}
				}
				if target == nil || !target.ArchivedNonzero || target.Outcome != "pending" {
					t.Fatal("new check lost archived pending evidence")
				}
				quantity := mustQuantity(t, "1")
				if _, err := s.BuildReconciliationCommands(ctx, []ReconciliationTarget{{HoldingID: &holding.ID, TargetQuantity: &quantity}}); err == nil {
					t.Fatal("archived adjustment accepted")
				}
				household, _ := s.requireHousehold(ctx)
				money, _ := domain.ParseMoney("10", domain.CurrencyCode("USD"))
				for _, side := range []domain.TradeSide{domain.TradeBuy, domain.TradeSell} {
					if _, err := s.PreviewChange(ctx, domain.TradeInput{HouseholdID: household.ID, Side: side, SettlementAccountID: aid, HoldingID: holding.ID, InstrumentID: instrument.ID, Quantity: quantity, Gross: money, EffectiveAt: s.clock()}); err == nil {
						t.Fatalf("archived %s accepted", side)
					}
				}

				next = recAction(t, s, ReconciliationAction{Action: "check", SessionID: next.ID, ExpectedRevision: next.Revision, AccountID: aid.String(), ItemID: target.ID, FinancialRevision: next.Accounts[0].FinancialRevision, Outcome: "unresolved", Note: "Restore before correcting"})
				next = recAction(t, s, ReconciliationAction{Action: "pause", SessionID: next.ID, ExpectedRevision: next.Revision})
				completed := recAction(t, s, ReconciliationAction{Action: "finish", SessionID: next.ID, ExpectedRevision: next.Revision, AllowPartial: true, Note: "Archived position remains unresolved"})
				if !completed.Partial || completed.Status != "completed" {
					t.Fatal("archived evidence blocked paused partial finish")
				}
			})
		}
	}
}

func TestReconciliationArchivedZeroHoldingReadContext(t *testing.T) {
	for _, archived := range []string{"holding", "instrument", "both"} {
		t.Run(archived, func(t *testing.T) {
			s, db, _ := reconciliationFixture(t)
			ctx := t.Context()
			aid := seedHoldingsCash(t, s, ctx, "100")
			instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic zero ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			holding, err := s.CreateHolding(ctx, HoldingInput{AccountID: aid.String(), InstrumentID: instrument.ID.String(), Quantity: "0"})
			if err != nil {
				t.Fatal(err)
			}
			v := recAction(t, s, ReconciliationAction{Action: "start", AccountIDs: []string{aid.String()}})
			if archived != "instrument" {
				if _, err := db.SQL.Exec("UPDATE holdings SET archived_at=? WHERE id=?", s.clock().Format(time.RFC3339), holding.ID.String()); err != nil {
					t.Fatal(err)
				}
			}
			if archived != "holding" {
				if _, err := db.SQL.Exec("UPDATE instruments SET archived_at=? WHERE id=?", s.clock().Format(time.RFC3339), instrument.ID.String()); err != nil {
					t.Fatal(err)
				}
			}
			revisions, err := sqlite.NewRepository(db).ReconciliationRevisions(ctx)
			if err != nil {
				t.Fatal(err)
			}
			// Legacy evidence can retain a zero holding at an already-current revision.
			if _, err := db.SQL.Exec("UPDATE reconciliation_accounts SET evidence=json_set(evidence,'$.financialRevision',?) WHERE session_id=? AND account_id=?", revisions[aid.String()], v.ID, aid.String()); err != nil {
				t.Fatal(err)
			}
			current, err := s.GetReconciliationSession(ctx, v.ID)
			if err != nil {
				t.Fatal(err)
			}
			a := current.Accounts[0]
			if a.InventoryCoverageGap || a.FinancialRevisionChanged || len(a.ArchivedHoldingIDs) != 1 || a.ArchivedHoldingIDs[0] != holding.ID.String() {
				t.Fatalf("zero archive confused with a nonzero coverage gap: %+v", a)
			}
			found := false
			for _, item := range a.Items {
				if item.HoldingID == holding.ID.String() {
					found = true
					if item.Kind != "holding" || item.ArchivedNonzero || item.Value != "0" {
						t.Fatal("zero evidence rewritten", item)
					}
				}
			}
			if !found {
				t.Fatal("legacy zero evidence lost")
			}
			current = recAction(t, s, ReconciliationAction{Action: "pause", SessionID: current.ID, ExpectedRevision: current.Revision})
			frozen, err := sqlite.NewRepository(db).ReconciliationSession(ctx, current.HouseholdID, current.ID)
			if err != nil || len(frozen.Accounts[0].ArchivedHoldingIDs) != 0 || frozen.Accounts[0].InventoryCoverageGap || frozen.Accounts[0].FinancialRevisionChanged || len(frozen.Accounts[0].UncoveredArchivedHoldings) != 0 {
				t.Fatal("current hints persisted into historical evidence", frozen, err)
			}
		})
	}
}
