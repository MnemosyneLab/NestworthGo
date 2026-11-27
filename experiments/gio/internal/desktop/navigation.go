package desktop

import (
	"fmt"
	"slices"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
	"github.com/waltwang/nestworth-go/internal/domain"
)

const navigationLimit, recentLimit = 40, 20

type navigationTarget struct {
	route       routeID
	object      directoryRef
	account     domain.AccountID
	institution domain.InstitutionID
	activity    domain.ActivityID
	instrument  domain.InstrumentID
	product     domain.ProductContractID
	statement   string
	health      healthFindingFocus
	accountList accountListScope
	fundsDay    string
	fundsAction fundsActionFocus

	portfolioAccount domain.AccountID
	instrumentData   bool
}

func (t navigationTarget) hasObject() bool {
	return t.object != (directoryRef{}) || t.account != "" || t.activity != "" || t.instrument != "" || t.product != ""
}

type navigationEntry struct {
	id               uint64
	target           navigationTarget
	name             string
	focus            event.Tag
	scroll           layout.Position
	directory        directoryVisit
	history          historyTimelineVisit
	historyFilters   history.Filters
	historyDetail    historyDetailState
	accountsArchived bool
	accountPositions accountPositionsVisit

	accountComposition accountCompositionVisit

	accountStatement statementVisit
	funds            fundsViewState
	portfolioSearch  string
	holdings         portfolioHoldingsVisit
	catalog          portfolioCatalogVisit
	portfolioTabs    portfolioTabsVisit
	instrumentDetail instrumentDetailVisit
	tasks            tasksVisit
	asset            assetChangesVisit
	returns          *returnAnalysisVisit
	marketData       marketDataVisit
	quoteHistory     *marketQuoteHistoryVisit
	healthScroll     layout.Position
	settingsSection  settingsSection
	visited          bool
}

type navigationTrail struct {
	entries []navigationEntry
	cursor  int
	recent  []navigationEntry
	nextID  uint64
}

func newNavigationTrail(route routeID) navigationTrail {
	return navigationTrail{entries: []navigationEntry{{id: 1, target: navigationTarget{route: route}}}, nextID: 1}
}

func (n *navigationTrail) current() *navigationEntry { return &n.entries[n.cursor] }

func (n *navigationTrail) adjacent(step int) *navigationEntry {
	if i := n.cursor + step; i >= 0 && i < len(n.entries) {
		return &n.entries[i]
	}
	return nil
}

func (n *navigationTrail) push(entry navigationEntry) {
	n.entries = slices.Delete(n.entries, n.cursor+1, len(n.entries))
	if len(n.entries) == navigationLimit {
		n.entries = slices.Delete(n.entries, 0, 1)
	}
	n.nextID++
	entry.id = n.nextID
	n.entries = append(n.entries, entry)
	n.cursor = len(n.entries) - 1
}

func (n *navigationTrail) rename(target navigationTarget, name string) {
	for _, entries := range [][]navigationEntry{n.entries, n.recent} {
		for i := range entries {
			if entries[i].target == target {
				entries[i].name = name
			}
		}
	}
}

func sameRecentDestination(a, b navigationEntry) bool {
	if a.target != b.target {
		return false
	}
	if a.target.route == accountsRoute && !a.target.hasObject() {
		return a.accountsArchived == b.accountsArchived
	}
	if a.target.route == tasksRoute {
		return a.tasks.tab == b.tasks.tab && a.tasks.reconciliationAccount == b.tasks.reconciliationAccount
	}
	if a.target.route == assetChangesRoute {
		return a.asset.tab == b.asset.tab && a.asset.selection == b.asset.selection
	}
	if a.target.route == returnAnalysisRoute && a.returns != nil && b.returns != nil {
		return a.returns.tab == b.returns.tab && a.returns.selection == b.returns.selection &&
			a.returns.month.Equal(b.returns.month) && a.returns.year == b.returns.year && a.returns.metric == b.returns.metric
	}
	if a.target.route == marketDataRoute {
		return a.marketData.tab == b.marketData.tab && strings.EqualFold(strings.TrimSpace(a.marketData.search), strings.TrimSpace(b.marketData.search))
	}
	if a.target.route == historyRoute && a.target.activity == "" {
		return a.historyFilters.Equal(b.historyFilters)
	}
	if a.target.route != directoryRoute || a.target.hasObject() {
		return true
	}
	return a.directory.tab == b.directory.tab &&
		a.directory.list.includeArchived == b.directory.list.includeArchived &&
		normalizeDirectoryQuery(a.directory.list.query) == normalizeDirectoryQuery(b.directory.list.query)
}

