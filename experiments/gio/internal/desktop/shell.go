package desktop

import (
	"image"
	"time"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type routeID string

const (
	overviewRoute         routeID = "overview"
	accountsRoute         routeID = "accounts"
	fundsRoute            routeID = "available-funds"
	portfolioRoute        routeID = "portfolio"
	wealthHistoryRoute    routeID = "historical-overview"
	returnAnalysisRoute   routeID = "returns"
	assetChangesRoute     routeID = "asset-changes"
	tasksRoute            routeID = "tasks"
	historyRoute          routeID = "history"
	directoryRoute        routeID = "directory"
	marketDataRoute       routeID = "market-data"
	dataHealthRoute       routeID = "data-health"
	settingsRoute         routeID = "settings"
	componentPreviewRoute routeID = "component-preview"
)

const (
	sidebarWidth     = 224
	sidebarRailWidth = 64
	headerHeight     = 53
)

type pageLayout func(layout.Context, ui.View) layout.Dimensions

type ledgerPages struct {
	prefs         *preferenceState
	directory     *directoryPage
	history       *historyPage
	overview      *overviewPage
	funds         *availableFundsPage
	tasks         *tasksPage
	accounts      *accountsPage
	portfolio     *portfolioPage
	wealthHistory *wealthHistoryPage
	returns       *returnAnalysisPage
	assets        *assetChangesPage
	marketData    *marketDataPage
	dataHealth    *dataHealthPage
	version       func() string
}

type navigationItem struct {
	id     routeID
	label  i18n.MessageID
	icon   ui.Glyph
	body   pageLayout
	button widget.Clickable
	hint   ui.Tooltip
}

func newItem(id routeID, label i18n.MessageID, icon ui.Glyph, body pageLayout) navigationItem {
	return navigationItem{id: id, label: label, icon: icon, body: body}
}

func (item *navigationItem) available() bool {
	return item.body != nil || item.id == historyRoute || item.id == directoryRoute || item.id == overviewRoute
}

type navigationGroup struct {
	label i18n.MessageID
	items []navigationItem
}

type shell struct {
	// stage scrolls the active page; stageEntry is the visit it was reset for.
	stage                                 widget.List
	stageEntry                            *navigationEntry
	prefs                                 *preferenceState
	onboarding                            *onboardingPage
	settingsPage                          *settingsPage
	directory                             *directoryPage
	historyPage                           *historyPage
	overviewPage                          *overviewPage
	funds                                 *availableFundsPage
	tasks                                 *tasksPage
	accounts                              *accountsPage
	portfolio                             *portfolioPage
	wealthHistory                         *wealthHistoryPage
	returns                               *returnAnalysisPage
	assets                                *assetChangesPage
	marketData                            *marketDataPage
	dataHealth                            *dataHealthPage
	maintenanceTabs                       [2]widget.Clickable
	componentPreview                      *componentPreview
	preferences                           sessionPreferences
	theme                                 *material.Theme
	wordmark                              widget.Image
	mark                                  widget.Image
	groups                                []navigationGroup
	routes                                map[routeID]*navigationItem
	trail                                 navigationTrail
	ledgerVersion                         func() string
	seenLedgerVersion                     string
	pendingNavigation                     *navigationRequest
	restoreFocus                          bool
	headingFocus                          byte
	back, forward                         widget.Clickable
	backHint, forwardHint, recentHint     ui.Tooltip
	recentPicker                          ui.Select
	navigation                            layout.List
	languagePicker                        ui.Select
	themeButton                           widget.Clickable
	railButton                            widget.Clickable
	languageHint, appearanceHint          ui.Tooltip
	wideCollapsed, narrowExpanded, narrow bool
	overlay                               ui.Overlay
	motion                                ui.Motion
}

func newShell(wordmark, mark image.Image, pages ledgerPages) *shell {
	preview := newComponentPreview()
	var directoryLayout, historyLayout, overviewLayout, accountsLayout, portfolioLayout, fundsLayout, tasksLayout pageLayout
	var wealthHistoryLayout, returnAnalysisLayout, assetChangesLayout, marketDataLayout pageLayout
	if pages.directory != nil {
		directoryLayout = pages.directory.layout
	}
	if pages.history != nil {
		historyLayout = pages.history.layout
	}
	if pages.overview != nil {
		overviewLayout = pages.overview.layout
	}
	if pages.tasks != nil {
		tasksLayout = pages.tasks.layout
	}
	if pages.funds != nil {
		fundsLayout = pages.funds.layout
	}
	if pages.accounts != nil {
		accountsLayout = pages.accounts.layout
	}
	if pages.portfolio != nil {
		portfolioLayout = pages.portfolio.layout
	}
	if pages.wealthHistory != nil {
		wealthHistoryLayout = pages.wealthHistory.layout
	}
	if pages.returns != nil {
		returnAnalysisLayout = pages.returns.layout
	}
	if pages.assets != nil {
		assetChangesLayout = pages.assets.layout
	}
	if pages.marketData != nil {
		marketDataLayout = pages.marketData.layout
	}
	s := &shell{
		prefs:            pages.prefs,
		settingsPage:     newSettingsPage(pages.prefs),
		directory:        pages.directory,
		historyPage:      pages.history,
		overviewPage:     pages.overview,
		funds:            pages.funds,
		tasks:            pages.tasks,
		accounts:         pages.accounts,
		portfolio:        pages.portfolio,
		wealthHistory:    pages.wealthHistory,
		returns:          pages.returns,
		assets:           pages.assets,
		marketData:       pages.marketData,
		dataHealth:       pages.dataHealth,
		componentPreview: preview,
		preferences:      sessionPreferences{language: i18n.SimplifiedChinese},
		theme:            ui.NewTheme(),
		wordmark:         widget.Image{Src: paint.NewImageOp(wordmark), Fit: widget.Contain},
		mark:             widget.Image{Src: paint.NewImageOp(mark), Fit: widget.Contain},
		trail:            newNavigationTrail(overviewRoute),
		navigation:       layout.List{Axis: layout.Vertical},
		stage:            widget.List{List: layout.List{Axis: layout.Vertical}},
	}
	if s.overviewPage != nil {
		s.settingsPage.credential.changed = s.overviewPage.healthScan.invalidate
	}
	maintenance := newItem(marketDataRoute, i18n.NavMarketData, lucide.RefreshCw, marketDataLayout)
	if s.marketData == nil && s.dataHealth != nil {
		maintenance = newItem(dataHealthRoute, i18n.NavMarketData, lucide.RefreshCw, s.dataHealth.layout)
	}
	s.groups = []navigationGroup{
		{items: []navigationItem{
			newItem(overviewRoute, i18n.NavOverview, lucide.House, overviewLayout),
			newItem(accountsRoute, i18n.NavAccounts, lucide.Wallet, accountsLayout),
			newItem(fundsRoute, i18n.NavAvailableFunds, lucide.Banknote, fundsLayout),
			newItem(portfolioRoute, i18n.NavPortfolio, lucide.ChartPie, portfolioLayout),
		}},
		{label: i18n.GroupActivity, items: []navigationItem{
			newItem(tasksRoute, i18n.NavTasks, lucide.ClipboardCheck, tasksLayout),
			newItem(historyRoute, i18n.NavHistory, lucide.ReceiptText, historyLayout),
		}},
		{label: i18n.GroupInsights, items: []navigationItem{
			newItem(wealthHistoryRoute, i18n.NavHistoricalOverview, lucide.CalendarClock, wealthHistoryLayout),
			newItem(returnAnalysisRoute, i18n.NavReturns, lucide.ChartLine, returnAnalysisLayout),
			newItem(assetChangesRoute, i18n.NavAssetChanges, lucide.GitCompareArrows, assetChangesLayout),
		}},
		{label: i18n.GroupManage, items: []navigationItem{
			newItem(directoryRoute, i18n.NavDirectory, lucide.Users, directoryLayout),
			maintenance,
			newItem(settingsRoute, i18n.NavSettings, lucide.Settings, s.settingsPage.layout),
			newItem(componentPreviewRoute, i18n.NavComponentPreview, lucide.Wrench, preview.layout),
		}},
	}
	s.routes = make(map[routeID]*navigationItem)
	for _, group := range s.groups {
		for i := range group.items {
			item := &group.items[i]
			s.routes[item.id] = item
		}
	}
	if s.dataHealth != nil {
		if s.marketData != nil {
			issues := newItem(dataHealthRoute, i18n.NavMarketData, nil, s.dataHealth.layout)
			s.routes[dataHealthRoute] = &issues
		}
		openHealth := func(scope healthFindingFocus, focus event.Tag) {
			s.pendingNavigation = &navigationRequest{target: navigationTarget{route: dataHealthRoute, health: scope}, focus: focus}
		}
		if s.marketData != nil {
			s.marketData.openHealth = func(focus *widget.Clickable) { openHealth(healthFindingFocus{}, focus) }
			if s.marketData.manual.service != nil {
				s.dataHealth.openManual = s.marketData.openManualQuote
			}
			if s.marketData.instrumentSources != nil {
				s.dataHealth.openInstrumentSource = s.marketData.openInstrumentSource
			}
			if s.marketData.agentReader != nil {
				s.dataHealth.openAgentEvidence = s.marketData.openAgentEvidence
			}
			if s.marketData.quoteHistoryReader != nil {
				s.dataHealth.quoteHistory = s.marketData.quoteHistoryAction
			}
			if s.marketData.sync != nil && s.dataHealth.repair == s.marketData.sync {
				s.dataHealth.previewQuoteRepair = func(scope application.SyncRequest, from event.Tag) {
					s.marketData.previewMarketSyncFrom(dataHealthRoute, scope, from)
				}
				s.dataHealth.openRepairDetails = func(from event.Tag) {
					s.marketData.openMarketOperation(dataHealthRoute, marketSyncDetails, from)
				}
			}
		}
		if s.overviewPage != nil {
			s.overviewPage.openHealth = openHealth
			s.overviewPage.upcoming.links.openHealth = openHealth
		}
		if s.tasks != nil {
			s.tasks.upcomingLinks.openHealth = openHealth
		}
		if s.assets != nil {
			s.assets.openHealth = openHealth
		}
		s.dataHealth.clearFocus = func(focus event.Tag) {
			openHealth(healthFindingFocus{}, focus)
		}
		s.dataHealth.inspect = openHealth
		s.dataHealth.snapshots.rebuilt = func() {
			if s.overviewPage != nil {
				s.overviewPage.trend.invalidate()
			}
		}
		s.dataHealth.openSettings = func(provider string, focus *widget.Clickable) {
			s.pendingNavigation = &navigationRequest{target: navigationTarget{route: settingsRoute}, focus: focus,
				providerSettings: true, settingsProvider: provider}
		}
		if s.accounts != nil {
			s.dataHealth.openAccount = func(id domain.AccountID, name string, focus *widget.Clickable) {
				s.pendingNavigation = &navigationRequest{target: navigationTarget{route: accountsRoute, account: id}, name: name, focus: focus}
			}
		}
	}
	if s.marketData != nil {
		s.marketData.openProviderSettings = func(from event.Tag) {
			s.pendingNavigation = &navigationRequest{target: navigationTarget{route: settingsRoute}, focus: from, providerSettings: true}
		}
	}
	if s.overviewPage != nil && s.wealthHistory != nil && s.historyPage != nil {
		s.overviewPage.openHistory = func(started bool, focus event.Tag) {
			route := historyRoute
			if started {
				route = wealthHistoryRoute
			}
			s.pendingNavigation = &navigationRequest{target: navigationTarget{route: route}, focus: focus}
		}
	}
	if s.overviewPage != nil && s.historyPage != nil {
		s.overviewPage.openLedger = func(focus event.Tag) {
			s.pendingNavigation = &navigationRequest{target: navigationTarget{route: historyRoute}, focus: focus}
		}
	}
	if s.overviewPage != nil && s.tasks != nil {
		s.overviewPage.upcoming.openTasks = func(tab tasksTab, focus event.Tag) {
			s.pendingNavigation = &navigationRequest{target: navigationTarget{route: tasksRoute}, tasksTab: tab, focus: focus}
		}
		s.overviewPage.upcoming.checks = s.tasks.checks
		s.tasks.reviewAck.onChange = s.overviewPage.upcoming.summary.reviewChanged
	}
	if s.overviewPage != nil && s.funds != nil {
		s.overviewPage.funds.reader = s.funds.service
		s.overviewPage.funds.open = func(day string, focus event.Tag) {
			s.pendingNavigation = &navigationRequest{target: navigationTarget{route: fundsRoute, fundsDay: day}, focus: focus}
		}
	}
	if s.returns != nil && s.assets != nil {
		s.returns.openAssets = func(selection analysisSelection, focus *widget.Clickable) {
			s.pendingNavigation = &navigationRequest{target: navigationTarget{route: assetChangesRoute},
				focus: focus, asset: assetChangesVisit{selection: selection, tab: assetDriversTab}}
		}
		s.assets.openReturns = func(selection analysisSelection, expanded bool, metric application.ContributionReturnType, focus event.Tag) {
			visit := s.returns.visit()
			visit.selection, visit.expanded, visit.tab, visit.metric = selection, expanded, returnContributionTab, metric
			visit.preset, visit.presetAt = 0, analysisDateWindow{}
			visit.list, visit.selectedKey = layout.Position{}, ""
			visit.rankingDirection, visit.rankingLimit, visit.rankingSearch = contributionLargest, 0, ""
			s.pendingNavigation = &navigationRequest{target: navigationTarget{route: returnAnalysisRoute}, focus: focus, returns: &visit}
		}
	}
	if s.returns != nil && s.historyPage != nil && s.historyPage.reads != nil {
		s.returns.openHistory = func(filters history.Filters, focus event.Tag) {
			s.pendingNavigation = &navigationRequest{target: navigationTarget{route: historyRoute},
				focus: focus, historyFilters: filters}
		}
	}
	if s.assets != nil && s.historyPage != nil && s.historyPage.reads != nil {
		s.assets.openHistory = func(filters history.Filters, activity domain.ActivityID, focus event.Tag) {
			s.pendingNavigation = &navigationRequest{target: navigationTarget{route: historyRoute, activity: activity},
				focus: focus, historyFilters: filters}
		}
	}
	if s.wealthHistory != nil && s.historyPage != nil && s.historyPage.reads != nil {
		s.wealthHistory.openHistory = func(filters history.Filters, focus event.Tag) {
			s.pendingNavigation = &navigationRequest{target: navigationTarget{route: historyRoute},
				focus: focus, historyFilters: filters}
		}
	}
	if s.directory != nil {
		for _, list := range s.directory.lists {
			list.openObject = func(ref directoryRef, name string, from event.Tag) {
				s.pendingNavigation = &navigationRequest{
					target: navigationTarget{route: directoryRoute, object: ref},
					name:   name, focus: from,
				}
			}
		}
	}
	if s.accounts != nil {
		s.accounts.open = func(target navigationTarget, name string, from event.Tag) {
			keepArchiveFilter := s.activeRoute() == accountsRoute && target.account == "" && s.accounts.archived
			s.pendingNavigation = &navigationRequest{target: target, name: name, focus: from, accountsArchived: keepArchiveFilter}
		}
		if s.tasks != nil && s.tasks.holdingRecon != nil && s.tasks.holdingRecon.ledger != nil {
			s.accounts.detail.openReconciliation = func(account domain.AccountID, session, name string, from event.Tag) {
				s.pendingNavigation = &navigationRequest{target: navigationTarget{route: tasksRoute}, name: name,
					focus: from, tasksTab: tasksReconciliation, tasksSession: session, tasksAccount: account}
			}
		}
		if s.historyPage != nil && s.historyPage.reads != nil {
			s.accounts.detail.openActivityList = func(account domain.AccountID, from event.Tag) {
				s.pendingNavigation = &navigationRequest{target: navigationTarget{route: historyRoute},
					focus: from, historyFilters: history.Filters{Account: account}}
			}
		}
		if s.returns != nil && s.assets != nil {
			s.accounts.detail.openAnalysis = func(route routeID, account domain.AccountID, name string, from event.Tag) {
				selection := analysisSelection{scope: domain.AnalysisScope{Kind: domain.ScopeAccount, ID: account.String()}, valuation: domain.ValuationBase}
				s.openScopedAnalysis(route, selection, name, from)
			}
		}
		if s.overviewPage != nil {
			s.overviewPage.breakdowns.open = s.accounts.open
			s.overviewPage.openAccounts = func(focus event.Tag) {
				s.accounts.open(navigationTarget{route: accountsRoute}, "", focus)
			}
			s.overviewPage.allocation.openAccount = func(id domain.AccountID, name string, from event.Tag) {
				s.accounts.open(navigationTarget{route: accountsRoute, account: id}, name, from)
			}
		}
		s.accounts.backToList = func() {
			if previous := s.trail.adjacent(-1); previous != nil && previous.target.route == accountsRoute && !previous.target.hasObject() {
				s.pendingNavigation = &navigationRequest{step: -1}
				return
			}
			s.accounts.open(navigationTarget{route: accountsRoute}, "", nil)
		}
		if s.directory != nil {
			s.directory.object.openAccounts = func(id domain.InstitutionID, name string, from event.Tag) {
				s.accounts.open(navigationTarget{route: accountsRoute, institution: id}, name, from)
			}
		}
	}
	s.ledgerVersion = pages.version
	if s.accounts != nil && s.accounts.opening != nil {
		s.accounts.opening.onSaved = s.refreshLedgerReads
	}
	if s.ledgerVersion != nil {
		s.seenLedgerVersion = s.ledgerVersion()
	}
	if s.historyPage != nil {
		s.historyPage.onPosted = s.refreshLedgerReads
	}
	if s.historyPage != nil && s.historyPage.reads != nil {
		if s.accounts != nil && s.historyPage.cashMovements != nil {
			money := s.historyPage.cashMovements
			scoped := newHistoryCashMovements(s.accounts.ctx, money.service, s.accounts.clock, s.accounts.invalidate)
			scoped.recovery = money.recovery
			s.accounts.detail.cashActions.form = scoped
		}
		if s.historyPage.investment != nil {
			s.historyPage.investment.onPosted = s.historyPage.recordPosted
		}
		s.historyPage.reads.openAction = func(id domain.ActivityID, action historyRowAction, focus event.Tag) {
			s.pendingNavigation = &navigationRequest{target: navigationTarget{route: historyRoute, activity: id}, focus: focus, historyAction: action}
		}
		s.historyPage.reads.open = func(id domain.ActivityID, focus event.Tag) {
			s.historyPage.reads.openAction(id, historyRowView, focus)
		}
		if s.tasks != nil {
			s.tasks.openActivity = s.historyPage.reads.open
		}
		if s.accounts != nil && s.accounts.cashBalance != nil {
			s.accounts.cashBalance.openActivity = s.historyPage.reads.open
		}
		if s.accounts != nil && s.accounts.balanceTarget != nil {
			s.accounts.balanceTarget.openActivity = s.historyPage.reads.open
		}
		if s.accounts != nil && s.accounts.liabilityPosting != nil {
			s.accounts.liabilityPosting.onPosted = s.historyPage.showPosted
			s.accounts.liabilityPosting.history = s.historyPage
		}
		if s.accounts != nil && s.accounts.investment != nil {
			s.accounts.investment.onPosted = s.historyPage.showPosted
		}
		if s.portfolio != nil && s.portfolio.investment != nil {
			s.portfolio.investment.onPosted = s.historyPage.showPosted
		}
		if s.accounts != nil {
			s.historyPage.reads.openAccount = func(id domain.AccountID, name string, focus event.Tag) {
				s.accounts.open(navigationTarget{route: accountsRoute, account: id}, name, focus)
			}
		}
		s.historyPage.reads.rename = func(id domain.ActivityID, name string) {
			target := navigationTarget{route: historyRoute, activity: id}
			entry := s.trail.current()
			if entry.target == target && entry.name != name {
				s.trail.rename(target, name)
			}
		}
		s.historyPage.reads.postedBackLabel = func() i18n.MessageID {
			source := s.trail.adjacent(-1)
			if source == nil {
				return i18n.NoMessage
			}
			if source.target.route == tasksRoute && source.tasks.holdingSession != "" {
				id := s.trail.current().target.activity
				if source.tasks.holdingReceipt == id || source.tasks.journalReceipt.operation == id.String() {
					return i18n.HoldingReconBack
				}
			}
			if source.target.route == historyRoute && source.target.activity != "" &&
				source.historyDetail.linked >= 3 && s.trail.current().target.activity == s.historyPage.reads.posted.ID {
				return i18n.CustodyAmendmentBack
			}
			if source.target.route == portfolioRoute && s.portfolio != nil && s.portfolio.investment != nil && source.focus == &s.portfolio.investment.importOpen {
				return i18n.PositionImportBackPortfolio
			}
			if source.target.route != accountsRoute || s.accounts == nil {
				return i18n.NoMessage
			}
			detail := &s.accounts.detail
			cash := &detail.cashActions
			if source.focus == &detail.record.trigger || source.focus == &cash.open || source.focus == &cash.fx || source.focus == &cash.transfer {
				return i18n.LiabilityBackToAccount
			}
			if f := s.accounts.liabilityPosting; f != nil && source.focus == &f.open {
				return i18n.LiabilityBackToAccount
			}
			if f := s.accounts.investment; f != nil && (source.focus == &f.buy || source.focus == &f.sell || source.focus == &f.dividend || source.focus == &f.importOpen ||
				source.focus == &f.custodyOpen || source.focus == &f.adjustmentOpen || f.holdingActionFallback(source.focus) != nil) {
				return i18n.LiabilityBackToAccount
			}
			return i18n.NoMessage
		}
		s.historyPage.reads.back = func() {
			if s.historyPage.reads.postedBackLabel() != i18n.NoMessage {
				s.pendingNavigation = &navigationRequest{step: -1}
				return
			}
			request := navigationRequest{target: navigationTarget{route: historyRoute}}
			if filters := s.trail.current().historyFilters; !filters.Equal(history.Filters{}) {
				request.historyFilters = filters
				s.pendingNavigation = &request
				return
			}
			for i := s.trail.cursor - 1; i >= 0; i-- {
				if target := s.trail.entries[i].target; target.route == historyRoute && target.activity == "" {
					request.step = i - s.trail.cursor
					break
				}
			}
			s.pendingNavigation = &request
		}
	}
	if p := s.portfolio; p != nil {
		p.detail.market = s.marketData
		if market := s.marketData; market != nil {
			p.detail.openMaintenance = func(scope healthFindingFocus, from event.Tag) {
				s.pendingNavigation = &navigationRequest{target: navigationTarget{route: marketDataRoute, health: scope}, focus: from}
			}
			market.openInstrumentDetails = func(id domain.InstrumentID, name string, from event.Tag) {
				scope := healthFindingFocus{instrument: id.String()}
				if current := market.focus; current.instrument == scope.instrument {
					scope.account, scope.from, scope.to = current.account, current.from, current.to
				}
				s.pendingNavigation = &navigationRequest{target: navigationTarget{route: marketDataRoute, health: scope}, name: name, focus: from}
			}
			if p.catalog.ledger != nil {
				market.openCatalogEditor = func(item domain.Instrument, from event.Tag) {
					s.pendingNavigation = &navigationRequest{target: navigationTarget{route: portfolioRoute}, focus: from, catalogEditor: &item}
				}
				p.catalog.finishExternal = func(message i18n.MessageID) {
					if source := s.trail.adjacent(-1); source != nil && source.target.route == marketDataRoute {
						market.catalogNotice = message
						s.pendingNavigation = &navigationRequest{step: -1}
					}
				}
			}
		}
		p.openInstrument = func(id domain.InstrumentID, name string, from event.Tag) {
			s.pendingNavigation = &navigationRequest{target: navigationTarget{route: portfolioRoute, instrument: id}, name: name, focus: from}
		}
		p.openHoldingInstrument = func(id domain.InstrumentID, account domain.AccountID, name string, from event.Tag) {
			s.pendingNavigation = &navigationRequest{target: navigationTarget{route: portfolioRoute, instrument: id, portfolioAccount: account}, name: name, focus: from}
		}
		if s.tasks != nil {
			s.tasks.openInstrument = p.openInstrument
		}
		if s.accounts != nil {
			p.openAccount = func(id domain.AccountID, name string, from event.Tag) {
				s.accounts.open(navigationTarget{route: accountsRoute, account: id}, name, from)
			}
			s.accounts.openInstrument = p.openHoldingInstrument
			if s.historyPage != nil && s.historyPage.reads != nil {
				s.accounts.openHoldingHistory = s.openHoldingHistory
			}
			if s.returns != nil && s.assets != nil {
				s.accounts.openHoldingAnalysis = s.openHoldingAnalysis
			}
		}
		if s.historyPage != nil && s.historyPage.reads != nil {
			p.openHistory = func(id domain.InstrumentID, name string, from event.Tag) {
				p.openHoldingHistory(id, p.accountScope, name, from)
			}
			p.openHoldingHistory = s.openHoldingHistory
			s.historyPage.reads.openInstrument = p.openInstrument
		}
		if s.returns != nil && s.assets != nil {
			p.openHoldingAnalysis = s.openHoldingAnalysis
		}
	}
	if s.tasks != nil && s.accounts != nil && s.accounts.statement != nil {
		s.tasks.openStatement = func(account domain.AccountID, session, name string, from event.Tag) {
			s.accounts.open(navigationTarget{route: accountsRoute, account: account, statement: session}, name, from)
		}
		if c := s.tasks.holdingRecon; c != nil && c.ledger != nil {
			s.accounts.statement.openReconciliation = func(session, name string, focus event.Tag) {
				s.pendingNavigation = &navigationRequest{
					target: navigationTarget{route: tasksRoute}, name: name, focus: focus,
					tasksTab: tasksReconciliation, tasksSession: session,
				}
			}
		}
	}
	if p := s.funds; p != nil {
		openAction := func(target fundsActionFocus, from event.Tag) {
			s.pendingNavigation = &navigationRequest{target: navigationTarget{route: fundsRoute, fundsAction: target}, focus: from}
		}
		if s.tasks != nil {
			s.tasks.upcomingLinks.openFunds = openAction
		}
		if s.overviewPage != nil {
			s.overviewPage.upcoming.links.openFunds = openAction
		}
		if p.productPage != nil {
			p.openProduct = func(id domain.ProductContractID, name string, from event.Tag) {
				s.pendingNavigation = &navigationRequest{target: navigationTarget{route: fundsRoute, product: id}, name: name, focus: from}
			}
			if s.tasks != nil {
				s.tasks.openProduct = p.openProduct
				s.tasks.verify = func(id domain.ProductContractID, name string, from event.Tag) {
					s.pendingNavigation = &navigationRequest{
						target: navigationTarget{route: fundsRoute, product: id}, name: name, focus: from, verifyProductReceipt: true,
					}
				}
			}
			if s.overviewPage != nil {
				s.overviewPage.upcoming.openProduct = func(id domain.ProductContractID, name string, due bool, focus event.Tag) {
					s.pendingNavigation = &navigationRequest{
						target: navigationTarget{route: fundsRoute, product: id}, name: name, focus: focus, verifyProductReceipt: due,
					}
				}
			}
			if s.portfolio != nil {
				s.portfolio.openProduct = p.openProduct
			}
			if s.accounts != nil {
				s.accounts.openProduct = p.openProduct
			}
		}
		if s.accounts != nil {
			p.openAccount = func(id domain.AccountID, name string, from event.Tag) {
				s.accounts.open(navigationTarget{route: accountsRoute, account: id}, name, from)
			}
		}
		if p.productPage != nil {
			p.productPage.openAccount = p.openAccount
			p.productPage.onProductSaved = s.refreshLedgerReads
			if s.historyPage != nil && s.historyPage.reads != nil {
				p.productPage.openActivity = s.historyPage.reads.open
				s.historyPage.reads.openProduct = p.openProduct
			}
		}
		if s.portfolio != nil {
			p.openInstrument = s.portfolio.openInstrument
		}
	}
	return s
}

func (s *shell) layout(gtx layout.Context) layout.Dimensions {
	// Drop animation state for controls that were not drawn this frame.
	defer s.motion.EndFrame()
	s.prefs.update()
	s.preferences = s.prefs.appearance()
	if p := s.onboarding; p != nil {
		p.update()
		if p.opened {
			s.onboarding = nil
			if s.directory != nil {
				s.directory.householdName = p.householdName
			}
			if p.attempted && s.accounts != nil {
				s.trail = newNavigationTrail(accountsRoute)
				s.accounts.enter(s.trail.current().target, false, layout.Position{})
				s.prefs.reload()
			}
		}
	}
	if s.accounts != nil && s.activeRoute() == accountsRoute {
		if cash := &s.accounts.detail.cashActions; cash.active() {
			notice := cash.form.formLabels().posted
			if activity := cash.form.update(); activity != nil {
				s.historyPage.showPosted(*activity, notice, cash.source)
			}
		}
		s.accounts.pollLiabilityPosting()
		s.accounts.investment.poll(s.accounts.investmentRuntime())
		s.accounts.pollOpeningPosition()
		if s.accounts.pollInlineInstitution() && s.directory != nil {
			for _, list := range s.directory.lists {
				if list.isInstitutions() {
					list.refreshExternal = true
				}
			}
		}
		if s.accounts.pollAccountMaintenance() {
			s.refreshLedgerReads()
		}
		if saved := s.accounts.pollCashBalance(); saved.ID != "" {
			s.refreshLedgerReads()
		}
		if saved := s.accounts.pollBalanceTarget(); saved.account != "" {
			s.refreshLedgerReads()
		}
		if record := s.accounts.pollAccountCreation(); record.Account.ID != "" {
			s.accounts.createdID = record.Account.ID
			s.accounts.createdMessage = i18n.AccountCreateSucceeded
			focus := &s.accounts.createBank
			unscoped := s.accounts.target.institution == ""
			if unscoped {
				s.accounts.createdMessage = i18n.AccountCreateAssetSucceeded
				focus = &s.accounts.createAccount
			}
			if record.Account.IsLiability() {
				s.accounts.createdMessage = i18n.AccountCreateLiabilitySucceeded
			}
			if record.Account.TrackingMode != domain.TrackingBalance {
				s.accounts.createdMessage = i18n.AccountCreateAssetSucceeded
				if !unscoped {
					focus = &s.accounts.createShell
				}
			}
			s.refreshLedgerReads()
			s.pendingNavigation = &navigationRequest{
				target: navigationTarget{route: accountsRoute, account: record.Account.ID},
				name:   record.Account.Name, focus: focus,
			}
		}
		s.accounts.updateAccountCreateTrigger(gtx)
	}
	windowContext := gtx
	if s.activeRoute() == historyRoute && s.historyPage != nil {
		s.historyPage.pollPosted()
		s.historyPage.updateRecordTrigger(gtx)
	}
	if s.marketData != nil && s.marketData.updateLatest() {
		s.refreshLedgerReads()
		switch s.activeRoute() {
		case marketDataRoute:
			s.marketData.read()
		case dataHealthRoute:
			if s.dataHealth != nil {
				s.dataHealth.read()
			}
		}
	}
	if s.activeRoute() == tasksRoute && s.tasks.startReconciliationDraft() != nil {
		s.tasks.pollHoldingReconciliation()
	}
	if s.settingsPage.confirming || s.confirmingArchive() || s.accountCreationOpen() || s.accountMaintenanceBlocking() || s.fundsPolicyConfirming() || s.returnDetailOpen() || s.assetDetailOpen() || s.healthRepairOpen() || s.snapshotRepairOpen() || s.wealthDetailOpen() || s.manualQuoteOpen() || s.marketOperationOpen() || s.quoteHistoryOpen() || s.fxSourceOpen() || s.instrumentSourceOpen() || s.agentEvidenceOpen() || s.historyRecordOpen() || s.startReconciliationReviewOpen() {
		gtx = gtx.Disabled()
	}
	s.overlay.BeginFrame(gtx)
	s.narrow = !ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, ui.BreakpointMedium)
	if s.railButton.Clicked(gtx) {
		if s.narrow {
			s.narrowExpanded = !s.narrowExpanded
		} else {
			s.wideCollapsed = !s.wideCollapsed
		}
	}
	gtx.Locale.Language = string(s.preferences.language)
	colors := s.preferences.colors()
	s.theme.Palette = material.Palette{
		Bg:         colors.PageBackground,
		Fg:         colors.Ink,
		ContrastBg: colors.Accent,
		ContrastFg: colors.OnAccent,
	}
	view := ui.View{
		Language: s.preferences.language, Colors: colors, Theme: s.theme, Overlay: &s.overlay, Motion: &s.motion,
		DateFormat: s.prefs.saved.DateFormat, DecimalSeparator: s.prefs.saved.DecimalSeparator, GroupingSeparator: s.prefs.saved.GroupingSeparator,
		TimeFormat: s.prefs.saved.TimeFormat, WeekStart: s.prefs.saved.WeekStart,
		TimeLocation: s.prefs.displayLocation(),
	}
	paint.Fill(gtx.Ops, colors.PageBackground)
	if p := s.onboarding; p != nil {
		dims := p.layout(gtx, view, s.prefs, s.wordmark)
		s.overlay.Layout(gtx, view)
		return dims
	}
	s.observeLedger(gtx)
	s.updateNavigation(gtx)
	if s.tasks != nil {
		s.tasks.pollReviewAcknowledgement(s.activeRoute() == tasksRoute)
	}
	entry := s.trail.current()
	if entry.target.route == directoryRoute && entry.target.hasObject() && s.directory.object.update() {
		s.trail.rename(entry.target, s.directory.object.name())
	}
	if entry.target.route == accountsRoute && s.accounts != nil && s.accounts.update(s.preferences.language) {
		s.trail.rename(entry.target, s.accounts.name())
	}
	background := func(gtx layout.Context) layout.Dimensions {
		dims := layout.Flex{}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.sidebar(gtx, view) }),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return s.page(gtx, view) }),
		)
		s.overlay.Layout(gtx, view)
		return dims
	}
	if s.historyRecordOpen() {
		dims := accessibility.Hidden(gtx, background)
		windowContext.Locale = gtx.Locale
		s.historyPage.layoutRecord(windowContext, view)
		return dims
	}
	if s.accountCreationOpen() {
		dims := accessibility.Hidden(gtx, background)
		windowContext.Locale = gtx.Locale
		s.accounts.layoutAccountCreateSheet(windowContext, view)
		return dims
	}
	if s.startReconciliationReviewOpen() {
		dims := accessibility.Hidden(gtx, background)
		windowContext.Locale = gtx.Locale
		s.tasks.layoutStartReconciliationReview(windowContext, view)
		return dims
	}
	dims := background(gtx)
	if s.settingsPage.confirming {
		s.settingsPage.layoutConfirmation(windowContext, view)
	} else if s.confirmingArchive() {
		s.directory.activeList().layoutConfirmation(windowContext, view)
	} else if s.accountMaintenanceBlocking() && s.accounts.maintenance.review != nil {
		s.accounts.layoutAccountArchive(windowContext, view)
	} else if s.fundsPolicyConfirming() {
		s.funds.layoutPolicyConfirmation(windowContext, view)
	} else if s.returnDetailOpen() {
		if s.returns.day != nil {
			s.returns.layoutDay(windowContext, view)
		} else {
			s.returns.layoutContributionSheet(windowContext, view)
		}
	} else if s.assetDetailOpen() {
		if s.assets.categoryDetail != nil {
			s.assets.layoutCategorySheet(windowContext, view)
		} else {
			s.assets.layoutDriverSheet(windowContext, view)
		}
	} else if s.wealthDetailOpen() {
		s.wealthHistory.layoutWealthDetail(windowContext, view)
	} else if s.healthRepairOpen() {
		s.dataHealth.layoutRepair(windowContext, view)
	} else if s.snapshotRepairOpen() {
		s.dataHealth.layoutSnapshotRebuild(windowContext, view)
	} else if s.quoteHistoryOpen() {
		if source, health := s.marketData.layoutQuoteHistory(windowContext, view, s.dataHealth != nil && s.activeRoute() == marketDataRoute); source != nil {
			s.trail.current().focus, s.restoreFocus = source, true
		} else if health != nil {
			d := s.marketData.quoteHistory
			visit := d.visit
			visit.scroll = d.sheet.list.Position
			s.trail.current().quoteHistory = &visit
			s.marketData.closeQuoteHistory()
			s.pendingNavigation = &navigationRequest{target: navigationTarget{route: dataHealthRoute, health: *health}, focus: visit.source}
		}
	} else if s.marketOperationOpen() {
		if source := s.marketData.layoutMarketOperation(windowContext, view); source != nil {
			s.trail.current().focus, s.restoreFocus = source, true
		}
	} else if s.fxSourceOpen() {
		if source, preference := s.marketData.layoutFXSource(windowContext, view); source != nil {
			s.trail.current().focus, s.restoreFocus = source, true
			if preference != nil {
				s.refreshLedgerReads()
				s.marketData.read()
				if preference.SourceKind == domain.QuoteSourceProvider {
					s.marketData.startLatest(marketLatestTarget{})
				}
			}
		}
	} else if s.instrumentSourceOpen() {
		if source, saved := s.marketData.layoutInstrumentSource(windowContext, view); source != nil {
			s.trail.current().focus, s.restoreFocus = source, true
			if saved {
				s.refreshLedgerReads()
				if s.activeRoute() == dataHealthRoute {
					s.dataHealth.read()
				} else if s.activeRoute() == marketDataRoute {
					s.marketData.read()
				}
			}
		}
	} else if s.agentEvidenceOpen() {
		source, saved := s.marketData.layoutAgentEvidence(windowContext, view)
		if source != nil {
			s.trail.current().focus, s.restoreFocus = source, true
		}
		if saved {
			s.refreshLedgerReads()
			if s.activeRoute() == dataHealthRoute {
				s.dataHealth.read()
			} else {
				s.marketData.read()
			}
		}
	} else if s.manualQuoteOpen() {
		if source, receipt := s.marketData.layoutManualQuote(windowContext, view); source != nil {
			s.trail.current().focus, s.restoreFocus = source, true
			if receipt.saved() {
				s.marketData.manual.latest = receipt
				s.refreshLedgerReads()
				if s.activeRoute() == dataHealthRoute {
					s.dataHealth.manualSaved = receipt
					s.dataHealth.read()
				} else if s.activeRoute() == marketDataRoute {
					s.marketData.read()
				}
			}
		}
	}
	return dims
}

