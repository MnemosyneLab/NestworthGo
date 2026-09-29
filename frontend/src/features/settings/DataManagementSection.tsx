import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { displayError } from "@/lib/display";
import { useLastBackupStatus, useCreateBackup, useExportJSON } from "@/queries/data";
import { RestoreBackupButton } from "@/features/backup/RestoreBackupButton";

export function DataManagementSection() {
  const { t } = useTranslation();
  const status = useLastBackupStatus();
  const createBackup = useCreateBackup();
  const exportJSON = useExportJSON();
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
    </section>
  );
}
