package desktop

import (
	"context"
	"image"
	"math"
	"time"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/preferences"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
)

// Run serves the native window and joins pending work before returning to the database owner.
func Run(ctx context.Context, service *application.Service, prefs *preferences.Service, receipts *pilot.CashReceiptStore, clock func() time.Time, wordmark, mark image.Image) error {
	ctx, cancel := context.WithCancel(ctx)
	window := new(app.Window)
	state := newPreferenceState(ctx, prefs, window.Invalidate)
	state.setLanguage = service.SetUILanguage
	stopWatching := preferences.WatchSystemChanges(state.systemUpdate)
	state.reload()
	directory := newDirectoryPage(
		newMembersList(ctx, service, window.Invalidate),
		newInstitutionsPage(ctx, service, window.Invalidate),
		newGroupsList(ctx, service, window.Invalidate),
	)
	timeline := newLedgerHistory(ctx, service, clock, window.Invalidate)
	timeline.reads.repeatAPI = historyapi.NewService(service)
	recovery := &cashReceiptRecovery{ctx: ctx, store: receipts, reader: service, invalidate: window.Invalidate}
	timeline.cashMovements.recovery = recovery
	recovery.read()
	overview := newOverviewPage(ctx, service, clock, window.Invalidate)
	wealthHistory := newWealthHistoryPage(ctx, service, clock, window.Invalidate)
	returns := &returnAnalysisPage{ctx: ctx, reader: service, clock: clock, invalidate: window.Invalidate}
	assets := &assetChangesPage{ctx: ctx, reader: service, clock: clock, invalidate: window.Invalidate}
	marketData := &marketDataPage{ctx: ctx, reader: service, invalidate: window.Invalidate,
		manual: marketManualQuotes{service: service, clock: clock}, latest: marketLatest{service: service}}
	healthRepair := &dataHealthRepair{service: service, latestBusy: func() bool { return marketData.latest.request != nil }}
	marketData.sync = healthRepair
	service.SetMarketDataSyncListener(func(string, application.SyncJobSnapshot) {
		healthRepair.revision.Add(1)
		window.Invalidate()
	})
	dataHealth := &dataHealthPage{ctx: ctx, reader: service, invalidate: window.Invalidate, repair: healthRepair,
		snapshots: dataHealthSnapshots{service: service}}
	portfolio := newPortfolioPage(ctx, service, window.Invalidate)
	tasks := newTasksPage(ctx, service, window.Invalidate)
	tasks.checks = service
	funds := newAvailableFundsPage(ctx, service, window.Invalidate)
	funds.reservationLedger = service
	funds.productPage = newProductDetailPage(ctx, service, window.Invalidate)
	funds.productPage.receipts, funds.productPage.clock = service, clock
	accounts := newAccountsPage(ctx, service, clock, window.Invalidate)
	accounts.creation = &balanceAccountCreation{service: service}
	accounts.maintenance = &accountMaintenance{service: service}
	accounts.cashBalance = &accountCashBalance{service: service}
	accounts.investment = &accountInvestment{service: service}
	accounts.opening = &accountOpeningPosition{service: service}
	accounts.balanceTarget = &accountBalanceTarget{service: service}
	accounts.statement = &accountStatement{service: reconciliation.NewService(service), origin: service}
	onboarding := newOnboardingPage(ctx, service, clock, window.Invalidate)
	onboarding.read()
	defer func() {
		stopWatching()
		cancel()
		state.wait()
		directory.wait()
		timeline.model.Wait()
		timeline.reads.workers.Wait()
		timeline.cashMovements.workers.Wait()
		recovery.workers.Wait()
		overview.workers.Wait()
		wealthHistory.workers.Wait()
		returns.workers.Wait()
		assets.workers.Wait()
		marketData.workers.Wait()
		dataHealth.workers.Wait()
		service.CancelMarketDataSyncAndWait()
		service.SetMarketDataSyncListener(nil)
		portfolio.workers.Wait()
		tasks.workers.Wait()
		funds.workers.Wait()
		funds.productPage.workers.Wait()
		accounts.workers.Wait()
		onboarding.workers.Wait()
	}()
	shell := newShell(wordmark, mark, ledgerPages{directory: directory, history: timeline, overview: overview, wealthHistory: wealthHistory, returns: returns, assets: assets, marketData: marketData, dataHealth: dataHealth, funds: funds, tasks: tasks, accounts: accounts, portfolio: portfolio, prefs: state,
		version: service.MobileDataVersion})
	shell.onboarding = onboarding
	return shell.run(window, prefs)
}

func (s *shell) run(window *app.Window, prefs *preferences.Service) error {
	initial := prefs.WindowSize()
	window.Option(app.Title(pilot.DisplayName), app.Size(unit.Dp(initial.X), unit.Dp(initial.Y)),
		app.MinSize(settings.MinWindowWidth, settings.MinWindowHeight))

	pending := make(chan image.Point, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for size := range pending {
			prefs.RememberWindowSize(s.prefs.ctx, size)
		}
	}()
	var lastSize image.Point
	defer func() {
		close(pending)
		<-finished
		// A Settings reset already in progress may have restored the defaults.
		// Flush the actual closing size after that write completes.
		s.prefs.wait()
		if lastSize != (image.Point{}) {
			prefs.RememberWindowSize(s.prefs.ctx, lastSize)
		}
	}()

	ax := accessibility.New(window)
	defer ax.Close()
	var ops op.Ops
	for {
		switch event := window.Event().(type) {
		case app.DestroyEvent:
			return event.Err
		case app.ViewEvent:
			ax.View(event)
		case app.ConfigEvent:
			if !event.Config.Focused && s.overlay.Trigger != nil {
				s.overlay.Close()
				window.Invalidate()
			}
		case app.FrameEvent:
			size := image.Pt(int(math.Round(float64(event.Metric.PxToDp(event.Size.X)))),
				int(math.Round(float64(event.Metric.PxToDp(event.Size.Y)))))
			if size != lastSize && preferences.ValidWindowSize(size) {
				lastSize = size
				// Keep the newest resize queued without blocking the render loop.
				select {
				case <-pending:
				default:
				}
				pending <- size
			}
			gtx := ax.Begin(app.NewContext(&ops, event))
			s.layout(gtx)
			event.Frame(gtx.Ops)
			ax.Publish(gtx)
		}
	}
}
