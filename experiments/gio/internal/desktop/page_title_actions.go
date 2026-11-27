package desktop

import (
	"fmt"
	"image"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (s *shell) pageTitleAction(gtx layout.Context, view ui.View) layout.Widget {
	switch s.activeRoute() {
	case accountsRoute:
		if p := s.accounts; p != nil {
			if row := p.accountTitleRow(); row != nil {
				return p.detailRecordButton(view.WithScrollAnchors(&s.stage), row.record.Account)
			}
			if p.canCreateBankAccount() {
				return p.createAction(view, &p.createBank, i18n.AccountCreateAdd, true)
			}
			if p.canCreateUnscoped() {
				return p.createAction(view, &p.createAccount, i18n.AccountAddEntry, true)
			}
		}
	case historyRoute:
		if p := s.historyPage; p != nil && p.canRecord() {
			return func(gtx layout.Context) layout.Dimensions { return p.layoutRecordTrigger(gtx, view) }
		}
	case fundsRoute:
		if p := s.funds; p != nil && s.trail.current().target.product == "" && len(p.productAccounts()) != 0 {
			return p.productTitleAction(view)
		}
	case portfolioRoute:
		if p := s.portfolio; p != nil {
			return p.portfolioTitleAction(view.WithScrollAnchors(&s.stage))
		}
	case marketDataRoute:
		if p := s.marketData; p != nil {
			return p.marketTitleAction(view)
		}
	case overviewRoute:
		if s.wealthHistory != nil && !ui.WidthAtLeast(gtx, gtx.Constraints.Max.X+s.chrome.sidebar, 960) {
			return s.headerDestination(view, &s.openWealthHistory, i18n.NavHistoricalOverview, wealthHistoryRoute)
		}
	case wealthHistoryRoute:
		if s.overviewPage != nil {
			return s.headerDestination(view, &s.exitWealthHistory, i18n.WealthExit, overviewRoute)
		}
	case tasksRoute:
		if p := s.tasks; p != nil && p.service != nil && p.tab == tasksUpcoming && !p.holdingRecon.active() {
			action := accountDetailButton(view, &s.refreshUpcoming, i18n.TasksRefreshUpcoming, ui.ButtonOutline, func() {
				p.invalidateTaskBadges()
				p.read()
			})
			return func(gtx layout.Context) layout.Dimensions {
				if p.request != nil {
					gtx = gtx.Disabled()
				}
				return action(gtx)
			}
		}
	}
	return nil
}

// accountHeaderActions places the secondary account menu at the trailing
// edge and leaves the remaining header width to navigation, title and record.
func (s *shell) accountHeaderActions(gtx layout.Context, view ui.View) layout.Context {
	p := s.accounts
	if s.activeRoute() != accountsRoute || p == nil || p.target.account == "" {
		return gtx
	}
	row := p.accountTitleRow()
	if row == nil {
		return gtx
	}
	menu, d := &s.accountTitleMenu, &p.detail
	if m := p.maintenance; m != nil && m.focus == &menu.Trigger && gtx.Enabled() {
		gtx.Execute(key.FocusCmd{Tag: m.focus})
		m.focus = nil
	}
	account := row.record.Account
	var choices []ui.Choice
	if d.openAnalysis != nil {
		choices = append(choices, ui.Choice{ID: string(returnAnalysisRoute), Label: view.Language.Text(i18n.AccountDetailReturns)},
			ui.Choice{ID: string(assetChangesRoute), Label: view.Language.Text(i18n.AccountDetailChanges)})
	}
	if d.openActivityList != nil {
		choices = append(choices, ui.Choice{ID: string(historyRoute), Label: view.Language.Text(i18n.AccountDetailTransactions)})
	}
	if p.maintenance != nil && accountMaintenanceSupported(account) {
		choices = append(choices, ui.Choice{ID: string(settingsRoute), Label: view.Language.Text(i18n.AccountMaintenanceEdit)})
	}
	if len(choices) == 0 {
		return gtx
	}
	side := gtx.Dp(ui.ControlSmall)
	x := gtx.Constraints.Max.X - gtx.Dp(titlebarRightPadding) - side
	if x <= 0 {
		return gtx
	}
	y := (gtx.Constraints.Max.Y - side) / 2
	origin := image.Pt(s.chrome.split+x, s.chrome.center-side/2)
	if !s.titlebar.active {
		origin.Y = y
	}
	bounds := image.Rectangle{Min: origin, Max: origin.Add(image.Pt(side, side))}
	label := fmt.Sprintf(view.Language.Text(i18n.AccountTitleActions), account.Name)
	menu.Menu(gtx, view, bounds, choices, label, "", &d.record.trigger, &s.headingFocus, 220, func(id string) {
		switch route := routeID(id); route {
		case returnAnalysisRoute, assetChangesRoute:
			d.openAnalysis(route, account.ID, account.Name, &menu.Trigger)
		case historyRoute:
			d.openActivityList(account.ID, &menu.Trigger)
		case settingsRoute:
			p.maintenance.startEdit(row.record, &menu.Trigger)
			gtx.Execute(op.InvalidateCmd{})
		}
	})
	offset := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	buttonContext := gtx
	buttonContext.Constraints = layout.Exact(bounds.Size())
	view.ButtonOf(&menu.Trigger, ui.ButtonSpec{Variant: ui.ButtonGhost, Size: ui.ButtonSmall, Icon: lucide.Ellipsis, Name: label})(buttonContext)
	offset.Pop()
	gtx.Constraints.Max.X -= side + gtx.Dp(ui.Space4)
	gtx.Constraints.Min.X = min(gtx.Constraints.Min.X, gtx.Constraints.Max.X)
	return gtx
}

func (p *accountsPage) accountTitleRow() *accountRow {
	if p.productSetup != nil || p.detail.record.open || p.detail.cashActions.active() ||
		p.statement != nil && (p.statement.screen != nil || p.target.statement != "" || p.statement.restore.open) ||
		p.maintenance != nil && p.maintenance.draft != nil ||
		p.liabilityPosting != nil && p.liabilityPosting.draft != nil ||
		p.cashBalance != nil && p.cashBalance.draft != nil ||
		p.balanceTarget != nil && p.balanceTarget.draft != nil ||
		p.opening != nil && p.opening.draft != nil ||
		p.investment != nil && p.investment.draft != nil {
		return nil
	}
	return p.freshDetail()
}

func (s *shell) headerDestination(view ui.View, button *widget.Clickable, label i18n.MessageID, target routeID) layout.Widget {
	return accountDetailButton(view, button, label, ui.ButtonOutline, func() {
		s.pendingNavigation = &navigationRequest{target: navigationTarget{route: target}, focus: button}
	})
}

func (p *accountsPage) createAction(view ui.View, button *widget.Clickable, label i18n.MessageID, primary bool) layout.Widget {
	variant := ui.ButtonOutline
	if primary {
		variant = ui.ButtonPrimary
	}
	return func(gtx layout.Context) layout.Dimensions {
		if p.focusCreate == button && gtx.Enabled() {
			gtx.Execute(key.FocusCmd{Tag: button})
			p.focusCreate = nil
		}
		return view.ButtonOf(button, ui.ButtonSpec{Variant: variant, Title: view.Language.Text(label)})(gtx)
	}
}

func (p *availableFundsPage) productTitleAction(view ui.View) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if p.request != nil {
			gtx = gtx.Disabled()
		}
		if gtx.Enabled() && p.setup == nil && p.addProduct.Clicked(gtx) {
			p.productSaved = false
			run := productSetupRuntime{ctx: p.ctx, ledger: p.productLedger, clock: p.productClock, workers: &p.workers, invalidate: p.invalidate}
			p.setup = newProductSetup(run, p.productAccounts(), view.Language.Text(i18n.ProductAddDeposit))
			gtx.Execute(op.InvalidateCmd{})
		}
		if p.restoreAddFocus && gtx.Enabled() {
			gtx.Execute(key.FocusCmd{Tag: &p.addProduct})
			p.restoreAddFocus = false
		}
		return view.PrimaryButton(&p.addProduct, view.Language.Text(i18n.ProductAddAction))(gtx)
	}
}

