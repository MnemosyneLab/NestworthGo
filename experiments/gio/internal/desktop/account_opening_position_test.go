package desktop

import (
	"context"
	"errors"
	"image"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analytics"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	holdingapi "github.com/waltwang/nestworth-go/internal/wailsapi/holding"
	"github.com/waltwang/nestworth-go/internal/wailsapi/portfolio"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type openingFixture struct {
	account       domain.AccountID
	security, eur domain.InstrumentID
}

func seedOpeningPosition(ctx context.Context, app *application.Service, at time.Time, quoted bool) (openingFixture, error) {
	var ids openingFixture
	app.SetClock(func() time.Time { return at.Add(-48 * time.Hour) })
	defer app.SetClock(func() time.Time { return at })
	if err := app.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Synthetic opening position", BaseCurrency: "USD", MemberNames: []string{"Ana Example"},
	}); err != nil {
		return ids, err
	}
	boot, err := app.Bootstrap(ctx)
	if err != nil {
		return ids, err
	}
	account, err := app.CreateAccount(ctx, application.AccountInput{
		Name: "Harbor Brokerage", AccountType: "brokerage", TrackingMode: "holdings", BalanceSheetRole: "asset",
		DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{boot.Members[0].ID}, IncludeInNetWorth: true, IncludeInPortfolio: true,
	})
	if err != nil {
		return ids, err
	}
	ids.account = account.Account.ID
	if _, err := app.AppendAccountCashValue(ctx, ids.account, "200", "USD", ""); err != nil {
		return ids, err
	}
	i, err := app.CreateInstrument(ctx, application.InstrumentInput{Name: "Synthetic Index Fund", Symbol: "QA-BASE", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		return ids, err
	}
	ids.security = i.ID
	foreign, err := app.CreateInstrument(ctx, application.InstrumentInput{Name: "Euro Sample Fund", Symbol: "QA-EUR", Type: "etf", QuoteCurrency: "EUR", QuoteSource: "manual"})
	if err != nil {
		return ids, err
	}
	ids.eur = foreign.ID
	if quoted {
		_, err = app.AppendManualInstrumentQuote(ctx, ids.security, "40", "", false)
	}
	return ids, err
}

func openStartingPositionFixture(t *testing.T, quoted bool) (*application.Service, *pilot.Session, pilot.Paths, openingFixture) {
	t.Helper()
	paths := prepareOnboardingProfile(t)
	app, session := reopenOnboardingProfile(t, paths)
	ids, err := seedOpeningPosition(t.Context(), app, accountTestClock, quoted)
	if err != nil {
		t.Fatal(err)
	}
	return app, session, paths, ids
}

type openingPositionFault struct {
	*application.Service
	writes         atomic.Int32
	failRead       atomic.Bool
	started, allow chan struct{}
}

func (f *openingPositionFault) CreateHolding(ctx context.Context, input application.HoldingInput) (domain.Holding, error) {
	if f.writes.Add(1) == 1 {
		close(f.started)
		<-f.allow
	}
	h, err := f.Service.CreateHolding(ctx, input)
	if err != nil {
		return h, err
	}
	f.failRead.Store(true)
	return domain.Holding{}, errors.New("synthetic lost holding observation reply")
}

func (f *openingPositionFault) HoldingsByAccounts(ctx context.Context, ids []domain.AccountID) (map[domain.AccountID][]domain.Holding, error) {
	if f.failRead.Swap(false) {
		return nil, errors.New("synthetic position verification read failure")
	}
	return f.Service.HoldingsByAccounts(ctx, ids)
}

func TestOpeningPositionAndCostMatchWails(t *testing.T) {
	t.Run("selected quote", func(t *testing.T) { testOpeningPositionAndCostMatchWails(t, true) })
	t.Run("missing quote", func(t *testing.T) { testOpeningPositionAndCostMatchWails(t, false) })
}

