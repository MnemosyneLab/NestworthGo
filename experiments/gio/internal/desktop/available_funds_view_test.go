package desktop

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	liquidityapi "github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

func TestAvailableFundsAmountsUseCurrencyPrecision(t *testing.T) {
	for _, tc := range []struct {
		currency       domain.CurrencyCode
		amount         string
		decimal, group string
		want           string
	}{
		{"USD", "1234.565", settings.DecimalDot, settings.GroupingComma, "US$1,234.56"},
		{"AUD", "1234.575", settings.DecimalComma, settings.GroupingDot, "A$1.234,58"},
		{"SGD", "12003.8999", settings.DecimalComma, settings.GroupingSpace, "S$12 003,90"},
		{"SGD", "0", settings.DecimalComma, settings.GroupingDot, "S$0,00"},
		{"JPY", "1234.56", settings.DecimalDot, settings.GroupingComma, "JP¥1,235"},
	} {
		view := ui.View{Language: i18n.English, DecimalSeparator: tc.decimal, GroupingSeparator: tc.group}
		amount, err := domain.ParseMoney(tc.amount, tc.currency)
		if err != nil {
			t.Fatal(err)
		}
		native := amount.Amount()
		if projection, component := fundsAmountText(view, &amount), fundsNativeAmountText(view, &native, tc.currency); projection != tc.want || component != tc.want {
			t.Errorf("%s %s displays as %q and %q, want %q", tc.amount, tc.currency, projection, component, tc.want)
		}
		if amount.CanonicalAmount() != tc.amount || native.String() != tc.amount {
			t.Errorf("formatting changed the exact %s amount: %s / %s", tc.currency, amount.CanonicalAmount(), native)
		}
	}
	view := ui.View{Language: i18n.English}
	if projection, component := fundsAmountText(view, nil), fundsNativeAmountText(view, nil, "SGD"); projection != "Unavailable" || component != "Unavailable" {
		t.Errorf("missing amounts display as %q and %q", projection, component)
	}
}

func TestAvailableFundsComparisonDistinguishesSubminorZeroAndUnknown(t *testing.T) {
	view := ui.View{Language: i18n.English, DecimalSeparator: settings.DecimalDot, GroupingSeparator: settings.GroupingComma}
	for _, tc := range []struct{ amount, display string }{
		{"0", "US$0.00"},
		{"0.004", "US$0.00 (0.004 USD)"},
		{"0.006", "US$0.01"},
	} {
		money, err := domain.ParseMoney(tc.amount, "USD")
		if err != nil {
			t.Fatal(err)
		}
		native := money.Amount()
		if got := fundsComparisonMoney(view, &money); got != tc.display {
			t.Errorf("base %s displayed as %q, want %q", tc.amount, got, tc.display)
		}
		if got := fundsComparisonAmount(view, &native, "USD"); got != tc.display {
			t.Errorf("native %s displayed as %q, want %q", tc.amount, got, tc.display)
		}
	}
	if got := fundsComparisonMoney(view, nil); got != "Unknown" {
		t.Errorf("unknown amount displayed as %q", got)
	}
}

func TestAvailableFundsNarrowToolbarAndCurrencyRows(t *testing.T) {
	app, _, _, _ := openFundsFixture(t, true)
	p, h, settle := reservationHarness(t, app, app)
	h.size = image.Pt(900, 600)
	settle()
	date := semanticBounds(t, &h.router, "Custom date", true)
	early := assertPageLabel(t, &h.router, "Consider early withdrawal")
	if date.Max.X >= early.Min.X || date.Min.Y >= early.Max.Y || early.Min.Y >= date.Max.Y || date.Dx() > 200 {
		t.Fatalf("date and early-withdrawal controls do not fit in one compact toolbar: %v / %v", date, early)
	}
	if early.Max.Y > h.size.Y || early.Dy() > 40 {
		t.Fatalf("early withdrawal exceeds its toolbar row: %v", early)
	}
	for _, label := range []string{"Native amounts · EUR", "Native amounts · USD", "Known available after reservations · US$85.31"} {
		if bounds := assertPageLabel(t, &h.router, label); bounds.Empty() || bounds.Min.Y < 0 || bounds.Max.Y > h.size.Y {
			t.Errorf("%q is outside the initial 900×600 window: %v", label, bounds)
		}
	}
	if p.list.Position.First != 0 {
		t.Fatal("overview visibility required scrolling")
	}
	h.click("About these amounts", false)
	if h.shell.overlay.Trigger != &p.controls.comparisonAbout || h.shell.overlay.Panel.Empty() {
		t.Fatal("comparison help did not open")
	}
	assertPageLabel(t, &h.router, i18n.English.Text(i18n.FundsComparisonNote))
	h.press(key.NameEscape)
	if h.shell.overlay.Trigger != nil {
		t.Fatal("Escape did not dismiss comparison help")
	}
	h.focused(&p.controls.comparisonAbout)
}

