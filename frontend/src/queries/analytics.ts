import { useQuery } from "@tanstack/react-query";
import { Service as AnalyticsService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics";
import { Service as PortfolioService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/portfolio";
import type { GainScopeRequest } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics/models";
import type { HoldingGainDTO } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { callService } from "@/lib/wails";
import { queryKeys } from "@/queries/keys";

export type AnalyticsTrendRange = "30d" | "ytd" | "1y" | "all";
export type AnalyticsRange =
  | { kind: "trend"; value: AnalyticsTrendRange }
  | { kind: "custom"; from: string; to: string };

export const portfolioScope: GainScopeRequest = {};

function validCustomRange(range: AnalyticsRange): boolean {
  return range.kind !== "custom" || (Boolean(range.from) && Boolean(range.to) && range.from <= range.to);
}

export function useRealizedGain(range: AnalyticsRange, scope: GainScopeRequest = portfolioScope) {
  return useQuery({
    queryKey: queryKeys.analytics.realizedGain(scope, range),
    queryFn: () => range.kind === "custom"
      ? callService(() => AnalyticsService.RealizedGainInRange(scope, range.from, range.to))
      : callService(() => AnalyticsService.RealizedGain(scope, range.value)),
    enabled: validCustomRange(range),
  });
}

export function useDividendIncome(range: AnalyticsRange, scope: GainScopeRequest = portfolioScope) {
  return useQuery({
    queryKey: queryKeys.analytics.dividendIncome(scope, range),
    queryFn: () => range.kind === "custom"
      ? callService(() => AnalyticsService.DividendIncomeInRange(scope, range.from, range.to))
      : callService(() => AnalyticsService.DividendIncome(scope, range.value)),
    enabled: validCustomRange(range),
  });
}

export function useNetWorthTrend(range: AnalyticsRange) {
  return useQuery({
    queryKey: queryKeys.analytics.netWorthTrend(range),
    queryFn: () => {
      if (range.kind !== "trend") {
        throw new Error("Net worth trend does not support custom date ranges");
      }
      return callService(() => PortfolioService.NetWorthTrend(range.value));
    },
    enabled: range.kind === "trend",
  });
}

export function useAccountGain(accountId: string) {
  return useQuery({
    queryKey: ["analytics", "accountGain", accountId] as const,
    queryFn: () => callService(() => AnalyticsService.AccountGain(accountId)),
    enabled: Boolean(accountId),
  });
}

export function useHoldingGain(holdingId: string) {
  return useQuery({
    queryKey: ["analytics", "holdingGain", holdingId] as const,
    queryFn: () => callService(() => AnalyticsService.HoldingGain(holdingId)),
    enabled: Boolean(holdingId),
  });
}

/**
 * useHoldingGainsByAccounts fetches household AccountGains in one query and
 * flattens holdings into a lookup. Go remains the sole calculation
 * authority. accountIds is kept so callers can wait until the Account list
 * is known; the backend still reads one snapshot for the whole household.
 */
export function useHoldingGainsByAccounts(accountIds: string[]) {
  const enabled = accountIds.length > 0;
  const result = useQuery({
    queryKey: queryKeys.analytics.accountGains.all,
    queryFn: () => callService(() => AnalyticsService.AccountGains()),
    enabled,
  });
  const byHoldingId = new Map<string, HoldingGainDTO>();
  const wanted = new Set(accountIds);
  for (const account of result.data ?? []) {
    if (wanted.size > 0 && !wanted.has(account.accountId)) {
      continue;
    }
    for (const holding of account.holdings ?? []) {
      byHoldingId.set(holding.holdingId, holding);
    }
  }
  return {
    byHoldingId,
    isLoading: enabled && result.isLoading,
    isError: result.isError,
    refetch: result.refetch,
  };
}
