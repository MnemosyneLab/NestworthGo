package desktop

import (
	"context"
	"errors"
	"image"
	"reflect"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	liquidityapi "github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
)

type standaloneLifecycleFault struct {
	*application.Service
	valuations                 []application.AppendProductValuationInput
	receipts                   []productReceiptAttempt
	loseValuation, loseReceipt bool
	revisionConflict           bool
	terms                      []application.UpdateProductTermsInput
	loseTerms                  bool
}

func (f *standaloneLifecycleFault) AppendProductValuation(ctx context.Context, input application.AppendProductValuationInput) (application.ProductDetail, error) {
	f.valuations = append(f.valuations, input)
	detail, err := f.Service.AppendProductValuation(ctx, input)
	if err == nil && f.loseValuation {
		f.loseValuation = false
		return application.ProductDetail{}, errors.New("synthetic lost valuation response")
	}
	return detail, err
}

func (f *standaloneLifecycleFault) RecordProductOperation(ctx context.Context, command application.ProductCommand, mutation, hash string) (application.ProductOperationReceipt, error) {
	f.receipts = append(f.receipts, productReceiptAttempt{command, mutation, hash})
	if f.revisionConflict {
		f.revisionConflict = false
		return application.ProductOperationReceipt{}, &domain.Error{Code: domain.ErrRevisionConflict, Field: "reservationId"}
	}
	receipt, err := f.Service.RecordProductOperation(ctx, command, mutation, hash)
	if err == nil && f.loseReceipt {
		f.loseReceipt = false
		return application.ProductOperationReceipt{}, errors.New("synthetic lost receipt response")
	}
	return receipt, err
}

func (f *standaloneLifecycleFault) UpdateProductTerms(ctx context.Context, input application.UpdateProductTermsInput) (application.ProductDetail, error) {
	f.terms = append(f.terms, input)
	detail, err := f.Service.UpdateProductTerms(ctx, input)
	if err == nil && f.loseTerms {
		f.loseTerms = false
		return application.ProductDetail{}, errors.New("synthetic lost terms response")
	}
	return detail, err
}

type standaloneLifecycleHarness struct {
	*previewHarness
	funds    *availableFundsPage
	accounts *accountsPage
	page     *productDetailPage
}

func newStandaloneLifecycleHarness(t *testing.T, app *application.Service, fault *standaloneLifecycleFault, clock func() time.Time) *standaloneLifecycleHarness {
	t.Helper()
	ctx := t.Context()
	funds := newAvailableFundsPage(ctx, app, func() {})
	funds.productPage = newProductDetailPage(ctx, app, func() {})
	p := funds.productPage
	p.receipts, p.valuations, p.clock = fault, fault, clock
	p.termsLedger = fault
	accounts := newAccountsPage(ctx, app, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{funds: funds, accounts: accounts, version: app.MobileDataVersion})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	out := &standaloneLifecycleHarness{h, funds, accounts, p}
	t.Cleanup(out.drain)
	out.settle()
	out.clickButton("Available funds")
	out.scrollClick("Open product · Synthetic locked product")
	return out
}

func (h *standaloneLifecycleHarness) drain() {
	h.funds.workers.Wait()
	h.accounts.workers.Wait()
	h.page.workers.Wait()
}

func (h *standaloneLifecycleHarness) settle() {
	for range 4 {
		h.frame()
		h.drain()
	}
	h.frame()
}

func (h *standaloneLifecycleHarness) clickButton(label string) {
	h.t.Helper()
	h.click(label, false)
	h.settle()
}

func (h *standaloneLifecycleHarness) scrollClick(label string) {
	h.t.Helper()
	scrollProductControl(h.t, h.previewHarness, label)
	h.clickButton(label)
}

func managedOperations(t *testing.T, app *application.Service, id domain.ProductContractID) []domain.ProductOperationHistoryRow {
	t.Helper()
	page, err := app.ListProductOperations(t.Context(), id, "", 25)
	if err != nil {
		t.Fatal(err)
	}
	return page.Operations
}

