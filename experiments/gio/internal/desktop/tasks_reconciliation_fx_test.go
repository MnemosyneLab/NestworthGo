package desktop

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestReconciliationLinkedCurrencyConversionMatchesWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	ref, _, _, refIDs := openExistingBuyFixture(t)
	clock := func() time.Time { return accountTestClock }
	app.SetClock(clock)
	ref.SetClock(clock)
	adapter, refAPI := reconciliation.NewService(app), reconciliation.NewService(ref)
	session, refSession := startTestHoldingReconciliation(t, adapter, ids.account), startTestHoldingReconciliation(t, refAPI, refIDs.account)
	wails := historyapi.NewService(ref)

	tasks := newTasksPage(ctx, app, func() {})
	tasks.checks = app
	c := &holdingReconciliation{service: adapter, ledger: app, clock: clock}
	tasks.holdingRecon = c
	h := newShellHarnessWithPages(t, ledgerPages{tasks: tasks, version: app.MobileDataVersion})
	h.size = image.Pt(640, 1200)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 5 {
			h.frame()
			tasks.workers.Wait()
			if c.cashDated != nil {
				c.cashDated.workers.Wait()
			}
		}
	}
	click := func(s string) { t.Helper(); h.click(s, false); settle() }
	settle()
	click("Tasks & checks")
	click("Reconciliation")
	click("Open holdings reconciliation · Harbor Brokerage")
	cash := &c.checkpointActions(ids.account.String(), "cash:USD").cash
	type sample struct {
		soldCurrency, boughtCurrency string
		sold, bought, fee            string
		minute, note                 string
	}
	out := sample{"USD", "EUR", "10.1234", "8.7654", "0.1", "00:46:01", "Synthetic conversion sold from USD"}
	in := sample{"EUR", "USD", "2.5", "2.8125", "0.125", "00:47:02", "Synthetic conversion bought into USD"}
	prepare := func(w sample) *cashMovementDraft {
		t.Helper()
		click("Record dated cash change · USD")
		f := c.cashDated
		d := f.draft
		d.day, d.time = "2026-11-22", w.minute
		setBankField(h, "Notes", &d.note, w.note)
		click("Currency conversion")
		d = f.draft
		h.focused(&d.amount.Editor)
		if d.movement != history.CashConversion || f.linked.link().ItemID != "cash:USD" ||
			d.account != ids.account || d.currency != "USD" || d.time != w.minute || d.note.Text() != w.note {
			t.Fatal("conversion lost its originating account, default currency or dated draft")
		}
		if w.soldCurrency != "USD" {
			pickAccountReference(h, "Sold currency", w.soldCurrency, w.soldCurrency)
			settle()
		}
		pickAccountReference(h, "Bought currency", w.boughtCurrency, w.boughtCurrency)
		settle()
		setBankField(h, "Sold amount · "+w.soldCurrency, &d.amount, w.sold)
		setBankField(h, "Bought amount · "+w.boughtCurrency, &d.received, w.bought)
		setBankField(h, "Fee in source currency (optional) · "+w.soldCurrency, &d.fee, w.fee)
		click("Preview conversion")
		h.focused(&d.post)
		return d
	}
	before := app.MobileDataVersion()
	prepare(out)
	click("Cancel")
	h.focused(cash)
	if app.MobileDataVersion() != before || len(c.links) != 0 {
		t.Fatal("Cancel wrote conversion facts or a reconciliation link")
	}

	for _, w := range []sample{out, in} {
		d := prepare(w)
		req := historyapi.ChangeCommandRequest{Kind: historyapi.ChangeFXConversion, AccountID: refIDs.account.String(),
			Sold: w.sold, SoldCurrency: w.soldCurrency, Bought: w.bought, BoughtCurrency: w.boughtCurrency,
			Fee: w.fee, FeeCurrency: w.soldCurrency, EffectiveLocalDate: "2026-11-22", EffectiveLocalTime: w.minute, Note: &w.note,
			MutationID: domain.NewMutationID().String(),
			ReconciliationLink: &domain.ReconciliationLink{SessionID: refSession.ID, AccountID: refIDs.account.String(),
				ItemID: "cash:USD", ExpectedRevision: refSession.Revision, FinancialRevision: refSession.Accounts[0].FinancialRevision}}
		want, err := wails.PreviewChange(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		got := d.review.Preview
		if want.Activity.EffectiveAt != wire.FormatTime(got.Activity.EffectiveAt) || len(want.Resulting) != len(got.Resulting) ||
			want.DerivedRate == nil || got.DerivedRate == nil || *want.DerivedRate != got.DerivedRate.Canonical() {
			t.Fatalf("linked FX preview differs from Wails: %+v / %+v", want, got)
		}
		for index, result := range got.Resulting {
			e := want.Resulting[index]
			if e.Amount != result.Amount || e.Currency != result.Currency.String() {
				t.Fatalf("FX result differs from Wails: %+v / %+v", e, result)
			}
		}
		if _, err := wails.RecordChange(ctx, req); err != nil {
			t.Fatal(err)
		}
		refSession, err = refAPI.Get(ctx, refSession.ID)
		if err != nil {
			t.Fatal(err)
		}
		h.press(key.NameReturn)
		settle()
		h.focused(cash)
	}
	if len(c.links) != 2 {
		t.Fatalf("expected two linked currency conversions, got %d", len(c.links))
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	wantCash := map[domain.CurrencyCode]string{"USD": "148.5891", "EUR": "6.1404"}
	assertNativeTradeCash(t, reopened, ids.account, wantCash)
	assertNativeTradeCash(t, ref, refIDs.account, wantCash)
	links, err := reconciliation.NewService(reopened).Links(ctx, session.ID)
	if err != nil || len(links) != 2 {
		t.Fatalf("reopen lost the linked conversions: %+v / %v", links, err)
	}
	for _, link := range links {
		a, err := reopened.Activity(ctx, domain.ActivityID(link.OperationID))
		if err != nil || a.Kind != domain.ActivityFXConversion || a.Note == nil || link.AccountID != ids.account.String() || link.ItemID != "cash:USD" ||
			len(a.Effects) != 3 || a.EffectiveLocalDate != "2026-11-22" || (*a.Note != out.note && *a.Note != in.note) {
			t.Fatalf("wrong saved FX evidence: %+v / %v", a, err)
		}
	}
}
