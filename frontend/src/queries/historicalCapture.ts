import { useEffect, useId, useMemo } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { callCancellableService } from "@/lib/wailsCancellation";

/** In-memory handle to a settled capture; contains no financial results. */
export interface HistoricalCaptureContext {
  id: string;
  request: number;
  seriesMode: boolean;
}

/** A capture belongs to this page session. Key changes/unmount consume the
 * query signal; disabling a retained observer needs explicit cancellation too.
 * Settled captures remain fixed for History Back and explicit refresh.
 */
export function useHistoricalCapture<T>(
  kind: "historical-overview" | "historical-series",
  from: string, to: string, enabled: boolean, request: number,
  call: () => Promise<T> & { cancel?: () => unknown },
  sharedSession?: string,
) {
  const localSession = useId();
  const session = sharedSession ?? localSession;
  const client = useQueryClient();
  const queryKey = useMemo(() => [kind, session, from, to, request], [kind, session, from, to, request]);
  useEffect(() => {
    let disposed = false;
    // Observer options are updated by useQuery's passive effects. Check after
    // all hosts have applied them so a visible fork can take over a pending read.
    if (!enabled) queueMicrotask(() => {
      if (!disposed && !client.getQueryCache().find({ queryKey, exact: true })?.isActive()) {
        void client.cancelQueries({ queryKey, exact: true });
      }
    });
    return () => { disposed = true; };
  }, [client, enabled, queryKey]);
  return useQuery({
    queryKey,
    queryFn: ({ signal }) => callCancellableService(signal, call),
    enabled,
    staleTime: Infinity,
    gcTime: 0,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    retry: false,
  });
}
