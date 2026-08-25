import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Service as MarketDataService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata";
import { callService } from "@/lib/wails";
import { overviewQueryKey } from "@/queries/portfolio";

/**
 * useRefreshAll calls the synchronous RefreshAll() binding. The Go
 * service also exposes a cancellable, event-streamed
 * StartRefreshAll/CancelRefresh pair (technical design Sec6) for a future
 * "cancel a long-running refresh" UX; this page uses the simpler
 * synchronous call, which TanStack Query's useMutation already treats as
 * async (a real Wails call is a Promise regardless of which Go-side
 * variant is used).
 */
export function useRefreshAll() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => callService(() => MarketDataService.RefreshAll()),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
      void queryClient.invalidateQueries({ queryKey: ["quote"] });
      void queryClient.invalidateQueries({ queryKey: ["holdings"] });
    },
  });
}

export function useRefreshRequiredFX() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => callService(() => MarketDataService.RefreshRequiredFX()),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: overviewQueryKey }),
  });
}
