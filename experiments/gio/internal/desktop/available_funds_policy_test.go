package desktop

import (
	"context"
	"errors"
	"image"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	liquidityapi "github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

type fundsPolicyFault struct {
	*application.Service
	saves, resets atomic.Int32
	failReadback  atomic.Bool
	brokenRead    atomic.Bool
}

func (f *fundsPolicyFault) SaveLiquidityPolicy(ctx context.Context, input application.SavePolicyInput) (domain.LiquidityPolicy, error) {
	f.saves.Add(1)
	value, err := f.Service.SaveLiquidityPolicy(ctx, input)
	if err == nil && f.failReadback.Swap(false) {
		f.brokenRead.Store(true)
	}
	return value, err
}

func (f *fundsPolicyFault) ResetLiquidityPolicy(ctx context.Context, ref domain.LiquiditySourceRef, expected int) error {
	f.resets.Add(1)
	return f.Service.ResetLiquidityPolicy(ctx, ref, expected)
}

func (f *fundsPolicyFault) LiquidityRead(ctx context.Context, query application.LiquidityOverviewQuery) (application.LiquidityReadResult, error) {
	if f.brokenRead.Load() {
		return application.LiquidityReadResult{}, errors.New("synthetic policy readback failure")
	}
	return f.Service.LiquidityRead(ctx, query)
}

func TestAvailableFundsOrdinaryPolicyFromOverview(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openFundsFixture(t, true)
	if _, err := app.AppendManualInstrumentQuote(ctx, ids.instrument, "12.3456", "", false); err != nil {
		t.Fatal(err)
	}
	holdings, err := app.HoldingsByAccounts(ctx, []domain.AccountID{ids.broker})
	if err != nil || len(holdings[ids.broker]) != 1 {
		t.Fatalf("fixture holdings: %+v, %v", holdings, err)
	}
	ref := domain.HoldingSourceRef(ids.broker, holdings[ids.broker][0].ID)
	today := accountTestClock.Format(time.DateOnly)
	month, unlock := fundsPresetDay(today, 30), fundsPresetDay(today, 14)
	zero := 0
	initial := application.ProductPolicyInput{AccessKind: "on_date", UnlockOn: &unlock,
		SettlementDays: &zero, DayBasis: productOptional("calendar"), EarlyKind: "not_allowed"}
	wails := liquidityapi.NewService(app)
	wailsRef := liquidityapi.SourceRefDTO{Kind: string(ref.Kind), AccountID: ref.AccountID.String()}
	holding := ref.HoldingID.String()
	wailsRef.HoldingID = &holding
	if _, err := wails.SavePolicy(ctx, liquidityapi.SavePolicyRequest{Source: wailsRef, Policy: initial}); err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return accountTestClock }
	overview := newOverviewPage(ctx, app, clock, func() {})
	fault := &fundsPolicyFault{Service: app}
	p := newAvailableFundsPage(ctx, fault, func() {})
	t.Cleanup(p.workers.Wait)
	t.Cleanup(overview.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{overview: overview, funds: p, version: app.MobileDataVersion})
	h.size = image.Pt(700, 460)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 4 {
			h.frame()
			overview.workers.Wait()
			p.workers.Wait()
		}
		h.frame()
	}
	click := func(name string) { t.Helper(); h.click(name, false); settle() }
	assertScope := func() {
		t.Helper()
		if h.shell.activeRoute() != fundsRoute || p.state.horizonOn != month || p.state.includeEarly || p.state.filter != fundsNeedsRules {
			t.Fatalf("30-day normal missing-rules scope changed: %+v", p.state)
		}
	}
	assertFee := func(fee string, revision int) {
		t.Helper()
		result := p.policySource(ref)
		if result == nil || result.Result.Policy == nil || result.Result.Policy.Revision != revision || result.Result.Policy.NormalExitFee == nil || result.Result.Policy.NormalExitFee.CanonicalAmount() != fee {
			t.Fatalf("saved fee differs at revision %d: %+v", revision, result)
		}
	}
	settle()
	if got := fundsSourceCount(overview.funds.current.Sources, today, fundsNeedsRules); got != 0 {
		t.Fatalf("fixture has %d missing rules today", got)
	}
	click("Within 30 days · 1 item needs availability rules")
	assertScope()
	button := &p.sourceButtons(ref.Key()).policy
	click("Complete availability rules")
	h.focused(&p.policy.fields.presets[policyNow])
	click("Cancel")
	assertScope()
	h.focused(button)
	click("Complete availability rules")
	if p.policy.revision != 1 || !p.policy.fields.advanced {
		t.Fatalf("did not open the saved custom rule: %+v", p.policy)
	}
	d := p.policy
	d.fields.fee.Editor.SetText("2")
	changed := initial
	changed.NormalExitFee = productOptional("1.25")
	if _, err := wails.SavePolicy(ctx, liquidityapi.SavePolicyRequest{Source: wailsRef, ExpectedRevision: 1, Policy: changed}); err != nil {
		t.Fatal(err)
	}
	click("Save policy")
	if p.policy != d || d.problem != i18n.FundsPolicyConflict || d.fields.fee.Text() != "2" || fault.saves.Load() != 1 {
		t.Fatalf("revision conflict did not retain the draft: %+v", p.policy)
	}
	assertFee("1.25", 2)
	h.focused(&d.reload)
	click("Load latest rules")
	if p.policy.revision != 2 || p.policy.fields.fee.Text() != "1.25" {
		t.Fatalf("reload did not start from the saved values: %+v", p.policy)
	}
	p.policy.fields.cap.Editor.SetText("20")
	p.policy.fields.fee.Editor.SetText("2")
	fault.failReadback.Store(true)
	click("Save policy")
	if p.policy == nil || !p.policy.checking || p.err == nil || fault.saves.Load() != 2 {
		t.Fatal("fault did not enter read-only recovery")
	}
	fault.brokenRead.Store(false)
	click("Retry")
	if p.policy != nil || fault.saves.Load() != 2 {
		t.Fatal("retry wrote the policy again or left the editor open")
	}
	assertScope()
	assertFee("2", 3)
	h.focused(&p.controls.filters[fundsNeedsRules])
	got := fundsResultForHorizon(p.policySource(ref).Result, month)
	if got == nil || got.NetNative == nil || got.NetNative.CanonicalAmount() != "18" || got.NeedsRules {
		t.Fatalf("shared cap and fee projection = %+v", got)
	}
	want := mustReadFundsAdapter(t, wails)
	if !slices.ContainsFunc(want.Sources, func(source liquidityapi.LiquiditySourceDTO) bool {
		return source.SourceKey == ref.Key() && slices.ContainsFunc(source.BucketResults, func(bucket liquidityapi.BucketResultDTO) bool {
			return bucket.HorizonOn == month && bucket.NetNative != nil && bucket.NetNative.Amount == "18" && !bucket.NeedsRules
		})
	}) {
		t.Fatal("Wails did not project USD 18 on the chosen horizon")
	}
	click("Back")
	click("Forward")
	assertScope()
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	reopened.SetClock(clock)
	read, err := reopened.LiquidityRead(ctx, application.LiquidityOverviewQuery{})
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range read.Sources {
		if source.Result.SourceKey == ref.Key() && source.Result.Policy.Revision == 3 && source.Result.Policy.NormalExitFee.CanonicalAmount() == "2" {
			return
		}
	}
	t.Fatal("reopened ledger did not retain the rule")
}

