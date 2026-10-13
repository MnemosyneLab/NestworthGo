import { useEffect, useId, useMemo } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { callCancellableService } from "@/lib/wails";

/** A capture belongs to this page session. Key changes/unmount consume the
 * query signal; disabling a retained observer needs explicit cancellation too.
 * Settled captures remain fixed for History Back and explicit refresh.
 */
export function useHistoricalCapture<T>(
  kind: "historical-overview" | "historical-series",
  from: string, to: string, enabled: boolean, request: number,
  call: () => Promise<T> & { cancel?: () => unknown },
) {
  const session = useId();
  const client = useQueryClient();
  const queryKey = useMemo(() => [kind, session, from, to, request], [kind, session, from, to, request]);
  useEffect(() => {
    if (!enabled) void client.cancelQueries({ queryKey, exact: true });
  }, [client, enabled, queryKey]);
  return useQuery({
    queryKey,
    queryFn: ({ signal }) => callCancellableService(signal, call),
    enabled,
    staleTime: "static",
    gcTime: 0,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    retry: false,
  });
}
