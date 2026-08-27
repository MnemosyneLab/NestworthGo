import { useState } from "react";
import { useTranslation } from "react-i18next";
import { RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { PageHeader } from "@/components/layout/PageHeader";
import { EmptyState, ErrorState } from "@/components/layout/PageState";
import { useRefreshAll, useRefreshRequiredFX } from "@/queries/marketdata";
import { useCurrentFXQuote, useCurrentInstrumentQuote, useInstruments } from "@/queries/investments";
import { displayEnum, displayError } from "@/lib/display";
import { formatAmount } from "@/lib/money";
import type { RefreshResultDTO, RefreshTargetResultDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata/models";

const STATUS_VARIANT: Record<string, "success" | "secondary" | "destructive" | "warning"> = {
  fetched: "success",
  cached: "secondary",
  skipped: "secondary",
  failed: "destructive",
  rate_limited: "warning",
};

const INSTRUMENT_KEY_PREFIX = "instrument:";
const FX_KEY_PREFIX = "fx:";

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

function formatQuotedAt(value: string, language: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat(language, { dateStyle: "medium", timeStyle: "short" }).format(parsed);
}

function InstrumentRefreshRow({
  item,
  name,
}: {
  item: RefreshTargetResultDTO;
  name: string;
}) {
  const { t, i18n } = useTranslation();
  const instrumentId = item.targetKey.slice(INSTRUMENT_KEY_PREFIX.length);
  const quote = useCurrentInstrumentQuote(instrumentId);

  return (
    <li className="flex flex-col gap-1 rounded-md border border-border px-3 py-3 text-sm">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="font-medium text-foreground">{name}</span>
        <Badge variant={STATUS_VARIANT[item.status] ?? "secondary"}>{displayEnum(t, "marketData.status", item.status)}</Badge>
      </div>
      <p className="text-muted-foreground">
        {quote.data ? t("marketData.latestPrice", { value: formatAmount(quote.data.unitPrice, quote.data.currency) }) : t("marketData.noQuoteYet")}
      </p>
      {quote.data && <p className="text-xs text-muted-foreground">{t("marketData.quotedAsOf", { time: formatQuotedAt(quote.data.quotedAt, i18n.language) })}</p>}
      {item.status === "skipped" && <p className="text-xs text-muted-foreground">{skipReasonText(t, item, "instrument")}</p>}
    </li>
  );
}

function FxRefreshRow({ item }: { item: RefreshTargetResultDTO }) {
  const { t, i18n } = useTranslation();
  const pair = item.targetKey.slice(FX_KEY_PREFIX.length);
  const [currencyA, currencyB] = pair.split("/");
  const quote = useCurrentFXQuote(currencyA, currencyB);

  return (
    <li className="flex flex-col gap-1 rounded-md border border-border px-3 py-3 text-sm">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="font-medium text-foreground">{pair}</span>
        <Badge variant={STATUS_VARIANT[item.status] ?? "secondary"}>{displayEnum(t, "marketData.status", item.status)}</Badge>
      </div>
      <p className="text-muted-foreground">
        {quote.data
          ? t("marketData.latestRate", { base: quote.data.baseCurrency, rate: formatAmount(quote.data.rate), quote: quote.data.quoteCurrency })
          : t("marketData.noQuoteYet")}
      </p>
      {quote.data && <p className="text-xs text-muted-foreground">{t("marketData.quotedAsOf", { time: formatQuotedAt(quote.data.quotedAt, i18n.language) })}</p>}
      {item.status === "skipped" && <p className="text-xs text-muted-foreground">{skipReasonText(t, item, "fx")}</p>}
    </li>
  );
}

function RefreshResults({ result, refreshedAt }: { result: RefreshResultDTO; refreshedAt?: Date }) {
  const { t, i18n } = useTranslation();
  const instruments = useInstruments();
  const instrumentNameById = new Map((instruments.data ?? []).map((instrument) => [instrument.id, instrument.name]));
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
            time: new Intl.DateTimeFormat(i18n.language, { dateStyle: "medium", timeStyle: "short" }).format(refreshedAt),
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
                name={instrumentNameById.get(item.targetKey.slice(INSTRUMENT_KEY_PREFIX.length)) ?? t("portfolio.unknownInstrument")}
              />
            ) : item.kind === "fx" ? (
              <FxRefreshRow key={`${item.kind}-${item.targetKey}`} item={item} />
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

/** MarketDataPage makes provider refresh explicit and keeps the provider's
 * technical target identifiers out of the user-facing result list. */
export function MarketDataPage() {
  const { t } = useTranslation();
  const refreshAll = useRefreshAll();
  const refreshRequiredFX = useRefreshRequiredFX();
  const [lastRefresh, setLastRefresh] = useState<"all" | "fx" | null>(null);
  const [result, setResult] = useState<RefreshResultDTO | undefined>();
  const [lastRefreshAt, setLastRefreshAt] = useState<Date | undefined>();
  const activeRefresh = lastRefresh === "fx" ? refreshRequiredFX : refreshAll;

  const runRefreshAll = () => {
    setLastRefresh("all");
    setResult(undefined);
    setLastRefreshAt(undefined);
    refreshAll.mutate(undefined, {
      onSuccess: (next) => {
        setResult(next);
        setLastRefreshAt(new Date());
      },
    });
  };

  const runRefreshFX = () => {
    setLastRefresh("fx");
    setResult(undefined);
    setLastRefreshAt(undefined);
    refreshRequiredFX.mutate(undefined, {
      onSuccess: (next) => {
        setResult(next);
        setLastRefreshAt(new Date());
      },
    });
  };

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t("nav.marketData")} description={t("marketData.description")} />
      <div className="flex flex-wrap gap-2">
        <Button onClick={runRefreshAll} disabled={refreshAll.isPending || refreshRequiredFX.isPending}>
          <RefreshCw className="size-4" aria-hidden="true" /> {refreshAll.isPending ? t("marketData.refreshing") : t("marketData.refreshAll")}
        </Button>
        <Button variant="outline" onClick={runRefreshFX} disabled={refreshAll.isPending || refreshRequiredFX.isPending}>
          <RefreshCw className="size-4" aria-hidden="true" /> {refreshRequiredFX.isPending ? t("marketData.refreshing") : t("marketData.refreshFX")}
        </Button>
      </div>

      {activeRefresh.isError && (
        <ErrorState
          title={t("marketData.loadError")}
          description={displayError(activeRefresh.error, t("ui.state.errorDescription"))}
          onRetry={lastRefresh === "fx" ? runRefreshFX : runRefreshAll}
          retryLabel={t("common.retryAction")}
        />
      )}

      {!activeRefresh.isError && result && <RefreshResults result={result} refreshedAt={lastRefreshAt} />}
      {!activeRefresh.isError && !result && (
        <p className="rounded-md border border-border bg-muted/30 p-3 text-sm text-muted-foreground">{t("marketData.explicitNotice")}</p>
      )}
    </div>
  );
}
