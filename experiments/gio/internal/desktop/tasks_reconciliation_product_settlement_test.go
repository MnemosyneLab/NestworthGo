package desktop

import (
	"image"
	"reflect"
	"slices"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
)

func TestLinkedLockedProductSettlementMatchesWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	ref, refStorage, refPaths, refIDs := openExistingBuyFixture(t)
	id := seedTestLockedProductWithUnlock(t, app, ids.account, nil, "2026-11-22")
	refID := seedTestLockedProductWithUnlock(t, ref, refIDs.account, nil, "2026-11-22")
	clock := func() time.Time { return time.Date(2026, time.November, 23, 17, 0, 0, 0, time.UTC) }
	app.SetClock(clock)
	ref.SetClock(clock)
	api, refAPI := reconciliation.NewService(app), reconciliation.NewService(ref)
	session := startTestHoldingReconciliation(t, api, ids.account)
	refSession := startTestHoldingReconciliation(t, refAPI, refIDs.account)
	before, err := app.ListActivities(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := app.Product(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	unrelated, err := app.ListHoldings(ctx, ids.account, false)
	if err != nil {
		t.Fatal(err)
	}
	fault := &linkedProductCommitFault{holdingReconcileAPI: api}
	tasks := newTasksPage(ctx, app, func() {})
	tasks.checks = app
	c := &holdingReconciliation{service: fault, ledger: app, clock: clock}
	tasks.holdingRecon = c
	h := newShellHarnessWithPages(t, ledgerPages{tasks: tasks, version: app.MobileDataVersion})
	h.size = image.Pt(650, 1000)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 5 {
			h.frame()
			tasks.workers.Wait()
		}
	}
	click := func(label string) {
		t.Helper()
		bounds := semanticBounds(t, &h.router, label, false)
		if y := bounds.Min.Add(bounds.Max).Y / 2; y < 70 || y >= h.size.Y-20 {
			h.shell.stage.Position.Offset = max(0, h.shell.stage.Position.Offset+y-250)
			settle()
		}
		h.click(label, false)
		settle()
	}
	label := func(text string) { t.Helper(); assertPageLabel(t, &h.router, text) }
	settle()
	click("Tasks & checks")
	click("Reconciliation")
	click("Open holdings reconciliation · Harbor Brokerage")
	const action, day, minute = "Confirm actual receipt", "2026-11-23", "18:00"
	gross, fee := "132.1234", "1.2222"
	itemID := "product:" + id.String()
	entry := &c.checkpointActions(ids.account.String(), itemID).settle
	edit := func() *holdingReconDraft {
		t.Helper()
		click(action)
		d := c.draft
		h.focused(&d.total.Editor)
		if d.product.id != id || d.product.link().AccountID != ids.account.String() || d.product.link().ItemID != itemID {
			t.Fatal("receipt opened the wrong product checkpoint")
		}
		setBankField(h, "Gross proceeds · USD", &d.total, gross)
		setBankField(h, "Fee (optional) · USD", &d.product.settlement.fee, fee)
		d.product.settlement.day, d.product.settlement.minute = day, minute
		return d
	}
	assertNoPosting := func() {
		t.Helper()
		links, err := api.Links(ctx, session.ID)
		rows, listErr := app.ListActivities(ctx, 100)
		product, productErr := app.Product(ctx, id)
		if err != nil || len(links) != 0 || listErr != nil || !reflect.DeepEqual(rows, before) ||
			productErr != nil || !reflect.DeepEqual(product.Contract, initial.Contract) {
			t.Fatalf("review or cancellation changed ledger: links=%+v activity error=%v product error=%v", links, listErr, productErr)
		}
		assertNativeTradeCash(t, app, ids.account, map[domain.CurrencyCode]string{"USD": "156"})
	}
	d := edit()
	setBankField(h, "Fee (optional) · USD", &d.product.settlement.fee, "200")
	click("Review")
	label("Could not review this receipt. Check the proceeds, fee and dates and try again.")
	setBankField(h, "Fee (optional) · USD", &d.product.settlement.fee, fee)
	click("Review")
	label("Gross proceeds · 132.1234 USD")
	label("Receipt fee · 1.2222 USD")
	label("Account cash after · 286.9012 USD")
	click("Cancel")
	h.focused(entry)
	assertNoPosting()

	d = edit()
	click("Review")
	staleID := d.review.planID
	review, err := api.Preview(ctx, application.ReconciliationAction{Action: "check", SessionID: session.ID, ExpectedRevision: session.Revision,
		AccountID: ids.account.String(), ItemID: itemID, FinancialRevision: session.Accounts[0].FinancialRevision,
		Outcome: "skipped", Note: "Another reviewer deferred this checkpoint."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.Commit(ctx, review.PlanID); err != nil {
		t.Fatal(err)
	}
	h.press(key.NameReturn)
	settle()
	assertNoPosting()
	f := d.product.settlement
	if d.review != nil || d.total.Text() != gross || f.fee.Text() != fee || f.day != day || f.minute != minute {
		t.Fatal("stale receipt did not preserve its input for another review")
	}
	h.focused(&d.total.Editor)
	click("Review")
	refLink := domain.ReconciliationLink{SessionID: refSession.ID, AccountID: refIDs.account.String(), ItemID: "product:" + refID.String(),
		ExpectedRevision: refSession.Revision, FinancialRevision: refSession.Accounts[0].FinancialRevision}
	refPreview, err := refAPI.PreviewProductOperation(ctx, liquidity.ProductCommandRequest{Kind: "settle", ReconciliationLink: &refLink,
		Settle: &application.SettleProductCommand{ProductID: refID.String(), GrossProceeds: &gross, Fee: &fee, EffectiveLocalDate: day, EffectiveLocalTime: minute}})
	if err != nil || refPreview.Operation == nil || d.review == nil || d.review.planID == staleID {
		t.Fatalf("could not compare refreshed Gio and Wails plans: %+v %v", refPreview, err)
	}
	got, want := d.review.operation, refPreview.Operation
	if got.EffectiveAt != want.EffectiveAt || got.Timezone != want.Timezone || got.LocalDate != want.LocalDate ||
		!reflect.DeepEqual(got.CashBefore, want.CashBefore) || !reflect.DeepEqual(got.CashAfter, want.CashAfter) ||
		!reflect.DeepEqual(got.ProductBefore, want.ProductBefore) || !reflect.DeepEqual(got.ProductAfter, want.ProductAfter) ||
		!reflect.DeepEqual(got.NetWorthDelta, want.NetWorthDelta) {
		t.Fatalf("Gio receipt review differs from Wails: %+v / %+v", got, want)
	}
	if err := refAPI.CommitProduct(ctx, refPreview.PlanID); err != nil {
		t.Fatal(err)
	}
	plan := d.review.planID
	fault.loseNext = true
	h.press(key.NameReturn)
	settle()
	if !c.unresolved() || d.review.planID != plan {
		t.Fatal("unknown settlement outcome lost its plan")
	}
	h.press(key.NameReturn)
	settle()
	h.focused(&c.back)
	label("Actual receipt recorded and linked.")
	if !reflect.DeepEqual(fault.plans, []string{staleID, plan, plan}) || len(c.links) != 2 {
		t.Fatalf("incorrect settlement retry or link count: %v / %d", fault.plans, len(c.links))
	}
	if findPageLabel(&h.router, action) != nil {
		t.Fatal("settled product retained its receipt action")
	}
	click("Linked changes")
	label("Gross proceeds · 132.1234 USD")
	label("Receipt fee · 1.2222 USD")

	saved := append([]domain.ReconciliationLinkedChange(nil), c.links...)
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	if err := refStorage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	reference, _ := reopenBaselineProfile(t, refPaths)
	links, err := reconciliation.NewService(reopened).Links(ctx, session.ID)
	if err != nil || !reflect.DeepEqual(links, saved) {
		t.Fatalf("reopen lost the settlement links: %+v %v", links, err)
	}
	rows, err := reopened.ListActivities(ctx, 100)
	if err != nil || len(rows) != len(before)+1 || !reflect.DeepEqual(rows[1:], before) {
		t.Fatalf("settlement changed unrelated financial history: %d %v", len(rows), err)
	}
	redemption := rows[0]
	if trade := redemption.TradeDetail; redemption.EffectiveLocalDate != day || redemption.Kind != domain.ActivitySell ||
		trade == nil || trade.Gross.CanonicalAmount() != gross || trade.Fee == nil || trade.Fee.CanonicalAmount() != fee {
		t.Fatalf("incorrect sole redemption: %+v", redemption)
	}
	for _, link := range links {
		if link.ItemID != itemID || link.AccountID != ids.account.String() {
			t.Fatal("settlement linked to the wrong checkpoint")
		}
		switch link.Family {
		case "activity":
			if link.OperationID != redemption.ID.String() {
				t.Fatal("redemption was not the linked activity")
			}
		case "product_operation":
			var evidence holdingLinkedEvidence
			if err := jsoncodec.Unmarshal(link.Evidence, &evidence); err != nil || link.OperationID != plan || evidence.Committed.Kind != "settle" ||
				!evidence.Committed.EffectiveAt.Equal(redemption.EffectiveAt) || len(evidence.Committed.Request) == 0 {
				t.Fatalf("incorrect linked receipt evidence: %+v %v", evidence, err)
			}
		default:
			t.Fatalf("unexpected settlement link: %+v", link)
		}
	}
	for _, copy := range []struct {
		service *application.Service
		product domain.ProductContractID
		account domain.AccountID
	}{{reopened, id, ids.account}, {reference, refID, refIDs.account}} {
		product, err := copy.service.Product(ctx, copy.product)
		if err != nil || product.Contract.State != domain.ProductStateSettled || product.Contract.ClosedOperationID == nil {
			t.Fatalf("reopen lost the settled contract: %+v %v", product.Contract, err)
		}
		assertNativeTradeCash(t, copy.service, copy.account, map[domain.CurrencyCode]string{"USD": "286.9012"})
	}
	remaining, err := reopened.ListHoldings(ctx, ids.account, true)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(remaining, func(h domain.Holding) bool { return h.ID == initial.Contract.HoldingID && h.Quantity.IsZero() }) {
		t.Fatal("the settled managed holding did not reopen at zero quantity")
	}
	for _, original := range unrelated {
		if original.ID == initial.Contract.HoldingID {
			continue
		}
		if !slices.ContainsFunc(remaining, func(h domain.Holding) bool { return reflect.DeepEqual(h, original) }) {
			t.Fatalf("settlement changed an unrelated holding: %+v", original)
		}
	}
}
