import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { useCancelCleanup, useConfigureRetention, useExecuteRetention, usePreviewRetention, useRetentionStatus } from "@/queries/continuousBackup";

export function HistoryRetentionSection({ scope, disabled, onBusyChange, active = true }: { active?: boolean; scope: string; disabled: boolean; onBusyChange: (busy: boolean) => void }) {
  const { t } = useTranslation();
  const status = useRetentionStatus(scope, active);
  const configure = useConfigureRetention();
  const preview = usePreviewRetention();
  const execute = useExecuteRetention();
  const cancel = useCancelCleanup();
  const dialogTrigger = useRef<HTMLElement | null>(null);
  const [selectedDays, setSelectedDays] = useState<number | null>(null);
  const [dialog, setDialog] = useState<"enable" | "execute" | null>(null);
  const [acknowledged, setAcknowledged] = useState(false);
  const busy = configure.isPending || preview.isPending || execute.isPending || Boolean(status.data?.running);
  const interactionBusy = busy || dialog !== null;
  useEffect(() => { onBusyChange(interactionBusy); return () => onBusyChange(false); }, [interactionBusy, onBusyChange]);
  const current = status.data;
  const days = selectedDays ?? current?.days ?? 30;
  const plan = preview.data;
  const items = plan?.items ?? [];
  const stamp = (value: string) => value ? new Date(value).toLocaleString() : t("cloudBackup.never");
  const open = (value: "enable" | "execute", trigger: HTMLElement) => { dialogTrigger.current = trigger; setAcknowledged(false); setDialog(value); };
  const reset = () => { setDialog(null); setAcknowledged(false); preview.reset(); };
  const ready = current?.enabled && days === current.days && plan?.ready && items.some((item) => item.eligible);
  return <section className="space-y-3 border-t pt-4" aria-labelledby="history-retention-title">
    <h3 id="history-retention-title" className="font-medium">{t("retention.title")}</h3>
    <p className="text-sm text-muted-foreground">{t("retention.description")}</p>
    {status.isLoading && <p role="status">{t("ui.state.loadingPage")}</p>}
    {!status.isLoading && !current && <div role="alert"><p>{t("retention.error")}</p><Button variant="outline" onClick={() => void status.refetch()}>{t("common.retryAction")}</Button></div>}
    {current && <>
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={current.enabled} disabled={disabled || busy} onChange={(event) => { if (event.target.checked) open("enable", event.currentTarget); else configure.mutate({ enabled: false, days: current.days, acknowledged: false }, { onSuccess: reset }); }} />{t("retention.enable")}</label>
      <div className="flex flex-wrap items-end gap-2"><div className="space-y-1"><Label htmlFor="history-retention-days">{t("retention.period")}</Label><NativeSelect id="history-retention-days" value={days} disabled={disabled || busy} onChange={(event) => { setSelectedDays(Number(event.target.value)); preview.reset(); }}><option value={30}>{t("retention.days", { count: 30 })}</option><option value={90}>{t("retention.days", { count: 90 })}</option></NativeSelect></div>
        {days !== current.days && <Button variant="outline" disabled={disabled || busy} onClick={(event) => { if (current.enabled) open("enable", event.currentTarget); else configure.mutate({ enabled: false, days, acknowledged: false }, { onSuccess: reset }); }}>{t("retention.apply")}</Button>}
        <Button variant="outline" disabled={disabled || busy} onClick={() => { execute.reset(); preview.mutate(days); }}>{t("retention.preview")}</Button>
        <Button variant="outline" disabled={disabled || busy || !ready} onClick={(event) => open("execute", event.currentTarget)}>{t("retention.clean")}</Button>
        {(current.running || preview.isPending || execute.isPending) && <Button variant="outline" disabled={cancel.isPending} onClick={() => cancel.mutate()}>{t("retention.stop")}</Button>}
      </div>
      <div role="status" aria-live="polite" className="space-y-1 text-sm">
        {busy && <p>{t("common.pending")}</p>}
        <p>{t("retention.scan", { scanned: (plan?.scannedBytes ?? current.scannedBytes).toLocaleString(), eligible: (plan?.eligibleBytes ?? current.eligibleBytes).toLocaleString(), time: stamp(plan?.scannedAt ?? current.scannedAt) })}</p>
        <p>{t("retention.last", { time: stamp(current.lastCleanup), result: t(`retention.results.${current.result || "none"}`, { defaultValue: t("retention.results.stopped") }) })}</p>
      </div>
      {(configure.isError || preview.isError || execute.isError || cancel.isError || current.errorSummary) && <p role="alert" className="text-sm text-destructive">{t("retention.error")}</p>}
      {plan && <div className="space-y-2 text-sm">
        {!plan.ready && <p>{t("retention.insufficient")}</p>}
        {items.length === 0 ? <p>{t("retention.empty")}</p> : <ul className="max-h-60 space-y-2 overflow-auto" aria-label={t("retention.candidates")}>{items.map((item) => <li key={item.streamID} className="break-all"><span>{item.streamID}</span><br /><span>{item.bytes.toLocaleString()} {t("retention.bytes")} · {t(`retention.reasons.${item.reason}`, { defaultValue: t("retention.reasons.unsealed") })}</span></li>)}</ul>}
      </div>}
    </>}
    <AlertDialog open={dialog !== null} onOpenChange={(value) => { if (!value && !busy) { setDialog(null); setAcknowledged(false); } }}>
      <AlertDialogContent finalFocus={dialogTrigger} className="max-h-[calc(100dvh-2rem)] overflow-y-auto"><AlertDialogHeader><AlertDialogTitle>{t(dialog === "execute" ? "retention.clean" : "retention.enable")}</AlertDialogTitle><AlertDialogDescription>{t("retention.warning")}</AlertDialogDescription></AlertDialogHeader>
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={acknowledged} disabled={busy} onChange={(event) => setAcknowledged(event.target.checked)} />{t("retention.acknowledge")}</label>
        <AlertDialogFooter>{(execute.isPending || current?.running) && <Button variant="outline" disabled={cancel.isPending} onClick={() => cancel.mutate()}>{t("retention.stop")}</Button>}<AlertDialogCancel disabled={busy}>{t("common.cancel")}</AlertDialogCancel><Button disabled={!acknowledged || busy || disabled} onClick={() => {
          if (!acknowledged || busy || disabled) return;
          if (dialog === "enable") configure.mutate({ enabled: true, days, acknowledged: true }, { onSuccess: reset });
          else if (dialog === "execute" && plan && ready) execute.mutate(plan.token, { onSettled: reset });
        }}>{t(dialog === "execute" ? "retention.confirmDelete" : "retention.confirmEnable")}</Button></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </section>;
}
