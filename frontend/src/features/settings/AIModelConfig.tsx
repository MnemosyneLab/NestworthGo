import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import * as API from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
    <form
      className="max-w-xl space-y-3 border-t border-border pt-4"
      onSubmit={(event) => {
        event.preventDefault();
        void save();
      }}
    >
      <p className="text-sm">{t("aiChat.provider")}</p>
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
      {failed && <p role="alert">{t("aiChat.configError")}</p>}
    </form>
  );
}