func (n *navigationTrail) recordRecent(entry navigationEntry) {
	n.recent = slices.DeleteFunc(n.recent, func(previous navigationEntry) bool {
		return previous.id == entry.id || sameRecentDestination(previous, entry)
	})
	n.recent = slices.Insert(n.recent, 0, entry)
	if len(n.recent) > recentLimit {
		n.recent = slices.Delete(n.recent, recentLimit, len(n.recent))
	}
}

type navigationRequest struct {
	target               navigationTarget
	name                 string
	focus                event.Tag
	step                 int
	recent               *navigationEntry
	accountsArchived     bool
	providerSettings     bool
	settingsProvider     string
	verifyProductReceipt bool
	tasksTab             tasksTab
	tasksSession         string
	tasksAccount         domain.AccountID
	asset                assetChangesVisit
	returns              *returnAnalysisVisit
	historyFilters       history.Filters
	historyAction        historyRowAction
	maintenanceTab       bool
	catalogEditor        *domain.Instrument
}

func (s *shell) activeRoute() routeID { return s.trail.current().target.route }

func (s *shell) navigate(gtx layout.Context, request navigationRequest) {
	if !gtx.Enabled() || s.settingsPage.confirming || s.confirmingArchive() || s.accountCreationOpen() || s.accountMaintenanceBlocking() || s.startReconciliationReviewOpen() || s.fundsReservationOpen() || s.fundsPolicyOpen() || s.fundsProductSetupOpen() || s.fundsProductTermsOpen() {
		return
	}
	if s.historyPage != nil && (s.historyPage.cashMovements.writing() || s.historyPage.amendmentUnresolved() || s.historyPage.repeatUnresolved() || s.historyPage.investment.unresolved()) {
		return
	}
	if s.accounts != nil && (s.accounts.detail.cashActions.unresolved() || s.accounts.cashBalanceUnresolved() || s.accounts.balanceTargetUnresolved() || s.accounts.liabilityPostingUnresolved() || s.accounts.statementUnresolved() || s.accounts.investment.unresolved() || s.accounts.productSetup.unresolved()) {
		return
	}
	if request.step == -1 && s.accounts != nil && s.activeRoute() == accountsRoute && s.accounts.productSetup != nil {
		if d := s.accounts.productSetup; d.plan != nil {
			d.plan, d.focus = nil, true
		} else {
			s.accounts.cancelProductSetup()
		}
		s.stage.Position = layout.Position{}
		gtx.Execute(op.InvalidateCmd{})
		return
	}
	if s.tasks != nil && s.tasks.holdingRecon.unresolved() {
		return
	}
	if s.portfolio != nil && (s.portfolio.investment.unresolved() ||
		s.portfolio.catalog.draft != nil && s.portfolio.catalog.draft.pending != nil) {
		return
	}
	if s.funds != nil && (s.funds.reservationUnresolved() || s.funds.setup.unresolved() || s.funds.productPage.unresolved() || s.funds.policyUnresolved()) {
		return
	}
	entry := s.trail.current()
	if request.step != 0 {
		if s.trail.adjacent(request.step) == nil {
			return
		}
	} else {
		request = resolveMaintenanceTarget(request)
		item := s.routes[request.target.route]
		if item == nil || !item.available() || (request.recent == nil && entry.target == request.target) {
			return
		}
		if request.target.object != (directoryRef{}) && (request.target.route != directoryRoute || s.directory == nil) {
			return
		}
		if (request.target.account != "" || request.target.institution != "" || request.target.accountList.active()) &&
			(request.target.route != accountsRoute || s.accounts == nil) {
			return
		}
		if request.target.accountList.active() && !validAccountTarget(request.target) {
			return
		}
		if request.target.statement != "" && (request.target.route != accountsRoute || request.target.account == "" || s.accounts.statement == nil) {
			return
		}
		if request.target.activity != "" && (request.target.route != historyRoute || s.historyPage == nil || s.historyPage.reads == nil) {
			return
		}
		if request.target.product != "" && (request.target.route != fundsRoute || s.funds == nil || s.funds.productPage == nil) {
			return
		}
		if request.target.instrument != "" && request.target.route != portfolioRoute &&
			(request.target.route != historyRoute || request.target.activity != "" || s.historyPage == nil || s.historyPage.reads == nil) {
			return
		}
		if request.target.portfolioAccount != "" && (request.target.route != portfolioRoute || request.target.instrument == "") {
			return
		}
	}

	source := *entry
	if request.focus != nil {
		source.focus = request.focus
	}
	s.saveVisit(&source)
	if request.recent != nil && sameRecentDestination(source, *request.recent) {
		return
	}
	*entry = source
	s.trail.recordRecent(source)
	if entry.target.route == directoryRoute && entry.target.hasObject() {
		s.directory.object.leave()
	}
	if entry.target.route == historyRoute && s.historyPage != nil && s.historyPage.reads != nil {
		if entry.target.activity != "" {
			s.historyPage.reads.leaveDetail()
		} else {
			s.historyPage.reads.closeRowActions()
		}
	}
	if entry.target.route == historyRoute && s.historyPage != nil && s.historyPage.investment != nil {
		s.historyPage.investment.draft, s.historyPage.investment.restore = nil, nil
	}
	if entry.target.route == accountsRoute {
		s.accounts.leave()
		s.accounts.detail.cashActions.leave()
		s.accounts.detail.record.open, s.accounts.detail.record.restore = false, false
		s.accounts.productSetup = nil
		s.accounts.creation.discard()
		if cash := s.accounts.cashBalance; cash != nil {
			cash.draft = nil
		}
		if balance := s.accounts.balanceTarget; balance != nil {
			balance.draft = nil
		}
		if posting := s.accounts.liabilityPosting; posting != nil {
			posting.draft = nil
			posting.restore = false
		}
		if investment := s.accounts.investment; investment != nil {
			investment.draft = nil
			investment.restore = nil
		}
		if statement := s.accounts.statement; statement != nil {
			statement.screen = nil
		}
		if m := s.accounts.maintenance; m != nil {
			m.draft, m.focus, m.issue, m.success = nil, nil, i18n.NoMessage, i18n.NoMessage
		}
		if entry.target.account == s.accounts.createdID {
			s.accounts.createdID = ""
		}
	}
	if entry.target.route == fundsRoute {
		s.funds.reservation, s.funds.setup = nil, nil
		s.funds.policy = nil
		if entry.target.product == "" {
			s.funds.leave()
		} else {
			s.funds.productPage.leave()
			s.funds.productPage.receipt, s.funds.productPage.valuation = nil, nil
		}
	}
	if entry.target.route == tasksRoute {
		s.tasks.holdingRecon.discard()
		s.tasks.leave()
	}
	if entry.target.route == portfolioRoute {
		if f := s.portfolio.investment; f != nil {
			f.draft, f.restore = nil, nil
		}
		s.portfolio.catalog.cancelDraft()
		s.portfolio.leave()
	}
	if entry.target.route == overviewRoute && s.overviewPage != nil {
		s.overviewPage.leave()
	}
	if entry.target.route == wealthHistoryRoute {
		s.wealthHistory.suspend()
	}
	if entry.target.route == returnAnalysisRoute {
		s.returns.leave()
	}
	if entry.target.route == assetChangesRoute {
		s.assets.leave()
	}
	if entry.target.route == marketDataRoute {
		s.marketData.closeMarketOperation()
		s.marketData.leave()
	}
	if entry.target.route == dataHealthRoute {
		s.dataHealth.leave()
		s.dataHealth.snapshots.restore = nil
		if repair := s.dataHealth.repair; repair != nil {
			repair.retirePreview()
		}
	}
	if entry.target.route == settingsRoute {
		s.settingsPage.credential.clearInput()
	}
	if request.step != 0 {
		s.trail.cursor += request.step
		s.restoreVisit(s.trail.current())
	} else if request.recent != nil {
		s.trail.push(*request.recent)
		s.restoreVisit(s.trail.current())
	} else {
		s.trail.push(navigationEntry{target: request.target, name: request.name, accountsArchived: request.accountsArchived,
			tasks: tasksVisit{tab: request.tasksTab, holdingSession: request.tasksSession, reconciliationAccount: request.tasksAccount}, asset: request.asset, returns: request.returns, historyFilters: request.historyFilters})
		entry = s.trail.current()
		if request.maintenanceTab {
			entry.focus = request.focus
			if entry.target.route == marketDataRoute {
				entry.marketData = s.marketData.visit()
			}
		}
		if entry.target.route == marketDataRoute {
			// Switching a top-level tab can reuse search and the inner tab.
			// Only a link that names a finding applies its scope to a new visit.
			entry.marketData.focus = entry.target.health
			if marketFindingFXPair(entry.marketData.focus) != (marketFXPair{}) {
				entry.marketData.tab = marketFX
			}
		}
		if request.catalogEditor != nil {
			entry.portfolioTabs.active = portfolioInstruments
		}
		if entry.target.instrumentData {
			window := marketQuoteFindingWindow(entry.target.health)
			entry.instrumentDetail.section = instrumentDetailData
			entry.instrumentDetail.actionDates = window
			for i := range entry.instrumentDetail.histories {
				entry.instrumentDetail.histories[i].window = window
			}
		}
	}
	entry = s.trail.current()
	if entry.target.route == directoryRoute && entry.target.hasObject() {
		ref := entry.target.object
		list := s.directory.objectList(ref)
		if list == nil {
			s.trail.rename(entry.target, "")
		}
		s.directory.object.enter(ref, list)
	}
	if entry.target.route == accountsRoute {
		s.accounts.filterLabel = entry.name
		s.accounts.enter(entry.target, entry.accountsArchived, entry.scroll)
		s.accounts.positions.visit = entry.accountPositions.copy()
		s.accounts.detail.restore(entry.accountComposition)
		if statement := s.accounts.statement; statement != nil {
			statement.restore = entry.accountStatement
		}
	}
	if entry.target.route == fundsRoute {
		if entry.target.product == "" {
			state := entry.funds
			if !entry.visited {
				state = s.funds.state
				state.focusSources = false
				if day := entry.target.fundsDay; day != "" {
					state.horizonOn, state.filter = day, fundsNeedsRules
					state.includeEarly, state.focusSources = false, true
				}
			}
			s.funds.enter(entry.scroll, state)
			if !entry.visited {
				s.funds.actionFocus = entry.target.fundsAction
			}
		} else {
			s.funds.productPage.enter(entry.target.product, entry.scroll, request.verifyProductReceipt)
		}
	}
	if entry.target.route == tasksRoute {
		if focus := s.tasks.enter(entry.tasks, entry.scroll.Offset); focus != nil {
			entry.focus = focus
		}
	}
	if entry.target.route == portfolioRoute {
		if entry.target.instrument == "" {
			s.portfolio.restoreHoldingsVisit(entry.holdings)
			s.portfolio.catalog.restore(entry.catalog)
			s.portfolio.restorePortfolioTabs(entry.portfolioTabs)
		} else {
			s.portfolio.detail.restoreVisit(entry.instrumentDetail)
		}
		s.portfolio.enterScoped(entry.target.instrument, entry.target.portfolioAccount, entry.portfolioSearch, entry.scroll)
		if request.catalogEditor != nil {
			s.portfolio.requestCatalogEditor(*request.catalogEditor)
		}
	}
	if entry.target.route == wealthHistoryRoute {
		s.wealthHistory.enter()
	}
	if entry.target.route == returnAnalysisRoute {
		if entry.returns != nil {
			s.returns.restore(*entry.returns)
		}
		s.returns.enter()
	}
	if entry.target.route == assetChangesRoute {
		s.assets.enter(entry.asset)
	}
	if entry.target.route == marketDataRoute {
		s.marketData.enter(entry.marketData)
		if entry.quoteHistory != nil {
			s.marketData.openQuoteHistory(*entry.quoteHistory)
			entry.quoteHistory = nil
		}
	}
	if entry.target.route == dataHealthRoute {
		s.dataHealth.enter(entry.healthScroll, entry.target.health)
	}
	if entry.target.route == settingsRoute && request.providerSettings {
		s.settingsPage.active = settingsMarket
		s.settingsPage.credential.reveal = request.settingsProvider == domain.CoinGeckoProviderKey
	}
	if entry.target.route == historyRoute && s.historyPage != nil {
		if entry.target.activity != "" {
			entry.focus = s.historyPage.reads.enterDetail(entry.target.activity, entry.historyDetail)
			s.historyPage.reads.detail.requested = request.historyAction
		} else {
			filters := entry.historyFilters
			if entry.target.instrument != "" {
				filters.Instrument = entry.target.instrument
			}
			s.historyPage.enterTimeline(entry.id, entry.visited, entry.history, filters)
			if entry.history.focused != "" && s.historyPage.reads != nil {
				entry.focus = s.historyPage.detailButton(entry.history.focused)
				if entry.history.focusActions {
					entry.focus = s.historyPage.historyActionButton(entry.history.focused)
				}
			}
			if s.historyPage.reads != nil && source.target.route != historyRoute {
				s.historyPage.reads.loadReferences()
			}
		}
	}
	s.narrowExpanded = false
	s.restoreFocus = request.step != 0 || request.recent != nil || entry.target.hasObject() || request.maintenanceTab
	if s.restoreFocus && s.hasMaintenanceTabs() {
		for i := range s.maintenanceTabs {
			tab := &s.maintenanceTabs[i]
			if gtx.Focused(tab) && entry.focus != tab {
				// A shared tab is still mounted across both visits. Its old
				// focus must not override the destination's saved control.
				gtx.Execute(key.FocusCmd{})
				break
			}
		}
	}
	gtx.Execute(op.InvalidateCmd{})
}

