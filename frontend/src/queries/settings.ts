import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Service as SettingsService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings";
import type { Settings } from "../../bindings/github.com/waltwang/nestworth-go/internal/settings/models";
import { callService } from "@/lib/wails";
import { queryKeys } from "@/queries/keys";

export const settingsQueryKey = queryKeys.settings.all;

export function useSettings(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: settingsQueryKey,
    queryFn: () => callService(() => SettingsService.Load()),
    enabled: options?.enabled ?? true,
  });
}

export function useSaveSettings() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (value: Settings) => callService(() => SettingsService.Save(value)),
    onMutate: async (value) => {
      await queryClient.cancelQueries({ queryKey: settingsQueryKey });
      const previous = queryClient.getQueryData<Settings>(settingsQueryKey);
      queryClient.setQueryData(settingsQueryKey, value);
      return { previous };
    },
    onError: (_error, _value, context) => {
      if (context?.previous) {
        queryClient.setQueryData(settingsQueryKey, context.previous);
      }
    },
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
    queryKey: queryKeys.settings.supportedCurrencies,
    queryFn: () => callService(() => SettingsService.SupportedCurrencies()),
    staleTime: Infinity,
  });
}

export function useFXProviders() {
  return useQuery({
    queryKey: queryKeys.settings.fxProviders,
    queryFn: () => {
      const service = SettingsService as typeof SettingsService & { FXProviders?: () => Promise<string[]> };
      return callService(() => service.FXProviders?.() ?? Promise.resolve(["frankfurter"]));
    },
    staleTime: Infinity,
  });
}

export type QuoteCacheTTL = "1h" | "3h" | "12h" | "24h";

export function quoteCacheTtlOf(settings: Settings): QuoteCacheTTL {
  const record = settings as Settings & { quote_cache_ttl?: string; quoteCacheTTL?: string };
  const value = record.quote_cache_ttl ?? record.quoteCacheTTL;
  if (value === "1h" || value === "3h" || value === "12h" || value === "24h") {
    return value;
  }
  return "12h";
}

export function withQuoteCacheTtl(settings: Settings, ttl: QuoteCacheTTL): Settings {
  return { ...settings, quote_cache_ttl: ttl } as Settings;
}
