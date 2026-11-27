package desktop

import (
	"context"
	"errors"
	"image"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/pointer"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	liquidityapi "github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

func seedAdditionalTaskProducts(ctx context.Context, service *application.Service, fixture managedProductFixture, at time.Time) error {
	api := liquidityapi.NewService(service)
	calendar, zero := "calendar", "0"
	for i, item := range []struct {
		name, maturity string
		days           int
	}{
		{"Awaiting Deposit", "2026-11-21", 3},
		{"Boundary Deposit", "2026-12-22", 0},
		{"Outside Deposit", "2026-12-23", 0},
		{"Same Day Deposit", "2026-11-22", 0},
	} {
		command := liquidityapi.ProductCommandRequest{Kind: "open", Open: &application.OpenProductCommand{
			AccountID: fixture.account.String(), Currency: "USD", Principal: "100",
			EffectiveAt: at.Add(time.Duration(20+i)*time.Minute - 60*time.Hour).Format(time.RFC3339),
			Terms: application.ProductTermsInput{Kind: "term_deposit", Name: item.name, StartOn: "2026-11-19",
				MaturityOn: &item.maturity, InterestMode: "none"},
			Policy: application.ProductPolicyInput{AccessKind: "on_date", UnlockOn: &item.maturity, EarlyKind: "not_allowed",
				SettlementDays: &item.days, DayBasis: &calendar, NormalExitFee: &zero},
		}}
		preview, err := api.PreviewProductOperation(ctx, command)
		if err != nil {
			return err
		}
		receipt, err := api.RecordProductOperation(ctx, liquidityapi.RecordProductOperationRequest{
			Command: command, ReviewedStateHash: preview.ReviewedStateHash, MutationID: domain.NewProductOperationID().String(),
		})
		if err != nil {
			return err
		}
		fixture.ids[item.name] = domain.ProductContractID(receipt.ProductIDs[0])
	}
	return nil
}

type firstTasksReadFailure struct {
	*application.Service
	failed atomic.Bool
}

func (s *firstTasksReadFailure) UpcomingActions(ctx context.Context) (domain.UpcomingActions, error) {
	if !s.failed.Swap(true) {
		return domain.UpcomingActions{}, errors.New("synthetic tasks read failure")
	}
	return s.Service.UpcomingActions(ctx)
}

func recordTaskFixtureReceipt(t *testing.T, s *application.Service, id domain.ProductContractID, amount string) {
	t.Helper()
	ctx := t.Context()
	api := liquidityapi.NewService(s)
	zero := "0"
	cmd := liquidityapi.ProductCommandRequest{Kind: "settle", Settle: &application.SettleProductCommand{
		ProductID: id.String(), ReturnedPrincipal: &amount, Interest: &zero, Fee: &zero,
		EffectiveAt: accountTestClock.Format(time.RFC3339Nano),
	}}
	preview, err := api.PreviewProductOperation(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.RecordProductOperation(ctx, liquidityapi.RecordProductOperationRequest{
		Command: cmd, ReviewedStateHash: preview.ReviewedStateHash, MutationID: domain.NewProductOperationID().String(),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTermDepositTasksMatchWailsAndRouteByProduct(t *testing.T) {
	ctx := t.Context()
	app, session, paths, fixture := openManagedProductFixture(t)
	ref, _, _, reference := openManagedProductFixture(t)
	for _, entry := range []struct {
		s *application.Service
		f managedProductFixture
	}{{app, fixture}, {ref, reference}} {
		if err := seedAdditionalTaskProducts(ctx, entry.s, entry.f, accountTestClock); err != nil {
			t.Fatal(err)
		}
	}
	wails := liquidityapi.NewService(ref)
	check := func(data *domain.UpcomingActions, extraSettled ...string) {
		t.Helper()
		want, err := wails.UpcomingActions(ctx)
		if err != nil || data == nil {
			t.Fatalf("Tasks read: %+v / %v", data, err)
		}
		if data.LocalDate != "2026-11-22" || data.LocalDate != want.LocalDate || data.WindowStartOn != want.WindowStartOn ||
			data.WindowEndOn != "2026-12-22" || data.WindowEndOn != want.WindowEndOn || data.Timezone != want.Timezone ||
			data.AsOf.UTC().Format(time.RFC3339Nano) != want.AsOf {
			t.Fatalf("inconsistent Tasks window: %s to %s (%s) at %v, want %s to %s (%s) at %s",
				data.WindowStartOn, data.WindowEndOn, data.Timezone, data.AsOf, want.WindowStartOn, want.WindowEndOn, want.Timezone, want.AsOf)
		}
		termItems := termDepositTasks(data.Items)
		wantItems := slices.DeleteFunc(want.Items, func(row liquidityapi.UpcomingActionDTO) bool {
			return row.Kind != "product_schedule" || row.ProductKind == nil || *row.ProductKind != "term_deposit" ||
				(row.Group != "scheduled" && row.Group != "attention")
		})
		phases := map[string]string{"Due Deposit": "receipt_due", "Same Day Deposit": "receipt_due", "Winter Deposit": "upcoming",
			"Boundary Deposit": "upcoming", "Awaiting Deposit": "awaiting_receipt", "Incomplete Deposit": "blocked"}
		for _, name := range extraSettled {
			delete(phases, name)
		}
		if len(termItems) != len(phases) || len(termItems) != len(wantItems) {
			t.Fatalf("unexpected term deposit tasks: %+v", termItems)
		}
		termProjection := *data
		termProjection.Items = termItems
		dto := liquidityapi.FromUpcomingActions(termProjection)
		for i, item := range dto.Items {
			ref := wantItems[i]
			if item.DisplayName != ref.DisplayName || item.Phase != phases[item.DisplayName] || item.Phase != ref.Phase ||
				item.Group != ref.Group || item.Priority != ref.Priority || item.ReceiptBasis != ref.ReceiptBasis ||
				item.AmountStatus != ref.AmountStatus || !reflect.DeepEqual(item.Amount, ref.Amount) ||
				!reflect.DeepEqual(item.SortOn, ref.SortOn) || !reflect.DeepEqual(item.DaysFromToday, ref.DaysFromToday) ||
				!reflect.DeepEqual(item.EstimatedReceiptOn, ref.EstimatedReceiptOn) || !reflect.DeepEqual(item.MaturityOn, ref.MaturityOn) ||
				!slices.Equal(item.ReasonCodes, ref.ReasonCodes) {
				t.Fatalf("task facts or order differ: %+v / %+v", item, ref)
			}
			if id := taskProductID(termItems[i]); id == "" || id != fixture.ids[item.DisplayName] ||
				ref.PrimaryTarget.ProductID == nil || *ref.PrimaryTarget.ProductID != reference.ids[item.DisplayName].String() {
				t.Fatalf("task does not target the exact product: %+v / %+v", item, ref)
			}
			switch item.DisplayName {
			case "Due Deposit":
				if item.Amount == nil || item.Amount.Amount != "606.25" || item.Amount.Currency != "USD" || item.DaysFromToday == nil || *item.DaysFromToday != -1 {
					t.Fatalf("incorrect overdue forecast: %+v", item)
				}
			case "Incomplete Deposit":
				if item.EstimatedReceiptOn != nil || item.Amount != nil || !slices.Contains(item.ReasonCodes, "unknown_receipt_timing") {
					t.Fatalf("incomplete task acquired a date or known amount: %+v", item)
				}
			}
		}
	}

	clock := func() time.Time { return accountTestClock }
	tasks := newTasksPage(ctx, &firstTasksReadFailure{Service: app}, func() {})
	funds := newAvailableFundsPage(ctx, app, func() {})
	product := newProductDetailPage(ctx, &oneProductReadFailure{Service: app}, func() {})
	product.receipts, product.clock = app, clock
	funds.productPage = product
	// Keep the active list snapshot fixed in this harness to reproduce a
	// settlement between the list read and a click on an old receipt row.
	h := newShellHarnessWithPages(t, ledgerPages{tasks: tasks, funds: funds})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	drain := func() {
		tasks.workers.Wait()
		product.workers.Wait()
		funds.workers.Wait()
	}
	t.Cleanup(drain)
	click := func(text string) {
		t.Helper()
		h.click(text, false)
		for range 3 {
			h.frame()
			drain()
		}
		h.frame()
	}
	label := func(text string) { t.Helper(); assertPageLabel(t, &h.router, text) }
	scrollToTop := func() {
		for range 8 {
			h.router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(470, 465), Scroll: f32.Pt(0, -240)})
			h.frame()
		}
	}
	before, err := app.ListActivities(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}
	version := app.MobileDataVersion()
	h.frame()
	click("Tasks & checks")
	label("Could not load upcoming actions.")
	click("Retry")
	label("Upcoming window · 2026-11-22 to 2026-12-22 (Asia/Singapore)")
	check(tasks.current)

	const due = "Verify receipt · Due Deposit"
	scrollProductControl(t, h, due)
	click(due)
	label("Could not load product details")
	click("Retry")
	label("Confirm actual receipt")
	if product.receipt == nil || product.receipt.principal.Text() != "600" || product.id != fixture.ids["Due Deposit"] {
		t.Fatal("receipt intent did not open the existing draft for the exact product")
	}
	click("Back")
	idx := slices.IndexFunc(tasks.current.Items, func(row domain.UpcomingAction) bool { return row.DisplayName == "Due Deposit" })
	h.focused(&tasks.links[tasks.current.Items[idx].ID].receipt)
	click("Forward")
	if product.receipt != nil {
		t.Fatal("Forward repeated a consumed receipt intent")
	}
	label("Estimated net proceeds · 606.25 USD")
	click("Back")
	const blocked = "Open product · Incomplete Deposit"
	scrollProductControl(t, h, blocked)
	label("Receipt schedule incomplete")
	label("Estimated receipt date · Unavailable")
	label("Access or receipt timing is unknown.")
	click(blocked)
	label("Term deposit · Needs information")
	if product.id != fixture.ids["Incomplete Deposit"] || product.receipt != nil {
		t.Fatal("blocked task did not open the product as read-only")
	}
	click("Back")
	// Use the scheduled card's stable ID, then restore the long list's scroll and focus.
	const scheduled = "Open product · Boundary Deposit"
	scrollProductControl(t, h, scheduled)
	click(scheduled)
	label("Maturity date · 2026-12-22")
	if product.id != fixture.ids["Boundary Deposit"] || product.receipt != nil {
		t.Fatal("scheduled task routed to the wrong product or opened a draft")
	}
	click("Back")
	idx = slices.IndexFunc(tasks.current.Items, func(row domain.UpcomingAction) bool { return row.DisplayName == "Boundary Deposit" })
	h.focused(&tasks.links[tasks.current.Items[idx].ID].product)
	if after, err := app.ListActivities(ctx, 30); err != nil || !reflect.DeepEqual(before, after) || app.MobileDataVersion() != version {
		t.Fatalf("list, product navigation or opening a draft changed the ledger: %v", err)
	}

	// These two explicit settlements are outside the read-only baseline.
	// First test a stale due row whose contract already closed elsewhere.
	scrollToTop()
	click("Reload")
	recordTaskFixtureReceipt(t, app, fixture.ids["Same Day Deposit"], "100")
	recordTaskFixtureReceipt(t, ref, reference.ids["Same Day Deposit"], "100")
	const stale = "Verify receipt · Same Day Deposit"
	scrollProductControl(t, h, stale)
	click(stale)
	label("Term deposit · Settled")
	label("The product can no longer be settled from this screen. Check its current status.")
	if product.receipt != nil {
		t.Fatal("stale list started a new receipt for a settled product")
	}
	click("Back")
	check(tasks.current, "Same Day Deposit")

	scrollToTop()
	scrollProductControl(t, h, due)
	click(due)
	click("Review receipt")
	click("Post actual receipt")
	label("Actual receipt recorded.")
	recordTaskFixtureReceipt(t, ref, reference.ids["Due Deposit"], "600")
	click("Back")
	check(tasks.current, "Same Day Deposit", "Due Deposit")
	if activities, err := app.ListActivities(ctx, 30); err != nil || len(activities) != len(before)+2 {
		t.Fatalf("expected only the two explicit settlement activities: %d / %v", len(activities), err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	data, err := reopened.UpcomingActions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	check(&data, "Same Day Deposit", "Due Deposit")
}

func TestTermDepositTasksEmpty(t *testing.T) {
	ctx := t.Context()
	service, _ := reopenBaselineProfile(t, prepareOnboardingProfile(t))
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Synthetic empty tasks", BaseCurrency: "USD", MemberNames: []string{"Ana Example"},
	}); err != nil {
		t.Fatal(err)
	}
	tasks := newTasksPage(ctx, service, func() {})
	t.Cleanup(tasks.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{tasks: tasks})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	h.click("Tasks & checks", false)
	tasks.workers.Wait()
	h.frame()
	assertPageLabel(t, &h.router, "No upcoming actions in this view.")
}

func TestProductTaskKeepsDueVerificationRestrictedToTermDeposits(t *testing.T) {
	term, locked := domain.ProductTermDeposit, domain.ProductLockedProduct
	a := domain.UpcomingAction{ProductKind: &term, Kind: "product_schedule", Phase: "receipt_due"}
	if !taskReceiptDue(a) {
		t.Fatal("a due term deposit lost receipt verification")
	}
	a.ProductKind = &locked
	if taskReceiptDue(a) {
		t.Fatal("a locked product must open its detail without a term-deposit receipt shortcut")
	}
	aID, bID := domain.NewProductContractID(), domain.NewProductContractID()
	a.ProductID, a.PrimaryTarget = &aID, domain.UpcomingTarget{Kind: "product", ProductID: &bID}
	if taskProductID(a) != "" {
		t.Fatal("mismatched target should not yield a link")
	}
}

// The existing end-to-end comparison checks the term-deposit subset on both
// projections. Upcoming now also presents account liquidity and reservations.
func termDepositTasks(items []domain.UpcomingAction) []domain.UpcomingAction {
	var terms []domain.UpcomingAction
	for _, row := range items {
		if row.Kind == "product_schedule" && row.ProductKind != nil && *row.ProductKind == domain.ProductTermDeposit &&
			(row.Group == "attention" || row.Group == "scheduled") {
			terms = append(terms, row)
		}
	}
	return terms
}
