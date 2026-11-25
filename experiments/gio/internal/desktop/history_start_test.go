package desktop

import (
	"context"
	"errors"
	"image"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	holdingapi "github.com/waltwang/nestworth-go/internal/wailsapi/holding"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type startingPointFault struct {
	*application.Service
	writes         atomic.Int32
	failedRead     atomic.Bool
	loseReply      bool
	blockAttempt   int32
	started, allow chan struct{}
}

func (s *startingPointFault) ConfirmHistoryStart(ctx context.Context, review application.HistoryStartReview) (domain.HistoryOrigin, error) {
	if s.writes.Add(1) == s.blockAttempt {
		close(s.started)
		<-s.allow
	}
	value, err := s.Service.ConfirmHistoryStart(ctx, review)
	if err != nil {
		return value, err
	}
	s.failedRead.Store(true)
	if s.loseReply {
		return domain.HistoryOrigin{}, errors.New("synthetic lost start reply after commit")
	}
	return value, nil
}

func (s *startingPointFault) HistoryOrigin(ctx context.Context) (*domain.HistoryOrigin, error) {
	if s.failedRead.Swap(false) {
		return nil, errors.New("synthetic read failure after start")
	}
	return s.Service.HistoryOrigin(ctx)
}

func historyStartHarness(t *testing.T, ledger historyService) (*historyPage, *previewHarness, func()) {
	t.Helper()
	clock := func() time.Time { return accountTestClock }
	page := newLedgerHistory(t.Context(), ledger, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{history: page})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language, h.shell.prefs.saved.Timezone = settings.LanguageEnglish, "Asia/Singapore"
	zone, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	h.shell.prefs.savedLocation = zone
	finish := func() {
		for range 3 {
			h.frame()
			page.reads.workers.Wait()
			page.model.Wait()
		}
		h.frame()
	}
	h.frame()
	h.click("Ledger activity", false)
	finish()
	return page, h, finish
}

func TestHistoryStartingPointCapturesExistingBalances(t *testing.T) {
	ctx := t.Context()
	service, session, paths, cash, bank := seedHistoryStartFixture(t, true)
	ref, refSession, _, refCash, refBank := seedHistoryStartFixture(t, true)
	adapter := historyapi.NewService(ref)
	if _, err := adapter.StartHistoryOnDate(ctx, "Asia/Singapore", "2026-11-21", nil); err == nil {
		t.Fatal("Wails accepted an earlier day with existing balances")
	}
	fault := &startingPointFault{Service: service, blockAttempt: 2, started: make(chan struct{}), allow: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.allow) })
	t.Cleanup(release)
	page, h, finish := historyStartHarness(t, fault)
	label := func(value string) { assertPageLabel(t, &h.router, value) }
	click := func(value string) { h.click(value, false); finish() }
	noOrigin := func() {
		t.Helper()
		if origin, err := service.HistoryOrigin(ctx); err != nil || origin != nil {
			t.Fatalf("unexpected persisted origin: %+v, %v", origin, err)
		}
	}

	click("Start recording history")
	label("History time zone · Asia/Singapore")
	h.click("2026-11-22", false)
	h.press(key.NameLeftArrow)
	h.press(key.NameReturn)
	click("Review Starting Point")
	label("An earlier date requires zero balances and holdings. Choose today to capture existing balances.")
	noOrigin()
	h.click("2026-11-21", false)
	h.press(key.NameRightArrow)
	h.press(key.NameReturn)
	click("Review Starting Point")
	label("Capture current balances at confirmation on 2026-11-22 (Asia/Singapore)")
	label("Harbor Savings · 125.5 USD")
	label("Archived Loan · Archived · 31.25 USD")
	label("Cash Studio · 14.125 USD")
	label("Cash Studio · 20.5 EUR")
	click("Cancel")
	noOrigin()
	click("Start recording history")
	click("Review Starting Point")
	for _, side := range []struct {
		app *application.Service
		id  domain.AccountID
	}{{service, cash}, {ref, refCash}} {
		if _, err := side.app.AppendAccountCashValue(ctx, side.id, "15.125", "USD", ""); err != nil {
			t.Fatal(err)
		}
	}
	click("Confirm Starting Point")
	label("The ledger changed. Preview this action again.")
	noOrigin()
	click("Review Starting Point")
	label("Cash Studio · 15.125 USD")
	review := page.startPoint.review.plan
	bounds := semanticBounds(t, &h.router, "Confirm Starting Point", false)
	point := bounds.Min.Add(bounds.Max).Div(2)
	h.clickAt(point)
	select {
	case <-fault.started:
	case <-time.After(3 * time.Second):
		t.Fatal("Starting Point confirmation did not start")
	}
	h.clickAt(point)
	h.click("Back", false)
	if h.shell.activeRoute() != historyRoute || !page.startPoint.unresolved() {
		t.Fatal("confirmation escaped to another page")
	}
	release()
	finish()
	label("Starting Point saved.")
	label("Could not verify the Starting Point. Retry checks the saved state.")
	click("Retry")
	label("No changes have been recorded yet.")
	if fault.writes.Load() != 2 || findPageLabel(&h.router, "Start recording history") != nil {
		t.Fatal("a saved Starting Point allowed another write")
	}
	got := page.reads.references.Origin
	want, err := adapter.StartHistoryOnDate(ctx, "Asia/Singapore", "2026-11-22", nil)
	if err != nil || got == nil || wire.FormatTime(got.StartedAt) != want.StartedAt || wire.FormatTime(got.CreatedAt) != want.CreatedAt || got.Timezone != want.Timezone || !got.StartedAt.Equal(accountTestClock) {
		t.Fatalf("Gio origin %+v differs from Wails %+v, %v", got, want, err)
	}
	assertStartingBalances(t, session, got.ID)
	assertStartingBalances(t, refSession, domain.HistoryOriginID(want.ID))

	// An ordinary later deposit appends activity after the baseline. Reopening
	// and replaying the old confirmation preserves the origin and the posting.
	for _, side := range []struct {
		app *application.Service
		id  domain.AccountID
	}{{service, bank}, {ref, refBank}} {
		_, err := historyapi.NewService(side.app).RecordChange(ctx, historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeMoneyAdded, AccountID: side.id.String(), Amount: "5", Currency: "USD", Reason: "contribution",
			EffectiveLocalDate: "2026-11-22", EffectiveLocalTime: "01:00",
		})
		if err != nil {
			t.Fatal(err)
		}
		if balance := readBankRecord(t, side.app, side.id).LatestValue.Amount.CanonicalAmount(); balance != "130.5" {
			t.Fatalf("first contribution balance = %s", balance)
		}
	}
	before, err := service.ListActivities(ctx, 50)
	if err != nil || len(before) != 1 {
		t.Fatalf("expected one activity after the first posting: %+v, %v", before, err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, afterSession := reopenOnboardingProfile(t, paths)
	again, err := reopened.ConfirmHistoryStart(ctx, review)
	if err != nil || again.ID != got.ID {
		t.Fatalf("confirmation retry after reopen returned %+v, %v", again, err)
	}
	after, err := reopened.ListActivities(ctx, 50)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("origin retry changed prior activity: %+v, %v", after, err)
	}
	assertStartingBalances(t, afterSession, got.ID)
}