func (s *shell) wealthDetailOpen() bool {
	return s.activeRoute() == wealthHistoryRoute && s.wealthHistory != nil && s.wealthHistory.detail != nil
}

func (s *shell) fundsPolicyConfirming() bool {
	return s.activeRoute() == fundsRoute && s.funds != nil && s.funds.policyConfirming()
}

func (s *shell) assetDetailOpen() bool {
	return s.activeRoute() == assetChangesRoute && s.assets != nil && (s.assets.driver != nil || s.assets.categoryDetail != nil)
}

func (s *shell) returnDetailOpen() bool {
	return s.activeRoute() == returnAnalysisRoute && s.returns != nil && (s.returns.day != nil || s.returns.item != nil)
}

func (s *shell) healthRepairOpen() bool {
	return s.activeRoute() == dataHealthRoute && s.dataHealth != nil && s.dataHealth.repair != nil &&
		s.dataHealth.repair.owner == dataHealthRoute && s.dataHealth.repair.plan != nil && !s.marketOperationOpen()
}

func (s *shell) snapshotRepairOpen() bool {
	return s.activeRoute() == dataHealthRoute && s.dataHealth != nil && s.dataHealth.snapshots.active != nil
}

func (s *shell) manualQuoteOpen() bool {
	return s.marketData != nil && s.marketData.manual.draft != nil
}

