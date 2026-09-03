import { useQuery } from "@tanstack/react-query";
import { Service as CatalogService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog";
import type { AccountCombinationDTO, CatalogDTO as GeneratedCatalogDTO } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog/models";
import { callService } from "@/lib/wails";
import { queryKeys } from "@/queries/keys";

export type { AccountCombinationDTO };

export type CatalogDTO = {
  currencies: string[];
  instrumentTypes: string[];
  institutionTypes: string[];
  quoteSources: string[];
  instrumentProviders: string[];
  instrumentCountryCodes: string[];
  instrumentMarketCodes: string[];
  accountTypes: string[];
  balanceSheetRoles: string[];
  trackingModes: string[];
  accountCombinations: AccountCombinationDTO[];
  trackingModesByAccountType: Record<string, string[]>;
  trendRanges: string[];
  appearances: string[];
  languages: string[];
  accents: string[];
  moneyInReasons: string[];
  moneyOutReasons: string[];
  valueUpdateReasons: string[];
  tradeSides: string[];
};

function normalizeCatalog(value: GeneratedCatalogDTO): CatalogDTO {
  return {
    currencies: value.currencies ?? [],
    instrumentTypes: value.instrumentTypes ?? [],
    institutionTypes: value.institutionTypes ?? [],
    quoteSources: value.quoteSources ?? [],
    instrumentProviders: value.instrumentProviders ?? [],
    instrumentCountryCodes: value.instrumentCountryCodes ?? [],
    instrumentMarketCodes: value.instrumentMarketCodes ?? [],
    accountTypes: value.accountTypes ?? [],
    balanceSheetRoles: value.balanceSheetRoles ?? [],
    trackingModes: value.trackingModes ?? [],
    accountCombinations: value.accountCombinations ?? [],
    trackingModesByAccountType: Object.fromEntries(
      Object.entries(value.trackingModesByAccountType ?? {}).map(([key, modes]) => [key, modes ?? []]),
    ),
    trendRanges: value.trendRanges ?? [],
    appearances: value.appearances ?? [],
    languages: value.languages ?? [],
    accents: value.accents ?? [],
    moneyInReasons: value.moneyInReasons ?? [],
    moneyOutReasons: value.moneyOutReasons ?? [],
    valueUpdateReasons: value.valueUpdateReasons ?? [],
    tradeSides: value.tradeSides ?? [],
  };
}

/** useCatalog is the frontend's single closed-vocabulary query. Selectors
 * must render options from this payload rather than local string arrays. */
export function useCatalog() {
  return useQuery({
    queryKey: queryKeys.catalog.all,
    queryFn: () => callService(() => CatalogService.Catalog()).then(normalizeCatalog),
    staleTime: Infinity,
  });
}
