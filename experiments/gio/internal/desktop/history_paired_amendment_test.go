package desktop

import (
	"errors"
	"image"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/holding"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestHistoryPairedCashAmendmentsMatchWails(t *testing.T) {
	ctx := t.Context()
	root := createFixtureRoot(t)
	seed, seedSession := openBankFixture(t, root)
	bank, reserve, studio, ids := seedPairedAmendments(t, seed)
	if err := seedSession.Close(); err != nil {
		t.Fatal(err)
	}
	ref, _ := openBankFixture(t, copyBankProfile(t, root))
	service, session := openBankFixture(t, root)
	adapter := historyapi.NewService(ref)
	fault := &amendmentFault{Service: service}
	page := newLedgerHistory(ctx, fault, func() time.Time { return accountTestClock }, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{history: page})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	finish := func() {
		h.frame()
		page.reads.workers.Wait()
		page.model.Wait()
		h.frame()
		page.reads.workers.Wait()
		h.frame()
	}
	click := func(label string) { h.click(label, false); finish() }
	label := func(text string) { assertPageLabel(t, &h.router, text) }
	read := func(app *application.Service, id domain.ActivityID) domain.Activity {
		t.Helper()
		got, err := app.Activity(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	compareRecord := func(id, other domain.ActivityID) {
		t.Helper()
		normalize := func(activity domain.Activity) wire.ActivityDTO {
			activity.ID, activity.CorrectionGroupID, activity.ReversesActivityID = "", nil, nil
			for i := range activity.Effects {
				activity.Effects[i].ID, activity.Effects[i].ActivityID = "", ""
			}
			return wire.FromActivity(activity)
		}
		got, want := normalize(read(service, id)), normalize(read(ref, other))
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("activity differs from Wails:\n%+v\n%+v", got, want)
		}
	}
	compareFix := func(id domain.ActivityID, preview domain.ChangePreview, saved domain.ActivityID, req historyapi.ChangeCommandRequest) domain.ActivityID {
		t.Helper()
		got := wire.FromReviewedChangePreview(preview)
		want, err := adapter.PreviewFixChange(ctx, id.String(), req)
		if err != nil || !reflect.DeepEqual(got.Review, want.Review) || !reflect.DeepEqual(got.Resulting, want.Resulting) || !reflect.DeepEqual(got.DerivedRate, want.DerivedRate) {
			t.Fatalf("review differs from Wails: %+v / %+v, %v", got, want, err)
		}
		other, err := adapter.FixChange(ctx, id.String(), req)
		if err != nil {
			t.Fatal(err)
		}
		otherID := domain.ActivityID(other.Activity.ID)
		compareRecord(saved, otherID)
		var inverses [2]domain.ActivityID
		for i, side := range []struct {
			app *application.Service
			id  domain.ActivityID
		}{{service, saved}, {ref, otherID}} {
			got, err := history.LoadDetail(ctx, side.app, id)
			if err != nil || got.Amendable || got.Links.Reversal == "" || got.Links.Replacement != side.id {
				t.Fatalf("original links: %+v, %v", got.Links, err)
			}
			inverse := read(side.app, got.Links.Reversal)
			inverses[i] = inverse.ID
			if inverse.ReversesActivityID == nil || *inverse.ReversesActivityID != id || !inverse.EffectiveAt.Equal(read(side.app, id).EffectiveAt) {
				t.Fatalf("correction reversal failed to identify the historical original: %+v", inverse)
			}
		}
		compareRecord(inverses[0], inverses[1])
		return otherID
	}
	compareUndo := func(parent, otherParent, saved domain.ActivityID) {
		t.Helper()
		other, err := adapter.UndoChange(ctx, otherParent.String())
		if err != nil {
			t.Fatal(err)
		}
		compareRecord(saved, domain.ActivityID(other.Activity.ID))
		got, err := history.LoadDetail(ctx, service, saved)
		if err != nil || got.Amendable || got.Links.Original != parent || got.Links.Replacement != "" {
			t.Fatalf("standalone reversal links: %+v, %v", got.Links, err)
		}
	}
	readCash := func(app *application.Service, wantUSD, wantEUR string) {
		t.Helper()
		values, err := holding.NewService(app).ListAccountCashValues(ctx, studio.String())
		if err != nil {
			t.Fatal(err)
		}
		latest := map[string]string{}
		for _, value := range values {
			if _, seen := latest[value.Amount.Currency]; !seen {
				latest[value.Amount.Currency] = value.Amount.Amount
			}
		}
		if latest["USD"] != wantUSD || latest["EUR"] != wantEUR {
			t.Fatalf("native balances %+v, want USD %s and EUR %s", latest, wantUSD, wantEUR)
		}
	}
	normal := history.AmendmentAmounts{}
	changed := history.AmendmentAmounts{Amount: "62.5", Received: "49.25", Fee: "1.5"}
	cross := historyapi.ChangeCommandRequest{Kind: historyapi.ChangeCashTransfer, FromAccountID: bank.String(), ToAccountID: studio.String(),
		Sent: changed.Amount, SentCurrency: "USD", Received: changed.Received, ReceivedCurrency: "EUR", Fee: changed.Fee, FeeCurrency: "USD"}
	h.frame()
	click("Ledger activity")
	h.click("Activity type", true)
	click("Cash transfer")
	click("View")
	if page.reads.detail.id != ids[1] {
		t.Fatal("expected the cross-currency transfer")
	}
	click("Correct amount")
	form := page.reads.detail.amendment
	setBankField(h, "Sent amount · USD", &form.amount, changed.Amount)
	setBankField(h, "Received amount · EUR", &form.received, "0")
	click("Preview correction")
	label("Enter the received amount as a positive value with up to four decimal places.")
	if len(fault.fix) != 0 {
		t.Fatal("invalid received amount called the writer")
	}
	setBankField(h, "Received amount · EUR", &form.received, changed.Received)
	setBankField(h, "Fee (optional) · USD", &form.fee, changed.Fee)
	click("Preview correction")
	label("Sent from Harbor Checking · 62.5 USD")
	label("Received by Cash Studio · 49.25 EUR")
	label("Fee paid by Harbor Checking · 1.5 USD")
	preview := form.review.Preview
	click("Confirm correction")
	label("Correction saved. Historical snapshots could not refresh.")
	click("Retry")
	label("Replacement record")
	saved := page.reads.detail.id
	refSaved := compareFix(ids[1], preview, saved, cross)
	if len(fault.fix) != 1 || readBankRecord(t, service, bank).LatestValue.Amount.CanonicalAmount() != "1659.13" {
		t.Fatal("read Retry changed write count or caused a wrong source projection")
	}
	readCash(service, "78.75", "151.5")
	readCash(ref, "78.75", "151.5")
	for _, app := range []*application.Service{service, ref} {
		assertAmendmentAttribution(t, app, "2026-11-21", "12.3", "-1.6", "-3.25")
	}
	click("Reverse activity")
	label("Cash Studio · Removed 49.25 EUR")
	label("Harbor Checking · Added 1.5 USD · Fee reversed")
	click("Confirm reversal")
	label("Posting could not be confirmed. Retry the same submission to check its result.")
	click("Retry same submission")
	undoID := page.reads.detail.id
	label("Harbor Checking balance after · 1723.13 USD")
	label("Cash Studio balance after · 102.25 EUR")
	compareUndo(saved, refSaved, undoID)
	if len(fault.undo) != 2 || !reflect.DeepEqual(fault.undo[0], fault.undo[1]) {
		t.Fatal("lost reversal reply did not replay the original intent")
	}
	click("View original")
	label("This record has been reversed.")
	label("View correction reversal")
	label("View reversal")
	if findPageLabel(&h.router, "Correct amount") != nil {
		t.Fatal("a terminal replacement offered correction again")
	}
	assertCode := func(err error, code domain.ErrorCode) {
		t.Helper()
		var issue *domain.Error
		if !errors.As(err, &issue) || issue.Code != code {
			t.Fatalf("got %v, want %s", err, code)
		}
	}
	_, err := history.ReviewAmendment(ctx, service, history.ReverseCash, saved, normal, accountTestClock)
	assertCode(err, domain.ErrAlreadyUndone)
	_, err = history.ReviewAmendment(ctx, service, history.CorrectCash, ids[0], history.AmendmentAmounts{Amount: "10000"}, accountTestClock)
	assertCode(err, domain.ErrInsufficientBalance)
	_, err = history.ReviewAmendment(ctx, service, history.CorrectCash, ids[0], history.AmendmentAmounts{Amount: "10.5"}, accountTestClock)
	assertCode(err, domain.ErrNoChange)
	retained, err := history.ReviewAmendment(ctx, service, history.CorrectCash, ids[0], history.AmendmentAmounts{Amount: "11.25"}, accountTestClock)
	if err != nil || len(retained.Preview.Effects) != 3 || retained.Preview.Effects[2].Money.CanonicalAmount() != "0.5" {
		t.Fatalf("blank transfer fee failed to retain the original: %+v, %v", retained.Preview.Effects, err)
	}

	// Compare the remaining types through the same guarded entry points. Wails
	// uses the original effective instant when its Fix request has no date.
	cases := []struct {
		id      domain.ActivityID
		amounts history.AmendmentAmounts
		request historyapi.ChangeCommandRequest
	}{
		{ids[0], history.AmendmentAmounts{Amount: "11.25", Fee: "0"}, historyapi.ChangeCommandRequest{Kind: historyapi.ChangeCashTransfer,
			FromAccountID: bank.String(), ToAccountID: reserve.String(), Sent: "11.25", SentCurrency: "USD", Received: "11.25", ReceivedCurrency: "USD", Fee: "0", FeeCurrency: "USD"}},
		{ids[2], history.AmendmentAmounts{Amount: "20.125"}, historyapi.ChangeCommandRequest{Kind: historyapi.ChangeMoneyAdded,
			AccountID: studio.String(), Amount: "20.125", Currency: "EUR", Reason: "contribution"}},
		{ids[3], history.AmendmentAmounts{Amount: "6.25"}, historyapi.ChangeCommandRequest{Kind: historyapi.ChangeMoneyRemoved,
			AccountID: studio.String(), Amount: "6.25", Currency: "EUR", Reason: "other"}},
		{ids[4], history.AmendmentAmounts{Amount: "50", Received: "41", Fee: "1.75"}, historyapi.ChangeCommandRequest{Kind: historyapi.ChangeFXConversion,
			AccountID: studio.String(), Sold: "50", SoldCurrency: "USD", Bought: "41", BoughtCurrency: "EUR", Fee: "1.75", FeeCurrency: "USD"}},
	}
	for _, tc := range cases {
		archivedDestination := tc.id == ids[0]
		if archivedDestination {
			for _, app := range []*application.Service{service, ref} {
				if err := app.ArchiveAccount(ctx, reserve, true); err != nil {
					t.Fatal(err)
				}
			}
			detail, err := history.LoadDetail(ctx, service, tc.id)
			if err != nil || !detail.Amendable || detail.Reversible {
				t.Fatalf("archived endpoint action state: %+v, %v", detail, err)
			}
		}
		review, err := history.ReviewAmendment(ctx, service, history.CorrectCash, tc.id, tc.amounts, accountTestClock)
		if err != nil {
			t.Fatalf("%s correction preview: %v", tc.request.Kind, err)
		}
		result, err := review.Commit(ctx, service)
		if err != nil {
			t.Fatalf("%s correction: %v", tc.request.Kind, err)
		}
		otherID := compareFix(tc.id, review.Preview, result.Activity.ID, tc.request)
		if archivedDestination {
			if len(result.Effects) != 2 {
				t.Fatalf("explicit zero retained a fee: %+v", result.Effects)
			}
			_, err := history.ReviewAmendment(ctx, service, history.ReverseCash, result.Activity.ID, normal, accountTestClock)
			assertCode(err, domain.ErrConflict)
			if _, err := adapter.UndoChange(ctx, otherID.String()); err == nil {
				t.Fatal("Wails allowed reversal into an archived endpoint")
			}
			for _, app := range []*application.Service{service, ref} {
				rows, err := app.ListAccounts(ctx, domain.AccountFilter{AccountIDs: []domain.AccountID{reserve}, IncludeArchived: true})
				if err != nil || len(rows) != 1 || rows[0].Account.ArchivedAt == nil {
					t.Fatalf("correction changed archive metadata: %+v, %v", rows, err)
				}
				if err := app.ArchiveAccount(ctx, reserve, false); err != nil {
					t.Fatal(err)
				}
			}
		}
		if tc.request.Kind == historyapi.ChangeFXConversion {
			for _, app := range []*application.Service{service, ref} {
				readCash(app, "68.25", "101")
				app.SetClock(func() time.Time { return accountTestClock.Add(48 * time.Hour) })
				assertAmendmentAttribution(t, app, "2026-11-22", "0", "-0.8", "-1.75")
				app.SetClock(func() time.Time { return accountTestClock })
			}
		}
		undo, err := history.ReviewAmendment(ctx, service, history.ReverseCash, result.Activity.ID, normal, accountTestClock)
		if err != nil {
			t.Fatalf("%s reversal preview: %v", tc.request.Kind, err)
		}
		reversed, err := undo.Commit(ctx, service)
		if err != nil {
			t.Fatalf("%s reversal: %v", tc.request.Kind, err)
		}
		compareUndo(result.Activity.ID, otherID, reversed.Activity.ID)
	}
	for _, app := range []*application.Service{service, ref} {
		readCash(app, "120", "60")
		for id, expected := range map[domain.AccountID]string{bank: "1734.13", reserve: "20"} {
			if got := readBankRecord(t, app, id).LatestValue.Amount.CanonicalAmount(); got != expected {
				t.Fatalf("reversed balance %s, want %s", got, expected)
			}
		}
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := openBankFixture(t, root)
	readCash(reopened, "120", "60")
	u := fault.undo[0]
	if again, err := reopened.RecordUndoChangeGuardedAt(ctx, u.id, u.at, u.mutation, u.hash, u.key); err != nil || again.Activity.ID != undoID {
		t.Fatalf("durable paired reversal replay returned %s, %v", again.Activity.ID, err)
	}
	_, err = reopened.RecordUndoChangeGuardedAt(ctx, u.id, u.at, u.mutation, strings.Repeat("0", 64), u.key)
	assertCode(err, domain.ErrConflict)
	if rows, err := reopened.ListActivities(ctx, 100); err != nil || len(rows) != 29 {
		t.Fatalf("reopened ledger has %d activities, want 29: %v", len(rows), err)
	}
}

func assertAmendmentAttribution(t *testing.T, app *application.Service, through, external, spread, fee string) {
	t.Helper()
	result, err := app.AssetChange(t.Context(), domain.AnalysisQuery{
		Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-11-20", To: through,
		Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true,
	})
	if err != nil || result.Status != domain.CompletenessOK {
		t.Fatalf("attribution unavailable: %+v, %v", result.AnalysisAvailability, err)
	}
	amounts := map[domain.AttributionBucket]string{}
	for _, entry := range result.Waterfall {
		if entry.Amount != nil {
			amounts[entry.Bucket] = entry.Amount.CanonicalAmount()
		}
	}
	if got := amounts[domain.BucketExternalFlow]; got != external && !(got == "" && external == "0") {
		t.Fatalf("net external flow %q, want %s", got, external)
	}
	if amounts[domain.BucketFXConversionSpread] != spread || amounts[domain.BucketFee] != fee {
		t.Fatalf("spread and fee %+v, want %s and %s", amounts, spread, fee)
	}
}

func seedPairedAmendments(t *testing.T, service *application.Service) (bank, reserve, studio domain.AccountID, ids []domain.ActivityID) {
	t.Helper()
	ctx := t.Context()
	boot, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := service.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil || len(rows) != 1 || rows[0].Account.Name != "Harbor Checking" {
		t.Fatalf("unexpected base fixture: %+v, %v", rows, err)
	}
	bank = rows[0].Account.ID
	service.SetClock(func() time.Time { return accountTestClock.Add(-96 * time.Hour) })
	create := func(name, mode, initial string) domain.AccountID {
		t.Helper()
		row, err := service.CreateAccount(ctx, application.AccountInput{Name: name, AccountType: "cash_on_hand", BalanceSheetRole: "asset",
			TrackingMode: mode, DefaultCurrency: "USD", InitialAmount: initial, OwnerIDs: []domain.MemberID{boot.Members[0].ID},
			IncludeInPortfolio: true, IncludeInNetWorth: true})
		if err != nil {
			t.Fatal(err)
		}
		return row.Account.ID
	}
	reserve, studio = create("Amendment reserve", "balance", "20"), create("Cash Studio", "holdings", "")
	if _, err := service.SetFXPreference(ctx, "EUR", "USD", "manual"); err != nil {
		t.Fatal(err)
	}
	service.SetClock(func() time.Time { return accountTestClock })
	if _, err := service.AppendManualFXQuote(ctx, "EUR", "USD", "1.2", "2026-11-18T16:00:00Z"); err != nil {
		t.Fatal(err)
	}
	adapter := historyapi.NewService(service)
	record := func(day, minute string, cmd historyapi.ChangeCommandRequest) domain.ActivityID {
		t.Helper()
		cmd.EffectiveLocalDate, cmd.EffectiveLocalTime = day, minute
		posted, err := adapter.RecordChange(ctx, cmd)
		if err != nil {
			t.Fatal(err)
		}
		return domain.ActivityID(posted.Activity.ID)
	}
	for currency, opening := range map[string]string{"USD": "120", "EUR": "60"} {
		record("2026-11-19", "01:00", historyapi.ChangeCommandRequest{Kind: historyapi.ChangeMoneyAdded,
			AccountID: studio.String(), Amount: opening, Currency: currency, Reason: "contribution"})
	}
	const day = "2026-11-20"
	ids = []domain.ActivityID{
		record(day, "01:00", historyapi.ChangeCommandRequest{Kind: historyapi.ChangeCashTransfer, FromAccountID: bank.String(), ToAccountID: reserve.String(),
			Sent: "10.5", SentCurrency: "USD", Received: "10.5", ReceivedCurrency: "USD", Fee: "0.5", FeeCurrency: "USD"}),
		record(day, "02:00", historyapi.ChangeCommandRequest{Kind: historyapi.ChangeCashTransfer, FromAccountID: bank.String(), ToAccountID: studio.String(),
			Sent: "50", SentCurrency: "USD", Received: "40", ReceivedCurrency: "EUR", Fee: "1.25", FeeCurrency: "USD"}),
		record(day, "03:00", historyapi.ChangeCommandRequest{Kind: historyapi.ChangeMoneyAdded, AccountID: studio.String(), Amount: "15.25", Currency: "EUR", Reason: "contribution"}),
		record(day, "04:00", historyapi.ChangeCommandRequest{Kind: historyapi.ChangeMoneyRemoved, AccountID: studio.String(), Amount: "5", Currency: "EUR", Reason: "other"}),
		record(day, "05:00", historyapi.ChangeCommandRequest{Kind: historyapi.ChangeFXConversion, AccountID: studio.String(),
			Sold: "40", SoldCurrency: "USD", Bought: "32", BoughtCurrency: "EUR", Fee: "1.25", FeeCurrency: "USD"}),
	}
	return
}