func (s *shell) quoteHistoryOpen() bool {
	return s.marketData != nil && s.marketData.quoteHistory != nil
}

func (s *shell) fxSourceOpen() bool {
	return s.activeRoute() == marketDataRoute && s.marketData != nil && s.marketData.fxSourceDraft != nil
}

func (s *shell) instrumentSourceOpen() bool {
	return s.marketData != nil && s.marketData.instrumentSource != nil
}

func (s *shell) agentEvidenceOpen() bool {
	return s.marketData != nil && s.marketData.agentEvidence != nil
}

func (s *shell) marketOperationOpen() bool {
	return s.marketData != nil && s.marketData.operation != nil && s.marketData.operation.route == s.activeRoute()
}

func (s *shell) observeLedger(gtx layout.Context) {
	if s.ledgerVersion == nil {
		return
	}
	if version := s.ledgerVersion(); version != s.seenLedgerVersion {
		s.seenLedgerVersion = version
		s.invalidateLedgerReads()
	}
	switch s.activeRoute() {
	case overviewRoute, accountsRoute, historyRoute, portfolioRoute, fundsRoute, tasksRoute:
		gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(time.Second)})
	}
}

// Capture the committed version before scheduling any replacement reads.
func (s *shell) refreshLedgerReads() {
	if s.ledgerVersion != nil {
		s.seenLedgerVersion = s.ledgerVersion()
	}
	s.invalidateLedgerReads()
}

