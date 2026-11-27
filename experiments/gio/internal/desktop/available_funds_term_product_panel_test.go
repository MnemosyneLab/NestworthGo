package desktop

import (
	"fmt"
	"github.com/waltwang/nestworth-go/internal/testfixture"
	"image"
	"reflect"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestAvailableFundsTermProductPanelsKeepVisitAndHoldingIdentity(t *testing.T) {
	ctx := t.Context()
	app, _, _, _ := openExistingBuyFixture(t)
	testfixture.SetClock(app, linkedContractClock)
	boot, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	create := func(name, tracking, currency string) domain.AccountID {
		t.Helper()
		input := application.AccountInput{
			Name: name, AccountType: "bank_account", TrackingMode: tracking, BalanceSheetRole: "asset",
			DefaultCurrency: currency, IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID},
		}
		if tracking == "balance" {
			input.InitialAmount = "0"
		}
		account, err := app.CreateAccount(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		if tracking == "holdings" {
			if _, err := app.AppendAccountCashValue(ctx, account.Account.ID, "1800", currency, ""); err != nil {
				t.Fatal(err)
			}
		}
		return account.Account.ID
	}
	const maple, continental = "Maple Deposit Bank", "Continental Savings"
	mapleID := create(maple, "holdings", "USD")
	euroID := create(continental, "holdings", "EUR")
	create("Ineligible checking", "balance", "USD")
	panelClock := func() time.Time { return linkedContractClock().Add(time.Hour) }
	testfixture.SetClock(app, panelClock)
	activities, err := app.ListActivities(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}

	funds := newAvailableFundsPage(ctx, app, func() {})
	funds.productLedger, funds.productClock = app, panelClock
	detail := newProductDetailPage(ctx, app, func() {})
	detail.termsLedger, detail.clock = app, panelClock
	funds.productPage = detail
	h := newShellHarnessWithPages(t, ledgerPages{funds: funds, version: app.MobileDataVersion})
	h.size = image.Pt(1280, 740)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	t.Cleanup(funds.workers.Wait)
	t.Cleanup(detail.workers.Wait)
	settle := func() {
		for range 5 {
			h.frame()
			funds.workers.Wait()
			detail.workers.Wait()
		}
		h.frame()
	}
	click := func(label string) {
		t.Helper()
		h.click(label, false)
		settle()
	}
	settle()
	click("Available funds")
	click("Within 30 days")
	month := funds.state.horizonOn
	if month != fundsPresetDay(funds.localDate, 30) {
		t.Fatal("test needs a nondefault Funds horizon")
	}
	missing := fundsSourceCount(funds.current.report.Sources, month, fundsNeedsRules)
	click(fundsMissingRuleText(i18n.English, missing))
	for tries := 0; tries < 4 && funds.list.Position.First == 0; tries++ {
		funds.list.ScrollBy(1)
		settle()
	}
	visit, position := funds.state, funds.list.Position
	if visit.filter != fundsNeedsRules || position.First == 0 {
		t.Fatalf("test needs a scrolled Missing rules visit: %+v / %+v", visit, position)
	}

	checkVisit := func() {
		t.Helper()
		if h.shell.activeRoute() != fundsRoute || funds.setup != nil || funds.state != visit || funds.list.Position.First != position.First {
			t.Fatalf("product panel changed the Funds visit: %+v / %+v", funds.state, funds.list.Position)
		}
		h.focused(&funds.addProduct)
	}
	noOpenWrite := func() {
		t.Helper()
		products, err := app.ListProducts(ctx, nil, true)
		gotActivities, readErr := app.ListActivities(ctx, 100)
		if err != nil || readErr != nil || len(products) != 0 || !reflect.DeepEqual(gotActivities, activities) {
			t.Fatalf("unconfirmed product changed the ledger: %v / %v (%d contracts)", err, readErr, len(products))
		}
	}
	const add = "Add product"
	click(add)
	setup := funds.setup
	if setup == nil || setup.form != nil || len(setup.choices) != 3 || findPageLabel(&h.router, "Ineligible checking") != nil {
		t.Fatalf("incorrect eligibility or chooser state: %+v", setup)
	}
	chooser := assertPageLabel(t, &h.router, i18n.English.Text(i18n.ProductChooseAccount))
	closeButton := semanticBounds(t, &h.router, "Close", false)
	if chooser.Min.X <= h.size.X/2 || closeButton.Min.X <= chooser.Min.X {
		t.Fatalf("account chooser did not open at the right edge: %v / %v", chooser, closeButton)
	}
	h.focused(setup.accountButtons[setup.choices[0].ID])
	click(continental)
	if funds.setup != setup || setup.account.ID != euroID || setup.currency != "EUR" {
		t.Fatalf("chooser did not keep the exact EUR account: %+v", setup)
	}
	h.focused(&setup.form.currency.Trigger)
	h.press(key.NameEscape)
	settle()
	checkVisit()
	noOpenWrite()

	const termName = "Panel QA term deposit"
	openDraft := func() {
		t.Helper()
		click(add)
		click(maple)
		d := funds.setup
		if d == nil || d.account.ID != mapleID || d.currency != "USD" {
			t.Fatalf("setup did not preserve the selected USD account: %+v", d)
		}
		h.focused(&d.form.currency.Trigger)
		d.form.principal.Editor.SetText("50.125")
		d.form.terms.name.Editor.SetText(termName)
		d.form.terms.terms.StartOn = funds.localDate
		d.form.terms.terms.MaturityOn = productOptional("2026-12-15")
		d.form.time.day, d.form.time.minute = funds.localDate, "01:30"
		d.form.terms.changeDepositMode(string(domain.InterestManualMaturityAmount))
		d.form.terms.deposit.maturityInterest.Editor.SetText("5.25")
		d.form.terms.fee.Editor.SetText("0.125")
		settle()
		noOpenWrite()
		click("Preview")
		if d.plan == nil || d.plan.command.Open == nil || d.plan.command.Open.AccountID != mapleID.String() ||
			d.plan.command.Open.Currency != "USD" {
			t.Fatalf("review did not retain selected account: %+v", d.plan)
		}
		noOpenWrite()
	}
	openDraft()
	click("Close")
	checkVisit()
	noOpenWrite()

	openDraft()
	click("Confirm")
	checkVisit()
	products, err := app.ListProducts(ctx, nil, true)
	if err != nil || len(products) != 1 {
		t.Fatalf("expected one saved term deposit: %v / %+v", err, products)
	}
	created := products[0]
	if created.Contract.Name != termName || created.Contract.AccountID != mapleID || created.Contract.Revision != 1 ||
		created.Contract.Principal.CanonicalAmount() != "50.125" || created.Contract.Kind != domain.ProductTermDeposit {
		t.Fatalf("wrong saved contract: %+v", created.Contract)
	}
	if created.Contract.HoldingID == "" || created.Contract.InstrumentID == "" {
		t.Fatal("new contract did not retain its managed position identity")
	}
	assertNativeTradeCash(t, app, mapleID, map[domain.CurrencyCode]string{"USD": "1749.875"})
	readProceeds := func(want string) {
		t.Helper()
		report, err := app.LiquidityRead(ctx, application.LiquidityOverviewQuery{})
		if err != nil {
			t.Fatal(err)
		}
		for _, src := range report.Sources {
			if src.Result.ProductID != nil && *src.Result.ProductID == created.Contract.ID {
				row := fundsResultForHorizon(src.Result, month)
				if row != nil && row.NetNative != nil && row.NetNative.CanonicalAmount() == want {
					return
				}
				t.Fatalf("expected projected proceeds %s at %s: %+v", want, month, row)
			}
		}
		t.Fatal("managed product missing from Funds read")
	}
	readProceeds("55.25")

	all := fundsSourceCount(funds.current.report.Sources, month, fundsAll)
	click(fmt.Sprintf("All %d", all))
	const edit = "Edit terms and rules"
	scrollProductControl(t, h, "Open product · "+termName)
	click("Open product · " + termName)
	scrollProductControl(t, h, edit)
	termsPosition := detail.list.Position
	click(edit)
	terms := detail.terms
	if terms == nil || detail.id != created.Contract.ID {
		t.Fatal("edit did not open on the saved product")
	}
	h.focused(&terms.fields.maturity.Trigger)
	terms.fields.deposit.maturityInterest.Editor.SetText("6.75")
	click("Close")
	if detail.terms != nil || detail.id != created.Contract.ID || detail.list.Position.First != termsPosition.First {
		t.Fatal("close changed the product detail visit")
	}
	h.focused(&detail.startTermsEdit)
	readProceeds("55.25")

	afterOpen, err := app.ListActivities(ctx, 100)
	if err != nil || len(afterOpen) != len(activities)+1 {
		t.Fatalf("expected one opening activity: %v", err)
	}
	click(edit)
	detail.terms.fields.deposit.maturityInterest.Editor.SetText("7.50")
	click("Save policy")
	if detail.terms != nil || detail.id != created.Contract.ID || detail.list.Position.First != termsPosition.First {
		t.Fatal("saving terms changed the product detail visit")
	}
	h.focused(&detail.startTermsEdit)
	saved, err := app.Product(ctx, created.Contract.ID)
	if err != nil || saved.Contract.HoldingID != created.Contract.HoldingID || saved.Contract.InstrumentID != created.Contract.InstrumentID ||
		saved.Contract.Revision != 2 || saved.Contract.Principal.CanonicalAmount() != "50.125" {
		t.Fatalf("terms save changed the position or failed: %+v / %v", saved.Contract, err)
	}
	gotActivities, err := app.ListActivities(ctx, 100)
	if err != nil || !reflect.DeepEqual(gotActivities, afterOpen) || len(managedOperations(t, app, created.Contract.ID)) != 1 {
		t.Fatal("editing terms added a ledger operation")
	}
	assertNativeTradeCash(t, app, mapleID, map[domain.CurrencyCode]string{"USD": "1749.875"})
	readProceeds("57.5")
}
