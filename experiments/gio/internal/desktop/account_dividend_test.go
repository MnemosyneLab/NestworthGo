package desktop

import (
	"context"
	"errors"
	"image"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analytics"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	portfolioapi "github.com/waltwang/nestworth-go/internal/wailsapi/portfolio"
)

func seedDividend(ctx context.Context, service *application.Service, now time.Time) (saleFixture, error) {
	ids, err := seedSales(ctx, service, now)
	if err != nil {
		return saleFixture{}, err
	}
	_, err = historyapi.NewService(service).RecordChange(ctx, historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeTrade, Side: "sell", SettlementAccountID: ids.account.String(),
		InstrumentID: ids.instrument.String(), HoldingID: ids.holding.String(),
		Quantity: "2", Gross: "70.125", GrossCurrency: "USD", Fee: "1.125", FeeCurrency: "USD",
		EffectiveAt: now.Add(-time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		return ids, err
	}
	service.SetClock(func() time.Time { return now.AddDate(0, 0, 2) })
	defer service.SetClock(func() time.Time { return now })
	for _, date := range []string{"2026-11-20", "2026-11-21", "2026-11-22"} {
		if _, err := service.AppendManualInstrumentQuote(ctx, ids.instrument, "40", date, false); err != nil {
			return ids, err
		}
	}
	return ids, nil
}

func openDividendFixture(t *testing.T) (*application.Service, *pilot.Session, pilot.Paths, saleFixture) {
	t.Helper()
	paths := prepareOnboardingProfile(t)
	service, storage := reopenOnboardingProfile(t, paths)
	ids, err := seedDividend(t.Context(), service, accountTestClock)
	if err != nil {
		t.Fatal(err)
	}
	return service, storage, paths, ids
}

func TestCashDividendGioPreviewReplayAndWailsParity(t *testing.T) {
	ctx := t.Context()
	service, storage, paths, ids := openDividendFixture(t)
	reference, _, _, ref := openDividendFixture(t)
	fault := &cashMovementFault{Service: service, loseAttempt: 2, started: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.release) })
	clock := func() time.Time { return accountTestClock }
	accounts := newAccountsPage(ctx, service, clock, func() {})
	accounts.investment = &accountInvestment{service: fault}
	ledger := newLedgerHistory(ctx, fault, clock, func() {})
	investments := newPortfolioPage(ctx, service, func() {})
	overview := newOverviewPage(ctx, service, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{accounts: accounts, history: ledger, portfolio: investments, overview: overview, version: service.MobileDataVersion})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	t.Cleanup(func() { release(); accounts.workers.Wait() })
	settle := func() {
		for range 3 {
			h.frame()
			accounts.workers.Wait()
			ledger.model.Wait()
			ledger.reads.workers.Wait()
			investments.workers.Wait()
			overview.workers.Wait()
		}
		h.frame()
	}
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	label := func(text string) { t.Helper(); assertPageLabel(t, &h.router, text) }

	settle()
	label("$ 254.00")
	click("Accounts")
	click("Harbor Brokerage")
	click("Record cash dividend")
	d := accounts.investment.draft
	if d.kind != domain.ActivityCashDividend || len(d.sources.Instruments) != 1 || d.sources.Holdings[d.instrument].ID != ids.holding ||
		d.day != "2026-11-22" || d.time != "01:00" {
		t.Fatalf("wrong dividend target or local time: %+v", d)
	}
	label("Enter the cash actually credited after any withholding or fees.")
	if liabilityTextAppears(h.router.AppendSemantics(nil), "Fee (optional) · USD") {
		t.Fatal("dividend exposed a separate trading fee")
	}
	adapter := historyapi.NewService(reference)
	request := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeCashDividend, HoldingID: ref.holding.String(), Currency: "USD",
		EffectiveLocalDate: "2026-11-22", EffectiveLocalTime: "01:00", MutationID: domain.NewMutationID().String(),
	}
	for _, amount := range []string{"0", "-1", "12.34567"} {
		setBankField(h, "Cash received · USD", &d.gross, amount)
		click("Review cash dividend")
		label("Enter a positive cash amount with up to four decimal places.")
		request.Amount = amount
		if _, err := adapter.PreviewChange(ctx, request); err == nil {
			t.Fatalf("Wails preview accepted %q", amount)
		}
	}
	if rows, err := service.ListActivities(ctx, 20); len(fault.attempts) != 0 || err != nil || len(rows) != 3 {
		t.Fatalf("invalid draft changed the seeded trades: %d records, %d attempts, %v", len(rows), len(fault.attempts), err)
	}
	setBankField(h, "Cash received · USD", &d.gross, "12.3456")
	click("Review cash dividend")
	label("Net cash credited · 12.3456 USD")
	label("Resulting balance · 146.3456 USD")

	contribute := func(app *application.Service, id domain.AccountID) {
		t.Helper()
		_, err := historyapi.NewService(app).RecordChange(ctx, historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeMoneyAdded, AccountID: id.String(), Reason: "contribution",
			Amount: "7.125", Currency: "USD", EffectiveLocalDate: request.EffectiveLocalDate, EffectiveLocalTime: "00:59",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	contribute(service, ids.account)
	contribute(reference, ref.account)
	click("Confirm cash dividend")
	label("The ledger changed. Review the dividend again before confirming.")
	if rows, err := service.ListActivities(ctx, 20); err != nil || len(rows) != 4 || len(fault.attempts) != 1 {
		t.Fatalf("stale review posted: %d records, %d attempts, %v", len(rows), len(fault.attempts), err)
	}
	click("Review cash dividend")
	label("Balance before · 141.125 USD")
	label("Net cash credited · 12.3456 USD")
	label("Resulting balance · 153.4706 USD")

	request.Amount = "12.3456"
	want := d.review.Preview.Review
	// The generic Wails request has fee fields for trades. Cash dividends
	// ignore them instead of deducting them from the amount already received.
	withUnrelatedFee := request
	withUnrelatedFee.Fee, withUnrelatedFee.FeeCurrency = "not-an-amount", "not-a-currency"
	for _, req := range []historyapi.ChangeCommandRequest{request, withUnrelatedFee} {
		preview, err := adapter.PreviewChange(ctx, req)
		if err != nil || len(preview.Review) != 1 || len(want) != 1 {
			t.Fatalf("unexpected Wails dividend preview: %+v / %v", preview, err)
		}
		got := preview.Review[0]
		if got.Before != want[0].Before || got.Delta != want[0].Delta || got.After != want[0].After ||
			got.Currency != want[0].Currency.String() || got.Target != string(want[0].Target) {
			t.Fatalf("Wails cash endpoint %+v differs from Gio %+v", got, want[0])
		}
	}

	bounds := semanticBounds(t, &h.router, "Confirm cash dividend", false)
	point := bounds.Min.Add(bounds.Max).Div(2)
	h.clickAt(point)
	select {
	case <-fault.started:
	case <-time.After(3 * time.Second):
		t.Fatal("dividend confirmation did not start")
	}
	h.clickAt(point)
	release()
	settle()
	label("Posting could not be confirmed. Retry the same submission to check its result.")
	if len(fault.attempts) != 2 || !accounts.investment.unresolved() {
		t.Fatal("double-click started another write or discarded the pending confirmation")
	}
	click("Retry same submission")
	if len(fault.attempts) != 3 || !reflect.DeepEqual(fault.attempts[1], fault.attempts[2]) ||
		fault.firstActivity == "" || ledger.reads.posted.ID != fault.firstActivity {
		t.Fatalf("response recovery did not replay the same dividend: %+v", fault.attempts)
	}
	label("Cash dividend recorded.")
	label("Could not load this activity.")
	click("Retry")
	label("Harbor Brokerage · Added 12.3456 USD")
	label("Income")
	click("Open instrument · QA-BUY")
	label("Quantity 3")
	label("Cost basis · USD $ 81.00")
	label("Native market value · USD $ 120.00")
	click("Open account · Harbor Brokerage")
	click("Overview")
	label("$ 273.47")
	if len(fault.attempts) != 3 {
		t.Fatal("refresh or history navigation wrote again")
	}

	refPosted, err := adapter.RecordChange(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	assertDividendState(t, service, ids, fault.firstActivity)
	assertDividendState(t, reference, ref, domain.ActivityID(refPosted.Activity.ID))
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	reopened.SetClock(clock)
	frozen := fault.attempts[1]
	replay, err := reopened.RecordChangeGuarded(ctx, frozen.command, frozen.mutation, frozen.hash, frozen.token)
	if err != nil || replay.Activity.ID != fault.firstActivity {
		t.Fatalf("reopened receipt diverged: %+v / %v", replay, err)
	}
	assertDividendState(t, reopened, ids, fault.firstActivity)
}

func assertDividendState(t *testing.T, app *application.Service, ids saleFixture, id domain.ActivityID) {
	t.Helper()
	ctx := t.Context()
	rows, err := app.ListActivities(ctx, 20)
	if err != nil || len(rows) != 5 {
		t.Fatalf("expected two buys, one sell, one contribution and one dividend: %+v / %v", rows, err)
	}
	dividend, err := app.Activity(ctx, id)
	detail := dividend.DividendDetail
	if err != nil || dividend.Kind != domain.ActivityCashDividend || dividend.Reason != domain.ReasonIncome ||
		dividend.EffectiveLocalDate != "2026-11-22" || !dividend.EffectiveAt.Equal(accountTestClock) ||
		detail == nil || detail.HoldingID != ids.holding || detail.InstrumentID != ids.instrument ||
		dividend.TradeDetail != nil || len(dividend.Effects) != 1 {
		t.Fatalf("unexpected dividend activity: %+v / %v", dividend, err)
	}
	e := dividend.Effects[0]
	if e.Target != domain.EffectTargetAccountCash || e.Direction != domain.EffectAdded || e.Classification != domain.ClassificationIncome ||
		e.AccountID == nil || *e.AccountID != ids.account || e.Money == nil ||
		e.Money.CanonicalAmount() != "12.3456" || e.Money.Currency() != "USD" || e.HoldingID != nil {
		t.Fatalf("unexpected dividend effect: %+v", e)
	}
	positions, err := analytics.NewService(app).AccountHoldingDetails(ctx, ids.account.String())
	if err != nil || len(positions) != 1 || positions[0].Amounts.Quantity != "3" || positions[0].Amounts.TotalCost == nil ||
		positions[0].Amounts.TotalCost.Amount != "81" || positions[0].Amounts.CurrentValue == nil || positions[0].Amounts.CurrentValue.Amount != "120" {
		t.Fatalf("dividend changed the position projection: %+v / %v", positions, err)
	}
	gain, err := app.HoldingGain(ctx, ids.holding)
	if err != nil || gain.RealizedGain == nil || gain.RealizedGain.Amount != "15" {
		t.Fatalf("dividend changed realized trade gain: %+v / %v", gain, err)
	}
	const day = "2026-11-22"
	income, err := app.DividendIncomeInRange(ctx, domain.GainScope{AccountID: &ids.account, InstrumentID: &ids.instrument}, day, day)
	if err != nil || !income.Available || len(income.ByInstrument) != 1 || income.ByInstrument[0].Gain.Amount != "12.3456" {
		t.Fatalf("unexpected dividend income: %+v / %v", income, err)
	}
	value, err := app.AccountValuation(ctx, ids.account)
	if err != nil || !slices.ContainsFunc(value.Components, func(c domain.ValuationComponent) bool {
		return c.HoldingID == nil && c.NativeCurrency == "USD" && c.NativeAmount == "153.4706"
	}) {
		t.Fatalf("unexpected account cash: %+v / %v", value, err)
	}
	ov, err := portfolioapi.NewService(app).Overview(ctx, account.AccountFilterRequest{})
	if err != nil || !ov.Complete || ov.NetWorth != "273.4706" {
		t.Fatalf("unexpected net worth: %+v / %v", ov, err)
	}
	// Analysis only accepts closed days. Move the fixture clock forward
	// after verifying the original as-of value.
	app.SetClock(func() time.Time { return accountTestClock.AddDate(0, 0, 2) })
	defer app.SetClock(func() time.Time { return accountTestClock })
	analysis, err := app.Analyze(ctx, domain.AnalysisQuery{
		Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: day, To: day,
		Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var bucket, returns, external string
	for _, d := range analysis.Days {
		for _, effect := range d.AttributedEffects {
			if effect.SourceEffect.ActivityID != id {
				continue
			}
			if effect.DietzCapitalFlow != nil || (effect.AssetBucket != nil && *effect.AssetBucket != domain.BucketDividendInterest) {
				t.Fatalf("dividend attributed outside investment income: %+v", effect)
			}
			if effect.AssetBucket != nil {
				bucket = effect.Amount.CanonicalAmount()
			}
			if effect.ReturnComponent != nil && (*effect.ReturnComponent != domain.ReturnDividendInterest ||
				effect.RelatedHoldingID == nil || *effect.RelatedHoldingID != ids.holding) {
				t.Fatalf("dividend attributed to the wrong investment: %+v", effect)
			}
		}
		if total, ok := d.AssetBuckets[domain.BucketExternalFlow]; ok && !total.IsZero() {
			external = total.CanonicalAmount()
		}
		if total, ok := d.ReturnComponents[domain.ReturnDividendInterest]; ok && !total.IsZero() {
			returns = total.CanonicalAmount()
		}
	}
	if bucket != "12.3456" || returns != "12.3456" || external != "7.125" {
		t.Fatalf("dividend attribution = %q, return = %q, external flow = %q", bucket, returns, external)
	}
}

func TestCashDividendDateAndZeroQuantityHolding(t *testing.T) {
	ctx := t.Context()
	service, _, _, ids := openDividendFixture(t)
	_, err := historyapi.NewService(service).RecordChange(ctx, historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeTrade, Side: "sell", SettlementAccountID: ids.account.String(),
		InstrumentID: ids.instrument.String(), HoldingID: ids.holding.String(),
		Quantity: "3", Gross: "60", GrossCurrency: "USD",
		EffectiveAt: accountTestClock.Add(-time.Minute).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	sources, err := history.LoadInvestmentSources(ctx, service, ids.account, domain.ActivityCashDividend)
	if err != nil || len(sources.Instruments) != 1 || sources.Holdings[ids.instrument].ID != ids.holding || !sources.Holdings[ids.instrument].Quantity.IsZero() {
		t.Fatalf("the active zero holding was not selectable: %+v / %v", sources, err)
	}
	input := history.DividendInput{Account: ids.account, Instrument: ids.instrument, Holding: ids.holding, Amount: "1", Date: "2026-11-22", Time: "01:00"}
	if _, err := history.ReviewDividend(ctx, service, input); err != nil {
		t.Fatal(err)
	}
	for _, date := range []string{"2026-11-19", "2026-11-23"} {
		bad := input
		bad.Date = date
		_, err := history.ReviewDividend(ctx, service, bad)
		var issue *domain.Error
		if !errors.As(err, &issue) || issue.Code != domain.ErrInvalidChangeTime {
			t.Fatalf("bad local date %s: %v", date, err)
		}
	}
	if err := service.ArchiveHolding(ctx, ids.holding, true); err != nil {
		t.Fatal(err)
	}
	sources, err = history.LoadInvestmentSources(ctx, service, ids.account, domain.ActivityCashDividend)
	if err != nil || len(sources.Instruments) != 0 {
		t.Fatalf("archive did not remove the holding: %+v / %v", sources, err)
	}
	if _, err := history.ReviewDividend(ctx, service, input); err == nil {
		t.Fatal("preview accepted an archived holding")
	}
}
