import { Service } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics";
import { useHistoricalCapture } from "./historicalCapture";

export function useHistoricalOverview(date: string, compareTo: string, enabled: boolean, request = 0) {
  return useHistoricalCapture("historical-overview", date, compareTo, enabled, request,
    () => Service.HistoricalOverview(date, compareTo));
}
