import { useMemo, useState } from "react";
import { LineChart } from "lucide-react";
import { useTranslation } from "react-i18next";
import { TrendChart } from "@/components/charts/TrendChart";
import { chartTheme, useThemeVersion } from "@/components/charts/chartTheme";
import { Button } from "@/components/ui/button";
import { ChangeText } from "@/components/ui/change-text";
import { InfoPopover } from "@/components/ui/info-popover";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { quoteTrendProps, type QuoteTrendSeries } from "@/features/marketdata/quoteTrend";
import { usePerformanceSeries } from "@/queries/investments";
import { useSettings } from "@/queries/settings";
import { instrumentDisplayLabel } from "@/lib/instrumentDisplay";
import { formatDecimalPercent } from "@/lib/money";
import { metalPriceSuffix } from "@/lib/preciousMetals";
import { formatCivilDateRange } from "@/lib/time";
import { amountDirection } from "./holdingGroups";
import type { InstrumentDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

/** One selectable period and its backend lookback in civil days (0 = all history). */
const PERIODS = [
  { id: "1w", days: 7 }, { id: "1m", days: 30 }, { id: "3m", days: 90 },
  { id: "6m", days: 180 }, { id: "1y", days: 365 }, { id: "all", days: 0 },
] as const;
type PeriodId = (typeof PERIODS)[number]["id"];
const KNOWN_REASONS = ["insufficient_daily_history", "incomparable_daily_prices", "unsupported_instrument"];

/**
 * Compact price / NAV performance for one instrument: a large change for the
 * selected period, one period selector and a direction-tinted chart. The chart
 * and the headline come from one backend read of dated daily closes, so they
 * always cover the same interval; the chart only maps stored observations to pixels.
 */
export function PricePerformancePanel({ instrument }: { instrument: InstrumentDTO }) {
  const { t, i18n } = useTranslation();
  const settings = useSettings();
  const [period, setPeriod] = useState<PeriodId>("1m");
  const days = PERIODS.find((entry) => entry.id === period)?.days ?? 30;
  const performance = usePerformanceSeries(instrument.id, days);
  useThemeVersion();
  const theme = chartTheme();

  const data = performance.data;
  const item = data?.item ?? undefined;
  const direction = item ? amountDirection(item.periodPercent) : undefined;
  const color = direction === "up" ? theme.gainPositive : direction === "down" ? theme.gainNegative : theme.primary;
  const reasonKey = data?.unavailableReason && KNOWN_REASONS.includes(data.unavailableReason) ? data.unavailableReason : "insufficient_daily_history";
  const name = instrumentDisplayLabel(instrument, instrument.name);
  const series = useMemo<QuoteTrendSeries>(() => {
    const points = data?.points ?? [];
    return { points, observations: [...points].reverse() };
  }, [data]);
  const emptyChart = !performance.isLoading && !performance.isError && series.points?.length === 0;

  const headline = performance.isLoading ? <div className="h-9 w-28 rounded-lg bg-muted motion-safe:animate-pulse" aria-hidden="true" />
    : performance.isError ? <p className="text-3xl font-semibold text-muted-foreground">—</p>
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
            {performance.isLoading || performance.isError ? null
              : item ? <span className="num">{formatCivilDateRange(item.baselineOn, item.latestOn, i18n.language)}</span>
              : <span>{t(`holdingPerformance.reason.${reasonKey}`)}</span>}
            <InfoPopover label={t("holdingPerformance.info")}>
              <p>{t("holdingPerformance.infoBody")}</p>
              {instrument.metalTemplate && <p className="mt-2">{t("metals.referenceDisclaimer")}</p>}
              {!performance.isLoading && !performance.isError && !item && <p className="mt-2">{t(`holdingPerformance.reasonDetail.${reasonKey}`)}</p>}
            </InfoPopover>
          </div>
        </div>
        <SegmentedControl
          label={t("holdingPerformance.periodLabel")}
          value={period}
          onChange={(value) => setPeriod(value as PeriodId)}
          options={PERIODS.map((entry) => ({ value: entry.id, label: t(`holdingPerformance.periods.${entry.id}`) }))}
        />
      </div>
      {performance.isLoading ? (
        <div role="status" className="h-56 rounded-xl bg-muted/60 motion-safe:animate-pulse"><span className="sr-only">{t("holdingPerformance.loading")}</span></div>
      ) : performance.isError ? (
        <div role="alert" className="flex h-56 flex-col items-center justify-center gap-2 rounded-xl bg-muted/40 text-sm text-muted-foreground">
          <p>{t("marketData.loadError")}</p>
          <Button type="button" variant="outline" size="sm" onClick={() => void performance.refetch()}>{t("common.retryAction")}</Button>
        </div>
      ) : emptyChart ? (
        <div className="flex h-56 flex-col items-center justify-center gap-2 rounded-xl bg-muted/40 px-4 text-center">
          <LineChart className="size-6 text-muted-foreground" aria-hidden="true" />
          <p className="text-sm font-medium">{t("holdingPerformance.empty")}</p>
          <p className="text-xs text-muted-foreground">{t("holdingPerformance.emptyHint")}</p>
        </div>
      ) : (
        <TrendChart
          variant="minimal"
          ariaLabel={t("holdingPerformance.chartLabel", { name })}
          summary={t("holdingPerformance.chartLabel", { name })}
          {...quoteTrendProps({ series, kind: "instrument", daily: true, currency: instrument.quoteCurrency, priceSuffix: metalPriceSuffix(instrument, t), color, timezone: settings.data?.timezone, language: i18n.language, t })}
          height={220}
          emptyTitle={t("charts.insufficientHistory")}
        />
      )}
      {instrument.providerKey === "coingecko" && <p className="text-xs text-muted-foreground">{t("portfolio.coinGeckoNotice")} · <a href="https://www.coingecko.com/en/api" target="_blank" rel="noreferrer" className="underline">{t("portfolio.coinGeckoAttribution")}</a></p>}
    </section>
  );
}
