import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import {
  ArrowDownLeft,
  ArrowLeftRight,
  ArrowUpRight,
  CandlestickChart,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  CreditCard,
  HandCoins,
  Inbox,
  Landmark,
  LayoutGrid,
  Plus,
  ReceiptText,
  RefreshCw,
  RotateCw,
  Scale,
  Search,
  Settings2,
  SlidersHorizontal,
  ShieldCheck,
  Wallet,
  WalletCards,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { DatePickerControl } from "@/components/ui/date-picker-control";
import { TimePicker } from "@/components/ui/time-picker";
import { ChangeText } from "@/components/ui/change-text";
import { Card } from "@/components/ui/card";
import { LoadingState } from "@/components/layout/PageState";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
} from "@/components/ui/alert-dialog";
import { formatAmount, formatSignedMoney, formatQuotePrice } from "@/lib/money";
import { formatCivilDate, formatQuoteAsOf, formatTimestamp } from "@/lib/time";
import { setDisplayPreferences } from "@/lib/displayPreferences";
import {
  api,
  MobileError,
  onSessionExpired,
  pendingOperation,
  savePending,
  setMobileSession,
  type Account,
  type Activity,
  type Envelope,
  type Issue,
  type Job,
  type MarketRequest,
  type Metric,
  type Money,
  type Pending,
  type RecordInput,
  type Review,
  type Session,
} from "./api";

import { ConnectionSettings } from "./ConnectionSettings";
import {
  backendBase,
  ConnectionError,
  mobilePreferences,
  saveMobilePreferences,
  saveBackend,
} from "./connection";

type Bootstrap = {
  householdId: string;
  name: string;
  currency: string;
  today: string;
  time: string;
  historyStarted: boolean;
  capabilities: Session["device"]["permissions"];
};
type Page<T> = { items: T[]; next: string; healthy?: boolean };
type Holding = {
  id: string;
  accountId: string;
  instrumentId: string;
  name: string;
  symbol: string;
  quantity: string;
  value: Money | null;
  cost: Money | null;
  gain: Money | null;
  missingReason: string;
};
type Component = {
  instrumentName: string;
  instrumentSymbol?: string;
  holdingId?: string;
  nativeAmount: string;
  nativeCurrency: string;
  available: boolean;
  priceEvidence?: {
    source: string;
    quotedAt: string;
    effectiveDate: string | null;
    fetchedAt: string | null;
    freshness: string;
  };
};
type AccountDetail = {
  account: {
    name: string;
    accountType?: string;
    balanceSheetRole?: string;
    trackingMode?: string;
    defaultCurrency?: string;
  };
  components: Component[];
  displayValue?: { value: Money | null } | null;
  baseValue?: Money | null;
  complete?: boolean;
};
type HoldingDetailResponse = {
  holding: {
    HoldingID?: string;
    InstrumentName: string;
    InstrumentSymbol?: string;
    CurrentValue?: Money | null;
    CurrentValueBase?: Money | null;
    Quantity: string;
    InstrumentID: string;
    TotalCost: Money | null;
    UnrealizedGain: { Amount: string; Currency: string } | null;
    RealizedGain: { Amount: string; Currency: string } | null;
  };
  valuation: AccountDetail;
  quote?: { unitPrice: string; currency: string } | null;
};
type DetailResponse = AccountDetail | HoldingDetailResponse | Activity;
const currencies = [
  "AUD",
  "CHF",
  "CNY",
  "EUR",
  "GBP",
  "HKD",
  "JPY",
  "KRW",
  "SGD",
  "TWD",
  "USD",
];

function MoneyText({ value }: { value: Money | null | undefined }) {
  const { t } = useTranslation();
  return (
    <span className="mobile-money">
      {value
        ? formatAmount(value.amount, value.currency)
        : t("mobile.unavailable")}
    </span>
  );
}
function ExactMoney({ value }: { value: Money | null | undefined }) {
  const { t } = useTranslation();
  return (
    <span className="mobile-money">
      {value ? `${value.amount} ${value.currency}` : t("mobile.unavailable")}
    </span>
  );
}
function GainMoneyText({ value }: { value: Money | null | undefined }) {
  const { t } = useTranslation();
  if (!value) return <span>{t("mobile.unavailable")}</span>;
  return (
    <ChangeText
      direction={
        value.amount.startsWith("-")
          ? "down"
          : /^0(?:\.0*)?$/.test(value.amount)
            ? "flat"
            : "up"
      }
    >
      {formatSignedMoney(value.amount, value.currency)}
    </ChangeText>
  );
}
function ErrorMessage({ error }: { error: unknown }) {
  const { t } = useTranslation();
  if (!error) return null;
  const message =
    error instanceof ConnectionError
      ? t(`mobile.transport.${error.code}`)
      : error instanceof MobileError
        ? t(`errorCode:${error.code}`, {
            defaultValue: t("mobile.requestFailed"),
          })
        : t("mobile.connectionUnavailable");
  return (
    <p
      role="alert"
      className="rounded-xl border border-destructive p-3 text-sm"
    >
      {message}
      {error instanceof MobileError && error.field
        ? ` · ${t(`error.field.${error.field}`, { defaultValue: "" })}`
        : ""}
    </p>
  );
}
function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-3">
      <h2 className="text-lg font-semibold">{title}</h2>
      {children}
    </section>
  );
}
function Field({ name, children }: { name: string; children: ReactNode }) {
  return (
    <div className="space-y-1">
      <Label>{name}</Label>
      {children}
    </div>
  );
}

