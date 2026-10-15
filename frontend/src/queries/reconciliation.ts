import { useQuery } from "@tanstack/react-query";
import { Service } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation";
import { callService } from "@/lib/wails";
export { Service as ReconciliationService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation";
export function useReconciliationSessions(accountId?: string) {
  return useQuery({
    queryKey: ["reconciliation", "sessions", accountId ?? ""],
    queryFn: async () => {
      const rows = await callService(() => Service.List());
      return accountId
        ? rows?.filter((row) => row.accountIds?.includes(accountId))
        : rows;
    },
  });
}
export function useReconciliationSession(id: string) {
  return useQuery({
    queryKey: ["reconciliation", "session", id],
    queryFn: () => callService(() => Service.Get(id)),
    enabled: Boolean(id),
  });
}
export function useReconciliationLinks(id: string) {
  return useQuery({
    queryKey: ["reconciliation", "links", id],
    queryFn: () => callService(() => Service.Links(id)),
    enabled: Boolean(id),
  });
}
export function useReconciliationLastSuccess(id: string) {
  return useQuery({
    queryKey: ["reconciliation", "lastSuccess", id],
    queryFn: () => callService(() => Service.LastSuccess(id)),
    enabled: Boolean(id),
  });
}

export function useReconciliationChanges(id: string) {
  return useQuery({
    queryKey: ["reconciliation", "changes", id],
    queryFn: () => callService(() => Service.Changes(id, "")),
    enabled: Boolean(id),
  });
}