func (s *shell) invalidateLedgerReads() {
	if s.overviewPage != nil {
		s.overviewPage.refresh()
	}
	if s.historyPage != nil {
		s.historyPage.invalidateLedgerReads()
	}
	if s.accounts != nil && s.activeRoute() == accountsRoute {
		s.accounts.refresh()
	}
	if s.funds != nil && s.activeRoute() == fundsRoute {
		if s.trail.current().target.product == "" {
			s.funds.read()
		} else {
			s.funds.productPage.read()
		}
	}
	if s.portfolio != nil && s.activeRoute() == portfolioRoute {
		s.portfolio.read()
	}
	if s.tasks != nil {
		s.tasks.invalidateTaskBadges()
		if s.activeRoute() == tasksRoute {
			s.tasks.read()
		}
	}
}

func (s *shell) accountMaintenanceBlocking() bool {
	if s.activeRoute() != accountsRoute || s.accounts == nil || s.accounts.maintenance == nil {
		return false
	}
	m := s.accounts.maintenance
	return m.pending != nil || m.review != nil
}

func (s *shell) confirmingArchive() bool {
	return s.activeRoute() == directoryRoute && !s.trail.current().target.hasObject() &&
		s.directory != nil && s.directory.activeList().archiveTarget != nil
}

func (s *shell) sidebar(gtx layout.Context, view ui.View) layout.Dimensions {
	colors := view.Colors
	size := image.Pt(gtx.Dp(s.sidebarWidth()), gtx.Constraints.Max.Y)
	gtx.Constraints = layout.Exact(size)
	paint.FillShape(gtx.Ops, colors.SidebarBackground, clip.Rect{Max: size}.Op())
	paint.FillShape(gtx.Ops, colors.Divider, clip.Rect(image.Rect(size.X-1, 0, size.X, size.Y)).Op())

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.sidebarBrand(gtx, view) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 12, Right: 12, Top: 4, Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.sidebarNavigation(gtx, view)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.sidebarPreferences(gtx, view, size.Y) }),
	)
}