func assertNoStandaloneReconciliation(t *testing.T, app *application.Service) {
	t.Helper()
	sessions, err := reconciliation.NewService(app).List(t.Context())
	if err != nil || len(sessions) != 0 {
		t.Fatalf("standalone operation added reconciliation evidence: %+v / %v", sessions, err)
	}
}

func TestStandaloneLockedProductValuationMatchesWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	ref, refStorage, refPaths, refIDs := openExistingBuyFixture(t)
	id, refID := seedTestLockedProduct(t, app, ids.account, nil), seedTestLockedProduct(t, ref, refIDs.account, nil)
	now := linkedContractClock().Add(123 * time.Millisecond)
	clock := func() time.Time { return now }
	app.SetClock(clock)
	ref.SetClock(clock)
	before, err := app.ListActivities(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}

	fault := &standaloneLifecycleFault{Service: app}
	h := newStandaloneLifecycleHarness(t, app, fault, clock)
	p := h.page
	h.scrollClick("Open account · Harbor Brokerage")
	h.scrollClick("Open product · Synthetic locked product")
	h.scrollClick("Update contract valuation")
	d := p.valuation
	if d == nil || d.currency != "USD" || d.amount.Text() != "" || d.before.CanonicalAmount() != "123.4567" {
		t.Fatal("valuation defaults differ from the standalone Wails action")
	}
	h.focused(&d.amount.Editor)
	const field = "New contract value · USD"
	setBankField(h.previewHarness, field, &d.amount, "131.2345")
	h.clickButton("Cancel")
	h.focused(&p.startValuation)
	if len(fault.valuations) != 0 || len(managedOperations(t, app, id)) != 1 {
		t.Fatal("cancel recorded an observation")
	}

	h.press(key.NameReturn)
	h.settle()
	d = p.valuation
	h.size = image.Pt(665, 480)
	h.settle()
	setBankField(h.previewHarness, field, &d.amount, "0")
	h.clickButton("Confirm")
	if d.problem != i18n.ProductValuationInvalid || d.amount.Text() != "0" || p.unresolved() {
		t.Fatal("shared invalid value did not keep an editable draft")
	}
	const amount = "131.2345"
	setBankField(h.previewHarness, field, &d.amount, amount)
	fault.loseValuation = true
	h.clickButton("Confirm")
	if !p.unresolved() || d.problem != i18n.ContributionUncertain || len(managedOperations(t, app, id)) != 2 {
		t.Fatal("a lost response did not preserve the original valuation for retry")
	}
	h.shell.pendingNavigation = &navigationRequest{step: -1}
	h.settle()
	if p.valuation != d {
		t.Fatal("Back discarded a pending outcome")
	}
	h.focused(&d.save)
	h.press(key.NameReturn)
	h.settle()
	if p.valuation != nil || p.saved != i18n.ProductValuationSaved || len(fault.valuations) != 3 ||
		fault.valuations[1] != fault.valuations[2] || fault.valuations[1].ObservedAt != "" {
		t.Fatal("valuation retry changed Wails's standalone request")
	}
	assertPageLabel(t, &h.router, "Current contract value · "+amount+" USD")
	h.clickButton("Back")
	h.focused(h.accounts.productButtons[id])
	assertPageLabel(t, &h.router, "Current contract value · "+amount+" USD")

	api := liquidityapi.NewService(ref)
	result, err := api.AppendProductValuation(ctx, liquidityapi.AppendProductValuationRequest{
		ProductID: refID.String(), Amount: amount, MutationID: domain.NewProductOperationID().String(),
	})
	if err != nil || result.Product.CurrentValue == nil || result.Product.CurrentValue.Amount != amount {
		t.Fatalf("Wails standalone valuation: %+v / %v", result, err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	if err := refStorage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	refReopened, _ := reopenBaselineProfile(t, refPaths)
	for _, copy := range []struct {
		app     *application.Service
		product domain.ProductContractID
		account domain.AccountID
	}{{reopened, id, ids.account}, {refReopened, refID, refIDs.account}} {
		copy.app.SetClock(clock)
		detail, err := copy.app.Product(ctx, copy.product)
		operations := managedOperations(t, copy.app, copy.product)
		if err != nil || detail.CurrentValue == nil || detail.CurrentValue.CanonicalAmount() != amount || detail.Contract.Revision != 1 ||
			len(operations) != 2 || operations[0].Kind != domain.ProductOpValueObservation || !operations[0].EffectiveAt.Equal(now) {
			t.Fatalf("valuation did not reopen exactly: %+v / %+v / %v", detail, operations, err)
		}
		assertNativeTradeCash(t, copy.app, copy.account, map[domain.CurrencyCode]string{"USD": "156"})
		assertNoStandaloneReconciliation(t, copy.app)
	}
	after, err := reopened.ListActivities(ctx, 100)
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatal("standalone valuation changed ledger activities")
	}
}

