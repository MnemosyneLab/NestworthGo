import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Service as HouseholdService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/household";
import type { CompleteOnboardingRequest } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/household/models";
import { callService } from "@/lib/wails";
import { queryKeys } from "@/queries/keys";

export const bootstrapQueryKey = queryKeys.household.bootstrap;

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

export function useCompleteOnboarding() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (request: CompleteOnboardingRequest) => callService(() => HouseholdService.CompleteOnboarding(request)),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.household.bootstrap });
    },
  });
}
