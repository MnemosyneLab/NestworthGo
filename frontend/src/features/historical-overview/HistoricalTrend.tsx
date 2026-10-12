import { useTranslation } from "react-i18next";
import { EChart, type EChartsOption } from "@/components/charts/EChart";
import { chartTheme, joinTooltipLines, useThemeVersion } from "@/components/charts/chartTheme";
import { Button } from "@/components/ui/button";
import { geometry } from "./historicalChartModel";
import type { HistoricalOverviewSeriesResult, HistoricalSeriesPoint } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";

export type HistoricalTrendMetric = "netWorth" | "assets" | "liabilities";
const connection = { netWorth: "connectNetWorth", assets: "connectAssets", liabilities: "connectLiabilities" } as const;

// Segment boundaries come from the backend. No sums, differences, percentages,
// interpolation or known-subtotal substitution occur in this geometry adapter.
export function historicalTrendSegments(points: HistoricalSeriesPoint[], metric: HistoricalTrendMetric) {
  const segments: Array<Array<{ name: string; value: number } | null>> = [];
  let segment: Array<{ name: string; value: number } | null> | null = null;
  points.forEach((point, index) => {
    const value = point[metric] == null ? null : geometry(point[metric]);
    if (!point.available || value === null) { segment = null; return; }
    if (!segment || !point[connection[metric]]) { segment = points.map(() => null); segments.push(segment); }
    segment[index] = { name: point.date, value };
  });
  return segments;
}