func (s *shell) sidebarNavigation(gtx layout.Context, view ui.View) layout.Dimensions {
	height := gtx.Constraints.Max.Y
	return s.navigation.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		// Fill the viewport when there is room and scroll all the groups together
		// when their natural height exceeds it.
		gtx.Constraints.Min.Y = height
		last := len(s.groups) - 1
		gap := gtx.Dp(20)
		return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceBetween, Gap: gap}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				groups := make([]layout.FlexChild, 0, last)
				for i := range s.groups[:last] {
					groups = append(groups, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.group(gtx, view, &s.groups[i])
					}))
				}
				return layout.Flex{Axis: layout.Vertical, Gap: gap}.Layout(gtx, groups...)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, view.Colors.Divider, clip.Rect(image.Rect(0, 0, gtx.Constraints.Max.X, 1)).Op())
				return layout.Inset{Top: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.group(gtx, view, &s.groups[last])
				})
			}),
		)
	})
}

func (s *shell) group(gtx layout.Context, view ui.View, group *navigationGroup) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(group.items)+1)
	if group.label != i18n.NoMessage && !s.collapsed() {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: 6, Left: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return view.Label(gtx, view.Language.Text(group.label), 11, font.Medium, view.Colors.SecondaryInk)
			})
		}))
	}
	for i := range group.items {
		item := &group.items[i]
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.item(gtx, view, item)
		}))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(2)}.Layout(gtx, children...)
}

