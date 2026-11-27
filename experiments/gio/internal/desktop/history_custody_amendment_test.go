package desktop

import (
	"context"
	"errors"
	"image"
	"os"
	"reflect"
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

type lostCustodyCorrection struct {
	*application.Service
	attempts []fixInvocation
	loseNext bool
}

func (f *lostCustodyCorrection) RecordFixChangeGuarded(ctx context.Context, id domain.ActivityID, command any, mutation, hash, token string) (domain.ChangePreview, error) {
	f.attempts = append(f.attempts, fixInvocation{id, command, mutation, hash, token})
	posted, err := f.Service.RecordFixChangeGuarded(ctx, id, command, mutation, hash, token)
	if err == nil && f.loseNext {
		f.loseNext = false
		return domain.ChangePreview{}, errors.New("synthetic lost custody acknowledgment")
	}
	return posted, err
}

func TestHistoryCustodyCorrectionAndReversalMatchWails(t *testing.T) {
	ctx := t.Context()
	seed, initialSession, paths, ids := openExistingBuyFixture(t)
	now := accountTestClock
	seed.SetClock(func() time.Time { return now })
	boot, err := seed.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	create := func(name string, existing bool) (domain.AccountID, domain.HoldingID) {
		t.Helper()
		rec, err := seed.CreateAccount(ctx, application.AccountInput{Name: name, AccountType: "brokerage",
			TrackingMode: "holdings", BalanceSheetRole: "asset", DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
		if err != nil {
			t.Fatal(err)
		}
		var holding domain.Holding
		if existing {
			holding, err = seed.CreateHolding(ctx, application.HoldingInput{AccountID: rec.Account.ID.String(), InstrumentID: ids.instrument.String(), Quantity: "0"})
			if err != nil {
				t.Fatal(err)
			}
		}
		return rec.Account.ID, holding.ID
	}
	target, _ := create("Original custody", false)
	other, otherHolding := create("Alternate custody", true)
	empty, _ := create("Empty custody", false)
	at := now.Add(-time.Hour)
	note := "Original synthetic custody"
	original, err := historyapi.NewService(seed).RecordChange(ctx, historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangePositionTransfer, FromHoldingID: ids.holding.String(), ToAccountID: target.String(),
		Quantity: "0.625", EffectiveAt: at.Format(time.RFC3339Nano), Note: &note,
	})
	if err != nil {
		t.Fatal(err)
	}
	originalID := domain.ActivityID(original.Activity.ID)
	originalTarget := domain.HoldingID(*original.Effects[1].HoldingID)
	if err := initialSession.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.SettingsPath()); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(paths.SettingsPath(), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ref, _ := openBankFixture(t, copyBankProfile(t, paths.RootPath()))
	app, storage := reopenBaselineProfile(t, paths)
	app.SetClock(func() time.Time { return now })
	ref.SetClock(func() time.Time { return now })
	fault := &lostCustodyCorrection{Service: app}
	page := newLedgerHistory(ctx, fault, func() time.Time { return now }, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{history: page, version: app.MobileDataVersion})
	h.size = image.Pt(640, 820)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	finish := func() {
		for range 5 {
			h.frame()
			page.model.Wait()
			page.reads.workers.Wait()
		}
	}
	click := func(text string) { t.Helper(); h.click(text, false); finish() }
	label := func(text string) { t.Helper(); assertPageLabel(t, &h.router, text) }
	positions := func(app *application.Service, from, prior, alternate string) {
		t.Helper()
		rows, err := app.HoldingsByAccounts(ctx, []domain.AccountID{ids.account, target, other})
		if err != nil {
			t.Fatal(err)
		}
		got := map[domain.HoldingID]string{}
		for _, holdings := range rows {
			for _, holding := range holdings {
				got[holding.ID] = holding.Quantity.Canonical()
			}
		}
		want := map[domain.HoldingID]string{ids.holding: from, originalTarget: prior, otherHolding: alternate}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("holdings = %v, want %v", got, want)
		}
	}

	finish()
	click("Ledger activity")
	filter := history.Filters{Instrument: ids.instrument}
	page.model.SetFilters(filter)
	finish()
	click("View")
	if page.reads.detail.id != originalID {
		t.Fatal("did not open the exact custody transaction")
	}
	click("Correct transfer")
	form := page.reads.detail.amendment
	if form.custody.from != ids.holding || form.custody.to != originalTarget ||
		form.amount.Text() != "0.625" || form.custody.note.Text() != note {
		t.Fatal("correction did not restore the stored transfer")
	}
	click("Cancel")
	h.focused(&page.reads.detail.correct)
	positions(app, "1.375", "0.625", "0")

	click("Correct transfer")
	form = page.reads.detail.amendment
	h.click("Destination account", true)
	finish()
	if findPageLabel(&h.router, "Empty custody") != nil || findPageLabel(&h.router, "Harbor Brokerage") != nil {
		t.Fatal("destination can only be an existing holding of the same instrument in another account")
	}
	click("Alternate custody")
	if form.custody.to != otherHolding {
		t.Fatal("destination selection lost existing holding identity")
	}
	setBankField(h, "Quantity to transfer", &form.amount, "2.00000001")
	click("Preview correction")
	label("This holding did not have enough quantity at the effective time.")
	correctedNote := "Corrected synthetic custody"
	setBankField(h, "Quantity to transfer", &form.amount, "0.81234567")
	setBankField(h, "Notes", &form.custody.note, correctedNote)
	click("Preview correction")
	label("Harbor Brokerage · Quantity at original time · 2 → 1.18765433")
	label("Alternate custody · Quantity at original time · 0 → 0.81234567")
	h.focused(&form.confirm)
	for _, s := range []*application.Service{app, ref} {
		_, err := historyapi.NewService(s).RecordChange(ctx, historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeMoneyAdded, AccountID: empty.String(), Currency: "USD", Amount: "1", Reason: "contribution",
			EffectiveAt: now.Add(-time.Minute).Format(time.RFC3339Nano),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	h.press(key.NameReturn)
	finish()
	label("The ledger changed. Preview this action again.")
	if form.review != nil || form.amount.Text() != "0.81234567" || form.custody.to != otherHolding || form.custody.note.Text() != correctedNote {
		t.Fatal("stale original review did not preserve the edited transfer")
	}
	positions(app, "1.375", "0.625", "0")
	click("Preview correction")
	in := historyapi.ChangeCommandRequest{Kind: historyapi.ChangePositionTransfer, FromHoldingID: ids.holding.String(),
		ToHoldingID: otherHolding.String(), Quantity: "0.81234567", Note: &correctedNote}
	api := historyapi.NewService(ref)
	want, err := api.PreviewFixChange(ctx, originalID.String(), in)
	got := wire.FromReviewedChangePreview(form.review.Preview)
	if err != nil || !reflect.DeepEqual(got.Review, want.Review) || !reflect.DeepEqual(got.Resulting, want.Resulting) {
		t.Fatalf("Gio differs from Wails preview: %+v / %+v, %v", got, want, err)
	}
	otherReceipt, err := api.FixChange(ctx, originalID.String(), in)
	if err != nil {
		t.Fatal(err)
	}
	fault.loseNext = true
	h.press(key.NameReturn)
	finish()
	positions(app, "1.18765433", "0", "0.81234567")
	label("Posting could not be confirmed. Retry the same submission to check its result.")
	h.focused(&form.confirm)
	h.press(key.NameReturn)
	finish()
	saved := page.reads.detail.record
	if len(fault.attempts) != 3 || !reflect.DeepEqual(fault.attempts[1], fault.attempts[2]) ||
		saved.Activity.CorrectionGroupID == nil || !saved.Activity.EffectiveAt.Equal(at) || *saved.Activity.Note != correctedNote ||
		saved.Links.Original != originalID || saved.Links.CorrectionReversal == "" {
		t.Fatalf("missing immutable correction or same-intent retry: %+v", saved)
	}
	label("Correction saved.")
	click("Back to original activity")
	h.focused(&page.reads.detail.linked[3])
	if page.reads.detail.record.Amendable {
		t.Fatal("an already corrected transfer can still be amended")
	}
	h.press(key.NameReturn)
	finish()
	positions(ref, "1.18765433", "0", "0.81234567")
	now = now.Add(time.Minute)
	click("Reverse activity")
	form = page.reads.detail.amendment
	label("Harbor Brokerage · Quantity after reversal · 2")
	label("Alternate custody · Quantity after reversal · 0")
	click("Cancel")
	h.focused(&page.reads.detail.reverse)
	click("Reverse activity")
	h.focused(&page.reads.detail.amendment.confirm)
	h.press(key.NameReturn)
	finish()
	reversal := page.reads.detail.record
	if reversal.Links.Original != saved.Activity.ID || !reversal.Activity.EffectiveAt.Equal(now) ||
		len(reversal.Activity.Effects) != 2 || *reversal.Activity.Effects[0].HoldingID != otherHolding ||
		reversal.Activity.Effects[0].Direction != domain.EffectRemoved || *reversal.Activity.Effects[1].HoldingID != ids.holding {
		t.Fatalf("reversal did not preserve both reviewed legs: %+v", reversal)
	}
	label("Alternate custody · Quantity after reversal · 0")
	_, err = api.UndoChange(ctx, otherReceipt.Activity.ID)
	if err != nil {
		t.Fatal(err)
	}
	click("Back to original activity")
	h.focused(&page.reads.detail.linked[2])
	click("Back to ledger activity")
	if !page.model.Filters().Equal(filter) {
		t.Fatal("returning to Ledger lost its filter")
	}
	positions(ref, "2", "0", "0")
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	reopened.SetClock(func() time.Time { return now })
	positions(reopened, "2", "0", "0")
	persisted, err := history.LoadDetail(ctx, reopened, originalID)
	if err != nil || persisted.Links.Replacement != saved.Activity.ID || persisted.Links.Reversal != saved.Links.CorrectionReversal {
		t.Fatalf("reopened audit lost its correction pair: %+v / %v", persisted.Links, err)
	}
	assertNativeTradeCash(t, reopened, ids.account, map[domain.CurrencyCode]string{"USD": "156"})
}