func TestAvailableFundsHorizonControlsMatchWails(t *testing.T) {
	ctx := t.Context()
	app, _, _, fixture := openManagedProductFixture(t)
	adapter := liquidityapi.NewService(app)
	const product = "Flexible Deposit"
	const customDate = "2026-12-02"
	zero, calendar, earlyMode := 0, "calendar", "fixed_gross"
	command := liquidityapi.ProductCommandRequest{Kind: "open", Open: &application.OpenProductCommand{
		AccountID: fixture.account.String(), Currency: "USD", Principal: "250",
		EffectiveAt: accountTestClock.Add(-time.Hour).Format(time.RFC3339),
		Terms: application.ProductTermsInput{Kind: "term_deposit", Name: product, StartOn: "2026-11-22",
			MaturityOn: productOptional(customDate), InterestMode: "manual_maturity_amount", MaturityInterest: productOptional("5")},
		Policy: application.ProductPolicyInput{AccessKind: "on_date", UnlockOn: productOptional(customDate), NormalExitFee: productOptional("1"),
			SettlementDays: &zero, DayBasis: &calendar, EarlyKind: "allowed", EarlySettlementDays: &zero, EarlyDayBasis: &calendar,
			EarlyFee: productOptional("12.5"), EarlyAmountMode: &earlyMode, EarlyGrossAmount: productOptional("250")},
	}}
	preview, err := adapter.PreviewProductOperation(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.RecordProductOperation(ctx, liquidityapi.RecordProductOperationRequest{
		Command: command, ReviewedStateHash: preview.ReviewedStateHash, MutationID: domain.NewProductOperationID().String(),
	}); err != nil {
		t.Fatal(err)
	}
	version := app.MobileDataVersion()
	p := newAvailableFundsPage(ctx, app, func() {})
	t.Cleanup(p.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{funds: p})
	h.size = image.Pt(665, 480)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		p.workers.Wait()
		h.frame()
		h.frame()
	}
	assertProjection := func(selected, value string, kind domain.RouteKind) {
		t.Helper()
		if p.current == nil {
			t.Fatalf("horizon %q has no report: %v (state %+v)", selected, p.err, p.state)
		}
		if got := p.selectedFundsBucket().HorizonOn; got != selected {
			t.Fatalf("selected horizon = %q, want %q", got, selected)
		}
		query := p.state.query()
		want, err := adapter.Overview(ctx, liquidityapi.OverviewRequest{CustomHorizonOn: query.CustomHorizonOn, IncludeEarlyWithdrawal: query.IncludeEarlyWithdrawal})
		if err != nil {
			t.Fatal(err)
		}
		var gotResult *domain.LiquidityBucketResult
		for _, row := range p.current.report.Sources {
			if row.Result.DisplayName == product {
				gotResult = fundsResultForHorizon(row.Result, selected)
			}
		}
		if gotResult == nil {
			t.Fatalf("Gio lacks %s at %s", product, selected)
		}
		if value == "" {
			if gotResult.SelectedRoute != nil || gotResult.NetNative != nil || gotResult.Status != domain.StatusComplete {
				t.Fatalf("later access showed proceeds early: %+v", gotResult)
			}
			return
		}
		if gotResult.SelectedRoute == nil || gotResult.SelectedRoute.Kind != kind || gotResult.NetNative == nil || gotResult.NetNative.CanonicalAmount() != value {
			t.Fatalf("Gio selected %+v, want %s for %s", gotResult, value, kind)
		}
		for _, source := range want.Sources {
			if source.DisplayName != product {
				continue
			}
			for _, result := range source.BucketResults {
				if result.HorizonOn == selected && result.SelectedRoute != nil && result.SelectedRoute.Kind == string(kind) && result.NetNative != nil && result.NetNative.Amount == value {
					return
				}
			}
		}
		t.Fatalf("Wails did not return %s %s on %s", kind, value, selected)
	}
	h.frame()
	h.click("Available funds", false)
	settle()
	assertProjection("2026-11-22", "", "")
	h.click("Within 7 days", false)
	for attempts := 0; attempts < 30 && !h.router.Source().Focused(&p.controls.horizons[2]); attempts++ {
		h.router.MoveFocus(key.FocusForward)
		h.frame()
	}
	h.focused(&p.controls.horizons[2])
	assertProjection("2026-11-29", "", "")
	h.press(key.NameReturn)
	assertProjection("2026-12-22", "254", domain.RouteNormal)

	h.click("Custom date", true)
	h.press(key.NamePageDown)
	h.press(key.NameRightArrow)
	h.press(key.NameReturn)
	settle()
	if p.state.customOn != customDate {
		t.Fatalf("keyboard chose %q, want %q", p.state.customOn, customDate)
	}
	assertProjection(customDate, "254", domain.RouteNormal)
	h.click("Today", false)
	for attempts := 0; attempts < 30 && !h.router.Source().Focused(&p.controls.early); attempts++ {
		h.router.MoveFocus(key.FocusForward)
		h.frame()
	}
	h.focused(&p.controls.early)
	h.press(key.NameSpace)
	settle()
	if !p.state.includeEarly {
		t.Fatal("early withdrawal did not update the query")
	}
	assertProjection("2026-11-22", "237.5", domain.RouteEarly)
	h.click("Custom date", true)
	h.press(key.NameDeleteBackward)
	settle()
	if p.state.customOn != "" || p.selectedFundsBucket().HorizonOn != "2026-11-22" {
		t.Fatalf("clearing custom date did not select Today: %+v", p.state)
	}
	if app.MobileDataVersion() != version {
		t.Fatal("using the read controls changed ledger data")
	}
}
