import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { displayError } from "@/lib/display";
import { useInspectBackup, useConfirmRestore } from "@/queries/data";

export function RestoreBackupButton({ onboarding = false, disabled = false, onBusyChange }: { onboarding?: boolean; disabled?: boolean; onBusyChange?: (busy: boolean) => void }) {
  const { t } = useTranslation();
  const inspect = useInspectBackup();
  const confirmRestore = useConfirmRestore();
  const [restoreOpen, setRestoreOpen] = useState(false);
  const [acknowledged, setAcknowledged] = useState(false);
  const [confirmation, setConfirmation] = useState("");
  const [restoreChrome, setRestoreChrome] = useState(false);
  const [restoreFormat, setRestoreFormat] = useState(false);
  const [restarting, setRestarting] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const request = useRef(false);
  const busy = inspect.isPending || confirmRestore.isPending || restoreOpen || restarting;
  useEffect(() => { onBusyChange?.(busy); return () => onBusyChange?.(false); }, [busy, onBusyChange]);
  const preview = inspect.data && !inspect.data.cancelled ? inspect.data : null;
  const restoreReady = acknowledged && confirmation.trim().toLowerCase() === "restore" && Boolean(preview?.token);

  const runInspect = () => {
    if (request.current || busy || disabled) return;
    request.current = true; onBusyChange?.(true);
    inspect.mutate(undefined, {
      onSettled: () => { request.current = false; },
      onSuccess: (result) => {
        if (!result.cancelled) {
          setAcknowledged(false);
          setConfirmation("");
          setRestoreOpen(true);
        }
      },
      onError: (error) => toast.error(displayError(error, t("settings.saveError"))),
    });
  };

  const runRestore = () => {
    if (!restoreReady || !preview?.token || request.current || confirmRestore.isPending || restarting) {
      return;
    }
    request.current = true;
    confirmRestore.mutate(
      { token: preview.token, confirmation, acknowledged, restoreChrome, restoreFormat, restoreRouting: false },
      {
        onSettled: () => { request.current = false; },
        onSuccess: (result) => {
          if (result.restartRequired) {
            setRestarting(true);
          }
        },
        onError: (error) => toast.error(displayError(error, t("settings.saveError"))),
      },
    );
  };

  return (
    <>
    <Button ref={trigger} type="button" variant="outline" onClick={runInspect} disabled={disabled || inspect.isPending || confirmRestore.isPending || restarting}>{t("settings.data.restore")}</Button>
      <AlertDialog open={restoreOpen || restarting} onOpenChange={(open) => { if (!confirmRestore.isPending && !restarting) { setRestoreOpen(open); if (!open) { inspect.reset(); setAcknowledged(false); setConfirmation(""); } } }}>
        <AlertDialogContent finalFocus={trigger} className="max-w-lg max-h-[calc(100dvh-2rem)] overflow-y-auto">
          <AlertDialogHeader>
            <AlertDialogTitle>{restarting ? t("settings.data.restoreRestart") : t(onboarding ? "onboarding.restoreTitle" : "settings.data.restoreTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t(onboarding ? "onboarding.restoreDescription" : "settings.data.restoreDescription")}</AlertDialogDescription>
          </AlertDialogHeader>
          {preview && !restarting ? (
            <div className={`grid gap-3 text-sm ${onboarding ? "" : "sm:grid-cols-2"}`}>
              {!onboarding && <div>
                <p className="font-medium">{t("settings.data.restoreCurrent")}</p>
                <p>{t("settings.data.restoreHousehold", { name: preview.currentHousehold || "—", currency: preview.currentCurrency || "—" })}</p>
                <p>{t("settings.data.restoreCounts", { accounts: preview.currentAccounts ?? 0, holdings: preview.currentHoldings ?? 0, activities: preview.currentActivities ?? 0 })}</p>
              </div>}
              <div>
                <p className="font-medium">{t("settings.data.restoreBackup")}</p>
                <p>{t("settings.data.restoreHousehold", { name: preview.backupHousehold || "—", currency: preview.backupCurrency || "—" })}</p>
                <p>{t("settings.data.restoreCounts", { accounts: preview.backupAccounts ?? 0, holdings: preview.backupHoldings ?? 0, activities: preview.backupActivities ?? 0 })}</p>
              </div>
            </div>
          ) : null}
          {!restarting ? (
            <div className="flex flex-col gap-2 text-sm">
              <p>{t("settings.data.restoreClassesHelp")}</p>
              <label className="flex items-center gap-2"><input type="checkbox" checked={restoreChrome} onChange={(event) => setRestoreChrome(event.target.checked)} /> {t("settings.data.restoreChrome")}</label>
              <label className="flex items-center gap-2"><input type="checkbox" checked={restoreFormat} onChange={(event) => setRestoreFormat(event.target.checked)} /> {t("settings.data.restoreFormat")}</label>
              <label className="flex items-center gap-2"><input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} /> {t(onboarding ? "onboarding.restoreAcknowledge" : "settings.data.restoreAcknowledge")}</label>
              <Label htmlFor="restore-confirm">{t("settings.data.restoreTypeLabel")}</Label>
              <Input id="restore-confirm" value={confirmation} onChange={(event) => setConfirmation(event.target.value)} autoComplete="off" />
            </div>
          ) : null}
          <AlertDialogFooter>
            {!restarting ? <AlertDialogCancel disabled={confirmRestore.isPending}>{t("common.cancel")}</AlertDialogCancel> : null}
            {!restarting ? (
              <Button type="button" onClick={runRestore} disabled={!restoreReady || confirmRestore.isPending}>{t(onboarding ? "onboarding.restoreConfirm" : "settings.data.restoreConfirm")}</Button>
            ) : null}
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
