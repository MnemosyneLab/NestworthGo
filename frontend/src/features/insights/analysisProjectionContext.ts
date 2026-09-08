import type { AnalysisQueryRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analysis/models";
import type { AnalysisSessionState } from "@/stores/analysis";
import { analysisRequest, effectiveRange } from "@/features/insights/analysisRequest";
import { currentMonth } from "@/features/insights/calendar";
import { useHistoryOrigin } from "@/queries/history";

export function useAnalysisProjectionContext(session: AnalysisSessionState): {
  request: AnalysisQueryRequest;
  origin: ReturnType<typeof useHistoryOrigin>;
  scopeReady: boolean;
  rangeAvailable: boolean;
  enabled: boolean;
} {
  const origin = useHistoryOrigin();
  const visibleMonth = currentMonth(origin.data?.timezone);
  const range = effectiveRange(session, visibleMonth, origin.data?.timezone, origin.data?.startedAt);
  const request = analysisRequest(session, range.from, range.to);
  const scopeReady = session.scope === "portfolio" || Boolean(session.scopeId);
  const rangeAvailable = range.from <= range.to;
  const enabled = !origin.isLoading && !origin.isError && Boolean(origin.data?.timezone) && scopeReady && rangeAvailable;
  return { request, origin, scopeReady, rangeAvailable, enabled };
}
