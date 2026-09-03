import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Service as DataService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/data";
import type { CSVOptionsRequest, CSVPreviewDTO as GeneratedCSVPreviewDTO } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/data/models";
import { Service as RecoveryService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/recovery";
import type { RestoreConfirmRequest } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/recovery/models";
import { callService } from "@/lib/wails";
import { queryKeys } from "@/queries/keys";

export type CSVPreviewDTO = {
  profile: string;
  headers: string[];
  previewRows: string[][];
  stats: {
    createAccounts: number;
    createHoldings: number;
    createInstruments: number;
    createMembers: number;
    createInstitutions: number;
    createGroups: number;
    references: number;
    warnings: number;
    errors: number;
    duplicates: number;
  };
  errors: { row: number; field: string; source?: string; code: string; message: string; suggestion?: string }[];
  warnings: { row: number; code: string; message: string }[];
  unresolved: { kind: string; name: string; field: string; row: number }[];
  canCommit: boolean;
};

function normalizePreview(preview: GeneratedCSVPreviewDTO): CSVPreviewDTO {
  return {
    profile: preview.profile,
    headers: preview.headers ?? [],
    previewRows: (preview.previewRows ?? []).map((row) => row ?? []),
    stats: preview.stats,
    errors: preview.errors ?? [],
    warnings: preview.warnings ?? [],
    unresolved: preview.unresolved ?? [],
    canCommit: preview.canCommit,
  };
}

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

export function useExportCSV() {
  return useMutation({
    mutationFn: ({ profile, includeArchived }: { profile: string; includeArchived: boolean }) =>
      callService(() => DataService.ExportCSV(profile, includeArchived)),
  });
}

export function useSelectCSV() {
  return useMutation({
    mutationFn: ({ profile, token }: { profile: string; token: string }) =>
      callService(() => DataService.SelectCSV(profile, token)),
  });
}

export function usePreviewCSV() {
  return useMutation({
    mutationFn: (request: CSVOptionsRequest) => callService(() => DataService.PreviewCSV(request).then(normalizePreview)),
  });
}

export function useConfirmCSV() {
  return useMutation({
    mutationFn: (token: string) => callService(() => DataService.ConfirmCSV(token)),
  });
}

export function useCommitCSV() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (token: string) => callService(() => DataService.CommitCSV(token)),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.accounts.all });
      void queryClient.invalidateQueries({ queryKey: queryKeys.holdings.all });
      void queryClient.invalidateQueries({ queryKey: queryKeys.instruments.all });
      void queryClient.invalidateQueries({ queryKey: queryKeys.directory.all });
      void queryClient.invalidateQueries({ queryKey: queryKeys.overview.all });
      void queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.all });
    },
  });
}

export function useCancelCSV() {
  return useMutation({
    mutationFn: (token: string) => callService(() => DataService.CancelCSV(token)),
  });
}

export function useDownloadCSVErrors() {
  return useMutation({
    mutationFn: (token: string) => callService(() => DataService.DownloadCSVErrors(token)),
  });
}
