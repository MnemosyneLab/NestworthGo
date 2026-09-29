import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { displayError } from "@/lib/display";
import { useLastBackupStatus, useCreateBackup, useExportJSON, useRebuildDerivedData } from "@/queries/data";
import { RestoreBackupButton } from "@/features/backup/RestoreBackupButton";

export function DataManagementSection() {
  const { t } = useTranslation();
  const status = useLastBackupStatus();
  const createBackup = useCreateBackup();
  const exportJSON = useExportJSON();
  const rebuild = useRebuildDerivedData();
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
        <RestoreBackupButton />
        <Button type="button" variant="outline" onClick={runExport} disabled={exportJSON.isPending}>{t(exportJSON.isPending ? "settings.data.exporting" : "settings.data.export")}</Button>
      </div>
      <div className="mt-3 flex flex-col gap-3 border-t pt-5">
        <div>
          <h3 className="text-sm font-semibold">{t("settings.data.rebuildTitle")}</h3>
          <p className="text-sm text-muted-foreground">{t("settings.data.rebuildDescription")}</p>
          <p className="mt-1 text-sm text-muted-foreground">{t("settings.data.rebuildIncompleteHint")}</p>
        </div>
        <div>
          <Button type="button" variant="outline" onClick={() => rebuild.mutate()} disabled={rebuild.isPending}>
            {t(rebuild.isPending ? "settings.data.rebuilding" : "settings.data.rebuild")}
          </Button>
        </div>
        {rebuild.isPending && <p role="status" className="text-sm text-muted-foreground">{t("settings.data.rebuildProgress")}</p>}
        {rebuild.isError && <p role="alert" className="text-sm text-destructive">{displayError(rebuild.error, t("settings.data.rebuildError"))}</p>}
        {rebuild.isSuccess && (
          <div role="status" className="text-sm">
            <p className="font-medium">{t("settings.data.rebuildComplete")}</p>
            <p>{t("settings.data.rebuildRange", { from: rebuild.data.from, to: rebuild.data.to })}</p>
            <p>{t("settings.data.rebuildCounts", { snapshotDays: rebuild.data.snapshotDays, incompleteDays: rebuild.data.incompleteDays })}</p>
          </div>
        )}
      </div>
    </section>
  );
}