func (s *shell) item(gtx layout.Context, view ui.View, item *navigationItem) layout.Dimensions {
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(36))
	gtx.Constraints = layout.Exact(size)
	if !item.available() {
		gtx = gtx.Disabled()
	}
	active := s.activeRoute()
	selected := active == item.id || isMaintenanceRoute(active) && isMaintenanceRoute(item.id)
	return accessibility.SelectableButton(gtx, &item.button, view.Language.Text(item.label), selected, func(gtx layout.Context) layout.Dimensions {
		return item.layoutButton(gtx, s, view, size, selected)
	})
}

func (item *navigationItem) layoutButton(gtx layout.Context, s *shell, view ui.View, size image.Point, selected bool) layout.Dimensions {
	colors := view.Colors
	content := func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		pointer.CursorPointer.Add(gtx.Ops)
		semantic.SelectedOp(selected).Add(gtx.Ops)
		bounds := image.Rectangle{Max: size}
		enabled := gtx.Enabled()
		motion := view.Motion.For(&item.button)
		motion.Hover.Set(gtx, boolFloat(enabled && item.button.Hovered()), ui.DurationBase, ui.EaseOut)
		motion.Select.Set(gtx, boolFloat(selected), ui.DurationBase, ui.EaseOut)
		motion.Focus.Set(gtx, boolFloat(enabled && gtx.Focused(&item.button)), ui.DurationBase, ui.EaseOut)
		hover, pick, focus := motion.Hover.Value(gtx), motion.Select.Value(gtx), motion.Focus.Value(gtx)

		// Hover and keyboard focus tint the row; the selected row lifts onto a
		// bordered card with the Wails accent marker.
		radius := ui.RadiusLG
		tint := max(hover, focus) * (1 - pick)
		ui.FillRoundRect(gtx, bounds, radius, ui.WithAlpha(colors.HoverSurface, tint))
		if pick > 0 {
			colors.DrawShadow(gtx, bounds, radius, ui.ElevationXS)
			ui.FillRoundRect(gtx, bounds, radius, ui.WithAlpha(colors.Surface, pick))
			ui.StrokeRoundRect(gtx, bounds, radius, 1, ui.WithAlpha(colors.Divider, pick))
			marker := image.Rect(gtx.Dp(1), gtx.Dp(8), gtx.Dp(3), size.Y-gtx.Dp(8))
			ui.FillRoundRect(gtx, marker, 1, ui.WithAlpha(colors.Accent, pick))
		}
		if focus > 0 {
			colors.FocusRingAt(gtx, bounds, radius, focus)
		}
		textColor := ui.Mix(colors.SecondaryInk, colors.Ink, max(pick, hover*0.6))
		iconColor := ui.Mix(colors.SecondaryInk, colors.Accent, pick)
		weight := font.Normal
		if selected {
			weight = font.Medium
		}
		item.hint.Layout(gtx, view, s.collapsed() && item.button.Hovered(), view.Language.Text(item.label))
		if s.collapsed() {
			semantic.LabelOp(view.Language.Text(item.label)).Add(gtx.Ops)
			return layout.Center.Layout(gtx, sidebarIcon(item.icon, iconColor, 18))
		}
		return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			return layout.Inset{Left: 11}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.LayoutGlyph(gtx, item.icon, 16, iconColor)
					}),
					layout.Rigid(layout.Spacer{Width: 12}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						style := ui.TextBody
						style.Weight = weight
						return view.Line(gtx, style, view.Language.Text(item.label), textColor)
					}),
				)
			})
		})
	}
	if gtx.Enabled() {
		return item.button.Layout(gtx, content)
	}
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	semantic.EnabledOp(false).Add(gtx.Ops)
	return content(gtx)
}

