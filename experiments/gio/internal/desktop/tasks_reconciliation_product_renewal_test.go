package desktop

import (
	"github.com/waltwang/nestworth-go/internal/testfixture"
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

func TestLinkedLockedProductRenewalMatchesWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	ref, refStorage, refPaths, refIDs := openExistingBuyFixture(t)
	id := seedTestLockedProductWithUnlock(t, app, ids.account, nil, "2026-11-22")
	refID := seedTestLockedProductWithUnlock(t, ref, refIDs.account, nil, "2026-11-22")
	testfixture.SetClock(app, linkedContractClock)
	testfixture.SetClock(ref, linkedContractClock)
	api, refAPI := reconciliation.NewService(app), reconciliation.NewService(ref)
	session := startTestHoldingReconciliation(t, api, ids.account)
	refSession := startTestHoldingReconciliation(t, refAPI, refIDs.account)
	baseline, err := app.Product(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	prior, err := app.ListActivities(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	priorHoldings, err := app.ListHoldings(ctx, ids.account, true)
	if err != nil {
		t.Fatal(err)
	}
	fault := &linkedProductCommitFault{holdingReconcileAPI: api}
	c := &holdingReconciliation{service: fault, ledger: app, clock: linkedContractClock}
	tasks := newTasksPage(ctx, app, func() {})
	tasks.checks, tasks.holdingRecon = app, c
	h := newShellHarnessWithPages(t, ledgerPages{tasks: tasks, version: app.MobileDataVersion})
	h.size = image.Pt(665, 1000)
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
		if y := bounds.Min.Add(bounds.Max).Y / 2; y < 70 || y >= h.size.Y-20 {
			h.shell.stage.Position.Offset = max(0, h.shell.stage.Position.Offset+y-245)
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
	const action, day, minute, successor = "Record actual renewal", "2026-11-23", "18:00", "Synthetic successor"
	const gross, closingFee, principal, openingFee = "132.1234", "1.2222", "101.2345", "0.1275"
	const maturity = "2026-12-23"
	itemID := "product:" + id.String()
	anchor := &c.checkpointActions(ids.account.String(), itemID).renew
	edit := func() *holdingReconDraft {
		t.Helper()
		click(action)
		d := c.draft
		h.focused(&d.total.Editor)
		f := d.product.renewal
		if d.product.id != id || d.product.link().ItemID != itemID || f.terms.terms.StartOn != "2026-11-20" {
			t.Fatal("renewal was not seeded from the selected product")
		}
		d.total.Editor.SetText(gross)
		f.settlement.fee.Editor.SetText(closingFee)
		f.settlement.day, f.settlement.minute = day, minute
		f.principal.Editor.SetText(principal)
		f.openingFee.Editor.SetText(openingFee)
		f.terms.name.Editor.SetText(successor)
		f.terms.terms.StartOn, f.terms.terms.MaturityOn = day, productOptional(maturity)
		f.terms.policy.UnlockOn = productOptional(maturity)
		settle()
		return d
	}
	noWrite := func() {
		t.Helper()
		products, err := app.ListProducts(ctx, &ids.account, true)
		activities, aerr := app.ListActivities(ctx, 100)
		links, lerr := api.Links(ctx, session.ID)
		if err != nil || aerr != nil || lerr != nil || len(products) != 1 || !reflect.DeepEqual(products[0].Contract, baseline.Contract) || !reflect.DeepEqual(activities, prior) || len(links) != 0 {
			t.Fatalf("unconfirmed renewal changed ledger: products=%d activities=%d links=%d", len(products), len(activities), len(links))
		}
		assertNativeTradeCash(t, app, ids.account, map[domain.CurrencyCode]string{"USD": "156"})
	}
	d := edit()
	d.product.renewal.principal.Editor.SetText("100000")
	settle()
	click("Review")
	label("Could not review renewal. Check the receipt and new contract details and try again.")
	d.product.renewal.principal.Editor.SetText(principal)
	settle()
	click("Review")
	label("New principal · 101.2345 USD")
	label("Opening fee · 0.1275 USD")
	label("Account cash after · 185.5392 USD")
	click("Back to details")
	h.focused(&d.total.Editor)
	if d.product.renewal.terms.name.Text() != successor || d.product.renewal.settlement.day != day {
		t.Fatal("Back lost renewal details")
	}
	click("Review")
	click("Cancel")
	h.focused(anchor)
	noWrite()

	d = edit()
	click("Review")
	stale := d.review.planID
	check, err := api.Preview(ctx, application.ReconciliationAction{Action: "check", SessionID: session.ID, ExpectedRevision: session.Revision,
		AccountID: ids.account.String(), ItemID: itemID, FinancialRevision: session.Accounts[0].FinancialRevision, Outcome: "skipped", Note: "Another reviewer deferred renewal."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = api.Commit(ctx, check.PlanID); err != nil {
		t.Fatal(err)
	}
	h.press(key.NameReturn)
	settle()
	noWrite()
	f := d.product.renewal
	if d.review != nil || d.total.Text() != gross || f.principal.Text() != principal || f.openingFee.Text() != openingFee || f.terms.name.Text() != successor || f.settlement.day != day || f.settlement.minute != minute {
		t.Fatal("stale confirmation lost renewal details")
	}
	h.focused(&d.total.Editor)
	click("Review")
	detail, err := ref.Product(ctx, refID)
	if err != nil {
		t.Fatal(err)
	}
	terms, policy := productTermInputs(liquidity.FromProductDetail(detail).Product)
	terms.Name, terms.StartOn, terms.MaturityOn = successor, day, productOptional(maturity)
	policy.UnlockOn = productOptional(maturity)
	refLink := domain.ReconciliationLink{SessionID: refSession.ID, AccountID: refIDs.account.String(), ItemID: "product:" + refID.String(),
		ExpectedRevision: refSession.Revision, FinancialRevision: refSession.Accounts[0].FinancialRevision}
	reference, err := refAPI.PreviewProductOperation(ctx, liquidity.ProductCommandRequest{Kind: "renew", ReconciliationLink: &refLink,
		Renew: &application.RenewProductCommand{Settle: application.SettleProductCommand{ProductID: refID.String(), GrossProceeds: productOptional(gross), Fee: productOptional(closingFee), EffectiveLocalDate: day, EffectiveLocalTime: minute},
			Principal: principal, OpeningFee: productOptional(openingFee), Terms: terms, Policy: policy}})
	if err != nil || reference.Operation == nil || d.review == nil || d.review.planID == stale {
		t.Fatalf("Wails confirmation or refreshed Gio review failed: %+v %v", reference, err)
	}
	got, want := d.review.operation, reference.Operation
	if got.EffectiveAt != want.EffectiveAt || got.Timezone != want.Timezone || len(got.Activities) != 2 || len(want.Activities) != 2 ||
		!reflect.DeepEqual(got.CashBefore, want.CashBefore) || !reflect.DeepEqual(got.CashAfter, want.CashAfter) ||
		!reflect.DeepEqual(got.ProductBefore, want.ProductBefore) || !reflect.DeepEqual(got.ProductAfter, want.ProductAfter) || !reflect.DeepEqual(got.NetWorthDelta, want.NetWorthDelta) {
		t.Fatalf("Gio renewal preview differs from Wails: %+v %+v", got, want)
	}
	if err := refAPI.CommitProduct(ctx, reference.PlanID); err != nil {
		t.Fatal(err)
	}
	plan := d.review.planID
	fault.loseNext = true
	h.press(key.NameReturn)
	settle()
	if !c.unresolved() || d.review.planID != plan {
		t.Fatal("retry lost its original renewal plan")
	}
	h.press(key.NameReturn)
	settle()
	h.focused(&c.back)
	label("Renewal recorded and linked.")
	if !reflect.DeepEqual(fault.plans, []string{stale, plan, plan}) || len(c.links) != 3 {
		t.Fatalf("incorrect retry or linked count: %v %d", fault.plans, len(c.links))
	}
	click("Linked changes")
	label("New contract · Synthetic successor")
	label("New principal · 101.2345 USD")
	saved := append([]domain.ReconciliationLinkedChange(nil), c.links...)
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	if err := refStorage.Close(); err != nil {
		t.Fatal(err)
	}
	app, _ = reopenBaselineProfile(t, paths)
	ref, _ = reopenBaselineProfile(t, refPaths)
	var oldHolding, newHolding domain.HoldingID
	for _, copy := range []struct {
		svc         *application.Service
		account     domain.AccountID
		predecessor domain.ProductContractID
		operation   string
	}{
		{app, ids.account, id, plan}, {ref, refIDs.account, refID, reference.PlanID},
	} {
		products, err := copy.svc.ListProducts(ctx, &copy.account, true)
		if err != nil || len(products) != 2 {
			t.Fatalf("expected two contracts after reopen: %v", err)
		}
		oldIndex := slices.IndexFunc(products, func(p application.ProductDetail) bool { return p.Contract.ID == copy.predecessor })
		if oldIndex < 0 {
			t.Fatal("predecessor disappeared")
		}
		old, next := products[oldIndex].Contract, products[1-oldIndex]
		nextTerms, nextPolicy := productTermInputs(liquidity.FromProductDetail(next).Product)
		if old.State != domain.ProductStateSettled || old.ClosedOperationID == nil || old.ClosedOperationID.String() != copy.operation || next.Contract.State != domain.ProductStateOpen ||
			next.Contract.RenewedFromID == nil || *next.Contract.RenewedFromID != old.ID || next.Contract.OpenedOperationID.String() != copy.operation ||
			next.Contract.AccountID != copy.account || next.Contract.Currency != "USD" || next.Contract.Principal.CanonicalAmount() != principal ||
			!reflect.DeepEqual(nextTerms, terms) || !reflect.DeepEqual(nextPolicy, policy) {
			t.Fatalf("renewal facts did not reopen: %+v %+v", old, next.Contract)
		}
		if copy.svc == app {
			oldHolding, newHolding = old.HoldingID, next.Contract.HoldingID
		}
		assertNativeTradeCash(t, copy.svc, copy.account, map[domain.CurrencyCode]string{"USD": "185.5392"})
	}
	links, err := reconciliation.NewService(app).Links(ctx, session.ID)
	if err != nil || !reflect.DeepEqual(links, saved) {
		t.Fatalf("journal did not reopen: %v", err)
	}
	activities, err := app.ListActivities(ctx, 100)
	if err != nil || len(activities) != len(prior)+2 || !reflect.DeepEqual(activities[2:], prior) {
		t.Fatalf("expected exactly two atomic renewal legs: %d %v", len(activities), err)
	}
	idsSeen := make(map[string]bool)
	for _, activity := range activities[:2] {
		trade := activity.TradeDetail
		if activity.EffectiveLocalDate != day || !activity.EffectiveAt.Equal(time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)) ||
			trade == nil || trade.Fee == nil || trade.Quantity.Canonical() != "1" {
			t.Fatalf("invalid renewal activity: %+v", activity)
		}
		switch trade.HoldingID {
		case oldHolding:
			if activity.Kind != domain.ActivitySell || trade.Gross.CanonicalAmount() != gross || trade.Fee.CanonicalAmount() != closingFee {
				t.Fatal("invalid redemption")
			}
		case newHolding:
			if activity.Kind != domain.ActivityBuy || trade.Gross.CanonicalAmount() != principal || trade.Fee.CanonicalAmount() != openingFee {
				t.Fatal("invalid opening leg")
			}
		default:
			t.Fatal("renewal affected an unrelated holding")
		}
		idsSeen[activity.ID.String()] = true
	}
	for _, link := range links {
		if link.ItemID != itemID || link.AccountID != ids.account.String() {
			t.Fatal("renewal linked outside the reviewed checkpoint")
		}
		switch link.Family {
		case "activity":
			if !idsSeen[link.OperationID] {
				t.Fatal("incorrect companion Ledger activity")
			}
		case "product_operation":
			var evidence holdingLinkedEvidence
			if err := jsoncodec.Unmarshal(link.Evidence, &evidence); err != nil || link.OperationID != plan || evidence.Committed.Kind != "renew" || len(evidence.Committed.Contracts) != 2 {
				t.Fatalf("incorrect frozen renewal receipt: %+v %v", evidence, err)
			}
		default:
			t.Fatal("unexpected linked receipt family")
		}
	}
	holdings, err := app.ListHoldings(ctx, ids.account, true)
	if err != nil || !slices.ContainsFunc(holdings, func(h domain.Holding) bool { return h.ID == oldHolding && h.Quantity.IsZero() }) ||
		!slices.ContainsFunc(holdings, func(h domain.Holding) bool { return h.ID == newHolding && h.Quantity.Canonical() == "1" }) {
		t.Fatalf("incorrect quantities after reopen: %+v %v", holdings, err)
	}
	for _, old := range priorHoldings {
		if old.ID != oldHolding && !slices.ContainsFunc(holdings, func(h domain.Holding) bool { return reflect.DeepEqual(h, old) }) {
			t.Fatal("renewal changed an unrelated holding")
		}
	}
}
