package desktop

import (
	"context"
	"errors"
	"image"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/layout"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	liquidityapi "github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
	portfolioapi "github.com/waltwang/nestworth-go/internal/wailsapi/portfolio"
)

type reservationFault struct {
	*application.Service
	saves, releases         atomic.Int32
	failRead                atomic.Bool
	saving, releasing       chan struct{}
	allowSave, allowRelease chan struct{}
}

func (f *reservationFault) SaveLiquidityReservation(ctx context.Context, input application.SaveReservationInput) (domain.LiquidityReservation, error) {
	switch f.saves.Add(1) {
	case 1:
		return domain.LiquidityReservation{}, errors.New("synthetic failure before saving")
	case 2:
		close(f.saving)
		<-f.allowSave
	}
	row, err := f.Service.SaveLiquidityReservation(ctx, input)
	if err == nil {
		f.failRead.Store(true)
		return domain.LiquidityReservation{}, errors.New("synthetic lost save reply")
	}
	return row, err
}

func (f *reservationFault) ReleaseLiquidityReservation(ctx context.Context, id domain.LiquidityReservationID, revision int) (domain.LiquidityReservation, error) {
	if f.releases.Add(1) == 1 {
		close(f.releasing)
		<-f.allowRelease
	}
	row, err := f.Service.ReleaseLiquidityReservation(ctx, id, revision)
	if err == nil {
		f.failRead.Store(true)
		return domain.LiquidityReservation{}, errors.New("synthetic lost release reply")
	}
	return row, err
}

func (f *reservationFault) ListLiquidityReservations(ctx context.Context) ([]domain.LiquidityReservation, error) {
	if f.failRead.Swap(false) {
		return nil, errors.New("synthetic first verification read failure")
	}
	return f.Service.ListLiquidityReservations(ctx)
}

func reservationHarness(t *testing.T, app *application.Service, ledger fundsReservationLedger) (*availableFundsPage, *previewHarness, func()) {
	t.Helper()
	ctx := t.Context()
	funds := newAvailableFundsPage(ctx, app, func() {})
	funds.reservationLedger = ledger
	accounts := newAccountsPage(ctx, app, func() time.Time { return accountTestClock }, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{funds: funds, accounts: accounts, version: app.MobileDataVersion})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	finish := func() {
		for range 4 {
			h.frame()
			funds.workers.Wait()
			accounts.workers.Wait()
		}
		h.frame()
	}
	h.frame()
	h.click("Available funds", false)
	finish()
	return funds, h, finish
}

func scrollReservationSource(t *testing.T, h *previewHarness, name string) {
	t.Helper()
	for range 10 {
		if findPageLabel(&h.router, name) != nil {
			bounds := semanticBounds(t, &h.router, name, false)
			if bounds.Min.Y > 135 && bounds.Max.Y < h.size.Y-32 {
				return
			}
		}
		h.router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(470, 465), Scroll: f32.Pt(0, 180)})
		h.frame()
	}
	t.Fatalf("could not scroll to %s", name)
}

