import { lazy, Suspense, useCallback, useEffect, useState, useRef, type ReactNode } from "react";
import { AppShell, DEFAULT_PAGE_ID } from "@/app/AppShell";
import { OnboardingPage } from "@/features/onboarding/OnboardingPage";
import { OverviewPage } from "@/features/overview/OverviewPage";
import { AccountsPage } from "@/features/accounts/AccountsPage";
import { BlockedStartupPage, StartupLoadingPage } from "@/features/startup/BlockedStartupPage";
import { LoadingState } from "@/components/layout/PageState";
import { useTranslation } from "react-i18next";
import { useBootstrap } from "@/queries/household";
import { useStartup } from "@/queries/app";
import { useSettings } from "@/queries/settings";
import { useUiStore, type Appearance, type Accent } from "@/stores/ui";
import { setLanguage } from "@/i18n";
import { resolveLegacyTarget, targetForPage, type HistoryNavigationFilters, type NavigationTarget, type PageId, type LiquidityFocus, type HistoricalNavigationContext, type InstrumentDetailContext, type PortfolioView } from "@/app/navigation";
import { useAnalysisStore } from "@/stores/analysis";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { hasUnresolvedSheetConfirmation } from "@/components/ui/sheet-confirmation";
import { NavigationContext } from "@/app/NavigationContext";
import type { HealthFocus, AccountListFocus } from "@/app/navigation";
import { AgentWorkspaceObserver } from "@/queries/agent";
import { MarketDataSyncWorkspaceObserver } from "@/queries/marketdata";

const ReconciliationPage = lazy(() => import("@/features/reconciliation/ReconciliationPage").then(module => ({default:module.ReconciliationPage})));
const HistoricalOverviewPage = lazy(() => import("@/features/historical-overview/HistoricalOverviewPage").then(module => ({ default: module.HistoricalOverviewPage })));
const PortfolioPage = lazy(() => import("@/features/portfolio/PortfolioPage").then((module) => ({ default: module.PortfolioPage })));
const InstrumentDetailPage = lazy(() => import("@/features/investments/InstrumentDetailPage").then((module) => ({ default: module.InstrumentDetailPage })));
const DirectoryPage = lazy(() => import("@/features/directory/DirectoryPage").then((module) => ({ default: module.DirectoryPage })));
const MarketDataPage = lazy(() => import("@/features/marketdata/MarketDataPage").then((module) => ({ default: module.MarketDataPage })));
const DataHealthPage = lazy(() => import("@/features/data-health/DataHealthPage").then((module) => ({ default: module.DataHealthPage })));
const SettingsPage = lazy(() => import("@/features/settings/SettingsPage").then((module) => ({ default: module.SettingsPage })));
const HistoryPage = lazy(() => import("@/features/history/HistoryPage").then((module) => ({ default: module.HistoryPage })));
const ReturnAnalysisPage = lazy(() => import("@/features/insights/ReturnAnalysisPage").then((module) => ({ default: module.ReturnAnalysisPage })));
const AssetChangesPage = lazy(() => import("@/features/insights/AssetChangesPage").then((module) => ({ default: module.AssetChangesPage })));
const UpcomingActionsPage = lazy(() => import("@/features/upcoming-actions/UpcomingActionsPage").then(module => ({ default: module.UpcomingActionsPage })));
const AvailableFundsPage = lazy(() => import("@/features/liquidity/AvailableFundsPage").then((module) => ({ default: module.AvailableFundsPage })));

