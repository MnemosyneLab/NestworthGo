import type { NavigationTarget } from "@/app/navigation";
import type { NavigationResult } from "./contracts";

export type ModelPageState = {
  stamp: string;
  surface: "desktop" | "mobile";
  blocked: () => boolean;
  dispatch: (target: NavigationTarget) => boolean | void;
};
export type ModelTarget = { kind: string; accountId: string; tab?: string };
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
