package desktop

import (
	"context"
	"errors"
	"github.com/waltwang/nestworth-go/internal/testfixture"
	"image"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	liquidityapi "github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type managedProductFixture struct {
	account domain.AccountID
	ids     map[string]domain.ProductContractID
}

func seedManagedProducts(ctx context.Context, app *application.Service, at time.Time) (managedProductFixture, error) {
	fixture := managedProductFixture{ids: make(map[string]domain.ProductContractID)}
	testfixture.SetClock(app, func() time.Time { return at.Add(-72 * time.Hour) })
	if err := app.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Synthetic term deposits", BaseCurrency: "USD", MemberNames: []string{"Ana Example"},
	}); err != nil {
		return fixture, err
	}
	boot, err := app.Bootstrap(ctx)
	if err != nil {
		return fixture, err
	}
	bank, err := testfixture.CreateAccount(app, ctx, application.AccountInput{
		Name: "Harbor Deposit Bank", AccountType: "bank_account", TrackingMode: "holdings",
		BalanceSheetRole: "asset", DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{boot.Members[0].ID},
		IncludeInNetWorth: true, IncludeInPortfolio: true,
	})
	if err != nil {
		return fixture, err
	}
	fixture.account = bank.Account.ID
	if _, err := testfixture.AppendAccountCashValue(app, ctx, fixture.account, "3000", "USD", ""); err != nil {
		return fixture, err
	}
	if _, err := testfixture.CaptureSyntheticOrigin(app, ctx, "Asia/Singapore"); err != nil {
		return fixture, err
	}
	testfixture.SetClock(app, func() time.Time { return at })

	api := liquidityapi.NewService(app)
	record := func(command liquidityapi.ProductCommandRequest) (liquidityapi.ProductOperationReceiptDTO, error) {
		preview, err := api.PreviewProductOperation(ctx, command)
		if err != nil {
			return liquidityapi.ProductOperationReceiptDTO{}, err
		}
		return api.RecordProductOperation(ctx, liquidityapi.RecordProductOperationRequest{
			Command: command, ReviewedStateHash: preview.ReviewedStateHash, MutationID: domain.NewProductOperationID().String(),
		})
	}
	zero, calendar := 0, "calendar"
	for i, item := range []struct {
		name, principal, maturity, interest, fee string
		days                                     *int
	}{
		{"Winter Deposit", "1000", "2026-12-20", "12.3456", "1.2345", &zero},
		{"Due Deposit", "600", "2026-11-21", "6.25", "0", &zero},
		{"Incomplete Deposit", "300", "2026-11-21", "3", "0", nil},
		{"Settled Deposit", "200", "2026-11-21", "2", "0", &zero},
	} {
		reply, err := record(liquidityapi.ProductCommandRequest{Kind: "open", Open: &application.OpenProductCommand{
			AccountID: fixture.account.String(), Currency: "USD", Principal: item.principal,
			EffectiveAt: at.Add(-60*time.Hour + time.Duration(i)*time.Minute).Format(time.RFC3339),
			Terms: application.ProductTermsInput{Kind: "term_deposit", Name: item.name, StartOn: "2026-11-19",
				MaturityOn: &item.maturity, InterestMode: "manual_maturity_amount", MaturityInterest: &item.interest},
			Policy: application.ProductPolicyInput{AccessKind: "on_date", UnlockOn: &item.maturity, EarlyKind: "not_allowed",
				SettlementDays: item.days, DayBasis: &calendar, NormalExitFee: &item.fee},
		}})
		if err != nil {
			return fixture, err
		}
		fixture.ids[item.name] = domain.ProductContractID(reply.ProductIDs[0])
	}
	principal, amountZero := "200", "0"
	_, err = record(liquidityapi.ProductCommandRequest{Kind: "settle", Settle: &application.SettleProductCommand{
		ProductID: fixture.ids["Settled Deposit"].String(), ReturnedPrincipal: &principal, Interest: &amountZero, Fee: &amountZero,
		EffectiveAt: at.Add(-12 * time.Hour).Format(time.RFC3339),
	}})
	return fixture, err
}

