package desktop

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	liquidityapi "github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

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
