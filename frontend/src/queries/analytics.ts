import { useQuery } from "@tanstack/react-query";
import { Service as AnalyticsService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics";
import { Service as PortfolioService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/portfolio";
import { callService } from "@/lib/wails";
import { queryKeys } from "@/queries/keys";

export type AnalyticsTrendRange = "30d" | "ytd" | "1y" | "all";
export type AnalyticsRange =
  | { kind: "trend"; value: AnalyticsTrendRange }
  | { kind: "custom"; from: string; to: string };

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

export function useInstrumentHoldings() {
  return useQuery({
    queryKey: queryKeys.analytics.instrumentHoldings,
    queryFn: () => callService(() => AnalyticsService.InstrumentHoldings()),
  });
}