func (s *shell) updateNavigation(gtx layout.Context) {
	if s.pendingNavigation != nil {
		request := *s.pendingNavigation
		s.pendingNavigation = nil
		s.navigate(gtx, request)
		return
	}
	switch {
	case s.back.Clicked(gtx):
		s.navigate(gtx, navigationRequest{step: -1})
		return
	case s.forward.Clicked(gtx):
		s.navigate(gtx, navigationRequest{step: 1})
		return
	}
	for _, group := range s.groups {
		for i := range group.items {
			item := &group.items[i]
			if item.available() && item.button.Clicked(gtx) {
				s.navigate(gtx, navigationRequest{target: navigationTarget{route: item.id}})
				return
			}
		}
	}
}

func (s *shell) saveVisit(entry *navigationEntry) {
	entry.visited = true
	switch entry.target.route {
	case accountsRoute:
		entry.accountsArchived = s.accounts.archived
		entry.accountPositions = s.accounts.positions.visit.copy()
		entry.accountComposition = s.accounts.detail.visit()
		entry.scroll = s.accounts.list.Position
		if statement := s.accounts.statement; statement != nil {
			entry.accountStatement = statement.visit()
		}
	case fundsRoute:
		if entry.target.product == "" {
			entry.scroll = s.funds.list.Position
			entry.funds = s.funds.state
			entry.funds.focusSources = false
		} else {
			entry.scroll = s.funds.productPage.list.Position
		}
	case tasksRoute:
		entry.tasks, entry.scroll = s.tasks.visit(entry.focus), s.stage.Position
	case portfolioRoute:
		entry.portfolioSearch = s.portfolio.search.Field.Text()
		entry.scroll = s.portfolio.list.Position
		if entry.target.instrument == "" {
			entry.holdings = s.portfolio.holdingsVisit()
			entry.catalog = s.portfolio.catalog.visit()
			entry.portfolioTabs = s.portfolio.portfolioTabsVisit()
		} else {
			entry.instrumentDetail = s.portfolio.detail.savedVisit()
		}
	case overviewRoute:
		if s.overviewPage != nil {
			entry.scroll = s.overviewPage.list.Position
		}
	case directoryRoute:
		if s.directory != nil && !entry.target.hasObject() {
			entry.directory = s.directory.visit()
		}
	case settingsRoute:
		entry.scroll = s.stage.Position
		entry.settingsSection = s.settingsPage.active
	case wealthHistoryRoute:
		entry.scroll = s.stage.Position
	case returnAnalysisRoute:
		entry.scroll = s.stage.Position
		if s.returns != nil {
			visit := s.returns.visit()
			entry.returns = &visit
		}
	case assetChangesRoute:
		entry.scroll = s.stage.Position
		entry.asset = s.assets.visit()
	case marketDataRoute:
		entry.scroll = s.stage.Position
		entry.marketData = s.marketData.visit()
	case dataHealthRoute:
		entry.scroll = s.stage.Position
		entry.healthScroll = s.dataHealth.list.Position
	case historyRoute:
		if s.historyPage != nil {
			if entry.target.activity == "" {
				entry.history = s.historyPage.timelineVisit(entry.focus)
			} else if d := s.historyPage.reads.detail; d != nil {
				entry.historyDetail = d.state(entry.focus)
			}
		}
	case componentPreviewRoute:
		entry.scroll = s.componentPreview.scroll.Position
	}
}

