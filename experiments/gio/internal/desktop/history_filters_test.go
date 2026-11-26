package desktop

import (
	"context"
	"errors"
	"image"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type timelineReadFault struct {
	*cashMovementFault
	failPage atomic.Bool
	missing  domain.ActivityID
}

func (f *timelineReadFault) ListActivityPage(ctx context.Context, query domain.ActivityQuery) (domain.ActivityPage, error) {
	if query.ReversesActivityID == nil && query.CorrectionGroupID == nil && f.failPage.Swap(false) {
		return domain.ActivityPage{}, errors.New("synthetic timeline refresh failure")
	}
	return f.Service.ListActivityPage(ctx, query)
}

func (f *timelineReadFault) Activity(ctx context.Context, id domain.ActivityID) (domain.Activity, error) {
	if id == f.missing {
		return domain.Activity{}, &domain.Error{Code: domain.ErrNotFound}
	}
	return f.cashMovementFault.Activity(ctx, id)
}

func TestHistoryFiltersAndDetailsMatchWailsAndPreserveVisits(t *testing.T) {
	ctx := t.Context()
	root := createFixtureRoot(t)
	seed, session := openBankFixture(t, root)
	reserve, instrument := seedMixedTimeline(t, seed)
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reference, _ := openBankFixture(t, copyBankProfile(t, root))
	service, _ := openBankFixture(t, root)
	fault := &timelineReadFault{cashMovementFault: &cashMovementFault{Service: service}}
	page := newLedgerHistory(ctx, fault, func() time.Time { return accountTestClock }, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{history: page})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	wait := func() {
		page.reads.workers.Wait()
		page.cashMovements.workers.Wait()
		page.model.Wait()
	}
	t.Cleanup(wait)
	finish := func() { h.frame(); wait(); h.frame(); wait(); h.frame() }
	label := func(text string) { assertPageLabel(t, &h.router, text) }
	pick := func(field, search, name string) {
		t.Helper()
		pickAccountReference(h, field, search, name)
		finish()
	}
	h.frame()
	h.click("Ledger activity", false)
	finish()

	adapter := historyapi.NewService(reference)
	query := func(q historyapi.ActivityQueryRequest) wire.ActivityPageDTO {
		t.Helper()
		q.Limit = 50
		got, err := adapter.ListActivityPage(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	compare := func(want []wire.ActivityDTO) {
		t.Helper()
		rows := page.model.Rows()
		if len(rows) != len(want) {
			t.Fatalf("Gio loaded %d rows, Wails %d", len(rows), len(want))
		}
		for i, row := range rows {
			if row.ID.String() != want[i].ID || row.EffectiveLocalDate != want[i].EffectiveLocalDate || row.Kind.String() != want[i].Kind {
				t.Fatalf("Gio and Wails page disagree at row %d, got %+v, want %+v", i, row, want[i])
			}
		}
	}
	first := query(historyapi.ActivityQueryRequest{})
	compare(first.Activities)
	if first.Next == nil || !page.model.HasMore() {
		t.Fatal("fixture did not exercise keyset pagination")
	}
	h.click("Load more", false)
	finish()
	next := query(historyapi.ActivityQueryRequest{AfterID: first.Next.ID, AfterEffectiveAt: first.Next.EffectiveAt, AfterCreatedAt: first.Next.CreatedAt})
	all := append(slices.Clone(first.Activities), next.Activities...)
	compare(all)
	if page.model.HasMore() {
		t.Fatal("expected two pages")
	}

	h.router.Source().Execute(key.FocusCmd{Tag: &page.list})
	h.frame()
	h.press(key.NameEnd)
	end := page.list.Position.First
	row := page.model.Rows()[end]
	button := page.detailButton(row.ID)
	h.router.Source().Execute(key.FocusCmd{Tag: button})
	h.frame()
	h.press(key.NameReturn)
	finish()
	if page.reads.detail == nil || page.reads.detail.id != row.ID {
		t.Fatal("keyboard opening did not follow the row ID")
	}
	h.click("Back", false)
	finish()
	h.focused(button)
	if page.list.Position.First != end {
		t.Fatalf("end-of-list scroll changed on Back: %d -> %d", end, page.list.Position.First)
	}
	compare(all)
	h.click("Forward", false)
	finish()
	if page.reads.detail == nil || page.reads.detail.id != row.ID {
		t.Fatal("Forward did not restore the same detail")
	}
	h.click("Back to ledger activity", false)
	finish()

	pick("Account filter", "Reserve", "Cash Reserve · Archived")
	reserveID := reserve.String()
	compare(query(historyapi.ActivityQueryRequest{AccountID: &reserveID}).Activities)
	if page.model.Filters().Account != reserve || page.list.Position.First != 0 {
		t.Fatal("a new filter inherited an old scroll or account")
	}
	h.click("View", false)
	finish()
	label("Internal transfer")
	label("Received by Cash Reserve · 10 USD")
	if findPageLabel(&h.router, "External inflow") != nil || findPageLabel(&h.router, "External outflow") != nil {
		t.Fatal("a transfer principal was presented as an external flow")
	}
	h.click("Back to ledger activity", false)
	finish()
	h.click("Activity type", true)
	h.click("Currency conversion", false)
	finish()
	label("No activity matches these filters.")
	compare(query(historyapi.ActivityQueryRequest{AccountID: &reserveID, Kinds: []string{"fx_conversion"}}).Activities)
	h.click("Overview", false)
	finish()
	if h.shell.activeRoute() != overviewRoute {
		t.Fatalf("leaving the timeline reached %s", h.shell.activeRoute())
	}
	h.click("Ledger activity", false)
	finish()
	if !page.model.Filters().Empty() {
		t.Fatal("a new timeline visit inherited the previous filter")
	}
	h.click("Back", false)
	h.click("Back", false)
	finish()
	label("No activity matches these filters.")
	if !page.model.Filters().Equal(history.Filters{Account: reserve, Kind: domain.ActivityFXConversion}) {
		t.Fatal("Back did not restore the original filtered visit")
	}
	h.click("Clear filters", false)
	finish()

	h.click("Activity type", true)
	h.click("Cash transfer", false)
	finish()
	compare(query(historyapi.ActivityQueryRequest{Kinds: []string{"cash_transfer"}}).Activities)
	if len(page.model.Rows()) != 2 {
		t.Fatal("same- and cross-currency transfers did not share the Wails type")
	}
	h.click("View", false)
	finish()
	label("Sent from Harbor Checking · 20 USD")
	label("Received by Euro cash · 16 EUR")
	label("Executed rate · 1 USD = 0.8 EUR")
	label("Internal transfer")
	h.click("Back to ledger activity", false)
	finish()
	h.click("Clear filters", false)
	finish()

	pick("Instrument filter", "BOND", instrument.SelectionLabel())
	for _, field := range []string{"From date", "Through date"} {
		h.click(field, true)
		h.press(key.NameLeftArrow)
		h.press(key.NameReturn)
		finish()
	}
	if page.model.Filters().From != "2026-11-21" || page.model.Filters().To != "2026-11-21" {
		t.Fatalf("wrong History-local dates: %+v", page.model.Filters())
	}
	instID := instrument.ID.String()
	compare(query(historyapi.ActivityQueryRequest{InstrumentID: &instID, FromLocalDate: "2026-11-21", ToLocalDate: "2026-11-21"}).Activities)
	if rows := page.model.Rows(); len(rows) != 1 || rows[0].Kind != domain.ActivityBuy {
		t.Fatalf("unexpected intersection of instrument and dates: %+v", rows)
	}
	tradeID := page.model.Rows()[0].ID
	fault.failDetail.Store(true)
	h.click("View", false)
	finish()
	label("Could not load this activity.")
	h.click("Retry", false)
	finish()
	label(instrument.DisplayLabel())
	label("Unit price · 10 USD")
	label("Trade principal")
	label("Fee")
	wantTrade, err := adapter.Activity(ctx, tradeID.String())
	if err != nil || wantTrade.TradeDetail == nil || wantTrade.TradeDetail.UnitPrice != "10" ||
		page.reads.detail.record.Activity.TradeDetail.UnitPrice.Canonical() != "10" {
		t.Fatalf("trade detail contract failed: %+v, %v", wantTrade, err)
	}
	h.click("Back to ledger activity", false)
	finish()
	fault.missing = tradeID
	h.click("View", false)
	finish()
	label("This activity is unavailable.")
	if findPageLabel(&h.router, "Retry") != nil {
		t.Fatal("an unavailable activity was shown as a transient read failure")
	}
	h.click("Back to ledger activity", false)
	finish()

	// Save outside the filtered scope, then fail the first timeline refresh
	// after acknowledgement. Retry must be a filtered read, never a write.
	h.click("Record contribution", false)
	finish()
	draft := page.cashMovements.draft
	pick("Receiving account", "Harbor", "Harbor Checking · USD")
	setBankField(h, "Contribution amount", &draft.amount, "3")
	h.click("Preview contribution", false)
	finish()
	fault.failPage.Store(true)
	h.click("Confirm contribution", false)
	finish()
	label("Contribution posted.")
	label("External inflow")
	h.click("Back to ledger activity", false)
	finish()
	label("Could not load activity history.")
	h.click("Retry", false)
	finish()
	if page.model.Filters().Instrument != instrument.ID || len(page.model.Rows()) != 1 || page.model.Rows()[0].ID != tradeID ||
		len(fault.attempts) != 1 {
		t.Fatal("refresh retry changed the active scope or called the writer")
	}
}

func seedMixedTimeline(t *testing.T, service *application.Service) (domain.AccountID, domain.Instrument) {
	t.Helper()
	ctx := t.Context()
	boot, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := service.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil || len(rows) != 1 || rows[0].Account.Name != "Harbor Checking" {
		t.Fatalf("unexpected seed: %+v, %v", rows, err)
	}
	bank := rows[0].Account.ID.String()
	service.SetClock(func() time.Time { return accountTestClock.Add(-96 * time.Hour) })
	defer service.SetClock(func() time.Time { return accountTestClock })
	create := func(name, kind, mode, currency, amount string) domain.AccountID {
		t.Helper()
		row, err := service.CreateAccount(ctx, application.AccountInput{
			Name: name, AccountType: kind, BalanceSheetRole: "asset", TrackingMode: mode, DefaultCurrency: currency,
			InitialAmount: amount, OwnerIDs: []domain.MemberID{boot.Members[0].ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		return row.Account.ID
	}
	reserve := create("Cash Reserve", "cash_on_hand", "balance", "USD", "0")
	euro := create("Euro cash", "cash_on_hand", "balance", "EUR", "0")
	studio := create("Sample brokerage", "brokerage", "holdings", "USD", "")
	instrument, err := service.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Sample Bond", Symbol: "BOND", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	service.SetClock(func() time.Time { return accountTestClock })
	adapter := historyapi.NewService(service)
	record := func(day, minute string, req historyapi.ChangeCommandRequest) wire.ActivityDTO {
		t.Helper()
		req.EffectiveLocalDate, req.EffectiveLocalTime = day, minute
		output, err := adapter.RecordChange(ctx, req)
		if err != nil {
			t.Fatalf("could not seed %s: %v", req.Kind, err)
		}
		return output.Activity
	}
	const day = "2026-11-21"
	for range 55 {
		record(day, "01:00", historyapi.ChangeCommandRequest{Kind: historyapi.ChangeMoneyAdded, AccountID: bank, Amount: "1", Currency: "USD", Reason: "contribution"})
	}
	record(day, "02:00", historyapi.ChangeCommandRequest{Kind: historyapi.ChangeMoneyRemoved, AccountID: bank, Amount: "4", Currency: "USD", Reason: "other"})
	record(day, "03:00", historyapi.ChangeCommandRequest{Kind: historyapi.ChangeCashTransfer, FromAccountID: bank, ToAccountID: reserve.String(), Sent: "10", Received: "10", SentCurrency: "USD", ReceivedCurrency: "USD"})
	record(day, "04:00", historyapi.ChangeCommandRequest{Kind: historyapi.ChangeCashTransfer, FromAccountID: bank, ToAccountID: euro.String(), Sent: "20", Received: "16", SentCurrency: "USD", ReceivedCurrency: "EUR"})
	record(day, "05:00", historyapi.ChangeCommandRequest{Kind: historyapi.ChangeMoneyAdded, AccountID: studio.String(), Amount: "100", Currency: "USD", Reason: "contribution"})
	record(day, "06:00", historyapi.ChangeCommandRequest{Kind: historyapi.ChangeFXConversion, AccountID: studio.String(), Sold: "20", Bought: "16", SoldCurrency: "USD", BoughtCurrency: "EUR"})
	record(day, "07:00", historyapi.ChangeCommandRequest{Kind: historyapi.ChangeTrade, SettlementAccountID: studio.String(), InstrumentID: instrument.ID.String(), Side: "buy", Quantity: "2", Gross: "20", GrossCurrency: "USD", Fee: "1", FeeCurrency: "USD"})
	if err := service.ArchiveAccount(ctx, reserve, true); err != nil {
		t.Fatal(err)
	}
	return reserve, instrument
}

type delayedActivityReader struct {
	*application.Service
	id               domain.ActivityID
	entered, release chan struct{}
}

func (r *delayedActivityReader) Activity(ctx context.Context, id domain.ActivityID) (domain.Activity, error) {
	activity, err := r.Service.Activity(ctx, id)
	if id == r.id {
		close(r.entered)
		<-r.release // Return a known record even after the detail visit was canceled.
	}
	return activity, err
}

func TestHistoryBackAndForwardIgnoreAnObsoleteDetail(t *testing.T) {
	ctx := t.Context()
	service, _ := openBankFixture(t, createFixtureRoot(t))
	batch, err := service.ListActivityPage(ctx, domain.ActivityQuery{Limit: 2})
	if err != nil || len(batch.Activities) != 2 {
		t.Fatalf("expected two fixture activities: %+v, %v", batch, err)
	}
	old, current := batch.Activities[0], batch.Activities[1]
	fault := &delayedActivityReader{Service: service, id: old.ID, entered: make(chan struct{}), release: make(chan struct{})}
	released := false
	release := func() {
		if !released {
			close(fault.release)
			released = true
		}
	}
	page := newLedgerHistory(ctx, fault, func() time.Time { return accountTestClock }, func() {})
	t.Cleanup(func() { release(); page.reads.workers.Wait(); page.model.Wait() })
	h := newShellHarnessWithPages(t, ledgerPages{history: page})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	h.click("Ledger activity", false)
	page.model.Wait()
	page.reads.workers.Wait()
	h.frame()

	h.click("View", false)
	h.frame()
	select {
	case <-fault.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("the older detail read did not start")
	}
	h.click("Back", false)
	h.frame()
	if page.reads.detail != nil {
		t.Fatal("Back did not retire the pending detail")
	}
	button := page.detailButton(current.ID)
	h.router.Source().Execute(key.FocusCmd{Tag: button})
	h.frame()
	h.press(key.NameReturn)
	h.frame()
	release()
	page.reads.workers.Wait()
	h.frame()
	if page.reads.detail == nil || page.reads.detail.record == nil || page.reads.detail.record.Activity.ID != current.ID {
		t.Fatal("the older detail replaced the new visit")
	}
	h.click("Back", false)
	h.focused(button)
	h.click("Forward", false)
	page.reads.workers.Wait()
	h.frame()
	if page.reads.detail == nil || page.reads.detail.record == nil || page.reads.detail.record.Activity.ID != current.ID {
		t.Fatal("Forward did not reopen the current activity")
	}
}
