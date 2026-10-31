import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { AlertCircle, CheckCircle2, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  backendBase,
  ConnectionError,
  normalizeBackend,
  takeConnectionHint,
} from "./connection";

export function ConnectionFailure({ error }: { error: unknown }) {
  const { t } = useTranslation();
  return error instanceof ConnectionError ? (
    <p role="alert" className="mobile-notice" data-tone="danger">
      <AlertCircle aria-hidden />
      <span>{t(`mobile.transport.${error.code}`)}</span>
    </p>
  ) : null;
}

export function ConnectionSettings({
  onChange,
  disabled = false,
}: {
  onChange: (backend: string) => boolean;
  disabled?: boolean;
}) {
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
    if (disabled || active.current) return;
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
      className="mobile-card mobile-connection"
      aria-label={t("mobile.connectionSettings")}
    >
      <h2 className="mobile-step-title">{t("mobile.connectionSettings")}</h2>
      <div className="mobile-field">
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
          disabled={disabled || busy}
          placeholder="https://nestworth.private.example"
          onChange={(e) => {
            setAddress(e.target.value);
            setError(null);
            setTested(false);
          }}
        />
      </div>
      <p id="backend-help" className="mobile-hint">
        {t("mobile.backendHelp")}
      </p>
      <p className="mobile-connection-origin">
        {t("mobile.frontendOrigin", { origin: window.location.origin })}
      </p>
      <ConnectionFailure error={error} />
      {tested && (
        <p role="status" className="mobile-notice" data-tone="success">
          <CheckCircle2 aria-hidden />
          <span>{t("mobile.connectionTestPassed")}</span>
        </p>
      )}
      {busy && (
        <p role="status" className="mobile-connection-pending">
          <Loader2 aria-hidden className="animate-spin" />
          <span>{t("common.pending")}</span>
        </p>
      )}
      <div className="mobile-form-actions">
        <Button
          type="button"
          variant="outline"
          disabled={disabled || busy}
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
          disabled={disabled || busy}
          onClick={() => {
            if (disabled || busy) return;
            try {
              const next = normalizeBackend(address);
              if (next !== backendBase() && !onChange(next)) return;
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