func (s *shell) restoreVisit(entry *navigationEntry) {
	if !entry.visited {
		return
	}
	switch entry.target.route {
	case overviewRoute:
		if s.overviewPage != nil {
			s.overviewPage.list.Position = restoredPosition(entry.scroll, s.overviewPage.rowCount())
		}
	case directoryRoute:
		if s.directory != nil && !entry.target.hasObject() {
			s.directory.restore(entry.directory)
		}
	case settingsRoute:
		s.settingsPage.active = entry.settingsSection
	case componentPreviewRoute:
		s.componentPreview.scroll.Position = entry.scroll
	}
}

func (s *shell) finishPageFocus(gtx layout.Context, focus ui.PageFocus) {
	entry := s.trail.current()
	if focus.Current != nil {
		entry.focus = focus.Current
	}
	if !s.restoreFocus || !gtx.Enabled() {
		return
	}
	// A user focus change while a scope is being reloaded takes precedence.
	if focus.Current != nil && focus.Current != focus.Target {
		s.restoreFocus = false
		return
	}
	if !focus.TargetDrawn && focus.Target != nil && s.waitingForPageFocus() {
		return
	}
	target := focus.Target
	if !focus.TargetDrawn {
		if s.activeRoute() == accountsRoute && s.accounts != nil {
			p := s.accounts
			if p.revealInvestmentShortcut(target) {
				gtx.Execute(op.InvalidateCmd{})
				return
			}
			if p.canImportPosition() {
				if fallback := p.investment.holdingActionFallback(target); fallback != nil {
					entry.focus = fallback
					gtx.Execute(op.InvalidateCmd{})
					return
				}
			}
		}
		if s.activeRoute() == dataHealthRoute && s.dataHealth != nil && focus.Target != nil {
			target = &s.dataHealth.recheck
		} else {
			target = &s.headingFocus
		}
	}
	gtx.Execute(key.FocusCmd{Tag: target})
	s.restoreFocus = false
}

