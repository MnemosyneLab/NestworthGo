import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { displayError } from "@/lib/display";
import {
  useLastBackupStatus,
  useCreateBackup,
  useInspectBackup,
  useConfirmRestore,
  useExportJSON,
} from "@/queries/data";

export function DataManagementSection() {
  const { t } = useTranslation();
  const status = useLastBackupStatus();
  const createBackup = useCreateBackup();
  const inspect = useInspectBackup();
  const confirmRestore = useConfirmRestore();
  const exportJSON = useExportJSON();
  const [restoreOpen, setRestoreOpen] = useState(false);
  const [acknowledged, setAcknowledged] = useState(false);
  const [confirmation, setConfirmation] = useState("");
  const [restoreChrome, setRestoreChrome] = useState(false);
  const [restoreFormat, setRestoreFormat] = useState(false);
  const [restoreRouting, setRestoreRouting] = useState(false);
  const [restarting, setRestarting] = useState(false);
  const preview = inspect.data && !inspect.data.cancelled ? inspect.data : null;
  const restoreReady = acknowledged && confirmation.trim().toLowerCase() === "restore" && Boolean(preview?.token);

  const runBackup = () => {
    createBackup.mutate(undefined, {
      onSuccess: (result) => {
        if (!result.cancelled && result.fileName) {
          toast.success(t("settings.data.backupCreated", { fileName: result.fileName }));
        }
      },
      onError: (error) => toast.error(displayError(error, t("settings.saveError"))),
    });
  };

  const runInspect = () => {
    inspect.mutate(undefined, {
      onSuccess: (result) => {
        if (!result.cancelled) {
          setRestoreOpen(true);
        }
      },
      onError: (error) => toast.error(displayError(error, t("settings.saveError"))),
    });
  };

  const runRestore = () => {
    if (!preview?.token) {
      return;
    }
    confirmRestore.mutate(
      { token: preview.token, confirmation, acknowledged, restoreChrome, restoreFormat, restoreRouting },
      {
        onSuccess: (result) => {
          if (result.restartRequired) {
            setRestarting(true);
          }
        },
        onError: (error) => toast.error(displayError(error, t("settings.saveError"))),
      },
    );
  };

  const runExport = () => {
    exportJSON.mutate(undefined, {
      onSuccess: (result) => {
        if (!result.cancelled && result.fileName) {
          toast.success(t("settings.data.exported", { fileName: result.fileName }));
        }
      },
      onError: (error) => toast.error(displayError(error, t("settings.saveError"))),
    });
  };

  return (
    <section className="flex max-w-2xl flex-col gap-3" aria-labelledby="settings-data-title">
      <div>
        <h2 id="settings-data-title" className="text-base font-semibold">{t("settings.data.title")}</h2>
        <p className="text-sm text-muted-foreground">{t("settings.data.description")}</p>
      </div>
      <p className="text-sm text-muted-foreground">{t("settings.data.backupSensitive")}</p>
      <p className="text-sm">
        <span className="font-medium">{t("settings.data.lastBackup")}: </span>
        {status.data?.available
          ? t("settings.data.lastBackupSummary", { fileName: status.data.backupFileName, schema: status.data.schemaVersion, result: status.data.verificationResult })
          : t("settings.data.lastBackupNone")}
      </p>
      <div className="flex flex-wrap gap-2">
        <Button type="button" onClick={runBackup} disabled={createBackup.isPending}>{t("settings.data.backup")}</Button>
        <Button type="button" variant="outline" onClick={runInspect} disabled={inspect.isPending}>{t("settings.data.restore")}</Button>
        <Button type="button" variant="outline" onClick={runExport} disabled={exportJSON.isPending}>{t(exportJSON.isPending ? "settings.data.exporting" : "settings.data.export")}</Button>
      </div>

      <AlertDialog open={restoreOpen || restarting} onOpenChange={setRestoreOpen}>
        <AlertDialogContent className="max-w-lg">
          <AlertDialogHeader>
            <AlertDialogTitle>{restarting ? t("settings.data.restoreRestart") : t("settings.data.restoreTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("settings.data.restoreDescription")}</AlertDialogDescription>
          </AlertDialogHeader>
          {preview && !restarting ? (
            <div className="grid gap-3 text-sm sm:grid-cols-2">
              <div>
                <p className="font-medium">{t("settings.data.restoreCurrent")}</p>
                <p>{t("settings.data.restoreHousehold", { name: preview.currentHousehold || "—", currency: preview.currentCurrency || "—" })}</p>
                <p>{t("settings.data.restoreCounts", { accounts: preview.currentAccounts ?? 0, holdings: preview.currentHoldings ?? 0, activities: preview.currentActivities ?? 0 })}</p>
              </div>
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
              <label className="flex items-center gap-2"><input type="checkbox" checked={restoreRouting} onChange={(event) => setRestoreRouting(event.target.checked)} /> {t("settings.data.restoreRouting")}</label>
              <label className="flex items-center gap-2"><input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} /> {t("settings.data.restoreAcknowledge")}</label>
              <Label htmlFor="restore-confirm">{t("settings.data.restoreTypeLabel")}</Label>
              <Input id="restore-confirm" value={confirmation} onChange={(event) => setConfirmation(event.target.value)} autoComplete="off" />
            </div>
          ) : null}
          <AlertDialogFooter>
            {!restarting ? <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel> : null}
            {!restarting ? (
              <Button type="button" onClick={runRestore} disabled={!restoreReady || confirmRestore.isPending}>{t("settings.data.restoreConfirm")}</Button>
            ) : null}
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

    </section>
  );
}
