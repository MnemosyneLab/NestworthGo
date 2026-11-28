package desktop

import (
	"context"
	"errors"
	"github.com/waltwang/nestworth-go/internal/testfixture"
	"image"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	liquidityapi "github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

type productReceiptAttempt struct {
	command  application.ProductCommand
	id, hash string
}

type productReceiptFault struct {
	*application.Service
	attempts         []productReceiptAttempt
	id               domain.ProductContractID
	started, unblock chan struct{}
	failRead         atomic.Bool
}

func (f *productReceiptFault) Product(ctx context.Context, id domain.ProductContractID) (application.ProductDetail, error) {
	if id == f.id && f.failRead.Load() {
		return application.ProductDetail{}, errors.New("synthetic read failure after acknowledged receipt")
	}
	return f.Service.Product(ctx, id)
}

func (f *productReceiptFault) RecordProductOperation(ctx context.Context, command application.ProductCommand, id, hash string) (application.ProductOperationReceipt, error) {
	f.attempts = append(f.attempts, productReceiptAttempt{command, id, hash})
	switch len(f.attempts) {
	case 2:
		close(f.started)
		<-f.unblock
		receipt, err := f.Service.RecordProductOperation(ctx, command, id, hash)
		if err == nil && receipt.OperationID != "" {
			return application.ProductOperationReceipt{}, errors.New("synthetic response lost after receipt committed")
		}
		return receipt, err
	default:
		return f.Service.RecordProductOperation(ctx, command, id, hash)
	}
}

func reserveTermDeposit(t *testing.T, app *application.Service, id domain.ProductContractID, label string) domain.LiquidityReservation {
	t.Helper()
	detail, err := app.Product(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	r, err := app.SaveLiquidityReservation(t.Context(), application.SaveReservationInput{
		Source: domain.HoldingSourceRef(detail.Contract.AccountID, detail.Contract.HoldingID), Label: label, Amount: "25",
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestStandaloneEarlyTermDepositReceiptMatchesWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, fixture := openManagedProductFixture(t)
	ref, refStorage, refPaths, reference := openManagedProductFixture(t)
	id, refID := fixture.ids["Winter Deposit"], reference.ids["Winter Deposit"]
	clock := func() time.Time { return accountTestClock }
	reserveTermDeposit(t, app, id, "Synthetic deposit reserve")
	refReserve := reserveTermDeposit(t, ref, refID, "Synthetic deposit reserve")
	const principal, interest, fee, day, minute = "987.6543", "4.5678", "1.2345", "2026-11-21", "18:15"
	before, err := app.ListActivities(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	restrict := productReceiptInput{principal: principal, interest: interest, fee: fee, day: day, minute: minute, fromTasks: true}
	if _, err := reviewProductReceipt(ctx, app, id, restrict, clock); !errors.Is(err, errProductReceiptUnavailable) {
		t.Fatalf("due-only Tasks receipt accepted an unmatured contract: %v", err)
	}
	unsafe := productReceiptInput{principal: principal, interest: interest, fee: fee, day: "2026-11-25", minute: minute}
	if _, err := reviewProductReceipt(ctx, app, id, unsafe, clock); productReceiptProblem(err) != i18n.ProductReceiptTimeInvalid {
		t.Fatalf("standalone receipt accepted a future time: %v", err)
	}
	defaultTime, err := reviewProductReceipt(ctx, app, id, productReceiptInput{principal: principal}, clock)
	if err != nil || !defaultTime.review.EffectiveAt.Equal(clock()) {
		t.Fatalf("standalone receipt did not freeze the default effective time: %+v / %v", defaultTime, err)
	}

	fault := &standaloneLifecycleFault{Service: app}
	funds := newAvailableFundsPage(ctx, app, func() {})
	funds.productPage = newProductDetailPage(ctx, app, func() {})
	p := funds.productPage
	p.receipts, p.clock = fault, clock
	accounts := newAccountsPage(ctx, app, clock, func() {})
	timeline := newLedgerHistory(ctx, app, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{funds: funds, accounts: accounts, history: timeline, version: app.MobileDataVersion})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 4 {
			h.frame()
			p.workers.Wait()
			funds.workers.Wait()
			accounts.workers.Wait()
			timeline.model.Wait()
			timeline.reads.workers.Wait()
		}
	}
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	scrollClick := func(label string) { t.Helper(); scrollProductControl(t, h, label); click(label) }
	settle()
	click("Available funds")
	chooseFundsSourceAction(t, h, "Winter Deposit · USD", "Open product")
	settle()
	edit := func() *productReceiptDraft {
		t.Helper()
		scrollClick("Confirm actual receipt")
		d := p.receipt
		if d == nil || d.fromTasks || d.earliest != "" || d.principal.Text() != "1000" || d.interest.Text() != "0" || d.fee.Text() != "0" {
			t.Fatal("standalone draft has the wrong date bounds or defaults")
		}
		setBankField(h, "Returned principal · USD", &d.principal, principal)
		setBankField(h, "Interest received · USD", &d.interest, interest)
		d.fee.Editor.SetText(fee)
		d.day, d.minute = day, minute
		click("Review receipt")
		assertPageLabel(t, &h.router, "Account cash after · 2090.9876 USD")
		return d
	}
	d := edit()
	click("Back to details")
	h.focused(&d.principal.Editor)
	if d.day != day || d.minute != minute || d.fee.Text() != fee {
		t.Fatal("Back discarded the proposed early receipt")
	}
	click("Review receipt")
	click("Cancel")
	h.focused(&p.startConfirmation)
	assertNativeTradeCash(t, app, fixture.account, map[domain.CurrencyCode]string{"USD": "1100"})
	if len(fault.receipts) != 0 || len(managedOperations(t, app, id)) != 1 {
		t.Fatal("Cancel posted an early receipt")
	}
	d = edit()
	reserveTermDeposit(t, app, id, "Synthetic added reserve")
	refAdded := reserveTermDeposit(t, ref, refID, "Synthetic added reserve")
	h.press(key.NameReturn)
	settle()
	if d.plan != nil || d.problem != i18n.ProductReceiptStale || d.day != day || len(managedOperations(t, app, id)) != 1 {
		t.Fatal("stale confirmation wrote or lost the early receipt draft")
	}
	click("Review receipt")
	api := liquidityapi.NewService(ref)
	request := liquidityapi.ProductCommandRequest{Kind: string(domain.ProductOpSettle), Settle: &application.SettleProductCommand{
		ProductID: refID.String(), ReturnedPrincipal: productOptional(principal), Interest: productOptional(interest), Fee: productOptional(fee),
		EffectiveLocalDate: day, EffectiveLocalTime: minute, ReleaseReservationIDs: []string{refReserve.ID.String(), refAdded.ID.String()},
	}}
	want, err := api.PreviewProductOperation(ctx, request)
	got := liquidityapi.FromProductOperationPreview(d.plan.review)
	if err != nil || got.EffectiveAt != want.EffectiveAt || got.LocalDate != want.LocalDate || !reflect.DeepEqual(got.CashAfter, want.CashAfter) ||
		!reflect.DeepEqual(got.NetWorthDelta, want.NetWorthDelta) || len(got.ReservationReleaseDetails) != 2 || d.plan.command.ReconciliationLink != nil {
		t.Fatalf("early receipt preview differs from Wails: %+v / %+v / %v", got, want, err)
	}
	refReceipt, err := api.RecordProductOperation(ctx, liquidityapi.RecordProductOperationRequest{Command: request, ReviewedStateHash: want.ReviewedStateHash, MutationID: domain.NewProductOperationID().String()})
	if err != nil {
		t.Fatal(err)
	}
	posting := d.plan.mutation
	h.size = image.Pt(665, 480)
	settle()
	fault.loseReceipt = true
	h.press(key.NameReturn)
	settle()
	h.focused(&d.replay)
	h.press(key.NameReturn)
	settle()
	if p.receipt != nil || p.saved != i18n.ProductReceiptSaved || len(fault.receipts) != 3 || !reflect.DeepEqual(fault.receipts[1], fault.receipts[2]) {
		t.Fatal("early receipt retry did not reuse its exact command")
	}
	scrollClick("Open principal receipt")
	if h.shell.trail.current().target.activity != p.current.settlement[0].ID {
		t.Fatal("principal link opened the wrong activity")
	}
	click("Back")
	h.focused(p.activities[p.current.settlement[0].ID])
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
		op      domain.ProductOperationID
	}{{app, id, fixture.account, posting}, {ref, refID, reference.account, domain.ProductOperationID(refReceipt.OperationID)}} {
		got, err := readProduct(ctx, c.app, c.id)
		if err != nil || got.detail.Contract.State != domain.ProductStateSettled || *got.detail.Contract.ClosedOperationID != c.op ||
			got.detail.Contract.Principal.CanonicalAmount() != "1000" || len(got.detail.Reservations) != 0 || len(got.settlement) != 2 {
			t.Fatalf("early receipt did not reopen: %+v / %v", got, err)
		}
		sell, income := got.settlement[0], got.settlement[1]
		if sell.TradeDetail.Gross.CanonicalAmount() != principal || sell.TradeDetail.Fee.CanonicalAmount() != fee ||
			sell.EffectiveAt.Format(time.RFC3339) != "2026-11-21T10:15:00Z" || !income.EffectiveAt.Equal(sell.EffectiveAt) ||
			sell.ProductContext.Purpose != domain.ProductPurposeRedemption || income.ProductContext.Purpose != domain.ProductPurposeInterest ||
			!reflect.DeepEqual(settlementEffects(income), []string{"external_inflow/added/4.5678 USD"}) {
			t.Fatalf("incorrect linked principal or interest: %+v / %+v", sell, income)
		}
		assertNativeTradeCash(t, c.app, c.account, map[domain.CurrencyCode]string{"USD": "2090.9876"})
		assertNoStandaloneReconciliation(t, c.app)
	}
	after, err := app.ListActivities(ctx, 50)
	if err != nil || len(after) != len(before)+2 {
		t.Fatal("early receipt did not add exactly two activities")
	}
}

func TestManagedTermDepositMaturityReceiptMatchesWails(t *testing.T) {
	ctx := t.Context()
	assertCode := func(err error, code domain.ErrorCode) {
		t.Helper()
		var issue *domain.Error
		if !errors.As(err, &issue) || issue.Code != code {
			t.Fatalf("got %v, want %s", err, code)
		}
	}
	app, storage, paths, fixture := openManagedProductFixture(t)
	ref, _, _, reference := openManagedProductFixture(t)
	id, refID := fixture.ids["Due Deposit"], reference.ids["Due Deposit"]
	clock := &atomic.Int64{}
	clock.Store(accountTestClock.UnixNano())
	now := func() time.Time { return time.Unix(0, clock.Load()).UTC() }
	testfixture.SetClock(app, now)
	reserveTermDeposit(t, app, id, "Rent reserve")
	refRent := reserveTermDeposit(t, ref, refID, "Rent reserve")
	refEmergency := reserveTermDeposit(t, ref, refID, "Emergency reserve")

	input := productReceiptInput{principal: "595.125", interest: "4.25", fee: "0.375", day: "2026-11-21", minute: "18:15", fromTasks: true}
	if _, err := reviewProductReceipt(ctx, app, fixture.ids["Winter Deposit"], input, now); !errors.Is(err, errProductReceiptUnavailable) {
		t.Fatalf("unmatured contract was available: %v", err)
	}
	if _, err := reviewProductReceipt(ctx, app, fixture.ids["Settled Deposit"], input, now); !errors.Is(err, errProductReceiptUnavailable) {
		t.Fatalf("settled contract was available: %v", err)
	}
	badTime := input
	badTime.day = "2026-11-20"
	_, err := reviewProductReceipt(ctx, app, id, badTime, now)
	assertCode(err, domain.ErrInvalidChangeTime)
	badTime.day = "2026-11-22"
	badTime.minute = "23:00"
	_, err = reviewProductReceipt(ctx, app, id, badTime, now)
	assertCode(err, domain.ErrInvalidChangeTime)
	tooMuch := input
	tooMuch.principal = "600.0001"
	_, err = reviewProductReceipt(ctx, app, id, tooMuch, now)
	assertCode(err, domain.ErrValidation)
	nowPlan, err := reviewProductReceipt(ctx, app, id, productReceiptInput{principal: input.principal, fromTasks: true}, now)
	if err != nil || nowPlan.command.Settle == nil || nowPlan.command.Settle.EffectiveAt != now().Format(time.RFC3339Nano) ||
		!nowPlan.review.EffectiveAt.Equal(now()) {
		t.Fatalf("the default time was not frozen for confirmation: %+v / %v", nowPlan, err)
	}

	fault := &productReceiptFault{Service: app, id: id, started: make(chan struct{}), unblock: make(chan struct{})}
	unblock := sync.OnceFunc(func() { close(fault.unblock) })
	funds := newAvailableFundsPage(ctx, app, func() {})
	funds.productPage = newProductDetailPage(ctx, fault, func() {})
	p := funds.productPage
	p.receipts, p.clock = fault, now
	timeline := newLedgerHistory(ctx, app, now, func() {})
	accounts := newAccountsPage(ctx, app, now, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{funds: funds, accounts: accounts, history: timeline, version: app.MobileDataVersion})
	refresh := p.onProductSaved
	p.onProductSaved = func() {
		fault.failRead.Store(true)
		refresh()
	}
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	drain := func() {
		funds.workers.Wait()
		p.workers.Wait()
		accounts.workers.Wait()
		timeline.model.Wait()
		timeline.reads.workers.Wait()
	}
	t.Cleanup(func() { unblock(); drain() })
	finish := func() {
		for range 3 {
			h.frame()
			drain()
		}
		h.frame()
	}
	click := func(label string) { t.Helper(); h.click(label, false); finish() }
	label := func(text string) { t.Helper(); assertPageLabel(t, &h.router, text) }

	initial, err := app.ListActivities(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	h.frame()
	click("Available funds")
	chooseFundsSourceAction(t, h, "Due Deposit · USD", "Open product")
	finish()
	label("Estimated net proceeds · 606.25 USD")
	label("Estimated unpaid interest at maturity · 6.25 USD")
	click("Confirm actual receipt")
	d := p.receipt
	if d == nil || d.principal.Text() != "600" || d.interest.Text() != "0" || d.fee.Text() != "0" {
		t.Fatal("draft defaults differ from Wails")
	}
	setBankField(h, "Returned principal · USD", &d.principal, input.principal)
	setBankField(h, "Interest received · USD", &d.interest, input.interest)
	d.fee.Editor.SetText(input.fee)
	d.day, d.minute = input.day, input.minute
	click("Review receipt")
	label("Returned principal · 595.125 USD")
	label("Interest received · 4.25 USD")
	label("Account cash before · 1100 USD")
	label("Account cash after · 1699 USD")
	firstPlan := *d.plan
	reserveTermDeposit(t, app, id, "Emergency reserve")
	click("Post actual receipt")
	label("The receipt preview is stale. Review the actual amounts again.")
	if d.plan != nil || len(fault.attempts) != 1 || d.principal.Text() != input.principal {
		t.Fatal("stale confirmation did not return to the original draft")
	}
	unposted, err := app.ReadProductOperationReceipt(ctx, firstPlan.mutation)
	if unposted.OperationID != "" {
		t.Fatal("stale plan posted")
	}
	assertCode(err, domain.ErrNotFound)

	click("Review receipt")
	if d.plan.mutation == firstPlan.mutation || len(d.plan.review.ReservationReleaseDetails) != 2 {
		t.Fatal("new preview did not include the current reservation releases")
	}
	compare := liquidityapi.NewService(ref)
	command := liquidityapi.ProductCommandRequest{Kind: "settle", Settle: &application.SettleProductCommand{
		ProductID: refID.String(), ReturnedPrincipal: &input.principal, Interest: &input.interest, Fee: &input.fee,
		EffectiveLocalDate: input.day, EffectiveLocalTime: input.minute,
		ReleaseReservationIDs: []string{refRent.ID.String(), refEmergency.ID.String()},
	}}
	wantPreview, err := compare.PreviewProductOperation(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	gotPreview := liquidityapi.FromProductOperationPreview(d.plan.review)
	if gotPreview.EffectiveAt != wantPreview.EffectiveAt || gotPreview.Timezone != wantPreview.Timezone ||
		!reflect.DeepEqual(gotPreview.CashBefore, wantPreview.CashBefore) || !reflect.DeepEqual(gotPreview.CashAfter, wantPreview.CashAfter) ||
		!reflect.DeepEqual(gotPreview.ProductAfter, wantPreview.ProductAfter) || !reflect.DeepEqual(gotPreview.NetWorthDelta, wantPreview.NetWorthDelta) ||
		len(gotPreview.Activities) != 2 || len(wantPreview.Activities) != 2 {
		t.Fatalf("Go review differs from Wails: %+v / %+v", gotPreview, wantPreview)
	}
	wantReceipt, err := compare.RecordProductOperation(ctx, liquidityapi.RecordProductOperationRequest{
		Command: command, ReviewedStateHash: wantPreview.ReviewedStateHash, MutationID: domain.NewProductOperationID().String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	post := semanticBounds(t, &h.router, "Post actual receipt", false)
	point := post.Min.Add(post.Max).Div(2)
	h.clickAt(point)
	select {
	case <-fault.started:
	case <-time.After(5 * time.Second):
		t.Fatal("confirmation did not reach the writer")
	}
	h.clickAt(point)
	unblock()
	finish()
	if !d.uncertain || len(fault.attempts) != 2 {
		t.Fatalf("double click or lost-response handling: %d, %+v", len(fault.attempts), d)
	}
	mutation := d.plan.mutation
	clock.Add(int64(35 * time.Second))
	click("Retry same submission")
	label("Actual receipt recorded.")
	label("Could not load product details")
	fault.failRead.Store(false)
	if !reflect.DeepEqual(fault.attempts[1], fault.attempts[2]) {
		t.Fatal("recovery changed the command, mutation, or reviewed state")
	}
	click("Retry")
	label("Term deposit · Settled")
	if findPageLabel(&h.router, "Confirm actual receipt") != nil || len(fault.attempts) != 3 {
		t.Fatal("a posted receipt could be submitted again or refresh called the writer")
	}
	receipt, err := app.ReadProductOperationReceipt(ctx, mutation)
	if err != nil || len(receipt.ActivityIDs) != 2 || p.current.detail.Contract.ClosedOperationID == nil || *p.current.detail.Contract.ClosedOperationID != mutation {
		t.Fatalf("unexpected durable receipt: %+v / %v", receipt, err)
	}
	click("Open interest receipt")
	if active := h.shell.trail.current().target.activity; active != receipt.ActivityIDs[1] {
		t.Fatalf("interest history link opened %s", active)
	}
	click("Open product · Due Deposit")
	label("Term deposit · Settled")
	click("Open principal receipt")
	if active := h.shell.trail.current().target.activity; active != receipt.ActivityIDs[0] {
		t.Fatalf("principal history link opened %s", active)
	}

	check := func(app *application.Service) {
		t.Helper()
		got, err := readProduct(ctx, app, id)
		other, refErr := compare.Product(ctx, refID.String())
		if err != nil || refErr != nil || got.detail == nil || got.detail.DisplayState != domain.ProductDisplaySettled ||
			other.Product.DisplayState != string(got.detail.DisplayState) || !slices.Equal(got.detail.PermittedActions, other.PermittedActions) ||
			got.detail.CurrentValue == nil || !got.detail.CurrentValue.IsZero() || other.Product.CurrentValue == nil || other.Product.CurrentValue.Amount != "0" ||
			len(got.settlement) != 2 || got.settlement[0].ID != receipt.ActivityIDs[0] || got.settlement[1].ID != receipt.ActivityIDs[1] {
			t.Fatalf("closed product differs: %+v / %+v, %v / %v", got, other, err, refErr)
		}
		holdings, err := app.HoldingsByAccounts(ctx, []domain.AccountID{fixture.account})
		if err != nil || !slices.ContainsFunc(holdings[fixture.account], func(h domain.Holding) bool {
			return h.ID == got.detail.Contract.HoldingID && h.Quantity.IsZero()
		}) {
			t.Fatalf("holding did not close: %+v / %v", holdings, err)
		}
		for i, activity := range got.settlement {
			other, err := ref.Activity(ctx, domain.ActivityID(wantReceipt.ActivityIDs[i]))
			if err != nil || activity.Kind != other.Kind || activity.Reason != other.Reason || !activity.EffectiveAt.Equal(other.EffectiveAt) ||
				activity.ProductContext == nil || other.ProductContext == nil || activity.ProductContext.Purpose != other.ProductContext.Purpose ||
				activity.ProductContext.OperationID != mutation || !reflect.DeepEqual(settlementEffects(activity), settlementEffects(other)) {
				t.Fatalf("activity %d differs: %+v / %+v / %v", i, activity, other, err)
			}
		}
		if g := settlementEffects(got.settlement[0]); !slices.Contains(g, "trade_principal/added/595.125 USD") ||
			!slices.Contains(g, "fee/removed/0.375 USD") {
			t.Fatalf("unexpected principal or fee classification: %+v", g)
		}
		if g := settlementEffects(got.settlement[1]); !reflect.DeepEqual(g, []string{"external_inflow/added/4.25 USD"}) {
			t.Fatalf("actual interest raw effect differs from Wails: %+v", g)
		}
		funds, err := readAvailableFunds(ctx, app)
		otherFunds, refErr := compare.Overview(ctx, liquidityapi.OverviewRequest{})
		i := slices.IndexFunc(otherFunds.Buckets, func(b liquidityapi.LiquidityBucketDTO) bool { return b.HorizonOn == otherFunds.LocalDate })
		if err != nil || refErr != nil || i < 0 || funds.today.KnownAvailableSubtotal == nil || funds.today.KnownAvailableSubtotal.CanonicalAmount() != "1699" ||
			otherFunds.Buckets[i].KnownAvailableSubtotal == nil || otherFunds.Buckets[i].KnownAvailableSubtotal.Amount != "1699" {
			t.Fatalf("available cash differs: %+v / %+v, %v / %v", funds.today, otherFunds.Buckets, err, refErr)
		}
		rows, err := app.ListActivities(ctx, 20)
		if err != nil || len(rows) != len(initial)+2 {
			t.Fatalf("expected exactly two additional activities: %d / %v", len(rows), err)
		}
		for _, service := range []*application.Service{app, ref} {
			attribution, err := service.AssetChange(ctx, domain.AnalysisQuery{
				Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-11-19", To: "2026-11-21",
				Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true,
			})
			if err != nil || attribution.Status != domain.CompletenessOK {
				t.Fatalf("could not compare external-flow attribution: %+v / %v", attribution.AnalysisAvailability, err)
			}
			income := ""
			for _, bucket := range attribution.Waterfall {
				if bucket.Bucket == domain.BucketExternalFlow && bucket.Amount != nil && !bucket.Amount.IsZero() {
					t.Fatalf("product receipt became an external flow: %+v", bucket)
				}
				if bucket.Bucket == domain.BucketDividendInterest && bucket.Amount != nil {
					income = bucket.Amount.CanonicalAmount()
				}
			}
			if income != "4.25" {
				t.Fatalf("product receipt income = %s, want 4.25", income)
			}
		}
		reservations, err := app.ListLiquidityReservations(ctx)
		if err != nil || len(reservations) != 2 || reservations[0].Active() || reservations[1].Active() {
			t.Fatalf("reviewed reservations did not release atomically: %+v / %v", reservations, err)
		}
	}
	check(app)
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	check(reopened)
	same, err := reopened.RecordProductOperation(ctx, fault.attempts[1].command, mutation.String(), fault.attempts[1].hash)
	if err != nil || !same.Replayed || !slices.Equal(same.ActivityIDs, receipt.ActivityIDs) {
		t.Fatalf("reopen lost idempotent receipt: %+v / %v", same, err)
	}
}

func settlementEffects(activity domain.Activity) []string {
	var facts []string
	for _, effect := range activity.Effects {
		if effect.Money != nil {
			facts = append(facts, string(effect.Classification)+"/"+string(effect.Direction)+"/"+
				effect.Money.CanonicalAmount()+" "+effect.Money.Currency().String())
		}
	}
	return facts
}
