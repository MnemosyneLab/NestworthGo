import { useEffect, useState } from "react";
import { AppShell, DEFAULT_PAGE_ID } from "@/app/AppShell";
import { ComingSoonPage } from "@/components/layout/ComingSoon";
import { NAV_ITEMS } from "@/app/navigation";
import { OnboardingPage } from "@/features/onboarding/OnboardingPage";
import { OverviewPage } from "@/features/overview/OverviewPage";
import { AccountsPage } from "@/features/accounts/AccountsPage";
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
 */
function App() {
  const { t } = useTranslation();
  const [activePageId, setActivePageId] = useState(DEFAULT_PAGE_ID);
  const activeItem = NAV_ITEMS.find((item) => item.id === activePageId) ?? NAV_ITEMS[0];
  const startup = useStartup();
  const settings = useSettings({ enabled: startup.data?.available === true });
  const setAppearance = useUiStore((state) => state.setAppearance);
  const [settingsHydrated, setSettingsHydrated] = useState(false);

  useEffect(() => {
    if (!settings.data) {
      return;
    }
    setAppearance(settings.data.appearance as Appearance);
    setLanguage(settings.data.language);
    setSettingsHydrated(true);
  }, [settings.data, setAppearance]);

  const workspaceReady = startup.data?.available === true && settingsHydrated && Boolean(settings.data);
  const bootstrap = useBootstrap({ enabled: workspaceReady });

  if (startup.isLoading) {
    return <StartupLoadingPage label={t("startup.loading")} />;
  }
  if (startup.isError || (startup.data && !startup.data.available)) {
    return <BlockedStartupPage startup={startup.data} failure={startup.error} />;
  }
  if (settings.isLoading || !settingsHydrated) {
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

  const implementedPageIds = ["overview", "accounts", "directory", "investments", "market-data", "settings", "history", "analytics"];

  return (
    <AppShell activePageId={activePageId} onNavigate={setActivePageId} settings={settings.data}>
      {activePageId === "overview" && (
        <OverviewPage
          onAddAccount={() => setActivePageId("accounts")}
          onOpenAccounts={() => setActivePageId("accounts")}
          onOpenHistory={() => setActivePageId("history")}
          onOpenMarketData={() => setActivePageId("market-data")}
        />
      )}
      {activePageId === "accounts" && <AccountsPage />}
      {activePageId === "directory" && <DirectoryPage />}
      {activePageId === "investments" && <InvestmentsPage />}
      {activePageId === "market-data" && <MarketDataPage />}
      {activePageId === "settings" && <SettingsPage />}
      {activePageId === "history" && <HistoryPage />}
      {activePageId === "analytics" && <AnalyticsPage />}
      {!implementedPageIds.includes(activePageId) && <ComingSoonPage titleKey={activeItem.translationKey} />}
    </AppShell>
  );
}

export default App;
