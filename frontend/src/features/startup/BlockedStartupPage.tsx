import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import type { StartupDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/app/models";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { LoadingState } from "@/components/layout/PageState";
import { parseWailsError, translateWailsError, type WireError } from "@/lib/wails";
import { useInspectBackup, useConfirmRestore } from "@/queries/data";
import { displayError } from "@/lib/display";
import { toast } from "sonner";

type BlockedStartup = Omit<StartupDTO, "foundSchemaVersion" | "supportedSchemaVersion"> & {
  foundSchemaVersion?: number | null;
  supportedSchemaVersion?: number | null;
};

export function StartupLoadingPage({ label }: { label: string }) {
  return (
    <main className="flex min-h-[100dvh] items-center justify-center bg-background p-6">
      <div className="w-full max-w-lg">
        <div className="mb-5 flex items-center gap-3">
          <div className="flex size-10 items-center justify-center rounded-xl bg-primary text-lg font-semibold text-primary-foreground" aria-hidden="true">
            N
          </div>
          <div>
            <p className="text-sm font-semibold text-foreground">Nestworth</p>
            <p className="text-xs text-muted-foreground">{label}</p>
          </div>
        </div>
        <LoadingState label={label} />
      </div>
    </main>
  );
}

function blockedCopy(code: string | undefined, t: TFunction) {
  switch (code) {
    case "database_upgrade_required":
      return { title: t("startup.upgradeTitle"), description: t("startup.upgradeDescription"), guidance: t("startup.upgradeGuidance") };
    case "database_from_newer_version":
      return { title: t("startup.newerTitle"), description: t("startup.newerDescription"), guidance: t("startup.newerGuidance") };
    case "database_integrity_failed":
      return { title: t("startup.integrityTitle"), description: t("startup.integrityDescription"), guidance: t("startup.integrityGuidance") };
    case "database_unavailable":
      return { title: t("startup.unavailableTitle"), description: t("startup.unavailableDescription"), guidance: t("startup.unavailableGuidance") };
    default:
      return { title: t("startup.blockedTitle"), description: t("startup.blockedDescription"), guidance: t("startup.readOnly") };
  }
}

/**
 * BlockedStartupPage is shown when the local database could not be opened,
 * so business writes stay disabled. Retry
 * is "quit and relaunch" rather than an in-process reopen — Wails
 * services are constructed once at process start. Restore stays available
 * in every blocked state.
 */
export function BlockedStartupPage({ startup, failure }: { startup?: BlockedStartup | null; failure?: unknown }) {
  const { t } = useTranslation();
  const inspect = useInspectBackup();
  const confirmRestore = useConfirmRestore();
  const [preview, setPreview] = useState<{ token?: string; backupHousehold?: string; backupCurrency?: string; backupAccounts?: number } | null>(null);
  const [acknowledged, setAcknowledged] = useState(false);
  const [confirmation, setConfirmation] = useState("");
  const [restarting, setRestarting] = useState(false);
  const wireError: WireError = startup?.code
    ? { code: startup.code, field: startup.field, message: startup.message ?? "" }
    : failure && typeof failure === "object" && "wireError" in failure
      ? ((failure as Error & { wireError: WireError }).wireError ?? parseWailsError(failure))
      : parseWailsError(failure);
  const diagnosticCode = startup?.code ?? wireError.code;
  const copy = blockedCopy(diagnosticCode, t);
  const message = startup?.code || failure ? translateWailsError(wireError) : t("startup.genericFailure");
  const showSchema =
    startup?.foundSchemaVersion != null && startup?.supportedSchemaVersion != null && (diagnosticCode === "database_upgrade_required" || diagnosticCode === "database_from_newer_version");

  return (
    <main className="flex min-h-[100dvh] w-screen items-center justify-center bg-background p-6">
      <Card className="w-full max-w-lg" role="alert">
        <CardHeader>
          <CardTitle>{copy.title}</CardTitle>
          <CardDescription>{copy.description}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <p className="text-sm text-muted-foreground">{copy.guidance}</p>
          {showSchema ? (
            <p className="text-sm text-muted-foreground">
              {t("startup.schemaVersion", { found: startup?.foundSchemaVersion, supported: startup?.supportedSchemaVersion })}
            </p>
          ) : null}
          <p className="rounded-md border border-warning/40 bg-warning/10 p-3 text-sm text-warning-foreground">{message}</p>
          <div className="flex flex-wrap items-center gap-2">
            <Button type="button" onClick={() => window.location.reload()}>
              {t("startup.retry")}
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() =>
                inspect.mutate(undefined, {
                  onSuccess: (result) => {
                    if (!result.cancelled) {
                      setPreview(result);
                    }
                  },
                  onError: (error) => toast.error(displayError(error, t("startup.genericFailure"))),
                })
              }
              disabled={inspect.isPending || restarting}
            >
              {t("startup.restore")}
            </Button>
            <details className="text-sm text-muted-foreground">
              <summary className="cursor-pointer rounded-md px-2 py-1 underline underline-offset-4 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50">
                {t("startup.diagnosis")}
              </summary>
              <p className="mt-2 max-w-sm">{t("startup.diagnosisDescription")}</p>
              <code className="mt-1 block rounded bg-muted px-2 py-1 text-xs text-foreground">{diagnosticCode}</code>
            </details>
          </div>
          {preview && !restarting ? (
            <div className="flex flex-col gap-2 text-sm">
              <p>{t("settings.data.restoreDescription")}</p>
              <p>{t("settings.data.restoreHousehold", { name: preview.backupHousehold || "—", currency: preview.backupCurrency || "—" })}</p>
              <label className="flex items-center gap-2">
                <input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} />
                {t("settings.data.restoreAcknowledge")}
              </label>
              <Label htmlFor="blocked-restore-confirm">{t("settings.data.restoreTypeLabel")}</Label>
              <Input id="blocked-restore-confirm" value={confirmation} onChange={(event) => setConfirmation(event.target.value)} autoComplete="off" />
              <Button
                type="button"
                disabled={!acknowledged || confirmation.trim().toLowerCase() !== "restore" || confirmRestore.isPending}
                onClick={() =>
                  confirmRestore.mutate(
                    { token: preview.token ?? "", confirmation, acknowledged, restoreChrome: false, restoreFormat: false, restoreRouting: false },
                    {
                      onSuccess: (result) => {
                        if (result.restartRequired) {
                          setRestarting(true);
                        }
                      },
                      onError: (error) => toast.error(displayError(error, t("startup.genericFailure"))),
                    },
                  )
                }
              >
                {t("settings.data.restoreConfirm")}
              </Button>
            </div>
          ) : null}
          {restarting ? <p className="text-sm">{t("settings.data.restoreRestart")}</p> : null}
        </CardContent>
      </Card>
    </main>
  );
}
