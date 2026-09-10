import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { DatePicker } from "@/components/ui/date-picker";
import { Badge } from "@/components/ui/badge";
import { NativeSelect } from "@/components/ui/select";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { PageIntro } from "@/components/layout/PageHeader";
import { PageChrome } from "@/components/layout/PageChrome";
import { EmptyState, ErrorState } from "@/components/layout/PageState";
import { useRefreshAll, useRefreshMissingOrStale } from "@/queries/marketdata";
import {
  useCurrentFXQuote,
  useCurrentInstrumentQuote,
  useFXPreferences,
  useInstruments,
  useAppendManualFXQuote,
  useSetFXPreference,
  useSetInstrumentQuoteSource,
} from "@/queries/investments";
import { useSettings, useSupportedCurrencies } from "@/queries/settings";
import { useOverview } from "@/queries/portfolio";
import { displayEnum, displayError } from "@/lib/display";
import { formatAmount } from "@/lib/money";
import { formatTimestamp } from "@/lib/time";
import { sortFxPairs } from "@/lib/groupByInstrumentType";
import type { RefreshResultDTO, RefreshTargetResultDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata/models";
import type { FXPreferenceDTO, InstrumentDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { EntityIcon } from "@/components/icons/EntityIcon";
import { QuoteHistorySheet, type QuoteHistoryTarget } from "@/features/marketdata/QuoteHistorySheet";
import { InstrumentManagement } from "@/features/investments/InstrumentManagement";
import { MarketDataSyncBar, useMarketDataSyncActions } from "@/features/marketdata/MarketDataSyncBar";
import { DataHealthIndicator } from "@/features/data-health/DataHealthIndicator";

const STATUS_VARIANT: Record<string, "success" | "secondary" | "destructive" | "warning"> = {
  fetched: "success",
  cached: "secondary",
  skipped: "secondary",
  failed: "destructive",
  rate_limited: "warning",
};

const INSTRUMENT_KEY_PREFIX = "instrument:";
const FX_KEY_PREFIX = "fx:";

type FxPair = { currencyA: string; currencyB: string };

function fxPairKey(currencyA: string, currencyB: string): string {
  return [currencyA, currencyB].map((value) => value.toUpperCase()).sort().join("/");
}

function normalizeFxPair(currencyA: string, currencyB: string): FxPair | undefined {
  const currencies = [currencyA.trim().toUpperCase(), currencyB.trim().toUpperCase()].filter(Boolean).sort();
  if (currencies.length !== 2 || currencies[0] === currencies[1]) {
    return undefined;
  }
  return { currencyA: currencies[0], currencyB: currencies[1] };
}

function pairFromTargetKey(targetKey: string): FxPair | undefined {
  const [currencyA, currencyB] = targetKey.slice(FX_KEY_PREFIX.length).split("/");
  return currencyA && currencyB ? normalizeFxPair(currencyA, currencyB) : undefined;
}

function collectFxPairs(
  preferences: FXPreferenceDTO[] | null | undefined,
  missingInputs: Array<{ kind: string; baseCurrency?: string; quoteCurrency?: string }> | null | undefined,
  result: RefreshResultDTO | undefined,
): FxPair[] {
  const pairs = new Map<string, FxPair>();
  const add = (currencyA?: string, currencyB?: string) => {
    if (!currencyA || !currencyB) {
      return;
    }
    const pair = normalizeFxPair(currencyA, currencyB);
    if (pair) {
      pairs.set(fxPairKey(pair.currencyA, pair.currencyB), pair);
    }
  };

  (preferences ?? []).forEach((preference) => add(preference.currencyA, preference.currencyB));
  (missingInputs ?? [])
    .filter((input) => input.kind === "fx_rate")
    .forEach((input) => add(input.baseCurrency, input.quoteCurrency));
  (result?.items ?? [])
    .filter((item) => item.kind === "fx" && item.targetKey.startsWith(FX_KEY_PREFIX))
    .forEach((item) => {
      const pair = pairFromTargetKey(item.targetKey);
      if (pair) {
        pairs.set(fxPairKey(pair.currencyA, pair.currencyB), pair);
      }
    });

  return [...pairs.values()].sort((left, right) => fxPairKey(left.currencyA, left.currencyB).localeCompare(fxPairKey(right.currencyA, right.currencyB)));
}

function preferenceForPair(preferences: FXPreferenceDTO[] | null | undefined, pair: FxPair): FXPreferenceDTO | undefined {
  const key = fxPairKey(pair.currencyA, pair.currencyB);
  return (preferences ?? []).find((preference) => fxPairKey(preference.currencyA, preference.currencyB) === key);
}

function fxSourceLabel(t: (key: string, options?: Record<string, unknown>) => string, preference: FXPreferenceDTO | undefined, providerKey?: string): string {
  if (!preference) {
    return t("marketData.sourceMissing");
  }
  return preference.sourceKind === "provider"
    ? providerKey
      ? t(`settings.provider.${providerKey}`, { defaultValue: providerKey })
      : t("settings.providers.fxProvider")
    : displayEnum(t, "portfolio", preference.sourceKind);
}

function skipReasonText(t: (key: string, options?: Record<string, unknown>) => string, item: RefreshTargetResultDTO, kind: "instrument" | "fx"): string | undefined {
  if (item.status !== "skipped") {
    return undefined;
  }
  if (item.errorCode === "unavailable" && kind === "instrument") {
    return t("marketData.skippedManual");
  }
  if (item.errorCode) {
    return displayEnum(t, "marketData.skipReason", item.errorCode);
  }
  return t("marketData.skipped");
}

function formatQuotedAt(value: string, language: string, timezone?: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return formatTimestamp(parsed, timezone, language);
}

function InstrumentRefreshRow({
  item,
  instrument,
}: {
  item: RefreshTargetResultDTO;
  instrument?: InstrumentDTO;
}) {
  const { t, i18n } = useTranslation();
  const settings = useSettings();
  const instrumentId = item.targetKey.slice(INSTRUMENT_KEY_PREFIX.length);
  const quote = useCurrentInstrumentQuote(instrumentId);

  return (
    <li className="flex flex-col gap-1 rounded-md border border-border px-3 py-3 text-sm">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="flex items-center gap-2 font-medium text-foreground">
          <EntityIcon iconKey={instrument?.iconKey} kind="instrument" />
          {instrument?.name ?? t("portfolio.unknownInstrument")}
        </span>
        <Badge variant={STATUS_VARIANT[item.status] ?? "secondary"}>{displayEnum(t, "marketData.status", item.status)}</Badge>
      </div>
      <p className="text-muted-foreground">
        {quote.data ? t("marketData.latestPrice", { value: formatAmount(quote.data.unitPrice, quote.data.currency) }) : t("marketData.noQuoteYet")}
      </p>
      {quote.data && <p className="text-xs text-muted-foreground">{t("marketData.quotedAsOf", { time: formatQuotedAt(quote.data.quotedAt, i18n.language, settings.data?.timezone) })}</p>}
      {item.status === "skipped" && <p className="text-xs text-muted-foreground">{skipReasonText(t, item, "instrument")}</p>}
    </li>
  );
}

function FxRefreshRow({
  item,
  configured,
  onConfigure,
  isConfiguring,
}: {
  item: RefreshTargetResultDTO;
  configured: boolean;
  onConfigure: () => void;
  isConfiguring: boolean;
}) {
  const { t, i18n } = useTranslation();
  const settings = useSettings();
  const pair = item.targetKey.slice(FX_KEY_PREFIX.length);
  const [currencyA, currencyB] = pair.split("/");
  const quote = useCurrentFXQuote(currencyA, currencyB);

  return (
    <li className="grid grid-cols-[minmax(0,1fr)_auto] gap-x-3 gap-y-1 rounded-md border border-border px-3 py-3 text-sm">
      <div className="col-span-2 row-start-1 flex min-w-0 items-center justify-between gap-2">
        <span className="font-medium text-foreground">{pair}</span>
        <span className="flex shrink-0 items-center gap-2">
          <Badge variant={STATUS_VARIANT[item.status] ?? "secondary"}>{displayEnum(t, "marketData.status", item.status)}</Badge>
          {!configured && <Button variant="outline" size="sm" onClick={onConfigure} disabled={isConfiguring}>
            {isConfiguring ? t("marketData.configuringFX") : t("marketData.configureFX")}
          </Button>}
        </span>
      </div>
      <div className="col-span-2 row-start-2 flex flex-wrap items-center gap-x-3 gap-y-1">
        <p className="text-muted-foreground">
          {quote.data
            ? t("marketData.latestRate", { base: quote.data.baseCurrency, rate: formatAmount(quote.data.rate), quote: quote.data.quoteCurrency })
            : t("marketData.noQuoteYet")}
        </p>
        {quote.data && <p className="text-xs text-muted-foreground">{t("marketData.quotedAsOf", { time: formatQuotedAt(quote.data.quotedAt, i18n.language, settings.data?.timezone) })}</p>}
        {item.status === "skipped" && <p className="text-xs text-muted-foreground">{skipReasonText(t, item, "fx")}</p>}
      </div>
    </li>
  );
}

function RefreshResults({
  result,
  refreshedAt,
  instruments,
  fxPreferences,
  onConfigureFX,
  configuringPair,
}: {
  result: RefreshResultDTO;
  refreshedAt?: Date;
  instruments: InstrumentDTO[];
  fxPreferences: FXPreferenceDTO[];
  onConfigureFX: (pair: FxPair) => void;
  configuringPair?: string;
}) {
  const { t, i18n } = useTranslation();
  const settings = useSettings();
  const instrumentById = new Map(instruments.map((instrument) => [instrument.id, instrument]));
  const items = result.items ?? [];
  const updated = items.filter((item) => item.status === "fetched").length;
  const reused = items.filter((item) => item.status === "cached").length;
  const skipped = items.filter((item) => item.status === "skipped").length;
  const failed = items.filter((item) => item.status === "failed" || item.status === "rate_limited").length;

  return (
    <div className="flex flex-col gap-3" aria-live="polite">
      <p className="text-sm text-muted-foreground">
        {t("marketData.refreshSummary", { count: items.length, updated, reused, skipped, failed })}
      </p>
      {refreshedAt && (
        <p className="text-xs text-muted-foreground">
          {t("marketData.refreshedAt", {
            time: formatTimestamp(refreshedAt, settings.data?.timezone, i18n.language),
          })}
        </p>
      )}
      {result.rateLimited && <p className="rounded-md border border-warning/40 bg-warning/10 p-3 text-sm text-warning-foreground">{t("marketData.rateLimited")}</p>}
      {items.length === 0 ? (
        <EmptyState title={t("marketData.noTargets")} description={t("marketData.description")} />
      ) : (
        <ul className="flex flex-col gap-2" data-testid="refresh-results">
          {items.map((item) =>
            item.kind === "instrument" ? (
              <InstrumentRefreshRow
                key={`${item.kind}-${item.targetKey}`}
                item={item}
                instrument={instrumentById.get(item.targetKey.slice(INSTRUMENT_KEY_PREFIX.length))}
              />
            ) : item.kind === "fx" ? (
              <FxRefreshRow
                key={`${item.kind}-${item.targetKey}`}
                item={item}
                configured={Boolean(
                  pairFromTargetKey(item.targetKey) && preferenceForPair(fxPreferences, pairFromTargetKey(item.targetKey)!)?.sourceKind === "provider",
                )}
                onConfigure={() => {
                  const pair = pairFromTargetKey(item.targetKey);
                  if (pair) {
                    onConfigureFX(pair);
                  }
                }}
                isConfiguring={Boolean(pairFromTargetKey(item.targetKey) && configuringPair === fxPairKey(pairFromTargetKey(item.targetKey)!.currencyA, pairFromTargetKey(item.targetKey)!.currencyB))}
              />
            ) : (
              <li key={`${item.kind}-${item.targetKey}`} className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border px-3 py-3 text-sm">
                <span>{displayEnum(t, "marketData.target", item.kind)}</span>
                <Badge variant={STATUS_VARIANT[item.status] ?? "secondary"}>{displayEnum(t, "marketData.status", item.status)}</Badge>
              </li>
            ),
          )}
        </ul>
      )}
    </div>
  );
}

function SavedFXRow({
  pair,
  preference,
  onConfigure,
  isConfiguring,
  onViewHistory,
}: {
  pair: FxPair;
  preference?: FXPreferenceDTO;
  onConfigure: (source?: string) => void;
  isConfiguring: boolean;
  onViewHistory: () => void;
}) {
  const { t, i18n } = useTranslation();
  const settings = useSettings();
  const quote = useCurrentFXQuote(pair.currencyA, pair.currencyB);

  return (
    <li className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-2 rounded-md border border-border px-3 py-3 text-sm lg:grid-cols-[minmax(0,1fr)_minmax(0,auto)_auto]" data-testid={`saved-fx-${fxPairKey(pair.currencyA, pair.currencyB)}`}>
      <span className="flex min-w-0 flex-wrap items-center gap-2">
        <span className="font-medium">{pair.currencyA}/{pair.currencyB}</span>
        <Badge variant={!preference ? "warning" : "secondary"}>{fxSourceLabel(t, preference, settings.data?.fxProvider)}</Badge>
      </span>
      <span className="col-span-2 row-start-2 flex min-w-0 flex-wrap items-center justify-end gap-x-3 gap-y-1 text-right lg:col-span-1 lg:col-start-2 lg:row-start-1">
        {quote.isLoading ? (
          <span className="text-muted-foreground">{t("portfolio.priceLoading")}</span>
        ) : quote.data ? (
          <>
            <span className="font-medium">
              {t("marketData.latestRate", {
                base: quote.data.baseCurrency,
                rate: formatAmount(quote.data.rate),
                quote: quote.data.quoteCurrency,
              })}
            </span>
            <span className="text-xs text-muted-foreground">
              {t("marketData.quotedAsOf", { time: formatQuotedAt(quote.data.quotedAt, i18n.language, settings.data?.timezone) })}
            </span>
            <span className="flex flex-wrap items-center justify-end gap-1 text-xs text-muted-foreground">
              {quote.data.delayed ? <Badge variant="warning">{t("charts.delayed")}</Badge> : null}
              {quote.data.sourceKind ? <span>{displayEnum(t, "portfolio", quote.data.sourceKind)}</span> : null}
              {quote.data.sourceKey ? <span>· {quote.data.sourceKey}</span> : null}
            </span>
          </>
        ) : (
          <span className="text-muted-foreground">{t("marketData.noQuoteYet")}</span>
        )}
      </span>
      <span className="col-start-2 row-start-1 flex shrink-0 items-center justify-end gap-2 lg:col-start-3">
        {preference ? (
          <NativeSelect
            aria-label={t("marketData.sourceMissing")}
            value={preference.sourceKind}
            disabled={isConfiguring}
            onChange={(event) => onConfigure(event.target.value)}
          >
            <option value="provider">{t("portfolio.provider")}</option>
            <option value="manual">{t("portfolio.manual")}</option>
          </NativeSelect>
        ) : (
          <Button variant="outline" size="sm" onClick={() => onConfigure("provider")} disabled={isConfiguring}>
            {isConfiguring ? t("marketData.configuringFX") : t("marketData.configureFX")}
          </Button>
        )}
        <Button type="button" variant="outline" size="sm" onClick={onViewHistory}>
          {t("charts.viewHistory")}
        </Button>
      </span>
    </li>
  );
}

function ManualFXQuoteForm() {
  const { t } = useTranslation();
  const currencies = useSupportedCurrencies();
  const appendQuote = useAppendManualFXQuote();
  const options = currencies.data ?? [];
  const [baseCurrency, setBaseCurrency] = useState("");
  const [quoteCurrency, setQuoteCurrency] = useState("");
  const [rate, setRate] = useState("");
  const [effectiveDate, setEffectiveDate] = useState("");
  const selectedBaseCurrency = baseCurrency || options[0] || "";
  const selectedQuoteCurrency = quoteCurrency || options[1] || options[0] || "";
  const invalidPair = !selectedBaseCurrency || !selectedQuoteCurrency || selectedBaseCurrency === selectedQuoteCurrency;

  const submit = () => {
    if (invalidPair || !rate.trim()) return;
    appendQuote.mutate({
      baseCurrency: selectedBaseCurrency,
      quoteCurrency: selectedQuoteCurrency,
      rate: rate.trim(),
      quotedAt: effectiveDate ? new Date(`${effectiveDate}T00:00:00`).toISOString() : "",
    });
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("marketData.manualFXTitle")}</CardTitle>
        <p className="text-sm text-muted-foreground">{t("marketData.manualFXDescription")}</p>
      </CardHeader>
      <CardContent className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="manual-fx-base">{t("marketData.baseCurrency")}</Label>
          <NativeSelect id="manual-fx-base" value={selectedBaseCurrency} onChange={(event) => setBaseCurrency(event.target.value)}>
            {options.map((currency) => <option key={currency} value={currency}>{currency}</option>)}
          </NativeSelect>
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="manual-fx-quote">{t("marketData.quoteCurrency")}</Label>
          <NativeSelect id="manual-fx-quote" value={selectedQuoteCurrency} onChange={(event) => setQuoteCurrency(event.target.value)}>
            {options.map((currency) => <option key={currency} value={currency}>{currency}</option>)}
          </NativeSelect>
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="manual-fx-rate">{t("marketData.rate")}</Label>
          <Input id="manual-fx-rate" inputMode="decimal" value={rate} onChange={(event) => setRate(event.target.value)} />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="manual-fx-date">{t("marketData.effectiveDate")}</Label>
          <DatePicker id="manual-fx-date" value={effectiveDate} onChange={setEffectiveDate} />
        </div>
        {invalidPair && <p className="text-sm text-destructive sm:col-span-2 lg:col-span-4">{t("marketData.invalidPair")}</p>}
        {appendQuote.isError && <p role="alert" className="text-sm text-destructive sm:col-span-2 lg:col-span-4">{displayError(appendQuote.error, t("marketData.loadError"))}</p>}
        <Button type="button" onClick={submit} disabled={appendQuote.isPending || invalidPair || !rate.trim()} className="sm:col-span-2 lg:col-span-4 lg:justify-self-start">
          {appendQuote.isPending ? t("common.pending") : t("marketData.saveManualFX")}
        </Button>
      </CardContent>
    </Card>
  );
}

function SavedFXRates({
  fxPairs,
  fxPreferences,
  onConfigureFX,
  configuringPair,
  onViewHistory,
}: {
  fxPairs: FxPair[];
  fxPreferences: FXPreferenceDTO[];
  onConfigureFX: (pair: FxPair, source?: string) => void;
  configuringPair?: string;
  onViewHistory: (target: QuoteHistoryTarget) => void;
}) {
  const { t } = useTranslation();
  const sortedFxPairs = sortFxPairs(fxPairs);

  return (
    <section className="flex flex-col gap-3" aria-labelledby="saved-fx-heading" data-testid="saved-market-data">
      <div>
        <h2 id="saved-fx-heading" className="text-lg font-semibold">{t("marketData.fx")}</h2>
        <p className="text-sm text-muted-foreground">{t("marketData.fxDailyReference")}</p>
      </div>
      {sortedFxPairs.length === 0 ? (
        <p className="rounded-md border border-border bg-muted/30 p-3 text-sm text-muted-foreground">{t("marketData.noSavedData")}</p>
      ) : (
        <section className="flex flex-col gap-2" data-testid="market-data-group-fx">
          <ul className="flex flex-col gap-2">
            {sortedFxPairs.map((pair) => {
              const key = fxPairKey(pair.currencyA, pair.currencyB);
              return (
                <SavedFXRow
                  key={key}
                  pair={pair}
                  preference={preferenceForPair(fxPreferences, pair)}
                  onConfigure={(source) => onConfigureFX(pair, source)}
                  isConfiguring={configuringPair === key}
                  onViewHistory={() => onViewHistory({ kind: "fx", currencyA: pair.currencyA, currencyB: pair.currencyB })}
                />
              );
            })}
          </ul>
        </section>
      )}
      <ManualFXQuoteForm />
    </section>
  );
}

/** MarketDataPage merges instrument management and FX rates onto one
 * destination, with Sync Data progress restored from the in-process job. */
export function MarketDataPage({ onOpenDataHealth }: { onOpenDataHealth?: () => void } = {}) {
  const { t } = useTranslation();
  const instruments = useInstruments();
  const fxPreferences = useFXPreferences();
  const overview = useOverview();
  const refreshAll = useRefreshAll();
  const refreshMissingOrStale = useRefreshMissingOrStale();
  const setFXPreference = useSetFXPreference();
  const setInstrumentQuoteSource = useSetInstrumentQuoteSource();
  const sync = useMarketDataSyncActions();
  const [tab, setTab] = useState("instruments");
  const [lastRefresh, setLastRefresh] = useState<"all" | "missing" | null>(null);
  const [result, setResult] = useState<RefreshResultDTO | undefined>();
  const [lastRefreshAt, setLastRefreshAt] = useState<Date | undefined>();
  const [configuringPair, setConfiguringPair] = useState<string | undefined>();
  const [configuringInstrument, setConfiguringInstrument] = useState<string | undefined>();
  const [historyTarget, setHistoryTarget] = useState<QuoteHistoryTarget | null>(null);
  const activeRefresh = lastRefresh === "missing" ? refreshMissingOrStale : refreshAll;
  const latestRefreshing = refreshAll.refreshing || refreshMissingOrStale.refreshing;

  const runRefreshAll = () => {
    setLastRefresh("all");
    setResult(undefined);
    setLastRefreshAt(undefined);
    refreshAll.mutate(undefined, {
      onSuccess: (next) => {
        if (next) {
          setResult(next);
          setLastRefreshAt(new Date());
        }
      },
    });
  };

  const runRefreshMissingOrStale = () => {
    setLastRefresh("missing");
    setResult(undefined);
    setLastRefreshAt(undefined);
    refreshMissingOrStale.mutate(undefined, {
      onSuccess: (next) => {
        if (next) {
          setResult(next);
          setLastRefreshAt(new Date());
        }
      },
    });
  };

  const configureInstrument = (instrument: InstrumentDTO, source: string) => {
    setConfiguringInstrument(instrument.id);
    setInstrumentQuoteSource.mutate(
      { instrumentId: instrument.id, source },
      {
        onSuccess: () => setConfiguringInstrument(undefined),
        onError: (error) => {
          setConfiguringInstrument(undefined);
          toast.error(displayError(error, t("marketData.loadError")));
        },
      },
    );
  };

  const configureFX = (pair: FxPair, source = "provider") => {
    const key = fxPairKey(pair.currencyA, pair.currencyB);
    setConfiguringPair(key);
    setFXPreference.mutate(
      { currencyA: pair.currencyA, currencyB: pair.currencyB, source },
      {
        onSuccess: () => {
          setConfiguringPair(undefined);
          if (source === "provider") {
            runRefreshMissingOrStale();
          }
        },
        onError: (error) => {
          setConfiguringPair(undefined);
          toast.error(displayError(error, t("marketData.loadError")));
        },
      },
    );
  };

  const fxPairs = collectFxPairs(fxPreferences.data, overview.data?.missingInputs, result);
  const instrumentList = instruments.data ?? [];
  const preferenceList = fxPreferences.data ?? [];

  return (
    <div className="flex flex-col gap-6">
      <PageChrome pageId="market-data" title={t("nav.marketData")} />
      <PageIntro
        description={t("marketData.description")}
        status={<DataHealthIndicator onOpen={onOpenDataHealth} />}
      />
      <MarketDataSyncBar
        latestRefreshing={latestRefreshing || setFXPreference.isPending || setInstrumentQuoteSource.isPending}
        onRefreshMissing={runRefreshMissingOrStale}
        onRefreshAll={runRefreshAll}
        onCancelLatest={activeRefresh.cancel}
        lastUpdatedAt={lastRefreshAt}
      />

      {activeRefresh.operationStatus === "cancelled" && <p role="status" className="text-sm text-muted-foreground">{t("marketData.refreshCancelled")}</p>}

      {activeRefresh.isError && (
        <ErrorState
          title={t("marketData.loadError")}
          description={displayError(activeRefresh.error, t("ui.state.errorDescription"))}
          onRetry={lastRefresh === "missing" ? runRefreshMissingOrStale : runRefreshAll}
          retryLabel={t("common.retryAction")}
        />
      )}

      {!activeRefresh.isError && (instruments.isError || fxPreferences.isError) && (
        <ErrorState
          title={t("marketData.loadError")}
          description={t("ui.state.errorDescription")}
          onRetry={() => {
            void instruments.refetch();
            void fxPreferences.refetch();
          }}
          retryLabel={t("common.retryAction")}
        />
      )}
      {!activeRefresh.isError && !instruments.isError && !fxPreferences.isError && (
        <Tabs value={tab} onValueChange={setTab}>
          <TabsList>
            <TabsTrigger value="instruments">{t("marketData.instruments")}</TabsTrigger>
            <TabsTrigger value="fx">{t("marketData.fx")}</TabsTrigger>
          </TabsList>
          <TabsContent value="instruments">
            <InstrumentManagement
              pageId="market-data"
              active={tab === "instruments"}
              enableSearch
              showSource
              groupTestIdPrefix="market-data-group"
              onConfigureSource={configureInstrument}
              configuringInstrumentId={configuringInstrument}
              onViewHistory={(instrument) => setHistoryTarget({ kind: "instrument", instrument })}
              onSyncInstrument={sync.syncInstrument}
              syncingInstrumentId={sync.syncingInstrumentId}
            />
          </TabsContent>
          <TabsContent value="fx">
            <SavedFXRates
              fxPairs={fxPairs}
              fxPreferences={preferenceList}
              onConfigureFX={configureFX}
              configuringPair={configuringPair}
              onViewHistory={setHistoryTarget}
            />
          </TabsContent>
        </Tabs>
      )}
      {historyTarget ? <QuoteHistorySheet target={historyTarget} onClose={() => setHistoryTarget(null)} /> : null}
      {!activeRefresh.isError && result && (
        <RefreshResults
          result={result}
          refreshedAt={lastRefreshAt}
          instruments={instrumentList}
          fxPreferences={preferenceList}
          onConfigureFX={configureFX}
          configuringPair={configuringPair}
        />
      )}
      {!activeRefresh.isError && !result && (
        <p className="rounded-md border border-border bg-muted/30 p-3 text-sm text-muted-foreground">{t("marketData.explicitNotice")}</p>
      )}
    </div>
  );
}
