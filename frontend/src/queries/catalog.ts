import { useQuery } from "@tanstack/react-query";
import { Service as CatalogService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog";
import { callService } from "@/lib/wails";
import { queryKeys } from "@/queries/keys";

export type AccountCombinationDTO = {
  accountType: string;
  balanceSheetRole: string;
  trackingMode: string;
  roleLocked: boolean;
  includeInNetWorth: boolean;
  includeInPortfolio: boolean;
  includeInLiquidAssets: boolean;
  wholeAccountWarning: boolean;
};

export type CatalogDTO = {
  currencies: string[];
  instrumentTypes: string[];
  quoteSources: string[];
  instrumentProviders: string[];
  accountTypes: string[];
  balanceSheetRoles?: string[];
  trackingModes?: string[];
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

/** useCatalog is the frontend's single closed-vocabulary query. Selectors
 * must render options from this payload rather than local string arrays. */
export function useCatalog() {
  return useQuery({
    queryKey: queryKeys.catalog.all,
    queryFn: () => callService(() => CatalogService.Catalog() as Promise<CatalogDTO>),
    staleTime: Infinity,
  });
}
