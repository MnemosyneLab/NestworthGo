import { useTranslation } from "react-i18next";
import { EChart, type EChartsOption } from "@/components/charts/EChart";
import { chartNumber, chartTheme, joinTooltipLines } from "@/components/charts/chartTheme";
import { EmptyState } from "@/components/layout/PageState";
import { formatAmount } from "@/lib/money";

export interface TrendPointMeta {
  sourceLabel?: string;
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
  const { t } = useTranslation();
  const theme = chartTheme();
  const formatValue = valueFormatter ?? ((value: string | null | undefined) => (value ? formatAmount(value, currency) : t("accounts.noValue")));

  if (dates.length === 0) {
    return <EmptyState title={emptyTitle} description={emptyDescription ?? t("charts.insufficientHistory")} />;
  }

  const option: EChartsOption = {
    color: series.map((item) => item.color),
    legend: {
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
        const date = String((items[0] as { axisValue?: string }).axisValue ?? "");
        const lines = items.flatMap((item) => {
          const point = item as { seriesName?: string; dataIndex?: number; seriesIndex?: number };
          const seriesItem = series[point.seriesIndex ?? 0];
          const raw = seriesItem?.values[point.dataIndex ?? -1];
          const meta = seriesItem?.pointMeta?.[point.dataIndex ?? -1];
          return [
            `${point.seriesName}: ${formatValue(raw)}`,
            meta?.sourceLabel ? `${t("charts.source")}: ${meta.sourceLabel}` : null,
            meta?.delayed == null ? null : (meta.delayed ? t("charts.delayed") : t("charts.onTime")),
          ];
        });
        return joinTooltipLines([date, ...lines]);
      },
    },
    xAxis: { type: "category", data: dates, axisLabel: { color: theme.muted } },
    yAxis: { type: "value", axisLabel: { color: theme.muted, ...(axisValueFormatter ? { formatter: axisValueFormatter } : {}) }, splitLine: { lineStyle: { color: theme.border } } },
    series: series.map((item) => {
      const showSourceMarks = (item.pointMeta ?? []).some((meta) => meta?.sourceLabel || meta?.delayed);
      return {
        type: "line" as const,
        name: item.name,
        data: item.values.map((value, index) => {
          const meta = item.pointMeta?.[index];
          return {
            value: chartNumber(value),
            symbol: meta?.delayed ? "diamond" : "circle",
            itemStyle: { color: meta?.sourceLabel ? sourceTint(meta.sourceLabel, theme.palette) : item.color },
          };
        }),
        emphasis: { disabled: true },
        connectNulls: false,
        showSymbol: dates.length < 24 || showSourceMarks,
        lineStyle: { color: item.color, width: 2 },
        itemStyle: { color: item.color },
      };
    }),
  };

  const columns = extraTableColumns ?? [t("charts.date"), ...series.map((item) => item.name)];
  const rows = extraTableRows ?? dates.map((date, index) => [date, ...series.map((item) => formatValue(item.values[index]))]);

  return (
    <EChart
      option={option}
      height={height}
      ariaLabel={ariaLabel}
      summary={summary}
      dataTableLabel={t("charts.viewDataTable")}
      dataTableColumns={columns}
      dataTableRows={rows}
    />
  );
}
