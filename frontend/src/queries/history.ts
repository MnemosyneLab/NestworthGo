import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Service as HistoryService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history";
import type { ChangeCommandRequest } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history/models";
import { callService } from "@/lib/wails";
import { overviewQueryKey } from "@/queries/portfolio";

export function useHistoryOrigin() {
  return useQuery({
    queryKey: ["history", "origin"],
    queryFn: () => callService(() => HistoryService.HistoryOrigin()),
  });
}

export function useStartHistory() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (timezone: string) => callService(() => HistoryService.StartHistory(timezone)),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["history"] }),
  });
}

export function useListActivities(limit = 50) {
  return useQuery({
    queryKey: ["history", "activities", limit],
    queryFn: () => callService(() => HistoryService.ListActivities(limit)),
  });
}

export function usePreviewChange() {
  return useMutation({
    mutationFn: (request: ChangeCommandRequest) => callService(() => HistoryService.PreviewChange(request)),
  });
}

function invalidateHistoryData(queryClient: ReturnType<typeof useQueryClient>) {
  void queryClient.invalidateQueries({ queryKey: ["history"] });
  void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
  void queryClient.invalidateQueries({ queryKey: ["accounts"] });
  void queryClient.invalidateQueries({ queryKey: ["holdings"] });
}

export function useRecordChange() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (request: ChangeCommandRequest) => callService(() => HistoryService.RecordChange(request)),
    onSuccess: () => invalidateHistoryData(queryClient),
  });
}

export function useUndoChange() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (activityId: string) => callService(() => HistoryService.UndoChange(activityId)),
    onSuccess: () => invalidateHistoryData(queryClient),
  });
}

export function useFixChange() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ activityId, replacement }: { activityId: string; replacement: ChangeCommandRequest }) =>
      callService(() => HistoryService.FixChange(activityId, replacement)),
    onSuccess: () => invalidateHistoryData(queryClient),
  });
}
