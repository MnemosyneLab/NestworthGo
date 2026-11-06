import { SecurityOperationRegion } from "@/lib/businessCapture";
import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import * as API from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/select";
import { Label } from "@/components/ui/label";
import { callService } from "@/lib/wails";

export function AIModelConfig({
  status,
  unavailable,
  onBusyChange,
}: {
  status: Awaited<ReturnType<typeof API.Status>>;
  unavailable: boolean;
  onBusyChange?: (busy: boolean) => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [model, setModel] = useState(
    status.config?.model === "unconfigured" ? "" : (status.config?.model ?? ""),
  );
  const [models, setModels] = useState<string[]>([]);
  const [listing, setListing] = useState(false);
  const [listFailed, setListFailed] = useState(false);
  async function refreshModels() {
    setListing(true); setListFailed(false);
    try { setModels((await callService(() => API.Models(status.epoch, status.revision))) ?? []); }
    catch {setListFailed(true);}
    finally {setListing(false);}
  }
  const [enabled, setEnabled] = useState(status.config?.enabled ?? false);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  async function save(disable = false) {
    if (busy) return;
    setBusy(true);
    setFailed(false);
    onBusyChange?.(true);
    try {
      await callService(() =>
        API.Configure(status.epoch, status.revision, {
          provider: "openai-responses",
          model: disable ? status.config.model : model,
          enabled: disable ? false : enabled,
        }),
      );
    } catch {
      setFailed(true);
    } finally {
      setBusy(false);
      onBusyChange?.(false);
      await client.invalidateQueries({ queryKey: ["aiCredentialStatus"] });
    }
  }
  return (
    <SecurityOperationRegion className="contents"><form
      className="max-w-xl space-y-3 border-t border-border pt-4"
      onSubmit={(event) => {
        event.preventDefault();
        void save();
      }}
    >
      <p className="text-sm">{t("aiChat.provider")}</p>
      <Label htmlFor="ai-model-list">{t("aiUsage.availableModels")}</Label>
      <div className="flex gap-2">
        <NativeSelect id="ai-model-list" className="min-w-0 flex-1" value={model} disabled={busy || unavailable || listing} onChange={event => setModel(event.target.value)}>
          {!model && <option value="">{t("aiUsage.chooseModel")}</option>}
          {model && !models.includes(model) && <option value={model}>{model}</option>}
          {models.map(id => <option key={id} value={id}>{id}</option>)}
        </NativeSelect>
        <Button type="button" variant="outline" disabled={busy || unavailable || listing || status.credentials !== "present"} onClick={() => void refreshModels()}>{t(listing ? "aiUsage.loading" : "aiUsage.refreshModels")}</Button>
      </div>
      <p className="text-xs text-muted-foreground">{t("aiUsage.modelHelp")}</p>
      {listFailed && <p role="alert" className="text-xs">{t("aiUsage.modelsError")}</p>}
      <Label htmlFor="ai-model">{t("aiChat.model")}</Label>
      <Input
        className="min-h-11 xl:min-h-9"
        id="ai-model"
        value={model}
        onChange={(event) => setModel(event.target.value)}
        disabled={busy || unavailable}
        autoComplete="off"
        maxLength={96}
      />
      <Label className="flex min-h-11 items-center gap-2">
        <input
          type="checkbox"
          checked={enabled}
          disabled={busy || unavailable || status.credentials !== "present"}
          onChange={(event) => setEnabled(event.target.checked)}
        />
        {t("aiChat.enable")}
      </Label>
      <p className="text-xs text-muted-foreground">{t("aiChat.consent")}</p>
      <p className="text-xs text-muted-foreground">{t("aiChat.titlePolicy")}</p>
      <div className="flex flex-wrap gap-2">
        <Button
          className="min-h-11 whitespace-normal xl:min-h-9"
          type="submit"
          disabled={
            busy ||
            unavailable ||
            !model ||
            !/^[A-Za-z0-9][A-Za-z0-9._:-]{0,95}$/.test(model)
          }
        >
          {t("aiChat.saveConfig")}
        </Button>
        {status.config?.enabled && (
          <Button
            className="min-h-11 whitespace-normal xl:min-h-9"
            type="button"
            variant="outline"
            disabled={busy || unavailable}
            onClick={() => void save(true)}
          >
            {t("aiChat.disable")}
          </Button>
        )}
      </div>
      {failed && <p role="alert">{t("aiChat.configError")}</p>}
    </form></SecurityOperationRegion>
  );
}