func testOpeningPositionAndCostMatchWails(t *testing.T, quoted bool) {
	ctx := t.Context()
	service, storage, paths, ids := openStartingPositionFixture(t, quoted)
	reference, refStorage, _, ref := openStartingPositionFixture(t, quoted)
	refHolding, refHistory := holdingapi.NewService(reference), historyapi.NewService(reference)
	opening := &openingPositionFault{Service: service, started: make(chan struct{}), allow: make(chan struct{})}
	start := &startingPointFault{Service: service, loseReply: true}
	release := sync.OnceFunc(func() { close(opening.allow) })
	now := accountTestClock
	clock := func() time.Time { return now }
	service.SetClock(clock)
	reference.SetClock(clock)
	accounts := newAccountsPage(ctx, service, clock, func() {})
	accounts.opening = &accountOpeningPosition{service: opening}
	accounts.investment = &accountInvestment{service: service}
	page := newLedgerHistory(ctx, start, clock, func() {})
	overview := newOverviewPage(ctx, service, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{accounts: accounts, history: page, overview: overview})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language, h.shell.prefs.saved.Timezone = settings.LanguageEnglish, "Asia/Singapore"
	zone, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	h.shell.prefs.savedLocation = zone
	t.Cleanup(func() { release(); accounts.workers.Wait(); page.reads.workers.Wait() })
	settle := func() {
		for range 4 {
			h.frame()
			accounts.workers.Wait()
			page.reads.workers.Wait()
			page.model.Wait()
			overview.workers.Wait()
		}
		h.frame()
	}
	click := func(text string) { t.Helper(); h.click(text, false); settle() }
	label := func(text string) { t.Helper(); assertPageLabel(t, &h.router, text) }
	settle()
	label("$ 200.00")
	click("Accounts")
	click("Harbor Brokerage")
	click("Record existing position")
	d := accounts.opening.draft
	if len(d.sources) != 1 || d.instrument != ids.security {
		t.Fatalf("position picker did not isolate the unused same-currency security: %+v", d.sources)
	}
	setBankField(h, "Existing quantity", &d.quantity, "-1")
	click("Review existing position")
	label("Enter a positive quantity with up to eight decimal places.")
	if _, err := refHolding.CreateHolding(ctx, holdingapi.CreateHoldingRequest{
		AccountID: ref.account.String(), InstrumentID: ref.security.String(), Quantity: "-1",
	}); err == nil {
		t.Fatal("Wails allowed negative opening quantity")
	}
	setBankField(h, "Existing quantity", &d.quantity, "2.5")
	click("Review existing position")
	label("Quantity 2.5")
	for _, side := range []struct {
		app *application.Service
		id  domain.AccountID
	}{{service, ids.account}, {reference, ref.account}} {
		if _, err := side.app.AppendAccountCashValue(ctx, side.id, "205", "USD", ""); err != nil {
			t.Fatal(err)
		}
	}
	click("Save position")
	label("The ledger changed. Preview this action again.")
	if opening.writes.Load() != 0 {
		t.Fatal("a stale plan wrote a holding")
	}
	click("Review existing position")
	oldPlan := *d.review
	box := semanticBounds(t, &h.router, "Save position", false)
	point := box.Min.Add(box.Max).Div(2)
	h.clickAt(point)
	select {
	case <-opening.started:
	case <-time.After(3 * time.Second):
		t.Fatal("position save did not start")
	}
	h.clickAt(point)
	release()
	settle()
	label("Could not verify the saved position. Retry checks the saved state.")
	click("Retry")
	label("Existing position saved.")
	if opening.writes.Load() != 1 {
		t.Fatal("confirmation or read retry created a second observation")
	}
	refSaved, err := refHolding.CreateHolding(ctx, holdingapi.CreateHoldingRequest{
		AccountID: ref.account.String(), InstrumentID: ref.security.String(), Quantity: "2.5", UnitCost: "99",
	})
	if err != nil || refSaved.Quantity != "2.5" {
		t.Fatalf("unexpected Wails quantity observation: %+v / %v", refSaved, err)
	}
	held, err := service.HoldingsByAccounts(ctx, []domain.AccountID{ids.account})
	if err != nil || len(held[ids.account]) != 1 || held[ids.account][0].Quantity.Canonical() != "2.5" {
		t.Fatalf("unexpected position observation: %+v / %v", held, err)
	}
	holdingID := held[ids.account][0].ID
	if _, err := history.LoadOpeningPositionSources(ctx, service, ids.account); err == nil {
		t.Fatal("the single-position workflow offered another opening observation")
	}
	// Cost is absent at this point even when Wails sends the optional UnitCost.
	for _, side := range []struct {
		app *application.Service
		id  domain.HoldingID
	}{{service, holdingID}, {reference, domain.HoldingID(refSaved.ID)}} {
		gain, err := side.app.HoldingGain(ctx, side.id)
		if err != nil || gain.TotalCost != nil {
			t.Fatalf("a quantity observation produced opening cost: %+v / %v", gain, err)
		}
		if origin, err := side.app.HistoryOrigin(ctx); err != nil || origin != nil {
			t.Fatalf("a quantity observation created an origin: %+v / %v", origin, err)
		}
	}
	click("Overview")
	quoteDefault := ""
	if quoted {
		label("$ 305.00")
		quoteDefault = "40"
	} else {
		label("Incomplete valuation")
		if _, err := refHistory.StartHistory(ctx, "Asia/Singapore"); err == nil {
			t.Fatal("Wails started History without an explicit cost or a quote")
		} else if issue, ok := apierror.Parse(err.Error()); !ok || issue.Code != string(domain.ErrCostBasisRequired) {
			t.Fatalf("missing quote fallback returned %v", err)
		}
	}
	click("Ledger activity")
	click("Start recording history")
	if page.startPoint.cost.Text() != "" || page.startPoint.holding == nil || page.startPoint.holding.UnitCost != quoteDefault {
		t.Fatal("market quote was used to fill original cost, or quote fixture was unavailable")
	}
	if plan, err := service.PreviewHistoryStart(ctx, "Asia/Singapore", "2026-11-22", holdingID, ""); err == nil {
		t.Fatalf("blank cost received a plan: %+v", plan)
	}
	if _, err := service.PreviewHistoryStart(ctx, "Asia/Singapore", "2026-11-22", holdingID, "0"); err != nil {
		t.Fatalf("a known zero acquisition cost should be valid: %v", err)
	}
	setBankField(h, "Original cost per unit · USD", &page.startPoint.cost, "-4")
	click("Review Starting Point")
	label("Enter zero or a positive unit cost with up to eight decimal places.")
	if _, err := refHistory.StartHistoryOnDate(ctx, "Asia/Singapore", "2026-11-22", map[string]string{refSaved.ID: "-4"}); err == nil {
		t.Fatal("Wails accepted a negative original cost")
	}
	setBankField(h, "Original cost per unit · USD", &page.startPoint.cost, "12.3456")
	click("Review Starting Point")
	label("Opening cost per unit · 12.3456 USD")
	label("Harbor Brokerage · 205 USD")
	for _, side := range []struct {
		app *application.Service
		id  domain.AccountID
	}{{service, ids.account}, {reference, ref.account}} {
		if _, err := side.app.AppendAccountCashValue(ctx, side.id, "210", "USD", ""); err != nil {
			t.Fatal(err)
		}
	}
	click("Confirm Starting Point")
	label("The ledger changed. Preview this action again.")
	click("Review Starting Point")
	label("Harbor Brokerage · 210 USD")
	startReview := page.startPoint.review.plan
	click("Confirm Starting Point")
	label("Could not verify the Starting Point. Retry checks the saved state.")
	click("Retry")
	label("No changes have been recorded yet.")
	if start.writes.Load() != 2 {
		t.Fatal("Starting Point verification issued another write")
	}
	origin := page.reads.references.Origin
	wailsOrigin, err := refHistory.StartHistoryOnDate(ctx, "Asia/Singapore", "2026-11-22", map[string]string{refSaved.ID: "12.3456"})
	if err != nil || origin == nil || !origin.StartedAt.Equal(accountTestClock) ||
		wire.FormatTime(origin.StartedAt) != wailsOrigin.StartedAt || origin.Timezone != wailsOrigin.Timezone {
		t.Fatalf("different origins: Gio %+v / Wails %+v / %v", origin, wailsOrigin, err)
	}
	assertOpeningState(t, service, storage, ids, origin.ID, holdingID, false, quoted)
	assertOpeningState(t, reference, refStorage, ref, domain.HistoryOriginID(wailsOrigin.ID), domain.HoldingID(refSaved.ID), false, quoted)
	if _, err := oldPlan.Confirm(ctx, service); err == nil {
		t.Fatal("a pre-History plan was reusable after starting History")
	}
	if _, err := history.PreviewOpeningPosition(ctx, service, ids.account, ids.security, "1"); err == nil {
		t.Fatal("new opening-quantity preview after starting History")
	}
	click("Accounts")
	click("Harbor Brokerage")
	if findPageLabel(&h.router, "Record existing position") != nil {
		t.Fatal("the opening observation remained available after History started")
	}

	now = now.Add(2 * time.Minute)
	for _, side := range []struct {
		app *application.Service
		ids openingFixture
		id  domain.HoldingID
	}{{service, ids, holdingID}, {reference, ref, domain.HoldingID(refSaved.ID)}} {
		if _, err := historyapi.NewService(side.app).RecordChange(ctx, historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeTrade, Side: "sell", SettlementAccountID: side.ids.account.String(),
			InstrumentID: side.ids.security.String(), HoldingID: side.id.String(), Quantity: "1.25",
			Gross: "25", GrossCurrency: "USD", Fee: "1", FeeCurrency: "USD",
			EffectiveLocalDate: "2026-11-22", EffectiveLocalTime: "01:01",
		}); err != nil {
			t.Fatal(err)
		}
	}
	assertOpeningState(t, reference, refStorage, ref, domain.HistoryOriginID(wailsOrigin.ID), domain.HoldingID(refSaved.ID), true, quoted)
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, session := reopenOnboardingProfile(t, paths)
	reopened.SetClock(clock)
	if again, err := reopened.ConfirmHistoryStart(ctx, startReview); err != nil || again.ID != origin.ID {
		t.Fatalf("reopened singleton returned %+v / %v", again, err)
	}
	assertOpeningState(t, reopened, session, ids, origin.ID, holdingID, true, quoted)
}

