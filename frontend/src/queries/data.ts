import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Service as DataService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/data";
import { Service as RecoveryService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/recovery";
import type { RestoreConfirmRequest } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/recovery/models";
import { callService } from "@/lib/wails";
import { queryKeys } from "@/queries/keys";

export function useLastBackupStatus() {
  return useQuery({
    queryKey: [...queryKeys.settings.all, "backupStatus"] as const,
    queryFn: () => callService(() => DataService.LastBackupStatus()),
  });
}

export function useCreateBackup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => callService(() => DataService.CreateBackup()),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: queryKeys.settings.all }),
  });
}

export function useInspectBackup() {
  return useMutation({
    mutationFn: () => callService(() => RecoveryService.InspectBackup()),
  });
}

export function useConfirmRestore() {
  return useMutation({
    mutationFn: (request: RestoreConfirmRequest) => callService(() => RecoveryService.ConfirmRestore(request)),
  });
}

export function useExportJSON() {
  return useMutation({ mutationFn: () => callService(() => DataService.ExportJSON()) });
}