func TestHistoryEmptyStartingPointVerifiesLostReplyWithoutRewriting(t *testing.T) {
	ctx := t.Context()
	service, session, paths, _, _ := seedHistoryStartFixture(t, false)
	ref, _, _, _, _ := seedHistoryStartFixture(t, false)
	fault := &startingPointFault{Service: service, loseReply: true}
	page, h, finish := historyStartHarness(t, fault)
	click := func(label string) { h.click(label, false); finish() }
	click("Start recording history")
	h.click("2026-11-22", false)
	h.press(key.NameLeftArrow)
	h.press(key.NameLeftArrow)
	h.press(key.NameReturn)
	click("Review Starting Point")
	assertPageLabel(t, &h.router, "Effective: 2026-11-20 00:00 (Asia/Singapore)")
	assertPageLabel(t, &h.router, "No opening amounts to capture.")
	click("Confirm Starting Point")
	assertPageLabel(t, &h.router, "Could not verify the Starting Point. Retry checks the saved state.")
	if findPageLabel(&h.router, "Confirm Starting Point") != nil || findPageLabel(&h.router, "Starting Point saved.") != nil {
		t.Fatal("an unknown write outcome offered another writer or claimed success")
	}
	click("Retry")
	assertPageLabel(t, &h.router, "Starting Point saved.")
	want, err := historyapi.NewService(ref).StartHistoryOnDate(ctx, "Asia/Singapore", "2026-11-20", nil)
	got := page.reads.references.Origin
	if err != nil || got == nil || wire.FormatTime(got.StartedAt) != want.StartedAt || got.Timezone != want.Timezone || fault.writes.Load() != 1 {
		t.Fatalf("Gio origin %+v differs from Wails %+v, %v", got, want, err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, closed := reopenOnboardingProfile(t, paths)
	after, err := reopened.HistoryOrigin(ctx)
	if err != nil || !reflect.DeepEqual(got, after) {
		t.Fatalf("reopened origin = %+v, %v", after, err)
	}
	data, err := sqlite.NewRepository(closed.DB).HistoryOriginData(ctx, got.ID)
	if err != nil || len(data.Components) != 0 {
		t.Fatalf("empty origin stored unexpected components: %+v, %v", data.Components, err)
	}
}

func TestHistoryStartingPointCapturesMultipleNativeCostPositions(t *testing.T) {
	ctx := t.Context()
	service, session, paths, ids := openStartingPositionFixture(t, true)
	reference, refSession, _, refIDs := openStartingPositionFixture(t, true)
	costs := map[string]string{"USD": "0", "EUR": "12.34567891"}
	refCosts := map[string]string{}
	wailsHoldings := holdingapi.NewService(reference)
	for _, item := range []struct {
		id, refID          domain.InstrumentID
		quantity, currency string
	}{
		{ids.security, refIDs.security, "2.5", "USD"},
		{ids.eur, refIDs.eur, "3.125", "EUR"},
	} {
		plan, err := history.PreviewOpeningPosition(ctx, service, ids.account, item.id, item.quantity)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := plan.Confirm(ctx, service); err != nil {
			t.Fatal(err)
		}
		if _, err := history.PreviewOpeningPosition(ctx, service, ids.account, item.id, item.quantity); err == nil {
			t.Fatal("an existing account-instrument pair was offered a second time")
		}
		row, err := wailsHoldings.CreateHolding(ctx, holdingapi.CreateHoldingRequest{
			AccountID: refIDs.account.String(), InstrumentID: item.refID.String(), Quantity: item.quantity,
		})
		if err != nil {
			t.Fatal(err)
		}
		refCosts[row.ID] = costs[item.currency]
	}
	fault := &startingPointFault{Service: service, loseReply: true}
	page, h, finish := historyStartHarness(t, fault)
	h.size = image.Pt(640, 775)
	h.frame()
	click := func(text string) { h.click(text, false); finish() }
	label := func(text string) { assertPageLabel(t, &h.router, text) }
	click("Start recording history")
	start := page.startPoint
	if len(start.holdings) != 2 || start.canPreview() {
		t.Fatalf("expected two positions requiring explicit costs, got %d rows and canPreview=%t", len(start.holdings), start.canPreview())
	}
	var foreign domain.HoldingID
	for _, row := range start.holdings {
		if row.Currency == "EUR" {
			foreign = row.HoldingID
		}
		field := "Original cost per unit · " + row.AccountName + " · " + row.InstrumentName + " · " + row.Currency.String()
		setBankField(h, field, &row.cost, costs[row.Currency.String()])
		h.focused(&row.cost.Editor)
	}
	// The quoted USD position also requires a reviewed cost.
	if _, err := service.PreviewHistoryStart(ctx, "Asia/Singapore", "2026-11-22", map[domain.HoldingID]string{foreign: costs["EUR"]}); err == nil {
		t.Fatal("an omitted cost silently fell back to the market quote")
	}
	click("Review Starting Point")
	label("Opening cost per unit · 0 USD")
	label("Opening cost per unit · 12.34567891 EUR")
	if _, err := service.UpdateHoldingQuantity(ctx, foreign, "4.125"); err != nil {
		t.Fatal(err)
	}
	refHoldings, err := reference.HoldingsByAccounts(ctx, []domain.AccountID{refIDs.account})
	if err != nil {
		t.Fatal(err)
	}
	for _, holding := range refHoldings[refIDs.account] {
		if holding.InstrumentID == refIDs.eur {
			if _, err := reference.UpdateHoldingQuantity(ctx, holding.ID, "4.125"); err != nil {
				t.Fatal(err)
			}
		}
	}
	click("Confirm Starting Point")
	label("The ledger changed. Preview this action again.")
	if origin, err := service.HistoryOrigin(ctx); err != nil || origin != nil {
		t.Fatalf("stale preview saved an Origin: %+v, %v", origin, err)
	}
	for _, row := range start.holdings {
		if row.cost.Text() != costs[row.Currency.String()] {
			t.Fatal("stale review lost the matching opening cost")
		}
	}
	click("Review Starting Point")
	label("Quantity 4.125")
	click("Confirm Starting Point")
	label("Could not verify the Starting Point. Retry checks the saved state.")
	click("Retry")
	label("No changes have been recorded yet.")
	if fault.writes.Load() != 2 {
		t.Fatal("read retry wrote another Origin")
	}
	origin, err := historyapi.NewService(reference).StartHistoryOnDate(ctx, "Asia/Singapore", "2026-11-22", refCosts)
	if err != nil || origin.Timezone != page.reads.references.Origin.Timezone ||
		origin.StartedAt != wire.FormatTime(page.reads.references.Origin.StartedAt) {
		t.Fatalf("Starting Point differs from Wails: %+v, %v", origin, err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	_, reopened := reopenOnboardingProfile(t, paths)
	readCosts := func(storage *pilot.Session) map[string][2]string {
		t.Helper()
		rows, err := storage.DB.SQL.QueryContext(ctx, `
			SELECT instruments.quote_currency, components.quantity, components.unit_cost
			FROM history_origin_components components JOIN instruments ON instruments.id=components.instrument_id
			WHERE components.component_kind='holding_quantity'`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		found := map[string][2]string{}
		for rows.Next() {
			var currency, quantity, cost string
			if err := rows.Scan(&currency, &quantity, &cost); err != nil {
				t.Fatal(err)
			}
			found[currency] = [2]string{quantity, cost}
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return found
	}
	want := map[string][2]string{"USD": {"2.5", "0"}, "EUR": {"4.125", "12.34567891"}}
	if got := readCosts(reopened); !reflect.DeepEqual(got, want) || !reflect.DeepEqual(got, readCosts(refSession)) {
		t.Fatalf("reopened quantities and costs differ from Wails: %+v", got)
	}
	assertBankObservations(t, reopened, ids.account, 0, 0)
}

func seedHistoryStartFixture(t *testing.T, populated bool) (*application.Service, *pilot.Session, pilot.Paths, domain.AccountID, domain.AccountID) {
	t.Helper()
	ctx := t.Context()
	paths := prepareOnboardingProfile(t)
	service, session := reopenOnboardingProfile(t, paths)
	service.SetClock(func() time.Time { return accountTestClock.Add(-24 * time.Hour) })
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Synthetic history start", BaseCurrency: "USD", MemberNames: []string{"Ana Example"},
	}); err != nil {
		t.Fatal(err)
	}
	var studio, bank domain.AccountID
	if populated {
		boot, err := service.Bootstrap(ctx)
		if err != nil {
			t.Fatal(err)
		}
		create := func(name, kind, mode, role, amount string) domain.AccountID {
			t.Helper()
			row, err := service.CreateAccount(ctx, application.AccountInput{
				Name: name, AccountType: kind, BalanceSheetRole: role, TrackingMode: mode, DefaultCurrency: "USD", InitialAmount: amount,
				OwnerIDs: []domain.MemberID{boot.Members[0].ID}, IncludeInNetWorth: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			return row.Account.ID
		}
		bank = create("Harbor Savings", "bank_account", "balance", "asset", "125.5")
		studio = create("Cash Studio", "cash_on_hand", "holdings", "asset", "")
		archived := create("Archived Loan", "loan", "balance", "liability", "31.25")
		if err := service.ArchiveAccount(ctx, archived, true); err != nil {
			t.Fatal(err)
		}
		for code, amount := range map[string]string{"USD": "14.125", "EUR": "20.5"} {
			if _, err := service.AppendAccountCashValue(ctx, studio, amount, code, ""); err != nil {
				t.Fatal(err)
			}
		}
	}
	service.SetClock(func() time.Time { return accountTestClock })
	return service, session, paths, studio, bank
}

func assertStartingBalances(t *testing.T, session *pilot.Session, id domain.HistoryOriginID) {
	t.Helper()
	repo := sqlite.NewRepository(session.DB)
	data, err := repo.HistoryOriginData(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ListAccountRecords(t.Context(), data.Origin.HouseholdID, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	names := map[domain.AccountID]string{}
	for _, row := range rows {
		names[row.Account.ID] = row.Account.Name
	}
	got := map[string]string{}
	for _, c := range data.Components {
		got[names[*c.AccountID]+"/"+c.Amount.Currency().String()] = c.Amount.CanonicalAmount()
	}
	want := map[string]string{"Harbor Savings/USD": "125.5", "Archived Loan/USD": "31.25", "Cash Studio/USD": "15.125", "Cash Studio/EUR": "20.5"}
	if len(data.Components) != len(want) || !reflect.DeepEqual(got, want) || len(data.AccountStates) != 3 || len(data.Ownership) != 3 {
		t.Fatalf("unexpected opening data: %+v %+v", got, data)
	}
	for _, state := range data.AccountStates {
		if (state.ArchivedAt != nil) != (names[state.AccountID] == "Archived Loan") || !state.IncludeInNetWorth {
			t.Fatalf("unexpected opening state: %+v", state)
		}
	}
	for _, owner := range data.Ownership {
		if owner.ShareBPS != 10000 {
			t.Fatalf("unexpected ownership baseline: %+v", owner)
		}
	}
}
