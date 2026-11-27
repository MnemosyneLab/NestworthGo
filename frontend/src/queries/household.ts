import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Service as HouseholdService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/household";
import type { CompleteOnboardingRequest } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/household/models";
import { callService } from "@/lib/wails";
import { queryKeys } from "@/queries/keys";

/**
 * useBootstrap is the app's single source of truth for "has onboarding
 * completed yet." App.tsx uses `data.household === null` to decide
 * whether to render the Onboarding flow or the main app shell.
 */
export function useBootstrap(options: { enabled?: boolean } = {}) {
  return useQuery({
    queryKey: queryKeys.household.bootstrap,
    queryFn: () => callService(() => HouseholdService.Bootstrap()),
    enabled: options.enabled ?? true,
  });
}

export function useCompleteOnboarding(onCompleted?: () => void) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (request: CompleteOnboardingRequest) => callService(() => HouseholdService.CompleteOnboarding(request)),
    onError: () => {
      // A response can fail after the atomic household/origin commit. Re-read
      // Bootstrap instead of leaving a submitted form as the only recovery.
      void queryClient.invalidateQueries({ queryKey: queryKeys.household.bootstrap });
    },
    onSuccess: () => {
      // Deliver the destination before Bootstrap can remove the onboarding UI.
      onCompleted?.();
      void queryClient.invalidateQueries({ queryKey: queryKeys.household.bootstrap });
      void queryClient.invalidateQueries({ queryKey: queryKeys.history.origin });
    },
  });
}
