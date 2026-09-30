import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { SecretField } from "./SecretField";
import { useContinuousBackup, useConfigureBackup, useTestBackupConnection, useBackupNow, useCloudRecoveryPoints, useInspectCloudRestore, useConfirmCloudRestore } from "@/queries/continuousBackup";
import type { Update } from "../../../bindings/github.com/waltwang/nestworth-go/internal/infrastructure/continuousbackup/models";

export function ContinuousBackupSection() {
  const { t } = useTranslation();
  const status = useContinuousBackup();
  const save = useConfigureBackup();
  const test = useTestBackupConnection();
  const backup = useBackupNow();
  const points = useCloudRecoveryPoints();
  const inspect = useInspectCloudRestore();
  const restore = useConfirmCloudRestore();
  const [draft, setDraft] = useState<Partial<Update> | null>(null);
  const [access, setAccess] = useState("");
  const [secret, setSecret] = useState("");
  const [secretGeneration, setSecretGeneration] = useState(0);
  const [selection, setSelection] = useState("");
  const [open, setOpen] = useState(false);
  const [confirmation, setConfirmation] = useState("");
  const [acknowledged, setAcknowledged] = useState(false);
  const [restarting, setRestarting] = useState(false);
  const busy = save.isPending || test.isPending || backup.isPending || inspect.isPending || restore.isPending || restarting || points.isFetching;
  const current = status.data;
  if (status.isLoading) return <p>{t("ui.state.loadingPage")}</p>;
  if (!current) return <div role="alert"><p>{t("cloudBackup.unavailable")}</p><Button type="button" variant="outline" onClick={() => status.refetch()}>{t("common.retryAction")}</Button></div>;
  const enabled = draft?.enabled ?? current.enabled;
  const accountID = draft?.accountID ?? current.accountID;
  const bucket = draft?.bucket ?? current.bucket;
  const dirty = draft !== null || access !== "" || secret !== "";
  const update = (patch: Partial<Update>) => setDraft((old) => ({ ...old, ...patch }));
  const input = (removeCredentials = false): Update => ({ enabled: removeCredentials ? false : enabled, accountID, bucket, accessKeyID: removeCredentials ? "" : access, secretAccessKey: removeCredentials ? "" : secret, removeCredentials });
  const partialPair = (access.trim() === "") !== (secret.trim() === "");
  const persist = (removeCredentials = false) => { if (busy || (!removeCredentials && partialPair)) return; save.mutate(input(removeCredentials), { onSuccess: () => { setDraft(null); setAccess(""); setSecret(""); setSecretGeneration((value) => value + 1); test.reset(); setSelection(""); inspect.reset(); } }); };
  const point = points.data?.find((item) => `${item.streamID}:${item.txID}` === selection);
  const preview = inspect.data;
  const timestamp = (value: string) => value ? new Date(value).toLocaleString() : t("cloudBackup.never");
  const ready = Boolean(preview?.token) && acknowledged && confirmation.trim().toLowerCase() === "restore";
  const restoreState = restore.isPending || restarting ? "installing" : inspect.isPending ? "downloading" : open ? "preview" : inspect.isError || restore.isError ? "error" : "";
  return <section className="space-y-4" aria-labelledby="cloud-backup-title">
    <div><h2 id="cloud-backup-title" className="text-base font-semibold">{t("cloudBackup.title")}</h2><p className="text-sm text-muted-foreground">{t("cloudBackup.description")}</p></div>
    <p className="text-sm text-muted-foreground">{t("cloudBackup.sensitive")}</p>
    <form aria-label={t("cloudBackup.form")} onSubmit={(event) => { event.preventDefault(); persist(); }} className="space-y-4">
      <label className="flex items-center gap-2"><input type="checkbox" checked={enabled} onChange={(event) => update({ enabled: event.target.checked })} disabled={busy} />{t("cloudBackup.enable")}</label>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2"><Label htmlFor="backup-account">{t("cloudBackup.account")}</Label><Input id="backup-account" value={accountID} onChange={(event) => update({ accountID: event.target.value })} disabled={busy} autoComplete="off" /></div>
        <div className="space-y-2"><Label htmlFor="backup-bucket">{t("cloudBackup.bucket")}</Label><Input id="backup-bucket" value={bucket} onChange={(event) => update({ bucket: event.target.value })} disabled={busy} autoComplete="off" /></div>
        <SecretField key={`access-${secretGeneration}`} id="backup-access" label={t("cloudBackup.access")} hasValue={current.credentialsConfigured} value={access} onChange={setAccess} disabled={busy} onRemove={() => persist(true)} removeLabel={t("cloudBackup.removePair")} />
        <SecretField key={`secret-${secretGeneration}`} id="backup-secret" label={t("cloudBackup.secret")} hasValue={current.credentialsConfigured} value={secret} onChange={setSecret} disabled={busy} />
      </div>
      <p className="text-sm text-muted-foreground">{t("cloudBackup.pairHelp")}</p>
      <div className="flex flex-wrap gap-2">
        <Button type="submit" disabled={busy || !dirty || partialPair}>{t(save.isPending ? "common.pending" : "cloudBackup.save")}</Button>
        <Button type="button" variant="outline" onClick={() => { setDraft(null); setAccess(""); setSecret(""); setSecretGeneration((value) => value + 1); }} disabled={busy || !dirty}>{t("common.cancel")}</Button>
        <Button type="button" variant="outline" onClick={() => test.mutate()} disabled={busy || dirty || !current.credentialsConfigured}>{t("cloudBackup.test")}</Button>
        <Button type="button" variant="outline" onClick={() => backup.mutate()} disabled={busy || !current.enabled || dirty}>{t("cloudBackup.now")}</Button>
      </div>
    </form>
    <div role="status" aria-live="polite" className="space-y-1 text-sm">
      <p>{t("cloudBackup.status")}: {t(`cloudBackup.states.${current.state}`, { defaultValue: t("cloudBackup.unavailable") })}</p>
      <p>{t("cloudBackup.lastSuccess")}: {timestamp(current.lastSuccessfulBackup)}</p>
      <p>{t("cloudBackup.lastAttempt")}: {timestamp(current.lastAttempt)}</p>
      <p className="text-muted-foreground">{t("cloudBackup.successHelp")}</p>
      {test.isSuccess && <p>{t("cloudBackup.connectionPassed")}</p>}
      {save.isSuccess && <p>{t("cloudBackup.saved")}</p>}
    </div>
    {(save.isError || test.isError || backup.isError || inspect.isError || restore.isError || current.errorSummary) && <p role="alert" className="text-sm text-destructive">{t("cloudBackup.error")}</p>}
    <div className="space-y-3 border-t pt-4">
      <h3 className="font-medium">{t("cloudBackup.recovery")}</h3>
      <p className="text-sm text-muted-foreground">{t("cloudBackup.recoveryHelp")}</p>
      {restoreState && <p role="status" aria-live="polite">{t(`cloudBackup.restoreStates.${restoreState}`)}</p>}
      <Button type="button" variant="outline" onClick={() => { setSelection(""); void points.refetch(); }} disabled={busy || points.isFetching || !current.credentialsConfigured || dirty}>{t("cloudBackup.list")}</Button>
      {points.isError && <p role="alert">{t("cloudBackup.error")}</p>}
      {points.isSuccess && points.data?.length === 0 && <p>{t("cloudBackup.empty")}</p>}
      {(points.data?.length ?? 0) > 0 && <div className="space-y-2">
        <Label htmlFor="backup-recovery-point">{t("cloudBackup.point")}</Label>
        <NativeSelect id="backup-recovery-point" value={selection} onChange={(event) => setSelection(event.target.value)} disabled={busy}>
          <option value="">{t("cloudBackup.select")}</option>
          {points.data?.map((item) => <option key={`${item.streamID}:${item.txID}`} value={`${item.streamID}:${item.txID}`}>{timestamp(item.capturedAt)} · {item.streamID} · {item.txID}</option>)}
        </NativeSelect>
        <Button type="button" variant="outline" disabled={busy || !point} onClick={() => { if (point) inspect.mutate(point, { onSuccess: () => { setConfirmation(""); setAcknowledged(false); setOpen(true); } }); }}>{t("cloudBackup.preview")}</Button>
      </div>}
    </div>
    <AlertDialog open={open || restarting} onOpenChange={(value) => { if (!restore.isPending && !restarting) { setOpen(value); if (!value) { inspect.reset(); setConfirmation(""); setAcknowledged(false); } } }}>
      <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{t(restarting ? "settings.data.restoreRestart" : "settings.data.restoreTitle")}</AlertDialogTitle><AlertDialogDescription>{t("cloudBackup.restoreDescription")}</AlertDialogDescription></AlertDialogHeader>
        {preview && !restarting && <div className="space-y-3 text-sm">
          <div><p className="font-medium">{t("settings.data.restoreCurrent")}</p><p>{t("settings.data.restoreHousehold", { name: preview.currentHousehold || "—", currency: preview.currentCurrency || "—" })}</p><p>{t("settings.data.restoreCounts", { accounts: preview.currentAccounts, holdings: preview.currentHoldings, activities: preview.currentActivities })}</p></div>
          <div><p className="font-medium">{t("settings.data.restoreBackup")}</p><p>{t("settings.data.restoreHousehold", { name: preview.backupHousehold || "—", currency: preview.backupCurrency || "—" })}</p><p>{t("settings.data.restoreCounts", { accounts: preview.backupAccounts, holdings: preview.backupHoldings, activities: preview.backupActivities })}</p></div>
          <p>{t("cloudBackup.restorePaused")}</p>
          <label className="flex items-center gap-2"><input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} disabled={restore.isPending} />{t("settings.data.restoreAcknowledge")}</label>
          <Label htmlFor="cloud-restore-confirm">{t("settings.data.restoreTypeLabel")}</Label><Input id="cloud-restore-confirm" value={confirmation} onChange={(event) => setConfirmation(event.target.value)} disabled={restore.isPending} autoComplete="off" />
        </div>}
        <AlertDialogFooter>{!restarting && <><AlertDialogCancel disabled={restore.isPending}>{t("common.cancel")}</AlertDialogCancel><Button type="button" disabled={!ready || restore.isPending} onClick={() => { if (ready && preview && !busy) restore.mutate({ token: preview.token ?? "", confirmation, acknowledged, restoreChrome: false, restoreFormat: false, restoreRouting: false }, { onSuccess: (result) => setRestarting(result.restartRequired) }); }}>{t("settings.data.restoreConfirm")}</Button></>}</AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </section>;
}
