package desktop

import (
	"context"
	"image"
	"time"

	"gioui.org/app"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/preferences"
	"github.com/waltwang/nestworth-go/internal/application"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
)

// Run serves the native window and joins pending work before returning to the database owner.
func Run(ctx context.Context, service *application.Service, prefs *preferences.Service, clock func() time.Time, wordmark, mark image.Image) error {
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
	overview := newOverviewPage(ctx, service, clock, window.Invalidate)
	portfolio := newPortfolioPage(ctx, service, window.Invalidate)
	funds := newAvailableFundsPage(ctx, service, window.Invalidate)
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
		overview.workers.Wait()
		portfolio.workers.Wait()
		funds.workers.Wait()
		accounts.workers.Wait()
		onboarding.workers.Wait()
	}()
	shell := newShell(wordmark, mark, ledgerPages{directory: directory, history: timeline, overview: overview, funds: funds, accounts: accounts, portfolio: portfolio, prefs: state,
		version: service.MobileDataVersion})
	shell.onboarding = onboarding
	return shell.run(window)
}

func (s *shell) run(window *app.Window) error {
	window.Option(app.Title(pilot.DisplayName), app.Size(1120, 840), app.MinSize(700, 460))
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
			gtx := ax.Begin(app.NewContext(&ops, event))
			s.layout(gtx)
			event.Frame(gtx.Ops)
			ax.Publish(gtx)
		}
	}
}
