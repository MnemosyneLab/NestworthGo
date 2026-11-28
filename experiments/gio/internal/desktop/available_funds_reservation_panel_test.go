package desktop

import (
	"context"
	"fmt"
	"image"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestAvailableFundsReservationPanelCancelRestoresFocusAndVisit(t *testing.T) {
	app, _, _, _ := openFundsFixture(t, true)
	funds, h, settle := reservationHarness(t, app, app)
	h.size = image.Pt(1200, 960)
	settle()
	click := func(label string) {
		t.Helper()
		h.click(label, false)
		settle()
	}
	records, err := app.ListLiquidityReservations(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	click("Within 30 days")
	click(fmt.Sprintf("Missing rules %d", fundsSourceCount(funds.current.report.Sources, funds.state.horizonOn, fundsNeedsRules)))
	before := funds.state
	if before.filter != fundsNeedsRules || before.horizonOn != fundsPresetDay(funds.localDate, 30) {
		t.Fatalf("could not select the test's date and filter: %+v", before)
	}
	click("Manage reservations")
	r := funds.reservation
	if r == nil || r.source.Kind != "" || !h.shell.fundsReservationOpen() {
		t.Fatalf("global Reservations panel did not open: %+v", r)
	}
	title := assertPageLabel(t, &h.router, "Reservations")
	closeButton := semanticBounds(t, &h.router, "Close", false)
	if title.Min.X < h.size.X/2 || closeButton.Min.X <= title.Min.X || closeButton.Max.X > h.size.X {
		t.Fatalf("Reservations was not presented at the right edge: title %v, close %v", title, closeButton)
	}
	h.focused(&r.add)
	click("Add reservation")
	h.focused(&r.picker.Trigger)
	assertPageLabel(t, &h.router, "Reservation source")
	click("Cancel")
	if funds.reservation != r || r.editing {
		t.Fatal("Cancel left the reservation list instead of returning to it")
	}
	h.focused(&r.add)

	h.press(key.NameEscape)
	settle()
	if h.shell.fundsReservationOpen() || h.shell.activeRoute() != fundsRoute || funds.state != before {
		t.Fatalf("Escape changed the Funds visit: %v, %+v", h.shell.activeRoute(), funds.state)
	}
	h.focused(&funds.manageReservations)
	click("Back")
	if h.shell.activeRoute() == fundsRoute {
		t.Fatal("closing the panel added a navigation entry")
	}
	click("Forward")
	if h.shell.activeRoute() != fundsRoute || h.shell.fundsReservationOpen() || funds.state != before {
		t.Fatalf("Forward did not restore the closed, filtered Funds visit: %+v", funds.state)
	}
	after, err := app.ListLiquidityReservations(t.Context())
	if err != nil || !reflect.DeepEqual(records, after) {
		t.Fatalf("cancel changed the reservations: %+v, %v", after, err)
	}
}

type reservationPanelBlockingLedger struct {
	*application.Service
	started, proceed chan struct{}
	saves            atomic.Int32
}

func (l *reservationPanelBlockingLedger) SaveLiquidityReservation(ctx context.Context, in application.SaveReservationInput) (domain.LiquidityReservation, error) {
	if l.saves.Add(1) == 1 {
		close(l.started)
		select {
		case <-l.proceed:
		case <-ctx.Done():
			return domain.LiquidityReservation{}, ctx.Err()
		}
	}
	return l.Service.SaveLiquidityReservation(ctx, in)
}

func TestAvailableFundsReservationPanelScopedWriteAndReleasedHistory(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, _ := openFundsFixture(t, true)
	boot, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := app.CreateAccount(ctx, application.AccountInput{
		Name: "AUD Cash", AccountType: "bank_account", TrackingMode: "balance", BalanceSheetRole: "asset",
		DefaultCurrency: "AUD", InitialAmount: "1000", OwnerIDs: []domain.MemberID{boot.Members[0].ID}, IncludeInNetWorth: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.SetFXPreference(ctx, "AUD", "USD", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.AppendManualFXQuote(ctx, "AUD", "USD", "0.625", ""); err != nil {
		t.Fatal(err)
	}
	const purpose = "QA AUD reserve"
	ref := domain.AccountValueSourceRef(account.Account.ID)
	ledger := &reservationPanelBlockingLedger{Service: app, started: make(chan struct{}), proceed: make(chan struct{})}
	proceed := sync.OnceFunc(func() { close(ledger.proceed) })
	funds, h, settle := reservationHarness(t, app, ledger)
	t.Cleanup(func() { proceed(); funds.workers.Wait() })
	h.size = image.Pt(1200, 960)
	settle()
	click := func(label string) {
		t.Helper()
		h.click(label, false)
		settle()
	}
	verify := func(rows []domain.LiquidityReservation, revision int, amount string, active bool) domain.LiquidityReservation {
		t.Helper()
		matches := slices.DeleteFunc(slices.Clone(rows), func(row domain.LiquidityReservation) bool { return row.Label != purpose })
		if len(matches) != 1 {
			t.Fatalf("expected a single reservation for %q, got %+v", purpose, matches)
		}
		row := matches[0]
		if row.Source.Key() != ref.Key() || row.Currency != "AUD" || row.Revision != revision ||
			row.Amount.CanonicalAmount() != amount || row.Active() != active {
			t.Fatalf("wrong persisted reservation at revision %d: %+v", revision, row)
		}
		return row
	}
	checkSaved := func(revision int, amount string, active bool) domain.LiquidityReservation {
		t.Helper()
		rows, err := app.ListLiquidityReservations(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return verify(rows, revision, amount, active)
	}

	click("Within 30 days")
	click(fmt.Sprintf("Available by date %d", fundsSourceCount(funds.current.report.Sources, funds.state.horizonOn, fundsAvailable)))
	visit := funds.state
	if visit.filter != fundsAvailable || visit.horizonOn != fundsPresetDay(funds.localDate, 30) {
		t.Fatalf("setup did not select the Funds visit: %+v", visit)
	}
	chooseFundsSourceAction(t, h, "AUD Cash · AUD", "Manage reservations")
	settle()
	r := funds.reservation
	if r == nil || r.source.Key() != ref.Key() || len(r.current.records) != 0 {
		t.Fatalf("source-scoped panel included unrelated reservations: %+v", r)
	}
	if findPageLabel(&h.router, "Edit · Rent") != nil {
		t.Fatal("unrelated reservation leaked into source-scoped panel")
	}
	h.focused(&r.add)
	origin := r.entry
	click("Add reservation")
	h.focused(&r.label.Editor)
	assertPageLabel(t, &h.router, "AUD Cash · AUD")
	setBankField(h, "Purpose", &r.label, purpose)
	setBankField(h, "Amount · AUD", &r.amount, "123.45")
	click("Review reservation")
	h.focused(&r.close)
	bounds := semanticBounds(t, &h.router, "Confirm", false)
	point := bounds.Min.Add(bounds.Max).Div(2)
	h.clickAt(point)
	select {
	case <-ledger.started:
	case <-time.After(3 * time.Second):
		t.Fatal("Confirm did not reach the ledger")
	}
	h.clickAt(point)
	h.press(key.NameEscape)
	if funds.reservation != r || ledger.saves.Load() != 1 {
		t.Fatalf("pending write allowed duplicate submission or dismissal, saves=%d", ledger.saves.Load())
	}
	proceed()
	settle()
	saved := checkSaved(1, "123.45", true)
	click("Edit · " + purpose)
	h.focused(&r.label.Editor)
	setBankField(h, "Amount · AUD", &r.amount, "234.56")
	click("Review reservation")
	click("Confirm")
	edited := checkSaved(2, "234.56", true)
	if edited.ID != saved.ID || ledger.saves.Load() != 2 {
		t.Fatal("edit did not update the same reservation exactly once")
	}
	click("Release · " + purpose)
	click("Confirm release")
	released := checkSaved(3, "234.56", false)
	if released.ID != saved.ID {
		t.Fatal("release changed reservation identity")
	}
	if findPageLabel(&h.router, "Released · "+purpose) != nil {
		t.Fatal("the released record was shown before opening history")
	}
	click("Released reservation (1)")
	assertPageLabel(t, &h.router, "Released · "+purpose)
	click("Close")
	if h.shell.activeRoute() != fundsRoute || h.shell.fundsReservationOpen() || funds.state != visit {
		t.Fatalf("Closing the source panel changed the Funds visit: %+v", funds.state)
	}
	h.focused(origin)
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	rows, err := reopened.ListLiquidityReservations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if row := verify(rows, 3, "234.56", false); row.ID != saved.ID {
		t.Fatal("released reservation did not survive reopen")
	}
}
