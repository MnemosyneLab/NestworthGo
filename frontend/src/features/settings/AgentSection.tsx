import { useState } from "react";
import { Check, ChevronDown, Copy, History, Plug, Power, ShieldCheck, Terminal } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/select";
import { Label } from "@/components/ui/label";
import { AgentService, agentKey, useAgentOperations, useAgentStatus } from "@/queries/agent";
import { callService } from "@/lib/wails";
import { displayError } from "@/lib/display";

export function AgentSection() {
  const { t } = useTranslation();
  const status = useAgentStatus();
  const operations = useAgentOperations();
  const client = useQueryClient();
  const [mode, setMode] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [connection, setConnection] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const selected = mode ?? status.data?.mode ?? "read_only";
  async function run(action: () => Promise<unknown>) {
    setBusy(true); setError(null); setConnection(null); setCopied(false);
    try { await action(); await client.invalidateQueries({ queryKey: agentKey }); }
    catch (cause) { setError(displayError(cause, t("settings.saveError"))); }
    finally { setBusy(false); }
  }
  const running = status.data?.running === true;
  const permissionChanged = running && selected !== status.data?.mode;
  return <section id="settings-agent" aria-labelledby="settings-agent-title" className="scroll-mt-6 space-y-5 border-t border-border pt-6">
    <header className="space-y-2">
      <h2 id="settings-agent-title" className="text-base font-semibold">{t("agent.title")}</h2>
      <p className="max-w-2xl text-sm leading-6 text-muted-foreground">{t("agent.description")}</p>
    </header>
    {status.isLoading ? <p className="text-sm text-muted-foreground">{t("agent.loading")}</p> : status.isError ? <p role="alert">{displayError(status.error, t("settings.loadError"))}</p> : <>
      <div className="overflow-hidden rounded-2xl border border-border bg-card shadow-sm">
        <div className="flex flex-col gap-4 p-5 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            <div className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary"><Plug className="size-5" aria-hidden="true" /></div>
            <div className="space-y-1">
              <h3 className="text-sm font-semibold">{t("agent.localConnection")}</h3>
              <p role="status" className="flex items-center gap-2 text-xs text-muted-foreground">
                <span aria-hidden="true" className={`size-1.5 rounded-full ${running ? "bg-emerald-500" : "bg-muted-foreground/50"}`} />
                {t(running ? "agent.running" : "agent.stopped")}
              </p>
            </div>
          </div>
          {running
            ? <Button type="button" disabled={busy} onClick={() => void run(async () => { const value = await callService(() => AgentService.Connection()); setConnection(value.json); })}><Terminal className="size-4" aria-hidden="true" />{t("agent.showConnection")}</Button>
            : <Button type="button" disabled={busy} onClick={() => void run(() => callService(() => AgentService.Enable(selected)))}><Power className="size-4" aria-hidden="true" />{t("agent.enable")}</Button>}
        </div>
        {running && <div className="space-y-3 border-t border-border bg-muted/25 px-5 py-4">
          <div className="flex min-w-0 flex-col gap-1.5 sm:flex-row sm:items-center sm:gap-5">
            <span className="shrink-0 text-xs text-muted-foreground">{t("agent.endpoint")}</span>
            <code className="break-all text-xs leading-5 text-foreground">{status.data?.endpoint}</code>
          </div>
          <details className="group text-xs">
            <summary className="flex w-fit cursor-pointer list-none items-center gap-1.5 rounded text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring [&::-webkit-details-marker]:hidden"><ChevronDown className="size-3.5 transition-transform group-open:rotate-180" aria-hidden="true" />{t("agent.connectionDetails")}</summary>
            <dl className="mt-3 space-y-1.5 pl-5"><dt className="text-muted-foreground">{t("agent.instance")}</dt><dd className="break-all font-mono">{status.data?.instanceId}</dd></dl>
          </details>
        </div>}
      </div>
      {status.data?.error && <p role="alert" className="text-sm text-destructive">{t(`agent.errors.${status.data.error}`, { defaultValue: t("agent.errors.server_stopped") })}</p>}
      <div className="space-y-4 rounded-2xl border border-border bg-card p-5">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex items-center gap-2"><ShieldCheck className="size-4 text-muted-foreground" aria-hidden="true" /><Label htmlFor="agent-mode" className="text-sm font-semibold">{t("agent.permission")}</Label></div>
          <NativeSelect id="agent-mode" className="w-full sm:w-auto sm:min-w-48" value={selected} disabled={busy} aria-describedby="agent-permission-help" onChange={(event) => setMode(event.target.value)}>
            <option value="read_only">{t("agent.readOnly")}</option>
            <option value="directory_write">{t("agent.directoryWrite")}</option>
            <option value="ledger_write">{t("agent.ledgerWrite")}</option>
          </NativeSelect>
        </div>
        <p id="agent-permission-help" className="max-w-2xl text-sm leading-6 text-muted-foreground">{t(selected === "ledger_write" ? "agent.ledgerHelp" : selected === "directory_write" ? "agent.writeHelp" : "agent.readHelp")}</p>
        {permissionChanged && <div className="flex flex-col gap-3 border-t border-border pt-4 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-xs text-muted-foreground">{t("agent.permissionChangeHelp")}</p>
          <Button type="button" disabled={busy} onClick={() => void run(() => callService(() => AgentService.Enable(selected)))}>{t("agent.apply")}</Button>
        </div>}
      </div>
    </>}
    {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
    {connection && running && <div className="overflow-hidden rounded-2xl border border-border bg-card">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border px-5 py-3">
        <h3 className="flex items-center gap-2 text-sm font-semibold"><Terminal className="size-4 text-muted-foreground" aria-hidden="true" />{t("agent.connectionLabel")}</h3>
        <div className="flex items-center gap-1">
          <Button type="button" variant="outline" size="sm" onClick={() => { void navigator.clipboard.writeText(connection).then(() => setCopied(true)).catch(() => setError(t("agent.copyFailed"))); }}>{copied ? <Check className="size-3.5" aria-hidden="true" /> : <Copy className="size-3.5" aria-hidden="true" />}{t(copied ? "agent.copied" : "agent.copy")}</Button>
          <Button type="button" variant="ghost" size="sm" onClick={() => setConnection(null)}>{t("agent.hide")}</Button>
        </div>
      </div>
      <textarea readOnly spellCheck={false} wrap="off" aria-label={t("agent.connectionLabel")} value={connection} rows={10} className="block w-full resize-y border-0 bg-muted/35 p-5 font-mono text-xs leading-6 outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring" />
      <p className="border-t border-border px-5 py-3 text-xs leading-5 text-muted-foreground">{t("agent.secretHelp")}</p>
    </div>}
    <div className="space-y-3 pt-1">
      <h3 className="flex items-center gap-2 text-sm font-semibold"><History className="size-4 text-muted-foreground" aria-hidden="true" />{t("agent.recent")}</h3>
      {operations.isError && <p role="alert">{displayError(operations.error, t("settings.loadError"))}</p>}
      {operations.data?.length === 0 && <p className="rounded-xl border border-dashed border-border px-4 py-5 text-center text-sm text-muted-foreground">{t("agent.noOperations")}</p>}
      <ul className="space-y-2">{operations.data?.map((op) => <li key={op.id} className="rounded-xl border border-border bg-card px-4 py-3 text-sm">
        <div className="flex flex-wrap items-center justify-between gap-2"><span className="font-mono text-xs">{op.tool}</span><span className={`rounded-full px-2 py-0.5 text-xs ${op.status === "succeeded" ? "bg-primary/10 text-primary" : "bg-muted text-muted-foreground"}`}>{t(`agent.operationStatus.${op.status}`)}</span></div>
        <p className="mt-2 break-all text-xs text-muted-foreground">{op.createdAt} · {op.id}</p>
      </li>)}</ul>
    </div>
    {running && <div className="flex flex-col gap-3 border-t border-border pt-4 sm:flex-row sm:items-center sm:justify-between">
      <p className="text-xs leading-5 text-muted-foreground">{t("agent.keepOpen")}</p>
      <Button type="button" variant="ghost" className="self-start text-muted-foreground hover:text-destructive sm:self-auto" disabled={busy} onClick={() => void run(() => callService(() => AgentService.Disable()))}><Power className="size-4" aria-hidden="true" />{t("agent.disable")}</Button>
    </div>}
  </section>;
}