func assertOpeningState(t *testing.T, app *application.Service, session *pilot.Session, ids openingFixture, origin domain.HistoryOriginID, holding domain.HoldingID, sold, quoted bool) {
	t.Helper()
	ctx := t.Context()
	data, err := sqlite.NewRepository(session.DB).HistoryOriginData(ctx, origin)
	if err != nil || len(data.Components) != 2 {
		t.Fatalf("unexpected opening facts: %+v / %v", data.Components, err)
	}
	for _, c := range data.Components {
		if c.AccountID == nil || *c.AccountID != ids.account {
			t.Fatalf("wrong opening account: %+v", c)
		}
		switch c.Kind {
		case domain.HistoryOriginHoldingQuantity:
			if c.InstrumentID == nil || *c.InstrumentID != ids.security || c.HoldingID == nil || *c.HoldingID != holding ||
				c.Quantity == nil || c.Quantity.Canonical() != "2.5" || c.UnitCost == nil || c.UnitCost.Canonical() != "12.3456" {
				t.Fatalf("wrong opening position or original cost: %+v", c)
			}
		case domain.HistoryOriginAccountCash:
			if c.Amount == nil || c.Amount.Currency() != "USD" || c.Amount.CanonicalAmount() != "210" {
				t.Fatalf("wrong opening cash: %+v", c)
			}
		default:
			t.Fatalf("unexpected opening component: %+v", c)
		}
	}
	count, quantity, basis, cash, realized := 0, "2.5", "30.864", "210", "0"
	if sold {
		count, quantity, basis, cash, realized = 1, "1.25", "15.432", "234", "8.568"
	}
	positions, err := analytics.NewService(app).AccountHoldingDetails(ctx, ids.account.String())
	if err != nil || len(positions) != 1 || positions[0].Amounts.Quantity != quantity ||
		positions[0].Amounts.TotalCost == nil || positions[0].Amounts.TotalCost.Amount != basis {
		t.Fatalf("wrong Wails position projection: %+v / %v", positions, err)
	}
	gain, err := app.HoldingGain(ctx, holding)
	if err != nil || gain.RealizedGain == nil || gain.RealizedGain.Amount != realized {
		t.Fatalf("wrong realized trade gain: %+v / %v", gain, err)
	}
	values, err := app.AccountValuation(ctx, ids.account)
	if err != nil || !slices.ContainsFunc(values.Components, func(c domain.ValuationComponent) bool {
		return c.HoldingID == nil && c.NativeAmount == cash && c.NativeCurrency == "USD"
	}) {
		t.Fatalf("wrong cash projection: %+v / %v", values, err)
	}
	quote, err := app.CurrentInstrumentQuote(ctx, ids.security)
	if err != nil || (quoted && (quote == nil || quote.UnitPrice.Canonical() != "40")) || (!quoted && quote != nil) {
		t.Fatalf("original cost changed market quote: %+v / %v", quote, err)
	}
	if !quoted {
		overview, err := portfolio.NewService(app).Overview(ctx, account.AccountFilterRequest{})
		if err != nil || overview.Complete || !slices.ContainsFunc(overview.MissingInputs, func(input wire.MissingInputDTO) bool {
			return input.Kind == string(domain.MissingInstrumentPrice) && input.InstrumentID != nil && *input.InstrumentID == ids.security.String()
		}) {
			t.Fatalf("opening cost became a market value: %+v / %v", overview, err)
		}
	}
	a, err := app.ListActivities(ctx, 10)
	if err != nil || len(a) != count || (sold && a[0].Kind != domain.ActivitySell) {
		t.Fatalf("opening position entered activity history: %+v / %v", a, err)
	}
}

func TestStartingPointWithoutOverrideUsesSelectedQuote(t *testing.T) {
	ctx := t.Context()
	app, _, _, ids := openStartingPositionFixture(t, true)
	row, err := holdingapi.NewService(app).CreateHolding(ctx, holdingapi.CreateHoldingRequest{
		AccountID: ids.account.String(), InstrumentID: ids.security.String(), Quantity: "2.5",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := historyapi.NewService(app).StartHistory(ctx, "Asia/Singapore"); err != nil {
		t.Fatal(err)
	}
	gain, err := app.HoldingGain(ctx, domain.HoldingID(row.ID))
	if err != nil || gain.TotalCost == nil || gain.TotalCost.Amount != "100" {
		t.Fatalf("omitting original cost did not use the selected quote: %+v / %v", gain, err)
	}
}