func (s *shell) waitingForPageFocus() bool {
	if s.activeRoute() == overviewRoute && s.overviewPage != nil {
		return s.overviewPage.loading || s.overviewPage.funds.request != nil ||
			s.overviewPage.upcoming.request != nil || s.overviewPage.upcoming.summary.request != nil ||
			s.overviewPage.healthScan.request != nil
	}
	if s.activeRoute() == historyRoute && s.historyPage != nil {
		if r := s.historyPage.reads; r != nil && r.detail != nil {
			return r.detail.pending != nil || r.detail.dirty
		}
		return s.historyPage.model.Loading()
	}
	if s.activeRoute() == accountsRoute && s.accounts != nil {
		if p := s.accounts.statement; p != nil && p.screen != nil && p.screen.loading != nil {
			return true
		}
		return s.accounts.request != nil
	}
	if s.activeRoute() == fundsRoute && s.funds != nil {
		if s.trail.current().target.product != "" {
			return s.funds.productPage.request != nil
		}
		return s.funds.request != nil
	}
	if s.activeRoute() == portfolioRoute && s.portfolio != nil {
		return s.portfolio.request != nil
	}
	if s.activeRoute() == tasksRoute && s.tasks != nil {
		return s.tasks.request != nil || s.tasks.holdingRecon != nil && s.tasks.holdingRecon.loading != nil
	}
	if s.activeRoute() == wealthHistoryRoute && s.wealthHistory != nil {
		return s.wealthHistory.originRead != nil || s.wealthHistory.seriesRead != nil || s.wealthHistory.accounts.request != nil
	}
	if s.activeRoute() == returnAnalysisRoute && s.returns != nil {
		return s.returns.refsRead != nil || s.returns.request != nil
	}
	if s.activeRoute() == assetChangesRoute && s.assets != nil {
		return s.assets.refsRead != nil || s.assets.request != nil
	}
	if s.activeRoute() == marketDataRoute && s.marketData != nil {
		return s.marketData.request != nil
	}
	if s.activeRoute() == dataHealthRoute && s.dataHealth != nil {
		return s.dataHealth.request != nil
	}
	if s.activeRoute() != directoryRoute || s.directory == nil {
		return false
	}
	if s.trail.current().target.hasObject() {
		return s.directory.object.result != nil
	}
	return s.directory.activeList().restoreVisit != nil
}

