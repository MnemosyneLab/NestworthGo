package desktop

import (
	"image"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

const investmentAmendmentHousehold = "Synthetic PR302 investment amendments"

type investmentAmendmentTarget struct {
	accountName string
	account     domain.AccountID
	activity    wire.ActivityDTO
	request     historyapi.ChangeCommandRequest
	holding     domain.HoldingID
}

type investmentAmendmentFixture struct {
	seed                   []byte
	buy, sell, dividend    investmentAmendmentTarget
	dependent              investmentAmendmentTarget
	holdingFee, holdingTax wire.ActivityDTO
}

func seedInvestmentAmendments(t *testing.T, asOf time.Time) investmentAmendmentFixture {
	t.Helper()
	ctx := t.Context()
	origin := asOf.UTC().Truncate(24*time.Hour).AddDate(0, 0, -5).Add(time.Hour)
	now := origin
	db, _, svc := openStage6Database(t, filepath.Join(t.TempDir(), "investments.db"), func() time.Time { return now },
		application.NewMarketDataRegistry(&unavailableOverviewProvider{}))
	if err := svc.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: investmentAmendmentHousehold, BaseCurrency: "EUR", MemberNames: []string{"Sample owner"},
	}); err != nil {
		t.Fatal(err)
	}
	boot, err := svc.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	record := func(request historyapi.ChangeCommandRequest, dayOffset, hour int, note string) wire.ActivityDTO {
		t.Helper()
		request.Note = &note
		request.EffectiveAt = origin.AddDate(0, 0, dayOffset).Add(time.Duration(hour) * time.Hour).Format(time.RFC3339Nano)
		receipt, err := historyapi.NewService(svc).RecordChange(ctx, request)
		if err != nil {
			t.Fatalf("seed %q: %v", note, err)
		}
		return receipt.Activity
	}
	type scope struct {
		name       string
		account    domain.AccountID
		instrument domain.InstrumentID
	}
	newScope := func(name, symbol string) scope {
		t.Helper()
		account, err := svc.CreateAccount(ctx, application.AccountInput{
			Name: name, AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
			DefaultCurrency: "EUR", IncludeInNetWorth: true, IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		instrument, err := svc.CreateInstrument(ctx, application.InstrumentInput{
			Name: name + " fund", Symbol: symbol, Type: "etf", QuoteCurrency: "EUR", QuoteSource: "manual",
		})
		if err != nil {
			t.Fatal(err)
		}
		return scope{name: name, account: account.Account.ID, instrument: instrument.ID}
	}
	buy := newScope("Sample Buy Desk", "PRBUY")
	sell := newScope("Sample Sell Desk", "PRSELL")
	dividend := newScope("Sample Dividend Desk", "PRDIV")
	dependent := newScope("Sample Dependent Desk", "PRDEP")
	now = asOf
	for _, target := range []scope{buy, sell, dividend, dependent} {
		record(historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeMoneyAdded, AccountID: target.account.String(), Amount: "1000", Currency: "EUR", Reason: "contribution",
		}, 1, 1, target.name+" opening cash")
	}
	record(historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeMoneyAdded, AccountID: dividend.account.String(), Amount: "30", Currency: "USD", Reason: "contribution",
	}, 1, 2, "Dividend USD opening cash")
	trade := func(target scope, holding domain.HoldingID, side, quantity, gross, fee string, dayOffset, hour int, note string) investmentAmendmentTarget {
		t.Helper()
		request := historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeTrade, Side: side, SettlementAccountID: target.account.String(), InstrumentID: target.instrument.String(),
			HoldingID: holding.String(), Quantity: quantity, Gross: gross, GrossCurrency: "EUR", Fee: fee,
		}
		if fee != "" {
			request.FeeCurrency = "EUR"
		}
		activity := record(request, dayOffset, hour, note)
		holding, err := domain.ParseHoldingID(activity.TradeDetail.HoldingID)
		if err != nil {
			t.Fatal(err)
		}
		request.HoldingID = holding.String()
		return investmentAmendmentTarget{target.name, target.account, activity, request, holding}
	}
	const buyNote = "Correct or reverse this buy"
	const sellNote = "Correct or reverse this sell"
	const dividendNote = "Correct or reverse this cash dividend"
	fixture := investmentAmendmentFixture{}
	fixture.buy = trade(buy, "", "buy", "8.25", "82.5", "0.1255", 2, 1, buyNote)
	initialSellPosition := trade(sell, "", "buy", "20", "200", "", 2, 2, "Sell desk opening position")
	fixture.sell = trade(sell, initialSellPosition.holding, "sell", "4.5", "90.25", "0.3333", 3, 2, sellNote)
	initialDividendPosition := trade(dividend, "", "buy", "10", "100", "", 2, 3, "Dividend desk opening position")
	dividendRequest := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeCashDividend, HoldingID: initialDividendPosition.holding.String(), Amount: "12.3456", Currency: "USD",
	}
	fixture.dividend = investmentAmendmentTarget{
		dividend.name, dividend.account, record(dividendRequest, 3, 3, dividendNote), dividendRequest, initialDividendPosition.holding,
	}
	fixture.dependent = trade(dependent, "", "buy", "10", "100", "0.25", 2, 4, "Buy blocked by later sale")
	trade(dependent, fixture.dependent.holding, "sell", "7", "91", "", 3, 4, "Later sale depends on buy")
	holdingExpense := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeMoneyRemoved, AccountID: dependent.account.String(), HoldingID: fixture.dependent.holding.String(),
		Amount: "1.125", Currency: "EUR", Reason: "fee",
	}
	fixture.holdingFee = record(holdingExpense, 3, 5, "Holding-associated fee control")
	holdingExpense.Amount, holdingExpense.Reason = "0.25", "tax"
	fixture.holdingTax = record(holdingExpense, 3, 6, "Holding-associated tax control")
	fixture.seed = stage6SeedBytes(t, db)
	return fixture
}