func openManagedProductFixture(t *testing.T) (*application.Service, *pilot.Session, pilot.Paths, managedProductFixture) {
	t.Helper()
	paths := prepareOnboardingProfile(t)
	app, storage := reopenBaselineProfile(t, paths)
	fixture, err := seedManagedProducts(t.Context(), app, accountTestClock)
	if err != nil {
		t.Fatal(err)
	}
	return app, storage, paths, fixture
}

type oneProductReadFailure struct {
	*application.Service
	failed atomic.Bool
}

func (s *oneProductReadFailure) Product(ctx context.Context, id domain.ProductContractID) (application.ProductDetail, error) {
	if !s.failed.Swap(true) {
		return application.ProductDetail{}, errors.New("synthetic product read failure")
	}
	return s.Service.Product(ctx, id)
}

func TestManagedTermDepositReadOnlyMatchesWails(t *testing.T) {
	ctx := t.Context()
	app, session, paths, fixture := openManagedProductFixture(t)
	reference, _, _, ref := openManagedProductFixture(t)
	adapter := liquidityapi.NewService(reference)
	report, err := adapter.Overview(ctx, liquidityapi.OverviewRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if report.LocalDate != "2026-11-22" || report.Timezone != "Asia/Singapore" {
		t.Fatalf("wrong valuation date: %s / %s", report.LocalDate, report.Timezone)
	}

	canonical := func(m *domain.Money) *wire.MoneyView {
		if m == nil {
			return nil
		}
		return &wire.MoneyView{Amount: m.CanonicalAmount(), Currency: m.Currency().String()}
	}
	check := func(service *application.Service) {
		t.Helper()
		gotList := readAccounts(ctx, service, navigationTarget{account: fixture.account}, false)
		refAccount := ref.account.String()
		wantList, err := adapter.ListProducts(ctx, liquidityapi.ListProductsRequest{AccountID: &refAccount, IncludeClosed: true})
		if err != nil || gotList.err != nil || len(gotList.products) != 4 || len(wantList) != 4 {
			t.Fatalf("account products: %d / %d, %v / %v", len(gotList.products), len(wantList), gotList.err, err)
		}
		for i, product := range wantList {
			if gotList.products[i].Contract.Name != product.Product.Name || !reflect.DeepEqual(canonical(gotList.products[i].CurrentValue), product.Product.CurrentValue) {
				t.Fatalf("account list differs from Wails: %+v / %+v", gotList.products[i], product)
			}
		}
		for _, want := range []struct {
			name, state, principal, value, forecast, date string
		}{
			{"Winter Deposit", "locked", "1000", "1000", "1011.1111", "2026-12-20"},
			{"Due Deposit", "due_unconfirmed", "600", "600", "606.25", "2026-11-21"},
			{"Incomplete Deposit", "needs_info", "300", "300", "", ""},
			{"Settled Deposit", "settled", "200", "0", "", ""},
		} {
			got, err := readProduct(ctx, service, fixture.ids[want.name])
			if err != nil || got.detail == nil {
				t.Fatalf("%s: %v / %+v", want.name, err, got)
			}
			other, err := adapter.Product(ctx, ref.ids[want.name].String())
			if err != nil {
				t.Fatal(err)
			}
			d, contract := got.detail, got.detail.Contract
			if contract.AccountID != fixture.account || other.Product.AccountID != ref.account.String() || got.account != "Harbor Deposit Bank" ||
				contract.Kind != domain.ProductTermDeposit || other.Product.Kind != string(contract.Kind) || contract.StartOn != "2026-11-19" ||
				other.Product.StartOn != contract.StartOn || !reflect.DeepEqual(contract.MaturityOn, other.Product.MaturityOn) ||
				!reflect.DeepEqual(canonical(&contract.Principal), &other.Product.Principal) || contract.Principal.CanonicalAmount() != want.principal ||
				!reflect.DeepEqual(canonical(d.CurrentValue), other.Product.CurrentValue) ||
				!reflect.DeepEqual(canonical(d.CurrentValue), &wire.MoneyView{Amount: want.value, Currency: "USD"}) ||
				string(d.DisplayState) != want.state || other.Product.DisplayState != want.state ||
				!slices.Equal(d.PermittedActions, other.PermittedActions) || !reflect.DeepEqual(d.DisabledReasons, other.DisabledReasons) {
				t.Fatalf("%s product mismatch: %+v / %+v", want.name, d, other)
			}
			if want.state == "settled" {
				if got.source != nil || len(d.PermittedActions) != 0 || d.DisabledReasons["settle"] != "the contract is not open" {
					t.Fatalf("closed product has an open action or forecast: %+v / %+v", d, got.source)
				}
				continue
			}
			i := slices.IndexFunc(report.Sources, func(source liquidityapi.LiquiditySourceDTO) bool {
				return source.ProductID != nil && *source.ProductID == ref.ids[want.name].String()
			})
			if got.source == nil || got.source.NormalRoute == nil || i < 0 || report.Sources[i].NormalRoute == nil {
				t.Fatalf("%s: missing normal route", want.name)
			}
			source, normal := report.Sources[i], got.source.NormalRoute
			if !reflect.DeepEqual(canonical(normal.NetNative), source.NormalRoute.NetNative) ||
				!reflect.DeepEqual(normal.ReceiptOn, source.NormalRoute.ReceiptOn) ||
				!slices.Equal(normal.MissingReasons, source.NormalRoute.MissingReasons) ||
				!slices.Contains(d.PermittedActions, "settle") || !slices.Contains(d.PermittedActions, "receive_interest") {
				t.Fatalf("%s: route/action mismatch: %+v / %+v", want.name, normal, source)
			}
			if want.forecast == "" {
				if normal.NetNative != nil || normal.ReceiptOn != nil || !slices.Contains(normal.MissingReasons, "receipt timing is unknown") {
					t.Fatalf("incomplete forecast became known: %+v", normal)
				}
			} else if normal.NetNative == nil || normal.NetNative.CanonicalAmount() != want.forecast || normal.ReceiptOn == nil || *normal.ReceiptOn != want.date {
				t.Fatalf("%s: incorrect forecast: %+v", want.name, normal)
			}
			today := slices.IndexFunc(source.BucketResults, func(row liquidityapi.BucketResultDTO) bool { return row.HorizonOn == report.LocalDate })
			if today < 0 || source.BucketResults[today].SelectedRoute != nil || source.DueUnconfirmed != (want.state == "due_unconfirmed") {
				t.Fatalf("%s: product was credited as available cash: %+v", want.name, source)
			}
		}
	}
	check(app)
	before, err := app.ListActivities(ctx, 20)
	if err != nil || len(before) == 0 {
		t.Fatalf("fixture activities: %+v / %v", before, err)
	}
	version := app.MobileDataVersion()
	assertCash := func(service *application.Service) {
		t.Helper()
		read, err := readAvailableFunds(ctx, service)
		if err != nil {
			t.Fatal(err)
		}
		i := slices.IndexFunc(read.report.Sources, func(source application.LiquidityReadSource) bool {
			return source.Result.Ref.Kind == domain.SourceAccountCash && source.Result.AccountID == fixture.account
		})
		if i < 0 || !reflect.DeepEqual(canonical(read.report.Sources[i].Result.CurrentNativeValue), &wire.MoneyView{Amount: "1100", Currency: "USD"}) {
			t.Fatal("forecast changed actual account cash")
		}
	}
	assertCash(app)

	funds := newAvailableFundsPage(ctx, app, func() {})
	funds.productPage = newProductDetailPage(ctx, &oneProductReadFailure{Service: app}, func() {})
	accounts := newAccountsPage(ctx, app, func() time.Time { return accountTestClock }, func() {})
	portfolio := newPortfolioPage(ctx, app, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{funds: funds, accounts: accounts, portfolio: portfolio, version: app.MobileDataVersion})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	drain := func() {
		funds.workers.Wait()
		funds.productPage.workers.Wait()
		accounts.workers.Wait()
		portfolio.workers.Wait()
	}
	t.Cleanup(drain)
	click := func(name string) {
		t.Helper()
		h.click(name, false)
		for range 3 {
			h.frame()
			drain()
		}
		h.frame()
	}
	label := func(name string) { t.Helper(); assertPageLabel(t, &h.router, name) }
	h.frame()
	click("Available funds")
	const winter = "Open product · Winter Deposit"
	scrollProductControl(t, h, winter)
	click(winter)
	label("Could not load product details")
	click("Retry")
	label("Current contract value · 1000 USD")
	label("Estimated net proceeds · 1011.1111 USD")
	label("Confirm actual receipt · Available for review")
	const bank = "Open account · Harbor Deposit Bank"
	click(bank)
	scrollToAccountDetailLabel(h, "No investment holdings.")
	label("No investment holdings.")
	scrollProductControl(t, h, winter)
	click(winter)
	label("Current contract value · US$1,000.00")
	click("Close")
	h.focused(accounts.productButtons[fixture.ids["Winter Deposit"]])
	click("Back")
	h.focused(funds.productPage.accounts[fixture.account])
	click("Back")
	fundsRef := funds.current.report.Sources[slices.IndexFunc(funds.current.report.Sources, func(row application.LiquidityReadSource) bool {
		return row.Result.ProductID != nil && *row.Result.ProductID == fixture.ids["Winter Deposit"]
	})].Result.SourceKey
	h.focused(&funds.sourceButtons(fundsRef).product)
	click("Forward")
	label("Current contract value · 1000 USD")
	click("Back")
	click("Investment assets")
	const cashLike = "Cash-like · 3 holdings"
	scrollProductControl(t, h, cashLike)
	label(cashLike)
	row := slices.IndexFunc(portfolio.current.groups[0].Instruments, func(row domain.InstrumentHoldingsView) bool {
		return row.Name == "Winter Deposit" && row.Amounts.Quantity != "0"
	})
	if row < 0 {
		t.Fatal("My holdings omitted the active term deposit")
	}
	instrument := portfolio.current.groups[0].Instruments[row]
	name := holdingInstrumentName(instrument)
	scrollProductControl(t, h, name)
	click(name)
	managed := portfolio.current
	if managed.product == nil || managed.product.Contract.ID != fixture.ids["Winter Deposit"] || len(managed.positions) != 0 || managed.price != nil {
		t.Fatalf("historical instrument target was classified as an ordinary position: %+v", managed)
	}
	click(winter)
	label("Current contract value · 1000 USD")
	if missing, err := readProduct(ctx, app, domain.NewProductContractID()); err != nil || missing.detail != nil {
		t.Fatalf("missing product was resolved or became a read error: %+v / %v", missing, err)
	}
	after, err := app.ListActivities(ctx, 20)
	if err != nil || !reflect.DeepEqual(before, after) || version != app.MobileDataVersion() {
		t.Fatalf("product browsing modified the ledger: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	check(reopened)
	assertCash(reopened)
}

func scrollProductControl(t *testing.T, h *previewHarness, name string) {
	t.Helper()
	for range 16 {
		if findPageLabel(&h.router, name) != nil {
			bounds := semanticBounds(t, &h.router, name, false)
			if bounds.Min.Y > 135 && bounds.Max.Y <= h.size.Y-10 {
				return
			}
		}
		h.router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(470, 440), Scroll: f32.Pt(0, 140)})
		h.frame()
	}
	t.Fatalf("could not scroll to %s", name)
}
