import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Service as MarketDataService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata";
import { callService } from "@/lib/wails";
import { invalidateQuoteReads, invalidateRefreshAll, invalidateRequiredFX } from "@/queries/invalidation";

function requireRefreshSuccess<T extends { items?: Array<{ status: string }> | null }>(result: T): T {
  const unsuccessful = result.items?.find((item) => item.status === "failed" || item.status === "rate_limited" || item.status === "skipped");
  if (unsuccessful) {
    throw new Error("Quote update was not completed");
  }
  return result;
}

/**
 * useRefreshAll calls the synchronous RefreshAll() binding. The Go
 * service also exposes a cancellable, event-streamed
 * StartRefreshAll/CancelRefresh pair for a future
 * "cancel a long-running refresh" UX; this page uses the simpler
 * synchronous call, which TanStack Query's useMutation already treats as
 * async (a real Wails call is a Promise regardless of which Go-side
 * variant is used).
 */
export function useRefreshAll() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => callService(() => MarketDataService.RefreshAll()),
    onSuccess: () => invalidateRefreshAll(queryClient),
  });
}

export function useRefreshRequiredFX() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => callService(() => MarketDataService.RefreshRequiredFX()),
    onSuccess: () => invalidateRequiredFX(queryClient),
  });
}

export function useRefreshMissingOrStale() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => {
      const service = MarketDataService as typeof MarketDataService & {
        RefreshMissingOrStale?: () => ReturnType<typeof MarketDataService.RefreshAll>;
      };
      if (typeof service.RefreshMissingOrStale === "function") {
        return callService(() => service.RefreshMissingOrStale());
      }
      return callService(() => MarketDataService.RefreshRequiredFX());
    },
    onSuccess: () => invalidateRefreshAll(queryClient),
  });
}

export function useRefreshInstrument() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (instrumentId: string) => requireRefreshSuccess(await callService(() => MarketDataService.RefreshInstrument(instrumentId))),
    onSuccess: (_data, instrumentId) => invalidateQuoteReads(queryClient, instrumentId),
  });
}

export function useRefreshFX() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ currencyA, currencyB }: { currencyA: string; currencyB: string }) =>
      requireRefreshSuccess(await callService(() => MarketDataService.RefreshFX(currencyA, currencyB))),
    onSuccess: () => invalidateRequiredFX(queryClient),
  });
}
