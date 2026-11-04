import { Service } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics";
import { useHistoricalCapture } from "./historicalCapture";

export function useHistoricalSeries(from: string, to: string, enabled: boolean, request: number, session?: string) {
  return useHistoricalCapture("historical-series", from, to, enabled, request,
    () => Service.HistoricalOverviewSeries(from, to), session);
}
