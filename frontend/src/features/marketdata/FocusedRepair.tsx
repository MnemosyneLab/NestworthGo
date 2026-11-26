import { SyncBlockerText } from "@/features/marketdata/SyncBlockerText";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import type { HealthFocus } from "@/app/navigation";
import { ErrorState, LoadingState } from "@/components/layout/PageState";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { useMarketDataSyncActions } from "./MarketDataSyncBar";
import { SyncWorkDetails } from "./SyncWorkDetails";
import { displayError } from "@/lib/display";

export function FocusedRepair({ focus }: { focus: HealthFocus }) {
  const { t } = useTranslation();
  const sync = useMarketDataSyncActions();
  const [open, setOpen] = useState(false);
  const request = focus.instrumentId ? { scope: "instrument", instrumentId: focus.instrumentId }
    : { scope: "fx", currencyA: focus.currencyA, currencyB: focus.currencyB };
  const plan = sync.preview.data;
  const executable = (plan?.estimatedRequestCount ?? 0) > 0 || (plan?.snapshotWorkEstimate ?? 0) > 0;
  const openPreview = () => sync.preview.mutate(request, {
    onSuccess: () => setOpen(true), onError: error => toast.error(displayError(error, t("marketData.loadError"))),
  });
  return <>
    <Button variant="outline" size="sm" disabled={sync.running || sync.preview.isPending} onClick={openPreview}>{t("connections.previewRepair")}</Button>
    <Sheet open={open} onOpenChange={setOpen}><SheetContent><SheetHeader><SheetTitle>{t("dataHealth.repairTitle")}</SheetTitle></SheetHeader>
      <p className="text-sm">{t("connections.repairScope")}</p>
      {sync.preview.isPending && <LoadingState label={t("common.loading")} variant="list" />}
      {sync.preview.isError && <ErrorState
        title={t("marketData.loadError")}
        description={displayError(sync.preview.error, t("marketData.loadError"))}
        onRetry={openPreview}
        retryLabel={t("common.retry")}
      />}
      {plan && <p>{t("marketData.previewRequests", { count: plan.estimatedRequestCount })}</p>}
      <SyncWorkDetails items={plan?.items} />
      {(plan?.unresolved ?? []).map(item => <p key={`${item.targetKey}-${item.code}`} className="text-sm"><SyncBlockerText item={item} /></p>)}
      {plan && !executable && <p role="status">{t("connections.noExecutableWork")}</p>}
      <Button disabled={!executable || !plan?.reviewId || sync.start.isPending || sync.running}
        onClick={() => sync.startReviewed(request, plan?.reviewId, () => setOpen(false))}>{t("dataHealth.confirmRepair")}</Button>
    </SheetContent></Sheet>
  </>;
}
