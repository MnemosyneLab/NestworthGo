import { useQuery } from "@tanstack/react-query";
import { Service as PortfolioService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/portfolio";
import { callService } from "@/lib/wails";
import { queryKeys } from "@/queries/keys";

export const overviewQueryKey = queryKeys.overview.all;

/** useOverview loads the Overview page's data through PortfolioService,
 * with no account filter (the whole-Household view). */
export function useOverview() {
  return useQuery({
    queryKey: queryKeys.overview.all,
    queryFn: () => callService(() => PortfolioService.Overview({})),
  });
}

/** usePortfolio loads the independent Portfolio page. The Go service
 * includes holding components only; cash and include_in_portfolio are ignored. */
export function usePortfolio() {
  return useQuery({
    queryKey: queryKeys.portfolio.all,
    queryFn: () => callService(() => PortfolioService.Portfolio({})),
  });
}
