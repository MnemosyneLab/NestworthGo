import { useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import * as AIService from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service";
import { Button } from "@/components/ui/button";
import { callService } from "@/lib/wails";
import { AIModelConfig } from "./AIModelConfig";
import { SecretField } from "./SecretField";

// New keys stay in local component state, never a query/mutation cache. The
// parent unmounts this editor on section exit so typed secrets are discarded.
export function AICredentialsSection({ onBusyChange }: { onBusyChange?: (busy: boolean) => void }) {
  const { t } = useTranslation();
  const status = useQuery({ networkMode: "always", queryKey: ["aiCredentialStatus"], queryFn: () => callService(() => AIService.Status()) });
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const [generation, setGeneration] = useState(0);
  const pending = useRef(false);
  // Credential editing needs host identity, not the separate model-ready flag.
  const unavailable = status.isError || !status.data?.epoch || (status.data.credentials !== "missing" && status.data.credentials !== "present");
  async function save(remove: boolean) {
    if (pending.current || unavailable || !status.data) return;
    pending.current = true; setBusy(true); onBusyChange?.(true); setFailed(false);
    const value = key;
    setKey(""); setGeneration((old) => old + 1);
    try {
      if (remove) await callService(() => AIService.DeleteCredential(status.data.epoch));
      else await callService(() => AIService.SaveCredential(status.data.epoch, value));
      await status.refetch();
    } catch { setFailed(true); await status.refetch(); }
    finally { pending.current = false; setBusy(false); onBusyChange?.(false); }
  }
  return <section className="mb-5 space-y-4 rounded-2xl border border-border bg-card p-5" aria-labelledby="ai-credentials-title">
    <h2 id="ai-credentials-title" className="font-semibold">{t("aiCredentials.title")}</h2>
    <p className="text-sm text-muted-foreground">{t("aiCredentials.help")}</p>
    <p className="text-sm text-muted-foreground">{t("aiCredentials.backup")}</p>
    <form onSubmit={(event) => { event.preventDefault(); void save(false); }} className="flex max-w-xl flex-col gap-3">
      <SecretField key={generation} id="ai-api-key" label={t("aiCredentials.key")} hasValue={status.data?.credentials === "present"} value={key} onChange={setKey} disabled={busy || unavailable} onRemove={() => void save(true)} />
      <p role="status">{t(unavailable ? "aiCredentials.unavailable" : status.data?.credentials === "present" ? "secretField.configured" : "aiCredentials.missing")}</p>
      <Button type="submit" disabled={busy || unavailable || !key.trim()}>{t("aiCredentials.save")}</Button>
      {(failed || status.isError) && <p role="alert">{t("aiCredentials.error")}</p>}
    </form>
    {status.data?.config && <AIModelConfig key={status.data.revision} status={status.data} unavailable={unavailable || busy} onBusyChange={onBusyChange} />}
  </section>;
}
