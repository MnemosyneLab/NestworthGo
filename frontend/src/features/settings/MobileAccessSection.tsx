import { useRef, useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { callService } from "@/lib/wails";
import { displayError } from "@/lib/display";
import * as MobileService from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/mobile/service";
import type { Config } from "../../../bindings/github.com/waltwang/nestworth-go/internal/mobileapi/models";

export function MobileAccessSection({ active = true }: { active?: boolean }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const status = useQuery({
    queryKey: ["mobile-access"],
    queryFn: () => callService(() => MobileService.Status()),
    enabled: active,
  });
  const [draft, setDraft] = useState<Config | null>(null);
  const [showToken, setShowToken] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const lock = useRef(false);
  const config = draft ?? status.data?.config;
  async function run(action: () => Promise<void>) {
    if (lock.current) return;
    lock.current = true;
    setBusy(true);
    setError(null);
    try {
      await action();
    } catch (e) {
      setError(displayError(e, t("settings.saveError")));
    } finally {
      await client.invalidateQueries({ queryKey: ["mobile-access"] });
      lock.current = false;
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
              setShowToken(false);
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
            <div className="space-y-2">
              <Label htmlFor="mobile-listen">{t("mobile.listen")}</Label>
              <Input
                id="mobile-listen"
                value={config.listen}
                required={config.enabled}
                aria-describedby="mobile-listen-help"
                placeholder="192.168.1.20:9443"
                onChange={(e) => set("listen", e.target.value.trim())}
              />
              <p id="mobile-listen-help" className="text-sm text-muted-foreground">
                {t("mobile.listenHelp")}
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="mobile-access-token">{t("mobile.token")}</Label>
              <Input
                id="mobile-access-token"
                type={showToken && active ? "text" : "password"}
                value={config.token}
                required={config.enabled}
                minLength={config.enabled ? 8 : undefined}
                maxLength={64}
                pattern={config.enabled ? "[A-Za-z0-9]{8,64}" : undefined}
                autoComplete="off"
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                aria-describedby="mobile-token-help"
                onChange={(e) => set("token", e.target.value)}
              />
              <div className="flex flex-wrap gap-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setShowToken(!showToken)}
                >
                  {t(showToken && active ? "mobile.hideToken" : "mobile.showToken")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => void run(async () => {
                    set("token", await callService(() => MobileService.GenerateToken()));
                    setShowToken(true);
                  })}
                >
                  {t("mobile.generateToken")}
                </Button>
              </div>
              <p id="mobile-token-help" className="text-sm text-muted-foreground">
                {t("mobile.tokenHelp")}
              </p>
            </div>
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
        <div className="space-y-2">
          <a
            className="break-all underline"
            href={status.data.url}
            target="_blank"
            rel="noreferrer"
          >
            {status.data.url}
          </a>
          <QRCodeSVG
            value={status.data.url}
            size={192}
            marginSize={4}
            role="img"
            title={t("mobile.connectionQR")}
          />
          <p className="text-sm">{t("mobile.connectionQRHelp")}</p>
        </div>
      )}
      <p className="text-sm text-muted-foreground">{t("mobile.connectionHelp")}</p>
    </section>
  );
}
