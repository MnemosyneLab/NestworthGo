import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Events } from "@wailsio/runtime";
import { Service as AgentService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/agent";
import { callService } from "@/lib/wails";

export { AgentService };
export const agentKey = ["agent"] as const;
export function useAgentStatus() {
  return useQuery({ queryKey: [...agentKey, "status"], queryFn: () => callService(() => AgentService.Status()), refetchInterval: 5000, retry: false });
}
export function useAgentOperations() {
  return useQuery({ queryKey: [...agentKey, "operations"], queryFn: () => callService(() => AgentService.RecentOperations()), refetchInterval: 5000, retry: false });
}
/** A write from another client must refresh mounted pages and stale hidden data. */
export function AgentWorkspaceObserver() {
  const client = useQueryClient();
  useEffect(() => Events.On("agent.data.changed", () => { void client.invalidateQueries(); }), [client]);
  return null;
}
