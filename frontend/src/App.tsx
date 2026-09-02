import { useEffect, useState } from "react";
import { AppShell, DEFAULT_PAGE_ID } from "@/app/AppShell";
import { OnboardingPage } from "@/features/onboarding/OnboardingPage";
import { OverviewPage } from "@/features/overview/OverviewPage";
import { AccountsPage } from "@/features/accounts/AccountsPage";
import { PortfolioPage } from "@/features/portfolio/PortfolioPage";
import { DirectoryPage } from "@/features/directory/DirectoryPage";
import { InvestmentsPage } from "@/features/investments/InvestmentsPage";
import { MarketDataPage } from "@/features/marketdata/MarketDataPage";
import { SettingsPage } from "@/features/settings/SettingsPage";
import { HistoryPage } from "@/features/history/HistoryPage";
import { AnalyticsPage } from "@/features/analytics/AnalyticsPage";
import { BlockedStartupPage, StartupLoadingPage } from "@/features/startup/BlockedStartupPage";
import { useTranslation } from "react-i18next";
import { useBootstrap } from "@/queries/household";
import { useStartup } from "@/queries/app";
import { useSettings } from "@/queries/settings";
import { useUiStore, type Appearance } from "@/stores/ui";
import { setLanguage } from "@/i18n";

/**
 * App gates on AppService.Startup() before any other bound service so a
 * failed database open renders BlockedStartupPage instead of a raw Wails
 * "service not found" rejection. Onboarding runs until a Household exists.
 *
 * Only the active workspace page is mounted. Server state stays in TanStack
 * Query; selectedAccountId is App-owned UI state that survives leaving
 * Accounts. Hidden pages must not keep query observers or effects alive.
 */
function App() {
  const { t } = useTranslation();
  const [activePageId, setActivePageId] = useState(DEFAULT_PAGE_ID);
  const [selectedAccountId, setSelectedAccountId] = useState<string | null>(null);
  const startup = useStartup();
  const settings = useSettings({ enabled: startup.data?.available === true });
  const setAppearance = useUiStore((state) => state.setAppearance);

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
  const handleNavigate = (pageId: string) => {
    // Top-level Accounts navigation always returns to the list. Opening a
    // specific account uses openAccount, which sets selectedAccountId first.
    if (pageId === "accounts") {
      setSelectedAccountId(null);
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
      {activePageId === "portfolio" && <PortfolioPage onOpenAccount={openAccount} />}
      {activePageId === "directory" && <DirectoryPage />}
      {activePageId === "investments" && <InvestmentsPage />}
      {activePageId === "market-data" && <MarketDataPage />}
      {activePageId === "settings" && <SettingsPage />}
      {activePageId === "history" && <HistoryPage />}
      {activePageId === "analytics" && <AnalyticsPage />}
    </AppShell>
  );
}

export default App;
