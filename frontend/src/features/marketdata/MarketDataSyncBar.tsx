import { useState } from "react";
import { useTranslation } from "react-i18next";
import { RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { displayEnum, displayError } from "@/lib/display";
import { formatTimestamp } from "@/lib/time";
import { useSettings } from "@/queries/settings";
import {
  syncJobIsRunning,
  useCancelSyncJob,
  useCurrentSyncJob,
  usePreviewMarketDataSync,
  useStartMarketDataSync,
} from "@/queries/marketdata";
import type { SyncRequestDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata/models";
import type { InstrumentDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

export function useMarketDataSyncActions() {
  const { t } = useTranslation();
  const current = useCurrentSyncJob();
  const preview = usePreviewMarketDataSync();
  const start = useStartMarketDataSync();
  const cancel = useCancelSyncJob();
  const running = syncJobIsRunning(current.data);
  const syncingInstrumentId = running && current.data?.scope?.scope === "instrument" ? current.data.scope.instrumentId : undefined;

  const startScope = (request: SyncRequestDTO) => {
    start.mutate(request, {
      onSuccess: (result) => {
        if (result.conflict) {
          toast.error(result.reason || t("marketData.syncConflict"));
          return;
        }
        if (result.attached) {
          toast.message(t("marketData.syncAttached"));
        }
      },
      onError: (error) => toast.error(displayError(error, t("marketData.loadError"))),
    });
  };

  return {
    current,
    preview,
    start,
    cancel,
    running,
    syncingInstrumentId,
    startScope,
    syncInstrument: (instrument: InstrumentDTO) => startScope({ scope: "instrument", instrumentId: instrument.id }),
  };
}

export function MarketDataSyncBar({
  latestRefreshing,
  onRefreshMissing,
  onRefreshAll,
  onCancelLatest,
  lastUpdatedAt,
}: {
  latestRefreshing: boolean;
  onRefreshMissing: () => void;
  onRefreshAll: () => void;
  onCancelLatest: () => void;
  lastUpdatedAt?: Date;
}) {
  const { t, i18n } = useTranslation();
  const settings = useSettings();
  const { current, preview, start, cancel, running, startScope } = useMarketDataSyncActions();
  const [previewOpen, setPreviewOpen] = useState(false);
  const [forceRecheck, setForceRecheck] = useState(false);
  const [progressOpen, setProgressOpen] = useState(false);
  const job = current.data;
  const busy = latestRefreshing || running || preview.isPending || start.isPending;

  const openPreview = () => {
    preview.mutate(
      { scope: "repair_all", forceRecheck },
      {
        onSuccess: () => setPreviewOpen(true),
        onError: (error) => toast.error(displayError(error, t("marketData.loadError"))),
      },
    );
  };

  const confirmStart = () => {
    setPreviewOpen(false);
    startScope({ scope: "repair_all", forceRecheck });
  };

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        {lastUpdatedAt ? (
          <p className="text-sm text-muted-foreground">
            {t("marketData.dataUpdated", { time: formatTimestamp(lastUpdatedAt, settings.data?.timezone, i18n.language) })}
          </p>
        ) : (
          <span />
        )}
        <div className="flex flex-wrap gap-2">
          <Button onClick={openPreview} disabled={busy}>
            <RefreshCw className="size-4" aria-hidden="true" /> {preview.isPending ? t("common.pending") : t("marketData.syncData")}
          </Button>
          {running && (
            <>
              <Button type="button" variant="outline" onClick={() => setProgressOpen(true)}>
                {t("marketData.viewProgress")}
              </Button>
              <Button type="button" variant="outline" onClick={() => job?.jobId && cancel.mutate(job.jobId)} disabled={cancel.isPending}>
                {t("marketData.cancelSync")}
              </Button>
            </>
          )}
        </div>
      </div>

      {running && job && (
        <p role="status" className="text-sm text-muted-foreground" data-testid="sync-progress">
          {t("marketData.syncingMarketData")}{" "}
          {displayEnum(t, "marketData.phase", job.phase)} {t("marketData.syncProgress", { completed: job.completedTargets, total: job.targetCount })}
        </p>
      )}
      {job && job.outcome === "cancelled" && <p role="status" className="text-sm text-muted-foreground">{t("marketData.syncCancelled")}</p>}

      <div className="flex flex-wrap gap-2">
        <Button variant="outline" onClick={onRefreshMissing} disabled={busy}>
          <RefreshCw className="size-4" aria-hidden="true" /> {latestRefreshing ? t("marketData.refreshing") : t("marketData.refreshMissingOrStale")}
        </Button>
        <Button variant="outline" onClick={onRefreshAll} disabled={busy}>
          <RefreshCw className="size-4" aria-hidden="true" /> {latestRefreshing ? t("marketData.refreshing") : t("marketData.forceRefreshAll")}
        </Button>
        {latestRefreshing && (
          <Button type="button" variant="outline" onClick={onCancelLatest}>
            {t("marketData.cancelRefresh")}
          </Button>
        )}
      </div>

      <Sheet open={previewOpen} onOpenChange={setPreviewOpen}>
        <SheetContent>
          <SheetHeader>
            <SheetTitle>{t("marketData.syncPreviewTitle")}</SheetTitle>
            <p className="text-sm text-muted-foreground">{t("marketData.syncPreviewDescription")}</p>
          </SheetHeader>
          {preview.data && (
            <ul className="flex flex-col gap-2 text-sm" data-testid="sync-preview">
              <li>{t("marketData.previewRequests", { count: preview.data.estimatedRequestCount })}</li>
              <li>{t("marketData.previewInstruments", { count: preview.data.instrumentTargets })}</li>
              <li>{t("marketData.previewFx", { count: preview.data.fxTargets })}</li>
              <li>{t("marketData.previewLatest", { instruments: preview.data.latestInstrumentCount, fx: preview.data.latestFxCount })}</li>
              <li>{t("marketData.previewSnapshots", { count: preview.data.snapshotWorkEstimate })}</li>
            </ul>
          )}
          {(preview.data?.unresolved ?? []).length > 0 && (
            <div className="flex flex-col gap-2">
              <p className="text-sm font-medium">{t("marketData.previewUnresolved")}</p>
              <ul className="flex flex-col gap-1 text-sm text-muted-foreground">
                {(preview.data?.unresolved ?? []).map((item) => (
                  <li key={`${item.targetKey}-${item.code}`}>{t("marketData.blocker", { target: item.targetKey, reason: item.reason || item.code })}</li>
                ))}
              </ul>
            </div>
          )}
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={forceRecheck}
              onChange={(event) => {
                const next = event.target.checked;
                setForceRecheck(next);
                preview.mutate({ scope: "repair_all", forceRecheck: next });
              }}
            />
            {t("marketData.forceRecheck")}
          </label>
          <SheetFooter>
            <Button type="button" variant="outline" onClick={() => setPreviewOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button type="button" onClick={confirmStart} disabled={start.isPending}>
              {t("marketData.startSync")}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>

      <Sheet open={progressOpen} onOpenChange={setProgressOpen}>
        <SheetContent>
          <SheetHeader>
            <SheetTitle>{t("marketData.viewProgress")}</SheetTitle>
          </SheetHeader>
          {job ? (
            <div className="flex flex-col gap-3 text-sm" data-testid="sync-progress-detail">
              <p>{displayEnum(t, "marketData.phase", job.phase)}</p>
              <p>{t("marketData.syncProgress", { completed: job.completedTargets, total: job.targetCount })}</p>
              <p>{t("marketData.previewRequests", { count: job.estimatedRequests })}</p>
              {(job.blockers ?? []).length > 0 && (
                <div>
                  <p className="font-medium">{t("marketData.previewUnresolved")}</p>
                  <ul>
                    {(job.blockers ?? []).map((item) => (
                      <li key={`${item.targetKey}-${item.code}`}>{t("marketData.blocker", { target: item.targetKey, reason: item.reason || item.code })}</li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">{t("marketData.noSavedData")}</p>
          )}
        </SheetContent>
      </Sheet>
    </div>
  );
}