func (s *shell) navigationButton(gtx layout.Context, view ui.View, step int) layout.Dimensions {
	button, hint := &s.back, &s.backHint
	label, targetLabel := i18n.NavigationBack, i18n.NavigationBackTo
	if step > 0 {
		button, hint = &s.forward, &s.forwardHint
		label, targetLabel = i18n.NavigationForward, i18n.NavigationForwardTo
	}
	title := view.Language.Text(label)
	name := title
	if entry := s.trail.adjacent(step); entry != nil {
		name = fmt.Sprintf(view.Language.Text(targetLabel), s.visitLabel(view.Language, entry))
	} else {
		gtx = gtx.Disabled()
	}
	icon := navigationBackIcon
	if step > 0 {
		icon = navigationForwardIcon
	}
	dims := view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonGhost, Name: name, ShortName: title, Icon: icon})(gtx)
	gtx.Constraints = layout.Exact(dims.Size)
	hint.Layout(gtx, view, button.Hovered(), name)
	return dims
}

func (s *shell) visitLabel(language i18n.Language, entry *navigationEntry) string {
	if entry.target.hasObject() {
		if entry.target.route == historyRoute && entry.target.instrument != "" {
			return language.Text(i18n.NavHistory) + " · " + entry.name
		}
		if entry.name != "" {
			if entry.target.statement != "" {
				return language.Text(i18n.StatementOpen) + " · " + entry.name
			}
			return entry.name
		}
		if entry.target.route == accountsRoute {
			return language.Text(i18n.AccountUnavailable)
		}
		if entry.target.product != "" {
			return language.Text(i18n.ProductUnavailable)
		}
		if entry.target.route == portfolioRoute {
			return language.Text(i18n.PortfolioInstrumentUnavailable)
		}
		if entry.target.route == historyRoute {
			return language.Text(i18n.HistoryDetailTitle)
		}
		return language.Text(i18n.DirectoryObjectUnavailable)
	}
	title := language.Text(s.routes[entry.target.route].label)
	if isMaintenanceRoute(entry.target.route) && s.marketData != nil && s.dataHealth != nil {
		label := i18n.MaintenanceIssues
		if entry.target.route == marketDataRoute {
			label = i18n.MaintenanceQuotes
		}
		title += " · " + language.Text(label)
	}
	if entry.target.route == tasksRoute && s.tasks.checks != nil && entry.visited {
		return title + " · " + language.Text(taskTabLabels[entry.tasks.tab])
	}
	if entry.target.route == accountsRoute {
		if entry.target.institution != "" {
			if entry.name == "" {
				return title + " · " + language.Text(i18n.AccountReferenceUnavailable)
			}
			title += " · " + entry.name
		}
		if entry.accountsArchived {
			title += " · " + language.Text(i18n.InstitutionsShowArchived)
		}
	}
	if entry.target.route == directoryRoute && entry.visited && s.directory != nil {
		title += " · " + language.Text(s.directory.lists[entry.directory.tab].copy.title)
		if entry.directory.list.includeArchived || strings.TrimSpace(entry.directory.list.query) != "" {
			title += " · " + language.Text(i18n.NavigationFiltered)
		}
	}
	return title
}

var (
	navigationBackIcon    = ui.Glyph(lucide.ArrowLeft)
	navigationForwardIcon = ui.Glyph(lucide.ArrowRight)
)
