package desktop

import (
	"github.com/waltwang/nestworth-go/internal/testfixture"
	"image"
	"reflect"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

func TestStandaloneProductInterestMatchesWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	ref, refStorage, refPaths, refIDs := openExistingBuyFixture(t)
	const forecast, amount, remaining, day, minute = "9.8765", "3.4568", "6.4197", "2026-11-23", "18:00"
	id := seedTestLockedProductWithUnlock(t, app, ids.account, productOptional(forecast), "2026-11-22")
	refID := seedTestLockedProductWithUnlock(t, ref, refIDs.account, productOptional(forecast), "2026-11-22")
	testfixture.SetClock(app, linkedContractClock)
	testfixture.SetClock(ref, linkedContractClock)
	before, err := app.ListActivities(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	fault := &standaloneLifecycleFault{Service: app}
	h := newStandaloneLifecycleHarness(t, app, fault, linkedContractClock)
	p := h.page
	enter := func() *productReceiptDraft {
		t.Helper()
		h.scrollClick("Record interest received")
		d := p.receipt
		if d == nil || d.received == nil || d.received.mode != domain.InterestManualMaturityAmount || d.principal.Text() != "" {
			t.Fatal("wrong interest draft")
		}
		h.focused(&d.principal.Editor)
		setBankField(h.previewHarness, "Interest received · USD", &d.principal, amount)
		d.day, d.minute = day, minute
		return d
	}
	d := enter()
	h.clickButton("Review receipt")
	if d.plan != nil || d.problem != i18n.ProductReceiptAmountsInvalid || d.principal.Text() != amount {
		t.Fatal("missing remaining interest escaped shared validation")
	}
	setBankField(h.previewHarness, "Remaining interest · USD", &d.received.remaining, remaining)
	h.clickButton("Review receipt")
	assertPageLabel(t, &h.router, "Account cash after · 159.4568 USD")
	h.clickButton("Back to details")
	h.focused(&d.principal.Editor)
	if d.received.remaining.Text() != remaining || d.day != day || d.minute != minute {
		t.Fatal("back discarded receipt inputs")
	}
	h.clickButton("Review receipt")
	h.clickButton("Cancel")
	h.focused(&p.startInterestReceipt)
	if len(managedOperations(t, app, id)) != 1 || len(fault.receipts) != 0 {
		t.Fatal("cancel wrote interest")
	}

	d = enter()
	setBankField(h.previewHarness, "Remaining interest · USD", &d.received.remaining, remaining)
	h.clickButton("Review receipt")
	reserveTermDeposit(t, app, id, "Synthetic interest reserve")
	reserveTermDeposit(t, ref, refID, "Synthetic interest reserve")
	h.press(key.NameReturn)
	h.settle()
	if d.plan != nil || d.problem != i18n.ProductReceiptStale || d.received.remaining.Text() != remaining || len(managedOperations(t, app, id)) != 1 {
		t.Fatal("stale interest preview wrote or lost input")
	}
	h.clickButton("Review receipt")
	api := liquidity.NewService(ref)
	request := liquidity.ProductCommandRequest{Kind: string(domain.ProductOpReceiveInterest), ReceiveInterest: &application.ReceiveInterestCommand{
		ProductID: refID.String(), Amount: amount, RemainingInterest: productOptional(remaining), EffectiveLocalDate: day, EffectiveLocalTime: minute,
	}}
	want, err := api.PreviewProductOperation(ctx, request)
	got := liquidity.FromProductOperationPreview(d.plan.review)
	if err != nil || got.EffectiveAt != want.EffectiveAt || !reflect.DeepEqual(got.CashAfter, want.CashAfter) || !reflect.DeepEqual(got.ProductAfter, want.ProductAfter) || d.plan.command.ReconciliationLink != nil {
		t.Fatalf("interest preview differs from Wails: %+v / %+v / %v", got, want, err)
	}
	refReceipt, err := api.RecordProductOperation(ctx, liquidity.RecordProductOperationRequest{Command: request,
		MutationID: domain.NewProductOperationID().String(), ReviewedStateHash: want.ReviewedStateHash})
	if err != nil {
		t.Fatal(err)
	}
	activityOp := d.plan.mutation
	h.size = image.Pt(665, 480)
	h.settle()
	fault.loseReceipt = true
	h.press(key.NameReturn)
	h.settle()
	h.focused(&d.replay)
	if !p.unresolved() {
		t.Fatal("lost response released the pending interest")
	}
	h.press(key.NameReturn)
	h.settle()
	if p.receipt != nil || p.saved != i18n.ProductStandaloneInterestSaved || len(fault.receipts) != 3 || !reflect.DeepEqual(fault.receipts[1], fault.receipts[2]) {
		t.Fatal("interest retry changed the submitted command")
	}

	for _, close := range []func() error{storage.Close, refStorage.Close} {
		if err := close(); err != nil {
			t.Fatal(err)
		}
	}
	app, _ = reopenBaselineProfile(t, paths)
	ref, _ = reopenBaselineProfile(t, refPaths)
	for _, c := range []struct {
		app       *application.Service
		id        domain.ProductContractID
		account   domain.AccountID
		operation domain.ProductOperationID
	}{{app, id, ids.account, activityOp}, {ref, refID, refIDs.account, domain.ProductOperationID(refReceipt.OperationID)}} {
		detail, err := c.app.Product(ctx, c.id)
		if err != nil || detail.Contract.Principal.CanonicalAmount() != "125.25" || detail.Contract.State != domain.ProductStateOpen ||
			detail.Contract.MaturityInterest == nil || detail.Contract.MaturityInterest.CanonicalAmount() != remaining || len(detail.Reservations) != 1 {
			t.Fatalf("wrong saved terms or reserve: %+v / %v", detail, err)
		}
		history, err := readProductHistoryPage(ctx, c.app, c.id, "")
		if err != nil {
			t.Fatal(err)
		}
		activity, err := c.app.Activity(ctx, history.interest)
		if err != nil || activity.Kind != domain.ActivityCashIn || activity.Reason != domain.ReasonInterest || activity.EffectiveAt.Format(time.RFC3339) != "2026-11-23T10:00:00Z" ||
			activity.ProductContext == nil || activity.ProductContext.OperationID != c.operation || len(activity.Effects) != 1 ||
			activity.Effects[0].Money == nil || activity.Effects[0].Money.CanonicalAmount() != amount {
			t.Fatalf("wrong interest receipt after reopen: %+v / %v", activity, err)
		}
		assertNativeTradeCash(t, c.app, c.account, map[domain.CurrencyCode]string{"USD": "159.4568"})
		assertNoStandaloneReconciliation(t, c.app)
	}
	after, err := app.ListActivities(ctx, 100)
	if err != nil || len(after) != len(before)+1 || !reflect.DeepEqual(after[1:], before) {
		t.Fatal("interest changed earlier activities")
	}
}

func TestStandaloneSimpleInterestPaidThroughMatchesWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	ref, refStorage, refPaths, refIDs := openExistingBuyFixture(t)
	id := seedLinkedTermDeposit(t, app, ids.account)
	refID := seedLinkedTermDeposit(t, ref, refIDs.account)
	const amount, day, minute = "1.0123", "2026-11-23", "18:00"
	input := productInterestInput{amount: amount, day: day, minute: minute}
	if _, err := reviewProductInterest(ctx, app, id, input, linkedContractClock); productReceiptProblem(err) != i18n.ProductReceiptAmountsInvalid {
		t.Fatalf("missing paid-through date escaped validation: %v", err)
	}
	input.paidThrough = day
	plan, err := reviewProductInterest(ctx, app, id, input, linkedContractClock)
	if err != nil {
		t.Fatal(err)
	}
	api := liquidity.NewService(ref)
	request := liquidity.ProductCommandRequest{Kind: string(domain.ProductOpReceiveInterest), ReceiveInterest: &application.ReceiveInterestCommand{
		ProductID: refID.String(), Amount: amount, InterestPaidThroughOn: productOptional(day), EffectiveLocalDate: day, EffectiveLocalTime: minute,
	}}
	want, err := api.PreviewProductOperation(ctx, request)
	got := liquidity.FromProductOperationPreview(plan.review)
	if err != nil || got.EffectiveAt != want.EffectiveAt || !reflect.DeepEqual(got.CashAfter, want.CashAfter) || !reflect.DeepEqual(got.ProductAfter, want.ProductAfter) {
		t.Fatalf("simple interest preview differs: %+v / %+v / %v", got, want, err)
	}
	if _, err := app.RecordProductOperation(ctx, plan.command, plan.mutation.String(), plan.review.ReviewedStateHash); err != nil {
		t.Fatal(err)
	}
	if _, err := api.RecordProductOperation(ctx, liquidity.RecordProductOperationRequest{Command: request,
		MutationID: domain.NewProductOperationID().String(), ReviewedStateHash: want.ReviewedStateHash}); err != nil {
		t.Fatal(err)
	}
	for _, close := range []func() error{storage.Close, refStorage.Close} {
		if err := close(); err != nil {
			t.Fatal(err)
		}
	}
	app, _ = reopenBaselineProfile(t, paths)
	ref, _ = reopenBaselineProfile(t, refPaths)
	for _, c := range []struct {
		app     *application.Service
		id      domain.ProductContractID
		account domain.AccountID
	}{{app, id, ids.account}, {ref, refID, refIDs.account}} {
		detail, err := c.app.Product(ctx, c.id)
		if err != nil || detail.Contract.Principal.CanonicalAmount() != "50.1234" || productString(detail.Contract.InterestPaidThroughOn) != day ||
			detail.Contract.State != domain.ProductStateOpen || len(managedOperations(t, c.app, c.id)) != 2 {
			t.Fatalf("updated deposit did not reopen: %+v / %v", detail, err)
		}
		assertNativeTradeCash(t, c.app, c.account, map[domain.CurrencyCode]string{"USD": "106.8766"})
		assertNoStandaloneReconciliation(t, c.app)
	}
}

func TestStandaloneProductTermsMatchWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	ref, refStorage, refPaths, refIDs := openExistingBuyFixture(t)
	id := seedTestLockedProductWithUnlock(t, app, ids.account, nil, "2026-11-22")
	refID := seedTestLockedProductWithUnlock(t, ref, refIDs.account, nil, "2026-11-22")
	testfixture.SetClock(app, linkedContractClock)
	testfixture.SetClock(ref, linkedContractClock)
	before, err := app.Product(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	activities, err := app.ListActivities(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	fault := &standaloneLifecycleFault{Service: app}
	h := newStandaloneLifecycleHarness(t, app, fault, linkedContractClock)
	p := h.page
	const action, name, short, fee = "Edit terms and rules", "Synthetic revised contract", "Revised", "0.0123"
	h.scrollClick(action)
	d := p.terms
	h.focused(&d.fields.name.Editor)
	d.fields.name.Editor.SetText("Canceled draft")
	h.clickButton("Cancel")
	h.focused(&p.startTermsEdit)
	if len(fault.terms) != 0 {
		t.Fatal("cancel wrote product terms")
	}
	h.press(key.NameReturn)
	h.settle()
	d = p.terms
	d.fields.name.Editor.SetText(name)
	d.shortName.Editor.SetText(short)
	d.fields.days.Editor.SetText("366")
	h.clickButton("Save policy")
	if d.problem != i18n.ProductStandaloneTermsInvalid || d.fields.name.Text() != name || d.fields.days.Text() != "366" {
		t.Fatal("invalid save lost draft or bypassed shared validation")
	}
	d.fields.days.Editor.SetText("2")
	d.fields.fee.Editor.SetText(fee)
	refCurrent, err := ref.Product(ctx, refID)
	if err != nil {
		t.Fatal(err)
	}
	refTerms, refPolicy := productTermInputs(liquidity.FromProductDetail(refCurrent).Product)
	refTerms.Name, refTerms.ShortName = name, productOptional(short)
	days, basis := 2, string(domain.DayBasisCalendar)
	refPolicy.SettlementDays, refPolicy.DayBasis, refPolicy.NormalExitFee = &days, &basis, productOptional(fee)
	want, err := liquidity.NewService(ref).UpdateProductTerms(ctx, liquidity.UpdateProductTermsRequest{ProductID: refID.String(), ExpectedRevision: refCurrent.Contract.Revision, Terms: refTerms, Policy: refPolicy})
	if err != nil {
		t.Fatal(err)
	}
	h.size = image.Pt(665, 480)
	h.settle()
	fault.loseTerms = true
	h.clickButton("Save policy")
	h.focused(&d.save)
	if p.terms != d || d.problem != i18n.ProductStandaloneTermsRetry {
		t.Fatal("unknown terms response discarded the submitted revision")
	}
	h.press(key.NameReturn)
	h.settle()
	h.focused(&d.reload)
	if d.problem != i18n.ProductStandaloneTermsStale || len(fault.terms) != 3 || !reflect.DeepEqual(fault.terms[1], fault.terms[2]) || fault.terms[2].ReconciliationLink != nil {
		t.Fatal("retry overwrote a newer revision")
	}
	h.press(key.NameReturn)
	h.settle()
	if p.terms == d || p.terms.fields.name.Text() != name || p.terms.shortName.Text() != short || p.terms.fields.fee.Text() != fee {
		t.Fatal("latest terms did not load saved facts")
	}
	h.clickButton("Cancel")
	h.focused(&p.startTermsEdit)
	for _, close := range []func() error{storage.Close, refStorage.Close} {
		if err := close(); err != nil {
			t.Fatal(err)
		}
	}
	app, _ = reopenBaselineProfile(t, paths)
	ref, _ = reopenBaselineProfile(t, refPaths)
	for _, c := range []struct {
		app     *application.Service
		id      domain.ProductContractID
		account domain.AccountID
	}{{app, id, ids.account}, {ref, refID, refIDs.account}} {
		got, err := c.app.Product(ctx, c.id)
		if err != nil || got.Contract.Revision != before.Contract.Revision+1 || got.Contract.Revision != want.Product.Revision ||
			got.Contract.Name != want.Product.Name || got.Contract.ShortName != want.Product.ShortName || got.Policy.NormalExitFee.CanonicalAmount() != want.Product.Policy.NormalExitFee.Amount ||
			*got.Policy.SettlementDays != days || !reflect.DeepEqual(got.Contract.Principal, before.Contract.Principal) || !reflect.DeepEqual(got.CurrentValue, before.CurrentValue) {
			t.Fatalf("terms differ after reopen: %+v / %v", got, err)
		}
		assertNativeTradeCash(t, c.app, c.account, map[domain.CurrencyCode]string{"USD": "156"})
		assertNoStandaloneReconciliation(t, c.app)
	}
	got, err := app.ListActivities(ctx, 100)
	if err != nil || !reflect.DeepEqual(activities, got) || len(managedOperations(t, app, id)) != 1 {
		t.Fatal("editing terms changed earlier activity or added a financial receipt")
	}
}
