import type { SourceRefDTO } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
import type { LucideIcon } from "lucide-react";
import { House, Wallet, PieChart, RefreshCw, Users, History, ChartLine, Settings, GitCompareArrows, Banknote } from "lucide-react";

export type PageId =
  | "overview"
  | "historical-overview"
  | "accounts"
  | "available-funds"
  | "upcoming-actions"
  | "portfolio"
  | "instrument-detail"
  | "reconciliation"
  | "history"
  | "return-analysis"
  | "asset-changes"
  | "directory"
  | "market-data"
  | "data-health"
  | "settings";

export type AnalysisTab = "calendar" | "trend" | "contribution";
export type AssetChangesTab = "drivers" | "trend" | "categories";

export interface AnalysisNavigationContext {
  scope?: "portfolio" | "account" | "instrument";
  scopeId?: string;
  valuation?: "native" | "base";
  includeCash?: boolean;
  from?: string;
  to?: string;
  moreFilters?: Record<string, string | boolean | undefined>;
  returnType?: "total_return" | "realized" | "dividend_interest";
}

export interface HistoryNavigationFilters {
  kinds?: string[];
  accountId?: string;
  instrumentId?: string;
  from?: string;
  to?: string;
}

export interface HealthFocus {
  id?: string;
  instrumentId?: string;
  accountId?: string;
  currencyA?: string;
  currencyB?: string;
  rangeStart?: string;
  rangeEnd?: string;
  label?: string;
  reason?: string;
  action?: string;
  kind?: string;
  executable?: boolean;
  // Original component label for a grouped snapshot range; separate from its display title.
  snapshotComponentLabel?: string;
}

export type PortfolioView = "holdings" | "instruments" | "allocation";

/** Shared, read-only instrument destination used by holdings, accounts and data maintenance. */
export interface InstrumentDetailContext {
  instrumentId: string;
  accountId?: string;
  section?: "holdings" | "performance" | "data";
  from?: string;
  to?: string;
  healthFocus?: HealthFocus;
}

export interface AccountListFocus {
  dimension: "institutionId" | "accountType" | "memberId" | "groupId";
  value: string;
  label: string;
}

export type LiquidityFocus =
  | { kind: "product"; productId: string }
  | { kind: "receipt"; productId: string }
  | { kind: "policy"; sourceRef: SourceRefDTO }
  | { kind: "reservations"; sourceRef?: SourceRefDTO; reservationId?: string };

export interface HistoricalCategoryFilter {
  role: "asset" | "liability";
  dimension: "byClass" | "byCurrency";
  key: string;
}

export interface HistoricalNavigationContext {
  compareTo?: string;
  view?: "allocation" | "delta" | "accounts" | "trend";
  categoryFilter?: HistoricalCategoryFilter;
  changesOnly?: boolean;
  categoryRole?: "asset" | "liability";
  dimension?: "byClass" | "byCurrency";
  expanded?: string[];
  fullTable?: boolean;
}

export type NavigationTarget =
  | { page: "historical-overview"; date?: string; historical?: HistoricalNavigationContext }
  | { page: "return-analysis"; tab?: AnalysisTab; cursor?: string; analysis?: AnalysisNavigationContext }
  | { page: "asset-changes"; tab?: AssetChangesTab; cursor?: string; analysis?: AnalysisNavigationContext }
  | { page: "reconciliation"; accountId?: string; sessionId?: string }
  | { page: "history"; filters?: HistoryNavigationFilters; activityId?: string }
  | { page: "accounts"; accountId?: string; filter?: AccountListFocus; updateValue?: boolean }
  | { page: "portfolio"; view?: PortfolioView }
  | ({ page: "instrument-detail" } & InstrumentDetailContext)
  | { page: "market-data" | "data-health"; focus?: HealthFocus }
  | { page: "available-funds"; productId?: string; accountId?: string; focus?: LiquidityFocus }
  | { page: Exclude<PageId, "historical-overview" | "return-analysis" | "asset-changes" | "history" | "reconciliation" | "accounts" | "portfolio" | "instrument-detail" | "market-data" | "data-health" | "available-funds"> };

/**
 * NavItem is the top-level navigation model. `translationKey` looks up the
 * label in the i18n `nav.*` namespace.
 */
export interface NavItem {
  id: PageId;
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
    id: "workspace",
    items: [
      { id: "accounts", translationKey: "nav.accounts", icon: Wallet },
      { id: "available-funds", translationKey: "nav.availableFunds", icon: Banknote },
      { id: "portfolio", translationKey: "nav.portfolio", icon: PieChart },
    ],
  },
  {
    id: "activity",
    translationKey: "navGroups.activity",
    items: [{ id: "upcoming-actions", translationKey: "nav.upcomingActions", icon: Banknote }, { id: "history", translationKey: "nav.history", icon: History }, { id: "reconciliation", translationKey: "nav.reconciliation", icon: GitCompareArrows }],
  },
  {
    id: "insights",
    translationKey: "navGroups.insights",
    items: [
      { id: "historical-overview", translationKey: "nav.historicalOverview", icon: History },
      { id: "return-analysis", translationKey: "nav.returnAnalysis", icon: ChartLine },
      { id: "asset-changes", translationKey: "nav.assetChanges", icon: GitCompareArrows },
    ],
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

export const DEFAULT_PAGE_ID = "overview";

export function targetForPage(page: PageId): NavigationTarget {
  if (page === "return-analysis") return { page };
  if (page === "asset-changes") return { page };
  if (page === "history") return { page };
  if (page === "available-funds") return { page };
  if (page === "portfolio") return { page, view: "holdings" };
  if (page === "instrument-detail") throw new Error("An instrument detail target needs an instrumentId");
  return { page };
}

/** A plain old Market Data object link now opens the shared read-only record. Repair links stay in maintenance. */
export function resolveLegacyTarget(target: NavigationTarget): NavigationTarget {
  if (target.page !== "market-data") return target;
  const focus = target.focus;
  if (!focus?.instrumentId || focus.id || focus.kind || focus.action || focus.reason || focus.executable !== undefined || focus.snapshotComponentLabel || focus.currencyA || focus.currencyB) return target;
  return { page: "instrument-detail", instrumentId: focus.instrumentId, accountId: focus.accountId,
    section: "data", from: focus.rangeStart, to: focus.rangeEnd, healthFocus: focus };
}
