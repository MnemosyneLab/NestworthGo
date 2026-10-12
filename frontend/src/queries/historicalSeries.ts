import { useId } from "react";
import { useQuery } from "@tanstack/react-query";
import { Service } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics";
import { callService } from "@/lib/wails";

export function useHistoricalSeries(from: string, to: string, enabled: boolean, request: number) {
  const session = useId();
  return useQuery({ queryKey: ["historical-series", session, from, to, request],
    queryFn: async ({ signal }) => {
      const pending = Service.HistoricalOverviewSeries(from, to);
      const abort = () => { void Promise.resolve(pending.cancel?.()).catch(() => {}); };
      signal.addEventListener("abort", abort, { once: true });
      if (signal.aborted) abort();
      try { return await callService(() => pending); }
      finally { signal.removeEventListener("abort", abort); }
    }, enabled,
    staleTime: "static", gcTime: 0, refetchOnWindowFocus: false, refetchOnReconnect: false, retry: false });
}
