import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { Service as SettingsService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings";
import type { SettingsDTO as Settings, CoinGeckoKeyStatusDTO, TiingoKeyStatusDTO } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings/models";
import { applySettingsDisplayPreferences } from "@/lib/displayPreferences";
import { callService } from "@/lib/wails";
import { queryKeys } from "@/queries/keys";

export const settingsQueryKey = queryKeys.settings.all;

export function useSettings(options?: { enabled?: boolean }) {
  const query = useQuery({
    queryKey: settingsQueryKey,
    queryFn: () => callService(() => SettingsService.Load()),
    enabled: options?.enabled ?? true,
    // Settings are local and deterministic; repeated retries only prolong
    // the startup loading screen and cannot repair a read failure.
    retry: false,
  });
  applySettingsDisplayPreferences(query.data);
  return query;
}

// All hook instances share submission ordering, including reset. Explicit patch
// fields win; untouched fields come from the latest authoritative document.
const writes = new WeakMap<QueryClient, Promise<unknown>>();
function enqueue<T>(client: QueryClient, write: () => Promise<T>): Promise<T> {
  const result = (writes.get(client) ?? Promise.resolve()).catch(() => undefined).then(write);
  writes.set(client, result);
  void result.finally(() => {
    if (writes.get(client) === result) writes.delete(client);
  }).catch(() => undefined);
  return result;
}

export function useSaveSettings() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (patch: Partial<Settings>) => enqueue(client, async () => {
      await client.cancelQueries({ queryKey: settingsQueryKey });
      const latest = await callService(() => SettingsService.Load());
      await callService(() => SettingsService.Save({ ...latest, ...patch }));
      const saved = await callService(() => SettingsService.Load());
      client.setQueryData(settingsQueryKey, saved);
      return saved;
    }),
  });
}

export function useResetSettings() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () => enqueue(client, async () => {
      await client.cancelQueries({ queryKey: settingsQueryKey });
      const defaults = await callService(() => SettingsService.Reset());
      client.setQueryData(settingsQueryKey, defaults);
      return defaults;
    }),
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
    queryFn: () => callService(() => SettingsService.FXProviders()),
    staleTime: Infinity,
  });
}

export function useTiingoKeyStatus() {
  return useQuery({
    queryKey: queryKeys.settings.tiingoKey,
    queryFn: () => callService(() => SettingsService.TiingoKeyStatus()),
    staleTime: 30_000,
  });
}

export function useSaveTiingoAPIKey() {
  const queryClient = useQueryClient();
  return useMutation<TiingoKeyStatusDTO, Error, string>({
    mutationFn: (value) => callService(() => SettingsService.SaveTiingoAPIKey(value)),
    onSuccess: (status) => queryClient.setQueryData(queryKeys.settings.tiingoKey, status),
  });
}

export function useDeleteTiingoAPIKey() {
  const queryClient = useQueryClient();
  return useMutation<TiingoKeyStatusDTO, Error, void>({
    mutationFn: () => callService(() => SettingsService.DeleteTiingoAPIKey()),
    onSuccess: (status) => queryClient.setQueryData(queryKeys.settings.tiingoKey, status),
  });
}

export type QuoteCacheTTL = "1h" | "3h" | "12h" | "24h";

export function quoteCacheTtlOf(settings: Settings): QuoteCacheTTL {
  const value = settings.quoteCacheTTL;
  if (value === "1h" || value === "3h" || value === "12h" || value === "24h") {
    return value;
  }
  return "12h";
}

export function withQuoteCacheTtl(settings: Settings, ttl: QuoteCacheTTL): Settings {
  return { ...settings, quoteCacheTTL: ttl };
}

export function useCoinGeckoKeyStatus() {
  return useQuery({
    queryKey: queryKeys.settings.coinGeckoKey,
    queryFn: () => callService(() => SettingsService.CoinGeckoKeyStatus()),
    staleTime: 30_000,
  });
}

export function useSaveCoinGeckoAPIKey() {
  const queryClient = useQueryClient();
  return useMutation<CoinGeckoKeyStatusDTO, Error, string>({
    mutationFn: (value) => callService(() => SettingsService.SaveCoinGeckoAPIKey(value)),
    onSuccess: (status) => {
      queryClient.setQueryData(queryKeys.settings.coinGeckoKey, status);
      void queryClient.invalidateQueries({ queryKey: ["coingecko-currencies"] });
      void queryClient.invalidateQueries({ queryKey: queryKeys.marketdata.health });
    },
  });
}

export function useDeleteCoinGeckoAPIKey() {
  const queryClient = useQueryClient();
  return useMutation<CoinGeckoKeyStatusDTO, Error, void>({
    mutationFn: () => callService(() => SettingsService.DeleteCoinGeckoAPIKey()),
    onSuccess: (status) => {
      queryClient.setQueryData(queryKeys.settings.coinGeckoKey, status);
      void queryClient.invalidateQueries({ queryKey: ["coingecko-currencies"] });
      void queryClient.invalidateQueries({ queryKey: queryKeys.marketdata.health });
    },
  });
}