func TestAvailableFundsOrdinaryPolicyResetConfirmation(t *testing.T) {
	ctx := t.Context()
	app, _, _, _ := openFundsFixture(t, false)
	bootstrap, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := app.CreateAccount(ctx, application.AccountInput{Name: "Reset bank", AccountType: "bank_account", TrackingMode: "balance",
		BalanceSheetRole: "asset", DefaultCurrency: "USD", InitialAmount: "10", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	ref := domain.AccountValueSourceRef(account.Account.ID)
	if _, err := app.SaveLiquidityPolicy(ctx, application.SavePolicyInput{Source: ref, Policy: simpleFundsPolicy(policySoon, application.ProductPolicyInput{})}); err != nil {
		t.Fatal(err)
	}
	fault := &fundsPolicyFault{Service: app}
	p := newAvailableFundsPage(ctx, fault, func() {})
	t.Cleanup(p.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{funds: p, version: app.MobileDataVersion})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 4 {
			h.frame()
			p.workers.Wait()
		}
	}
	click := func(name string) { t.Helper(); h.click(name, false); settle() }
	settle()
	click("Available funds")
	for range 20 {
		if findPageLabel(&h.router, "Edit source rules") != nil {
			break
		}
		h.router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(550, 500), Scroll: f32.Pt(0, 150)})
		h.frame()
	}
	click("Edit source rules")
	if p.policy.revision != 1 || p.policy.fields.advanced {
		t.Fatalf("did not select the existing Soon quick choice: %+v", p.policy)
	}
	h.focused(&p.policy.fields.presets[policySoon])
	h.press(key.NameLeftArrow)
	settle()
	h.focused(&p.policy.fields.presets[policyNow])
	if current, ok := p.policy.fields.input(); !ok || selectedFundsPolicy(current) != policyNow {
		t.Fatal("keyboard did not select the Available now quick choice")
	}
	largeWindow := h.size
	h.size = image.Pt(720, 360)
	settle()
	h.press(key.NameTab)
	settle()
	h.focused(&p.policy.fields.toggle)
	if p.policy.sheet.list.Position.Offset == 0 {
		t.Fatal("Tab left the Advanced control below the sheet viewport")
	}
	h.router.Queue(key.Event{Name: key.NameTab, Modifiers: key.ModShift, State: key.Press},
		key.Event{Name: key.NameTab, Modifiers: key.ModShift, State: key.Release})
	settle()
	h.focused(&p.policy.fields.presets[policyNow])
	h.size = largeWindow
	settle()
	click("Available on a date")
	h.press(key.NameTab)
	settle()
	h.focused(&p.policy.fields.quickUnlock.Trigger)
	click("Save policy")
	if p.policy == nil || p.policy.problem != i18n.FundsPolicyDateRequired || p.policy.revision != 1 {
		t.Fatal("invalid date policy reached the ledger")
	}
	click("Reset to default")
	if !p.policyConfirming() || fault.resets.Load() != 0 {
		t.Fatal("reset did not wait for confirmation")
	}
	h.press(key.NameEscape)
	settle()
	if p.policyConfirming() || fault.resets.Load() != 0 {
		t.Fatal("Escape changed the policy")
	}
	h.focused(&p.policy.reset)
	h.press(key.NameReturn)
	settle()
	if !p.policyConfirming() {
		t.Fatal("Return did not reopen reset confirmation")
	}
	click("Confirm reset")
	row := p.policySource(ref)
	if p.policy != nil || fault.resets.Load() != 1 || row == nil || row.Result.PolicyOrigin != domain.PolicyOriginAssumed ||
		p.selectedFundsBucket().FullUnreserved.CanonicalAmount() != "10" {
		t.Fatalf("default rule or availability was not restored: %+v", row)
	}
	h.focused(&p.sourceButtons(ref.Key()).policy)
}
