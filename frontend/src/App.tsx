import { useState } from "react";
import { AppShell, DEFAULT_PAGE_ID } from "@/app/AppShell";
import { ComingSoonPage } from "@/components/layout/ComingSoon";
import { NAV_ITEMS } from "@/app/navigation";

/**
 * App renders the persistent AppShell with a "coming soon" placeholder
 * for every nav destination (implementation plan Phase 3). Phase 4/5
 * replace each placeholder with its real feature page one at a time;
 * this file's job through Phase 3 is only to prove the shell, theme,
 * language switching, and simple page-state navigation work together.
 */
function App() {
  const [activePageId, setActivePageId] = useState(DEFAULT_PAGE_ID);
  const activeItem = NAV_ITEMS.find((item) => item.id === activePageId) ?? NAV_ITEMS[0];

  return (
    <AppShell activePageId={activePageId} onNavigate={setActivePageId}>
      <ComingSoonPage titleKey={activeItem.translationKey} />
    </AppShell>
  );
}

export default App;
