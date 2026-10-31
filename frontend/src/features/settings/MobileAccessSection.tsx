import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { callService } from "@/lib/wails";
import { displayError } from "@/lib/display";
import { formatTimestamp } from "@/lib/time";
import * as MobileService from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/mobile/service";
import type {
  Config,
  Permissions,
} from "../../../bindings/github.com/waltwang/nestworth-go/internal/mobileapi/models";

export function MobileAccessSection({ active = true }: { active?: boolean }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const status = useQuery({
    queryKey: ["mobile-access"],
    queryFn: () => callService(() => MobileService.Status()),
    enabled: active,
  });
  const [draft, setDraft] = useState<Config | null>(null);
  const [name, setName] = useState("");
  const [permissions, setPermissions] = useState<Permissions>({
    read: true,
    record: true,
    market_refresh: true,
    manual_quote: false,
  });
  const [token, setToken] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  if (!active && token !== null) setToken(null);
  const config = draft ?? status.data?.config;
  async function run(action: () => Promise<unknown>) {
    if (busy) return;
    setBusy(true);
    setToken(null);
    setError(null);
    try {
      await action();
      await client.invalidateQueries({ queryKey: ["mobile-access"] });
    } catch (e) {
      setError(displayError(e, t("settings.saveError")));
    } finally {
      await client.invalidateQueries({ queryKey: ["mobile-access"] });
      setBusy(false);
    }
  }
  const set = (key: keyof Config, value: string | boolean) => {
    if (config) setDraft({ ...config, [key]: value });
  };
  return (
    <section
      className="space-y-5 rounded-2xl border border-border bg-card p-5"
      aria-labelledby="mobile-access-title"
    >
      <h2 id="mobile-access-title" className="text-lg font-semibold">
        {t("mobile.access")}
      </h2>
      <p className="text-sm text-muted-foreground">{t("mobile.setupHelp")}</p>
      <p className="text-sm">{t("mobile.connectionHelp")}</p>
      {(error || status.error) && (
        <p role="alert">{error || t("mobile.requestFailed")}</p>
      )}
      {config && (
        <form
          className="space-y-4"
          onSubmit={(event) => {
            event.preventDefault();
            void run(async () => {
              await callService(() => MobileService.Configure(config));
              setDraft(null);
            });
          }}
        >
          <fieldset disabled={busy} className="space-y-4">
            <Label className="flex gap-2">
              <input
                type="checkbox"
                checked={config.enabled}
                onChange={(e) => set("enabled", e.target.checked)}
              />
              {t("mobile.enabled")}
            </Label>
            <Label htmlFor="mobile-origin">{t("mobile.origin")}</Label>
            <Input
              id="mobile-origin"
              value={config.origin}
              placeholder="https://nestworth.private.example"
              onChange={(e) => set("origin", e.target.value)}
            />
            <Label htmlFor="mobile-listen">{t("mobile.listen")}</Label>
            <Input
              id="mobile-listen"
              value={config.listen}
              placeholder="127.0.0.1:9443"
              onChange={(e) => set("listen", e.target.value)}
            />
            <Label htmlFor="mobile-mode">{t("mobile.mode")}</Label>
            <NativeSelect
              id="mobile-mode"
              value={config.mode}
              onChange={(e) => set("mode", e.target.value)}
            >
              <option value="">{t("history.selectEmpty")}</option>
              <option value="tls">{t("mobile.tls")}</option>
              <option value="loopback_proxy">{t("mobile.proxy")}</option>
            </NativeSelect>
            {config.mode === "tls" && (
              <>
                <Label htmlFor="mobile-cert">{t("mobile.certificate")}</Label>
                <Input
                  id="mobile-cert"
                  value={config.certificate}
                  onChange={(e) => set("certificate", e.target.value)}
                />
                <Label htmlFor="mobile-key">{t("mobile.keyFile")}</Label>
                <Input
                  id="mobile-key"
                  value={config.key}
                  onChange={(e) => set("key", e.target.value)}
                />
              </>
            )}
            <Button>{t("mobile.saveAccess")}</Button>
          </fieldset>
        </form>
      )}
      {status.data?.error && (
        <p role="alert">
          {t(`mobile.connectionError.${status.data.error}`, {
            defaultValue: t("mobile.requestFailed"),
          })}
        </p>
      )}
      <p role="status">
        {status.data?.running ? t("mobile.running") : t("mobile.stopped")}
      </p>
      {status.data?.running && (
        <>
          <a
            className="break-all underline"
            href={status.data.config.origin}
            target="_blank"
            rel="noreferrer"
          >
            {status.data.config.origin}
          </a>
          <form
            className="space-y-3"
            onSubmit={(event) => {
              event.preventDefault();
              void run(async () => {
                const p = await callService(() =>
                  MobileService.Pair(name, permissions),
                );
                setToken(p.token);
              });
            }}
          >
            <Label htmlFor="mobile-device">{t("mobile.deviceName")}</Label>
            <Input
              id="mobile-device"
              value={name}
              maxLength={100}
              required
              onChange={(e) => setName(e.target.value)}
            />
            {(
              ["read", "record", "market_refresh", "manual_quote"] as const
            ).map((p) => (
              <Label key={p} className="flex gap-2">
                <input
                  type="checkbox"
                  disabled={p === "read" || busy}
                  checked={permissions[p]}
                  onChange={(e) =>
                    setPermissions({ ...permissions, [p]: e.target.checked })
                  }
                />
                {t(`mobile.${p}`)}
              </Label>
            ))}
            <Button disabled={busy}>{t("mobile.pair")}</Button>
          </form>
          <p className="text-sm">{t("mobile.pairHelp")}</p>
          {token && (
            <Input
              aria-label={t("mobile.token")}
              readOnly
              autoComplete="off"
              value={token}
              onFocus={(e) => e.target.select()}
            />
          )}
          <Button
            variant="outline"
            disabled={busy}
            onClick={() =>
              void run(() => callService(() => MobileService.Revoke("")))
            }
          >
            {t("mobile.revokeAll")}
          </Button>
          {(status.data.devices ?? []).map((d) => (
            <div
              key={d.id}
              className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border p-3"
            >
              <div className="space-y-1">
                <p>{d.name}</p>
                <p className="text-xs text-muted-foreground">
                  {(
                    [
                      "read",
                      "record",
                      "market_refresh",
                      "manual_quote",
                    ] as const
                  )
                    .filter((p) => d.permissions?.[p])
                    .map((p) => t(`mobile.${p}`))
                    .join(" · ")}
                </p>
                <p className="text-xs">
                  {t("mobile.lastUsed")} {formatTimestamp(d.lastUsed)} ·{" "}
                  {t("mobile.expiresAt")} {formatTimestamp(d.expiresAt)}
                </p>
              </div>
              <Button
                variant="outline"
                disabled={busy}
                onClick={() =>
                  void run(() => callService(() => MobileService.Revoke(d.id)))
                }
              >
                {t("mobile.revoke")}
              </Button>
            </div>
          ))}
        </>
      )}
    </section>
  );
}
