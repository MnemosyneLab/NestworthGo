import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  backendBase,
  ConnectionError,
  normalizeBackend,
  saveBackend,
  takeConnectionHint,
} from "./connection";
import { setMobileSession } from "./api";

export function ConnectionFailure({ error }: { error: unknown }) {
  const { t } = useTranslation();
  return error instanceof ConnectionError ? (
    <p role="alert">{t(`mobile.transport.${error.code}`)}</p>
  ) : null;
}

export function ConnectionSettings({ onChanged }: { onChanged: () => void }) {
  const { t } = useTranslation();
  const [address, setAddress] = useState(
    () => takeConnectionHint() ?? backendBase(),
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const [tested, setTested] = useState(false);
  const active = useRef<AbortController | null>(null);
  useEffect(() => () => active.current?.abort(), []);
  const cancel = () => {
    active.current?.abort();
    active.current = null;
    setBusy(false);
  };
  async function test() {
    if (active.current) return;
    setTested(false);
    setError(null);
    const controller = new AbortController();
    active.current = controller;
    setBusy(true);
    let timeout = false;
    const timer = setTimeout(() => {
      timeout = true;
      controller.abort();
    }, 10000);
    try {
      const base = normalizeBackend(address);
      const response = await fetch(`${base}/mobile-api/v1/connection`, {
        method: "GET",
        credentials: "omit",
        cache: "no-store",
        redirect: "error",
        referrerPolicy: "no-referrer",
        headers: { "X-Nestworth-Mobile": "1" },
        signal: controller.signal,
      });
      let result;
      try {
        result = await response.json();
      } catch {
        throw new ConnectionError("invalid_response");
      }
      if (
        !response.ok ||
        result.service !== "nestworth-mobile" ||
        result.version !== 1 ||
        result.authentication !== (base ? "bearer" : "cookie")
      )
        throw new ConnectionError("invalid_response");
      if (active.current === controller) setTested(true);
    } catch (e) {
      if (active.current === controller)
        setError(
          controller.signal.aborted
            ? new ConnectionError(timeout ? "timeout" : "cancelled")
            : e instanceof ConnectionError
              ? e
              : new ConnectionError("transport"),
        );
    } finally {
      clearTimeout(timer);
      if (active.current === controller) {
        active.current = null;
        setBusy(false);
      }
    }
  }
  return (
    <section
      className="space-y-3 rounded-xl border border-border p-4"
      aria-label={t("mobile.connectionSettings")}
    >
      <h2 className="font-semibold">{t("mobile.connectionSettings")}</h2>
      <Label htmlFor="backend-address">{t("mobile.backendAddress")}</Label>
      <Input
        id="backend-address"
        value={address}
        inputMode="url"
        autoCapitalize="none"
        autoComplete="off"
        spellCheck={false}
        aria-describedby="backend-help"
        aria-invalid={!!error}
        disabled={busy}
        placeholder="https://nestworth.private.example"
        onChange={(e) => {
          setAddress(e.target.value);
          setError(null);
          setTested(false);
        }}
      />
      <p id="backend-help" className="text-sm text-muted-foreground">
        {t("mobile.backendHelp")}
      </p>
      <p className="text-sm break-all">
        {t("mobile.frontendOrigin", { origin: window.location.origin })}
      </p>
      <ConnectionFailure error={error} />
      {tested && <p role="status">{t("mobile.connectionTestPassed")}</p>}
      {busy && <p role="status">{t("common.pending")}</p>}
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          disabled={busy}
          onClick={() => void test()}
        >
          {t("mobile.testConnection")}
        </Button>
        {busy && (
          <Button type="button" variant="outline" onClick={cancel}>
            {t("common.cancel")}
          </Button>
        )}
        <Button
          type="button"
          disabled={busy}
          onClick={() => {
            try {
              const next = normalizeBackend(address);
              if (next !== backendBase()) {
                setMobileSession(null);
                saveBackend(next);
                onChanged();
              }
              setAddress(next);
              setError(null);
            } catch (e) {
              setError(e);
            }
          }}
        >
          {t("mobile.useConnection")}
        </Button>
      </div>
    </section>
  );
}
