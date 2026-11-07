import { SecurityOperationRegion } from "@/lib/businessCapture";
import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import * as API from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { callService } from "@/lib/wails";
import { configuredModels } from "@/features/assistant/modelSelection";

export function AIModelConfig({ status, unavailable, onBusyChange }: {
  status: Awaited<ReturnType<typeof API.Status>>;
  unavailable: boolean;
  onBusyChange?: (busy: boolean) => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [selected, setSelected] = useState(() => configuredModels(status.config));
  const [custom, setCustom] = useState("");
  const [enabled, setEnabled] = useState(status.config?.enabled ?? false);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const catalog = status.modelCatalog ?? [];
  const customModels = selected.filter(id => !catalog.some(option => option.id === id));
  const validCustom = /^[A-Za-z0-9][A-Za-z0-9._:-]{0,95}$/.test(custom) && !custom.startsWith("sk-") && custom !== "unconfigured";
  function toggle(id: string) { setSelected(current => current.includes(id) ? current.filter(item => item !== id) : [...current, id]); }
  async function save(disable = false) {
    if (busy) return;
    setBusy(true); setFailed(false); onBusyChange?.(true);
    try {
      await callService(() => API.Configure(status.epoch, status.revision, disable ? { ...status.config, enabled: false } : {
        provider: "openai-responses",
        model: selected.includes(status.config.model) ? status.config.model : selected[0],
        enabledModels: selected,
        enabled,
      }));
    } catch { setFailed(true); }
    finally {
      setBusy(false); onBusyChange?.(false);
      await client.invalidateQueries({ queryKey: ["aiCredentialStatus"] });
    }
  }
  return <SecurityOperationRegion className="contents"><form className="max-w-xl space-y-3 border-t border-border pt-4" onSubmit={event => { event.preventDefault(); void save(); }}>
    <p className="text-sm">{t("aiChat.provider")}</p>
    <fieldset disabled={busy || unavailable} className="space-y-2">
      <legend className="mb-2 text-sm font-medium">{t("aiModels.enabled")}</legend>
      <div className="grid grid-cols-1 gap-x-4 sm:grid-cols-2">
        {catalog.map(option => <Label key={option.id} className="flex min-h-11 items-center gap-2">
          <input type="checkbox" checked={selected.includes(option.id)} onChange={() => toggle(option.id)} />{option.id}
        </Label>)}
      </div>
      <p className="text-xs text-muted-foreground">{t("aiModels.catalogHelp")}</p>
      {customModels.length > 0 && <div className="space-y-1"><p className="text-xs text-muted-foreground">{t("aiModels.customSaved")}</p>
        {customModels.map(id => <Label key={id} className="flex min-h-11 items-center gap-2"><input type="checkbox" checked onChange={() => toggle(id)} />{id}</Label>)}
      </div>}
      <Label htmlFor="ai-model">{t("aiChat.model")}</Label>
      <div className="flex gap-2"><Input id="ai-model" className="min-w-0 flex-1" value={custom} onChange={event => setCustom(event.target.value)} maxLength={96} autoComplete="off" />
        <Button type="button" variant="outline" disabled={!validCustom || selected.includes(custom) || selected.length >= 32} onClick={() => {setSelected([...selected, custom]); setCustom("");}}>{t("aiModels.add")}</Button>
      </div>
      <p className="text-xs text-muted-foreground">{t("aiModels.customHelp")}</p>
    </fieldset>
    <Label className="flex min-h-11 items-center gap-2"><input type="checkbox" checked={enabled} disabled={busy || unavailable || status.credentials !== "present"} onChange={event => setEnabled(event.target.checked)} />{t("aiChat.enable")}</Label>
    <p className="text-xs text-muted-foreground">{t("aiChat.consent")}</p>
    <p className="text-xs text-muted-foreground">{t("aiChat.titlePolicy")}</p>
    <div className="flex flex-wrap gap-2"><Button type="submit" disabled={busy || unavailable || selected.length === 0 || selected.length > 32}>{t("aiChat.saveConfig")}</Button>
      {status.config?.enabled && <Button type="button" variant="outline" disabled={busy || unavailable} onClick={() => void save(true)}>{t("aiChat.disable")}</Button>}
    </div>
    {failed && <p role="alert">{t("aiChat.configError")}</p>}
  </form></SecurityOperationRegion>;
}
