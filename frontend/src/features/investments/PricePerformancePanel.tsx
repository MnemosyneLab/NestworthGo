import { useState } from "react";
import { LineChart, SlidersHorizontal } from "lucide-react";
import { useTranslation } from "react-i18next";
import { TrendChart } from "@/components/charts/TrendChart";
import { SourceFilterToggle } from "@/components/charts/RangeToggle";
import { presetRange, type DatePreset } from "@/components/charts/dateRanges";
import { chartTheme, useThemeVersion } from "@/components/charts/chartTheme";
import { useTrendDateRange } from "@/components/charts/useTrendDateRange";
import { Button, buttonVariants } from "@/components/ui/button";
import { ChangeText } from "@/components/ui/change-text";
import { InfoPopover } from "@/components/ui/info-popover";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { quoteTrendProps } from "@/features/marketdata/quoteTrend";
import { useInstrumentQuoteSeries, useQuotePeriods } from "@/queries/investments";
import { useSettings } from "@/queries/settings";
import { instrumentDisplayLabel } from "@/lib/instrumentDisplay";
import { formatDecimalPercent } from "@/lib/money";
import { metalPriceSuffix } from "@/lib/preciousMetals";
import { formatCivilDateRange } from "@/lib/time";
import { cn } from "@/lib/utils";
import { amountDirection } from "./holdingGroups";
import type { InstrumentDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

/** One selectable period: the chart preset and the backend lookback in civil days (0 = all history). */
const PERIODS: ReadonlyArray<{ id: DatePreset; days: number }> = [
  { id: "1w", days: 7 }, { id: "1m", days: 30 }, { id: "3m", days: 90 },
  { id: "6m", days: 180 }, { id: "1y", days: 365 }, { id: "all", days: 0 },
];
const KNOWN_REASONS = ["insufficient_daily_history", "incomparable_daily_prices", "unsupported_instrument"];

/**
 * Compact price / NAV performance for one instrument: a large period change,
 * one period selector and a direction-tinted chart. Figures come from the
 * backend; the chart only maps stored observations to pixels.
 */
export function PricePerformancePanel({ instrument }: { instrument: InstrumentDTO }) {
  const { t, i18n } = useTranslation();
  const settings = useSettings();
  const bounds = useTrendDateRange(undefined, true);
  const [period, setPeriod] = useState<DatePreset>("1m");
  const [sourceFilter, setSourceFilter] = useState("all");
  const periods = useQuotePeriods(instrument.id);
  const preset = presetRange(period, bounds.max, bounds.min);
  const series = useInstrumentQuoteSeries(instrument.id, period === "all" ? "all" : `${preset.from}:${preset.to}`, sourceFilter);
  useThemeVersion();
  const theme = chartTheme();

  const days = PERIODS.find((item) => item.id === period)?.days ?? 30;
  const item = periods.data?.items?.find((entry) => entry.windowDays === days);
  const direction = item ? amountDirection(item.periodPercent) : undefined;
  const color = direction === "up" ? theme.gainPositive : direction === "down" ? theme.gainNegative : theme.primary;
  const reasonKey = periods.data?.unavailableReason && KNOWN_REASONS.includes(periods.data.unavailableReason) ? periods.data.unavailableReason : "insufficient_daily_history";
  const name = instrumentDisplayLabel(instrument, instrument.name);
  const observations = series.data?.observations ?? [];
  const emptyRange = !series.isLoading && !series.isError && observations.length === 0;
  const hasFactsOutsideRange = emptyRange && series.data?.outsideRange === true;
  const sourceName = t(`charts.source${sourceFilter.charAt(0).toUpperCase()}${sourceFilter.slice(1)}`);

  const headline = periods.isLoading ? <div className="h-9 w-28 rounded-lg bg-muted motion-safe:animate-pulse" aria-hidden="true" />
    : periods.isError ? <p className="text-3xl font-semibold text-muted-foreground">—</p>
    : item ? <p className="num text-3xl font-semibold tracking-tight">
      <ChangeText direction={direction ?? "flat"}>{`${direction === "up" ? "+" : ""}${formatDecimalPercent(item.periodPercent)}`}</ChangeText>
    </p>
    : <p className="text-3xl font-semibold text-muted-foreground" aria-hidden="true">—</p>;

  return (
    <section aria-label={t("holdingPerformance.label")} className="flex min-w-0 flex-col gap-3 rounded-xl border border-border bg-card p-4">
      <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-3">
        <div className="min-w-0">
          {headline}
          <div className="mt-1 flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
            {periods.isLoading ? null
              : periods.isError ? <button type="button" className="rounded underline underline-offset-4 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50" onClick={() => void periods.refetch()}>{t("common.retryAction")}</button>
              : item ? <>
                <span className="num">{formatCivilDateRange(item.baselineOn, item.latestOn, i18n.language)}</span>
                {item.annualizedPercent != null && <span className="num">· {t("holdingPerformance.annualized", { percent: formatDecimalPercent(item.annualizedPercent) })}</span>}
              </> : <span>{t(`holdingPerformance.reason.${reasonKey}`)}</span>}
            <InfoPopover label={t("holdingPerformance.info")}>
              <p>{t("holdingPerformance.infoBody")}</p>
              {instrument.metalTemplate && <p className="mt-2">{t("metals.referenceDisclaimer")}</p>}
              {!periods.isLoading && !periods.isError && !item && <p className="mt-2">{t(`holdingPerformance.reasonDetail.${reasonKey}`)}</p>}
            </InfoPopover>
          </div>
        </div>
        <div className="flex items-center gap-1.5">
          <SegmentedControl
            label={t("holdingPerformance.periodLabel")}
            value={period}
            onChange={(value) => setPeriod(value as DatePreset)}
            options={PERIODS.map((entry) => ({ value: entry.id, label: t(`holdingPerformance.periods.${entry.id}`) }))}
          />
          <Popover>
            <PopoverTrigger
              type="button"
              aria-label={sourceFilter === "all" ? t("holdingPerformance.source") : t("holdingPerformance.sourceActive", { source: sourceName })}
              className={cn(buttonVariants({ variant: "ghost", size: "icon" }), "relative size-8 text-muted-foreground")}
            >
              <SlidersHorizontal aria-hidden="true" />
              {sourceFilter !== "all" && <span className="absolute right-1.5 top-1.5 size-2 rounded-full bg-primary" aria-hidden="true" />}
            </PopoverTrigger>
            <PopoverContent align="end" className="p-3"><SourceFilterToggle value={sourceFilter} onChange={setSourceFilter} /></PopoverContent>
          </Popover>
        </div>
      </div>
      {series.isLoading ? (
        <div role="status" className="h-56 rounded-xl bg-muted/60 motion-safe:animate-pulse"><span className="sr-only">{t("holdingPerformance.loading")}</span></div>
      ) : series.isError ? (
        <div role="alert" className="flex h-56 flex-col items-center justify-center gap-2 rounded-xl bg-muted/40 text-sm text-muted-foreground">
          <p>{t("marketData.loadError")}</p>
          <Button type="button" variant="outline" size="sm" onClick={() => void series.refetch()}>{t("common.retryAction")}</Button>
        </div>
      ) : emptyRange ? (
        <div className="flex h-56 flex-col items-center justify-center gap-2 rounded-xl bg-muted/40 px-4 text-center">
          <LineChart className="size-6 text-muted-foreground" aria-hidden="true" />
          <p className="text-sm font-medium">{hasFactsOutsideRange ? t("holdingPerformance.emptyRange") : t("holdingPerformance.empty")}</p>
          {hasFactsOutsideRange
            ? <Button type="button" variant="outline" size="sm" onClick={() => setPeriod("all")}>{t("charts.showAllRange")}</Button>
            : <p className="text-xs text-muted-foreground">{t("holdingPerformance.emptyHint")}</p>}
        </div>
      ) : (
        <TrendChart
          variant="minimal"
          ariaLabel={t("holdingPerformance.chartLabel", { name })}
          summary={t("holdingPerformance.chartLabel", { name })}
          {...quoteTrendProps({ series: series.data, kind: "instrument", currency: instrument.quoteCurrency, priceSuffix: metalPriceSuffix(instrument, t), color, timezone: settings.data?.timezone, language: i18n.language, t })}
          height={220}
          emptyTitle={t("charts.insufficientHistory")}
        />
      )}
      {instrument.providerKey === "coingecko" && <p className="text-xs text-muted-foreground">{t("portfolio.coinGeckoNotice")} · <a href="https://www.coingecko.com/en/api" target="_blank" rel="noreferrer" className="underline">{t("portfolio.coinGeckoAttribution")}</a></p>}
    </section>
  );
}
