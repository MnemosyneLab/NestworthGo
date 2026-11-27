package desktop

import (
	"image"
	"reflect"
	"strings"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type reconciliationDebtAccounts struct {
	holdings, loan, property, unrelated domain.AccountID
}

func seedReconciliationDebtAccounts(t *testing.T, app *application.Service, holdings domain.AccountID) reconciliationDebtAccounts {
	t.Helper()
	state, err := app.Bootstrap(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	add := func(name, kind, mode, role, amount string) domain.AccountID {
		t.Helper()
		row, err := app.CreateAccount(t.Context(), application.AccountInput{Name: name, AccountType: kind, TrackingMode: mode,
			BalanceSheetRole: role, DefaultCurrency: "USD", InitialAmount: amount, IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{state.Members[0].ID}})
		if err != nil {
			t.Fatal(err)
		}
		return row.Account.ID
	}
	return reconciliationDebtAccounts{holdings: holdings,
		loan:      add("Synthetic Fixed Loan", "loan", "balance", "liability", "80"),
		property:  add("Synthetic Manual Property", "property", "manual_value", "asset", "410.25"),
		unrelated: add("Synthetic Unrelated Savings", "bank_account", "balance", "asset", "12.34")}
}

func startDebtValueReconciliation(t *testing.T, api *reconciliation.Service, ids reconciliationDebtAccounts) domain.ReconciliationSession {
	t.Helper()
	p, err := api.Preview(t.Context(), application.ReconciliationAction{Action: "start",
		AccountIDs: []string{ids.holdings.String(), ids.loan.String(), ids.property.String()}})
	if err != nil {
		t.Fatal(err)
	}
	session, err := api.Commit(t.Context(), p.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func TestLinkedDebtAndValueMatchWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, seeded := openExistingBuyFixture(t)
	ref, refStorage, refPaths, refSeeded := openExistingBuyFixture(t)
	ids, refIDs := seedReconciliationDebtAccounts(t, app, seeded.account), seedReconciliationDebtAccounts(t, ref, refSeeded.account)
	app.SetClock(linkedContractClock)
	ref.SetClock(linkedContractClock)
	api, refAPI := reconciliation.NewService(app), reconciliation.NewService(ref)
	session, reference := startDebtValueReconciliation(t, api, ids), startDebtValueReconciliation(t, refAPI, refIDs)
	fault := &linkedDatedFault{Service: app}
	c := &holdingReconciliation{service: api, ledger: fault, clock: linkedContractClock}
	tasks := newTasksPage(ctx, app, func() {})
	tasks.checks, tasks.holdingRecon = app, c
	shell := newShellHarnessWithPages(t, ledgerPages{tasks: tasks, version: app.MobileDataVersion})
	shell.size = image.Pt(665, 1000)
	shell.shell.prefs.saved.Language = settings.LanguageEnglish
	h := &linkedProductHarness{previewHarness: shell, c: c, tasks: tasks}
	h.settle()
	h.activate(t, "Tasks & checks")
	h.activate(t, "Reconciliation")
	label := "Open reconciliation · " + strings.Join(c.directory.sessions[0].AccountNames, " · ")
	h.activate(t, label)
	if c.id != session.ID {
		t.Fatal("wrong reconciliation session opened")
	}
	before, err := app.ListActivities(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	untouched := readBankRecord(t, app, ids.unrelated)

	type step struct {
		action, button, item, amount, fee, clock, note string
		peerLabel, peerName                            string
		account, referenceAccount, peer                domain.AccountID
		kind                                           historyapi.ChangeCommandKind
	}
	draw := step{action: "Debt draw", button: "Record dated cash change · USD", item: "cash:USD",
		amount: "30.125", clock: "12:00", note: "Synthetic linked debt draw", peerLabel: "Debt account", peerName: "Synthetic Fixed Loan",
		account: ids.holdings, referenceAccount: refIDs.holdings, peer: ids.loan, kind: historyapi.ChangeDebtDraw}
	payment := step{action: "Debt payment", button: "Record a change", item: "balance", amount: "10.1234", fee: "0.0567",
		clock: "13:00", note: "Synthetic linked debt payment", peerLabel: "Cash account", peerName: "Harbor Brokerage",
		account: ids.loan, referenceAccount: refIDs.loan, peer: ids.holdings, kind: historyapi.ChangeDebtPayment}
	value := step{action: "Value update", button: "Record a change", item: "balance", amount: "424.5678",
		clock: "14:00", note: "Synthetic linked property value", account: ids.property, referenceAccount: refIDs.property, kind: historyapi.ChangeValueUpdate}
	const day = "2026-11-23"
	entered := func(s step) (*cashMovementDraft, *historyCashMovements) {
		t.Helper()
		button := &c.checkpointActions(s.account.String(), s.item).cash
		if s.item == "balance" {
			// Several accounts have a balance checkpoint with the same label.
			// Activate the specific button and assert the resulting immutable link.
			h.router.Source().Execute(key.FocusCmd{Tag: button})
			h.press(key.NameReturn)
			h.settle()
		} else {
			h.activate(t, s.button)
		}
		f := c.cashDated
		if f == nil || f.linked.link().AccountID != s.account.String() || f.linked.link().ItemID != s.item {
			t.Fatalf("wrong checkpoint form for %s", s.action)
		}
		f.draft.day, f.draft.time = day, s.clock
		f.draft.note.Editor.SetText(s.note)
		h.activate(t, s.action)
		d := f.draft
		if d.account != s.account || d.day != day || d.time != s.clock || d.note.Text() != s.note {
			t.Fatal("switching operation lost checkpoint scope or draft date/note")
		}
		if s.peerName != "" {
			h.focused(&d.destinationPicker.Trigger)
			pickAccountReference(h.previewHarness, s.peerLabel, s.peerName, s.peerName+" · USD")
			h.settle()
			if d.destination != s.peer {
				t.Fatal("wrong selected debt counterpart")
			}
			setBankField(h.previewHarness, "Debt principal · USD", &d.amount, s.amount)
			if s.fee != "" {
				setBankField(h.previewHarness, "Interest or fee (optional) · USD", &d.fee, s.fee)
			}
		} else {
			if d.movement != history.CashValueUpdate || d.amount.Text() != "410.25" || !f.valueUnchanged() || findPageLabel(&h.router, "Debt draw") != nil {
				t.Fatal("manual-value checkpoint offered debt or lost its current-value default")
			}
			setBankField(h.previewHarness, "New value · USD", &d.amount, s.amount)
			d.reason = domain.ReasonReconciliation
		}
		h.activate(t, "Preview change")
		if d.review == nil {
			t.Fatalf("preview %s failed: %v", s.action, d.problem)
		}
		h.focused(&d.post)
		return d, f
	}
	d, _ := entered(draw)
	h.activate(t, "Back to details")
	h.focused(&d.amount.Editor)
	if d.amount.Text() != draw.amount || d.destination != draw.peer || d.note.Text() != draw.note {
		t.Fatal("Back changed the debt draft")
	}
	h.activate(t, "Cancel")
	h.focused(&c.checkpointActions(ids.holdings.String(), draw.item).cash)
	if rows, err := app.ListActivities(ctx, 100); err != nil || !reflect.DeepEqual(rows, before) || len(c.links) != 0 {
		t.Fatal("cancel wrote financial evidence or a link")
	}

	wails := historyapi.NewService(ref)
	for _, s := range []step{draw, payment, value} {
		d, f := entered(s)
		if s.kind == historyapi.ChangeDebtDraw {
			current := session
			for _, action := range []string{"pause", "resume"} {
				r, err := api.Preview(ctx, application.ReconciliationAction{Action: action, SessionID: current.ID, ExpectedRevision: current.Revision})
				if err != nil {
					t.Fatal(err)
				}
				current, err = api.Commit(ctx, r.PlanID)
				if err != nil {
					t.Fatal(err)
				}
			}
			h.press(key.NameReturn)
			h.settle()
			if d.review != nil || f.draft != d || len(c.links) != 0 || d.amount.Text() != s.amount || d.destination != s.peer || d.note.Text() != s.note {
				t.Fatal("stale debt review wrote or lost its draft")
			}
			h.focused(&d.amount.Editor)
			h.activate(t, "Preview change")
		}
		acct, _, ok := checkpointInSession(&reference, s.referenceAccount.String(), s.item, false)
		if !ok {
			t.Fatal("missing matching Wails checkpoint")
		}
		req := historyapi.ChangeCommandRequest{Kind: s.kind, EffectiveLocalDate: day, EffectiveLocalTime: s.clock, Note: &s.note,
			MutationID: domain.NewMutationID().String(), ReconciliationLink: &domain.ReconciliationLink{SessionID: reference.ID,
				AccountID: acct.AccountID, ItemID: s.item, ExpectedRevision: reference.Revision, FinancialRevision: acct.FinancialRevision}}
		if s.kind == historyapi.ChangeValueUpdate {
			req.AccountID, req.NewValue, req.NewValueCurrency, req.Reason = refIDs.property.String(), s.amount, "USD", "reconciliation"
		} else {
			req.CashAccountID, req.DebtAccountID, req.Principal, req.PrincipalCurrency = refIDs.holdings.String(), refIDs.loan.String(), s.amount, "USD"
			req.InterestOrFee, req.InterestOrFeeCurrency = s.fee, "USD"
		}
		want, err := wails.PreviewChange(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		got := wire.FromReviewedChangePreview(d.review.Preview)
		if got.Activity.Kind != want.Activity.Kind || got.Activity.Reason != want.Activity.Reason ||
			got.Activity.EffectiveAt != want.Activity.EffectiveAt || len(got.Review) != len(want.Review) || len(got.Effects) != len(want.Effects) {
			t.Fatalf("linked preview differs from Wails: %+v / %+v", got, want)
		}
		for i, a := range got.Review {
			b := want.Review[i]
			if a.AccountName != b.AccountName || a.Before != b.Before || a.After != b.After || a.Currency != b.Currency {
				t.Fatalf("reviewed endpoint differs: %+v / %+v", a, b)
			}
		}
		if _, err := wails.RecordChange(ctx, req); err != nil {
			t.Fatal(err)
		}
		reference, err = refAPI.Get(ctx, reference.ID)
		if err != nil {
			t.Fatal(err)
		}
		fault.loseNext = s.kind == historyapi.ChangeDebtPayment
		h.press(key.NameReturn)
		h.settle()
		if s.kind == historyapi.ChangeDebtPayment {
			if !c.unresolved() || !d.uncertain {
				t.Fatal("lost result released the reviewed debt command")
			}
			h.press(key.NameReturn)
			h.settle()
		}
		h.focused(&c.checkpointActions(s.account.String(), s.item).cash)
	}
	if len(fault.attempts) != 5 || fault.attempts[0].mutation == fault.attempts[1].mutation || fault.attempts[2] != fault.attempts[3] || len(c.links) != 3 {
		t.Fatal("unexpected linked change or retry identity")
	}
	for _, s := range []struct {
		storage interface{ Close() error }
		paths   pilot.Paths
		ids     reconciliationDebtAccounts
		session domain.ReconciliationSession
	}{{storage, paths, ids, session}, {refStorage, refPaths, refIDs, reference}} {
		if err := s.storage.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, _ := reopenBaselineProfile(t, s.paths)
		reopened.SetClock(linkedContractClock)
		assertNativeTradeCash(t, reopened, s.ids.holdings, map[domain.CurrencyCode]string{"USD": "175.9449"})
		if loan := readBankRecord(t, reopened, s.ids.loan).LatestValue; loan == nil || loan.Amount.CanonicalAmount() != "100.0016" {
			t.Fatalf("reopened debt = %+v", loan)
		}
		if property := readBankRecord(t, reopened, s.ids.property).LatestValue; property == nil || property.Amount.CanonicalAmount() != value.amount {
			t.Fatalf("reopened value = %+v", property)
		}
		if record := readBankRecord(t, reopened, s.ids.unrelated); record.LatestValue.Amount.CanonicalAmount() != "12.34" ||
			s.ids.unrelated == ids.unrelated && !reflect.DeepEqual(record, untouched) {
			t.Fatal("unrelated balance changed")
		}
		rows, err := reopened.ListActivities(ctx, 100)
		if err != nil || len(rows) != len(before)+3 || s.ids.holdings == ids.holdings && !reflect.DeepEqual(rows[3:], before) {
			t.Fatal("only the three explicit linked activities may post")
		}
		links, err := reconciliation.NewService(reopened).Links(ctx, s.session.ID)
		if err != nil || len(links) != 3 {
			t.Fatalf("persisted links: %+v, %v", links, err)
		}
		scopes := []domain.AccountID{s.ids.property, s.ids.loan, s.ids.holdings}
		for i, expected := range []step{value, payment, draw} {
			a := rows[i]
			at, _ := time.Parse(time.RFC3339, []string{"2026-11-23T06:00:00Z", "2026-11-23T05:00:00Z", "2026-11-23T04:00:00Z"}[i])
			if string(a.Kind) != string(expected.kind) || a.Note == nil || *a.Note != expected.note || !a.EffectiveAt.Equal(at) {
				t.Fatalf("reopened activity changed kind, date or note: %+v", a)
			}
			matched := false
			for _, link := range links {
				matched = matched || link.AccountID == scopes[i].String() && link.ItemID == expected.item && link.OperationID == a.ID.String()
			}
			if !matched {
				t.Fatal("reopened activity is not linked to its original checkpoint")
			}
		}
	}
}