func TestStandaloneLockedProductRedemptionMatchesWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	ref, refStorage, refPaths, refIDs := openExistingBuyFixture(t)
	id := seedTestLockedProductWithUnlock(t, app, ids.account, nil, "2026-11-22")
	refID := seedTestLockedProductWithUnlock(t, ref, refIDs.account, nil, "2026-11-22")
	clock := linkedContractClock
	app.SetClock(clock)
	ref.SetClock(clock)
	reserveTermDeposit(t, app, id, "Synthetic first reserve")
	refA := reserveTermDeposit(t, ref, refID, "Synthetic first reserve")
	before, err := app.ListActivities(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}

	fault := &standaloneLifecycleFault{Service: app}
	h := newStandaloneLifecycleHarness(t, app, fault, clock)
	p := h.page
	const receipt = "Confirm actual receipt"
	const gross, fee, day, minute = "132.1234", "1.2222", "2026-11-23", "18:00"
	edit := func() *productReceiptDraft {
		t.Helper()
		h.scrollClick(receipt)
		d := p.receipt
		if d == nil || d.kind != domain.ProductLockedProduct || d.principal.Text() != "" || d.fee.Text() != "0" {
			t.Fatal("standalone locked-product receipt has the wrong defaults")
		}
		setBankField(h.previewHarness, "Gross proceeds · USD", &d.principal, gross)
		setBankField(h.previewHarness, "Fee (optional) · USD", &d.fee, fee)
		d.day, d.minute = day, minute
		h.clickButton("Review receipt")
		if d.plan == nil || d.plan.command.ReconciliationLink != nil || d.plan.command.Settle.GrossProceeds == nil ||
			d.plan.command.Settle.ReturnedPrincipal != nil || d.plan.command.Settle.Interest != nil {
			t.Fatal("locked receipt preview did not use the standalone gross proceeds")
		}
		assertPageLabel(t, &h.router, "Account cash after · 286.9012 USD")
		return d
	}
	d := edit()
	h.clickButton("Cancel")
	h.focused(&p.startConfirmation)
	if len(fault.receipts) != 0 || len(managedOperations(t, app, id)) != 1 {
		t.Fatal("cancel posted a receipt")
	}

	d = edit()
	stale := d.plan.mutation
	reserveTermDeposit(t, app, id, "Synthetic added reserve")
	refB := reserveTermDeposit(t, ref, refID, "Synthetic added reserve")
	h.focused(&d.confirm)
	h.press(key.NameReturn)
	h.settle()
	if d.plan != nil || d.problem != i18n.ProductReceiptStale || d.principal.Text() != gross || d.fee.Text() != fee || len(managedOperations(t, app, id)) != 1 {
		t.Fatal("stale receipt did not retain the draft without posting")
	}
	h.clickButton("Review receipt")
	if d.plan == nil || d.plan.mutation == stale || len(d.plan.review.ReservationReleaseDetails) != 2 {
		t.Fatal("fresh review did not pick up both active reservations")
	}

	api := liquidityapi.NewService(ref)
	command := liquidityapi.ProductCommandRequest{Kind: string(domain.ProductOpSettle), Settle: &application.SettleProductCommand{
		ProductID: refID.String(), GrossProceeds: productOptional(gross), Fee: productOptional(fee), EffectiveLocalDate: day, EffectiveLocalTime: minute,
		ReleaseReservationIDs: []string{refA.ID.String(), refB.ID.String()},
	}}
	want, err := api.PreviewProductOperation(ctx, command)
	got := liquidityapi.FromProductOperationPreview(d.plan.review)
	if err != nil || got.EffectiveAt != want.EffectiveAt || !reflect.DeepEqual(got.CashAfter, want.CashAfter) || !reflect.DeepEqual(got.NetWorthDelta, want.NetWorthDelta) {
		t.Fatalf("standalone review differs from Wails: %+v / %+v / %v", got, want, err)
	}
	refReceipt, err := api.RecordProductOperation(ctx, liquidityapi.RecordProductOperationRequest{Command: command,
		MutationID: domain.NewProductOperationID().String(), ReviewedStateHash: want.ReviewedStateHash})
	if err != nil {
		t.Fatal(err)
	}
	mutation := d.plan.mutation
	h.size = image.Pt(665, 480)
	h.settle()
	fault.loseReceipt = true
	h.press(key.NameReturn)
	h.settle()
	if !p.unresolved() || !d.uncertain {
		t.Fatal("unknown standalone receipt outcome was discarded")
	}
	h.focused(&d.replay)
	h.press(key.NameReturn)
	h.settle()
	if p.receipt != nil || p.saved != i18n.ProductReceiptSaved || len(fault.receipts) != 3 || !reflect.DeepEqual(fault.receipts[1], fault.receipts[2]) {
		t.Fatal("standalone receipt retry replaced the reviewed request")
	}
	h.size = image.Pt(900, 660)
	h.settle()
	h.scrollClick("Open account · Harbor Brokerage")
	h.scrollClick("Open product · Synthetic locked product")
	h.clickButton("Back")
	h.focused(h.accounts.productButtons[id])

	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	if err := refStorage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	refReopened, _ := reopenBaselineProfile(t, refPaths)
	for _, copy := range []struct {
		app     *application.Service
		product domain.ProductContractID
		account domain.AccountID
		op      domain.ProductOperationID
	}{{reopened, id, ids.account, mutation}, {refReopened, refID, refIDs.account, domain.ProductOperationID(refReceipt.OperationID)}} {
		copy.app.SetClock(clock)
		detail, err := readProduct(ctx, copy.app, copy.product)
		if err != nil || detail.detail.Contract.State != domain.ProductStateSettled || *detail.detail.Contract.ClosedOperationID != copy.op ||
			len(detail.detail.Reservations) != 0 || len(detail.settlement) != 1 {
			t.Fatalf("standalone receipt did not reopen: %+v / %v", detail, err)
		}
		activity := detail.settlement[0]
		if activity.Note != nil || activity.EffectiveAt.Format(time.RFC3339) != "2026-11-23T10:00:00Z" ||
			activity.TradeDetail.Gross.CanonicalAmount() != gross || activity.TradeDetail.Fee.CanonicalAmount() != fee ||
			activity.ProductContext.Purpose != domain.ProductPurposeRedemption {
			t.Fatalf("wrong standalone receipt details: %+v", activity)
		}
		assertNativeTradeCash(t, copy.app, copy.account, map[domain.CurrencyCode]string{"USD": "286.9012"})
		assertNoStandaloneReconciliation(t, copy.app)
	}
	after, err := reopened.ListActivities(ctx, 100)
	if err != nil || len(after) != len(before)+1 || !reflect.DeepEqual(after[1:], before) {
		t.Fatal("standalone redemption modified earlier activities")
	}
}
