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

export interface NavGroup {
  id: string;
  translationKey?: string;
  items: NavItem[];
}

export const NAV_GROUPS: NavGroup[] = [
  {
    id: "overview",
    items: [{ id: "overview", translationKey: "nav.overview", icon: House }],
  },
  {
    id: "portfolio",
    translationKey: "navGroups.portfolio",
    items: [
      { id: "accounts", translationKey: "nav.accounts", icon: Wallet },
      { id: "investments", translationKey: "nav.investments", icon: ChartNoAxesCombined },
    ],
  },
  {
    id: "activity",
    translationKey: "navGroups.activity",
    items: [{ id: "history", translationKey: "nav.history", icon: History }],
  },
  {
    id: "insights",
    translationKey: "navGroups.insights",
    items: [{ id: "analytics", translationKey: "nav.analytics", icon: ChartLine }],
  },
  {
    id: "manage",
    translationKey: "navGroups.manage",
    items: [{ id: "directory", translationKey: "nav.directory", icon: Users }],
  },
  {
    id: "settings",
    translationKey: "navGroups.settings",
    items: [
      { id: "market-data", translationKey: "nav.marketData", icon: RefreshCw },
      { id: "settings", translationKey: "nav.settings", icon: Settings },
    ],
  },
];

export const NAV_ITEMS: NavItem[] = NAV_GROUPS.flatMap((group) => group.items);

export const DEFAULT_PAGE_ID = "overview";