export function MobileApp() {
  const { t, i18n } = useTranslation();
  const client = useQueryClient();
  const activeDevice = useRef<string | null>(null);
  const [session, setSession] = useState<Session | null | undefined>();
  const [connectionVersion, setConnectionVersion] = useState(0);
  const [showConnection, setShowConnection] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);
  const reconnectLock = useRef(false);
  const [theme, setTheme] = useState(() => mobilePreferences().theme);
  const [sessionError, setSessionError] = useState<unknown>();
  const [tab, setTab] = useState("overview");
  const [detail, setDetail] = useState<{
    kind: "accounts" | "holdings" | "activities";
    id: string;
  } | null>(null);
  const [record, setRecord] = useState<string | null>(null);
  const [initialKind, setInitialKind] = useState("expense");
  const [notice, setNotice] = useState("");
  const [quote, setQuote] = useState<Issue | null>(null);
  const [market, setMarket] = useState<MarketRequest | null>(null);
  const [pending, setPending] = useState<Pending | null>(() =>
    pendingOperation(),
  );
  const [recoveryPersistent, setRecoveryPersistent] = useState(true);
  const [receipt, setReceipt] = useState<Activity | null>(null);
  const [offline, setOffline] = useState(!navigator.onLine);
  const connectionLocked =
    !!pending || record !== null || quote !== null || market !== null;
  const acceptSession = useCallback(
    (value: Session | null) => {
      const changed = activeDevice.current !== (value?.device.id ?? null);
      activeDevice.current = value?.device.id ?? null;
      setMobileSession(value);
      setSession(value);
      if (value) setSessionError(null);
      if (!value || changed) {
        client.clear();
        setDetail(null);
        setRecord(null);
        setQuote(null);
        setMarket(null);
        setReceipt(null);
      }
    },
    [client],
  );
  useEffect(() => {
    let mounted = true;
    const controller = new AbortController();
    void api<Session>("/session", "GET", undefined, {
      signal: controller.signal,
    })
      .then((value) => {
        if (mounted) {
          setMobileSession(value);
          activeDevice.current = value.device.id;
          setSession(value);
        }
      })
      .catch((error) => {
        if (mounted) {
          setSession(null);
          if (!(error instanceof MobileError && error.status === 401))
            setSessionError(error);
        }
      });
    return () => {
      mounted = false;
      controller.abort();
    };
  }, []);
  useEffect(() => onSessionExpired(() => acceptSession(null)), [acceptSession]);
  useEffect(() => {
    const update = () => setOffline(!navigator.onLine);
    window.addEventListener("online", update);
    window.addEventListener("offline", update);
    return () => {
      window.removeEventListener("online", update);
      window.removeEventListener("offline", update);
    };
  }, []);
  const bootstrap = useQuery({
    queryKey: ["mobile", "bootstrap"],
    queryFn: () => api<Envelope<Bootstrap>>("/bootstrap"),
    enabled: !!session,
  });
  useEffect(() => {
    if (bootstrap.data)
      setDisplayPreferences({ timezone: bootstrap.data.ledgerTimezone });
  }, [bootstrap.data]);
  const updatePending = (value: Pending | null) => {
    if (!session?.device.id || activeDevice.current !== session.device.id)
      return;
    setRecoveryPersistent(savePending(value));
    setPending(value);
  };
  const success = (activity?: Activity, keepOpen = false) => {
    if (!session?.device.id || activeDevice.current !== session.device.id)
      return false;
    updatePending(null);
    if (!keepOpen) setRecord(null);
    setQuote(null);
    setReceipt(activity ?? null);
    setNotice("saved");
    void client.invalidateQueries({ queryKey: ["mobile"] });
    return true;
  };
  const changeConnection = (backend: string) => {
    // The page owns endpoint and session changes; a previously opened panel
    // cannot discard a draft, immutable plan or unresolved receipt reference.
    if (connectionLocked || reconnectLock.current) return false;
    saveBackend(backend);
    acceptSession(null);
    setPending(pendingOperation());
    setSessionError(null);
    setShowConnection(false);
    setConnectionVersion((v) => v + 1);
    return true;
  };
  const reconnect = async () => {
    if (reconnectLock.current) return;
    reconnectLock.current = true;
    setReconnecting(true);
    setSessionError(null);
    try {
      const current = await api<Session>("/session");
      // Retain mounted drafts and immutable reviews across transient reconnects.
      if (current.device.id !== activeDevice.current) acceptSession(current);
      await client.invalidateQueries({ queryKey: ["mobile"] });
    } catch (error) {
      setSessionError(error);
      if (error instanceof MobileError && error.status === 401)
        acceptSession(null);
    } finally {
      reconnectLock.current = false;
      setReconnecting(false);
    }
  };
  useEffect(() => {
    const scheme = window.matchMedia("(prefers-color-scheme: dark)");
    const apply = () =>
      document.documentElement.classList.toggle(
        "dark",
        theme === "dark" || (theme === "system" && scheme.matches),
      );
    apply();
    scheme.addEventListener("change", apply);
    return () => scheme.removeEventListener("change", apply);
  }, [theme]);
  if (session === undefined)
    return (
      <main className="mobile-shell">
        <LoadingState label={t("common.loading")} />
      </main>
    );
  if (!session)
    return (
      <Login
        key={connectionVersion}
        error={sessionError}
        onLogin={acceptSession}
        onConnectionChanged={changeConnection}
        connectionDisabled={connectionLocked}
      />
    );
  const b = bootstrap.data?.data;
  const hiddenNav = record !== null || quote !== null || market !== null;
  const connectionState = offline
    ? "offline"
    : reconnecting
      ? "reconnecting"
      : sessionError || bootstrap.error
        ? "attention"
        : "connected";
  const address = backendBase() || window.location.origin;
  const goTab = (value: string) => {
    setDetail(null);
    setTab(value);
    document.scrollingElement?.scrollTo?.({ top: 0 });
  };
  return (
    <main className="mobile-shell" data-nav={hiddenNav ? "hidden" : "shown"}>
      <header className="mobile-appbar">
        <div className="min-w-0 flex-1">
          <h1 className="mobile-household">{b?.name || "Nestworth"}</h1>
          <p
            className="mobile-status"
            data-state={connectionState}
            title={t("mobile.connectedTo", { address })}
          >
            <span className="mobile-status-dot" aria-hidden />
            <span className="truncate">
              {t(`mobile.connectionState.${connectionState}`)}
              <span className="text-muted-foreground">
                {" · "}
                {address.replace(/^https:\/\//, "")}
              </span>
            </span>
          </p>
        </div>
        <Button
          variant="outline"
          size="icon"
          className="mobile-icon-button"
          aria-label={t("mobile.reconnect")}
          title={t("mobile.reconnect")}
          disabled={reconnecting || !!pending}
          onClick={() => void reconnect()}
        >
          <RefreshCw
            aria-hidden
            className={reconnecting ? "motion-safe:animate-spin" : undefined}
          />
        </Button>
        <Button
          variant={showSettings ? "soft" : "outline"}
          size="icon"
          className="mobile-icon-button"
          aria-label={t("mobile.settings")}
          title={t("mobile.settings")}
          aria-expanded={showSettings}
          aria-controls="mobile-settings"
          onClick={() => setShowSettings((v) => !v)}
        >
          <Settings2 aria-hidden />
        </Button>
      </header>
      {showSettings && (
        <section
          id="mobile-settings"
          aria-label={t("mobile.settings")}
          className="mobile-panel mobile-enter space-y-3"
        >
          <div className="grid grid-cols-2 gap-2">
            <Field name={t("mobile.language")}>
              <NativeSelect
                aria-label={t("mobile.language")}
                value={i18n.language}
                onChange={(e) => {
                  saveMobilePreferences({ language: e.target.value, theme });
                  void i18n.changeLanguage(e.target.value);
                }}
              >
                <option value="en">English</option>
                <option value="zh-CN">简体中文</option>
                <option value="zh-TW">繁體中文</option>
              </NativeSelect>
            </Field>
            <Field name={t("mobile.theme")}>
              <NativeSelect
                aria-label={t("mobile.theme")}
                value={theme}
                onChange={(e) => {
                  const value = e.target.value as typeof theme;
                  setTheme(value);
                  saveMobilePreferences({
                    language: i18n.language,
                    theme: value,
                  });
                }}
              >
                {["system", "light", "dark"].map((value) => (
                  <option key={value} value={value}>
                    {t(`mobile.themeChoice.${value}`)}
                  </option>
                ))}
              </NativeSelect>
            </Field>
          </div>
          <dl className="mobile-facts">
            <div>
              <dt>{t("mobile.server")}</dt>
              <dd className="break-all">{address}</dd>
            </div>
            {bootstrap.data && (
              <div>
                <dt>{t("mobile.timezone")}</dt>
                <dd>{bootstrap.data.ledgerTimezone}</dd>
              </div>
            )}
          </dl>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              className="flex-1"
              disabled={connectionLocked || reconnecting}
              aria-expanded={showConnection}
              onClick={() => setShowConnection((v) => !v)}
            >
              {t("mobile.connectionSettings")}
            </Button>
            <Button
              variant="outline"
              className="flex-1 text-destructive"
              disabled={
                !!pending ||
                record !== null ||
                quote !== null ||
                market !== null ||
                reconnecting
              }
              onClick={() => {
                if (reconnectLock.current) return;
                reconnectLock.current = true;
                setReconnecting(true);
                setSessionError(null);
                void api("/session", "DELETE")
                  .then(() => acceptSession(null))
                  .catch((error) => {
                    setSessionError(error);
                    if (error instanceof MobileError && error.status === 401)
                      acceptSession(null);
                  })
                  .finally(() => {
                    reconnectLock.current = false;
                    setReconnecting(false);
                  });
              }}
            >
              {t("mobile.logout")}
            </Button>
          </div>
        </section>
      )}
      {showConnection && !reconnecting && (
        <ConnectionSettings
          disabled={connectionLocked}
          onChange={changeConnection}
        />
      )}
      {pending && !recoveryPersistent && (
        <p role="alert">{t("mobile.recoveryMemoryOnly")}</p>
      )}
      <ErrorMessage error={sessionError} />
      {offline && <p role="status">{t("mobile.connectionUnavailable")}</p>}
      <ErrorMessage error={bootstrap.error} />
      {!b ? (
        <LoadingState label={t("common.loading")} />
      ) : (
        <>
          {pending && record === null && quote === null && (
            <Recovery
              pending={pending}
              householdId={b.householdId}
              onResolved={(activity) => success(activity)}
              onForget={() => updatePending(null)}
            />
          )}
          {notice && (
            <div className="mobile-notice mobile-enter" data-tone="success">
              <CheckCircle2 aria-hidden className="size-5 shrink-0" />
              <p role="status" className="min-w-0 flex-1">
                {t(`mobile.${notice}`)}
              </p>
              <Button
                variant="ghost"
                size="icon"
                className="mobile-icon-button -my-2 -mr-2"
                aria-label={t("mobile.dismiss")}
                onClick={() => {
                  setNotice("");
                  setReceipt(null);
                }}
              >
                <X aria-hidden />
              </Button>
            </div>
          )}
          {receipt && (
            <Card className="mobile-receipt p-4">
              <ActivityView activity={receipt} />
            </Card>
          )}
          {record !== null ? (
            <RecordForm
              bootstrap={b}
              timezone={bootstrap.data!.ledgerTimezone}
              accountId={record}
              initialKind={initialKind}
              offline={offline}
              onCancel={() => setRecord(null)}
              onDispatch={updatePending}
              onSuccess={success}
            />
          ) : quote ? (
            <QuoteForm
              issue={quote}
              bootstrap={b}
              offline={offline}
              onCancel={() => setQuote(null)}
              onDispatch={updatePending}
              onSuccess={(pending) => {
                if (!success()) return;
                if (pending) setNotice("derivedPending");
              }}
            />
          ) : market ? (
            <MarketReview
              key={JSON.stringify(market)}
              request={market}
              onCancel={() => setMarket(null)}
              onStarted={() => {
                setMarket(null);
                void client.invalidateQueries({ queryKey: ["mobile", "job"] });
              }}
            />
          ) : detail ? (
            <>
              <Button
                variant="ghost"
                className="mobile-back"
                onClick={() => setDetail(null)}
              >
                <ChevronLeft aria-hidden />
                {t("mobile.back")}
              </Button>
              <Detail
                kind={detail.kind}
                id={detail.id}
                onHolding={(id) => setDetail({ kind: "holdings", id })}
                onRecord={() => {
                  setInitialKind("expense");
                  setRecord(detail.id);
                }}
                onMarket={(request) => setMarket(request)}
                canWrite={
                  !pending &&
                  !offline &&
                  b.capabilities.record &&
                  b.historyStarted
                }
                canRefresh={b.capabilities.market_refresh && !offline}
              />
            </>
          ) : (
            <>
              {b.capabilities.market_refresh && (
                <JobPanel onRetry={setMarket} disabled={offline || !!pending} />
              )}
              {tab === "overview" && (
                <Overview
                  onMarket={() => setMarket({ mode: "latest", scope: "stale" })}
                  canRefresh={b.capabilities.market_refresh && !offline}
                  onActivity={(id) => setDetail({ kind: "activities", id })}
                />
              )}
              {tab === "accounts" && (
                <Accounts
                  onOpen={(id) => setDetail({ kind: "accounts", id })}
                />
              )}
              {tab === "activities" && (
                <Activities
                  onOpen={(id) => setDetail({ kind: "activities", id })}
                  today={b.today}
                />
              )}
              {tab === "health" && (
                <Health
                  onQuote={setQuote}
                  onRecord={(id) => {
                    setInitialKind("balance");
                    setRecord(id);
                  }}
                  onMarket={setMarket}
                  capabilities={b.capabilities}
                  disabled={offline || !!pending}
                />
              )}
              <Button
                className="mobile-reload"
                variant="ghost"
                onClick={() =>
                  void client.invalidateQueries({ queryKey: ["mobile"] })
                }
              >
                <RotateCw aria-hidden />
                {t("mobile.reloadLocal")}
              </Button>
            </>
          )}
          {!hiddenNav && (
            <nav
              aria-label={t("mobile.mainNavigation")}
              className="mobile-tabbar"
            >
              {(
                [
                  ["overview", t("mobile.overview"), LayoutGrid],
                  ["accounts", t("mobile.accounts"), WalletCards],
                  null,
                  ["activities", t("mobile.ledger"), ReceiptText],
                  ["health", t("mobile.health"), ShieldCheck],
                ] as const
              ).map((item) =>
                item === null ? (
                  <button
                    key="record"
                    type="button"
                    className="mobile-fab"
                    aria-label={t("mobile.quickRecord")}
                    title={t("mobile.quickRecord")}
                    disabled={
                      !b.capabilities.record ||
                      !b.historyStarted ||
                      !!pending ||
                      offline
                    }
                    onClick={() => {
                      setInitialKind("expense");
                      setRecord("");
                    }}
                  >
                    <Plus aria-hidden />
                  </button>
                ) : (
                  <button
                    key={item[0]}
                    type="button"
                    className="mobile-tab"
                    aria-current={
                      tab === item[0] && !detail ? "page" : undefined
                    }
                    data-active={tab === item[0] || undefined}
                    onClick={() => goTab(item[0])}
                  >
                    {(() => {
                      const Icon = item[2];
                      return <Icon aria-hidden />;
                    })()}
                    <span>{item[1]}</span>
                  </button>
                ),
              )}
            </nav>
          )}
          {!b.historyStarted && (
            <p className="mobile-notice" data-tone="warning">
              {t("mobile.historyRequired")}
            </p>
          )}
        </>
      )}
    </main>
  );
}

function Login({
  onLogin,
  error: initialError,
  onConnectionChanged,
  connectionDisabled,
}: {
  onConnectionChanged: (backend: string) => boolean;
  connectionDisabled: boolean;
  onLogin: (s: Session) => void;
  error?: unknown;
}) {
  const { t } = useTranslation();
  const [token, setToken] = useState("");
  const [error, setError] = useState(initialError);
  const [busy, setBusy] = useState(false);
  const lock = useRef(false);
  const controller = useRef<AbortController | null>(null);
  useEffect(() => () => controller.current?.abort(), []);
  return (
    <main className="mobile-shell flex flex-col justify-center gap-5">
      <h1 className="text-3xl font-bold">Nestworth</h1>
      <ConnectionSettings
        disabled={connectionDisabled}
        onChange={onConnectionChanged}
      />
      <p>{t("mobile.loginHelp")}</p>
      <form
        className="space-y-4"
        onSubmit={(event) => {
          event.preventDefault();
          if (lock.current) return;
          lock.current = true;
          setBusy(true);
          setError(null);
          const request = new AbortController();
          controller.current = request;
          const secret = token;
          setToken("");
          void api<Session>(
            "/session",
            "POST",
            { token: secret },
            { signal: request.signal },
          )
            .then(onLogin)
            .catch(setError)
            .finally(() => {
              lock.current = false;
              setBusy(false);
            });
        }}
      >
        <Label htmlFor="mobile-token">{t("mobile.token")}</Label>
        <Input
          id="mobile-token"
          type="password"
          autoComplete="off"
          autoCapitalize="none"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          disabled={busy}
          required
        />
        <ErrorMessage error={error} />
        <Button className="w-full" disabled={busy}>
          {busy ? t("common.pending") : t("mobile.login")}
        </Button>
        {busy && (
          <Button
            type="button"
            variant="outline"
            onClick={() => controller.current?.abort()}
          >
            {t("common.cancel")}
          </Button>
        )}
      </form>
      <p className="text-sm text-muted-foreground">
        {t("mobile.connectionHelp")}
      </p>
    </main>
  );
}

function MetricItem({
  label,
  metric,
  className,
}: {
  label: string;
  metric: Metric;
  className?: string;
}) {
  const { t } = useTranslation();
  return (
    <div className={className}>
      <dt>{label}</dt>
      <dd>
        {metric.total ? (
          <MoneyText value={metric.total} />
        ) : (
          <span aria-label={t("mobile.unavailable")}>—</span>
        )}
        {!metric.total && metric.knownSubtotal && (
          <small className="mobile-subtotal">
            {t("mobile.knownSubtotal")}{" "}
            <MoneyText value={metric.knownSubtotal} />
          </small>
        )}
      </dd>
    </div>
  );
}
function Segmented<T extends string>({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: T;
  options: { value: T; label: string }[];
  onChange: (value: T) => void;
}) {
  return (
    <div className="mobile-segmented" role="group" aria-label={label}>
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          aria-pressed={o.value === value}
          onClick={() => onChange(o.value)}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}
// Geometry only: plotted from server totals; displayed figures stay exact strings.
function Sparkline({ points }: { points: { date: string; valuation: Metric }[] }) {
  const values = points.map((p) =>
    p.valuation.total ? Number(p.valuation.total.amount) : NaN,
  );
  const known = values.filter(Number.isFinite);
  if (known.length < 2) return null;
  const min = Math.min(...known);
  const span = Math.max(...known) - min;
  const w = 320;
  const h = 72;
  const step = w / Math.max(values.length - 1, 1);
  let d = "";
  let pen = false;
  values.forEach((v, i) => {
    if (!Number.isFinite(v)) {
      pen = false;
      return;
    }
    const y = span ? 4 + (h - 8) * (1 - (v - min) / span) : h / 2;
    d += `${pen ? "L" : "M"}${(i * step).toFixed(1)},${y.toFixed(1)}`;
    pen = true;
  });
  return (
    <div className="mobile-sparkline" aria-hidden="true">
      <svg viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="none" focusable="false">
        <path d={d} vectorEffect="non-scaling-stroke" />
      </svg>
      <p className="num">
        <span>{formatCivilDate(points[0].date)}</span>
        <span>{formatCivilDate(points[points.length - 1].date)}</span>
      </p>
    </div>
  );
}
function EmptyState({
  icon,
  children,
}: {
  icon?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="mobile-empty">
      {icon ?? <Inbox aria-hidden="true" />}
      <p>{children}</p>
    </div>
  );
}
function RowsSkeleton({ rows = 3 }: { rows?: number }) {
  const { t } = useTranslation();
  return (
    <div className="mobile-skeleton" role="status" aria-label={t("common.loading")}>
      {Array.from({ length: rows }, (_, i) => (
        <span key={i} />
      ))}
    </div>
  );
}
function Overview({
  onMarket,
  canRefresh,
  onActivity,
}: {
  onMarket: () => void;
  canRefresh: boolean;
  onActivity: (id: string) => void;
}) {
  const { t, i18n } = useTranslation();
  const [range, setRange] = useState<"30d" | "1y">("30d");
  const query = useQuery({
    queryKey: ["mobile", "overview", range],
    placeholderData: (previous) => previous,
    queryFn: () =>
      api<
        Envelope<{
          valuation: { netWorth: Metric; assets: Metric; liabilities: Metric };
          complete: boolean;
          trend: { date: string; valuation: Metric }[];
          recent: Activity[];
          currency: string;
          distribution: {
            key: string;
            label: string;
            amount: string;
            shareBps?: number;
          }[];
        }>
      >(`/overview?range=${range}`),
  });
  if (!query.data)
    return (
      <>
        <ErrorMessage error={query.error} />
        {!query.error && <LoadingState label={t("common.loading")} />}
      </>
    );
  const o = query.data.data;
  return (
    <div className="mobile-stack">
      <ErrorMessage error={query.error} />
      <section className="mobile-hero" aria-label={t("mobile.netWorth")}>
        <dl>
          <MetricItem
            className="mobile-hero-total"
            label={t("mobile.netWorth")}
            metric={o.valuation.netWorth}
          />
        </dl>
        <p className="mobile-meta">
          <span
            className="mobile-badge"
            data-tone={o.complete ? "success" : "warning"}
          >
            {o.complete ? t("mobile.complete") : t("mobile.partial")}
          </span>
          <span className="num">
            {formatTimestamp(query.data.asOf, query.data.ledgerTimezone)}
          </span>
        </p>
        <dl className="mobile-split">
          <MetricItem label={t("mobile.assets")} metric={o.valuation.assets} />
          <MetricItem
            label={t("mobile.liabilities")}
            metric={o.valuation.liabilities}
          />
        </dl>
      </section>
      <section className="mobile-card" aria-labelledby="mobile-trend">
        <div className="mobile-card-head">
          <h2 id="mobile-trend">{t("mobile.trend")}</h2>
          <Segmented
            label={t("mobile.trend")}
            value={range}
            onChange={setRange}
            options={[
              { value: "30d", label: t("mobile.month") },
              { value: "1y", label: t("mobile.year") },
            ]}
          />
        </div>
        <div aria-busy={query.isFetching}>
          <Sparkline points={o.trend} />
          {o.trend.length ? (
            <details className="mobile-disclosure">
              <summary>{t("mobile.showValues")}</summary>
              <dl className="mobile-values">
                {o.trend.map((p) => (
                  <div key={p.date}>
                    <dt>{formatCivilDate(p.date)}</dt>
                    <dd>
                      <MoneyText value={p.valuation.total} />
                    </dd>
                  </div>
                ))}
              </dl>
            </details>
          ) : (
            <EmptyState>{t("mobile.empty")}</EmptyState>
          )}
        </div>
      </section>
      <Button
        variant="outline"
        className="mobile-wide"
        disabled={!canRefresh}
        onClick={onMarket}
      >
        <RefreshCw aria-hidden="true" />
        {t("mobile.updateMarket")}
      </Button>
      {o.distribution.length > 0 && (
        <section className="mobile-card" aria-labelledby="mobile-distribution">
          <h2 id="mobile-distribution">{t("mobile.distribution")}</h2>
          {o.distribution.some((d) => d.shareBps !== undefined) && (
            <div className="mobile-meter" aria-hidden="true">
              {o.distribution.map((d, i) => (
                <span
                  key={d.key}
                  data-series={i % 6}
                  style={{ flexGrow: Math.max(d.shareBps ?? 0, 0) }}
                />
              ))}
            </div>
          )}
          <ul className="mobile-legend">
            {o.distribution.map((d, i) => (
              <li key={d.key}>
                <span className="mobile-swatch" data-series={i % 6} />
                <span className="mobile-legend-label">
                  {t(`enum.${d.key}`, { defaultValue: d.label })}
                </span>
                {d.shareBps !== undefined && (
                  <span className="mobile-legend-share num">
                    {formatShare(d.shareBps, i18n.language)}
                  </span>
                )}
                <MoneyText
                  value={{ amount: d.amount, currency: o.currency }}
                />
              </li>
            ))}
          </ul>
        </section>
      )}
      <section className="mobile-card" aria-labelledby="mobile-recent">
        <h2 id="mobile-recent">{t("mobile.recent")}</h2>
        <ActivityList items={o.recent} onOpen={onActivity} />
      </section>
    </div>
  );
}
function formatShare(bps: number, language: string) {
  return `${(bps / 100).toLocaleString(language, {
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
  })}%`;
}

function usePage<T>(endpoint: string, filters = "") {
  const key = endpoint + "?" + filters;
  const [page, setPage] = useState({ key, cursor: "" });
  const cursor = page.key === key ? page.cursor : "";
  const query = useQuery({
    queryKey: ["mobile", endpoint, filters, cursor],
    queryFn: () =>
      api<Envelope<Page<T>>>(
        `${endpoint}?${filters}&cursor=${encodeURIComponent(cursor)}`,
      ),
  });
  return {
    ...query,
    paged: cursor !== "",
    next: () => setPage({ key, cursor: query.data?.data.next ?? "" }),
    first: () => setPage({ key, cursor: "" }),
  };
}
function Pager({
  next,
  onNext,
  onFirst,
  paged,
}: {
  next?: string;
  onNext: () => void;
  onFirst: () => void;
  paged: boolean;
}) {
  const { t } = useTranslation();
  if (!next && !paged) return null;
  return (
    <div className="mobile-pager">
      <Button variant="ghost" size="sm" disabled={!paged} onClick={onFirst}>
        {t("mobile.firstPage")}
      </Button>
      <Button variant="outline" size="sm" disabled={!next} onClick={onNext}>
        {t("mobile.nextPage")}
        <ChevronRight aria-hidden="true" />
      </Button>
    </div>
  );
}
const accountTypes = [
  "bank_account",
  "cash_on_hand",
  "brokerage",
  "investment_account",
  "credit_card",
  "loan",
  "digital_wallet",
  "crypto_exchange",
  "receivable",
  "other",
];
function AccountIcon({ type }: { type: string }) {
  const Icon =
    type === "bank_account"
      ? Landmark
      : type === "cash_on_hand" || type === "digital_wallet"
        ? Wallet
        : type === "brokerage" ||
            type === "investment_account" ||
            type === "crypto_exchange"
          ? CandlestickChart
          : type === "credit_card"
            ? CreditCard
            : type === "loan"
              ? HandCoins
              : WalletCards;
  return (
    <span className="mobile-row-icon" data-kind={type}>
      <Icon aria-hidden="true" />
    </span>
  );
}
function Accounts({ onOpen }: { onOpen: (id: string) => void }) {
  const { t } = useTranslation();
  const [search, setSearch] = useState("");
  const [role, setRole] = useState("");
  const [type, setType] = useState("");
  const query = usePage<Account>(
    "/accounts",
    `q=${encodeURIComponent(search)}&role=${role}&type=${type}`,
  );
  const items = query.data?.data.items;
  return (
    <section className="mobile-stack" aria-labelledby="mobile-accounts">
      <h2 id="mobile-accounts" className="mobile-page-title">
        {t("mobile.accounts")}
      </h2>
      <div className="mobile-search">
        <Search aria-hidden="true" />
        <Input
          type="search"
          aria-label={t("mobile.search")}
          placeholder={t("mobile.search")}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
      </div>
      <div className="mobile-filter-row">
        <Segmented
          label={t("mobile.role")}
          value={role}
          onChange={setRole}
          options={[
            { value: "", label: t("mobile.all") },
            { value: "asset", label: t("mobile.assets") },
            { value: "liability", label: t("mobile.liabilities") },
          ]}
        />
        <NativeSelect
          aria-label={t("mobile.accountType")}
          value={type}
          onChange={(e) => setType(e.target.value)}
        >
          <option value="">{t("mobile.allTypes")}</option>
          {accountTypes.map((k) => (
            <option key={k} value={k}>
              {t(`enum.${k}`)}
            </option>
          ))}
        </NativeSelect>
      </div>
      <ErrorMessage error={query.error} />
      {query.isPending && <RowsSkeleton />}
      {items &&
        (items.length ? (
          <ul className="mobile-list mobile-card" aria-busy={query.isFetching}>
            {items.map((a) => (
              <li key={a.id}>
                <button
                  type="button"
                  className="mobile-row"
                  onClick={() => onOpen(a.id)}
                >
                  <AccountIcon type={a.type} />
                  <span className="mobile-row-main">
                    <span className="mobile-row-title">{a.name}</span>
                    <span className="mobile-row-sub">
                      {[
                        a.institution,
                        t(`enum.${a.type}`, { defaultValue: "" }),
                        a.currency,
                      ]
                        .filter(Boolean)
                        .join(" · ")}
                    </span>
                  </span>
                  <span className="mobile-row-end">
                    <MoneyText value={a.value} />
                    {a.role === "liability" && (
                      <small>{t("mobile.liability")}</small>
                    )}
                    {!a.complete && (
                      <small data-tone="warning">{t("mobile.partial")}</small>
                    )}
                  </span>
                  <ChevronRight
                    className="mobile-row-chevron"
                    aria-hidden="true"
                  />
                </button>
              </li>
            ))}
          </ul>
        ) : (
          <EmptyState>{t("mobile.noAccounts")}</EmptyState>
        ))}
      <Pager
        next={query.data?.data.next}
        onNext={query.next}
        onFirst={query.first}
        paged={query.paged}
      />
    </section>
  );
}
function activityLabel(t: TFunction, a: Activity) {
  return t(`history.reason.${a.reason || a.kind}`, {
    defaultValue: t(`history.kind.${a.kind}`, {
      defaultValue: t("mobile.ledger"),
    }),
  });
}
function ActivityIcon({ kind }: { kind: string }) {
  const Icon =
    kind === "cash_in" || kind === "cash_dividend"
      ? ArrowDownLeft
      : kind === "cash_out"
        ? ArrowUpRight
        : kind === "cash_transfer"
          ? ArrowLeftRight
          : kind === "debt_payment"
            ? CreditCard
            : kind === "value_update"
              ? Scale
              : ReceiptText;
  return (
    <span className="mobile-row-icon" data-kind={kind}>
      <Icon aria-hidden="true" />
    </span>
  );
}
function ActivityView({ activity }: { activity: Activity }) {
  const { t } = useTranslation();
  return (
    <div className="mobile-activity">
      <div className="mobile-activity-head">
        <ActivityIcon kind={activity.kind} />
        <div>
          <h3>{activityLabel(t, activity)}</h3>
          <p className="num">
            {activity.effectiveLocalDate
              ? formatCivilDate(activity.effectiveLocalDate)
              : activity.effectiveAt}
          </p>
        </div>
      </div>
      {activity.resulting?.length > 0 && (
        <dl className="mobile-values">
          {activity.resulting.map((r, i) => (
            <div key={i}>
              <dt>{r.name}</dt>
              <dd>
                {r.amount ? (
                  <MoneyText
                    value={{ amount: r.amount, currency: r.currency }}
                  />
                ) : r.quantity ? (
                  <span className="mobile-money">
                    {formatAmount(r.quantity)}
                  </span>
                ) : (
                  t("mobile.unavailable")
                )}
              </dd>
            </div>
          ))}
        </dl>
      )}
      {activity.note && <p className="mobile-note">{activity.note}</p>}
    </div>
  );
}
function ActivityRow({
  activity,
  onOpen,
  showDate,
}: {
  activity: Activity;
  onOpen: (id: string) => void;
  showDate?: boolean;
}) {
  const { t } = useTranslation();
  const date = activity.effectiveLocalDate
    ? formatCivilDate(activity.effectiveLocalDate)
    : activity.effectiveAt;
  const money = activity.effects?.find((e) => e.money)?.money;
  const accounts = [...new Set(activity.resulting?.map((r) => r.name))];
  return (
    <button
      type="button"
      className="mobile-row"
      onClick={() => onOpen(activity.id)}
    >
      <ActivityIcon kind={activity.kind} />
      <span className="mobile-row-main">
        <span className="mobile-row-title">{activityLabel(t, activity)}</span>
        <span className="mobile-row-sub">
          {[showDate && date, accounts.join(" → "), activity.note]
            .filter(Boolean)
            .join(" · ")}
        </span>
      </span>
      {money && (
        <span className="mobile-row-end">
          <MoneyText value={money} />
        </span>
      )}
      <ChevronRight className="mobile-row-chevron" aria-hidden="true" />
    </button>
  );
}
function ActivityList({
  items,
  onOpen,
  grouped = false,
}: {
  items: Activity[];
  onOpen: (id: string) => void;
  grouped?: boolean;
}) {
  const { t } = useTranslation();
  if (!items.length) return <EmptyState>{t("mobile.empty")}</EmptyState>;
  const groups: { date: string; items: Activity[] }[] = [];
  for (const a of items) {
    const date = a.effectiveLocalDate || a.effectiveAt.slice(0, 10);
    const last = groups[groups.length - 1];
    if (last?.date === date) last.items.push(a);
    else groups.push({ date, items: [a] });
  }
  if (!grouped)
    return (
      <ul className="mobile-list">
        {items.map((a) => (
          <li key={a.id}>
            <ActivityRow activity={a} onOpen={onOpen} showDate />
          </li>
        ))}
      </ul>
    );
  return (
    <div className="mobile-groups">
      {groups.map((g) => (
        <section key={g.date} aria-label={formatCivilDate(g.date)}>
          <h3 className="mobile-group-head num">{formatCivilDate(g.date)}</h3>
          <ul className="mobile-list">
            {g.items.map((a) => (
              <li key={a.id}>
                <ActivityRow activity={a} onOpen={onOpen} />
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}
function Activities({
  onOpen,
  today,
}: {
  onOpen: (id: string) => void;
  today: string;
}) {
  const { t } = useTranslation();
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [account, setAccount] = useState("");
  const [kind, setKind] = useState("");
  const [showFilters, setShowFilters] = useState(false);
  const accounts = usePage<Account>("/accounts");
  const query = usePage<Activity>(
    "/activities",
    `from=${from}&to=${to}&accountId=${account}&kind=${kind}`,
  );
  const active = [from, to, account, kind].filter(Boolean).length;
  return (
    <section className="mobile-stack" aria-labelledby="mobile-ledger">
      <div className="mobile-page-head">
        <h2 id="mobile-ledger" className="mobile-page-title">
          {t("mobile.ledger")}
        </h2>
        <Button
          variant={active ? "soft" : "outline"}
          size="sm"
          aria-expanded={showFilters}
          aria-controls="mobile-ledger-filters"
          onClick={() => setShowFilters((v) => !v)}
        >
          <SlidersHorizontal aria-hidden="true" />
          {t("mobile.filters")}
          {active > 0 && <span className="mobile-count num">{active}</span>}
        </Button>
      </div>
      {showFilters && (
        <div id="mobile-ledger-filters" className="mobile-panel mobile-enter">
          <div className="grid grid-cols-2 gap-3">
            <Field name={t("mobile.from")}>
              <DatePickerControl
                aria-label={t("mobile.from")}
                value={from}
                onChange={setFrom}
                max={today}
                clearable
              />
            </Field>
            <Field name={t("mobile.to")}>
              <DatePickerControl
                aria-label={t("mobile.to")}
                value={to}
                onChange={setTo}
                max={today}
                clearable
              />
            </Field>
          </div>
          <Field name={t("mobile.account")}>
            <NativeSelect
              aria-label={t("mobile.account")}
              value={account}
              onChange={(e) => setAccount(e.target.value)}
            >
              <option value="">{t("mobile.all")}</option>
              {accounts.data?.data.items.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </NativeSelect>
          </Field>
          <Field name={t("mobile.kind")}>
            <NativeSelect
              aria-label={t("mobile.kind")}
              value={kind}
              onChange={(e) => setKind(e.target.value)}
            >
              <option value="">{t("mobile.all")}</option>
              {[
                "cash_in",
                "cash_out",
                "cash_transfer",
                "debt_payment",
                "value_update",
              ].map((k) => (
                <option key={k} value={k}>
                  {t(`history.kind.${k}`)}
                </option>
              ))}
            </NativeSelect>
          </Field>
          {active > 0 && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setFrom("");
                setTo("");
                setAccount("");
                setKind("");
              }}
            >
              {t("mobile.clearFilters")}
            </Button>
          )}
        </div>
      )}
      <ErrorMessage error={query.error} />
      {query.isPending && <RowsSkeleton rows={5} />}
      {query.data && (
        <div className="mobile-card" aria-busy={query.isFetching}>
          <ActivityList items={query.data.data.items} onOpen={onOpen} grouped />
        </div>
      )}
      <Pager
        next={query.data?.data.next}
        onNext={query.next}
        onFirst={query.first}
        paged={query.paged}
      />
    </section>
  );
}

function gainOf(value: { Amount: string; Currency: string } | null) {
  return value ? { amount: value.Amount, currency: value.Currency } : null;
}
function EvidenceRows({
  components,
  timezone,
}: {
  components: Component[];
  timezone: string;
}) {
  const { t } = useTranslation();
  return (
    <ul className="mobile-list">
      {components.map((c, i) => (
        <li key={i} className="mobile-evidence">
          <div className="mobile-evidence-head">
            <span className="mobile-row-title">
              {c.instrumentName || t("mobile.value")}
              <small className="mobile-symbol">
                {c.instrumentSymbol || c.nativeCurrency}
              </small>
            </span>
            <MoneyText
              value={
                c.available
                  ? { amount: c.nativeAmount, currency: c.nativeCurrency }
                  : null
              }
            />
          </div>
          {c.priceEvidence && (
            <dl className="mobile-evidence-facts">
              <div>
                <dt>{t("mobile.effectiveDate")}</dt>
                <dd>
                  {formatQuoteAsOf(
                    {
                      quotedAt: c.priceEvidence.quotedAt,
                      effectiveDate: c.priceEvidence.effectiveDate ?? undefined,
                      timestampBasis: c.priceEvidence.effectiveDate
                        ? "date_label"
                        : undefined,
                    },
                    timezone,
                  )}
                </dd>
              </div>
              <div>
                <dt>{t("mobile.source")}</dt>
                <dd>
                  {t(`portfolio.source.${c.priceEvidence.source}`, {
                    defaultValue: t("mobile.unavailable"),
                  })}
                </dd>
              </div>
              {c.priceEvidence.fetchedAt && (
                <div>
                  <dt>{t("mobile.fetchedAt")}</dt>
                  <dd>{formatTimestamp(c.priceEvidence.fetchedAt, timezone)}</dd>
                </div>
              )}
            </dl>
          )}
        </li>
      ))}
    </ul>
  );
}
function Detail({
  kind,
  id,
  onHolding,
  onRecord,
  onMarket,
  canWrite,
  canRefresh,
}: {
  kind: "accounts" | "holdings" | "activities";
  id: string;
  onHolding: (id: string) => void;
  onRecord: () => void;
  onMarket: (request: MarketRequest) => void;
  canWrite: boolean;
  canRefresh: boolean;
}) {
  const { t } = useTranslation();
  const query = useQuery({
    queryKey: ["mobile", kind, id],
    queryFn: () => api<Envelope<DetailResponse>>(`/${kind}/${id}`),
  });
  const holdings = usePage<Holding>(
    "/holdings",
    `accountId=${kind === "accounts" ? id : ""}`,
  );
  if (!query.data)
    return (
      <>
        <ErrorMessage error={query.error} />
        {!query.error && <LoadingState label={t("common.loading")} />}
      </>
    );
  const timezone = query.data.ledgerTimezone;
  if (kind === "activities")
    return (
      <div className="mobile-card">
        <ActivityView activity={query.data.data as Activity} />
      </div>
    );
  const data = query.data.data as HoldingDetailResponse & AccountDetail;
  const valuation = kind === "accounts" ? data : data.valuation;
  const components = valuation.components ?? [];
  if (kind === "holdings") {
    const h = data.holding;
    const own = components.filter(
      (c) => !h.HoldingID || c.holdingId === h.HoldingID,
    );
    return (
      <div className="mobile-stack">
        <ErrorMessage error={query.error} />
        <section className="mobile-hero" aria-labelledby="mobile-detail-title">
          {h.InstrumentSymbol && (
            <p className="mobile-eyebrow">{h.InstrumentSymbol}</p>
          )}
          <h2 id="mobile-detail-title" className="mobile-page-title">
            {h.InstrumentName}
          </h2>
          {h.CurrentValue !== undefined && (
            <p className="mobile-hero-figure">
              <MoneyText value={h.CurrentValue} />
            </p>
          )}
          {h.CurrentValueBase &&
            h.CurrentValue &&
            h.CurrentValueBase.currency !== h.CurrentValue.currency && (
              <p className="mobile-meta">
                <MoneyText value={h.CurrentValueBase} />
              </p>
            )}
        </section>
        <dl className="mobile-stats mobile-card">
          <div>
            <dt>{t("mobile.quantity")}</dt>
            <dd className="mobile-money">{formatAmount(h.Quantity)}</dd>
          </div>
          <div>
            <dt>{t("mobile.quoteValue")}</dt>
            <dd className="mobile-money">
              {data.quote
                ? formatQuotePrice(data.quote.unitPrice, data.quote.currency)
                : t("mobile.unavailable")}
            </dd>
          </div>
          <div>
            <dt>{t("mobile.cost")}</dt>
            <dd>
              <MoneyText value={h.TotalCost} />
            </dd>
          </div>
          <div>
            <dt>{t("mobile.gain")}</dt>
            <dd>
              <GainMoneyText value={gainOf(h.UnrealizedGain)} />
            </dd>
          </div>
          <div>
            <dt>{t("mobile.realizedGain")}</dt>
            <dd>
              <GainMoneyText value={gainOf(h.RealizedGain)} />
            </dd>
          </div>
        </dl>
        <Button
          variant="outline"
          className="mobile-wide"
          disabled={!canRefresh}
          onClick={() =>
            onMarket({
              mode: "latest",
              scope: "instrument",
              instrumentId: h.InstrumentID,
            })
          }
        >
          <RefreshCw aria-hidden="true" />
          {t("mobile.updateMarket")}
        </Button>
        {own.length > 0 && (
          <section className="mobile-card" aria-labelledby="mobile-evidence">
            <h2 id="mobile-evidence">{t("mobile.valuationEvidence")}</h2>
            <EvidenceRows components={own} timezone={timezone} />
          </section>
        )}
      </div>
    );
  }
  const account = data.account;
  const holdingItems = holdings.data?.data.items;
  const showHoldings =
    account.trackingMode !== "balance" || !!holdingItems?.length;
  return (
    <div className="mobile-stack">
      <ErrorMessage error={query.error} />
      <section className="mobile-hero" aria-labelledby="mobile-detail-title">
        {(account.accountType || account.defaultCurrency) && (
          <p className="mobile-eyebrow">
            {[
              account.accountType &&
                t(`enum.${account.accountType}`, { defaultValue: "" }),
              account.defaultCurrency,
            ]
              .filter(Boolean)
              .join(" · ")}
          </p>
        )}
        <h2 id="mobile-detail-title" className="mobile-page-title">
          {account.name}
        </h2>
        {data.displayValue !== undefined && (
          <p className="mobile-hero-figure">
            <MoneyText value={data.displayValue?.value} />
          </p>
        )}
        <p className="mobile-meta">
          {data.complete !== undefined && (
            <span
              className="mobile-badge"
              data-tone={data.complete ? "success" : "warning"}
            >
              {data.complete ? t("mobile.complete") : t("mobile.partial")}
            </span>
          )}
          {account.balanceSheetRole === "liability" && (
            <span className="mobile-badge">{t("mobile.liability")}</span>
          )}
          {data.baseValue &&
            data.displayValue?.value &&
            data.baseValue.currency !== data.displayValue.value.currency && (
              <MoneyText value={data.baseValue} />
            )}
        </p>
        <Button
          className="mobile-wide"
          disabled={!canWrite}
          onClick={onRecord}
        >
          <Plus aria-hidden="true" />
          {t("mobile.quickRecord")}
        </Button>
      </section>
      {showHoldings && (
        <section className="mobile-card" aria-labelledby="mobile-holdings">
          <h2 id="mobile-holdings">{t("mobile.holdings")}</h2>
          <ErrorMessage error={holdings.error} />
          {holdings.isPending && <RowsSkeleton rows={2} />}
          {holdingItems &&
            (holdingItems.length ? (
              <ul className="mobile-list" aria-busy={holdings.isFetching}>
                {holdingItems.map((h) => (
                  <li key={h.id}>
                    <button
                      type="button"
                      className="mobile-row"
                      onClick={() => onHolding(h.id)}
                    >
                      <span className="mobile-row-main">
                        <span className="mobile-row-title">
                          {h.name}
                          {h.symbol && (
                            <small className="mobile-symbol">{h.symbol}</small>
                          )}
                        </span>
                        <span className="mobile-row-sub num">
                          {t("mobile.quantity")} {formatAmount(h.quantity)}
                        </span>
                      </span>
                      <span className="mobile-row-end">
                        <MoneyText value={h.value} />
                        <small>
                          {h.gain ? (
                            <GainMoneyText value={h.gain} />
                          ) : (
                            h.missingReason && t("mobile.unavailable")
                          )}
                        </small>
                      </span>
                      <ChevronRight
                        className="mobile-row-chevron"
                        aria-hidden="true"
                      />
                    </button>
                  </li>
                ))}
              </ul>
            ) : (
              <EmptyState>{t("mobile.noHoldings")}</EmptyState>
            ))}
          <Pager
            next={holdings.data?.data.next}
            onNext={holdings.next}
            onFirst={holdings.first}
            paged={holdings.paged}
          />
        </section>
      )}
      {components.length > 0 && (
        <section className="mobile-card" aria-labelledby="mobile-evidence">
          <h2 id="mobile-evidence">{t("mobile.valuationEvidence")}</h2>
          <EvidenceRows components={components} timezone={timezone} />
        </section>
      )}
    </div>
  );
}
function Health({
  onQuote,
  onRecord,
  onMarket,
  capabilities,
  disabled,
}: {
  onQuote: (i: Issue) => void;
  onRecord: (id: string) => void;
  onMarket: (r: MarketRequest) => void;
  capabilities: Bootstrap["capabilities"];
  disabled: boolean;
}) {
  const { t } = useTranslation();
  const [collapsed, setCollapsed] = useState(false);
  const [scan, setScan] = useState(0);
  const query = usePage<Issue>("/health", `scan=${scan}`);
  return (
    <Section title={t("mobile.health")}>
      <Button variant="outline" onClick={() => setScan((p) => p + 1)}>
        {t("mobile.rescan")}
      </Button>
      <Button
        variant="ghost"
        aria-pressed={collapsed}
        onClick={() => setCollapsed(!collapsed)}
      >
        {t("mobile.showRelated")}
      </Button>
      <ErrorMessage error={query.error} />
      {query.isPending && <LoadingState label={t("common.loading")} />}
      {(query.data?.data.items ?? [])
        .filter((i) => collapsed || !i.Collapsed)
        .map((i) => {
          const manual = ["missing_manual_price", "missing_manual_fx"].includes(
            i.Kind,
          );
          const balance = [
            "stale_account_value",
            "missing_account_value",
          ].includes(i.Kind);
          const sync =
            i.Executable &&
            i.Action === "repair" &&
            (i.InstrumentID || (i.CurrencyA && i.CurrencyB));
          return (
            <Card className="p-4 space-y-3" key={i.ID}>
              <h3 className="font-semibold">{i.Label}</h3>
              <p className="text-xs">
                {t(`mobile.severity.${i.Severity}`, {
                  defaultValue: t("mobile.partial"),
                })}
              </p>
              <p>
                {t(`dataHealth.kind.${i.Kind}`, {
                  defaultValue: t("mobile.desktopRequired"),
                })}
              </p>
              <p className="text-sm">
                {t(`dataHealth.reason.${i.Reason}`, {
                  defaultValue: t("mobile.partial"),
                })}
              </p>
              {i.RangeStart && (
                <p>
                  {i.RangeEnd && i.RangeEnd < i.RangeStart
                    ? t("mobile.initialAnchor", {
                        date: formatCivilDate(i.RangeStart),
                      })
                    : `${formatCivilDate(i.RangeStart)} — ${formatCivilDate(i.RangeEnd || i.RangeStart)}`}
                </p>
              )}
              {manual && capabilities.manual_quote ? (
                <Button disabled={disabled} onClick={() => onQuote(i)}>
                  {t("mobile.manualQuote")}
                </Button>
              ) : balance && capabilities.record && i.AccountID ? (
                <Button
                  disabled={disabled}
                  onClick={() => onRecord(i.AccountID)}
                >
                  {t("mobile.balance")}
                </Button>
              ) : sync && capabilities.market_refresh ? (
                <Button
                  disabled={disabled}
                  onClick={() =>
                    onMarket({
                      mode: "history",
                      scope: i.InstrumentID ? "instrument" : "fx",
                      instrumentId: i.InstrumentID || undefined,
                      currencyA: i.CurrencyA || undefined,
                      currencyB: i.CurrencyB || undefined,
                    })
                  }
                >
                  {t("mobile.syncHistory")}
                </Button>
              ) : (
                <p className="text-sm text-muted-foreground">
                  {t("mobile.desktopRequired")}
                </p>
              )}
            </Card>
          );
        })}
      {query.data?.data.healthy && <p>{t("mobile.healthy")}</p>}
      <Pager
        next={query.data?.data.next}
        onNext={query.next}
        onFirst={query.first}
        paged={query.paged}
      />
    </Section>
  );
}

function JobPanel({
  onRetry,
  disabled,
}: {
  onRetry: (r: MarketRequest) => void;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [cursor, setCursor] = useState("");
  const [pinned, setPinned] = useState("");
  const [cancelling, setCancelling] = useState(false);
  const [cancelRequested, setCancelRequested] = useState("");
  const [error, setError] = useState<unknown>();
  const lock = useRef(false);
  const query = useQuery({
    queryKey: ["mobile", "job", pinned, cursor],
    queryFn: ({ signal }) =>
      api<{ job: Job | null; next?: string }>(
        `/market-jobs/${pinned || "current"}?cursor=${encodeURIComponent(cursor)}`,
        "GET",
        undefined,
        { signal },
      ),
    refetchOnWindowFocus: "always",
    refetchInterval: (query) =>
      document.visibilityState === "hidden"
        ? false
        : query.state.data?.job?.outcome === "running"
          ? 2000
          : 5000,
  });
  const job = query.data?.job;
  const jobId = job?.jobId;
  const outcome = job?.outcome;
  useEffect(() => {
    if (jobId && outcome !== "running") {
      void client.invalidateQueries({ queryKey: ["mobile", "/health"] });
      void client.invalidateQueries({ queryKey: ["mobile", "overview"] });
    }
  }, [jobId, outcome, client]);
  const reload = () => {
    setCursor("");
    setPinned("");
    void client.invalidateQueries({ queryKey: ["mobile", "job"] });
  };
  if (!job)
    return query.error ? (
      <div>
        <ErrorMessage error={query.error} />
        <Button variant="outline" onClick={reload}>
          {t("mobile.reloadJob")}
        </Button>
      </div>
    ) : null;
  const codes = [
    ...new Set([job.errorCode, ...(job.blockers ?? [])].filter(Boolean)),
  ];
  return (
    <Card className="p-4 space-y-3" aria-label={t("mobile.marketJob")}>
      <h2 className="font-semibold">{t("mobile.marketJob")}</h2>
      <p role="status">
        {job.phase === "queued"
          ? t("mobile.jobQueued")
          : t(`marketData.outcome.${job.outcome}`, {
              defaultValue: t("mobile.requestFailed"),
            })}{" "}
        · {job.completed}/{job.total}
      </p>
      <p>
        {t(`marketData.phase.${job.phase}`, {
          defaultValue: t(
            job.request.mode === "history"
              ? "mobile.syncHistory"
              : "mobile.updateMarket",
          ),
        })}
      </p>
      {job.source === "shared" && (
        <p className="text-sm">{t("mobile.sharedJob")}</p>
      )}
      {job.current && (
        <p>{t("mobile.currentTarget", { target: job.current })}</p>
      )}
      {job.outcome === "running" && (
        <progress
          className="w-full"
          aria-label={t("mobile.marketProgress")}
          max={Math.max(job.total, 1)}
          value={job.completed}
        />
      )}
      {job.nextEligibilityAt && (
        <p>
          {t("mobile.nextEligibility", {
            time: formatTimestamp(job.nextEligibilityAt),
          })}
        </p>
      )}
      <ul className="space-y-2">
        {job.items?.map((item, index) => (
          <li key={index} className="text-sm break-words">
            {item.label || t("mobile.unavailable")} ·{" "}
            {t(`marketData.status.${item.status}`, {
              defaultValue: t(
                item.status === "failed"
                  ? "mobile.requestFailed"
                  : "mobile.complete",
              ),
            })}
            {item.startDate && item.endDate && (
              <>
                {" "}
                · {formatCivilDate(item.startDate)} —{" "}
                {formatCivilDate(item.endDate)}
              </>
            )}
            {item.errorCode && (
              <p>
                {t(`errorCode:${item.errorCode}`, {
                  defaultValue: t("mobile.requestFailed"),
                })}
              </p>
            )}
          </li>
        ))}
      </ul>
      {codes.map((code) => (
        <p key={code}>
          {t(`errorCode:${code}`, { defaultValue: t("mobile.requestFailed") })}
        </p>
      ))}
      {!job.items?.length &&
        job.result?.Items?.filter(
          (i) => i.Status === "failed" || i.Status === "rate_limited",
        ).map((item, index) => (
          <p key={index}>
            {t(`errorCode:${item.ErrorCode}`, {
              defaultValue: t("mobile.requestFailed"),
            })}
          </p>
        ))}
      <ErrorMessage error={query.error || error} />
      {query.error && (
        <Button variant="outline" onClick={reload}>
          {t("mobile.reloadJob")}
        </Button>
      )}
      {job.cancellable && (
        <Button
          variant="outline"
          disabled={disabled || cancelling || cancelRequested === job.jobId}
          onClick={() => {
            if (lock.current) return;
            lock.current = true;
            setCancelling(true);
            setError(null);
            void api<{ requested: boolean }>(
              `/market-jobs/${job.jobId}/cancel`,
              "POST",
              {},
            )
              .then((result) => {
                if (result.requested) setCancelRequested(job.jobId);
                else throw new MobileError(409, "unavailable");
                void client.invalidateQueries({ queryKey: ["mobile", "job"] });
              })
              .catch(setError)
              .finally(() => {
                lock.current = false;
                setCancelling(false);
              });
          }}
        >
          {t("mobile.cancelJob")}
        </Button>
      )}
      {job.outcome === "running" && cancelRequested === job.jobId && (
        <p role="status">{t("mobile.cancelRequested")}</p>
      )}
      {["partial", "failed", "cancelled"].includes(job.outcome) &&
        (job.retryable ? (
          <>
            <p className="text-sm">{t("mobile.retryJobHelp")}</p>
            <Button
              disabled={disabled}
              onClick={() =>
                onRetry({
                  ...job.request,
                  scope:
                    job.request.scope === "all" ? "stale" : job.request.scope,
                })
              }
            >
              {t("mobile.retryFailed")}
            </Button>
          </>
        ) : (
          <p>{t("mobile.desktopRequired")}</p>
        ))}
      {(cursor || query.data?.next) && (
        <Pager
          next={query.data?.next}
          paged={!!cursor}
          onFirst={reload}
          onNext={() => {
            setPinned(job.jobId);
            setCursor(query.data?.next ?? "");
          }}
        />
      )}
    </Card>
  );
}

function useLeaveGuard(active: boolean) {
  useEffect(() => {
    if (!active) return;
    const guard = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", guard);
    return () => window.removeEventListener("beforeunload", guard);
  }, [active]);
}
function CancelDraft({
  disabled,
  onCancel,
}: {
  disabled: boolean;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button
        type="button"
        disabled={disabled}
        variant="outline"
        onClick={() => setOpen(true)}
      >
        {t("common.cancel")}
      </Button>
      <AlertDialog open={open} onOpenChange={setOpen}>
        <AlertDialogContent>
          <AlertDialogTitle>{t("mobile.discardTitle")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("mobile.discardBody")}
          </AlertDialogDescription>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <Button onClick={onCancel}>{t("mobile.discard")}</Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

function RecordForm({
  bootstrap: b,
  timezone,
  accountId,
  initialKind,
  offline,
  onCancel,
  onDispatch,
  onSuccess,
}: {
  bootstrap: Bootstrap;
  timezone: string;
  accountId: string;
  initialKind: string;
  offline: boolean;
  onCancel: () => void;
  onDispatch: (p: Pending | null) => void;
  onSuccess: (activity?: Activity, keepOpen?: boolean) => boolean;
}) {
  const { t } = useTranslation();
  const accounts = usePage<Account>("/accounts", "limit=100");
  const [input, setInput] = useState<RecordInput>({
    operationId: crypto.randomUUID(),
    kind: initialKind,
    accountId,
    amount: "",
    currency: b.currency,
    effectiveLocalDate: b.today,
    effectiveLocalTime: b.time.slice(0, 5),
    note: "",
  });
  const [customTime, setCustomTime] = useState(false);
  const [review, setReview] = useState<Review | null>(null);
  const [busy, setBusy] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const [error, setError] = useState<unknown>();
  const lock = useRef(false);
  const continueEntry = useRef(false);
  const amountRef = useRef<HTMLInputElement>(null);
  const [focusAmount, setFocusAmount] = useState(0);
  useEffect(() => {
    if (focusAmount > 0) amountRef.current?.focus();
  }, [focusAmount]);
  function saved(activity?: Activity) {
    if (!onSuccess(activity, continueEntry.current)) return;
    if (continueEntry.current) {
      setReview(null);
      setUncertain(false);
      setError(null);
      setCustomTime(false);
      setInput((previous) => ({
        ...previous,
        operationId: crypto.randomUUID(),
        amount: "",
        received: "",
        fee: "",
        note: "",
        effectiveLocalDate: b.today,
        effectiveLocalTime: b.time.slice(0, 5),
      }));
      setFocusAmount((value) => value + 1);
    }
  }
  useLeaveGuard(true);
  const frozen = busy || uncertain;
  const update = (field: keyof RecordInput, value: string) => {
    if (frozen) return;
    setInput((p) => ({ ...p, [field]: value }));
    setReview(null);
  };
  const selected = accounts.data?.data.items.find(
    (a) => a.id === input.accountId,
  );
  const [initialized, setInitialized] = useState(false);
  if (!initialized && selected && !review && !frozen) {
    setInitialized(true);
    if (input.currency !== selected.currency)
      setInput((p) => ({ ...p, currency: selected.currency }));
  }
  const eligible = (a: Account) =>
    a.mode !== "manual_value" &&
    (input.kind.startsWith("card_")
      ? a.type === "credit_card"
      : input.kind === "repayment"
        ? a.role === "liability"
        : input.kind === "balance" || a.role !== "liability");
  async function preview() {
    if (lock.current) return;
    lock.current = true;
    setBusy(true);
    setError(null);
    try {
      const r = await api<Review>("/record-previews", "POST", {
        ...input,
        effectiveLocalDate:
          input.kind === "balance" || !customTime
            ? undefined
            : input.effectiveLocalDate,
        effectiveLocalTime:
          input.kind === "balance" || !customTime
            ? undefined
            : input.effectiveLocalTime,
      });
      setReview(r);
    } catch (e) {
      setError(e);
    } finally {
      lock.current = false;
      setBusy(false);
    }
  }
  async function confirm(another = false) {
    if (lock.current || !review) return;
    lock.current = true;
    setBusy(true);
    if (!uncertain) continueEntry.current = another;
    setUncertain(true);
    onDispatch({ operationId: input.operationId, householdId: b.householdId });
    setError(null);
    try {
      const r = await api<{ receipt: { activity: Activity } }>(
        "/record-confirmations",
        "POST",
        { planId: review.planId, operationId: input.operationId },
      );
      saved(r.receipt.activity);
    } catch (e) {
      setError(e);
      if (e instanceof MobileError && e.operationState === "rejected") {
        setUncertain(false);
        setReview(null);
        onDispatch(null);
      }
    } finally {
      lock.current = false;
      setBusy(false);
    }
  }
  return (
    <Section title={t("mobile.quickRecord")}>
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault();
          void (review ? confirm() : preview());
        }}
      >
        <fieldset disabled={frozen || !!review} className="space-y-4">
          <Field name={t("mobile.kind")}>
            <NativeSelect
              aria-label={t("mobile.kind")}
              value={input.kind}
              onChange={(e) => {
                update("kind", e.target.value);
                update("accountId", "");
              }}
            >
              {[
                "income",
                "expense",
                "transfer",
                "card_purchase",
                "card_refund",
                "repayment",
                "balance",
              ].map((k) => (
                <option key={k} value={k}>
                  {t(`mobile.${k}`)}
                </option>
              ))}
            </NativeSelect>
          </Field>
          <Field name={t("mobile.account")}>
            <NativeSelect
              aria-label={t("mobile.account")}
              value={input.accountId}
              required
              onChange={(e) => {
                update("accountId", e.target.value);
                const a = accounts.data?.data.items.find(
                  (a) => a.id === e.target.value,
                );
                if (a) update("currency", a.currency);
              }}
            >
              <option value="">{t("mobile.choose")}</option>
              {accounts.data?.data.items.filter(eligible).map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </NativeSelect>
          </Field>
          <Field name={t("mobile.amount")}>
            <Input
              ref={amountRef}
              aria-label={t("mobile.amount")}
              inputMode="decimal"
              value={input.amount}
              onChange={(e) => update("amount", e.target.value)}
              required
            />
          </Field>
          <Field name={t("mobile.currency")}>
            <NativeSelect
              aria-label={t("mobile.currency")}
              disabled={selected?.mode !== "holdings"}
              value={input.currency}
              onChange={(e) => update("currency", e.target.value)}
            >
              {currencies.map((c) => (
                <option key={c}>{c}</option>
              ))}
            </NativeSelect>
          </Field>
          {["transfer", "repayment"].includes(input.kind) && (
            <Field name={t("mobile.otherAccount")}>
              <NativeSelect
                aria-label={t("mobile.otherAccount")}
                value={input.otherAccountId ?? ""}
                required
                onChange={(e) => {
                  update("otherAccountId", e.target.value);
                  const a = accounts.data?.data.items.find(
                    (a) => a.id === e.target.value,
                  );
                  if (a) update("receivedCurrency", a.currency);
                }}
              >
                <option value="">{t("mobile.choose")}</option>
                {accounts.data?.data.items
                  .filter(
                    (a) =>
                      a.role !== "liability" &&
                      a.mode !== "manual_value" &&
                      a.id !== input.accountId,
                  )
                  .map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.name}
                    </option>
                  ))}
              </NativeSelect>
            </Field>
          )}
          {input.kind === "transfer" && (
            <>
              <Field name={t("mobile.received")}>
                <Input
                  aria-label={t("mobile.received")}
                  inputMode="decimal"
                  value={input.received ?? ""}
                  onChange={(e) => update("received", e.target.value)}
                  required
                />
              </Field>
              <Field name={t("mobile.receivedCurrency")}>
                <NativeSelect
                  aria-label={t("mobile.receivedCurrency")}
                  value={input.receivedCurrency ?? ""}
                  onChange={(e) => update("receivedCurrency", e.target.value)}
                  required
                >
                  <option value="">{t("mobile.choose")}</option>
                  {currencies.map((c) => (
                    <option key={c}>{c}</option>
                  ))}
                </NativeSelect>
              </Field>
            </>
          )}
          {["transfer", "repayment"].includes(input.kind) && (
            <Field name={t("mobile.fee")}>
              <Input
                aria-label={t("mobile.fee")}
                inputMode="decimal"
                value={input.fee ?? ""}
                onChange={(e) => update("fee", e.target.value)}
              />
            </Field>
          )}
          {input.kind !== "balance" ? (
            <div className="grid grid-cols-2 gap-3">
              <DatePickerControl
                aria-label={t("mobile.date")}
                value={input.effectiveLocalDate!}
                onChange={(v) => {
                  setCustomTime(true);
                  update("effectiveLocalDate", v);
                }}
                max={b.today}
              />
              <TimePicker
                aria-label={t("mobile.time")}
                value={input.effectiveLocalTime!.slice(0, 5)}
                onChange={(v) => {
                  setCustomTime(true);
                  update("effectiveLocalTime", v);
                }}
              />
            </div>
          ) : (
            <p>{t("mobile.balanceHelp")}</p>
          )}
          <p className="text-xs">{t("mobile.ledgerTimezone", { timezone })}</p>
          <Field name={t("mobile.note")}>
            <Input
              aria-label={t("mobile.note")}
              value={input.note}
              maxLength={500}
              onChange={(e) => update("note", e.target.value)}
            />
          </Field>
        </fieldset>
        <ErrorMessage error={error || accounts.error} />
        {review && (
          <Card className="p-4 space-y-2">
            <p>{t("mobile.reviewEffects")}</p>
            {review.before?.map((r, i) => (
              <p key={i}>
                {r.name} ·{" "}
                <ExactMoney
                  value={{ amount: r.amount, currency: r.currency }}
                />{" "}
                →{" "}
                <ExactMoney
                  value={
                    review.preview!.resulting[i]
                      ? {
                          amount: review.preview!.resulting[i].amount,
                          currency: r.currency,
                        }
                      : null
                  }
                />
              </p>
            ))}
            <ActivityView
              activity={{
                ...review.preview!.activity,
                resulting: review.preview!.resulting,
              }}
            />
            {review.preview!.effects.map((e, i) => (
              <p key={i}>
                {t(`mobile.classification.${e.classification}`, {
                  defaultValue: t("mobile.value"),
                })}{" "}
                · <MoneyText value={e.money} />
              </p>
            ))}
          </Card>
        )}
        {uncertain && <p role="status">{t("mobile.confirming")}</p>}
        <div className="flex flex-wrap gap-2">
          <Button disabled={busy || offline || !input.accountId}>
            {busy
              ? t("common.pending")
              : uncertain
                ? t("mobile.retrySame")
                : review
                  ? t("mobile.confirm")
                  : t("mobile.preview")}
          </Button>
          {review && !uncertain && input.kind !== "balance" && (
            <Button
              type="button"
              disabled={busy || offline}
              onClick={() => void confirm(true)}
            >
              {t("mobile.confirmAnother")}
            </Button>
          )}
          {uncertain && (
            <Button
              type="button"
              variant="outline"
              disabled={busy || offline}
              onClick={async () => {
                if (lock.current) return;
                lock.current = true;
                setBusy(true);
                try {
                  const result = await api<{
                    operationState: string;
                    receipt?: { activity: Activity };
                  }>(`/operations/${input.operationId}`);
                  if (result.operationState === "committed")
                    saved(result.receipt?.activity);
                  else setError(new MobileError(503, "unavailable"));
                } catch (e) {
                  setError(e);
                } finally {
                  lock.current = false;
                  setBusy(false);
                }
              }}
            >
              {t("mobile.queryResult")}
            </Button>
          )}
          {review && (
            <Button
              type="button"
              variant="outline"
              disabled={frozen}
              onClick={() => setReview(null)}
            >
              {t("mobile.edit")}
            </Button>
          )}
          <CancelDraft disabled={frozen} onCancel={onCancel} />
        </div>
      </form>
    </Section>
  );
}

function QuoteForm({
  issue,
  bootstrap: b,
  offline,
  onCancel,
  onDispatch,
  onSuccess,
}: {
  issue: Issue;
  bootstrap: Bootstrap;
  offline: boolean;
  onCancel: () => void;
  onDispatch: (p: Pending | null) => void;
  onSuccess: (pending: boolean) => void;
}) {
  const { t } = useTranslation();
  const [operationId] = useState(() => crypto.randomUUID());
  const [value, setValue] = useState("");
  const [date, setDate] = useState(issue.RangeEnd || b.today);
  const [review, setReview] = useState<Review | null>(null);
  const [busy, setBusy] = useState(false);
  const [unknown, setUnknown] = useState(false);
  const [error, setError] = useState<unknown>();
  const lock = useRef(false);
  useLeaveGuard(true);
  async function submit() {
    if (lock.current) return;
    lock.current = true;
    setBusy(true);
    setError(null);
    try {
      if (!review)
        setReview(
          await api<Review>("/quote-previews", "POST", {
            operationId,
            instrumentId: issue.InstrumentID || undefined,
            currency: issue.CurrencyA || undefined,
            quoteCurrency: issue.CurrencyB || undefined,
            value,
            quotedAt: date,
          }),
        );
      else {
        setUnknown(true);
        onDispatch({ operationId, householdId: b.householdId });
        const saved = await api<{ receipt: { derivedPending?: boolean } }>(
          "/quote-confirmations",
          "POST",
          { planId: review.planId, operationId },
        );
        onSuccess(saved.receipt.derivedPending === true);
      }
    } catch (e) {
      setError(e);
      if (e instanceof MobileError && e.operationState === "rejected") {
        setUnknown(false);
        setReview(null);
        onDispatch(null);
      }
    } finally {
      lock.current = false;
      setBusy(false);
    }
  }
  return (
    <Section title={t("mobile.manualQuote")}>
      <p>{issue.Label}</p>
      <p>{t("mobile.manualHelp")}</p>
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
      >
        <fieldset className="space-y-4" disabled={busy || unknown || !!review}>
          <Field name={t("mobile.quoteValue")}>
            <Input
              aria-label={t("mobile.quoteValue")}
              value={value}
              inputMode="decimal"
              required
              onChange={(e) => setValue(e.target.value)}
            />
          </Field>
          <DatePickerControl
            aria-label={t("mobile.effectiveDate")}
            value={date}
            onChange={setDate}
            max={b.today}
          />
        </fieldset>
        <ErrorMessage error={error} />
        {review?.receipt && (
          <Card className="p-4">
            <p>
              {review.receipt.value} {review.receipt.currency}
              {review.receipt.quoteCurrency
                ? `/${review.receipt.quoteCurrency}`
                : ""}
            </p>
            <p>
              {formatQuoteAsOf(
                {
                  quotedAt: review.receipt.quotedAt,
                  effectiveDate: review.receipt.effectiveDate,
                  timestampBasis: review.receipt.effectiveDate
                    ? "date_label"
                    : undefined,
                },
                "UTC",
              )}
            </p>
          </Card>
        )}
        {unknown && <p role="status">{t("mobile.confirming")}</p>}
        <div className="flex gap-2">
          <Button disabled={busy || offline}>
            {unknown
              ? t("mobile.retrySame")
              : review
                ? t("mobile.confirm")
                : t("mobile.preview")}
          </Button>
          {unknown && (
            <Button
              type="button"
              variant="outline"
              disabled={busy || offline}
              onClick={async () => {
                if (lock.current) return;
                lock.current = true;
                setBusy(true);
                try {
                  const result = await api<{ operationState: string }>(
                    `/operations/${operationId}`,
                  );
                  if (result.operationState === "committed") onSuccess(false);
                  else setError(new MobileError(503, "unavailable"));
                } catch (e) {
                  setError(e);
                } finally {
                  lock.current = false;
                  setBusy(false);
                }
              }}
            >
              {t("mobile.queryResult")}
            </Button>
          )}
          <CancelDraft disabled={busy || unknown} onCancel={onCancel} />
        </div>
      </form>
    </Section>
  );
}

function MarketReview({
  request,
  onCancel,
  onStarted,
}: {
  request: MarketRequest;
  onCancel: () => void;
  onStarted: () => void;
}) {
  const { t } = useTranslation();
  const [review, setReview] = useState<Review | null>(null);
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  const lock = useRef(false);
  const [uncertain, setUncertain] = useState(false);
  useLeaveGuard(busy || uncertain);
  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    void api<Review>("/market-previews", "POST", request, {
      signal: controller.signal,
    })
      .then((r) => {
        if (active) setReview(r);
      })
      .catch((e) => {
        if (active) setError(e);
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [request]);
  return (
    <Section
      title={
        request.mode === "history"
          ? t("mobile.syncHistory")
          : t("mobile.updateMarket")
      }
    >
      <p>
        {request.mode === "history"
          ? t("mobile.historyScope")
          : t("mobile.latestScope")}
      </p>
      <p>
        {review?.targetLabel ||
          t(`mobile.scope.${request.scope}`, { defaultValue: "" })}
      </p>
      {review?.historicalPlan && (
        <>
          <p>
            {t("mobile.plannedRequests", {
              count: review.historicalPlan.EstimatedRequestCount,
            })}
          </p>
          {(review.historicalPlan.Items ?? []).slice(0, 100).map((i, index) => (
            <p key={index}>
              {i.Label} · {formatCivilDate(i.StartDate)} —{" "}
              {formatCivilDate(i.EndDate)}
            </p>
          ))}
          {review.historicalPlan.Unresolved?.map((item, index) => (
            <p key={index}>
              {t(`errorCode:${item.Code}`, {
                defaultValue: t("mobile.desktopRequired"),
              })}
            </p>
          ))}
        </>
      )}
      {uncertain && <p role="status">{t("mobile.jobAcceptanceUnknown")}</p>}
      <ErrorMessage error={error} />
      <div className="flex gap-2">
        <Button
          disabled={busy || !review}
          onClick={() => {
            if (lock.current || !review) return;
            lock.current = true;
            setBusy(true);
            setUncertain(true);
            setError(null);
            void api<{ conflict: boolean }>("/market-jobs", "POST", {
              planId: review.planId,
            })
              .then((r) => {
                if (r.conflict)
                  throw new MobileError(409, "conflict", "", "rejected");
                onStarted();
              })
              .catch((error) => {
                setError(error);
                if (
                  error instanceof MobileError &&
                  error.operationState === "rejected"
                )
                  setUncertain(false);
              })
              .finally(() => {
                lock.current = false;
                setBusy(false);
              });
          }}
        >
          {uncertain ? t("mobile.retrySame") : t("mobile.confirm")}
        </Button>
        <Button
          disabled={busy || uncertain}
          variant="outline"
          onClick={onCancel}
        >
          {t("common.cancel")}
        </Button>
      </div>
    </Section>
  );
}

function Recovery({
  pending,
  householdId,
  onResolved,
  onForget,
}: {
  pending: Pending;
  householdId: string;
  onResolved: (a?: Activity) => void;
  onForget: () => void;
}) {
  const { t } = useTranslation();
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  const [forget, setForget] = useState(false);
  return (
    <Card className="p-4 space-y-3">
      <p role="status">{t("mobile.confirming")}</p>
      <p>{t("mobile.recoveryHelp")}</p>
      <ErrorMessage error={error} />
      <Button
        disabled={busy || pending.householdId !== householdId}
        onClick={() => {
          setBusy(true);
          void api<{
            operationState: string;
            receipt?: { activity?: Activity };
          }>(`/operations/${pending.operationId}`)
            .then((r) => {
              if (r.operationState === "committed")
                onResolved(r.receipt?.activity);
              else setError(new MobileError(503, "unavailable"));
            })
            .catch(setError)
            .finally(() => setBusy(false));
        }}
      >
        {t("mobile.queryResult")}
      </Button>
      <Button variant="outline" disabled={busy} onClick={() => setForget(true)}>
        {t("mobile.forgetRecovery")}
      </Button>
      <AlertDialog open={forget} onOpenChange={setForget}>
        <AlertDialogContent>
          <AlertDialogTitle>{t("mobile.forgetRecovery")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("mobile.forgetHelp")}
          </AlertDialogDescription>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <Button onClick={onForget}>{t("mobile.verifiedOnDesktop")}</Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}
