import { useTranslation } from "react-i18next";
import { EChart, type EChartsOption } from "@/components/charts/EChart";
import { chartNumber, chartTheme, resolveSeriesColor, useThemeVersion, joinTooltipLines } from "@/components/charts/chartTheme";
import { EmptyState } from "@/components/layout/PageState";
import { formatAmount } from "@/lib/money";
import { formatCivilDate } from "@/lib/time";

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
  /** Select observations without treating other series' dates as missing values. */
  pointIndices?: number[];
  presentation?: "line" | "points";
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
  /** "minimal" hides the legend, softens gridlines and keeps the tooltip to date and value. */
  variant?: "default" | "minimal";
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

/** Compact axis labels for the minimal variant: month and day, or month and year for multi-year spans. */
function formatTrendAxisDate(value: string | number, language: string, longSpan: boolean): string {
  const timestamp = typeof value === "number" ? value : trendChartTimestamp(value);
  if (!Number.isFinite(timestamp)) return String(value);
  try {
    return new Intl.DateTimeFormat(language, longSpan ? { month: "short", year: "2-digit", timeZone: "UTC" } : { month: "short", day: "numeric", timeZone: "UTC" }).format(new Date(timestamp));
  } catch {
    return new Date(timestamp).toISOString().slice(0, 10);
  }
}

function chartCivilDate(value: string | number): string {
  if (typeof value === "string" && /^\d{4}-\d{2}-\d{2}$/.test(value.trim())) return value.trim();
  const timestamp = typeof value === "number" ? value : trendChartTimestamp(value);
  if (!Number.isFinite(timestamp)) return "";
  const date = new Date(timestamp);
  const month = String(date.getUTCMonth() + 1).padStart(2, "0");
  const day = String(date.getUTCDate()).padStart(2, "0");
  return `${date.getUTCFullYear()}-${month}-${day}`;
}

