package desktop

import (
	"reflect"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type linkedTransferPeers struct {
	savings, manual, unrelated domain.AccountID
}

func seedLinkedTransferPeers(t *testing.T, app *application.Service) linkedTransferPeers {
	t.Helper()
	ctx := t.Context()
	bootstrap, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	create := func(name, kind, mode, role, currency, amount string) domain.AccountID {
		t.Helper()
		a, err := app.CreateAccount(ctx, application.AccountInput{Name: name, AccountType: kind, TrackingMode: mode,
			BalanceSheetRole: role, DefaultCurrency: currency, InitialAmount: amount, IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
		if err != nil {
			t.Fatal(err)
		}
		return a.Account.ID
	}
	peers := linkedTransferPeers{
		savings:   create("Dollar Savings", "bank_account", "balance", "asset", "USD", "100.5"),
		manual:    create("Euro Manual Value", "property", "manual_value", "asset", "EUR", "410.25"),
		unrelated: create("Untouched Savings", "bank_account", "balance", "asset", "USD", "12.34"),
	}
	create("Synthetic Liability", "credit_card", "balance", "liability", "USD", "4")
	archived := create("Archived Savings", "bank_account", "balance", "asset", "USD", "3")
	if err := app.ArchiveAccount(ctx, archived, true); err != nil {
		t.Fatal(err)
	}
	return peers
}

func TestLinkedCashTransferMatchesWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	ref, refStorage, refPaths, refIDs := openExistingBuyFixture(t)
	peers, refPeers := seedLinkedTransferPeers(t, app), seedLinkedTransferPeers(t, ref)
	app.SetClock(linkedContractClock)
	ref.SetClock(linkedContractClock)
	api, refAPI := reconciliation.NewService(app), reconciliation.NewService(ref)
	session, refSession := startTestHoldingReconciliation(t, api, ids.account), startTestHoldingReconciliation(t, refAPI, refIDs.account)
	before, err := app.ListActivities(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	fault := &linkedDatedFault{Service: app}
	h := newLinkedProductHarness(t, app, api)
	c := h.c
	c.ledger = fault
	cash := &c.checkpointActions(ids.account.String(), "cash:USD").cash
	const day = "2026-11-23"
	type sample struct {
		incoming            bool
		from, to, fee       string
		destination, search string
		time, note          string
	}
	out := sample{false, "10.1234", "10.1234", "0.0123", "Dollar Savings · USD", "Dollar Savings", "12:00:00", "Synthetic linked outgoing cash"}
	in := sample{true, "20.125", "22.2345", "0.125", "Euro Manual Value · EUR", "Euro Manual", "13:00:00", "Synthetic linked incoming cash"}
	edit := func(s sample) *cashMovementDraft {
		t.Helper()
		h.activate(t, "Record dated cash change · USD")
		f := c.cashDated
		f.draft.day, f.draft.time = day, s.time
		f.draft.note.Editor.SetText(s.note)
		h.activate(t, "Cash transfer")
		d := f.draft
		if d.movement != history.CashTransfer || f.linked.link().ItemID != "cash:USD" || d.account != ids.account || d.currency != "USD" || d.time != s.time || d.note.Text() != s.note {
			t.Fatal("transfer lost its checkpoint, endpoint or draft")
		}
		h.focused(&d.destinationPicker.Trigger)
		h.press(key.NameReturn)
		h.settle()
		assertPageLabel(t, &h.router, s.destination)
		for _, invalid := range []string{"Synthetic Liability · USD", "Archived Savings · USD", "Harbor Brokerage · USD"} {
			if findPageLabel(&h.router, invalid) != nil {
				t.Fatalf("ineligible peer offered: %s", invalid)
			}
		}
		h.press(key.NameEscape)
		h.settle()
		label, currency := "To account", "USD"
		if s.incoming {
			h.activate(t, "Transfer in")
			label, currency = "From account", "EUR"
		}
		pickAccountReference(h.previewHarness, label, s.search, s.destination)
		h.settle()
		if s.incoming && (d.account != peers.manual || d.destination != ids.account) || !s.incoming && (d.account != ids.account || d.destination != peers.savings) || d.currency.String() != currency || d.toCurrency != "USD" {
			t.Fatalf("wrong transfer endpoints or currencies: %v -> %v, %s -> %s", d.account, d.destination, d.currency, d.toCurrency)
		}
		setBankField(h.previewHarness, "Sent amount · "+currency, &d.amount, s.from)
		if s.incoming {
			setBankField(h.previewHarness, "Received amount · USD", &d.received, s.to)
		} else {
			assertPageLabel(t, &h.router, "Received amount · "+s.to+" USD")
		}
		setBankField(h.previewHarness, "Fee in source currency (optional) · "+currency, &d.fee, s.fee)
		h.activate(t, "Preview change")
		if d.review == nil {
			t.Fatalf("could not review linked transfer: %v", d.problem)
		}
		h.focused(&d.post)
		return d
	}
	d := edit(out)
	h.activate(t, "Back to details")
	h.focused(&d.amount.Editor)
	if d.amount.Text() != out.from || d.note.Text() != out.note || d.destination != peers.savings {
		t.Fatal("review Back lost the scoped amount or destination")
	}
	h.activate(t, "Preview change")
	h.activate(t, "Cancel")
	h.focused(cash)
	rows, err := app.ListActivities(ctx, 100)
	if err != nil || !reflect.DeepEqual(rows, before) || len(c.links) != 0 {
		t.Fatal("Cancel recorded an activity or a link")
	}
	wails := historyapi.NewService(ref)
	for _, s := range []sample{out, in} {
		d := edit(s)
		if !s.incoming {
			current := session
			for _, action := range []string{"pause", "resume"} {
				review, err := api.Preview(ctx, application.ReconciliationAction{Action: action, SessionID: current.ID, ExpectedRevision: current.Revision})
				if err != nil {
					t.Fatal(err)
				}
				current, err = api.Commit(ctx, review.PlanID)
				if err != nil {
					t.Fatal(err)
				}
			}
			h.press(key.NameReturn)
			h.settle()
			if d.review != nil || d.amount.Text() != s.from || d.note.Text() != s.note || d.destination != peers.savings || len(c.links) != 0 {
				t.Fatal("stale transfer posted or lost the original draft")
			}
			h.focused(&d.amount.Editor)
			h.activate(t, "Preview change")
		}
		from, to, currency := refIDs.account, refPeers.savings, "USD"
		if s.incoming {
			from, to, currency = refPeers.manual, refIDs.account, "EUR"
		}
		request := historyapi.ChangeCommandRequest{Kind: historyapi.ChangeCashTransfer, FromAccountID: from.String(), ToAccountID: to.String(),
			Sent: s.from, SentCurrency: currency, Received: s.to, ReceivedCurrency: "USD", Fee: s.fee, FeeCurrency: currency,
			EffectiveLocalDate: day, EffectiveLocalTime: s.time, Note: &s.note, MutationID: domain.NewMutationID().String(),
			ReconciliationLink: &domain.ReconciliationLink{SessionID: refSession.ID, AccountID: refIDs.account.String(), ItemID: "cash:USD",
				ExpectedRevision: refSession.Revision, FinancialRevision: refSession.Accounts[0].FinancialRevision}}
		want, err := wails.PreviewChange(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		got := wire.FromReviewedChangePreview(d.review.Preview)
		if got.Activity.EffectiveAt != want.Activity.EffectiveAt || !reflect.DeepEqual(got.DerivedRate, want.DerivedRate) || len(got.Review) != len(want.Review) {
			t.Fatalf("preview differs from Wails: %+v / %+v", got, want)
		}
		for i, actual := range got.Review {
			expected := want.Review[i]
			if actual.Currency != expected.Currency || actual.Before != expected.Before || actual.Delta != expected.Delta || actual.After != expected.After {
				t.Fatalf("review differs from Wails: %+v / %+v", actual, expected)
			}
		}
		if _, err := wails.RecordChange(ctx, request); err != nil {
			t.Fatal(err)
		}
		refSession, err = refAPI.Get(ctx, refSession.ID)
		if err != nil {
			t.Fatal(err)
		}
		fault.loseNext = s.incoming
		h.press(key.NameReturn)
		h.settle()
		if s.incoming {
			if !c.unresolved() || !d.uncertain {
				t.Fatal("unknown outcome did not retain the transfer")
			}
			h.press(key.NameReturn)
			h.settle()
		}
		h.focused(cash)
	}
	if len(fault.attempts) != 4 || fault.attempts[0].mutation == fault.attempts[1].mutation || fault.attempts[2] != fault.attempts[3] || len(c.links) != 2 {
		t.Fatalf("retry or link evidence differs: %+v / %+v", fault.attempts, c.links)
	}
	links := c.links
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	if err := refStorage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	reference, _ := reopenOnboardingProfile(t, refPaths)
	for _, copy := range []struct {
		app     *application.Service
		account domain.AccountID
		peers   linkedTransferPeers
	}{{reopened, ids.account, peers}, {reference, refIDs.account, refPeers}} {
		copy.app.SetClock(linkedContractClock)
		assertNativeTradeCash(t, copy.app, copy.account, map[domain.CurrencyCode]string{"USD": "168.0988"})
		for _, row := range []struct {
			id     domain.AccountID
			amount string
		}{{copy.peers.savings, "110.6234"}, {copy.peers.manual, "390"}, {copy.peers.unrelated, "12.34"}} {
			records, err := copy.app.ListAccounts(ctx, domain.AccountFilter{AccountIDs: []domain.AccountID{row.id}})
			if err != nil || len(records) != 1 || records[0].LatestValue == nil || records[0].LatestValue.Amount.CanonicalAmount() != row.amount {
				t.Fatalf("peer balance differs after reopen: %+v / %v", records, err)
			}
		}
	}
	reopenedLinks, err := reconciliation.NewService(reopened).Links(ctx, session.ID)
	if err != nil || !reflect.DeepEqual(reopenedLinks, links) {
		t.Fatalf("transfer link IDs did not reopen: %+v / %v", reopenedLinks, err)
	}
	rows, err = reopened.ListActivities(ctx, 100)
	if err != nil || len(rows) != len(before)+2 || !reflect.DeepEqual(rows[2:], before) {
		t.Fatal("unexpected accounting rows after reopening")
	}
	for i, s := range []sample{in, out} {
		a := rows[i]
		utc, _ := time.Parse(time.RFC3339, []string{"2026-11-23T05:00:00Z", "2026-11-23T04:00:00Z"}[i])
		if a.Kind != domain.ActivityCashTransfer || a.Note == nil || *a.Note != s.note || !a.EffectiveAt.Equal(utc) || len(a.Effects) != 3 ||
			a.Effects[0].Money.CanonicalAmount() != s.from || a.Effects[1].Money.CanonicalAmount() != s.to || a.Effects[2].Money.CanonicalAmount() != s.fee || a.Effects[2].Money.Currency() != a.Effects[0].Money.Currency() {
			t.Fatalf("transfer legs, fee or timestamp changed after reopen: %+v", a)
		}
		matched := false
		for _, link := range reopenedLinks {
			matched = matched || link.AccountID == ids.account.String() && link.ItemID == "cash:USD" && link.OperationID == a.ID.String()
		}
		if !matched {
			t.Fatal("transfer did not link to the originating cash checkpoint")
		}
	}
}
