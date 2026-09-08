import { lazy, Suspense, useEffect, useState, type ReactNode } from "react";
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
import { useUiStore, type Appearance } from "@/stores/ui";
import { setLanguage } from "@/i18n";
import { targetForPage, type HistoryNavigationFilters, type NavigationTarget, type PageId } from "@/app/navigation";
import { useAnalysisStore } from "@/stores/analysis";

const PortfolioPage = lazy(() => import("@/features/portfolio/PortfolioPage").then((module) => ({ default: module.PortfolioPage })));
const DirectoryPage = lazy(() => import("@/features/directory/DirectoryPage").then((module) => ({ default: module.DirectoryPage })));
const InvestmentsPage = lazy(() => import("@/features/investments/InvestmentsPage").then((module) => ({ default: module.InvestmentsPage })));
const MarketDataPage = lazy(() => import("@/features/marketdata/MarketDataPage").then((module) => ({ default: module.MarketDataPage })));
const SettingsPage = lazy(() => import("@/features/settings/SettingsPage").then((module) => ({ default: module.SettingsPage })));
const HistoryPage = lazy(() => import("@/features/history/HistoryPage").then((module) => ({ default: module.HistoryPage })));
const AnalyticsPage = lazy(() => import("@/features/analytics/AnalyticsPage").then((module) => ({ default: module.AnalyticsPage })));
const ReturnAnalysisPage = lazy(() => import("@/features/insights/ReturnAnalysisPage").then((module) => ({ default: module.ReturnAnalysisPage })));
const AssetChangesPage = lazy(() => import("@/features/insights/AssetChangesPage").then((module) => ({ default: module.AssetChangesPage })));

function WorkspaceLazy({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  return <Suspense fallback={<LoadingState label={t("ui.state.loadingPage")} />}>{children}</Suspense>;
}

/**
 * App gates on AppService.Startup() before any other bound service so a
 * failed database open renders BlockedStartupPage instead of a raw Wails
 * "service not found" rejection. Onboarding runs until a Household exists.
 *
 * Only the active workspace page is mounted. Overview stays a static import
 * so the default landing page has no Suspense flash. Other routes load
 * through dynamic imports; server state stays in TanStack Query.
 */
function App() {
  const { t } = useTranslation();
  const [activePageId, setActivePageId] = useState<PageId>(DEFAULT_PAGE_ID);
  const [selectedAccountId, setSelectedAccountId] = useState<string | null>(null);
  const [historyFilters, setHistoryFilters] = useState<HistoryNavigationFilters | undefined>();
  const startup = useStartup();
  const settings = useSettings({ enabled: startup.data?.available === true });
  const setAppearance = useUiStore((state) => state.setAppearance);
  const setAnalysisFilters = useAnalysisStore((state) => state.setFilters);
  const setReturnView = useAnalysisStore((state) => state.setReturnView);
  const setAssetView = useAnalysisStore((state) => state.setAssetView);

  useEffect(() => {
    if (!settings.data) {
      return;
    }
    setAppearance(settings.data.appearance as Appearance);
    setLanguage(settings.data.language);
  }, [settings.data, setAppearance]);

  const workspaceReady = startup.data?.available === true && Boolean(settings.data);
  const bootstrap = useBootstrap({ enabled: workspaceReady });

  if (startup.isLoading) {
    return <StartupLoadingPage label={t("startup.loading")} />;
  }
  if (startup.isError || (startup.data && !startup.data.available)) {
    return <BlockedStartupPage startup={startup.data} failure={startup.error} />;
  }
  if (settings.isLoading || !settings.data) {
    return <StartupLoadingPage label={t("ui.state.loadingWorkspace")} />;
  }
  if (settings.isError || !settings.data) {
    return <BlockedStartupPage failure={settings.error} />;
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

  const openAccount = (accountId: string) => {
    setSelectedAccountId(accountId);
    setActivePageId("accounts");
  };
  const handleNavigate = (target: NavigationTarget | PageId) => {
    const navigation = typeof target === "string" ? targetForPage(target) : target;
    const pageId = navigation.page;
    // Top-level Accounts navigation always returns to the list. Opening a
    // specific account uses openAccount, which sets selectedAccountId first.
    if (pageId === "accounts") {
      setSelectedAccountId(null);
    }
    if (navigation.page === "history") {
      setHistoryFilters(navigation.filters);
    }
    if (navigation.page === "return-analysis") {
      if (navigation.analysis) setAnalysisFilters(navigation.analysis);
      setReturnView({ tab: navigation.tab, cursor: navigation.cursor });
    } else if (navigation.page === "asset-changes") {
      if (navigation.analysis) setAnalysisFilters(navigation.analysis);
      setAssetView({ tab: navigation.tab, cursor: navigation.cursor });
    }
    setActivePageId(pageId);
  };

  return (
    <AppShell activePageId={activePageId} onNavigate={handleNavigate} settings={settings.data}>
      {activePageId === "overview" && (
        <OverviewPage
          onAddAccount={() => handleNavigate("accounts")}
          onOpenAccounts={() => handleNavigate("accounts")}
          onOpenHistory={() => handleNavigate("history")}
          onOpenMarketData={() => handleNavigate("market-data")}
          onOpenInvestments={() => handleNavigate("investments")}
        />
      )}
      {activePageId === "accounts" && (
        <AccountsPage selectedAccountId={selectedAccountId} onSelectAccount={setSelectedAccountId} />
      )}
      {activePageId === "portfolio" && (
        <WorkspaceLazy>
          <PortfolioPage onOpenAccount={openAccount} />
        </WorkspaceLazy>
      )}
      {activePageId === "directory" && (
        <WorkspaceLazy>
          <DirectoryPage />
        </WorkspaceLazy>
      )}
      {activePageId === "investments" && (
        <WorkspaceLazy>
          <InvestmentsPage />
        </WorkspaceLazy>
      )}
      {activePageId === "market-data" && (
        <WorkspaceLazy>
          <MarketDataPage />
        </WorkspaceLazy>
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
      {activePageId === "analytics" && (
        <WorkspaceLazy>
          <AnalyticsPage />
        </WorkspaceLazy>
      )}
      {activePageId === "return-analysis" && (
        <WorkspaceLazy>
          <ReturnAnalysisPage onOpenAssetChanges={(analysis) => handleNavigate({ page: "asset-changes", tab: "drivers", analysis })} onOpenHistory={(filters) => handleNavigate({ page: "history", filters })} />
        </WorkspaceLazy>
      )}
      {activePageId === "asset-changes" && (
        <WorkspaceLazy>
          <AssetChangesPage onOpenHistory={(filters) => handleNavigate({ page: "history", filters })} />
        </WorkspaceLazy>
      )}
    </AppShell>
  );
}

export default App;
