import type { AccountCombinationDTO } from "@/queries/catalog";
import { percentToShareBps } from "@/lib/money";

/** Preferred primary create-wizard types. Catalog remains the closed set:
 * types missing from the catalog are omitted, and unknown catalog types
 * appear under More. */
export const PRIMARY_ACCOUNT_TYPE_ORDER = [
  "bank_account",
  "brokerage",
  "investment_account",
  "crypto_exchange",
  "digital_wallet",
  "property",
  "vehicle",
  "credit_card",
  "loan",
  "other",
] as const;

export const MORE_ACCOUNT_TYPE_ORDER = [
  "cash_on_hand",
  "pension",
  "insurance_policy",
  "collectible",
  "receivable",
] as const;

const TOTAL_OWNERSHIP_BPS = 10000;

export type TrackingPromptKind = "skip" | "ask";

export function splitAccountTypes(catalogTypes: readonly string[]): { primary: string[]; more: string[] } {
  const catalog = new Set(catalogTypes);
  const primary = PRIMARY_ACCOUNT_TYPE_ORDER.filter((type) => catalog.has(type));
  const listed = new Set<string>([...PRIMARY_ACCOUNT_TYPE_ORDER, ...MORE_ACCOUNT_TYPE_ORDER]);
  const more = [
    ...MORE_ACCOUNT_TYPE_ORDER.filter((type) => catalog.has(type)),
    ...catalogTypes.filter((type) => !listed.has(type)),
  ];
  return { primary, more };
}

export function combinationsFor(
  combinations: readonly AccountCombinationDTO[],
  accountType: string,
  role?: string,
): AccountCombinationDTO[] {
  return combinations.filter(
    (item) => item.accountType === accountType && (role === undefined || item.balanceSheetRole === role),
  );
}

export function rolesFor(combinations: readonly AccountCombinationDTO[], accountType: string): string[] {
  return Array.from(new Set(combinationsFor(combinations, accountType).map((item) => item.balanceSheetRole)));
}

export function defaultRole(combinations: readonly AccountCombinationDTO[], accountType: string): string {
  const match = combinationsFor(combinations, accountType)[0];
  return match?.balanceSheetRole ?? "asset";
}

export function roleLocked(combinations: readonly AccountCombinationDTO[], accountType: string): boolean {
  const matches = combinationsFor(combinations, accountType);
  return matches.length > 0 && matches.every((item) => item.roleLocked);
}

export function trackingModesFor(
  combinations: readonly AccountCombinationDTO[],
  accountType: string,
  role: string,
): string[] {
  return Array.from(new Set(combinationsFor(combinations, accountType, role).map((item) => item.trackingMode)));
}

export function trackingPrompt(
  combinations: readonly AccountCombinationDTO[],
  accountType: string,
  role: string,
): { kind: TrackingPromptKind; options: string[]; defaultMode: string } {
  const options = trackingModesFor(combinations, accountType, role);
  if (options.length <= 1) {
    return { kind: "skip", options, defaultMode: options[0] ?? "balance" };
  }
  if (accountType === "cash_on_hand" || accountType === "bank_account" || accountType === "digital_wallet") {
    return { kind: "ask", options, defaultMode: options.includes("balance") ? "balance" : options[0] };
  }
  if (accountType === "other") {
    return { kind: "ask", options, defaultMode: "" };
  }
  return { kind: "ask", options, defaultMode: options.includes("holdings") ? "holdings" : options[0] };
}

export function matchingCombination(
  combinations: readonly AccountCombinationDTO[],
  accountType: string,
  role: string,
  trackingMode: string,
): AccountCombinationDTO | undefined {
  return combinations.find(
    (item) => item.accountType === accountType && item.balanceSheetRole === role && item.trackingMode === trackingMode,
  );
}

export function compatibleAccountTypes(
  combinations: readonly AccountCombinationDTO[],
  catalogTypes: readonly string[],
  role: string,
  trackingMode: string,
  currentType?: string,
): string[] {
  const compatible = new Set(
    combinations.filter((item) => item.balanceSheetRole === role && item.trackingMode === trackingMode).map((item) => item.accountType),
  );
  if (currentType) {
    compatible.add(currentType);
  }
  return catalogTypes.filter(
    (type) =>
      compatible.has(type) &&
      (trackingMode !== "holdings" || !currentType || (type === "cash_on_hand") === (currentType === "cash_on_hand")),
  );
}

/** ownershipShares maps checked owners to basis-point shares. An empty owner
 * list stays empty (it is not the whole household). One or more checked
 * owners with no custom percentages even-split among those checked owners. */
export function ownershipShares(
  ownerIds: string[],
  percentages: string[] | undefined,
  useCustom: boolean,
): { memberId: string; shareBps: number }[] {
  if (ownerIds.length === 0) {
    return [];
  }
  if (useCustom && percentages && percentages.length === ownerIds.length) {
    return ownerIds.map((memberId, index) => ({
      memberId,
      shareBps: percentToShareBps(percentages[index]),
    }));
  }
  const base = Math.floor(TOTAL_OWNERSHIP_BPS / ownerIds.length);
  const remainder = TOTAL_OWNERSHIP_BPS % ownerIds.length;
  return ownerIds.map((memberId, index) => ({
    memberId,
    shareBps: index < remainder ? base + 1 : base,
  }));
}

export function trackingMethodKey(mode: string, accountType?: string): string {
  if (mode === "holdings") {
    if (accountType === "cash_on_hand") {
      return "accounts.tracking.multiCurrencyCash";
    }
    return "accounts.tracking.holdings";
  }
  if (mode === "manual_value") {
    return "accounts.tracking.manualValue";
  }
  return "accounts.tracking.balance";
}

export function trackingHelpKey(mode: string, accountType?: string): string {
  if (mode === "holdings") {
    return accountType === "cash_on_hand" ? "accounts.tracking.multiCurrencyCashHelp" : "accounts.tracking.holdingsHelp";
  }
  return mode === "manual_value" ? "accounts.tracking.manualValueHelp" : "accounts.tracking.balanceHelp";
}
