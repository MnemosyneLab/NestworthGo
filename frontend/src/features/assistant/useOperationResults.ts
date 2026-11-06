import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import * as API from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service";
import { callService } from "@/lib/wails";

/** Reconcile saved receipts without another result panel or mutation dispatch.
 * ConversationTimeline owns the visible tool results and assistant replies. */
export function useOperationResults(epoch: string, id: string, busy: boolean, run?: { id: string; state: string }) {
  const client = useQueryClient();
  const running = busy || run?.state === "accepted" || run?.state === "running";
  const results = useQuery({
    queryKey: ["aiReviews", epoch, id, `${run?.id}:${run?.state}`],
    queryFn: () => callService(() => API.Reviews(epoch, id)),
    enabled: !!epoch && !!id,
    networkMode: "always",
    retry: false,
    refetchInterval: query => running || query.state.status === "error" || query.state.data?.some(result =>
      !["succeeded", "cancelled", "failed"].includes(result.state)) ? 1500 : false,
  });
  const completed = (results.data ?? []).filter(result => result.state === "succeeded").map(result => result.id).join(",");
  useEffect(() => {
    if (completed) void client.invalidateQueries({ predicate: query => !String(query.queryKey[0]).startsWith("ai") });
  }, [client, epoch, id, completed]);
}
