package desktop

import (
	"image"
	"reflect"
	"sync"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestAccountPositionImportAfterHistoryMatchesWails(t *testing.T) {
	ctx := t.Context()
	app, session, paths, ids := openExistingBuyFixture(t)
	ref, _, _, refIDs := openExistingBuyFixture(t)
	adapter := historyapi.NewService(ref)
	clock := func() time.Time { return accountTestClock }
	app.SetClock(clock)
	ref.SetClock(clock)
	fault := &cashMovementFault{Service: app, loseAttempt: 2, started: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.release) })
	p := newAccountsPage(ctx, app, clock, func() {})
	p.investment = &accountInvestment{service: fault}
	ledger := newLedgerHistory(ctx, fault, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{accounts: p, history: ledger, version: app.MobileDataVersion})
	h.size = image.Pt(640, 790)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	t.Cleanup(func() { release(); p.workers.Wait() })
	finish := func() {
		for range 4 {
			h.frame()
			p.workers.Wait()
			ledger.model.Wait()
			ledger.reads.workers.Wait()
		}
	}
	click := func(text string) { t.Helper(); h.click(text, false); finish() }
	label := func(text string) { t.Helper(); assertPageLabel(t, &h.router, text) }

	finish()
	click("Accounts")
	click("Harbor Brokerage")
	scrollToAccountDetailLabel(h, "Record existing position")
	click("Record existing position")
	d := p.investment.draft
	if len(d.sources.Instruments) != 1 || d.instrument != ids.foreign || d.currency != "EUR" {
		t.Fatal("picker must exclude the already-held domestic security")
	}
	h.focused(&d.quantity.Editor)
	before := app.MobileDataVersion()
	const costLabel = "Original cost per unit · EUR"
	setBankField(h, "Existing quantity", &d.quantity, "2.12345678")
	setBankField(h, costLabel, &d.unitCost, "1.123456789")
	click("Review existing position")
	label("Enter a non-negative original cost per unit with up to eight decimal places.")
	if d.quantity.Text() != "2.12345678" || d.unitCost.Text() != "1.123456789" {
		t.Fatal("validation lost the import draft")
	}
	click("Cancel")
	h.focused(&p.investment.importOpen)
	if app.MobileDataVersion() != before {
		t.Fatal("opening, reviewing or canceling import changed the ledger")
	}

	scrollToAccountDetailLabel(h, "Record existing position")
	click("Record existing position")
	d = p.investment.draft
	setBankField(h, "Existing quantity", &d.quantity, "2.12345678")
	setBankField(h, costLabel, &d.unitCost, "0")
	setBankField(h, "Notes", &d.note, "  Sample transfer in  ")
	// Use a civil wall time after Origin but before the sample's current time.
	d.day, d.time, d.defaultTime = "2026-11-22", "00:45:00", time.Time{}
	finish()
	click("Review existing position")
	label("Quantity 2.12345678")
	label("Original cost per unit · 0 EUR")
	label("This position does not move cash.")
	h.focused(&d.confirm)

	for _, setup := range []struct {
		api *historyapi.Service
		id  domain.AccountID
	}{{historyapi.NewService(app), ids.account}, {adapter, refIDs.account}} {
		_, err := setup.api.RecordChange(ctx, historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeMoneyAdded, AccountID: setup.id.String(), Currency: "USD", Amount: "2",
			Reason: "contribution", EffectiveAt: clock().Add(-time.Minute).Format(time.RFC3339Nano),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	click("Save position")
	label("The ledger changed. Review the position again before saving.")
	if d.instrument != ids.foreign || d.quantity.Text() != "2.12345678" || d.unitCost.Text() != "0" || d.time != "00:45:00" {
		t.Fatal("stale preview lost compatible import input")
	}
	click("Review existing position")
	note := "Sample transfer in"
	request := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangePositionImport, AccountID: refIDs.account.String(), InstrumentID: refIDs.foreign.String(),
		Currency: "EUR", Quantity: "2.12345678", UnitCost: "0", Note: &note,
		EffectiveLocalDate: "2026-11-22", EffectiveLocalTime: "00:45:00",
	}
	want, err := adapter.RecordChange(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	plan := *d.review
	h.press(key.NameReturn)
	select {
	case <-fault.started:
	case <-time.After(5 * time.Second):
		t.Fatal("import did not confirm")
	}
	release()
	finish()
	h.focused(&d.retryConfirm)
	click("Retry same submission")
	if ledger.reads.detail.err == nil {
		t.Fatal("expected the simulated read failure after acknowledgement")
	}
	click("Retry")
	if len(fault.attempts) != 3 || ledger.reads.detail.id != fault.firstActivity {
		t.Fatal("retrying a receipt read changed the submission or the activity")
	}
	label("Position adjustment")
	label("Unit cost · 0 EUR")
	id := ledger.reads.detail.id
	got, err := app.Activity(ctx, id)
	if err != nil || len(got.Effects) != 1 || len(want.Effects) != 1 {
		t.Fatalf("unexpected import activity: %+v / %v", got, err)
	}
	if got.Kind != domain.ActivityPositionTransfer || wire.FormatTime(got.EffectiveAt) != want.Activity.EffectiveAt ||
		!reflect.DeepEqual(got.Note, want.Activity.Note) || got.Effects[0].Target != domain.EffectTargetHoldingQuantity ||
		got.Effects[0].Quantity == nil || want.Effects[0].Quantity == nil || got.Effects[0].Quantity.Canonical() != *want.Effects[0].Quantity ||
		got.Effects[0].CostUnitPrice == nil || got.Effects[0].CostUnitPrice.Canonical() != "0" || got.Effects[0].HoldingID == nil ||
		got.Effects[0].InstrumentID == nil || *got.Effects[0].InstrumentID != ids.foreign {
		t.Fatalf("import differs from Wails: %+v / %+v", got, want)
	}
	click("Back to account")
	h.focused(&p.investment.importOpen)
	scrollToAccountDetailLabel(h, "Record existing position")
	click("Record existing position")
	label("No unused securities are available for this account.")
	click("Cancel")
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	reopened.SetClock(clock)
	posted, err := plan.Commit(ctx, reopened)
	if err != nil || posted.Activity.ID != got.ID {
		t.Fatalf("retry across reopen did not read the same receipt: %+v / %v", posted, err)
	}
	holdings, err := reopened.HoldingsByAccounts(ctx, []domain.AccountID{ids.account})
	if err != nil || len(holdings[ids.account]) != 2 {
		t.Fatalf("unexpected persisted positions: %+v / %v", holdings, err)
	}
	for _, holding := range holdings[ids.account] {
		if holding.InstrumentID == ids.foreign && (*got.Effects[0].HoldingID != holding.ID || holding.Quantity.Canonical() != "2.12345678") {
			t.Fatalf("wrong quantity or ledger link: %+v", holding)
		}
	}
	if _, err := history.ReviewPositionImport(ctx, reopened, history.PositionImportInput{
		Account: ids.account, Instrument: ids.foreign, Currency: "EUR", Quantity: "1", UnitCost: "0", EffectiveAt: clock(),
	}); err == nil {
		t.Fatal("duplicate active position was offered")
	}
	assertNativeTradeCash(t, reopened, ids.account, map[domain.CurrencyCode]string{"USD": "158"})
	assertNativeTradeCash(t, ref, refIDs.account, map[domain.CurrencyCode]string{"USD": "158"})
}
