import { useId } from "react";
import { useQuery } from "@tanstack/react-query";
import { Service } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics";
import { callService } from "@/lib/wails";

export function useHistoricalOverview(date: string, compareTo: string, enabled: boolean) {
  const session = useId();
  return useQuery({
    queryKey: ["historical-overview", session, date, compareTo],
    queryFn: () => callService(() => Service.HistoricalOverview(date, compareTo)),
    enabled,
    // A captured comparison stays fixed even when workspace observers invalidate
    // other reads. The user explicitly refreshes both sides together.
    staleTime: "static",
    gcTime: 0,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    retry: false,
  });
}
