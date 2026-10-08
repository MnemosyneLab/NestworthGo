import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Service } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/continuousbackup";
import type { RecoveryPoint, Update } from "../../bindings/github.com/waltwang/nestworth-go/internal/infrastructure/continuousbackup/models";
import type { RestoreConfirmRequest } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/recovery/models";
import { callService } from "@/lib/wails";
const key = ["continuousBackup"] as const;
export function useContinuousBackup(active = true) {
  return useQuery({ queryKey: key, queryFn: () => callService(() => Service.Status()), retry: false, enabled: active, refetchInterval: active ? 5000 : false });
}
export function useConfigureBackup() {
  const client = useQueryClient();
  return useMutation({ mutationFn: (input: Update) => callService(() => Service.Configure(input)), onSuccess: (value) => { client.setQueryData(key, value); void client.resetQueries({ queryKey: [...key, "recoveryPoints"] }); } });
}
export function useTestBackupConnection() {
  return useMutation({ mutationFn: () => callService(() => Service.TestConnection()) });
}
export function useBackupNow() {
  const client = useQueryClient();
  return useMutation({ mutationFn: () => callService(() => Service.BackupNow()), onSettled: () => void client.invalidateQueries({ queryKey: key }) });
}
export function useCloudRecoveryPoints(scope: string) {
  const client = useQueryClient();
  const queryKey = [...key, "recoveryPoints", scope];
  const query = useInfiniteQuery({
    queryKey, initialPageParam: "", enabled: false, retry: false,
    queryFn: async ({ pageParam }) => {
      const page = await callService(() => Service.RecoveryPointPage(pageParam));
      const loaded = client.getQueryData<{ pageParams: string[] }>(queryKey);
      if (page.nextCursor && (page.nextCursor === pageParam || loaded?.pageParams.includes(page.nextCursor))) {
        throw new Error("Recovery page cursor did not advance");
      }
      return page;
    },
    getNextPageParam: (last) => last.nextCursor || undefined,
  });
  const unique = new Map<string, RecoveryPoint>();
  for (const page of query.data?.pages ?? []) for (const point of page.points ?? []) unique.set(`${point.streamID}:${point.txID}`, point);
  const timestampKey = (value: string) => value.replace(/(?:\.(\d+))?Z$/, (_, fraction = "") => `.${fraction.padEnd(9, "0")}Z`);
  const points = [...unique.values()].sort((a, b) => timestampKey(b.capturedAt).localeCompare(timestampKey(a.capturedAt)) || b.streamID.localeCompare(a.streamID) || b.txID.localeCompare(a.txID));
  return { ...query, points, restart: async () => {
    // A refresh starts one new traversal, rather than refetching every loaded page.
    await client.resetQueries({ queryKey, exact: true });
    return query.refetch();
  } };
}
export function useInspectCloudRestore() {
  return useMutation({ mutationFn: (point: RecoveryPoint) => callService(() => Service.InspectRestore(point)) });
}
export function useConfirmCloudRestore() {
  return useMutation({ mutationFn: (input: RestoreConfirmRequest) => callService(() => Service.ConfirmRestore(input)) });
}

export function useRetentionStatus(scope: string, active = true) {
  return useQuery({ queryKey: [...key, "retention", scope], queryFn: () => callService(() => Service.RetentionStatus()), retry: false, enabled: active, refetchInterval: active ? 5000 : false });
}
export function useConfigureRetention() {
  const client = useQueryClient();
  return useMutation({ mutationFn: (input: { enabled: boolean; days: number; acknowledged: boolean }) => callService(() => Service.ConfigureRetention(input)), onSettled: () => void client.invalidateQueries({ queryKey: key }) });
}
export function usePreviewRetention() {
  const client = useQueryClient();
  return useMutation({ mutationFn: (days: number) => callService(() => Service.PreviewRetention(days)), onSettled: () => void client.invalidateQueries({ queryKey: [...key, "retention"] }) });
}
export function useExecuteRetention() {
  const client = useQueryClient();
  return useMutation({ mutationFn: (token: string) => callService(() => Service.ExecuteRetention({ token, acknowledged: true })), onSettled: () => { void client.resetQueries({ queryKey: [...key, "recoveryPoints"] }); void client.invalidateQueries({ queryKey: key }); } });
}
export function useCancelCleanup() {
  const client = useQueryClient();
  return useMutation({ mutationFn: () => callService(() => Service.CancelCleanup()), onSettled: () => void client.invalidateQueries({ queryKey: [...key, "retention"] }) });
}
