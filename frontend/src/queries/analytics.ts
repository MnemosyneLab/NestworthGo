import { useQuery } from "@tanstack/react-query";
import { Service as AnalyticsService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics";
import { Service as PortfolioService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/portfolio";
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
