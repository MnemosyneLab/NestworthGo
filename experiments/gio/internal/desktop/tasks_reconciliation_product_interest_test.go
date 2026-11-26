package desktop

import (
	"image"
	"reflect"
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

func TestLinkedLockedProductInterestMatchesWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	ref, _, _, refIDs := openExistingBuyFixture(t)
	forecast, remainder := "9.8765", "6.4197"
	id := seedTestLockedProduct(t, app, ids.account, &forecast)
	refID := seedTestLockedProduct(t, ref, refIDs.account, &forecast)
	api, refAPI := reconciliation.NewService(app), reconciliation.NewService(ref)
	session, refSession := startTestHoldingReconciliation(t, api, ids.account), startTestHoldingReconciliation(t, refAPI, refIDs.account)
	before, err := app.ListActivities(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := app.Product(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	fault := &linkedProductCommitFault{holdingReconcileAPI: api}
	tasks := newTasksPage(ctx, app, func() {})
	tasks.checks = app
	c := &holdingReconciliation{service: fault, ledger: app, clock: func() time.Time { return accountTestClock }}
	tasks.holdingRecon = c
	h := newShellHarnessWithPages(t, ledgerPages{tasks: tasks, version: app.MobileDataVersion})
	h.size = image.Pt(640, 1050)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 5 {
			h.frame()
			tasks.workers.Wait()
		}
	}
	click := func(s string) {
		t.Helper()
		bounds := semanticBounds(t, &h.router, s, false)
		if center := bounds.Min.Add(bounds.Max).Y / 2; center < 70 || center >= h.size.Y-20 {
			h.shell.stage.Position.Offset = max(0, h.shell.stage.Position.Offset+center-250)
			settle()
		}
		h.click(s, false)
		settle()
	}
	label := func(s string) { t.Helper(); assertPageLabel(t, &h.router, s) }
	settle()
	click("Tasks & checks")
	click("Reconciliation")
	click("Open holdings reconciliation · Harbor Brokerage")
	const action, amount, day, minute = "Record interest received", "3.4568", "2026-11-22", "01:00"
	itemID := "product:" + id.String()
	entry := &c.checkpointActions(ids.account.String(), itemID).interest
	verifyNoPosting := func() {
		t.Helper()
		links, err := api.Links(ctx, session.ID)
		if err != nil || len(links) != 0 {
			t.Fatalf("review or cancellation created a link: %+v %v", links, err)
		}
		rows, err := app.ListActivities(ctx, 100)
		if err != nil || !reflect.DeepEqual(rows, before) {
			t.Fatalf("review or cancellation changed activities: %v", err)
		}
		product, err := app.Product(ctx, id)
		if err != nil || !reflect.DeepEqual(product.Contract, initial.Contract) {
			t.Fatalf("review or cancellation changed contract: %+v %v", product.Contract, err)
		}
		assertNativeTradeCash(t, app, ids.account, map[domain.CurrencyCode]string{"USD": "156"})
	}
	edit := func() *holdingReconDraft {
		t.Helper()
		click(action)
		d := c.draft
		h.focused(&d.total.Editor)
		if d.product.id != id || d.product.link().ItemID != itemID {
			t.Fatal("interest receipt opened the wrong product checkpoint")
		}
		setBankField(h, "Interest received · USD", &d.total, amount)
		d.product.interest.day, d.product.interest.minute = day, minute
		return d
	}
	d := edit()
	click("Review")
	label("Could not review this interest receipt. Check the amount and dates and try again.")
	if d.total.Text() != amount {
		t.Fatal("the shared validation error cleared the entered amount")
	}
	setBankField(h, "Remaining interest · USD", &d.product.interest.remaining, remainder)
	click("Review")
	label("Interest received · 3.4568 USD")
	label("Account cash after · 159.4568 USD")
	label("Remaining interest · 6.4197 USD")
	click("Cancel")
	h.focused(entry)
	verifyNoPosting()

	d = edit()
	setBankField(h, "Remaining interest · USD", &d.product.interest.remaining, remainder)
	click("Review")
	staleID := d.review.planID
	review, err := api.Preview(ctx, application.ReconciliationAction{Action: "check", SessionID: session.ID, ExpectedRevision: session.Revision,
		AccountID: ids.account.String(), ItemID: itemID, FinancialRevision: session.Accounts[0].FinancialRevision,
		Outcome: "skipped", Note: "Another reviewer deferred the checkpoint."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.Commit(ctx, review.PlanID); err != nil {
		t.Fatal(err)
	}
	h.press(key.NameReturn)
	settle()
	verifyNoPosting()
	if d.review != nil || d.total.Text() != amount || d.product.interest.remaining.Text() != remainder ||
		d.product.interest.day != day || d.product.interest.minute != minute {
		t.Fatal("stale review did not preserve the complete draft for a new preview")
	}
	h.focused(&d.total.Editor)
	click("Review")
	refLink := domain.ReconciliationLink{SessionID: refSession.ID, AccountID: refIDs.account.String(), ItemID: "product:" + refID.String(),
		ExpectedRevision: refSession.Revision, FinancialRevision: refSession.Accounts[0].FinancialRevision}
	refPreview, err := refAPI.PreviewProductOperation(ctx, liquidity.ProductCommandRequest{Kind: "receive_interest", ReconciliationLink: &refLink,
		ReceiveInterest: &application.ReceiveInterestCommand{ProductID: refID.String(), Amount: amount,
			EffectiveLocalDate: day, EffectiveLocalTime: minute, RemainingInterest: &remainder}})
	if err != nil || refPreview.Operation == nil || d.review == nil || d.review.planID == staleID {
		t.Fatalf("could not compare refreshed Gio and Wails plans: %+v %v", refPreview, err)
	}
	got, want := d.review.operation, refPreview.Operation
	if got.EffectiveAt != want.EffectiveAt || got.Timezone != want.Timezone || got.LocalDate != want.LocalDate ||
		!reflect.DeepEqual(got.CashBefore, want.CashBefore) || !reflect.DeepEqual(got.CashAfter, want.CashAfter) ||
		!reflect.DeepEqual(got.ProductBefore, want.ProductBefore) || !reflect.DeepEqual(got.ProductAfter, want.ProductAfter) ||
		!reflect.DeepEqual(got.NetWorthDelta, want.NetWorthDelta) {
		t.Fatalf("Gio review differs from Wails: %+v / %+v", got, want)
	}
	if err := refAPI.CommitProduct(ctx, refPreview.PlanID); err != nil {
		t.Fatal(err)
	}
	plan := d.review.planID
	fault.loseNext = true
	h.press(key.NameReturn)
	settle()
	if !c.unresolved() || d.review.planID != plan {
		t.Fatal("unknown outcome lost the reviewed interest plan")
	}
	h.press(key.NameReturn)
	settle()
	h.focused(entry)
	if !reflect.DeepEqual(fault.plans, []string{staleID, plan, plan}) || len(c.links) != 2 {
		t.Fatalf("incorrect retry or companion link count: %v / %d", fault.plans, len(c.links))
	}
	var productLink, activityLink domain.ReconciliationLinkedChange
	for _, link := range c.links {
		if link.ItemID != itemID || link.AccountID != ids.account.String() {
			t.Fatal("interest linked to the wrong checkpoint")
		}
		switch link.Family {
		case "product_operation":
			productLink = link
		case "activity":
			activityLink = link
		}
	}
	if productLink.OperationID != plan || activityLink.OperationID == "" {
		t.Fatal("interest did not retain both the product operation and its companion activity")
	}
	click("Linked changes")
	label("Interest received · 3.4568 USD")
	label("Remaining interest · 6.4197 USD")

	savedLinks := append([]domain.ReconciliationLinkedChange(nil), c.links...)
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	links, err := reconciliation.NewService(reopened).Links(ctx, session.ID)
	if err != nil || !reflect.DeepEqual(links, savedLinks) {
		t.Fatalf("reopen did not preserve both links: %v", err)
	}
	rows, err := reopened.ListActivities(ctx, 100)
	if err != nil || len(rows) != len(before)+1 {
		t.Fatalf("expected exactly one posted activity: %d %v", len(rows), err)
	}
	interest := rows[0]
	if !reflect.DeepEqual(rows[1:], before) || interest.ID.String() != activityLink.OperationID ||
		interest.Reason != domain.ReasonInterest || interest.EffectiveLocalDate != day || !interest.EffectiveAt.Equal(accountTestClock) ||
		len(interest.Effects) != 1 || interest.Effects[0].Money == nil || interest.Effects[0].Money.CanonicalAmount() != amount {
		t.Fatalf("incorrect posted interest evidence: %+v", interest)
	}
	detail, err := reopened.Product(ctx, id)
	if err != nil || detail.Contract.State != initial.Contract.State || !reflect.DeepEqual(detail.Contract.Principal, initial.Contract.Principal) ||
		detail.Contract.MaturityInterest == nil || detail.Contract.MaturityInterest.CanonicalAmount() != remainder {
		t.Fatalf("incorrect persisted product forecast or principal: %+v %v", detail, err)
	}
	var evidence holdingLinkedEvidence
	if err := jsoncodec.Unmarshal(productLink.Evidence, &evidence); err != nil || evidence.Committed.Kind != "receive_interest" ||
		!evidence.Committed.EffectiveAt.Equal(interest.EffectiveAt) {
		t.Fatalf("incorrect persisted journal evidence: %+v %v", evidence, err)
	}
	assertNativeTradeCash(t, reopened, ids.account, map[domain.CurrencyCode]string{"USD": "159.4568"})
	assertNativeTradeCash(t, ref, refIDs.account, map[domain.CurrencyCode]string{"USD": "159.4568"})
}
