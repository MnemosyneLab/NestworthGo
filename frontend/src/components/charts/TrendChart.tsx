import { useTranslation } from "react-i18next";
import { EChart, type EChartsOption } from "@/components/charts/EChart";
import { chartNumber, chartTheme, resolveSeriesColor, useThemeVersion, joinTooltipLines } from "@/components/charts/chartTheme";
import { EmptyState } from "@/components/layout/PageState";
import { formatAmount } from "@/lib/money";

export interface TrendPointMeta {
 /** A semantic boundary supplied by the owning read model. */
 breakBefore?: boolean;
 provisional?: boolean;
 qualityLabel?: string;
  sourceLabel?: string;
  observationLabel?: string;
  observationKind?: string;
  effectiveDate?: string;
  delayed?: boolean;
}

export interface TrendSeries {
  key: string;
  name: string;
  color: string;
  values: Array<string | null | undefined>;
  pointMeta?: Array<TrendPointMeta | undefined>;
}

export interface TrendChartProps {
  ariaLabel: string;
  summary: string;
  dates: string[];
  series: TrendSeries[];
  currency: string;
  height?: number;
  emptyTitle: string;
  emptyDescription?: string;
  valueFormatter?: (value: string | null | undefined) => string;
  axisValueFormatter?: (value: string | number) => string;
  extraTableColumns?: string[];
  extraTableRows?: string[][];
}

/** Converts an ISO date or timestamp into the numeric coordinate required by
 * an ECharts time axis. Date-only values are kept at UTC midnight so the
 * calendar date cannot shift with the viewer's local timezone. */
export function trendChartTimestamp(value: string): number {
  const dateOnly = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value.trim());
  if (dateOnly) {
    return Date.UTC(Number(dateOnly[1]), Number(dateOnly[2]) - 1, Number(dateOnly[3]));
  }
  return Date.parse(value);
}

function formatTrendChartDate(value: string | number, language: string): string {
  const timestamp = typeof value === "number" ? value : trendChartTimestamp(value);
  if (!Number.isFinite(timestamp)) {
    return String(value);
  }
  try {
    return new Intl.DateTimeFormat(language, {
      year: "numeric",
      month: "short",
      day: "numeric",
      timeZone: "UTC",
    }).format(new Date(timestamp));
  } catch {
    return new Date(timestamp).toISOString().slice(0, 10);
  }
}

function uniqueSourceLabels(meta: Array<TrendPointMeta | undefined> | undefined): string[] {
  return [...new Set((meta ?? []).map((point) => point?.sourceLabel).filter((label): label is string => Boolean(label)))];
}

function delayedStates(meta: Array<TrendPointMeta | undefined> | undefined): Array<boolean> {
  return [...new Set((meta ?? []).flatMap((point) => (point?.delayed == null ? [] : [point.delayed])))];
}

function sourceTint(label: string, palette: string[]): string {
  let hash = 0;
  for (let index = 0; index < label.length; index += 1) {
    hash = (hash * 31 + label.charCodeAt(index)) | 0;
  }
  return palette[Math.abs(hash) % palette.length];
}

export function trendLegendLabel(name: string, item: TrendSeries, mixedSources: string, mixedDelay: string, delayed: string): string {
  const sources = uniqueSourceLabels(item.pointMeta);
  const delay = delayedStates(item.pointMeta);
  const parts = [name];
  if (sources.length > 1) {
    parts.push(mixedSources);
  } else if (sources.length === 1) {
    parts.push(sources[0]);
  }
  if (delay.length > 1) {
    parts.push(mixedDelay);
  } else if (delay.length === 1 && delay[0]) {
    parts.push(delayed);
  }
  return parts.join(" · ");
}

// Split only geometry: any edge touching an estimate is dashed. True gaps
// remain gaps, and each rendered datum retains its original tooltip index.
export function trendQualitySegments(item: TrendSeries): Array<{ indices: number[]; provisional: boolean }> {
  if (!(item.pointMeta ?? []).some(meta => meta?.provisional || meta?.breakBefore)) return [{ indices: item.values.map((_, index) => index), provisional: false }];
  const segments: Array<{ indices: number[]; provisional: boolean }> = [];
  let last = -1;
  let active: { indices: number[]; provisional: boolean } | undefined;
  item.values.forEach((value, index) => {
    if (item.pointMeta?.[index]?.breakBefore) { last = -1; active = undefined; }
    if (value == null) { last = -1; active = undefined; return; }
    const provisional = Boolean(item.pointMeta?.[index]?.provisional || (last >= 0 && item.pointMeta?.[last]?.provisional));
    if (!active || active.provisional !== provisional) {
      active = { indices: last >= 0 ? [last, index] : [index], provisional }; segments.push(active);
    } else active.indices.push(index);
    last = index;
  });
  return segments;
}