func (p *portfolioPage) portfolioTitleAction(view ui.View) layout.Widget {
	if p.canImportPosition() && p.investment.draft == nil {
		f := p.investment
		return func(gtx layout.Context) layout.Dimensions {
			if gtx.Enabled() && f.importOpen.Clicked(gtx) {
				f.draft = newAccountInvestmentDraft(domain.ActivityPositionTransfer, domain.Account{})
				f.draft.pickAccount, f.draft.focusAccount = true, true
				f.draft.accounts = p.current.importAccounts
				view.RestorePageScroll(0)
				gtx.Execute(op.InvalidateCmd{})
			}
			if f.restore == &f.importOpen && gtx.Enabled() {
				gtx.Execute(key.FocusCmd{Tag: &f.importOpen})
				f.restore = nil
			}
			return view.PrimaryButton(&f.importOpen, view.Language.Text(i18n.PositionImportPortfolioAction))(gtx)
		}
	}
	c := &p.catalog
	if p.id != "" || p.tabs.active != portfolioInstruments || c.ledger == nil || p.current == nil {
		return nil
	}
	return func(gtx layout.Context) layout.Dimensions {
		if p.request != nil || p.err != nil {
			gtx = gtx.Disabled()
		}
		if gtx.Enabled() {
			if c.add.Clicked(gtx) {
				p.beginCatalogEditor(gtx, domain.Instrument{QuoteCurrency: p.current.summary.Currency}, &c.add)
			}
			if c.focus == &c.add {
				gtx.Execute(key.FocusCmd{Tag: &c.add})
				c.focus = nil
			}
		}
		return view.PrimaryButton(&c.add, view.Language.Text(i18n.PortfolioCatalogAdd))(gtx)
	}
}

func (p *marketDataPage) marketTitleAction(view ui.View) layout.Widget {
	if p.focus.active() {
		return nil
	}
	if p.tab == marketFX && p.manual.service != nil {
		return accountDetailButton(view, &p.manual.addFX, i18n.MarketManualFX, ui.ButtonPrimary, func() {
			p.openManualQuote(marketManualTarget{}, &p.manual.addFX)
		})
	}
	if p.tab != marketInstruments || p.openCatalogEditor == nil {
		return nil
	}
	return func(gtx layout.Context) layout.Dimensions {
		if p.request != nil || p.err != nil || p.current[marketInstruments] == nil {
			gtx = gtx.Disabled()
		}
		if gtx.Enabled() && p.addInstrument.Clicked(gtx) {
			p.catalogNotice = i18n.NoMessage
			p.openCatalogEditor(domain.Instrument{}, &p.addInstrument)
			gtx.Execute(op.InvalidateCmd{})
		}
		return view.PrimaryButton(&p.addInstrument, view.Language.Text(i18n.PortfolioCatalogAdd))(gtx)
	}
}
