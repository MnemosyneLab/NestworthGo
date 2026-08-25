import { useState } from "react";
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
import { useBootstrap } from "@/queries/household";

/**
 * App renders Onboarding until a Household exists, then the persistent
 * AppShell with a real page for Overview/Accounts (implementation plan
 * Phase 4's vertical slice) and a "coming soon" placeholder for every
 * other nav destination (Phase 5's remaining work).
 */
function App() {
  const [activePageId, setActivePageId] = useState(DEFAULT_PAGE_ID);
  const activeItem = NAV_ITEMS.find((item) => item.id === activePageId) ?? NAV_ITEMS[0];
  const bootstrap = useBootstrap();

  if (bootstrap.isLoading) {
    return null;
  }

  if (!bootstrap.isError && bootstrap.data && !bootstrap.data.household) {
    return <OnboardingPage />;
  }

  const implementedPageIds = ["overview", "accounts", "directory", "investments", "market-data", "settings"];

  return (
    <AppShell activePageId={activePageId} onNavigate={setActivePageId}>
      {activePageId === "overview" && <OverviewPage />}
      {activePageId === "accounts" && <AccountsPage />}
      {activePageId === "directory" && <DirectoryPage />}
      {activePageId === "investments" && <InvestmentsPage />}
      {activePageId === "market-data" && <MarketDataPage />}
      {activePageId === "settings" && <SettingsPage />}
      {!implementedPageIds.includes(activePageId) && <ComingSoonPage titleKey={activeItem.translationKey} />}
    </AppShell>
  );
}

export default App;