function WorkspaceLazy({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  return <Suspense fallback={<LoadingState label={t("ui.state.loadingPage")} />}>{children}</Suspense>;
}

/**
 * App gates on AppService.Startup() before any other bound service so a
 * failed database open renders BlockedStartupPage instead of a raw Wails
 * "service not found" rejection. Onboarding runs until a Household exists.
 *
 * Object-navigation source pages remain mounted until the user returns. Overview stays a static import
 * so the default landing page has no Suspense flash. Other routes load
 * through dynamic imports; server state stays in TanStack Query.
 */
function App() {
  const { t } = useTranslation();
  const [activePageId, setActivePageId] = useState<PageId>(DEFAULT_PAGE_ID);
  const [selectedAccountId, setSelectedAccountId] = useState<string | null>(null);
  const [portfolioView, setPortfolioView] = useState<PortfolioView>("holdings");
  const [accountCreateRequestId, setAccountCreateRequestId] = useState(0);
  const [returnToHoldingsAfterCreate, setReturnToHoldingsAfterCreate] = useState(false);
  const [holdingDraftRequest, setHoldingDraftRequest] = useState<{ requestId: number; accountId: string }>();
  const nextHoldingDraftRequest = useRef(0);
  const [instrumentDetail, setInstrumentDetail] = useState<InstrumentDetailContext>();
  const [reconciliationFocus,setReconciliationFocus]=useState<{accountId?:string;sessionId?:string}>({});
  const [historyFilters, setHistoryFilters] = useState<HistoryNavigationFilters | undefined>();
  const [historyActivityId, setHistoryActivityId] = useState<string>();
  const [historyRequestId, setHistoryRequestId] = useState(0);
  const nextHistoryRequestId = useRef(0);
  const handleHistoryFiltersChange = useCallback((filters: HistoryNavigationFilters) => setHistoryFilters(filters), []);
  const [accountFilter, setAccountFilter] = useState<AccountListFocus>();
  const [updateValue, setUpdateValue] = useState(false);
  const [healthFocus, setHealthFocus] = useState<HealthFocus>();
  const [marketFocus, setMarketFocus] = useState<HealthFocus>();
  const [marketFocusRequestId, setMarketFocusRequestId] = useState(0);
  const nextMarketFocusRequestId = useRef(0);
  const [visitedMaintenance, setVisitedMaintenance] = useState({ market: false, health: false });
  const [historicalDate, setHistoricalDate] = useState<string>();
  const [historicalContext, setHistoricalContext] = useState<HistoricalNavigationContext>();
  const [historicalRequest, setHistoricalRequest] = useState(0);
  const [liquidityProductId, setLiquidityProductId] = useState<string | undefined>();
  const [liquidityAccountId, setLiquidityAccountId] = useState<string | undefined>();
  const [liquidityFocus, setLiquidityFocus] = useState<LiquidityFocus>();
  const [liquidityRequestId, setLiquidityRequestId] = useState(0);
  const nextLiquidityRequestId = useRef(0);
  const [trail, setTrail] = useState<Array<{ page: PageId; accountId: string | null; filter?: AccountListFocus; health?: HealthFocus; market?: HealthFocus; marketFocusRequestId: number; portfolioView: PortfolioView; instrumentDetail?: InstrumentDetailContext; scroll: number; focus: HTMLElement | null; analysis: ReturnType<typeof useAnalysisStore.getState>; history?: HistoryNavigationFilters; historyActivityId?: string; historyRequestId?: number; liquidityProduct?: string; liquidityAccount?: string; liquidityFocus?: LiquidityFocus; liquidityRequest?: number }>>([]);
  const restorePosition = useRef<{ scroll: number; focus: HTMLElement | null; focusId?: string } | null>(null);
  useEffect(() => {
    const position = restorePosition.current;
    restorePosition.current = null;
    const frame = requestAnimationFrame(() => {
      const main = document.getElementById("main-content");
      if (main) main.scrollTop = position?.scroll ?? 0;
      const focus = position?.focus?.isConnected ? position.focus : document.getElementById(position?.focusId ?? position?.focus?.id ?? "");
      const fallback = document.getElementById(activePageId === "upcoming-actions" ? "upcoming-actions-heading" : activePageId === "overview" ? "overview-upcoming-actions-heading" : "main-content");
      (focus ?? fallback)?.focus({ preventScroll: true });
    });
    return () => cancelAnimationFrame(frame);
  }, [activePageId, trail.length]);
  const startup = useStartup();
  const settings = useSettings({ enabled: startup.data?.available === true });
  const setAccent = useUiStore((state) => state.setAccent);
  const setAppearance = useUiStore((state) => state.setAppearance);
  const setReturnView = useAnalysisStore((state) => state.setReturnView);
  const setAssetView = useAnalysisStore((state) => state.setAssetView);

  useEffect(() => {
    if (!settings.data) {
      return;
    }
    setAppearance(settings.data.appearance as Appearance);
    setAccent(settings.data.accent as Accent);
    setLanguage(settings.data.language);
  }, [settings.data, setAppearance, setAccent]);

  const workspaceReady = startup.data?.available === true && Boolean(settings.data);
  const bootstrap = useBootstrap({ enabled: workspaceReady });

  const confirmationUnresolved = hasUnresolvedSheetConfirmation();
  if (startup.isLoading) {
    return <StartupLoadingPage label={t("startup.loading")} />;
  }
  if (!confirmationUnresolved && (startup.isError || (startup.data && !startup.data.available))) {
    return <BlockedStartupPage startup={startup.data} failure={startup.error} />;
  }
  if (settings.isError && !confirmationUnresolved) {
    return <BlockedStartupPage failure={settings.error} />;
  }
  if (settings.isLoading || !settings.data) {
    return <StartupLoadingPage label={t("ui.state.loadingWorkspace")} />;
  }
  if (bootstrap.isLoading) {
    return <StartupLoadingPage label={t("ui.state.loadingWorkspace")} />;
  }
  if (bootstrap.isError && !confirmationUnresolved) {
    return <BlockedStartupPage failure={bootstrap.error} />;
  }

  if (bootstrap.data && !bootstrap.data.household && !confirmationUnresolved) {
    return <OnboardingPage onCompleted={() => setActivePageId("accounts")} />;
  }

  const openAccount = (accountId: string) => open({ page: "accounts", accountId });
  const handleNavigate = (target: NavigationTarget | PageId, retainSource = false) => {
    if (hasUnresolvedSheetConfirmation()) return;
    if (!retainSource) { setTrail([]); setReturnToHoldingsAfterCreate(false); }
    const navigation = resolveLegacyTarget(typeof target === "string" ? targetForPage(target) : target);
    const pageId = navigation.page;
    if (navigation.page === "historical-overview") { setHistoricalDate(navigation.date); setHistoricalContext(navigation.historical); setHistoricalRequest(previous => previous + 1); }
    // Top-level Accounts navigation always returns to the list. Opening a
    // specific account uses openAccount, which sets selectedAccountId first.
    if (pageId === "accounts") {
      if (navigation.page === "accounts" && navigation.create) setAccountCreateRequestId(previous => previous + 1);
      setSelectedAccountId(navigation.page === "accounts" ? navigation.accountId ?? null : null);
      setAccountFilter(navigation.page === "accounts" ? navigation.filter : undefined);
      setUpdateValue(navigation.page === "accounts" && navigation.updateValue === true);
    }
    if (navigation.page === "market-data") {
      setMarketFocus(navigation.focus);
      setMarketFocusRequestId(++nextMarketFocusRequestId.current);
      setVisitedMaintenance(previous => ({ ...previous, market: true }));
    }
    if (navigation.page === "data-health") { setHealthFocus(navigation.focus); setVisitedMaintenance(previous => ({ ...previous, health: true })); }
    if (navigation.page === "portfolio") setPortfolioView(navigation.view ?? "holdings");
    if (navigation.page === "instrument-detail") setInstrumentDetail({ instrumentId: navigation.instrumentId,
      accountId: navigation.accountId, section: navigation.section, from: navigation.from, to: navigation.to, healthFocus: navigation.healthFocus });
    if(navigation.page === "reconciliation") setReconciliationFocus({accountId:navigation.accountId,sessionId:navigation.sessionId});
    if (navigation.page === "available-funds") {
      setLiquidityProductId(navigation.productId);
      setLiquidityAccountId(navigation.accountId);
      setLiquidityFocus(navigation.focus);
      setLiquidityRequestId(++nextLiquidityRequestId.current);
    }
    if (navigation.page === "history") {
      setHistoryFilters(navigation.filters);
      setHistoryActivityId(navigation.activityId);
      setHistoryRequestId(++nextHistoryRequestId.current);
    }
    if (navigation.page === "return-analysis") {
      if (navigation.analysis) useAnalysisStore.getState().replaceFilters(navigation.analysis);
      setReturnView({ tab: navigation.tab, cursor: navigation.cursor });
    } else if (navigation.page === "asset-changes") {
      if (navigation.analysis) useAnalysisStore.getState().replaceFilters(navigation.analysis);
      setAssetView({ tab: navigation.tab, cursor: navigation.cursor });
    }
    setActivePageId(pageId);
  };

  const open = (target: NavigationTarget) => {
    if (hasUnresolvedSheetConfirmation()) return;
    if (target.page === "available-funds" && target.page === activePageId) { handleNavigate(target, true); return; }
    setReturnToHoldingsAfterCreate(target.page === "accounts" && target.create === true && activePageId === "portfolio");
    setTrail(previous => [...previous, { page: activePageId, accountId: selectedAccountId, filter: accountFilter, portfolioView, instrumentDetail,
      health: healthFocus, market: marketFocus, scroll: document.getElementById("main-content")?.scrollTop ?? 0,
      marketFocusRequestId, focus: document.activeElement instanceof HTMLElement ? document.activeElement : null, analysis: useAnalysisStore.getState(), history: historyFilters, historyActivityId, historyRequestId, liquidityProduct: liquidityProductId, liquidityAccount: liquidityAccountId, liquidityFocus, liquidityRequest: liquidityRequestId }]);
    handleNavigate(target, true);
  };
  const openHealth = (focus: HealthFocus) => {
    if (focus.accountId && !focus.instrumentId && !focus.currencyA) open({ page: "accounts", accountId: focus.accountId, updateValue: true });
    else if (focus.action === "provider_settings") open({ page: "settings" });
    else if (focus.instrumentId || (focus.currencyA && focus.currencyB)) open({ page: "market-data", focus });
    else open({ page: "data-health", focus });
  };
  const back = () => {
    if (hasUnresolvedSheetConfirmation()) return;
    const previous = trail[trail.length - 1];
    if (!previous) return;
    setReturnToHoldingsAfterCreate(false);
    restorePosition.current = previous;
    useAnalysisStore.setState(previous.analysis); setHistoryFilters(previous.history); setHistoryActivityId(previous.historyActivityId); setHistoryRequestId(previous.historyRequestId ?? 0);
    setSelectedAccountId(previous.accountId); setAccountFilter(previous.filter); setUpdateValue(false);
    setHealthFocus(previous.health); setMarketFocus(previous.market);
    setMarketFocusRequestId(previous.marketFocusRequestId);
    setPortfolioView(previous.portfolioView); setInstrumentDetail(previous.instrumentDetail);
    setLiquidityProductId(previous.liquidityProduct); setLiquidityAccountId(previous.liquidityAccount); setLiquidityFocus(previous.liquidityFocus); setLiquidityRequestId(previous.liquidityRequest ?? 0);
    setActivePageId(previous.page); setTrail(trail.slice(0, -1));
  };
  const onAccountCreated = (accountId: string, tracksHoldings: boolean) => {
    if (!returnToHoldingsAfterCreate || trail[trail.length - 1]?.page !== "portfolio") return false;
    if (!tracksHoldings) { setReturnToHoldingsAfterCreate(false); return false; }
    back();
    setHoldingDraftRequest({ requestId: ++nextHoldingDraftRequest.current, accountId });
    return true;
  };
  const retained = (page: PageId) => activePageId === page || (activePageId !== "historical-overview" && trail.some(entry => entry.page === page));
  return (
    <NavigationContext.Provider value={{ open, openHealth }}>
    <AppShell activePageId={activePageId} onNavigate={handleNavigate} settings={settings.data}>
      <MarketDataSyncWorkspaceObserver />
      <AgentWorkspaceObserver />
      {trail.length > 0 && <Button variant="ghost" className="mb-4" onClick={back}>{t("connections.back", { page: trail[trail.length - 1].page === "instrument-detail" ? t("instrumentDetail.title") : t(`nav.${({ "historical-overview": "historicalOverview", "data-health": "dataHealth", "market-data": "marketData", "return-analysis": "returnAnalysis", "asset-changes": "assetChanges", "available-funds": "availableFunds", "upcoming-actions": "upcomingActions" } as Record<string, string>)[trail[trail.length - 1].page] ?? trail[trail.length - 1].page}`) })}</Button>}
      {retained("overview") && (
        <div hidden={activePageId !== "overview"}>
        <OverviewPage
          isActive={activePageId === "overview"}
          onAddAccount={() => handleNavigate("accounts")}
          onOpenAccounts={() => handleNavigate("accounts")}
          onOpenHistory={() => handleNavigate("history")}
          onOpenMarketData={() => handleNavigate("market-data")}
          onOpenDataHealth={() => handleNavigate("data-health")}
        />
        </div>
      )}
      {retained("accounts") && (
        <div hidden={activePageId !== "accounts"}>
        <AccountsPage navigationFilter={accountFilter} onClearFilter={() => setAccountFilter(undefined)} updateValue={updateValue} selectedAccountId={selectedAccountId} onSelectAccount={setSelectedAccountId} createRequestId={accountCreateRequestId} onCreated={onAccountCreated} />
        </div>
      )}
      {retained("available-funds") && (
        <div hidden={activePageId !== "available-funds"}>
        <WorkspaceLazy>
          <AvailableFundsPage productId={liquidityProductId} accountId={liquidityAccountId} focus={liquidityFocus} navigationRequestId={liquidityRequestId} isActive={activePageId === "available-funds"} />
        </WorkspaceLazy>
        </div>
      )}
      {retained("reconciliation") && <div hidden={activePageId !== "reconciliation"}><WorkspaceLazy><ReconciliationPage key={`${reconciliationFocus.accountId}:${reconciliationFocus.sessionId}`} {...reconciliationFocus} /></WorkspaceLazy></div>}
      {retained("upcoming-actions") && <div hidden={activePageId !== "upcoming-actions"}><WorkspaceLazy><UpcomingActionsPage isActive={activePageId === "upcoming-actions"} /></WorkspaceLazy></div>}
      {retained("portfolio") && (
        <div hidden={activePageId !== "portfolio"}>
        <WorkspaceLazy>
          <PortfolioPage onOpenAccount={openAccount} view={portfolioView} onViewChange={setPortfolioView} active={activePageId === "portfolio"} holdingDraftRequest={holdingDraftRequest} />
        </WorkspaceLazy>
        </div>
      )}
      {activePageId === "instrument-detail" && instrumentDetail && <WorkspaceLazy>
        <InstrumentDetailPage key={`${instrumentDetail.instrumentId}:${instrumentDetail.accountId ?? ""}`} context={instrumentDetail} onContextChange={setInstrumentDetail} />
      </WorkspaceLazy>}
      {retained("directory") && (
        <div hidden={activePageId !== "directory"}>
        <WorkspaceLazy>
          <DirectoryPage />
        </WorkspaceLazy>
        </div>
      )}
      {(retained("market-data") || retained("data-health")) && <div hidden={activePageId !== "market-data" && activePageId !== "data-health"}>
        <Tabs value={activePageId === "market-data" || activePageId === "data-health" ? activePageId : "none"}
          onValueChange={(value) => {
            if (hasUnresolvedSheetConfirmation()) return;
            setVisitedMaintenance(previous => ({ ...previous, [value === "data-health" ? "health" : "market"]: true }));
            setActivePageId(value === "data-health" ? "data-health" : "market-data");
          }}>
          <TabsList aria-label={t("nav.marketData")}>
            <TabsTrigger value="data-health">{t("dataMaintenance.issues")}</TabsTrigger>
            <TabsTrigger value="market-data">{t("dataMaintenance.quotes")}</TabsTrigger>
          </TabsList>
          <TabsContent value="market-data" keepMounted>
            {visitedMaintenance.market && <WorkspaceLazy>
              <MarketDataPage focus={marketFocus} focusRequestId={marketFocusRequestId} onOpenDataHealth={() => open({ page: "data-health", focus: marketFocus })} />
            </WorkspaceLazy>}
          </TabsContent>
          <TabsContent value="data-health" keepMounted>
            {visitedMaintenance.health && <WorkspaceLazy>
              <DataHealthPage focus={healthFocus}
                onOpenSettings={() => handleNavigate("settings")}
                onOpenMarketData={() => open({ page: "market-data", focus: healthFocus })}
              />
            </WorkspaceLazy>}
          </TabsContent>
        </Tabs>
      </div>}
      {activePageId === "settings" && (
        <WorkspaceLazy>
          <SettingsPage />
        </WorkspaceLazy>
      )}
      {retained("historical-overview") && <div hidden={activePageId !== "historical-overview"}><WorkspaceLazy><HistoricalOverviewPage key={historicalRequest} initialDate={historicalDate} initialContext={historicalContext} isActive={activePageId === "historical-overview"} onExit={() => {
        if (trail.length > 0) back();
        else { restorePosition.current = { scroll: 0, focus: null, focusId: "open-historical-overview" }; handleNavigate("overview"); }
      }} /></WorkspaceLazy></div>}
      {activePageId === "history" && (
        <WorkspaceLazy>
          <HistoryPage navigationFilters={historyFilters} navigationActivityId={historyActivityId} navigationRequestId={historyRequestId} onFiltersChange={handleHistoryFiltersChange} onNavigationActivityClose={() => setHistoryActivityId(undefined)} />
        </WorkspaceLazy>
      )}
      {retained("return-analysis") && (
        <div hidden={activePageId !== "return-analysis"}>
        <WorkspaceLazy>
          <ReturnAnalysisPage onOpenAssetChanges={(analysis) => open({ page: "asset-changes", tab: "drivers", analysis })} onOpenHistory={(filters) => open({ page: "history", filters })} />
        </WorkspaceLazy>
        </div>
      )}
      {retained("asset-changes") && (
        <div hidden={activePageId !== "asset-changes"}>
        <WorkspaceLazy>
          <AssetChangesPage onOpenHistory={(filters, activityId) => open({ page: "history", filters, activityId })} onOpenReturnAnalysis={(analysis) => open({ page: "return-analysis", tab: "contribution", analysis })} />
        </WorkspaceLazy>
        </div>
      )}
    </AppShell>
    </NavigationContext.Provider>
  );
}

export default App;
