import { useQueries, useQuery } from "@tanstack/react-query";
import { Service as AnalyticsService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics";
import { Service as PortfolioService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/portfolio";
import type { HoldingGainDTO } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { callService } from "@/lib/wails";

export function useRealizedGain(trendRange: string) {
  return useQuery({
    queryKey: ["analytics", "realizedGain", trendRange],
    queryFn: () => callService(() => AnalyticsService.RealizedGain({}, trendRange)),
  });
}

export function useAccountGain(accountId: string) {
  return useQuery({
    queryKey: ["analytics", "accountGain", accountId],
    queryFn: () => callService(() => AnalyticsService.AccountGain(accountId)),
    enabled: Boolean(accountId),
  });
}

export function useNetWorthTrend(trendRange: string) {
  return useQuery({
    queryKey: ["analytics", "netWorthTrend", trendRange],
    queryFn: () => callService(() => PortfolioService.NetWorthTrend(trendRange)),
  });
}

/**
 * useHoldingGainsByAccounts fetches AnalyticsService.AccountGain for every
 * given Account and flattens the result into a per-Holding lookup, so a
 * flat Holdings list (Investments page) can show cost/current
 * value/gain columns per row without re-deriving them client-side. Go remains
 * the sole calculation authority. Reuses the same query key as useAccountGain, so the two
 * hooks share one cache entry per Account.
 */
export function useHoldingGainsByAccounts(accountIds: string[]) {
  const results = useQueries({
    queries: accountIds.map((accountId) => ({
      queryKey: ["analytics", "accountGain", accountId],
      queryFn: () => callService(() => AnalyticsService.AccountGain(accountId)),
    })),
  });
  const byHoldingId = new Map<string, HoldingGainDTO>();
  for (const result of results) {
    for (const holding of result.data?.holdings ?? []) {
      byHoldingId.set(holding.holdingId, holding);
    }
  }
  return { byHoldingId, isLoading: results.some((result) => result.isLoading) };
}
