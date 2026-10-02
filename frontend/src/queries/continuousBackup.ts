import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Service } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/continuousbackup";
import type { RecoveryPoint, Update } from "../../bindings/github.com/waltwang/nestworth-go/internal/infrastructure/continuousbackup/models";
import type { RestoreConfirmRequest } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/recovery/models";
import { callService } from "@/lib/wails";
const key = ["continuousBackup"] as const;
export function useContinuousBackup() {
  return useQuery({ queryKey: key, queryFn: () => callService(() => Service.Status()), retry: false, refetchInterval: 5000 });
}
export function useConfigureBackup() {
  const client = useQueryClient();
  return useMutation({ mutationFn: (input: Update) => callService(() => Service.Configure(input)), onSuccess: (value) => { client.setQueryData(key, value); client.setQueryData([...key, "recoveryPoints"], []); } });
}
export function useTestBackupConnection() {
  return useMutation({ mutationFn: () => callService(() => Service.TestConnection()) });
}
export function useBackupNow() {
  const client = useQueryClient();
  return useMutation({ mutationFn: () => callService(() => Service.BackupNow()), onSettled: () => void client.invalidateQueries({ queryKey: key }) });
}
export function useCloudRecoveryPoints() {
  return useQuery({ queryKey: [...key, "recoveryPoints"], queryFn: () => callService(() => Service.RecoveryPoints()), enabled: false, retry: false });
}
export function useInspectCloudRestore() {
  return useMutation({ mutationFn: (point: RecoveryPoint) => callService(() => Service.InspectRestore(point)) });
}
export function useConfirmCloudRestore() {
  return useMutation({ mutationFn: (input: RestoreConfirmRequest) => callService(() => Service.ConfirmRestore(input)) });
}

export function useRetentionStatus(scope: string) {
  return useQuery({ queryKey: [...key, "retention", scope], queryFn: () => callService(() => Service.RetentionStatus()), retry: false, refetchInterval: 5000 });
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
  return useMutation({ mutationFn: (token: string) => callService(() => Service.ExecuteRetention({ token, acknowledged: true })), onSettled: () => { client.setQueryData([...key, "recoveryPoints"], []); void client.invalidateQueries({ queryKey: key }); } });
}
export function useCancelCleanup() {
  return useMutation({ mutationFn: () => callService(() => Service.CancelCleanup()) });
}
