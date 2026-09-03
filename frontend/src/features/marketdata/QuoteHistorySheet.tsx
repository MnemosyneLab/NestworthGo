import { useState } from "react";
import { useTranslation } from "react-i18next";
import { TrendChart } from "@/components/charts/TrendChart";
import { RangeToggle, SourceFilterToggle } from "@/components/charts/RangeToggle";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { ErrorState, EmptyState, LoadingState } from "@/components/layout/PageState";
import { useInstrumentQuoteSeries, useFXQuoteSeries } from "@/queries/investments";
import { useCatalog } from "@/queries/catalog";
import { useSettings } from "@/queries/settings";
import { displayEnum } from "@/lib/display";
import { formatAmount } from "@/lib/money";
import { formatTimestamp } from "@/lib/time";
import { chartTheme } from "@/components/charts/chartTheme";
import type { InstrumentDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

export type QuoteHistoryTarget =
  | { kind: "instrument"; instrument: InstrumentDTO }
  | { kind: "fx"; currencyA: string; currencyB: string };

function formatQuotedAt(value: string, language: string, timezone?: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return formatTimestamp(parsed, timezone, language);
}

export function QuoteHistorySheet({
  target,
  onClose,
}: {
  target: QuoteHistoryTarget;
  onClose: () => void;
}) {
  const { t, i18n } = useTranslation();
  const catalog = useCatalog();
  const settings = useSettings();
  const ranges = catalog.data?.trendRanges ?? ["30d", "ytd", "1y", "all"];
  const [range, setRange] = useState("30d");
  const [sourceFilter, setSourceFilter] = useState("all");
  const [swapped, setSwapped] = useState(false);
  const instrumentId = target.kind === "instrument" ? target.instrument.id : "";
  const fxBase = target.kind === "fx" ? (swapped ? target.currencyB : target.currencyA) : "";
  const fxQuote = target.kind === "fx" ? (swapped ? target.currencyA : target.currencyB) : "";
  const instrumentSeries = useInstrumentQuoteSeries(instrumentId, range, sourceFilter, target.kind === "instrument");
  const fxSeries = useFXQuoteSeries(fxBase, fxQuote, range, sourceFilter, target.kind === "fx");
  const series = target.kind === "instrument" ? instrumentSeries : fxSeries;
  const theme = chartTheme();

  const title = target.kind === "instrument"
    ? t("marketData.instrumentHistory", { name: target.instrument.name })
    : t("marketData.fxHistory", { base: fxBase, quote: fxQuote });
  const description = target.kind === "instrument"
    ? [target.instrument.symbol, target.instrument.quoteCurrency, displayEnum(t, "portfolio", target.instrument.quoteSource)].filter(Boolean).join(" · ")
    : `${fxBase}/${fxQuote}`;

  const points = series.data?.points ?? [];
  const observations = series.data?.observations ?? [];
  const currency = target.kind === "instrument" ? target.instrument.quoteCurrency : undefined;
  const emptyRange = !series.isLoading && !series.isError && observations.length === 0;
  const hasFactsOutsideRange = emptyRange && series.data?.outsideRange === true;
  const sourceLabel = (kind: string, key?: string) => {
    const label = displayEnum(t, "portfolio", kind);
    return key ? `${label} · ${key}` : label;
  };

  return (
    <Sheet open onOpenChange={(open) => { if (!open) onClose(); }}>
      <SheetContent className="max-w-xl overflow-y-auto" side="right">
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>{description}</SheetDescription>
        </SheetHeader>
        <div className="flex flex-col gap-4">
          <RangeToggle ranges={ranges} value={range} onChange={setRange} label={t("analytics.range")} />
          <SourceFilterToggle value={sourceFilter} onChange={setSourceFilter} />
          {target.kind === "fx" && (
            <Button type="button" variant="outline" size="sm" onClick={() => setSwapped((current) => !current)}>
              {t("charts.swapDirection")}
            </Button>
          )}
          {series.isLoading ? (
            <LoadingState label={t("ui.state.loadingPage")} />
          ) : series.isError ? (
            <ErrorState
              title={t("marketData.loadError")}
              description={t("ui.state.errorDescription")}
              onRetry={() => void series.refetch()}
              retryLabel={t("common.retryAction")}
            />
          ) : emptyRange ? (
            <EmptyState
              title={hasFactsOutsideRange ? t("charts.rangeEmpty") : t("charts.noLocalHistory")}
              action={hasFactsOutsideRange ? (
                <Button type="button" variant="outline" size="sm" onClick={() => setRange("all")}>
                  {t("charts.showAllRange")}
                </Button>
              ) : undefined}
            />
          ) : (
            <TrendChart
              ariaLabel={title}
              summary={title}
              dates={points.map((point) => point.quotedAt)}
              series={[{
                key: "value",
                name: target.kind === "fx" ? t("charts.rate") : t("charts.price"),
                color: theme.primary,
                values: points.map((point) => point.value),
                pointMeta: points.map((point) => ({
                  sourceLabel: sourceLabel(point.sourceKind, point.sourceKey),
                  delayed: point.delayed,
                })),
              }]}
              currency={currency ?? ""}
              height={300}
              emptyTitle={t("charts.insufficientHistory")}
              valueFormatter={(value) => (value ? (currency ? formatAmount(value, currency) : formatAmount(value)) : t("accounts.noValue"))}
              extraTableColumns={[t("charts.quotedAt"), target.kind === "fx" ? t("charts.rate") : t("charts.price"), t("charts.source"), t("charts.delayed")]}
              extraTableRows={observations.map((item) => [
                formatQuotedAt(item.quotedAt, i18n.language, settings.data?.timezone),
                currency ? formatAmount(item.value, currency) : formatAmount(item.value),
                `${displayEnum(t, "portfolio", item.sourceKind)}${item.sourceKey ? ` · ${item.sourceKey}` : ""}`,
                item.delayed ? t("charts.delayed") : t("charts.onTime"),
              ])}
            />
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}
