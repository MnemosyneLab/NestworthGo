import { resolveLegacyTarget, type NavigationTarget, type ResolvedNavigationTarget } from "./navigation";

export type RevisitableDestination =
  | Exclude<ResolvedNavigationTarget, { page: "accounts" } | { page: "history" } | { page: "available-funds" }>
  | Omit<Extract<ResolvedNavigationTarget, { page: "accounts" }>, "create" | "updateValue">
  | Omit<Extract<ResolvedNavigationTarget, { page: "history" }>, "activityId">
  | { page: "available-funds"; productId?: string; focus?: { kind: "product"; productId: string } };

export const NAVIGATION_ENTRY_LIMIT = 40;
export const RECENT_DESTINATION_LIMIT = 20;
export const TASK_SESSION_LIMIT = 20;

/** Destinations contain no account creation/value-update intent. Activity sheets are transient too. */
export function revisitTarget(target: NavigationTarget): RevisitableDestination {
  const resolved = structuredClone(resolveLegacyTarget(target));
  if (resolved.page === "accounts") return { page: resolved.page, accountId: resolved.accountId, filter: resolved.filter };
  if (resolved.page === "history") return { page: resolved.page, filters: resolved.filters };
  if (resolved.page === "available-funds") return { page: resolved.page, productId: resolved.productId, focus: resolved.focus?.kind === "product" ? resolved.focus : undefined };
  return resolved;
}

/** Stable semantic identity, independent of object property order and omitted optional fields. */
export function destinationKey(target: NavigationTarget): string {
  return JSON.stringify(revisitTarget(target), (_key, value) => value && typeof value === "object" && !Array.isArray(value)
    ? Object.fromEntries(Object.entries(value).sort(([a], [b]) => a.localeCompare(b))) : value);
}

export function appendVisit<T>(entries: readonly T[], cursor: number, current: T, destination: T): T[] {
  return [...entries.slice(0, cursor), current, destination].slice(-NAVIGATION_ENTRY_LIMIT);
}

export function recordRecent<T extends { target: NavigationTarget }>(recent: readonly T[], current: T): T[] {
  const key = destinationKey(current.target);
  return [current, ...recent.filter(entry => destinationKey(entry.target) !== key)].slice(0, RECENT_DESTINATION_LIMIT);
}
