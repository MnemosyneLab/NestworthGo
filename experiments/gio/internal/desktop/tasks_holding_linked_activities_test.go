package desktop

import (
	"github.com/waltwang/nestworth-go/internal/testfixture"
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestHoldingReconciliationLinkedTradeDividendAndCustodyMatchWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	ref, _, _, refIDs := openExistingBuyFixture(t)
	clock := func() time.Time { return accountTestClock }
	addDestination := func(service *application.Service) domain.AccountID {
		t.Helper()
		testfixture.SetClock(service, clock)
		boot, err := service.Bootstrap(ctx)
		if err != nil {
			t.Fatal(err)
		}
		row, err := service.CreateAccount(ctx, application.AccountInput{Name: "Second desk", AccountType: "brokerage",
			TrackingMode: "holdings", BalanceSheetRole: "asset", DefaultCurrency: "USD", IncludeInPortfolio: true,
			OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
		if err != nil {
			t.Fatal(err)
		}
		return row.Account.ID
	}
	target, refTarget := addDestination(app), addDestination(ref)
	api, refAPI := reconciliation.NewService(app), reconciliation.NewService(ref)
	session, wailsSession := startTestHoldingReconciliation(t, api, ids.account), startTestHoldingReconciliation(t, refAPI, refIDs.account)
	account, item := testHoldingCheckpoint(t, session, ids.holding)
	plan, err := api.Preview(ctx, application.ReconciliationAction{Action: "check", SessionID: session.ID, AccountID: account.AccountID,
		ItemID: item.ID, ExpectedRevision: session.Revision, FinancialRevision: account.FinancialRevision, Outcome: "confirmed",
		Source:              domain.ReconciliationSource{Reference: "Synthetic prior statement", AsOfDate: "2026-11-22", AsOfTime: "00:55"},
		ReferenceComparable: true, ReferenceValue: "2", Note: "Synthetic previous check"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.Commit(ctx, plan.PlanID); err != nil {
		t.Fatal(err)
	}
	fault := &linkedDatedFault{Service: app}
	tasks := newTasksPage(ctx, app, func() {})
	tasks.checks = app
	c := &holdingReconciliation{service: api, ledger: fault, clock: clock}
	tasks.holdingRecon = c
	ledger := newLedgerHistory(ctx, app, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{history: ledger, tasks: tasks, version: app.MobileDataVersion})
	h.size = image.Pt(640, 1280)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 5 {
			h.frame()
			tasks.workers.Wait()
			ledger.model.Wait()
			ledger.reads.workers.Wait()
		}
	}
	click := func(s string) { t.Helper(); h.click(s, false); settle() }
	label := func(s string) { t.Helper(); assertPageLabel(t, &h.router, s) }
	settle()
	click("Tasks & checks")
	click("Reconciliation")
	click("Open holdings reconciliation · Harbor Brokerage")
	action := &c.itemButtons[account.AccountID+"/"+item.ID].record
	const day = "2026-11-21"

	wails := historyapi.NewService(ref)
	var refTransferHolding domain.HoldingID
	activities := make(map[string]domain.ActivityID)
	steps := []struct {
		tab, time, note, review string
		kind                    domain.ActivityKind
		fill                    func(*accountInvestmentDraft, *historyapi.ChangeCommandRequest)
	}{
		{"Buy", "20:40", "Synthetic linked buy", "Review buy", domain.ActivityBuy, func(d *accountInvestmentDraft, req *historyapi.ChangeCommandRequest) {
			setBankField(h, "Bought quantity", &d.quantity, "0.5")
			setBankField(h, "Gross before fees · USD", &d.gross, "11.25")
			setBankField(h, "Fee (optional) · USD", &d.fee, "0.25")
			req.Kind, req.Side, req.Quantity, req.Gross, req.Fee = historyapi.ChangeTrade, "buy", "0.5", "11.25", "0.25"
		}},
		{"Sell", "20:50", "Synthetic linked sale", "Review sell", domain.ActivitySell, func(d *accountInvestmentDraft, req *historyapi.ChangeCommandRequest) {
			setBankField(h, "Sold quantity", &d.quantity, "0.25")
			setBankField(h, "Gross proceeds before fees · USD", &d.gross, "5.25")
			setBankField(h, "Fee (optional) · USD", &d.fee, "0.125")
			req.Kind, req.Side, req.Quantity, req.Gross, req.Fee = historyapi.ChangeTrade, "sell", "0.25", "5.25", "0.125"
		}},
		{"Cash dividend", "21:00", "Synthetic linked EUR dividend", "Review cash dividend", domain.ActivityCashDividend, func(d *accountInvestmentDraft, req *historyapi.ChangeCommandRequest) {
			pickAccountReference(h, "Cash currency", "EUR", "EUR")
			settle()
			setBankField(h, "Cash received · EUR", &d.gross, "1.2345")
			req.Kind, req.Currency, req.Amount = historyapi.ChangeCashDividend, "EUR", "1.2345"
		}},
		{"Transfer position", "21:10", "Synthetic linked outward transfer", "Review position transfer", domain.ActivityPositionTransfer, func(d *accountInvestmentDraft, req *historyapi.ChangeCommandRequest) {
			if d.custody.from != ids.holding {
				t.Fatal("outbound custody source was not locked")
			}
			h.click("Destination account", true)
			settle()
			click("Second desk")
			setBankField(h, "Quantity to transfer", &d.quantity, "0.375")
			req.Kind, req.FromHoldingID, req.ToAccountID, req.Quantity = historyapi.ChangePositionTransfer, refIDs.holding.String(), refTarget.String(), "0.375"
		}},
		{"Transfer position", "21:20", "Synthetic linked inward transfer", "Review position transfer", domain.ActivityPositionTransfer, func(d *accountInvestmentDraft, req *historyapi.ChangeCommandRequest) {
			click("Transfer in")
			h.click("Source holding", true)
			settle()
			for _, row := range d.custody.sources.Holdings {
				if row.Account.ID == target && row.Holding.InstrumentID == ids.instrument {
					click(row.Instrument.SelectionLabel() + " · Second desk · 0.375")
					break
				}
			}
			if d.custody.scope.holding.ID != ids.holding {
				t.Fatal("inbound custody destination was not locked")
			}
			setBankField(h, "Quantity to transfer", &d.quantity, "0.125")
			req.Kind, req.FromHoldingID, req.ToHoldingID, req.Quantity = historyapi.ChangePositionTransfer, refTransferHolding.String(), refIDs.holding.String(), "0.125"
		}},
	}
	for i, step := range steps {
		click("Record dated position change")
		d := c.dated.draft
		h.focused(&d.quantity.Editor)
		d.day, d.time = day, step.time
		setBankField(h, "Notes", &d.note, step.note)
		if i != 0 {
			click(step.tab)
			d = c.dated.draft
			if d.day != day || d.time != step.time || d.note.Text() != step.note {
				t.Fatal("action selection reset the civil date or explicit note")
			}
		}
		if d.kind != step.kind || (d.custody == nil && d.lockedHolding.ID != ids.holding) {
			t.Fatalf("wrong linked activity scope for %s", step.tab)
		}
		refAccount, refItem := testHoldingCheckpoint(t, wailsSession, refIDs.holding)
		req := historyapi.ChangeCommandRequest{HoldingID: refIDs.holding.String(), InstrumentID: refIDs.instrument.String(),
			SettlementAccountID: refIDs.account.String(), GrossCurrency: "USD", FeeCurrency: "USD",
			EffectiveLocalDate: day, EffectiveLocalTime: step.time, Note: &step.note, MutationID: domain.NewMutationID().String(),
			ReconciliationLink: &domain.ReconciliationLink{SessionID: wailsSession.ID, AccountID: refAccount.AccountID, ItemID: refItem.ID,
				ExpectedRevision: wailsSession.Revision, FinancialRevision: refAccount.FinancialRevision}}
		step.fill(d, &req)
		if i == 0 {
			// Native input can deliver the last edit and Review click together.
			// The saved note must include that edit.
			last := len(step.note) - 1
			setBankField(h, "Notes", &d.note, step.note[:last])
			bounds := semanticBounds(t, &h.router, step.review, false)
			center := bounds.Min.Add(bounds.Max).Div(2)
			pos := f32.Pt(float32(center.X), float32(center.Y))
			h.router.Queue(key.EditEvent{Text: step.note[last:], Range: key.Range{Start: last, End: last}},
				pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: pos},
				pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pos})
			settle()
		} else {
			click(step.review)
		}
		h.focused(&d.confirm)
		want, err := wails.PreviewChange(ctx, req)
		if d.review == nil || err != nil || want.Activity.EffectiveAt != wire.FormatTime(d.review.Preview.Activity.EffectiveAt) ||
			len(want.Resulting) != len(d.review.Preview.Resulting) || len(want.Review) != len(d.review.Preview.Review) {
			t.Fatalf("%s preview differs from Wails: %+v / %v", step.note, want, err)
		}
		for j, e := range d.review.Preview.Review {
			if w := want.Review[j]; w.Target != string(e.Target) || w.Before != e.Before || w.After != e.After || w.Delta != e.Delta || w.Currency != string(e.Currency) {
				t.Fatalf("%s preview differs from Wails: %+v / %+v", step.note, w, e)
			}
		}
		for j, e := range d.review.Preview.Resulting {
			if w := want.Resulting[j]; w.Amount != e.Amount || w.Quantity != e.Quantity || w.Currency != string(e.Currency) {
				t.Fatalf("%s resulting evidence differs from Wails: %+v / %+v", step.note, w, e)
			}
		}
		if _, err := wails.RecordChange(ctx, req); err != nil {
			t.Fatalf("%s reference commit: %v", step.note, err)
		}
		if wailsSession, err = refAPI.Get(ctx, wailsSession.ID); err != nil {
			t.Fatal(err)
		}
		if i == 3 {
			refPositions, err := ref.HoldingsByAccounts(ctx, []domain.AccountID{refTarget})
			if err != nil || len(refPositions[refTarget]) != 1 {
				t.Fatalf("missing reference transfer leg: %+v / %v", refPositions, err)
			}
			refTransferHolding = refPositions[refTarget][0].ID
		}
		fault.loseNext = step.kind == domain.ActivityCashDividend
		h.press(key.NameReturn)
		settle()
		if step.kind == domain.ActivityCashDividend {
			if !c.unresolved() {
				t.Fatal("a lost dividend response did not preserve the reviewed mutation")
			}
			h.press(key.NameReturn)
			settle()
			last := len(fault.attempts) - 1
			if fault.attempts[last] != fault.attempts[last-1] {
				t.Fatal("retry changed the linked dividend mutation")
			}
		}
		h.focused(action)
		if len(c.links) != i+1 {
			t.Fatalf("%s produced unexpected link count: %+v", step.note, c.links)
		}
		for _, link := range c.links {
			var evidence holdingLinkedEvidence
			if err := jsoncodec.Unmarshal(link.Evidence, &evidence); err != nil || evidence.Before == nil ||
				evidence.Before.HoldingID != ids.holding.String() || link.ItemID != item.ID || link.AccountID != account.AccountID {
				t.Fatalf("unexpected frozen checkpoint link: %+v / %v", link, err)
			}
			if evidence.Committed.Note != nil && *evidence.Committed.Note == step.note {
				activities[step.note] = domain.ActivityID(link.OperationID)
			}
		}
		if activities[step.note] == "" {
			t.Fatalf("missing linked activity for %s", step.note)
		}
	}

	// The visible earlier attempt is still the exact quantity and evidence that
	// were checked, even after several later postings.
	versionBeforeInspection := app.MobileDataVersion()
	click("Show check attempts (1)")
	label("Synthetic prior statement")
	label("Synthetic previous check")
	label("Source value · 2")
	buy := activities[steps[0].note]
	click("View linked activity · 2026-11-21 · Synthetic linked buy")
	if ledger.reads.detail.id != buy {
		t.Fatal("opened the wrong linked receipt")
	}
	click("Back to reconciliation")
	h.focused(c.receiptButtons[buy])
	label("Hide check attempts")
	// Use the original buy's disclosure; each receipt has its own focus target.
	button := &c.audit[buy].button
	h.router.Source().Execute(key.FocusCmd{Tag: button})
	h.frame()
	h.focused(button)
	h.press(key.NameReturn)
	settle()
	label("Reviewed before · 2")
	if !c.audit[buy].open {
		t.Fatal("the receipt disclosure was not keyboard-operable")
	}
	receipt := "View linked activity · 2026-11-21 · Synthetic linked buy"
	offset := semanticBounds(t, &h.router, receipt, false).Min.Y - 190
	if offset <= 0 {
		t.Fatal("expected the receipt below the fold at compact height")
	}
	h.size.Y = 480
	h.shell.stage.Position.Offset = offset
	settle()
	offset = h.shell.stage.Position.Offset
	click(receipt)
	click("Back to reconciliation")
	h.focused(c.receiptButtons[buy])
	if h.shell.stage.Position.Offset == 0 || h.shell.stage.Position.Offset != offset {
		t.Fatalf("compact Back lost the receipt scroll position: got %d want %d", h.shell.stage.Position.Offset, offset)
	}
	if !c.audit[buy].open {
		t.Fatal("Back discarded the open receipt evidence")
	}
	if app.MobileDataVersion() != versionBeforeInspection {
		t.Fatal("inspecting stored evidence changed the ledger revision")
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	assertNativeTradeCash(t, reopened, ids.account, map[domain.CurrencyCode]string{"USD": "149.625", "EUR": "1.2345"})
	positions, err := reopened.HoldingsByAccounts(ctx, []domain.AccountID{ids.account, target})
	if err != nil || len(positions[ids.account]) != 1 || len(positions[target]) != 1 ||
		positions[ids.account][0].ID != ids.holding || positions[ids.account][0].Quantity.Canonical() != "2" || positions[target][0].Quantity.Canonical() != "0.25" {
		t.Fatalf("reopen lost the original holding or custody endpoints: %+v / %v", positions, err)
	}
	links, err := reconciliation.NewService(reopened).Links(ctx, session.ID)
	if err != nil || len(links) != len(steps) {
		t.Fatalf("reopen did not preserve the linked activity journal: %+v / %v", links, err)
	}
}
