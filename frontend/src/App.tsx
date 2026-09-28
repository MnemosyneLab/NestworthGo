import { lazy, Suspense, useEffect, useState, useRef, type ReactNode } from "react";
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
import { targetForPage, type HistoryNavigationFilters, type NavigationTarget, type PageId } from "@/app/navigation";
import { useAnalysisStore } from "@/stores/analysis";
import { Button } from "@/components/ui/button";
import { NavigationContext } from "@/app/NavigationContext";
import type { HealthFocus, AccountListFocus } from "@/app/navigation";
import { MarketDataSyncWorkspaceObserver } from "@/queries/marketdata";

const PortfolioPage = lazy(() => import("@/features/portfolio/PortfolioPage").then((module) => ({ default: module.PortfolioPage })));
const DirectoryPage = lazy(() => import("@/features/directory/DirectoryPage").then((module) => ({ default: module.DirectoryPage })));
const MarketDataPage = lazy(() => import("@/features/marketdata/MarketDataPage").then((module) => ({ default: module.MarketDataPage })));
const DataHealthPage = lazy(() => import("@/features/data-health/DataHealthPage").then((module) => ({ default: module.DataHealthPage })));
const SettingsPage = lazy(() => import("@/features/settings/SettingsPage").then((module) => ({ default: module.SettingsPage })));
const HistoryPage = lazy(() => import("@/features/history/HistoryPage").then((module) => ({ default: module.HistoryPage })));
const ReturnAnalysisPage = lazy(() => import("@/features/insights/ReturnAnalysisPage").then((module) => ({ default: module.ReturnAnalysisPage })));
const AssetChangesPage = lazy(() => import("@/features/insights/AssetChangesPage").then((module) => ({ default: module.AssetChangesPage })));
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
  const [historyFilters, setHistoryFilters] = useState<HistoryNavigationFilters | undefined>();
  const [accountFilter, setAccountFilter] = useState<AccountListFocus>();
  const [updateValue, setUpdateValue] = useState(false);
  const [healthFocus, setHealthFocus] = useState<HealthFocus>();
  const [marketFocus, setMarketFocus] = useState<HealthFocus>();
  const [liquidityProductId, setLiquidityProductId] = useState<string | undefined>();
  const [liquidityAccountId, setLiquidityAccountId] = useState<string | undefined>();
  const [trail, setTrail] = useState<Array<{ page: PageId; accountId: string | null; filter?: AccountListFocus; health?: HealthFocus; market?: HealthFocus; scroll: number; focus: HTMLElement | null; analysis: ReturnType<typeof useAnalysisStore.getState>; history?: HistoryNavigationFilters }>>([]);
  const restorePosition = useRef<{ scroll: number; focus: HTMLElement | null } | null>(null);
  useEffect(() => {
    const position = restorePosition.current;
    restorePosition.current = null;
    const frame = requestAnimationFrame(() => {
      const main = document.getElementById("main-content");
      if (main) main.scrollTop = position?.scroll ?? 0;
      if (position?.focus?.isConnected) position.focus.focus({ preventScroll: true });
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

  if (startup.isLoading) {
    return <StartupLoadingPage label={t("startup.loading")} />;
  }
  if (startup.isError || (startup.data && !startup.data.available)) {
    return <BlockedStartupPage startup={startup.data} failure={startup.error} />;
  }
  if (settings.isError) {
    return <BlockedStartupPage failure={settings.error} />;
  }
  if (settings.isLoading || !settings.data) {
    return <StartupLoadingPage label={t("ui.state.loadingWorkspace")} />;
  }
  if (bootstrap.isLoading) {
    return <StartupLoadingPage label={t("ui.state.loadingWorkspace")} />;
  }
  if (bootstrap.isError) {
    return <BlockedStartupPage failure={bootstrap.error} />;
  }

  if (bootstrap.data && !bootstrap.data.household) {
    return <OnboardingPage onCompleted={() => setActivePageId("accounts")} />;
  }

  const openAccount = (accountId: string) => open({ page: "accounts", accountId });
  const handleNavigate = (target: NavigationTarget | PageId, retainSource = false) => {
    if (!retainSource) setTrail([]);
    const navigation = typeof target === "string" ? targetForPage(target) : target;
    const pageId = navigation.page;
    // Top-level Accounts navigation always returns to the list. Opening a
    // specific account uses openAccount, which sets selectedAccountId first.
    if (pageId === "accounts") {
      setSelectedAccountId(navigation.page === "accounts" ? navigation.accountId ?? null : null);
      setAccountFilter(navigation.page === "accounts" ? navigation.filter : undefined);
      setUpdateValue(navigation.page === "accounts" && navigation.updateValue === true);
    }
    if (navigation.page === "market-data") setMarketFocus(navigation.focus);
    if (navigation.page === "data-health") setHealthFocus(navigation.focus);
    if (navigation.page === "available-funds") {
      setLiquidityProductId(navigation.productId);
      setLiquidityAccountId(navigation.accountId);
    }
    if (navigation.page === "history") {
      setHistoryFilters(navigation.filters);
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
    setTrail(previous => [...previous, { page: activePageId, accountId: selectedAccountId, filter: accountFilter,
      health: healthFocus, market: marketFocus, scroll: document.getElementById("main-content")?.scrollTop ?? 0,
      focus: document.activeElement instanceof HTMLElement ? document.activeElement : null, analysis: useAnalysisStore.getState(), history: historyFilters }]);
    handleNavigate(target, true);
  };
  const openHealth = (focus: HealthFocus) => {
    if (focus.accountId && !focus.instrumentId && !focus.currencyA) open({ page: "accounts", accountId: focus.accountId, updateValue: true });
    else if (focus.action === "provider_settings") open({ page: "settings" });
    else if (focus.instrumentId || (focus.currencyA && focus.currencyB)) open({ page: "market-data", focus });
    else open({ page: "data-health", focus });
  };
  const back = () => {
    const previous = trail[trail.length - 1];
    if (!previous) return;
    restorePosition.current = previous;
    useAnalysisStore.setState(previous.analysis); setHistoryFilters(previous.history);
    setSelectedAccountId(previous.accountId); setAccountFilter(previous.filter); setUpdateValue(false);
    setHealthFocus(previous.health); setMarketFocus(previous.market);
    setActivePageId(previous.page); setTrail(trail.slice(0, -1));
  };
  const retained = (page: PageId) => activePageId === page || trail.some(entry => entry.page === page);
  return (
    <NavigationContext.Provider value={{ open, openHealth }}>
    <AppShell activePageId={activePageId} onNavigate={handleNavigate} settings={settings.data}>
      <MarketDataSyncWorkspaceObserver />
      {trail.length > 0 && <Button variant="ghost" className="mb-4" onClick={back}>{t("connections.back", { page: t(`nav.${({ "data-health": "dataHealth", "market-data": "marketData", "return-analysis": "returnAnalysis", "asset-changes": "assetChanges", "available-funds": "availableFunds" } as Record<string, string>)[trail[trail.length - 1].page] ?? trail[trail.length - 1].page}`) })}</Button>}
      {retained("overview") && (
        <div hidden={activePageId !== "overview"}>
        <OverviewPage
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
        <AccountsPage navigationFilter={accountFilter} onClearFilter={() => setAccountFilter(undefined)} updateValue={updateValue} selectedAccountId={selectedAccountId} onSelectAccount={setSelectedAccountId} />
        </div>
      )}
      {retained("available-funds") && (
        <div hidden={activePageId !== "available-funds"}>
        <WorkspaceLazy>
          <AvailableFundsPage key={`${liquidityProductId ?? ""}:${liquidityAccountId ?? ""}`} productId={liquidityProductId} accountId={liquidityAccountId} />
        </WorkspaceLazy>
        </div>
      )}
      {retained("portfolio") && (
        <div hidden={activePageId !== "portfolio"}>
        <WorkspaceLazy>
          <PortfolioPage onOpenAccount={openAccount} />
        </WorkspaceLazy>
        </div>
      )}
      {retained("directory") && (
        <div hidden={activePageId !== "directory"}>
        <WorkspaceLazy>
          <DirectoryPage />
        </WorkspaceLazy>
        </div>
      )}
      {activePageId === "market-data" && (
        <WorkspaceLazy>
          <MarketDataPage key={JSON.stringify(marketFocus)} focus={marketFocus} onOpenDataHealth={() => handleNavigate("data-health")} />
        </WorkspaceLazy>
      )}
      {retained("data-health") && (
        <div hidden={activePageId !== "data-health"}>
        <WorkspaceLazy>
          <DataHealthPage focus={healthFocus}
            onOpenSettings={() => handleNavigate("settings")}
            onOpenMarketData={() => handleNavigate("market-data")}
          />
        </WorkspaceLazy>
        </div>
      )}
      {activePageId === "settings" && (
        <WorkspaceLazy>
          <SettingsPage />
        </WorkspaceLazy>
      )}
      {activePageId === "history" && (
        <WorkspaceLazy>
          <HistoryPage navigationFilters={historyFilters} />
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
          <AssetChangesPage onOpenHistory={(filters) => open({ page: "history", filters })} onOpenReturnAnalysis={(analysis) => open({ page: "return-analysis", tab: "contribution", analysis })} />
        </WorkspaceLazy>
        </div>
      )}
    </AppShell>
    </NavigationContext.Provider>
  );
}

export default App;