export function TrendChart({
  ariaLabel,
  summary,
  dates,
  series,
  currency,
  height = 320,
  emptyTitle,
  emptyDescription,
  valueFormatter,
  axisValueFormatter,
  extraTableColumns,
  extraTableRows,
}: TrendChartProps) {
  const { t, i18n } = useTranslation();
  useThemeVersion();
  const theme = chartTheme();
  const formatValue = valueFormatter ?? ((value: string | null | undefined) => (value ? formatAmount(value, currency) : t("accounts.noValue")));

  if (dates.length === 0) {
    return <EmptyState title={emptyTitle} description={emptyDescription ?? t("charts.insufficientHistory")} />;
  }

  const option: EChartsOption = {
    color: series.map((item) => resolveSeriesColor(item.color)),
    // ECharts 6 moved the default legend to the bottom, where it collides with the time axis.
    legend: {
      top: 0,
      left: 0,
      icon: "roundRect",
      data: series.map((item) => item.name),
      textStyle: { color: theme.foreground },
      formatter: (name) => {
        const item = series.find((seriesItem) => seriesItem.name === name);
        return item ? trendLegendLabel(name, item, t("charts.mixedSources"), t("charts.mixedDelay"), t("charts.delayed")) : name;
      },
    },
    tooltip: {
      trigger: "axis",
      confine: true,
      transitionDuration: 0,
      formatter: (params) => {
        const items = Array.isArray(params) ? params : [params];
        const date = formatTrendChartDate((items[0] as { axisValue?: string | number }).axisValue ?? "", i18n.language);
        const lines = items.flatMap((item) => {
          const point = item as { seriesName?: string; dataIndex?: number; seriesIndex?: number; data?: { originalIndex?: number } };
 const seriesItem = series.find(entry => entry.name === point.seriesName) ?? series[point.seriesIndex ?? 0];
 const index = point.data?.originalIndex ?? point.dataIndex ?? -1;
 const raw = seriesItem?.values[index];
 const meta = seriesItem?.pointMeta?.[index];
          return [
            `${point.seriesName}: ${formatValue(raw)}`,
            meta?.observationLabel,
 meta?.qualityLabel,
            meta?.effectiveDate ? `${t("quoteDetails.effectiveDate")}: ${meta.effectiveDate}` : null,
            meta?.sourceLabel ? `${t("charts.source")}: ${meta.sourceLabel}` : null,
            meta?.delayed == null ? null : (meta.delayed ? t("charts.delayed") : t("charts.onTime")),
          ];
        });
        return joinTooltipLines([date, ...lines]);
      },
    },
    grid: { left: 8, right: 16, top: 44, bottom: 8, containLabel: true },
    xAxis: {
      type: "time",
      axisLabel: {
        color: theme.muted,
        hideOverlap: true,
        formatter: (value) => formatTrendChartDate(value, i18n.language),
      },
    },
    yAxis: { type: "value", scale: true, axisLabel: { color: theme.muted, ...(axisValueFormatter ? { formatter: axisValueFormatter } : {}) }, splitLine: { lineStyle: { color: theme.border } } },
    series: series.flatMap((item) => trendQualitySegments(item).map((segment, segmentIndex) => {
      const showSourceMarks = (item.pointMeta ?? []).some((meta) => meta?.sourceLabel || meta?.delayed || meta?.provisional);
      return {
        type: "line" as const,
        name: item.name,
 id: `${item.key}-${segmentIndex}`,
 data: segment.indices.map((index) => {
 const value = item.values[index];
          const meta = item.pointMeta?.[index];
          return {
            originalIndex:index,
 value: [trendChartTimestamp(dates[index]), chartNumber(value)],
            symbol: meta?.provisional ? "diamond" : meta?.observationKind === "close" || meta?.observationKind === "daily_reference" ? "rect" : meta?.observationKind === "manual" || meta?.delayed ? "diamond" : "circle",
            itemStyle: { color: meta?.sourceLabel ? sourceTint(meta.sourceLabel, theme.palette) : resolveSeriesColor(item.color) },
          };
        }),
        // ECharts cannot brighten CSS OKLCH colors. Explicit emphasis colors
        // keep the stroke/fill visible when its automatic color lift fails.
        emphasis: {
          focus: "series" as const,
          scale: 1.4,
          lineStyle: { color: resolveSeriesColor(item.color), width: 2.5, opacity: 1 },
          areaStyle: { color: resolveSeriesColor(item.color), opacity: 0.1 },
        },
        connectNulls: false,
        showSymbol: dates.length < 24 || showSourceMarks,
        lineStyle: { color: resolveSeriesColor(item.color), width: 2.5, type: segment.provisional ? "dashed" as const : "solid" as const },
        ...(series.length === 1 ? { areaStyle: { color: resolveSeriesColor(item.color), opacity: 0.1 } } : {}),
        itemStyle: { color: resolveSeriesColor(item.color) },
      };
    })),
  };

  const columns = extraTableColumns ?? [t("charts.date"), ...series.map((item) => item.name)];
  const rows = extraTableRows ?? dates.map((date, index) => [date, ...series.map((item) => formatValue(item.values[index]))]);

  return (
    <EChart
      option={option}
      replaceSeries
      height={height}
      ariaLabel={ariaLabel}
      summary={summary}
      dataTableLabel={t("charts.viewDataTable")}
      dataTableColumns={columns}
      dataTableRows={rows}
    />
  );
}