func (s *shell) page(gtx layout.Context, view ui.View) layout.Dimensions {
	entry := s.trail.current()
	route := s.routes[entry.target.route]
	body := route.body
	if entry.target.route == directoryRoute && entry.target.hasObject() {
		body = s.directory.object.layout
	}
	if entry.target.product != "" {
		body = s.funds.productPage.layout
	}
	view.Origin = image.Pt(gtx.Dp(s.sidebarWidth()), gtx.Dp(headerHeight))
	focus := ui.PageFocus{Target: entry.focus}
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			headerView := view
			headerView.Focus = &focus
			return s.pageHeader(gtx, headerView, route.label)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !s.hasMaintenanceTabs() {
				return layout.Dimensions{}
			}
			tabsView := view
			tabsView.Focus = &focus
			dims := s.layoutMaintenanceTabs(gtx, tabsView)
			view.Origin.Y += dims.Size.Y
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p := s.historyPage; p != nil && p.cashMovements != nil && p.cashMovements.recovery != nil {
				dims := p.cashMovements.recovery.layout(gtx, view, p.reads.open)
				view.Origin.Y += dims.Size.Y
				return dims
			}
			return layout.Dimensions{}
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			view.Focus = &focus
			return s.pageBody(gtx, view, body)
		}),
	)
	s.finishPageFocus(gtx, focus)
	return dims
}

