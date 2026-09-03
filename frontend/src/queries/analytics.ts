import { useQuery } from "@tanstack/react-query";
import { Service as AnalyticsService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics";
import { Service as PortfolioService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/portfolio";
import type { HoldingGainDTO } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { callService } from "@/lib/wails";
import { queryKeys } from "@/queries/keys";

export function useRealizedGain(trendRange: string) {
  return useQuery({
    queryKey: queryKeys.analytics.realizedGain(trendRange),
    queryFn: () => callService(() => AnalyticsService.RealizedGain({}, trendRange)),
  });
}

export function useDividendIncome(trendRange: string) {
  return useQuery({
    queryKey: queryKeys.analytics.dividendIncome(trendRange),
    queryFn: () => callService(() => AnalyticsService.DividendIncome({}, trendRange)),
  });
}

export function useNetWorthTrend(trendRange: string) {
  return useQuery({
    queryKey: queryKeys.analytics.netWorthTrend(trendRange),
    queryFn: () => callService(() => PortfolioService.NetWorthTrend(trendRange)),
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
