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

/**
 * usePreviewFixChange backs the Fix form's "Preview" step. It must call
 * PreviewFixChange, not PreviewChange: a plain PreviewChange ignores the
 * original Activity being replaced and previews the replacement command
 * against the state *after* that original effect already applied,
 * double-counting it, since Confirm (FixChange) correctly inverts the
 * original effect first (see internal/application/history_changes.go's
 * fixChangePreview doc comment for the full explanation, and
 * HistoryPage.test.tsx's "fixes a change" test for the regression).
 */
export function usePreviewFixChange() {
  return useMutation({
    mutationFn: ({ activityId, replacement }: { activityId: string; replacement: ChangeCommandRequest }) =>
      callService(() => HistoryService.PreviewFixChange(activityId, replacement)),
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