func (s *shell) pageHeader(gtx layout.Context, view ui.View, title i18n.MessageID) layout.Dimensions {
	name := view.Language.Text(title)
	if s.activeRoute() == accountsRoute && s.accounts != nil && s.accounts.target.account != "" {
		if accountName := s.accounts.name(); accountName != "" {
			name = accountName
		}
	}
	recordView := view
	view.Focus = nil
	colors := view.Colors
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(headerHeight))
	gtx.Constraints = layout.Exact(size)
	paint.FillShape(gtx.Ops, colors.Divider, clip.Rect(image.Rect(0, size.Y-1, size.X, size.Y)).Op())
	return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = 0
		return layout.Inset{Left: 20, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gap := gtx.Dp(2)
			x := view.Origin.X + gtx.Dp(20)
			direction := func(step int) layout.Widget {
				return func(gtx layout.Context) layout.Dimensions {
					dims := s.navigationButton(gtx, view, step)
					x += dims.Size.X + gap
					return dims
				}
			}
			return layout.Flex{Alignment: layout.Middle, Gap: gap}.Layout(gtx,
				layout.Rigid(direction(-1)),
				layout.Rigid(direction(1)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.recentButton(gtx, view, x) }),
				layout.Rigid(layout.Spacer{Width: 10}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					for {
						if _, ok := gtx.Event(key.FocusFilter{Target: &s.headingFocus}); !ok {
							break
						}
					}
					defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
					event.Op(gtx.Ops, &s.headingFocus)
					return view.Line(gtx, ui.TextPageTitle, name, colors.Ink)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if s.activeRoute() == historyRoute && s.historyPage.canRecord() {
						return s.historyPage.layoutRecordTrigger(gtx, recordView)
					}
					return layout.Dimensions{}
				}),
			)
		})
	})
}

// pageInset is the gutter around page content.
var pageInset = layout.Inset{Top: 26, Left: 28, Right: 28, Bottom: 16}

// pageBody is the stage every page is laid out on. It gives a page the full
// width of the window (a page that wants a readable column centers itself with
// View.Measure), lets it be taller than the window and scrolls it, and tells it
// the viewport height through Constraints.Min.Y so a list can fill what is
// left. Pages that scroll internally keep doing so: a shared list inside the
// stage takes the viewport height instead of laying out every row.
func (s *shell) pageBody(gtx layout.Context, view ui.View, body pageLayout) layout.Dimensions {
	if body == nil {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}
	entry := s.trail.current()
	// Analysis pages briefly lay out a shorter loading view on Back. Wait for
	// the results so the stage does not clamp the saved offset to that view.
	deferAnalysisScroll := entry != s.stageEntry && entry.visited && entry.scroll.Offset != 0 && s.waitingForAnalysisScroll()
	if entry != s.stageEntry {
		if !deferAnalysisScroll {
			s.stageEntry = entry
		}
		s.stage.Position = layout.Position{}
		if entry.visited && !deferAnalysisScroll {
			switch entry.target.route {
			case settingsRoute, wealthHistoryRoute, returnAnalysisRoute, assetChangesRoute, marketDataRoute, dataHealthRoute, tasksRoute:
				s.stage.Position = entry.scroll
			}
		}
	}
	inset := pageInset
	if s.hasMaintenanceTabs() {
		inset.Top = ui.Space12
	}
	top, bottom := gtx.Dp(inset.Top), gtx.Dp(inset.Bottom)
	viewport := gtx.Constraints.Max
	bounds := image.Rectangle{Min: view.Origin, Max: view.Origin.Add(viewport)}
	view.Overlay.ScrollOutsidePanel(gtx, &s.stage, bounds)
	dims := view.List(&s.stage).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = max(0, viewport.Y-top-bottom)
			view := view.WithScrollAnchors(&s.stage)
			view.Origin = view.Origin.Add(image.Pt(gtx.Dp(inset.Left), top))
			return body(gtx, view)
		})
	})
	view.Overlay.OffsetScrollAnchor(&s.stage, bounds)
	if deferAnalysisScroll && !s.waitingForAnalysisScroll() {
		gtx.Execute(op.InvalidateCmd{})
	}
	return dims
}

func (s *shell) waitingForAnalysisScroll() bool {
	switch s.activeRoute() {
	case returnAnalysisRoute:
		return s.returns != nil && s.returns.current == nil && s.waitingForPageFocus()
	case assetChangesRoute:
		return s.assets != nil && s.assets.current == nil && s.waitingForPageFocus()
	default:
		return false
	}
}

func boolFloat(on bool) float64 {
	if on {
		return 1
	}
	return 0
}