export function HistoricalTrend({ capture, metric, selectedDate, onMetric, onSelect, onEndpoint, onOpenDay }: {
  capture: HistoricalOverviewSeriesResult; metric: HistoricalTrendMetric; selectedDate: string | null;
  onMetric: (metric: HistoricalTrendMetric) => void; onSelect: (date: string) => void;
  onEndpoint: (side: "A" | "B", date: string) => void; onOpenDay: (date: string) => void;
}) {
  const { t } = useTranslation();
  useThemeVersion(); const theme = chartTheme();
  const points = capture.points ?? []; const overview = capture.overview;
  const currency = overview.left.currency;
  const exact = (value: string | null) => value == null ? t("historicalOverview.unknown") : `${value} ${currency}`;
  const metricLabel = (value: HistoricalTrendMetric) => t(`overview.${value}`);
  const safe = points.every(point => point[metric] == null || geometry(point[metric]) !== null);
  const segments = historicalTrendSegments(points, metric);
  const option: EChartsOption = {
    grid: { left: 18, right: 18, top: 15, bottom: 28, outerBoundsMode: "same" },
    xAxis: { type: "category", data: points.map(point => point.date), axisLabel: { color: theme.muted } },
    yAxis: { type: "value", scale: false, axisLabel: { color: theme.muted }, splitLine: { lineStyle: { color: theme.border } } },
    tooltip: { trigger: "item", formatter: params => {
      const point = Array.isArray(params) ? params[0] : params;
      const entry = points.find(entry => entry.date === point?.name);
      return entry ? joinTooltipLines([`${entry.date} · ${metricLabel(metric)}`, exact(entry[metric]), `${overview.timezone} · ${overview.capturedAt}`, ...(entry.missing ?? []).map(missing => `${missing.name} · ${missing.key}: ${(missing.reasons ?? []).map(reason => t(`historicalOverview.missing.${reason}`)).join(", ")}`)]) : "";
    } },
    series: segments.map((data, index) => ({ id: `historical-trend-${metric}-${index}`, type: "line", connectNulls: false, smooth: false,
      showSymbol: true, itemStyle: { color: theme.primary }, lineStyle: { color: theme.primary },
      data: data.map(point => point && ({ ...point, symbolSize: point.name === selectedDate ? 14 : point.name === capture.from || point.name === capture.to ? 10 : 5 })),
    })),
  };
  return <section aria-label={t("historicalOverview.trend.title")} className="min-w-0 rounded-xl border border-border bg-card p-4">
    <h2 className="font-semibold">{t("historicalOverview.trend.title")}</h2>
    <p className="mt-1 text-xs text-muted-foreground">A · {capture.from} → B · {capture.to} · {currency} · {overview.timezone}</p>
    <p className="mt-1 text-xs text-muted-foreground">{t("historicalOverview.trend.basis")}</p>
    <div role="group" aria-label={t("historicalOverview.trend.metric")} className="my-3 flex flex-wrap gap-2">{(["netWorth", "assets", "liabilities"] as const).map(value => <Button key={value} size="sm" aria-pressed={metric === value} variant={metric === value ? "default" : "outline"} onClick={() => onMetric(value)}>{metricLabel(value)}</Button>)}</div>
    {points.length === 1 && <p className="text-sm">{t("historicalOverview.trend.onePoint")}</p>}
    {!safe && <p className="text-sm">{t("historicalOverview.geometryUnavailable")}</p>}
    {segments.length === 0 && <p className="text-sm">{t("historicalOverview.trend.noCompleteTotals")}</p>}
    {safe && segments.length > 0 && <EChart key={`${capture.from}-${capture.to}-${metric}-${segments.length}`} option={option} height={280} ariaLabel={t("historicalOverview.trend.title")} summary={`${metricLabel(metric)} · ${capture.from} → ${capture.to} · ${currency}`} onSelectName={date => { if (date && points.some(point => point.date === date)) onSelect(date); }} />}
    <p className="my-3 text-xs text-muted-foreground">{t("historicalOverview.trend.actionsNote")}</p>
    <div className="overflow-x-auto"><table className="w-full text-left text-sm"><caption className="pb-2 text-left font-medium">{t("historicalOverview.fullChartData")}</caption><thead><tr><th scope="col">{t("historicalOverview.date")}</th><th scope="col">{metricLabel("netWorth")}</th><th scope="col">{metricLabel("assets")}</th><th scope="col">{metricLabel("liabilities")}</th><th scope="col">{t("historicalOverview.knownSubtotal")}</th><th scope="col">{t("historicalOverview.comparability")}</th><th scope="col">{t("historicalOverview.trend.actions")}</th></tr></thead><tbody>{points.map(point => <tr key={point.date} className="border-t border-border" aria-selected={point.date === selectedDate}><th scope="row"><Button size="sm" variant="link" aria-pressed={point.date === selectedDate} onClick={() => onSelect(point.date)}>{point.date}</Button>{point.date === capture.from && <span> · A</span>}{point.date === capture.to && <span> · B</span>}</th>{(["netWorth", "assets", "liabilities"] as const).map(value => <td key={value} className="num whitespace-nowrap p-2">{exact(point[value])}</td>)}<td className="p-2">{metricLabel("assets")}: {exact(point.knownAssets)}<br />{metricLabel("liabilities")}: {exact(point.knownLiabilities)}<br />{t("historicalOverview.trend.componentCounts", { assetKnown: point.assetCoverage.known, assetTotal: point.assetCoverage.included, debtKnown: point.liabilityCoverage.known, debtTotal: point.liabilityCoverage.included })}</td><td className="p-2">{!point.available && t("historicalOverview.trend.skippedDate")}{point.scopeChanged && <p>{t("historicalOverview.scopeChanged")}</p>}{point.basisChanged && <p>{t("historicalOverview.trend.basisChanged")}</p>}{(point.missing ?? []).map(missing => <p key={missing.key}>{missing.name} · {missing.key}: {(missing.reasons ?? []).map(reason => t(`historicalOverview.missing.${reason}`)).join(", ")}</p>)}</td><td className="p-2"><div className="flex flex-wrap gap-2"><Button size="sm" variant="outline" disabled={!point.available} onClick={() => onEndpoint("A", point.date)}>{t("historicalOverview.trend.setA")}</Button><Button size="sm" variant="outline" disabled={!point.available} onClick={() => onEndpoint("B", point.date)}>{t("historicalOverview.trend.setB")}</Button><Button size="sm" variant="outline" disabled={!point.available} onClick={() => onOpenDay(point.date)}>{t("historicalOverview.trend.openDay")}</Button></div></td></tr>)}</tbody></table></div>
  </section>;
}
