package desktop

import (
	"errors"
	"github.com/waltwang/nestworth-go/internal/testfixture"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"gioui.org/layout"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	liquidityapi "github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

func TestAvailableFundsPairedMixedCurrencyReservationScopesAndConflicts(t *testing.T) {
	ctx := t.Context()
	app, session, paths, ids := openFundsFixture(t, true)
	eur := domain.AccountCashSourceRef(ids.foreign, "EUR")
	usd := domain.AccountCashSourceRef(ids.foreign, "USD")
	bank := domain.AccountValueSourceRef(ids.bank)
	const horizon = "2026-12-03"
	const purpose = "Travel"
	if _, err := testfixture.AppendAccountCashValue(app, ctx, ids.foreign, "50.6789", "USD", ""); err != nil {
		t.Fatal(err)
	}
	for _, policy := range []application.SavePolicyInput{
		{Source: bank, Policy: simpleFundsPolicy(policyExcluded, application.ProductPolicyInput{})},
		{Source: usd, Policy: simpleFundsPolicy(policyDate, application.ProductPolicyInput{UnlockOn: productOptional(horizon)})},
	} {
		if _, err := app.SaveLiquidityPolicy(ctx, policy); err != nil {
			t.Fatal(err)
		}
	}

	// Keep the account/source IDs and starting revisions identical by copying
	// only the checkpointed synthetic ledger before opening either client.
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	seed, err := os.ReadFile(session.DB.Path)
	if err != nil {
		t.Fatal(err)
	}
	refPath := filepath.Join(t.TempDir(), "wails-reference.db")
	if err := os.WriteFile(refPath, seed, 0o600); err != nil {
		t.Fatal(err)
	}
	app, session = reopenBaselineProfile(t, paths)
	clock := func() time.Time { return accountTestClock }
	testfixture.SetClock(app, clock)
	db, err := sqlite.Open(refPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	reference := testfixture.NewService(testfixture.Repository(db))
	testfixture.SetClock(reference, clock)
	adapter := liquidityapi.NewService(reference)

	funds, h, settle := reservationHarness(t, app, app)
	t.Cleanup(funds.workers.Wait)
	h.size = image.Pt(1240, 1020)
	funds.enter(layout.Position{}, fundsViewState{customOn: horizon, horizonOn: horizon, includeEarly: true})
	settle()
	click := func(name string) {
		t.Helper()
		h.click(name, false)
		settle()
	}
	checkScope := func(ref domain.LiquiditySourceRef) {
		t.Helper()
		if r := funds.reservation; r == nil || r.current == nil || r.source.Key() != ref.Key() || r.current.horizon != horizon ||
			funds.state.customOn != horizon || funds.state.horizonOn != horizon || !funds.state.includeEarly {
			t.Fatalf("reservation manager lost its source or the selected horizon: %+v / %+v", r, funds.state)
		}
	}
	checkCash := func(ref domain.LiquiditySourceRef, want string) {
		t.Helper()
		if value := funds.reservation.current.available(ref); value == nil || value.Currency() != *ref.Currency || value.CanonicalAmount() != want {
			t.Fatalf("forecast for %s = %+v, want %s", ref.Key(), value, want)
		}
	}
	fill := func(amount string) {
		t.Helper()
		r := funds.reservation
		source := r.draftSource()
		if source == nil {
			t.Fatal("reservation draft has no source")
		}
		setBankField(h, "Purpose", &r.label, purpose)
		setBankField(h, "Amount · "+source.NativeCurrency.String(), &r.amount, amount)
		click("Review reservation")
		click("Confirm")
		assertPageLabel(t, &h.router, "Reservation saved.")
	}
	saveWails := func(ref domain.LiquiditySourceRef, row *liquidityapi.ReservationDTO, amount string) liquidityapi.ReservationDTO {
		t.Helper()
		request := liquidityapi.SaveReservationRequest{
			Source: liquidityapi.SourceRefDTO{Kind: string(ref.Kind), AccountID: ref.AccountID.String(), Currency: productOptional(ref.Currency.String())},
			Label:  purpose, Amount: amount,
		}
		if row != nil {
			request.ID, request.ExpectedRevision = &row.ID, row.Revision
		}
		saved, err := adapter.SaveReservation(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		return saved
	}

	click("Manage reservations")
	checkScope(domain.LiquiditySourceRef{})
	click("Add reservation")
	h.click("Reservation source", true)
	h.frame()
	assertPageLabel(t, &h.router, "Travel Wallet cash · USD")
	click("Travel Wallet cash · EUR")
	fill("4.1234")
	wailsEUR := saveWails(eur, nil, "4.1234")
	checkCash(eur, "21.1266")
	click("Close")

	click("Funding sources")
	chooseFundsSourceAction(t, h, "Travel Wallet cash · USD", "Manage reservations")
	settle()
	checkScope(usd)
	if len(funds.reservation.current.records) != 0 {
		t.Fatal("the USD source manager showed a reservation for the EUR leg")
	}
	click("Add reservation")
	draft := funds.reservation
	h.focused(&draft.label.Editor)
	assertPageLabel(t, &h.router, "Reservation source")
	if source := draft.draftSource(); source == nil || source.Ref.Key() != usd.Key() {
		t.Fatal("the scoped form offered the sibling cash leg")
	}
	fill("7.2345")
	wailsUSD := saveWails(usd, nil, "7.2345")
	checkCash(usd, "43.4444")
	click("Close")

	// Exclusion prevents adding, while an existing reservation can still be
	// managed at that exact source as in the Wails source row.
	chooseFundsSourceAction(t, h, "Harbor Savings · USD", "Manage reservations")
	settle()
	checkScope(bank)
	assertPageLabel(t, &h.router, "Edit · Rent")
	if findPageLabel(&h.router, "Add reservation") != nil {
		t.Fatal("the excluded source allowed a new reservation")
	}
	click("Close")
	funds.list.ScrollTo(0)
	settle()
	click("Manage reservations")
	click("Edit · Travel · Travel Wallet cash · EUR")
	r := funds.reservation
	first := *r.editRow
	setBankField(h, "Amount · EUR", &r.amount, "999.4321")
	click("Review reservation")
	if _, err := app.SaveLiquidityReservation(ctx, application.SaveReservationInput{
		ID: &first.ID, ExpectedRevision: first.Revision, Source: eur, Label: purpose, Amount: "4.1111",
	}); err != nil {
		t.Fatal(err)
	}
	staleWails := wailsEUR
	wailsEUR = saveWails(eur, &wailsEUR, "4.1111")
	_, err = adapter.SaveReservation(ctx, liquidityapi.SaveReservationRequest{ID: &staleWails.ID, ExpectedRevision: staleWails.Revision,
		Source: staleWails.SourceRef, Label: purpose, Amount: "999.4321"})
	var wireErr *apierror.WireError
	if !errors.As(err, &wireErr) || wireErr.Code != string(domain.ErrRevisionConflict) {
		t.Fatalf("expected Wails to reject the stale revision, got %v", err)
	}

	click("Confirm")
	checkScope(domain.LiquiditySourceRef{})
	if r.amount.Text() != "999.4321" || r.editRow.Revision != first.Revision || findPageLabel(&h.router, "Review reservation") != nil {
		t.Fatal("a conflicting draft was overwritten or could proceed without loading")
	}
	assertPageLabel(t, &h.router, "The ledger changed. Preview this action again.")
	checkCash(eur, "21.1389")
	click("Load latest reservation")
	if r.editRow.ID != first.ID || r.editRow.Revision != wailsEUR.Revision || r.amount.Text() != "4.1111" {
		t.Fatal("the latest revision did not replace the explicitly discarded draft")
	}
	fill("5.4321")
	wailsEUR = saveWails(eur, &wailsEUR, "5.4321")
	checkCash(eur, "19.8179")
	checkCash(usd, "43.4444")

	click("Release · Travel · Travel Wallet cash · USD")
	click("Confirm release")
	wailsUSD, err = adapter.ReleaseReservation(ctx, liquidityapi.ReleaseReservationRequest{ID: wailsUSD.ID, ExpectedRevision: wailsUSD.Revision})
	if err != nil {
		t.Fatal(err)
	}
	click("Released reservation (1)")
	assertPageLabel(t, &h.router, "Released · Travel")
	checkCash(usd, "50.6789")
	checkCash(eur, "19.8179")
	click("Close")

	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	app, _ = reopenBaselineProfile(t, paths)
	testfixture.SetClock(app, clock)
	db, err = sqlite.Open(refPath)
	if err != nil {
		t.Fatal(err)
	}
	reference = testfixture.NewService(testfixture.Repository(db))
	testfixture.SetClock(reference, clock)
	adapter = liquidityapi.NewService(reference)
	want, err := adapter.Overview(ctx, liquidityapi.OverviewRequest{CustomHorizonOn: productOptional(horizon), IncludeEarlyWithdrawal: true})
	if err != nil {
		t.Fatal(err)
	}
	report, err := readAvailableFundsQuery(ctx, app, funds.state.query())
	if err != nil {
		t.Fatal(err)
	}
	assertFundsParity(t, report, want)
	custom := func(report liquidityapi.LiquidityOverviewDTO) *liquidityapi.LiquidityBucketDTO {
		if at := slices.IndexFunc(report.Buckets, func(bucket liquidityapi.LiquidityBucketDTO) bool { return bucket.HorizonOn == horizon }); at >= 0 {
			return &report.Buckets[at]
		}
		return nil
	}
	if gotFix, wantFix := custom(liquidityapi.FromOverview(report.report.Overview)), custom(want); gotFix == nil || !reflect.DeepEqual(gotFix, wantFix) {
		t.Fatalf("the exact custom-date buckets differ after reopen: %+v / %+v", gotFix, wantFix)
	}
	saved, err := readFundsReservation(ctx, app, domain.LiquiditySourceRef{}, funds.state)
	if err != nil || saved.horizon != horizon {
		t.Fatalf("reopen lost the selected projection: %v, %v", err, saved.horizon)
	}
	persisted, err := adapter.ListReservations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{wailsEUR.ID, wailsUSD.ID} {
		at := slices.IndexFunc(persisted, func(row liquidityapi.ReservationDTO) bool { return row.ID == id })
		if at < 0 {
			t.Fatalf("Wails lost reservation on reopen: %s", id)
		}
		wails := persisted[at]
		ref, err := wails.SourceRef.ToDomain()
		if err != nil || !slices.ContainsFunc(saved.records, func(row domain.LiquidityReservation) bool {
			return row.Source.Key() == ref.Key() && row.Label == purpose && row.Amount.CanonicalAmount() == wails.Amount.Amount &&
				row.Currency.String() == wails.Currency && row.Active() == (wails.ReleasedAt == nil) && row.Revision == wails.Revision
		}) {
			t.Fatalf("Gio did not persist the equivalent exact reservation: %+v / %v", wails, err)
		}
	}
}
