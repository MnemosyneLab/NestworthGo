import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
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
type AccountDetail = { account: { name: string }; components: Component[] };
type HoldingDetailResponse = {
  holding: {
    InstrumentName: string;
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
  return (
    <main className="mobile-shell space-y-6">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-xl font-bold">{b?.name || "Nestworth"}</h1>
        <div className="flex gap-2">
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
          <NativeSelect
            aria-label={t("mobile.theme")}
            value={theme}
            onChange={(e) => {
              const value = e.target.value as typeof theme;
              setTheme(value);
              saveMobilePreferences({ language: i18n.language, theme: value });
            }}
          >
            {["system", "light", "dark"].map((value) => (
              <option key={value} value={value}>
                {t(`mobile.themeChoice.${value}`)}
              </option>
            ))}
          </NativeSelect>
          <Button
            variant="ghost"
            disabled={connectionLocked || reconnecting}
            onClick={() => setShowConnection((v) => !v)}
          >
            {t("mobile.connectionSettings")}
          </Button>
          <Button
            variant="ghost"
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
      </header>
      {showConnection && !reconnecting && (
        <ConnectionSettings
          disabled={connectionLocked}
          onChange={changeConnection}
        />
      )}
      <p className="text-xs break-all">
        {t("mobile.connectedTo", {
          address: backendBase() || window.location.origin,
        })}
      </p>
      <Button
        variant="outline"
        disabled={reconnecting || !!pending}
        onClick={() => void reconnect()}
      >
        {t("mobile.reconnect")}
      </Button>
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
          <p className="text-xs text-muted-foreground">
            {t("mobile.ledgerTimezone", {
              timezone: bootstrap.data!.ledgerTimezone,
            })}
          </p>
          {pending && record === null && quote === null && (
            <Recovery
              pending={pending}
              householdId={b.householdId}
              onResolved={(activity) => success(activity)}
              onForget={() => updatePending(null)}
            />
          )}
          {notice && <p role="status">{t(`mobile.${notice}`)}</p>}
          {receipt && (
            <Card className="p-4">
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
              <Button variant="outline" onClick={() => setDetail(null)}>
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
                className="w-full"
                variant="outline"
                onClick={() =>
                  void client.invalidateQueries({ queryKey: ["mobile"] })
                }
              >
                {t("mobile.reloadLocal")}
              </Button>
            </>
          )}
          {!hiddenNav && (
            <nav className="flex items-center justify-around gap-1 border-t border-border bg-card/95 backdrop-blur">
              <Button
                size="sm"
                variant={tab === "overview" ? "soft" : "ghost"}
                onClick={() => {
                  setDetail(null);
                  setTab("overview");
                }}
              >
                {t("mobile.overview")}
              </Button>
              <Button
                size="sm"
                variant={tab === "accounts" ? "soft" : "ghost"}
                onClick={() => {
                  setDetail(null);
                  setTab("accounts");
                }}
              >
                {t("mobile.accounts")}
              </Button>
              <Button
                aria-label={t("mobile.quickRecord")}
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
                ＋
              </Button>
              <Button
                size="sm"
                variant={tab === "activities" ? "soft" : "ghost"}
                onClick={() => {
                  setDetail(null);
                  setTab("activities");
                }}
              >
                {t("mobile.ledger")}
              </Button>
              <Button
                size="sm"
                variant={tab === "health" ? "soft" : "ghost"}
                onClick={() => {
                  setDetail(null);
                  setTab("health");
                }}
              >
                {t("mobile.health")}
              </Button>
            </nav>
          )}
          {!b.historyStarted && <p>{t("mobile.historyRequired")}</p>}
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

function MetricView({ label, metric }: { label: string; metric: Metric }) {
  const { t } = useTranslation();
  return (
    <Card className="p-4">
      <p className="text-sm text-muted-foreground">{label}</p>
      <p className="text-2xl font-semibold">
        {metric.total ? (
          <MoneyText value={metric.total} />
        ) : (
          <span aria-label={t("mobile.unavailable")}>—</span>
        )}
      </p>
      {!metric.total && metric.knownSubtotal && (
        <p>
          {t("mobile.knownSubtotal")} <MoneyText value={metric.knownSubtotal} />
        </p>
      )}
    </Card>
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
  const { t } = useTranslation();
  const [range, setRange] = useState("30d");
  const query = useQuery({
    queryKey: ["mobile", "overview", range],
    queryFn: () =>
      api<
        Envelope<{
          valuation: { netWorth: Metric; assets: Metric; liabilities: Metric };
          complete: boolean;
          trend: { date: string; valuation: Metric }[];
          recent: Activity[];
          currency: string;
          distribution: { key: string; label: string; amount: string }[];
        }>
      >(`/overview?range=${range}`),
  });
  if (!query.data)
    return (
      <>
        <ErrorMessage error={query.error} />
        <LoadingState label={t("common.loading")} />
      </>
    );
  const o = query.data.data;
  return (
    <div className="space-y-5">
      <Button className="w-full" disabled={!canRefresh} onClick={onMarket}>
        {t("mobile.updateMarket")}
      </Button>
      <ErrorMessage error={query.error} />
      <MetricView label={t("mobile.netWorth")} metric={o.valuation.netWorth} />
      <div className="grid grid-cols-2 gap-3">
        <MetricView label={t("mobile.assets")} metric={o.valuation.assets} />
        <MetricView
          label={t("mobile.liabilities")}
          metric={o.valuation.liabilities}
        />
      </div>
      <p className="text-xs">
        {o.complete ? t("mobile.complete") : t("mobile.partial")} ·{" "}
        {formatTimestamp(query.data.asOf, query.data.ledgerTimezone)}
      </p>
      <Section title={t("mobile.trend")}>
        <NativeSelect
          aria-label={t("mobile.trend")}
          value={range}
          onChange={(e) => setRange(e.target.value)}
        >
          <option value="30d">{t("mobile.month")}</option>
          <option value="1y">{t("mobile.year")}</option>
        </NativeSelect>
        <div className="max-h-64 overflow-auto rounded-xl border border-border">
          <dl className="divide-y divide-border">
            {o.trend.map((p) => (
              <div key={p.date} className="flex justify-between gap-2 p-3">
                <dt>{formatCivilDate(p.date)}</dt>
                <dd>
                  <MoneyText value={p.valuation.total} />
                </dd>
              </div>
            ))}
          </dl>
        </div>
      </Section>
      <Section title={t("mobile.distribution")}>
        {o.distribution.map((d) => (
          <div className="flex justify-between gap-3" key={d.key}>
            <span>{t(`enum.${d.key}`, { defaultValue: d.label })}</span>
            <MoneyText value={{ amount: d.amount, currency: o.currency }} />
          </div>
        ))}
      </Section>
      <Section title={t("mobile.recent")}>
        <ActivityList items={o.recent} onOpen={onActivity} />
      </Section>
    </div>
  );
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
    next: () => setPage({ key, cursor: query.data?.data.next ?? "" }),
    first: () => setPage({ key, cursor: "" }),
  };
}
function Pager({
  next,
  onNext,
  onFirst,
}: {
  next?: string;
  onNext: () => void;
  onFirst: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex gap-2">
      <Button variant="outline" onClick={onFirst}>
        {t("mobile.firstPage")}
      </Button>
      <Button variant="outline" disabled={!next} onClick={onNext}>
        {t("mobile.nextPage")}
      </Button>
    </div>
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
  return (
    <Section title={t("mobile.accounts")}>
      <Input
        aria-label={t("mobile.search")}
        placeholder={t("mobile.search")}
        value={search}
        onChange={(e) => setSearch(e.target.value)}
      />
      <NativeSelect
        aria-label={t("mobile.role")}
        value={role}
        onChange={(e) => setRole(e.target.value)}
      >
        <option value="">{t("mobile.all")}</option>
        <option value="asset">{t("mobile.assets")}</option>
        <option value="liability">{t("mobile.liabilities")}</option>
      </NativeSelect>
      <NativeSelect
        aria-label={t("mobile.accountType")}
        value={type}
        onChange={(e) => setType(e.target.value)}
      >
        <option value="">{t("mobile.all")}</option>
        {[
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
        ].map((k) => (
          <option key={k} value={k}>
            {t(`enum.${k}`)}
          </option>
        ))}
      </NativeSelect>
      <ErrorMessage error={query.error} />
      {query.isPending && <LoadingState label={t("common.loading")} />}
      {query.data?.data.items.map((a) => (
        <Card className="p-4" key={a.id}>
          <Button
            className="h-auto w-full justify-between gap-3 whitespace-normal text-left"
            variant="ghost"
            onClick={() => onOpen(a.id)}
          >
            <span>
              {a.name}
              <small className="block text-muted-foreground">
                {a.institution} · {a.currency}
              </small>
            </span>
            <span>
              <MoneyText value={a.value} />
              {!a.complete && (
                <small className="block">{t("mobile.partial")}</small>
              )}
            </span>
          </Button>
        </Card>
      ))}
      <Pager
        next={query.data?.data.next}
        onNext={query.next}
        onFirst={query.first}
      />
    </Section>
  );
}
function ActivityView({ activity }: { activity: Activity }) {
  const { t } = useTranslation();
  const label = activity.reason || activity.kind;
  return (
    <div className="space-y-2">
      <h3 className="font-semibold">
        {t(`history.reason.${label}`, {
          defaultValue: t(`history.kind.${activity.kind}`, {
            defaultValue: t("mobile.ledger"),
          }),
        })}
      </h3>
      <p className="text-xs">
        {activity.effectiveLocalDate
          ? formatCivilDate(activity.effectiveLocalDate)
          : activity.effectiveAt}
      </p>
      {activity.resulting?.map((r, i) => (
        <p key={i}>
          {r.name} ·{" "}
          {r.amount ? (
            <MoneyText value={{ amount: r.amount, currency: r.currency }} />
          ) : r.quantity ? (
            formatAmount(r.quantity)
          ) : (
            t("mobile.unavailable")
          )}
        </p>
      ))}
      {activity.note && <p className="break-words">{activity.note}</p>}
    </div>
  );
}
function ActivityList({
  items,
  onOpen,
}: {
  items: Activity[];
  onOpen: (id: string) => void;
}) {
  const { t } = useTranslation();
  return items.length ? (
    <div className="space-y-3">
      {items.map((a) => (
        <Card className="p-4" key={a.id}>
          <ActivityView activity={a} />
          <Button variant="link" onClick={() => onOpen(a.id)}>
            {t("mobile.details")}
          </Button>
        </Card>
      ))}
    </div>
  ) : (
    <p>{t("mobile.empty")}</p>
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
  const accounts = usePage<Account>("/accounts");
  const query = usePage<Activity>(
    "/activities",
    `from=${from}&to=${to}&accountId=${account}&kind=${kind}`,
  );
  return (
    <Section title={t("mobile.ledger")}>
      <div className="grid grid-cols-2 gap-3">
        <DatePickerControl
          aria-label={t("mobile.from")}
          value={from}
          onChange={setFrom}
          max={today}
          clearable
        />
        <DatePickerControl
          aria-label={t("mobile.to")}
          value={to}
          onChange={setTo}
          max={today}
          clearable
        />
      </div>
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
      <ErrorMessage error={query.error} />
      {query.data && (
        <ActivityList items={query.data.data.items} onOpen={onOpen} />
      )}
      <Pager
        next={query.data?.data.next}
        onNext={query.next}
        onFirst={query.first}
      />
    </Section>
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
        <LoadingState label={t("common.loading")} />
      </>
    );
  const data = query.data.data as HoldingDetailResponse & AccountDetail;
  if (kind === "activities")
    return <ActivityView activity={query.data.data as Activity} />;
  const valuation = kind === "accounts" ? data : data.valuation;
  return (
    <div className="space-y-4">
      <h2 className="text-xl font-bold">
        {kind === "accounts" ? data.account.name : data.holding.InstrumentName}
      </h2>
      <ErrorMessage error={query.error} />
      {kind === "accounts" && (
        <Button disabled={!canWrite} onClick={onRecord}>
          {t("mobile.quickRecord")}
        </Button>
      )}
      {kind === "holdings" && (
        <>
          <p>
            {t("mobile.quantity")} {formatAmount(data.holding.Quantity)}
          </p>
          <p>
            {t("mobile.quoteValue")}{" "}
            {data.quote
              ? formatQuotePrice(data.quote.unitPrice, data.quote.currency)
              : t("mobile.unavailable")}
          </p>
          <p>
            {t("mobile.cost")} <MoneyText value={data.holding.TotalCost} />
          </p>
          <p>
            {t("mobile.gain")}{" "}
            <GainMoneyText
              value={
                data.holding.UnrealizedGain
                  ? {
                      amount: data.holding.UnrealizedGain.Amount,
                      currency: data.holding.UnrealizedGain.Currency,
                    }
                  : null
              }
            />
          </p>
          <p>
            {t("mobile.realizedGain")}{" "}
            <GainMoneyText
              value={
                data.holding.RealizedGain
                  ? {
                      amount: data.holding.RealizedGain.Amount,
                      currency: data.holding.RealizedGain.Currency,
                    }
                  : null
              }
            />
          </p>
          <Button
            disabled={!canRefresh}
            onClick={() =>
              onMarket({
                mode: "latest",
                scope: "instrument",
                instrumentId: data.holding.InstrumentID,
              })
            }
          >
            {t("mobile.updateMarket")}
          </Button>
        </>
      )}
      {(valuation.components ?? []).map((c: Component, i: number) => (
        <Card className="p-4 space-y-2" key={i}>
          <h3>{c.instrumentName || t("mobile.value")}</h3>
          <MoneyText
            value={
              c.available
                ? { amount: c.nativeAmount, currency: c.nativeCurrency }
                : null
            }
          />
          {c.priceEvidence && (
            <>
              <p>
                {t("mobile.effectiveDate")}{" "}
                {formatQuoteAsOf(
                  {
                    quotedAt: c.priceEvidence.quotedAt,
                    effectiveDate: c.priceEvidence.effectiveDate ?? undefined,
                    timestampBasis: c.priceEvidence.effectiveDate
                      ? "date_label"
                      : undefined,
                  },
                  query.data.ledgerTimezone,
                )}
              </p>
              <p>
                {t("mobile.source")}{" "}
                {t(`portfolio.source.${c.priceEvidence.source}`, {
                  defaultValue: t("mobile.unavailable"),
                })}
              </p>
              {c.priceEvidence.fetchedAt && (
                <p>
                  {t("mobile.fetchedAt")}{" "}
                  {formatTimestamp(
                    c.priceEvidence.fetchedAt,
                    query.data.ledgerTimezone,
                  )}
                </p>
              )}
            </>
          )}
        </Card>
      ))}
      {kind === "accounts" && (
        <Section title={t("mobile.holdings")}>
          <ErrorMessage error={holdings.error} />
          {holdings.data?.data.items.map((h) => (
            <Card className="p-4 space-y-2" key={h.id}>
              <Button variant="link" onClick={() => onHolding(h.id)}>
                {h.name} {h.symbol}
              </Button>
              <p>
                {t("mobile.quantity")} {formatAmount(h.quantity)}
              </p>
              <p>
                {t("mobile.value")} <MoneyText value={h.value} />
              </p>
              <p>
                {t("mobile.cost")} <MoneyText value={h.cost} />
              </p>
              <p>
                {t("mobile.gain")}{" "}
                {h.gain ? (
                  <ChangeText
                    direction={
                      h.gain.amount.startsWith("-")
                        ? "down"
                        : /^0(?:\.0*)?$/.test(h.gain.amount)
                          ? "flat"
                          : "up"
                    }
                  >
                    {formatSignedMoney(h.gain.amount, h.gain.currency)}
                  </ChangeText>
                ) : (
                  t("mobile.unavailable")
                )}
              </p>
            </Card>
          ))}
          <Pager
            next={holdings.data?.data.next}
            onNext={holdings.next}
            onFirst={holdings.first}
          />
        </Section>
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