func TestExportInvestmentAmendmentFixtures(t *testing.T) {
	output := os.Getenv("NESTWORTH_INVESTMENT_AMENDMENT_QA_DIR")
	if output == "" {
		t.Skip("set NESTWORTH_INVESTMENT_AMENDMENT_QA_DIR to an existing disposable directory to export native QA fixtures")
	}
	root, err := os.MkdirTemp(output, "investment-amendments-")
	if err != nil {
		t.Fatal(err)
	}
	fixture := seedInvestmentAmendments(t, time.Now())
	for _, profile := range []string{"source", "gio-correct", "wails-correct", "gio-reverse", "wails-reverse"} {
		writeStage6Fixture(t, filepath.Join(root, profile), fixture.seed)
	}
	t.Logf("fixtures=%s household=%q dates=%s..%s", root, investmentAmendmentHousehold,
		fixture.buy.activity.EffectiveLocalDate, fixture.dividend.activity.EffectiveLocalDate)
}

func TestHistoryInvestmentAmendmentsMatchWails(t *testing.T) {
	now := time.Date(2026, time.November, 28, 11, 13, 17, 123000000, time.UTC)
	clock := func() time.Time { return now }
	fixture := seedInvestmentAmendments(t, now)
	type projection struct {
		cash     map[domain.CurrencyCode]string
		quantity string
	}
	cases := []struct {
		title                 string
		target                investmentAmendmentTarget
		kind                  domain.ActivityKind
		quantity, amount, fee string
		before, fixed, undone projection
	}{
		{
			title: "Correct buy", target: fixture.buy, kind: domain.ActivityBuy, quantity: "8.5", amount: "85.125", fee: "0.25",
			before: projection{map[domain.CurrencyCode]string{"EUR": "917.3745"}, "8.25"},
			fixed:  projection{map[domain.CurrencyCode]string{"EUR": "914.625"}, "8.5"},
			undone: projection{map[domain.CurrencyCode]string{"EUR": "1000"}, "0"},
		},
		{
			title: "Correct sell", target: fixture.sell, kind: domain.ActivitySell, quantity: "5.25", amount: "93.5", fee: "0.375",
			before: projection{map[domain.CurrencyCode]string{"EUR": "889.9167"}, "15.5"},
			fixed:  projection{map[domain.CurrencyCode]string{"EUR": "893.125"}, "14.75"},
			undone: projection{map[domain.CurrencyCode]string{"EUR": "800"}, "20"},
		},
		{
			title: "Correct dividend", target: fixture.dividend, kind: domain.ActivityCashDividend, amount: "13.4567",
			before: projection{map[domain.CurrencyCode]string{"EUR": "900", "USD": "42.3456"}, "10"},
			fixed:  projection{map[domain.CurrencyCode]string{"EUR": "900", "USD": "43.4567"}, "10"},
			undone: projection{map[domain.CurrencyCode]string{"EUR": "900", "USD": "30"}, "10"},
		},
	}
	for _, action := range []history.AmendmentAction{history.CorrectActivity, history.ReverseActivity} {
		t.Run(string(action), func(t *testing.T) {
			ctx := t.Context()
			root := t.TempDir()
			gioPath := writeStage6Fixture(t, filepath.Join(root, "gio"), fixture.seed)
			wailsPath := writeStage6Fixture(t, filepath.Join(root, "wails"), fixture.seed)
			db, _, service := openStage6Database(t, gioPath, clock)
			_, _, reference := openStage6Database(t, wailsPath, clock)
			wails := historyapi.NewService(reference)
			page, h, finish, click := investmentAmendmentHarness(t, service, clock)
			count := func(app *application.Service) int {
				t.Helper()
				rows, err := app.ListActivities(ctx, 50)
				if err != nil {
					t.Fatal(err)
				}
				return len(rows)
			}
			assertProjection := func(t *testing.T, app *application.Service, target investmentAmendmentTarget, want projection) {
				t.Helper()
				assertNativeTradeCash(t, app, target.account, want.cash)
				rows, err := app.HoldingsByAccounts(ctx, []domain.AccountID{target.account})
				if err != nil {
					t.Fatal(err)
				}
				if held := rows[target.account]; len(held) != 1 || held[0].ID != target.holding || held[0].Quantity.Canonical() != want.quantity {
					t.Fatalf("holdings for %s = %+v, want %s", target.accountName, held, want.quantity)
				}
			}
			assertAudit := func(t *testing.T, app *application.Service, original, saved domain.ActivityID) {
				t.Helper()
				from, err := history.LoadDetail(ctx, app, original)
				if err != nil {
					t.Fatal(err)
				}
				to, err := history.LoadDetail(ctx, app, saved)
				if err != nil {
					t.Fatal(err)
				}
				if to.Links.Original != original || from.Amendable {
					t.Fatalf("posted activity does not link to protected original: %+v / %+v", from.Links, to.Links)
				}
				if action == history.CorrectActivity {
					if from.Links.Replacement != saved || to.Links.CorrectionReversal == "" || from.Links.Reversal != to.Links.CorrectionReversal ||
						!to.Activity.EffectiveAt.Equal(from.Activity.EffectiveAt) || !to.Activity.CreatedAt.Equal(now) {
						t.Fatalf("correction links or effective instant differ: %+v / %+v", from, to)
					}
				} else if from.Links.Reversal != saved || !to.Activity.EffectiveAt.Equal(now) {
					t.Fatalf("reversal links or current effective instant differ: %+v / %+v", from, to)
				}
			}
			savedIDs := make(map[domain.ActivityID]domain.ActivityID)
			for _, tc := range cases {
				t.Logf("%s: %s", action, tc.title)
				original := domain.ActivityID(tc.target.activity.ID)
				filter := history.Filters{Account: tc.target.account, Kind: tc.kind}
				page.model.SetFilters(filter)
				finish()
				click("View")
				if page.reads.detail.id != original {
					t.Fatalf("opened %s, want %s", page.reads.detail.id, original)
				}
				before := count(service)
				assertProjection(t, service, tc.target, tc.before)
				title, confirm, mutationRows := "Reverse activity", "Confirm reversal", 1
				want := tc.undone
				var expected wire.ChangePreviewDTO
				var err error
				if action == history.CorrectActivity {
					title, confirm, mutationRows = tc.title, "Confirm correction", 2
					want = tc.fixed
				}
				click(title)
				if form := page.reads.detail.amendment; form == nil || form.investment == nil {
					t.Fatal("investment amendment form did not open")
				}
				replacement := tc.target.request
				if action == history.CorrectActivity {
					form := page.reads.detail.amendment
					note := "Verified investment correction"
					form.investment.note.Editor.SetText(note)
					form.amount.Editor.SetText(tc.amount)
					replacement.Note = &note
					replacement.EffectiveAt = tc.target.activity.EffectiveAt
					if tc.kind == domain.ActivityCashDividend {
						replacement.Amount = tc.amount
					} else {
						form.investment.quantity.Editor.SetText(tc.quantity)
						form.fee.Editor.SetText(tc.fee)
						replacement.Quantity, replacement.Gross, replacement.Fee = tc.quantity, tc.amount, tc.fee
					}
					click("Preview correction")
					expected, err = wails.PreviewFixChange(ctx, original.String(), replacement)
					if err != nil {
						t.Fatal(err)
					}
				}
				form := page.reads.detail.amendment
				if form.review == nil {
					t.Fatalf("amendment preview missing (problem %v)", form.problem)
				}
				review := *form.review
				preview := wire.FromReviewedChangePreview(review.Preview)
				if action == history.CorrectActivity {
					assertInvestmentAmendmentParity(t, preview, expected)
				} else {
					click("Cancel")
					if page.reads.detail.amendment != nil || count(service) != before {
						t.Fatal("cancel did not discard the reversal review")
					}
					click(title)
					review = *page.reads.detail.amendment.review
					preview = wire.FromReviewedChangePreview(review.Preview)
				}
				if count(service) != before {
					t.Fatal("review wrote an audit row")
				}
				assertProjection(t, service, tc.target, tc.before)
				click(confirm)
				saved := page.reads.detail.record
				if saved == nil || saved.Activity.ID == original {
					t.Fatal("the confirmed activity did not open")
				}
				if action == history.CorrectActivity {
					expected, err = wails.FixChange(ctx, original.String(), replacement)
				} else {
					expected, err = wails.UndoChange(ctx, original.String())
				}
				if err != nil {
					t.Fatal(err)
				}
				persistedWailsActivity, err := wails.Activity(ctx, expected.Activity.ID)
				if err != nil {
					t.Fatal(err)
				}
				persistedGioActivity := wire.FromActivity(saved.Activity)
				assertInvestmentAmendmentParity(t,
					wire.ChangePreviewDTO{Activity: persistedGioActivity, Effects: persistedGioActivity.Effects, Resulting: persistedGioActivity.Resulting},
					wire.ChangePreviewDTO{Activity: persistedWailsActivity, Effects: persistedWailsActivity.Effects, Resulting: persistedWailsActivity.Resulting},
				)
				if action == history.ReverseActivity && !reflect.DeepEqual(preview.Resulting, expected.Resulting) {
					t.Fatalf("reversal result differs from committed Wails effect: %+v / %+v", preview.Resulting, expected.Resulting)
				}
				assertProjection(t, service, tc.target, want)
				assertProjection(t, reference, tc.target, want)
				if tc.kind == domain.ActivitySell {
					cost, realized := "200", "0"
					if action == history.CorrectActivity {
						cost, realized = "147.5", "40.625"
					}
					for _, app := range []*application.Service{service, reference} {
						gain, err := app.HoldingGain(ctx, tc.target.holding)
						if err != nil || gain.TotalCost == nil || gain.TotalCost.Amount != cost || gain.TotalCost.Currency != "EUR" ||
							gain.RealizedGain == nil || gain.RealizedGain.Amount != realized || gain.RealizedGain.Currency != "EUR" {
							t.Fatalf("sale cost and gain = %+v / %+v (%v), want %s / %s EUR", gain.TotalCost, gain.RealizedGain, err, cost, realized)
						}
					}
				}
				assertAudit(t, service, original, saved.Activity.ID)
				assertAudit(t, reference, original, domain.ActivityID(expected.Activity.ID))
				retry, err := review.Commit(ctx, service)
				if err != nil || retry.Activity.ID != saved.Activity.ID || count(service) != before+mutationRows || count(reference) != count(service) {
					t.Fatalf("retry did not return the same single write: %+v (%v)", retry.Activity, err)
				}
				savedIDs[original] = saved.Activity.ID
				click("Back to original activity")
				if prior := page.reads.detail; prior == nil || prior.id != original {
					t.Fatal("receipt did not return to the original activity")
				} else if prior.contextual {
					h.press(key.NameEscape)
					finish()
				} else {
					click("Back to ledger activity")
				}
				if page.reads.detail != nil || !page.model.Filters().Equal(filter) {
					t.Fatalf("amendment lost Ledger scope: %+v", page.model.Filters())
				}
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			_, _, reopened := openStage6Database(t, gioPath, clock)
			for _, tc := range cases {
				original := domain.ActivityID(tc.target.activity.ID)
				want := tc.undone
				if action == history.CorrectActivity {
					want = tc.fixed
				}
				assertAudit(t, reopened, original, savedIDs[original])
				assertProjection(t, reopened, tc.target, want)
			}
		})
	}
}

func investmentAmendmentHarness(t *testing.T, service *application.Service, clock func() time.Time) (*historyPage, *previewHarness, func(), func(string)) {
	t.Helper()
	page := newLedgerHistory(t.Context(), service, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{history: page, version: service.MobileDataVersion})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	wait := func() {
		page.model.Wait()
		page.reads.workers.Wait()
	}
	t.Cleanup(wait)
	finish := func() {
		for range 4 {
			h.frame()
			wait()
		}
	}
	click := func(label string) {
		t.Helper()
		h.click(label, false)
		finish()
	}
	finish()
	click("Ledger activity")
	return page, h, finish, click
}

func assertInvestmentAmendmentParity(t *testing.T, got, want wire.ChangePreviewDTO) {
	t.Helper()
	a, b := got.Activity, want.Activity
	if a.Kind != b.Kind || a.Reason != b.Reason || a.EffectiveAt != b.EffectiveAt || !reflect.DeepEqual(a.Note, b.Note) ||
		!reflect.DeepEqual(a.TradeDetail, b.TradeDetail) || !reflect.DeepEqual(a.DividendDetail, b.DividendDetail) ||
		!reflect.DeepEqual(got.Review, want.Review) || !reflect.DeepEqual(got.Resulting, want.Resulting) ||
		len(got.Effects) != len(want.Effects) {
		t.Fatalf("amendment differs from Wails: %+v / %+v", got, want)
	}
	for i, effect := range got.Effects {
		expected := want.Effects[i]
		effect.ID, effect.ActivityID = "", ""
		expected.ID, expected.ActivityID = "", ""
		if !reflect.DeepEqual(effect, expected) {
			t.Fatalf("effect differs from Wails: %+v / %+v", effect, expected)
		}
	}
}

func TestHistoryInvestmentAmendmentsRejectDependentAndProtectLinkedExpenses(t *testing.T) {
	now := time.Date(2026, time.November, 28, 11, 13, 17, 123000000, time.UTC)
	clock := func() time.Time { return now }
	ctx := t.Context()
	fixture := seedInvestmentAmendments(t, now)
	path := writeStage6Fixture(t, filepath.Join(t.TempDir(), "gio"), fixture.seed)
	_, _, service := openStage6Database(t, path, clock)
	page, h, finish, click := investmentAmendmentHarness(t, service, clock)
	activities := func() []domain.Activity {
		t.Helper()
		rows, err := service.ListActivities(ctx, 50)
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	before := activities()
	page.model.SetFilters(history.Filters{Account: fixture.dependent.account, Kind: domain.ActivityBuy})
	finish()
	click("View")
	original := fixture.dependent.activity.ID
	if page.reads.detail.id.String() != original {
		t.Fatal("did not open the acquisition used by the later sale")
	}
	click("Reverse activity")
	form := page.reads.detail.amendment
	if form.review != nil || form.problem == i18n.NoMessage {
		t.Fatal("reversal preview must explain the blocked acquisition")
	}
	if got := findPageLabel(&h.router, "Confirm reversal"); got != nil {
		t.Fatal("blocked reversal exposed a confirm control")
	}
	click("Cancel")
	click("Correct buy")
	form = page.reads.detail.amendment
	form.investment.quantity.Editor.SetText("6")
	click("Preview correction")
	if form.review != nil || form.problem == i18n.NoMessage {
		t.Fatal("correction preview did not reject the reduction below the subsequent sale")
	}
	if got := findPageLabel(&h.router, "Confirm correction"); got != nil {
		t.Fatal("blocked correction exposed a confirm control")
	}
	click("Cancel")
	h.press(key.NameEscape)
	finish()

	wails := historyapi.NewService(service)
	if _, err := wails.UndoChange(ctx, original); err == nil {
		t.Fatal("Wails accepted a dependent reversal")
	}
	replacement := fixture.dependent.request
	replacement.Quantity, replacement.EffectiveAt = "6", fixture.dependent.activity.EffectiveAt
	if _, err := wails.PreviewFixChange(ctx, original, replacement); err == nil {
		t.Fatal("Wails accepted the historical reduction below the later sale")
	}
	for _, expense := range []wire.ActivityDTO{fixture.holdingFee, fixture.holdingTax} {
		page.reads.enterDetail(domain.ActivityID(expense.ID), historyDetailState{contextual: true})
		finish()
		detail := page.reads.detail.record
		if detail == nil || detail.Amendable || detail.Reversible {
			t.Fatalf("holding-associated %s lost its protected detail: %+v", expense.Reason, detail)
		}
		for _, label := range []string{"Correct amount", "Correct buy", "Correct sell", "Correct dividend", "Reverse activity"} {
			if findPageLabel(&h.router, label) != nil {
				t.Fatalf("holding-associated %s exposed %q", expense.Reason, label)
			}
		}
		page.reads.leaveDetail()
	}
	assertNativeTradeCash(t, service, fixture.dependent.account, map[domain.CurrencyCode]string{"EUR": "989.375"})
	rows, err := service.HoldingsByAccounts(ctx, []domain.AccountID{fixture.dependent.account})
	if err != nil {
		t.Fatal(err)
	}
	if holdings := rows[fixture.dependent.account]; len(holdings) != 1 || holdings[0].Quantity.Canonical() != "3" {
		t.Fatalf("blocked changes perturbed the position: %+v", holdings)
	}
	if got := activities(); !reflect.DeepEqual(got, before) {
		t.Fatal("blocked activities wrote to the Ledger")
	}
}
