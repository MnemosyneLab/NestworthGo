import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import type { HealthFocus } from "@/app/navigation";
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
  const executable = (sync.preview.data?.estimatedRequestCount ?? 0) > 0 || (sync.preview.data?.snapshotWorkEstimate ?? 0) > 0;
  return <>
    <Button variant="outline" size="sm" disabled={sync.running || sync.preview.isPending} onClick={() => sync.preview.mutate(request, {
      onSuccess: () => setOpen(true), onError: error => toast.error(displayError(error, t("marketData.loadError"))),
    })}>{t("connections.previewRepair")}</Button>
    <Sheet open={open} onOpenChange={setOpen}><SheetContent><SheetHeader><SheetTitle>{t("dataHealth.repairTitle")}</SheetTitle></SheetHeader>
      <p className="text-sm">{t("connections.repairScope")}</p>
      <p>{t("marketData.previewRequests", { count: sync.preview.data?.estimatedRequestCount ?? 0 })}</p>
      <SyncWorkDetails items={sync.preview.data?.items} />
      {(sync.preview.data?.unresolved ?? []).map(item => <p key={`${item.targetKey}-${item.code}`} className="text-sm">{t("marketData.blocker", { target: item.targetKey, reason: item.reason || item.code })}</p>)}
      {!executable && <p role="status">{t("connections.noExecutableWork")}</p>}
      <Button disabled={!executable || sync.start.isPending || sync.running} onClick={() => { setOpen(false); sync.startScope(request); }}>{t("dataHealth.confirmRepair")}</Button>
    </SheetContent></Sheet>
  </>;
}
