import { signedAccountAmount, signedAccountValuation } from "@/lib/liabilityDisplay";
import { formatCivilDate, formatCivilDateRange } from "@/lib/time";
import { formatHistoricalAmount, historicalDisplayAmount } from "./historicalAmount";
import { fullDisplayAmount, valuationDetails } from "@/components/valuation/valuationQuality";
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
  let previous = -1;
  let previousProvisional = false;
  let edgeProvisional = false;
  points.forEach((point, index) => {
    const raw = fullDisplayAmount(point.displayValuation?.[metric],point[metric]);
    const value = raw == null ? null : geometry(signedAccountAmount(raw, metric === "liabilities"));
    if (!point.available || value === null) { segment = null; previous = -1; return; }
    const provisional = point.displayValuation?.[metric]?.quality?.coverage === "provisional";
    const connected = previous >= 0 && point[connection[metric]];
    const nextEdge = provisional || (connected && previousProvisional);
    if (!segment || !connected || edgeProvisional !== nextEdge) {
      const prior = connected && segment ? segment[previous] : null;
      segment = points.map(() => null); segments.push(segment);
      if (prior) segment[previous] = prior;
      edgeProvisional = nextEdge;
    }
    segment[index] = { name: point.date, value };
    previous = index; previousProvisional = provisional;
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
  const money = (value: string | null) => value == null ? t("historicalOverview.unknown") : formatHistoricalAmount(value, currency)!;
  const metricLabel = (value: HistoricalTrendMetric) => t(`overview.${value}`);
  const safe = points.every(point => {const raw=fullDisplayAmount(point.displayValuation?.[metric],point[metric]);return raw==null || geometry(raw)!==null;});
  const segments = historicalTrendSegments(points, metric);
  const option: EChartsOption = {
    grid: { left: 18, right: 18, top: 15, bottom: 28, outerBoundsMode: "same" },
    xAxis: { type: "category", data: points.map(point => point.date), axisLabel: { color: theme.muted, formatter: (value: string) => formatCivilDate(String(value)) } },
    yAxis: { type: "value", scale: false, axisLabel: { color: theme.muted }, splitLine: { lineStyle: { color: theme.border } } },
    tooltip: { trigger: "item", formatter: params => {
      const point = Array.isArray(params) ? params[0] : params;
      const entry = points.find(entry => entry.date === point?.name);
      return entry ? joinTooltipLines([`${formatCivilDate(entry.date)} · ${metricLabel(metric)}`, historicalDisplayAmount(t,signedAccountValuation(entry.displayValuation?.[metric],metric === "liabilities"),entry[metric]==null ? null : {amount:signedAccountAmount(entry[metric],metric === "liabilities"),currency},metric==="netWorth"), valuationDetails(t,entry.displayValuation?.[metric]),  t("moneyDisplay.missingCount", { count: entry.missing?.length ?? 0 }), ...(entry.missing ?? []).slice(0, 3).map(missing => `${missing.name}: ${(missing.reasons ?? []).map(reason => t(`historicalOverview.missing.${reason}`)).join(", ")}`)]) : "";
    } },
    series: segments.map((data, index) => ({ id: `historical-trend-${metric}-${index}`, type: "line", connectNulls: false, smooth: false,
      showSymbol: true, itemStyle: { color: theme.primary }, lineStyle: { color: theme.primary, type: data.some(point => point && points.find(entry => entry.date===point.name)?.displayValuation?.[metric]?.quality?.coverage === "provisional") ? "dashed" : "solid" },
      data: data.map(point => point && ({ ...point, symbol:points.find(entry=>entry.date===point.name)?.displayValuation?.[metric]?.quality?.coverage === "provisional" ? "diamond" : "circle", symbolSize: point.name === selectedDate ? 14 : point.name === capture.from || point.name === capture.to ? 10 : 5 })),
    })),
  };
  return <section aria-label={t("historicalOverview.trend.title")} className="min-w-0 rounded-xl border border-border bg-card p-4">
    <h2 className="font-semibold">{t("historicalOverview.trend.title")}</h2>
    <p className="mt-1 text-xs text-muted-foreground">A · {formatCivilDate(capture.from)}{capture.from === capture.to ? "" : ` → B · ${formatCivilDate(capture.to)}`} · {currency} · {overview.timezone}</p>
    <p className="mt-1 text-xs text-muted-foreground">{t("historicalOverview.trend.basis")}</p>
    <div role="group" aria-label={t("historicalOverview.trend.metric")} className="my-3 flex flex-wrap gap-2">{(["netWorth", "assets", "liabilities"] as const).map(value => <Button key={value} size="sm" aria-pressed={metric === value} variant={metric === value ? "default" : "outline"} onClick={() => onMetric(value)}>{metricLabel(value)}</Button>)}</div>
    {points.length === 1 && <p className="text-sm">{t("historicalOverview.trend.onePoint")}</p>}
    {!safe && <p className="text-sm">{t("historicalOverview.geometryUnavailable")}</p>}
    {segments.length === 0 && <p className="text-sm">{t("historicalOverview.trend.noCompleteTotals")}</p>}
    {safe && segments.length > 0 && <EChart key={`${capture.from}-${capture.to}-${metric}-${segments.length}`} option={option} height={280} ariaLabel={t("historicalOverview.trend.title")} summary={`${metricLabel(metric)} · ${formatCivilDateRange(capture.from, capture.to)} · ${currency}`} onSelectName={date => { if (date && points.some(point => point.date === date)) onSelect(date); }} />}
    <p className="my-3 text-xs text-muted-foreground">{t("historicalOverview.trend.actionsNote")}</p>
    <div className="overflow-x-auto"><table className="min-w-[60rem] w-full text-left text-sm"><caption className="pb-2 text-left font-medium">{t("historicalOverview.fullChartData")}</caption><thead><tr><th scope="col">{t("historicalOverview.date")}</th><th scope="col">{metricLabel("netWorth")}</th><th scope="col">{metricLabel("assets")}</th><th scope="col">{metricLabel("liabilities")}</th><th scope="col">{t("historicalOverview.knownSubtotal")}</th><th scope="col">{t("historicalOverview.comparability")}</th><th scope="col">{t("historicalOverview.trend.actions")}</th></tr></thead><tbody>{points.map(point => <tr key={point.date} className="border-t border-border" aria-selected={point.date === selectedDate}><th scope="row"><Button size="sm" variant="link" aria-pressed={point.date === selectedDate} onClick={() => onSelect(point.date)}>{formatCivilDate(point.date)}</Button>{point.date === capture.from && <span> · A</span>}{point.date === capture.to && <span> · B</span>}</th>{(["netWorth", "assets", "liabilities"] as const).map(value => <td key={value} className="num whitespace-nowrap p-2">{historicalDisplayAmount(t,signedAccountValuation(point.displayValuation?.[value],value === "liabilities"),point[value]==null ? null : {amount:signedAccountAmount(point[value],value === "liabilities"),currency},value==="netWorth")}</td>)}<td className="p-2">{metricLabel("assets")}: {money(point.knownAssets)}<br />{metricLabel("liabilities")}: {money(point.knownLiabilities == null ? null : signedAccountAmount(point.knownLiabilities,true))}<br />{t("historicalOverview.trend.componentCounts", { assetKnown: point.assetCoverage.known, assetTotal: point.assetCoverage.included, debtKnown: point.liabilityCoverage.known, debtTotal: point.liabilityCoverage.included })}</td><td className="p-2">{valuationDetails(t,point.displayValuation?.[metric])}{!point.available && t("historicalOverview.trend.skippedDate")}{point.scopeChanged && <p>{t("historicalOverview.scopeChanged")}</p>}{point.basisChanged && <p>{t("historicalOverview.trend.basisChanged")}</p>}{(point.missing ?? []).length > 0 && <details><summary className="cursor-pointer">{t("moneyDisplay.missingCount", { count: point.missing?.length ?? 0 })}</summary>{(point.missing ?? []).map(missing => <p key={missing.key}>{missing.name}: {(missing.reasons ?? []).map(reason => t(`historicalOverview.missing.${reason}`)).join(", ")}</p>)}</details>}</td><td className="p-2"><div className="flex flex-wrap gap-2"><Button size="sm" variant="outline" disabled={!point.available} onClick={() => onEndpoint("A", point.date)}>{t("historicalOverview.trend.setA")}</Button><Button size="sm" variant="outline" disabled={!point.available} onClick={() => onEndpoint("B", point.date)}>{t("historicalOverview.trend.setB")}</Button><Button size="sm" variant="outline" disabled={!point.available} onClick={() => onOpenDay(point.date)}>{t("historicalOverview.trend.openDay")}</Button></div></td></tr>)}</tbody></table></div>
  </section>;
}
