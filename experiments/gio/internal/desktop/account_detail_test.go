package desktop

import (
	"image"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
)

func TestAccountDetailCompositionScopedActionsAndLastReconciliation(t *testing.T) {
	ctx := t.Context()
	clock := func() time.Time { return accountTestClock }
	seed, storage, paths, ids := openExistingBuyFixture(t)
	boot, err := seed.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bank, err := seed.CreateAccount(ctx, application.AccountInput{
		Name: "Synthetic balance bank", AccountType: "bank_account", TrackingMode: "balance", BalanceSheetRole: "asset",
		DefaultCurrency: "USD", InitialAmount: "100", OwnerIDs: []domain.MemberID{boot.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	card, err := seed.CreateAccount(ctx, application.AccountInput{
		Name: "Synthetic credit card", AccountType: "credit_card", TrackingMode: "balance", BalanceSheetRole: "liability",
		DefaultCurrency: "USD", InitialAmount: "40", OwnerIDs: []domain.MemberID{boot.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.CreateAccount(ctx, application.AccountInput{
		Name: "Synthetic custody peer", AccountType: "brokerage", TrackingMode: "holdings", BalanceSheetRole: "asset",
		DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{boot.Members[0].ID},
	}); err != nil {
		t.Fatal(err)
	}
	api := reconciliation.NewService(seed)
	last := startTestHoldingReconciliation(t, api, ids.account)
	source := domain.ReconciliationSource{AsOfDate: "2026-11-22", Reference: "Synthetic completed brokerage check"}
	for _, item := range last.Accounts[0].Items {
		last = commitStatementAction(t, api, application.ReconciliationAction{Action: "check", SessionID: last.ID,
			ExpectedRevision: last.Revision, AccountID: ids.account.String(), FinancialRevision: last.Accounts[0].FinancialRevision,
			ItemID: item.ID, Outcome: "confirmed", Source: source})
	}
	last = commitStatementAction(t, api, application.ReconciliationAction{Action: "finish", SessionID: last.ID, ExpectedRevision: last.Revision})
	bankSession := startTestHoldingReconciliation(t, api, bank.Account.ID)
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	app, _ := reopenBaselineProfile(t, paths)
	raw, err := os.ReadFile(filepath.Join(paths.RootPath(), "data", "nestworth.db"))
	if err != nil {
		t.Fatal(err)
	}
	copied := writeStage6Fixture(t, filepath.Join(t.TempDir(), "account-detail"), raw)
	_, _, reference := openStage6Database(t, copied, clock)
	wailsAPI := reconciliation.NewService(reference)
	want, err := wailsAPI.LastSuccess(ctx, ids.account.String())
	if err != nil || want == nil || want.ID != last.ID {
		t.Fatalf("fixture lacks its Wails completed record: %+v / %v", want, err)
	}
	never, err := wailsAPI.LastSuccess(ctx, bank.Account.ID.String())
	if err != nil || never != nil {
		t.Fatalf("open bank session must not be a successful record: %+v / %v", never, err)
	}

	p := newAccountsPage(ctx, app, clock, func() {})
	p.investment = &accountInvestment{service: app}
	p.liabilityPosting = &accountLiabilityPosting{service: app}
	p.balanceTarget = &accountBalanceTarget{service: app}
	p.cashBalance = &accountCashBalance{service: app}
	ledger := newLedgerHistory(ctx, app, clock, func() {})
	tasks := newTasksPage(ctx, app, func() {})
	tasks.checks = app
	tasks.holdingRecon = &holdingReconciliation{service: reconciliation.NewService(app), ledger: app, clock: clock}
	h := newShellHarnessWithPages(t, ledgerPages{accounts: p, history: ledger, tasks: tasks, version: app.MobileDataVersion})
	h.size = image.Pt(1280, 800)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	wait := func() {
		p.workers.Wait()
		p.detail.cashActions.form.workers.Wait()
		tasks.workers.Wait()
		ledger.model.Wait()
		ledger.reads.workers.Wait()
	}
	t.Cleanup(wait)
	settle := func() {
		for range 4 {
			h.frame()
			wait()
		}
		h.frame()
	}
	open := func(id domain.AccountID) {
		h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: accountsRoute, account: id}}
		settle()
	}
	click := func(text string) { t.Helper(); h.click(text, false); settle() }
	label := func(text string) { t.Helper(); assertPageLabel(t, &h.router, text) }
	open(ids.account)
	title := assertPageLabel(t, &h.router, "Harbor Brokerage")
	composition := assertPageLabel(t, &h.router, "Account composition")
	check := assertPageLabel(t, &h.router, "View last successful record")
	parent := assertPageLabel(t, &h.router, "Back to accounts")
	if parent.Max.Y > h.shell.chrome.height || title.Min.Y > h.shell.chrome.height || check.Min.Y > composition.Min.Y {
		t.Fatalf("account header sequence: title %v, parent %v, reconciliation %v, composition %v", title, parent, check, composition)
	}
	for _, shortcut := range []string{"Record debt activity", "Record buy", "Transfer position"} {
		if node := findPageLabel(&h.router, shortcut); node != nil && node.Desc.Bounds.Min.Y < composition.Min.Y {
			t.Fatalf("%q interrupted the balance, reconciliation and composition sequence", shortcut)
		}
	}
	label("By type")
	label("By detail")
	if !reflect.DeepEqual(p.current.lastCheck, want) {
		t.Fatal("Gio did not retain the shared completed record")
	}
	click("By detail")
	if !p.detail.composition.detail {
		t.Fatal("details grouping did not select shared detail slices")
	}
	click("View last successful record")
	if tasks.reconciliationAccount != ids.account || tasks.holdingRecon.session == nil || tasks.holdingRecon.session.ID != last.ID {
		t.Fatal("last-success link opened a different account or session")
	}
	h.shell.pendingNavigation = &navigationRequest{step: -1}
	settle()
	h.focused(&p.detail.receipt)
	if !p.detail.composition.detail {
		t.Fatal("Back dropped the selected composition grouping")
	}
	h.assertTitlebarAction("Record a change")
	click("Record a change")
	h.press(key.NameEscape)
	settle()
	h.focused(&p.detail.record.trigger)
	click("Record a change")
	for _, shown := range []string{"Buy investment", "Sell", "Record cash dividend", "Money added", "Transfer", "Convert currency"} {
		label(shown)
	}
	click("Buy investment")
	if draft := p.investment.draft; draft == nil || draft.account.ID != ids.account || draft.kind != domain.ActivityBuy {
		t.Fatal("buy changed or lost the selected brokerage scope")
	}
	click("Cancel")
	h.focused(&p.detail.record.trigger)

	version := app.MobileDataVersion()
	click("Record a change")
	click("Investment transfer")
	transfer := p.investment.draft
	if transfer == nil || transfer.custody == nil || transfer.custody.scope.account.ID != ids.account ||
		!transfer.custody.available() || transfer.originAction != &p.detail.record.trigger {
		t.Fatal("investment transfer did not open a usable account-scoped editor")
	}
	click("Transfer in")
	if transfer.custody.to != ids.account || transfer.custody.from != "" {
		t.Fatal("incoming transfer did not lock the selected account")
	}
	click("Cancel")
	h.focused(&p.detail.record.trigger)

	click("Record a change")
	click("Investment quantity adjustment")
	adjustment := p.investment.draft
	if adjustment == nil || adjustment.adjustment == nil || adjustment.adjustment.scope.account.ID != ids.account ||
		adjustment.originAction != &p.detail.record.trigger || adjustment.adjustment.sources == nil {
		t.Fatal("quantity adjustment did not open an account-scoped editor")
	}
	holdings := adjustment.adjustment.sources.Holdings
	if len(holdings) != 1 || holdings[0].Holding.ID != ids.holding {
		t.Fatal("quantity adjustment offered holdings outside the selected account")
	}
	click("Cancel")
	h.focused(&p.detail.record.trigger)
	if app.MobileDataVersion() != version {
		t.Fatal("canceling the position editors changed the ledger")
	}

	open(bank.Account.ID)
	label("Last successful reconciliation: No successful reconciliation yet")
	if findPageLabel(&h.router, "View last successful record") != nil {
		t.Fatal("open check was presented as a completed record")
	}
	click("Open account reconciliation")
	if tasks.reconciliationAccount != bank.Account.ID || len(tasks.holdingRecon.directory.sessions) == 0 {
		t.Fatal("account reconciliation did not retain the bank scope")
	}
	seen := false
	for _, session := range tasks.holdingRecon.directory.sessions {
		seen = seen || session.ID == bankSession.ID
	}
	if !seen || findPageLabel(&h.router, "Harbor Brokerage") != nil {
		t.Fatal("scoped reconciliation failed to hide an unrelated session")
	}
	h.shell.pendingNavigation = &navigationRequest{step: -1}
	settle()
	h.focused(&p.detail.reconcile)
	click("Record a change")
	for _, absent := range []string{"Buy investment", "Sell", "Convert currency", "Card purchase", "Investment transfer", "Investment quantity adjustment"} {
		if findPageLabel(&h.router, absent) != nil {
			t.Fatalf("balance account offered %q", absent)
		}
	}
	click("Money added")
	f := p.detail.cashActions.form
	label("Reason")
	if f.account != bank.Account.ID || f.draft.account != bank.Account.ID || f.draft.movement != history.CashContribution || f.draft.reason != domain.ReasonOther {
		t.Fatal("cash entry did not lock the selected bank account")
	}
	click("Cancel")
	h.focused(&p.detail.record.trigger)

	open(card.Account.ID)
	click("Record a change")
	if findPageLabel(&h.router, "Money added") != nil {
		t.Fatal("credit card offered a generic asset cash entry")
	}
	click("Card purchase")
	draft := p.liabilityPosting.draft
	if draft == nil || draft.account.ID != card.Account.ID || draft.kind != historyapi.ChangeMoneyAdded || draft.reason != domain.ReasonCardPurchase {
		t.Fatal("credit card did not preserve purchase mode, reason and account")
	}
}

// Account details use a virtual list. Scroll to materialize a newly lower row,
// then let the existing pointer helper locate and activate its actual control.
func scrollToAccountDetailLabel(h *previewHarness, label string) {
	h.t.Helper()
	for range 35 {
		if node := findPageLabel(&h.router, label); node != nil {
			center := node.Desc.Bounds.Min.Add(node.Desc.Bounds.Max).Div(2)
			if node.Desc.Gestures == 0 || center.Y >= h.shell.chrome.height+24 && center.Y < h.size.Y-16 {
				return
			}
		}
		centre := f32.Pt(float32(h.size.X*3/4), float32(h.size.Y*3/4))
		h.router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: centre, Scroll: f32.Pt(0, 102)})
		h.frame()
	}
	h.t.Fatalf("could not reveal account detail %q", label)
}

// Open settings through the actual titlebar menu and leave the modal ready for input.
func openAccountSettings(h *previewHarness) {
	h.t.Helper()
	h.click("More actions for "+h.shell.accounts.name(), false)
	h.click("Account settings", false)
	h.frame()
}

func TestAccountPresentationPreservesNativeCashAndActivityReason(t *testing.T) {
	prefs := settings.Default()
	view := ui.View{Language: i18n.English, DecimalSeparator: prefs.DecimalSeparator, GroupingSeparator: prefs.GroupingSeparator, TimeLocation: time.UTC}
	item := domain.AccountCompositionItem{Key: "cash:aud", Label: "AUD", NativeAmount: "800", NativeCurrency: "AUD",
		BaseAmountExact: "530.22", ShareBPS: 25, ValuedCount: 1}
	comp := &domain.AccountComposition{Status: "ready", Currency: "USD", TotalExact: "530.22", DetailSlices: []domain.AccountCompositionItem{item}}
	donut, _ := accountCompositionSpec(view, comp, true, false)
	row := donut.Slices[0]
	if row.LegendDetail != "A$800.00" || row.Text != "US$530.22" || row.Share != "0.2%" || donut.Compact {
		t.Fatalf("foreign composition legend: %+v", row)
	}
	comp.Currency = "AUD"
	same, _ := accountCompositionSpec(view, comp, true, false)
	if same.Slices[0].LegendDetail != "" {
		t.Fatal("the household-currency row repeated its amount")
	}
	kind, reason, note := domain.ActivityCashIn, domain.ReasonIncome, "Sample pay"
	amount, err := domain.ParseMoney("800", "AUD")
	if err != nil {
		t.Fatal(err)
	}
	history := accountCashHistoryTable(view, []domain.AccountCashValue{{
		Amount: amount, ObservationKind: "event", ActivityKind: &kind, ActivityReason: &reason, ActivityNote: &note,
	}})
	if len(history.Columns) != 6 || history.Columns[0].Title != "Type" || history.Columns[3].Title != "Currency" ||
		history.Rows[0][2] != "A$800.00" || history.Rows[0][3] != "AUD" {
		t.Fatalf("cash history columns or currency: %+v", history)
	}
	if lines := strings.Split(history.Rows[0][0], "\n"); len(lines) != 3 || lines[1] != "Income" || lines[2] != note {
		t.Fatalf("activity reason and note are not distinct: %q", history.Rows[0][0])
	}
}
