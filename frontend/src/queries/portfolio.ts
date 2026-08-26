import { useQuery } from "@tanstack/react-query";
import { Service as PortfolioService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/portfolio";
import { callService } from "@/lib/wails";

export const overviewQueryKey = ["overview"] as const;

/** useOverview loads the Overview page's data through PortfolioService,
 * with no account filter (the whole-Household view). */
export function useOverview() {
  return useQuery({
    queryKey: overviewQueryKey,
    queryFn: () => callService(() => PortfolioService.Overview({})),
  });
}
