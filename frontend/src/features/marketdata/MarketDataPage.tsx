import { useTranslation } from "react-i18next";
import { RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useRefreshAll, useRefreshRequiredFX } from "@/queries/marketdata";

const STATUS_VARIANT: Record<string, "success" | "secondary" | "destructive" | "warning"> = {
  fetched: "success",
  cached: "secondary",
  skipped: "secondary",
  failed: "destructive",
  rate_limited: "warning",
};

/**
 * MarketDataPage implements explicit, user-triggered provider refresh
 * (implementation plan Phase 5) through MarketDataService. Refresh is
 * never automatic and never required for startup or any read (migration
 * plan Sec4's privacy/local-first boundary), matching the current Fyne
 * page's behavior exactly.
 */
export function MarketDataPage() {
  const { t } = useTranslation();
  const refreshAll = useRefreshAll();
  const refreshRequiredFX = useRefreshRequiredFX();

  return (
    <div className="flex flex-col gap-4">
      <div className="flex gap-2">
        <Button onClick={() => refreshAll.mutate()} disabled={refreshAll.isPending}>
          <RefreshCw className="size-4" /> {t("nav.marketData")}
        </Button>
        <Button variant="outline" onClick={() => refreshRequiredFX.mutate()} disabled={refreshRequiredFX.isPending}>
          <RefreshCw className="size-4" /> FX
        </Button>
      </div>

      {refreshAll.isError && (
        <p role="alert" className="text-sm text-destructive">
          {(refreshAll.error as Error).message}
        </p>
      )}

      {refreshAll.data && (
        <ul className="flex flex-col gap-2" data-testid="refresh-results">
          {(refreshAll.data.items ?? []).length === 0 && <li className="text-sm text-muted-foreground">No refresh targets found.</li>}
          {(refreshAll.data.items ?? []).map((item) => (
            <li key={item.targetKey} className="flex items-center justify-between rounded-md border border-border px-3 py-2 text-sm">
              <span>{item.targetKey}</span>
              <Badge variant={STATUS_VARIANT[item.status] ?? "secondary"}>{item.status}</Badge>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
