import type { LucideIcon } from "lucide-react";
import { House, Wallet, ChartNoAxesCombined, RefreshCw, Users, History, ChartLine, Settings } from "lucide-react";

/**
 * NavItem is the top-level navigation model. `translationKey` looks up the
 * label in the i18n `nav.*` namespace.
 */
export interface NavItem {
  id: string;
  translationKey: string;
  icon: LucideIcon;
}

export const NAV_ITEMS: NavItem[] = [
  { id: "overview", translationKey: "nav.overview", icon: House },
  { id: "accounts", translationKey: "nav.accounts", icon: Wallet },
  { id: "investments", translationKey: "nav.investments", icon: ChartNoAxesCombined },
  { id: "market-data", translationKey: "nav.marketData", icon: RefreshCw },
  { id: "directory", translationKey: "nav.directory", icon: Users },
  { id: "history", translationKey: "nav.history", icon: History },
  { id: "analytics", translationKey: "nav.analytics", icon: ChartLine },
  { id: "settings", translationKey: "nav.settings", icon: Settings },
];

export const DEFAULT_PAGE_ID = "overview";
