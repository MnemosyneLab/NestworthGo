import { useQuery } from "@tanstack/react-query";
import { Service as AnalysisService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analysis";
import type {
  AnalysisQueryRequest,
  AssetChangeDTO,
  AssetDriverDetailDTO,
  ContributionDTO,
  ReturnCalendarDTO,
  ReturnDayDTO,
  ReturnTrendDTO,
} from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analysis/models";
import { callService } from "@/lib/wails";
import { queryKeys } from "@/queries/keys";

export function useReturnCalendar(request: AnalysisQueryRequest, cursor: string, enabled = true) {
  return useQuery<ReturnCalendarDTO>({
    queryKey: queryKeys.analysis.returnCalendar(request, cursor),
    queryFn: () => fetchReturnCalendar(request, cursor),
    enabled,
  });
}

export function useAssetChange(request: AnalysisQueryRequest, enabled = true) {
  return useQuery<AssetChangeDTO>({
    queryKey: queryKeys.analysis.assetChange(request),
    queryFn: () => callService(() => AnalysisService.AssetChange(request)),
    enabled,
  });
}

export function useAssetDriverDetail(request: AnalysisQueryRequest, driverKey: string, enabled = true) {
  return useQuery<AssetDriverDetailDTO>({
    queryKey: queryKeys.analysis.assetDriverDetail(request, driverKey),
    queryFn: () => callService(() => AnalysisService.AssetDriverDetail(request, driverKey)),
    enabled: enabled && Boolean(driverKey),
  });
}

export function fetchReturnCalendar(request: AnalysisQueryRequest, cursor: string): Promise<ReturnCalendarDTO> {
  return callService(() => AnalysisService.ReturnCalendar(request, cursor, "day"));
}

export function useReturnDay(request: AnalysisQueryRequest, date: string, enabled = true) {
  return useQuery<ReturnDayDTO>({
    queryKey: queryKeys.analysis.returnDay(request, date),
    queryFn: () => callService(() => AnalysisService.ReturnDay(request, date)),
    enabled: enabled && Boolean(date),
  });
}

export function useReturnTrend(request: AnalysisQueryRequest, display: string, enabled = true) {
  return useQuery<ReturnTrendDTO>({
    queryKey: queryKeys.analysis.returnTrend(request, display),
    queryFn: () => callService(() => AnalysisService.ReturnTrend(request, display)),
    enabled,
  });
}

export function useContribution(request: AnalysisQueryRequest, returnType: string, groupBy: string, ordering: string, enabled = true) {
  return useQuery<ContributionDTO>({
    queryKey: queryKeys.analysis.contribution(request, returnType, groupBy, ordering),
    queryFn: () => callService(() => AnalysisService.Contribution(request, returnType, groupBy, ordering)),
    enabled,
  });
}
