import { useState } from "react";
import { useTranslation } from "react-i18next";
import { RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { PageHeader } from "@/components/layout/PageHeader";
import { EmptyState, ErrorState } from "@/components/layout/PageState";
import { useRefreshAll, useRefreshRequiredFX } from "@/queries/marketdata";
import { displayEnum, displayError } from "@/lib/display";
import type { RefreshResultDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata/models";

const STATUS_VARIANT: Record<string, "success" | "secondary" | "destructive" | "warning"> = {
  fetched: "success",
  cached: "secondary",
  skipped: "secondary",
  failed: "destructive",
  rate_limited: "warning",
};

function RefreshResults({ result, refreshedAt }: { result: RefreshResultDTO; refreshedAt?: Date }) {
  const { t, i18n } = useTranslation();
  const items = result.items ?? [];
  const updated = items.filter((item) => item.status === "fetched").length;
  const reused = items.filter((item) => item.status === "cached").length;
  const skipped = items.filter((item) => item.status === "skipped").length;
  const failed = items.filter((item) => item.status === "failed" || item.status === "rate_limited").length;

  return (
    <div className="flex flex-col gap-3" aria-live="polite">
      <p className="text-sm text-muted-foreground">
        {t("marketData.refreshSummary", { total: items.length, updated, reused, skipped, failed })}
      </p>
      {refreshedAt && (
        <p className="text-xs text-muted-foreground">
          {t("marketData.refreshedAt", {
            time: new Intl.DateTimeFormat(i18n.language, { dateStyle: "medium", timeStyle: "short" }).format(refreshedAt),
          })}
        </p>
      )}
      {result.rateLimited && <p className="rounded-md border border-warning/40 bg-warning/10 p-3 text-sm text-warning-foreground">{t("marketData.rateLimited")}</p>}
      {items.length === 0 ? (
        <EmptyState title={t("marketData.noTargets")} description={t("marketData.description")} />
      ) : (
        <ul className="flex flex-col gap-2" data-testid="refresh-results">
          {items.map((item) => (
            <li key={`${item.kind}-${item.targetKey}`} className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border px-3 py-3 text-sm">
              <span>{displayEnum(t, "marketData.target", item.kind)}</span>
              <Badge variant={STATUS_VARIANT[item.status] ?? "secondary"}>{displayEnum(t, "marketData.status", item.status)}</Badge>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

/** MarketDataPage makes provider refresh explicit and keeps the provider's
 * technical target identifiers out of the user-facing result list. */
export function MarketDataPage() {
  const { t } = useTranslation();
  const refreshAll = useRefreshAll();
  const refreshRequiredFX = useRefreshRequiredFX();
  const [lastRefresh, setLastRefresh] = useState<"all" | "fx" | null>(null);
  const [result, setResult] = useState<RefreshResultDTO | undefined>();
  const [lastRefreshAt, setLastRefreshAt] = useState<Date | undefined>();
  const activeRefresh = lastRefresh === "fx" ? refreshRequiredFX : refreshAll;

  const runRefreshAll = () => {
    setLastRefresh("all");
    setResult(undefined);
    setLastRefreshAt(undefined);
    refreshAll.mutate(undefined, {
      onSuccess: (next) => {
        setResult(next);
        setLastRefreshAt(new Date());
      },
    });
  };

  const runRefreshFX = () => {
    setLastRefresh("fx");
    setResult(undefined);
    setLastRefreshAt(undefined);
    refreshRequiredFX.mutate(undefined, {
      onSuccess: (next) => {
        setResult(next);
        setLastRefreshAt(new Date());
      },
    });
  };

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t("nav.marketData")} description={t("marketData.description")} />
      <div className="flex flex-wrap gap-2">
        <Button onClick={runRefreshAll} disabled={refreshAll.isPending || refreshRequiredFX.isPending}>
          <RefreshCw className="size-4" aria-hidden="true" /> {refreshAll.isPending ? t("marketData.refreshing") : t("marketData.refreshAll")}
        </Button>
        <Button variant="outline" onClick={runRefreshFX} disabled={refreshAll.isPending || refreshRequiredFX.isPending}>
          <RefreshCw className="size-4" aria-hidden="true" /> {refreshRequiredFX.isPending ? t("marketData.refreshing") : t("marketData.refreshFX")}
        </Button>
      </div>

      {activeRefresh.isError && (
        <ErrorState
          title={t("marketData.loadError")}
          description={displayError(activeRefresh.error, t("ui.state.errorDescription"))}
          onRetry={lastRefresh === "fx" ? runRefreshFX : runRefreshAll}
          retryLabel={t("common.retryAction")}
        />
      )}

      {!activeRefresh.isError && result && <RefreshResults result={result} refreshedAt={lastRefreshAt} />}
      {!activeRefresh.isError && !result && (
        <p className="rounded-md border border-border bg-muted/30 p-3 text-sm text-muted-foreground">{t("marketData.explicitNotice")}</p>
      )}
    </div>
  );
}