func TestAvailableFundsReservationLifecycleMatchesWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openFundsFixture(t, true)
	reference, _, _, ref := openFundsFixture(t, true)
	fault := &reservationFault{Service: app, saving: make(chan struct{}), releasing: make(chan struct{}), allowSave: make(chan struct{}), allowRelease: make(chan struct{})}
	unblockSave := sync.OnceFunc(func() { close(fault.allowSave) })
	unblockRelease := sync.OnceFunc(func() { close(fault.allowRelease) })
	funds, h, finish := reservationHarness(t, app, fault)
	t.Cleanup(func() { unblockSave(); unblockRelease(); funds.workers.Wait() })
	click := func(text string) { t.Helper(); h.click(text, false); finish() }
	label := func(text string) { t.Helper(); assertPageLabel(t, &h.router, text) }
	adapter := liquidityapi.NewService(reference)

	netWorth := func(s *application.Service) portfolioapi.OverviewDTO {
		t.Helper()
		value, err := portfolioapi.NewService(s).Overview(ctx, account.AccountFilterRequest{})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	before, refBefore := netWorth(app), netWorth(reference)
	initialActivities := make(map[*application.Service][]domain.Activity)
	for _, service := range []*application.Service{app, reference} {
		rows, err := service.ListActivities(ctx, 10)
		if err != nil {
			t.Fatal(err)
		}
		initialActivities[service] = rows
	}
	checkState := func(s *application.Service, bank domain.AccountID, baseline portfolioapi.OverviewDTO, active, applied, available, shortfall string) {
		t.Helper()
		if again := netWorth(s); !reflect.DeepEqual(again, baseline) {
			t.Fatalf("reservation changed net worth: %+v -> %+v", baseline, again)
		}
		activities, err := s.ListActivities(ctx, 10)
		if err != nil || !reflect.DeepEqual(activities, initialActivities[s]) {
			t.Fatalf("reservation changed activities: %+v / %v", activities, err)
		}
		report, err := readAvailableFunds(ctx, s)
		if err != nil {
			t.Fatal(err)
		}
		row := report.report.Sources[slices.IndexFunc(report.report.Sources, func(row application.LiquidityReadSource) bool {
			return row.Result.Ref.Key() == domain.AccountValueSourceRef(bank).Key()
		})].Result
		at := slices.IndexFunc(row.BucketResults, func(b domain.LiquidityBucketResult) bool { return b.HorizonOn == report.today.HorizonOn })
		bucket := row.BucketResults[at]
		if row.CurrentNativeValue.CanonicalAmount() != "125.4321" || row.ReservationRequested.CanonicalAmount() != active || bucket.AppliedReserveNative.CanonicalAmount() != applied ||
			bucket.UnreservedNative.CanonicalAmount() != available || bucket.ReserveShortfallNative.CanonicalAmount() != shortfall {
			t.Fatalf("wrong amounts: %+v / %+v", row, bucket)
		}
	}
	parity := func(s *application.Service) {
		t.Helper()
		want, err := adapter.Overview(ctx, liquidityapi.OverviewRequest{})
		if err != nil {
			t.Fatal(err)
		}
		read, err := readAvailableFunds(ctx, s)
		if err != nil {
			t.Fatal(err)
		}
		assertFundsParity(t, read, want)
	}

	accounts := semanticBounds(t, &h.router, "Accounts", false)
	entry := "Manage reservations · Harbor Savings"
	scrollReservationSource(t, h, entry)
	click(entry)
	if funds.reservation == nil || funds.reservation.current == nil {
		t.Fatal("the source button did not open the reservation manager")
	}
	label("Available after reservations · 85.3071 USD")
	click("Add reservation")
	r := funds.reservation
	setBankField(h, "Purpose", &r.label, "Rent")
	setBankField(h, "Amount · USD", &r.amount, "0")
	click("Review reservation")
	label("Enter a positive amount with up to four decimal places.")
	if fault.saves.Load() != 0 {
		t.Fatal("invalid draft reached the writer")
	}
	setBankField(h, "Amount · USD", &r.amount, "40.125")
	click("Review reservation")
	if err := app.WithWrite(ctx, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	click("Confirm")
	label("The ledger changed. Preview this action again.")
	if fault.saves.Load() != 0 {
		t.Fatal("a stale review reached the writer")
	}
	click("Review reservation")
	click("Confirm")
	label("The saved reservation state did not change. Review the action again.")
	// An identical pre-existing reservation is insufficient to recover a reply.
	if len(r.current.records) != 1 || r.current.records[0].Label != "Rent" || fault.saves.Load() != 1 {
		t.Fatalf("pre-existing same payload was mistaken for a new reservation: %+v", r.current.records)
	}

	setBankField(h, "Purpose", &r.label, "Emergency fund")
	setBankField(h, "Amount · USD", &r.amount, "100.5")
	click("Review reservation")
	label("Requested reservation · 100.5 USD")
	clickTwiceWhileBlocked := func(text string, started <-chan struct{}, unblock func()) {
		t.Helper()
		box := semanticBounds(t, &h.router, text, false)
		point := box.Min.Add(box.Max).Div(2)
		h.clickAt(point)
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("confirmation did not reach the writer")
		}
		h.clickAt(point)
		unblock()
		finish()
		label("Could not verify the reservation. Retry checks the saved state.")
		h.clickAt(accounts.Min.Add(accounts.Max).Div(2))
		if h.shell.activeRoute() != fundsRoute || !funds.reservationUnresolved() {
			t.Fatal("navigation discarded an unresolved reservation")
		}
		click("Retry")
	}
	clickTwiceWhileBlocked("Confirm", fault.saving, unblockSave)
	label("Reservation saved.")
	label("Available after reservations · 0 USD")
	if fault.saves.Load() != 2 || len(r.current.records) != 2 {
		t.Fatal("double-confirm or a read retry wrote again")
	}
	refSaved, err := adapter.SaveReservation(ctx, liquidityapi.SaveReservationRequest{
		Source: liquidityapi.SourceRefDTO{Kind: string(domain.SourceAccountValue), AccountID: ref.bank.String()},
		Label:  "Emergency fund", Amount: "100.5",
	})
	if err != nil {
		t.Fatal(err)
	}
	parity(app)
	checkState(app, ids.bank, before, "140.625", "125.4321", "0", "15.1929")
	checkState(reference, ref.bank, refBefore, "140.625", "125.4321", "0", "15.1929")

	click("Close")
	label("Reservation shortfall · 15.1929 USD")
	scrollReservationSource(t, h, entry)
	click(entry)
	click("Release · Emergency fund")
	clickTwiceWhileBlocked("Confirm release", fault.releasing, unblockRelease)
	label("Reservation released. Available funds will be recalculated.")
	click("Released reservation (1)")
	label("Released · Emergency fund")
	label("Available after reservations · 85.3071 USD")
	if fault.releases.Load() != 1 || findPageLabel(&h.router, "Release · Emergency fund") != nil {
		t.Fatal("release retry wrote again or offered release of a released row")
	}
	if _, err := adapter.ReleaseReservation(ctx, liquidityapi.ReleaseReservationRequest{ID: refSaved.ID, ExpectedRevision: refSaved.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.ReleaseReservation(ctx, liquidityapi.ReleaseReservationRequest{ID: refSaved.ID, ExpectedRevision: refSaved.Revision}); err == nil {
		t.Fatal("Wails accepted the pre-release revision a second time")
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	reopened.SetClock(func() time.Time { return accountTestClock })
	initialActivities[reopened] = initialActivities[app]
	parity(reopened)
	checkState(reopened, ids.bank, before, "40.125", "40.125", "85.3071", "0")
	checkState(reference, ref.bank, refBefore, "40.125", "40.125", "85.3071", "0")
	state, err := readFundsReservation(ctx, reopened, domain.AccountValueSourceRef(ids.bank), fundsViewState{})
	if err != nil || len(state.records) != 2 || !slices.ContainsFunc(state.records, func(row domain.LiquidityReservation) bool {
		return row.Label == "Emergency fund" && row.Revision == 2 && !row.Active()
	}) {
		t.Fatalf("release evidence missing after reopen: %+v / %v", state.records, err)
	}
}

func TestAvailableFundsReservationOnZeroCashMatchesWails(t *testing.T) {
	ctx := t.Context()
	app, _, _, _ := openFundsFixture(t, true)
	reference, _, _, _ := openFundsFixture(t, true)
	funds, h, finish := reservationHarness(t, app, app)
	click := func(text string) { t.Helper(); h.click(text, false); finish() }
	entry := "Manage reservations · Empty Savings"
	scrollReservationSource(t, h, entry)
	click(entry)
	if funds.reservation == nil || funds.reservation.current == nil {
		t.Fatal("the zero-balance source button did not open the reservation manager")
	}
	assertPageLabel(t, &h.router, "Available after reservations · 0 USD")
	click("Add reservation")
	setBankField(h, "Purpose", &funds.reservation.label, "Future expenses")
	setBankField(h, "Amount · USD", &funds.reservation.amount, "9")
	click("Review reservation")
	click("Confirm")
	assertPageLabel(t, &h.router, "Reservation saved.")
	assertPageLabel(t, &h.router, "Available after reservations · 0 USD")
	other, err := reference.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(other, func(row domain.AccountRecord) bool { return row.Account.Name == "Empty Savings" })
	adapter := liquidityapi.NewService(reference)
	want, err := adapter.SaveReservation(ctx, liquidityapi.SaveReservationRequest{
		Source: liquidityapi.SourceRefDTO{Kind: string(domain.SourceAccountValue), AccountID: other[i].Account.ID.String()},
		Label:  "Future expenses", Amount: "9",
	})
	if err != nil || want.Amount.Amount != "9" {
		t.Fatalf("Wails would not accept the zero-balance request: %+v / %v", want, err)
	}
	refView, err := adapter.Overview(ctx, liquidityapi.OverviewRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if funds.current == nil || !slices.ContainsFunc(funds.current.report.Overview.UnresolvedReservations, func(row domain.UnresolvedReservation) bool {
		return row.Reservation.Label == "Future expenses" && row.Reservation.Amount.CanonicalAmount() == "9"
	}) {
		t.Fatal("the zero-balance request was incorrectly applied to available cash")
	}
	// The shared report sorts reservations by created time and ID. These
	// independent fixtures have different IDs, so compare the exact new row.
	if !slices.ContainsFunc(refView.UnresolvedReservations, func(row liquidityapi.UnresolvedReservationDTO) bool {
		return row.Reservation.ID == want.ID && row.Reservation.Amount.Amount == "9" && row.Reason != ""
	}) {
		t.Fatalf("Wails did not preserve the same unresolved reservation: %+v", refView.UnresolvedReservations)
	}
	click("Release · Future expenses")
	releaseButton := semanticBounds(t, &h.router, "Confirm release", false)
	point := releaseButton.Min.Add(releaseButton.Max).Div(2)
	h.clickAt(point)
	finish()
	h.clickAt(point)
	finish()
	if funds.reservation.editing {
		t.Fatal("a second release click opened the add form after the first release completed")
	}
	click("Released reservation (1)")
	assertPageLabel(t, &h.router, "Released · Future expenses")
	assertPageLabel(t, &h.router, "Available after reservations · 0 USD")
	if slices.ContainsFunc(funds.current.report.Overview.UnresolvedReservations, func(row domain.UnresolvedReservation) bool {
		return row.Reservation.Label == "Future expenses"
	}) {
		t.Fatal("release left the zero-balance reservation unresolved")
	}
}

func TestAvailableFundsGlobalReservationEditKeepsSelectedHorizon(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openFundsFixture(t, true)
	reference, _, _, ref := openFundsFixture(t, true)
	funds, h, finish := reservationHarness(t, app, app)
	click := func(text string) { t.Helper(); h.click(text, false); finish() }

	horizon := fundsPresetDay(funds.localDate, 12)
	funds.enter(layout.Position{}, fundsViewState{customOn: horizon, horizonOn: horizon, includeEarly: true})
	finish()
	scrollReservationSource(t, h, "Manage reservations")
	click("Manage reservations")
	r := funds.reservation
	if r.current.horizon != horizon || len(r.current.records) != 2 {
		t.Fatalf("manager lost the selected date or an unavailable reservation: %+v", r.current)
	}
	click("Edit · Rent")
	if r.label.Text() != "Rent" || r.amount.Text() != "40.125" {
		t.Fatal("edit form did not load persisted values")
	}
	click("Cancel")
	click("Edit · Rent")
	setBankField(h, "Purpose", &r.label, "Fall rent")
	setBankField(h, "Amount · USD", &r.amount, "50.1234")
	click("Review reservation")
	click("Confirm")
	assertPageLabel(t, &h.router, "Reservation saved.")
	if funds.state.horizonOn != horizon || r.current.horizon != horizon || !funds.state.includeEarly {
		t.Fatal("saving reset the date or early withdrawal setting")
	}
	cash := r.current.available(domain.AccountValueSourceRef(ids.bank))
	if cash == nil || cash.CanonicalAmount() != "75.3087" {
		t.Fatalf("edited amount did not reflow into the selected projection: %v", cash)
	}

	adapter := liquidityapi.NewService(reference)
	reservations, err := adapter.ListReservations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, existing := range reservations {
		if existing.Label != "Rent" {
			continue
		}
		_, err := adapter.SaveReservation(ctx, liquidityapi.SaveReservationRequest{
			ID: &existing.ID, ExpectedRevision: existing.Revision,
			Source: liquidityapi.SourceRefDTO{Kind: string(domain.SourceAccountValue), AccountID: ref.bank.String()},
			Label:  "Fall rent", Amount: "50.1234",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	want, err := adapter.Overview(ctx, liquidityapi.OverviewRequest{CustomHorizonOn: &horizon, IncludeEarlyWithdrawal: true})
	if err != nil {
		t.Fatal(err)
	}
	assertFundsParity(t, *funds.current, want)

	click("Add reservation")
	h.click("Reservation source", true)
	h.frame()
	h.click("Travel Wallet cash · EUR", false)
	finish()
	setBankField(h, "Purpose", &r.label, "Travel")
	setBankField(h, "Amount · EUR", &r.amount, "4.9234")
	click("Review reservation")
	click("Confirm")
	eur := domain.CurrencyCode("EUR")
	travel := domain.AccountCashSourceRef(ids.foreign, eur)
	cash = r.current.available(travel)
	if cash == nil || cash.CanonicalAmount() != "20.3266" {
		t.Fatalf("foreign source retained the wrong amount: %v", cash)
	}
	_, err = adapter.SaveReservation(ctx, liquidityapi.SaveReservationRequest{
		Source: liquidityapi.SourceRefDTO{Kind: string(domain.SourceAccountCash), AccountID: ref.foreign.String(), Currency: productOptional(eur.String())},
		Label:  "Travel", Amount: "4.9234",
	})
	if err != nil {
		t.Fatal(err)
	}
	click("Release · Old reserve")
	click("Confirm release")
	for _, existing := range reservations {
		if existing.Label == "Old reserve" {
			if _, err := adapter.ReleaseReservation(ctx, liquidityapi.ReleaseReservationRequest{ID: existing.ID, ExpectedRevision: existing.Revision}); err != nil {
				t.Fatal(err)
			}
		}
	}
	want, err = adapter.Overview(ctx, liquidityapi.OverviewRequest{CustomHorizonOn: &horizon, IncludeEarlyWithdrawal: true})
	if err != nil {
		t.Fatal(err)
	}
	assertFundsParity(t, *funds.current, want)
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	saved, err := readFundsReservation(ctx, reopened, domain.LiquiditySourceRef{}, funds.state)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []struct {
		source   domain.LiquiditySourceRef
		label    string
		amount   string
		revision int
	}{
		{domain.AccountValueSourceRef(ids.bank), "Fall rent", "50.1234", 2},
		{travel, "Travel", "4.9234", 1},
	} {
		if !slices.ContainsFunc(saved.records, func(row domain.LiquidityReservation) bool {
			return row.Source.Key() == expected.source.Key() && row.Active() && row.Label == expected.label &&
				row.Revision == expected.revision && row.Amount.CanonicalAmount() == expected.amount
		}) {
			t.Fatalf("saved reservation missing after reopen: %+v", expected)
		}
	}
}
