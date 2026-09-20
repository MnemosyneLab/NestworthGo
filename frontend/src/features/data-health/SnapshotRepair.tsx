import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { HealthFocus } from "@/app/navigation";
import { useRebuildHistoricalSnapshots } from "@/queries/history";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { displayError } from "@/lib/display";

/** The snapshot API accepts at most 31 days per transaction. Retry resumes at the failed chunk. */
export function SnapshotRepair({ focus, onClose }: { focus: HealthFocus; onClose: () => void }) {
  const { t } = useTranslation();
  const rebuild = useRebuildHistoricalSnapshots();
  const [nextDate, setNextDate] = useState(focus.rangeStart ?? "");
  const [running, setRunning] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState<string>();
  const run = async () => {
    if (!nextDate || !focus.rangeEnd) return;
    setRunning(true); setError(undefined);
    let from = nextDate;
    try {
      while (from <= focus.rangeEnd) {
        const date = new Date(`${from}T00:00:00Z`);
        date.setUTCDate(date.getUTCDate() + 30);
        const to = date.toISOString().slice(0, 10) < focus.rangeEnd ? date.toISOString().slice(0, 10) : focus.rangeEnd;
        await rebuild.mutateAsync({ startDate: from, endDate: to });
        const next = new Date(`${to}T00:00:00Z`); next.setUTCDate(next.getUTCDate() + 1);
        from = next.toISOString().slice(0, 10); setNextDate(from);
      }
      setDone(true);
    } catch (cause) { setError(displayError(cause, t("dataHealth.loadError"))); }
    finally { setRunning(false); }
  };
  return <Sheet open onOpenChange={open => { if (!open && !running) onClose(); }}><SheetContent><SheetHeader><SheetTitle>{t("connections.rebuildSnapshots")}</SheetTitle></SheetHeader>
    <p>{focus.rangeStart} – {focus.rangeEnd}</p>
    <p className="text-sm text-muted-foreground">{t("connections.snapshotNote")}</p>
    {running && <p role="status">{t("common.pending")} · {nextDate}</p>}
    {error && <p role="alert">{error}</p>}
    {done ? <p role="status">{t("connections.rebuilt")}</p> : <Button disabled={running || !nextDate || !focus.rangeEnd} onClick={() => void run()}>{t(error ? "common.retryAction" : "connections.rebuildSnapshots")}</Button>}
    <Button variant="outline" disabled={running} onClick={onClose}>{t("common.close")}</Button>
  </SheetContent></Sheet>;
}
