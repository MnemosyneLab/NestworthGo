import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Service as SettingsService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings";
import type { Settings } from "../../bindings/github.com/waltwang/nestworth-go/internal/settings/models";
import { callService } from "@/lib/wails";

export const settingsQueryKey = ["settings"] as const;

export function useSettings() {
  return useQuery({
    queryKey: settingsQueryKey,
    queryFn: () => callService(() => SettingsService.Load()),
  });
}

export function useSaveSettings() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (value: Settings) => callService(() => SettingsService.Save(value)),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: settingsQueryKey }),
  });
}

export function useResetSettings() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => callService(() => SettingsService.Reset()),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: settingsQueryKey }),
  });
}

/** useSupportedCurrencies backs every currency <select> in the frontend
 * (Onboarding's base currency, Account's currency), so the list always
 * matches internal/settings.SupportedCurrencies() rather than a
 * hand-duplicated frontend copy. */
export function useSupportedCurrencies() {
  return useQuery({
    queryKey: ["settings", "supportedCurrencies"],
    queryFn: () => callService(() => SettingsService.SupportedCurrencies()),
    staleTime: Infinity,
  });
}
