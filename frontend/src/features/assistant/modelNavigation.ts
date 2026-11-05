import type { AnalysisNavigationContext, NavigationTarget } from "@/app/navigation";
import type { NavigationResult } from "./contracts";
import type { TrendObservation } from "./trendVerification";

export type ModelPageState = {
  stamp: string;
  surface: "desktop" | "mobile";
  blocked: () => boolean;
  dispatch: (target: NavigationTarget) => boolean | void;
  verifyTrend?: (target: NavigationTarget, expected: TrendObservation) => Promise<NavigationResult>;
};
export type TrendAction = { from: string; to: string; valuation: string; includeCash: boolean; display: string; currency?: string; assetClass?: string };
export type ModelTarget = { kind: string; accountId: string; tab?: string; trend?: TrendAction | null };
function civilDate(value: string) {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value) || value < "0001-01-01") return false;
  const [year, month, day] = value.split("-").map(Number);
  const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  return month >= 1 && month <= 12 && day >= 1 && day <= [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][month - 1];
}
const canonicalID =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
export function modelNavigationAllowed(stamp: string, current: ModelPageState) {
  return (
    !!stamp &&
    current.stamp === stamp &&
    current.surface === "desktop" &&
    !current.blocked()
  );
}
// The backend consumes a one-shot intent; the frontend independently fences
// visible page generation and unresolved financial review before AND after IPC.
export async function applyModelNavigation(
  stamp: string,
  current: () => ModelPageState,
  resolve: () => Promise<ModelTarget>,
): Promise<NavigationResult> {
  let state = current();
  if (state.stamp !== stamp || state.surface !== "desktop")
    return "stale_context";
  if (state.blocked()) return "blocked";
  const target = await resolve();
  state = current();
  if (state.stamp !== stamp || state.surface !== "desktop")
    return "stale_context";
  if (state.blocked()) return "blocked";
  if (target.kind === "return-trend") {
    const a = target.trend;
    if (!a || target.tab || (target.accountId !== "" && !canonicalID.test(target.accountId)) ||
        !civilDate(a.from) || !civilDate(a.to) || a.from > a.to ||
        (a.valuation !== "base" && a.valuation !== "native") || typeof a.includeCash !== "boolean" ||
        (a.display !== "cumulative_amount" && a.display !== "linked_rate" && a.display !== "period_return_amount") ||
        (a.currency !== undefined && !/^[A-Z]{3}$/.test(a.currency)) ||
        (a.assetClass !== undefined && !/^[a-z_]{1,40}$/.test(a.assetClass))) return "invalid_target";
    if (!state.verifyTrend) return "unverified";
    const analysis: AnalysisNavigationContext = { scope: target.accountId ? "account" : "portfolio", scopeId: target.accountId,
      from: a.from, to: a.to, valuation: a.valuation, includeCash: a.includeCash,
      trendDisplay: a.display, moreFilters: { currency: a.currency, assetClass: a.assetClass } };
    return state.verifyTrend({ page: "return-analysis", tab: "trend", cursor: a.to.slice(0, 7), analysis }, {
      accountId: target.accountId, ...a, currency: a.currency ?? "", assetClass: a.assetClass ?? "", status: "ready",
    });
  }
  if (target.trend) return "invalid_target";
  if (target.kind === "return-analysis" && target.accountId === "" &&
      (target.tab === "calendar" || target.tab === "trend" || target.tab === "contribution")) {
    return state.dispatch({page: "return-analysis", tab: target.tab}) === false ? "blocked" : "opened";
  }
  if (target.tab) return "invalid_target";
  if (target.kind === "overview" && target.accountId === "") {
    return state.dispatch({ page: "overview" }) === false
      ? "blocked"
      : "opened";
  }
  if (target.kind === "accounts" && target.accountId === "") {
    return state.dispatch({ page: "accounts" }) === false
      ? "blocked"
      : "opened";
  }
  if (target.kind === "account" && canonicalID.test(target.accountId)) {
    return state.dispatch({ page: "accounts", accountId: target.accountId }) ===
      false
      ? "blocked"
      : "opened";
  }
  return "invalid_target";
}