function formatTrendChartDate(value: string | number, language: string): string {
  const civil = chartCivilDate(value);
  return civil ? formatCivilDate(civil, language) : String(value);
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
  const indices = item.pointIndices ?? item.values.map((_, index) => index);
  if (!indices.some(index => item.pointMeta?.[index]?.provisional || item.pointMeta?.[index]?.breakBefore)) return [{ indices, provisional: false }];
  const segments: Array<{ indices: number[]; provisional: boolean }> = [];
  let last = -1;
  let active: { indices: number[]; provisional: boolean } | undefined;
  indices.forEach(index => {
    const value = item.values[index];
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
  variant = "default",
}: TrendChartProps) {
  const { t, i18n } = useTranslation();
  useThemeVersion();
  const theme = chartTheme();
  const formatValue = valueFormatter ?? ((value: string | null | undefined) => (value ? formatAmount(value, currency) : t("accounts.noValue")));

  if (dates.length === 0) {
    return <EmptyState title={emptyTitle} description={emptyDescription ?? t("charts.insufficientHistory")} />;
  }

  const minimal = variant === "minimal";
  // ECharts reports `seriesIndex` against the flattened segment list, never the
  // caller's series array, so every segment keeps a pointer to its owner.
  const segments = series.flatMap((item, itemIndex) => trendQualitySegments(item).map((segment, segmentIndex) => ({ item, itemIndex, segment, segmentIndex })));
  const owner = (seriesIndex: number | undefined, seriesName?: string) => segments[seriesIndex ?? -1]?.item ?? series.find((entry) => entry.name === seriesName) ?? series[0];
  const stamps = dates.map(trendChartTimestamp).filter(Number.isFinite);
  const longSpan = stamps.length > 1 && Math.max(...stamps) - Math.min(...stamps) > 330 * 86_400_000;
  const option: EChartsOption = {
    color: series.map((item) => resolveSeriesColor(item.color)),
    // ECharts 6 moved the default legend to the bottom, where it collides with the time axis.
    legend: minimal ? { show: false } : {
      type: "scroll",
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
      ...(minimal ? { axisPointer: { type: "line" as const, lineStyle: { color: theme.muted, opacity: 0.5, type: "dashed" as const } } } : {}),
      formatter: (params) => {
        const items = Array.isArray(params) ? params : [params];
        const date = formatTrendChartDate((items[0] as { axisValue?: string | number }).axisValue ?? "", i18n.language);
        if (minimal) {
          const point = items[0] as { seriesName?: string; dataIndex?: number; seriesIndex?: number; data?: { originalIndex?: number } };
          const seriesItem = owner(point.seriesIndex, point.seriesName);
          const index = point.data?.originalIndex ?? point.dataIndex ?? -1;
          const meta = seriesItem?.pointMeta?.[index];
          return joinTooltipLines([date, formatValue(seriesItem?.values[index]), meta?.delayed ? t("charts.delayed") : null]);
        }
        const lines = items.flatMap((item) => {
          const point = item as { seriesName?: string; dataIndex?: number; seriesIndex?: number; data?: { originalIndex?: number } };
 const seriesItem = owner(point.seriesIndex, point.seriesName);
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
    grid: { left: 8, right: minimal ? 12 : 16, top: minimal ? 12 : 44, bottom: 8, containLabel: true },
    xAxis: {
      type: "time",
      ...(minimal ? { splitNumber: 4, axisTick: { show: false }, axisLine: { lineStyle: { color: theme.border } } } : {}),
      axisLabel: {
        color: theme.muted,
        hideOverlap: true,
        ...(minimal ? { fontSize: 11, margin: 12 } : {}),
        formatter: (value) => minimal ? formatTrendAxisDate(value, i18n.language, longSpan) : formatTrendChartDate(value, i18n.language),
      },
    },
    yAxis: {
      type: "value", scale: true,
      ...(minimal ? { axisLine: { show: false }, axisTick: { show: false }, splitNumber: 4 } : {}),
      axisLabel: { color: theme.muted, ...(minimal ? { fontSize: 11 } : {}), ...(axisValueFormatter ? { formatter: axisValueFormatter } : {}) },
      splitLine: { lineStyle: minimal ? { color: theme.border, opacity: 0.5, type: "dashed" as const } : { color: theme.border } },
    },
    series: segments.map(({ item, segment, segmentIndex }) => {
      const pointsOnly = item.presentation === "points";
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
            itemStyle: { color: !item.pointIndices && meta?.sourceLabel ? sourceTint(meta.sourceLabel, theme.palette) : resolveSeriesColor(item.color) },
          };
        }),
        // ECharts cannot brighten CSS OKLCH colors. Explicit emphasis colors
        // keep the stroke/fill visible when its automatic color lift fails.
        emphasis: {
          focus: "series" as const,
          scale: 1.4,
          lineStyle: { color: resolveSeriesColor(item.color), width: pointsOnly ? 0 : 2.5, opacity: 1 },
          areaStyle: { color: resolveSeriesColor(item.color), opacity: pointsOnly ? 0 : 0.1 },
        },
        connectNulls: false,
        // An isolated observation has no line to draw, so its segment always
        // keeps the marker regardless of how many dates the chart has overall.
        showSymbol: pointsOnly || segment.indices.length === 1 || (!minimal && (dates.length < 24 || showSourceMarks)),
        ...(minimal ? { symbolSize: 7, smooth: 0.15 } : {}),
        lineStyle: { color: resolveSeriesColor(item.color), width: pointsOnly ? 0 : 2.5, type: segment.provisional ? "dashed" as const : "solid" as const },
        ...(!pointsOnly && series.length === 1 ? { areaStyle: { color: resolveSeriesColor(item.color), opacity: minimal ? 0.14 : 0.1 } } : {}),
        itemStyle: { color: resolveSeriesColor(item.color) },
      };
    }),
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
      subtleDataTable={minimal}
    />
  );
}
