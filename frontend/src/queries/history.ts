import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef } from "react";
import { Service as HistoryService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history";
import type { ActivityQueryRequest, ChangeCommandRequest } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history/models";
import { callService } from "@/lib/wails";
import { queryKeys } from "@/queries/keys";
import { invalidateActivityChange, invalidateHistoryReads } from "@/queries/invalidation";

export function useHistoryOrigin() {
  return useQuery({
    queryKey: queryKeys.history.origin,
    queryFn: () => callService(() => HistoryService.HistoryOrigin()),
  });
}

export function useHistoryMutationAllowed() {
  return useQuery({
    queryKey: queryKeys.history.mutationAllowed,
    queryFn: () => callService(() => HistoryService.HistoryMutationAllowed()),
  });
}

export function useDailySnapshotState() {
  return useQuery({
    queryKey: queryKeys.history.snapshotState,
    queryFn: () => callService(() => HistoryService.DailySnapshotState("")),
  });
}

export function useStartingPointDraft(enabled = true) {
  return useQuery({
    queryKey: queryKeys.history.startingPointDraft,
    queryFn: () => callService(() => HistoryService.StartingPointDraft()),
    enabled,
  });
}

export function useStartHistory() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ timezone, costOverrides }: { timezone: string; costOverrides?: Record<string, string> }) =>
      costOverrides
        ? callService(() => HistoryService.StartHistoryWithCosts(timezone, costOverrides))
        : callService(() => HistoryService.StartHistory(timezone)),
    onSuccess: () => invalidateHistoryReads(queryClient),
  });
}

export function useListActivities(limit = 50) {
  return useQuery({
    queryKey: queryKeys.history.activities(limit),
    queryFn: () => callService(() => HistoryService.ListActivities(limit)),
  });
}

export function useActivity(activityId: string) {
  return useQuery({
    queryKey: queryKeys.history.activity(activityId),
    queryFn: () => callService(() => HistoryService.Activity(activityId)),
    enabled: Boolean(activityId),
  });
}

export function useActivityPage(request: ActivityQueryRequest = {}) {
  const normalized: ActivityQueryRequest = {
    ...(request.accountId ? { accountId: request.accountId } : {}),
    ...(request.instrumentId ? { instrumentId: request.instrumentId } : {}),
    ...(request.kinds && request.kinds.length > 0 ? { kinds: [...request.kinds].sort() } : {}),
    ...(request.fromLocalDate ? { fromLocalDate: request.fromLocalDate } : {}),
    ...(request.toLocalDate ? { toLocalDate: request.toLocalDate } : {}),
    limit: request.limit ?? 50,
  };
  return useInfiniteQuery({
    queryKey: queryKeys.history.activityPage(normalized),
    queryFn: ({ pageParam }) =>
      callService(() =>
        HistoryService.ListActivityPage({
          ...normalized,
          ...(pageParam ?? {}),
        }),
      ),
    initialPageParam: undefined as Pick<ActivityQueryRequest, "afterId" | "afterEffectiveAt" | "afterCreatedAt"> | undefined,
    getNextPageParam: (lastPage) => {
      if (!lastPage.hasMore || !lastPage.next) {
        return undefined;
      }
      return {
        afterId: lastPage.next.id,
        afterEffectiveAt: lastPage.next.effectiveAt,
        afterCreatedAt: lastPage.next.createdAt,
      };
    },
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

function affectedAccountIds(request: ChangeCommandRequest): string[] {
  return [...new Set([
    request.accountId,
    request.fromAccountId,
    request.toAccountId,
    request.settlementAccountId,
    request.debtAccountId,
    request.cashAccountId,
  ].filter((id): id is string => Boolean(id)))];
}

type clientMutationSlot = { payload: string; id: string };

function commandPayloadKey(request: ChangeCommandRequest): string {
  return JSON.stringify({ ...request, mutationId: "" });
}

function withClientMutationID(request: ChangeCommandRequest, slot: { current: clientMutationSlot | null }): ChangeCommandRequest {
  const payload = commandPayloadKey(request);
  if (slot.current == null || slot.current.payload !== payload) {
    const existing = request.mutationId?.trim();
    slot.current = { payload, id: existing || crypto.randomUUID() };
  }
  return { ...request, mutationId: slot.current.id };
}

export function useRecordChange() {
  const queryClient = useQueryClient();
  const mutationSlot = useRef<clientMutationSlot | null>(null);
  return useMutation({
    mutationFn: (request: ChangeCommandRequest) =>
      callService(() => HistoryService.RecordChange(withClientMutationID(request, mutationSlot))),
    onSuccess: (_data, variables) => {
      mutationSlot.current = null;
      invalidateActivityChange(queryClient, affectedAccountIds(variables));
    },
  });
}

export function useUndoChange() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (activityId: string) => callService(() => HistoryService.UndoChange(activityId)),
    onSuccess: () => invalidateActivityChange(queryClient),
  });
}

export function useFixChange() {
  const queryClient = useQueryClient();
  const mutationSlot = useRef<clientMutationSlot | null>(null);
  return useMutation({
    mutationFn: ({ activityId, replacement }: { activityId: string; replacement: ChangeCommandRequest }) =>
      callService(() => HistoryService.FixChange(activityId, withClientMutationID(replacement, mutationSlot))),
    onSuccess: (_data, variables) => {
      mutationSlot.current = null;
      invalidateActivityChange(queryClient, affectedAccountIds(variables.replacement));
    },
  });
}

export function useRebuildHistoricalSnapshots() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ startDate, endDate }: { startDate: string; endDate: string }) =>
      callService(() => HistoryService.RebuildHistoricalSnapshots(startDate, endDate)),
    onSuccess: () => invalidateHistoryReads(queryClient),
  });
}
