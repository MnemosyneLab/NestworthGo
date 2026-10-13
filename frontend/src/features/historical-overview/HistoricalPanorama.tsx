import { ValuationQualitySummary } from "@/components/valuation/ValuationQualitySummary";
import { fullDisplayAmount } from "@/components/valuation/valuationQuality";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { EChart, type EChartsOption } from "@/components/charts/EChart";
import { chartTheme, joinTooltipLines, useThemeVersion } from "@/components/charts/chartTheme";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { displayEnum } from "@/lib/display";
import type { HistoricalOverviewResult, HistoricalOverviewRow, HistoricalOverviewState, HistoricalSeriesPoint } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
import { historicalRowChange, accountChanges, alignedAllocations, allocationMode, comparableStatuses, geometry, isZero, missingAssetLeaves } from "./historicalChartModel";
import { historicalDisplayAmount, formatHistoricalAmount as amount } from "./historicalAmount";

type Dimension = "byClass" | "byCurrency";
type Side = "left" | "right";

function stateLabel(state: HistoricalOverviewState, side: Side, current: string) {
  return `${side === "left" ? "A" : "B"} · ${state.current ? current : state.date}`;
}

export function HistoricalMetricCards({ data, seriesPoints }: { data: HistoricalOverviewResult; seriesPoints?: HistoricalSeriesPoint[] | null }) {
  const { t } = useTranslation();
  const { left, right } = data;
 const change = data.estimatedChange ?? data.change;
  const mismatch = Boolean(right && left.currency !== right.currency);
  return <div className="grid min-w-0 gap-3 min-[700px]:grid-cols-2 min-[1100px]:grid-cols-3">
    {(["netWorth", "assets", "liabilities"] as const).map(key => <Card key={key} className="min-w-0">
      <CardHeader><CardTitle>{t(`overview.${key}`)}</CardTitle></CardHeader>
      <CardContent>
        <dl className="grid grid-cols-[repeat(auto-fit,minmax(min(100%,12rem),1fr))] gap-3">
          {(["left", "right"] as const).filter(side => side === "left" || right).map(side => {
            const state = side === "left" ? left : right!;
            const point = seriesPoints?.find(point => point.date === state.date);
            const subtotalKey = key === "assets" ? "knownAssets" : "knownLiabilities";
            const known = point ? point[subtotalKey] : state[subtotalKey];
            return <div key={side} className="min-w-0">
              <dt className="text-xs text-muted-foreground">{stateLabel(state, side, t("historicalOverview.current"))}</dt>
              <dd tabIndex={0} aria-label={t(`overview.${key}`)} className="mt-1 overflow-x-auto text-lg font-semibold focus-visible:outline-primary">
                <span className="num whitespace-nowrap" title={state[key] == null ? undefined : `${state[key]} ${state.currency}`}>{historicalDisplayAmount(t,state.displayValuation?.[key],state[key]==null ? null : {amount:state[key],currency:state.currency},key==="netWorth")}</span>
              </dd>
              <ValuationQualitySummary value={state.displayValuation?.[key]} current={state.current} />
 {fullDisplayAmount(state.displayValuation?.[key],state[key]) == null && !state.displayValuation && key !== "netWorth" && <p className="text-xs text-muted-foreground">{t("historicalOverview.knownSubtotal")}: {amount(known, state.currency) ?? t("historicalOverview.unknown")}</p>}
            </div>;
          })}
          {right && <div className="min-w-0">
            <dt className="text-xs text-muted-foreground">{data.estimatedChange ? t("valuation.estimatedChange") : t("historicalOverview.deltaBA")}</dt>
            <dd tabIndex={0} aria-label={t(`overview.${key}`)} className="mt-1 overflow-x-auto text-lg font-semibold focus-visible:outline-primary">
              <span className="num whitespace-nowrap" title={mismatch || change?.[key] == null ? undefined : `${change[key]} ${left.currency}`}>
                {mismatch ? t("historicalOverview.currencyMismatch") : amount(change?.[key], left.currency, true) ?? t("historicalOverview.unknown")}
              </span>
            </dd>
          </div>}
        </dl>
        {key === "liabilities" && right && <p className="mt-2 text-xs text-muted-foreground">{t("historicalOverview.debtBalanceNote")}</p>}
      </CardContent>
    </Card>)}
  </div>;
}

export function HistoricalCoverage({ data, onDetails }: { data: HistoricalOverviewResult; onDetails: () => void }) {
  const { t } = useTranslation();
  const sides: Side[] = data.right ? ["left", "right"] : ["left"];
  return <div role="status" className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-border bg-card p-3 text-sm">
    <div className="flex flex-wrap gap-2">{sides.map(side => {
      const state = data[side]!;
      const count = missingAssetLeaves(data, side);
      return <span key={side}>{stateLabel(state, side, t("historicalOverview.current"))}: {state.complete ? t("historicalOverview.coverageComplete") : t("historicalOverview.coveragePartial")}
        {count > 0 && ` · ${t("historicalOverview.missingAssetLeaves", { count })}`}
      </span>;
    })}{data.right && data.left.currency !== data.right.currency && <Badge variant="warning">{t("historicalOverview.currencyMismatch")}</Badge>}</div>
    <Button variant="link" onClick={onDetails}>{t("historicalOverview.viewEvidence")}</Button>
  </div>;
}

export function HistoricalAllocationView({ data, onFilter, selectedDimension, onDimension }: { data: HistoricalOverviewResult; onFilter: (dimension: Dimension, key: string, trigger?: HTMLButtonElement) => void; selectedDimension?: Dimension; onDimension?: (dimension: Dimension) => void }) {
  const { t } = useTranslation();
  useThemeVersion();
  const theme = chartTheme();
  const [localDimension, setLocalDimension] = useState<Dimension>("byClass");
  const dimension = selectedDimension ?? localDimension;
  const setDimension = onDimension ?? setLocalDimension;
  const categoryButtons = useRef(new Map<string, HTMLButtonElement>());
  const left = data.left[dimension] ?? [];
  const right = data.right?.[dimension] ?? [];
  const label = (key: string) => dimension === "byClass" ? displayEnum(t, "enum", key) || key : key;
  const paired = data.right ? alignedAllocations(left, right) : [];
  const items = data.right ? paired.map(item => item.key) : left.map(item => item.key);
  const currency = data.left.currency;
  const mismatch = Boolean(data.right && currency !== data.right.currency);
  const mode = allocationMode(data.left, left);
  const allAmountsZero = data.right
    ? paired.length > 0
      ? paired.every(item => (!item.left || isZero(item.left.amount)) && (!item.right || isZero(item.right.amount)))
      : isZero(data.left.knownAssets) && isZero(data.right.knownAssets)
    : mode === "zero";
  const geometrySafe = !mismatch && (data.right
    ? paired.every(item => (!item.left || geometry(item.left.amount) !== null) && (!item.right || geometry(item.right.amount) !== null))
    : left.every(item => geometry(item.amount) !== null));
  let option: EChartsOption | null = null;
  if (items.length && geometrySafe && !data.right && mode === "donut") {
    option = {
      color: theme.palette,
      tooltip: { trigger: "item", formatter: params => {
        const point = Array.isArray(params) ? params[0] : params;
        const item = left.find(entry => entry.key === point?.name);
        return item ? joinTooltipLines([`${label(item.key)}: ${amount(item.amount, currency)} · ${item.share}%`]) : "";
      } },
      series: [{ id: "historical-allocation", type: "pie", radius: ["45%", "72%"], label: { show: false }, data: left.filter(item => !isZero(item.amount)).map(item => ({ name: item.key, value: geometry(item.amount)! })) }],
    };
  } else if (items.length && geometrySafe && !allAmountsZero && !mismatch) {
    option = {
      grid: { left: 12, right: 24, top: 15, bottom: 28, containLabel: true },
      tooltip: { trigger: "item", formatter: params => {
        const point = Array.isArray(params) ? params[0] : params;
        const datum = point?.data;
        const key = typeof datum === "object" && datum !== null && "name" in datum ? String(datum.name) : point?.name ?? "";
        const side = data.right && point?.seriesName === "B" ? "right" : "left";
        const item = data.right ? paired.find(entry => entry.key === key)?.[side] : left.find(entry => entry.key === key);
        const state = side === "right" ? data.right! : data.left;
        const sideLabel = data.right ? `${stateLabel(state, side, t("historicalOverview.current"))}${state.current ? ` · ${data.capturedAt}` : ""}` : state.date;
        return joinTooltipLines([`${label(key)} · ${sideLabel}`, item ? `${item.amount} ${state.currency} · ${item.share}%` : t("historicalOverview.notReturned")]);
      } },
      xAxis: { type: "value", scale: false, axisLine: { onZero: true, lineStyle: { color: theme.border } }, axisLabel: { color: theme.muted }, splitLine: { lineStyle: { color: theme.border } } },
      yAxis: { type: "category", data: items.map(label), inverse: true, axisLabel: { color: theme.foreground, width: 110, overflow: "truncate" } },
      series: data.right ? [
        { id: "historical-A", name: "A", type: "bar", barMaxWidth: 17, itemStyle: { color: theme.border, borderColor: theme.primary, borderWidth: 1 }, data: paired.map(item => item.left ? { name: item.key, value: geometry(item.left.amount)! } : null) },
        { id: "historical-B", name: "B", type: "bar", barMaxWidth: 17, itemStyle: { color: theme.primary }, data: paired.map(item => item.right ? { name: item.key, value: geometry(item.right.amount)! } : null) },
      ] : [{ id: "historical-single", type: "bar", barMaxWidth: 22, itemStyle: { color: theme.primary }, data: left.map(item => ({ name: item.key, value: geometry(item.amount)! })) }],
    };
  }
  const rows = data.right ? paired.map(item => [label(item.key), item.left?.amount ?? t("historicalOverview.notReturned"), item.left?.share == null ? "—" : `${item.left.share}%`, item.right?.amount ?? t("historicalOverview.notReturned"), item.right?.share == null ? "—" : `${item.right.share}%`])
    : left.map(item => [label(item.key), item.amount, `${item.share}%`]);
  return <section aria-label={t(data.right ? "historicalOverview.allocationCompare" : "historicalOverview.allocation")} className="min-w-0 rounded-xl border border-border bg-card p-4">
    <h2 className="font-semibold">{t(data.right ? "historicalOverview.allocationCompare" : "historicalOverview.allocation")}</h2>
    <p className="mt-1 text-xs text-muted-foreground">{t("historicalOverview.allocationBasis")} · {t("historicalOverview.assetOnly")} · {currency}</p>
    <div className="my-3 flex flex-wrap gap-2">{(["byClass", "byCurrency"] as const).map(value => <Button key={value} size="sm" variant={dimension === value ? "default" : "outline"} aria-pressed={dimension === value} onClick={() => setDimension(value)}>{t(`historicalOverview.${value}`)}</Button>)}</div>
    {data.right && <div className="mb-3 flex flex-wrap gap-4 text-sm" aria-label={t("historicalOverview.comparisonLegend")}>
      <span className="flex items-center gap-2"><span aria-hidden="true" className="inline-block size-4 rounded border" style={{ backgroundColor: theme.border, borderColor: theme.primary }} />{stateLabel(data.left, "left", t("historicalOverview.current"))}</span>
      <span className="flex items-center gap-2"><span aria-hidden="true" className="inline-block size-4 rounded" style={{ backgroundColor: theme.primary }} />{stateLabel(data.right, "right", t("historicalOverview.current"))}{data.right.current && <> · <time dateTime={data.capturedAt}>{data.capturedAt}</time></>}</span>
    </div>}
    {data.left.assets == null && <p className="text-sm text-warning-foreground">{t("historicalOverview.valuedOnly", { count: missingAssetLeaves(data, "left") })}</p>}
    {data.right && data.right.assets == null && <p className="text-sm text-warning-foreground">{t("historicalOverview.valuedOnlyB", { count: missingAssetLeaves(data, "right") })}</p>}
    {mismatch && <p className="text-sm text-warning-foreground">{t("historicalOverview.currencyMismatch")}</p>}
    {items.length === 0 && mode !== "zero" && <p className="text-sm text-muted-foreground">{t("historicalOverview.noValuedAssets")}</p>}
    {!data.right && mode === "zero" && <p className="text-sm">{t("historicalOverview.zeroKnownAssets")}</p>}
    {data.right && allAmountsZero && <p className="text-sm">{t("historicalOverview.zeroBothAssets")}</p>}
    {!option && items.length > 0 && !mismatch && !allAmountsZero && <p className="text-sm text-muted-foreground">{t("historicalOverview.geometryUnavailable")}</p>}
    {option && <EChart key={`${dimension}-${data.right ? "paired" : mode}`} option={option} height={Math.max(220, items.length * (data.right ? 55 : 38) + 60)} ariaLabel={t(data.right ? "historicalOverview.allocationCompare" : "historicalOverview.allocation")} summary={t("historicalOverview.chartSummary", { currency })}
      center={mode === "donut" && !data.right ? <><span className="text-xs text-muted-foreground">{t("historicalOverview.valuedAssets")}</span><strong className="num text-sm">{amount(data.left.knownAssets, currency)}</strong></> : undefined}
      onSelectName={name => { const item = items.find(key => label(key) === name || key === name); if (item) onFilter(dimension, item, categoryButtons.current.get(item)); }} />}
    <div className="mt-3 flex flex-wrap gap-2">{items.map(key => <Button key={key} id={`historical-category-asset-${dimension}-${encodeURIComponent(key)}`} ref={node => { if (node) categoryButtons.current.set(key, node); else categoryButtons.current.delete(key); }} size="sm" variant="outline" onClick={event => onFilter(dimension, key, event.currentTarget)}>{label(key)}</Button>)}</div>
    <details className="mt-3" open={!option}><summary className="cursor-pointer text-sm text-primary">{t("historicalOverview.fullChartData")}</summary>
      <div className="mt-2 overflow-x-auto"><table className="w-full text-left text-sm"><thead><tr><th scope="col">{t(`historicalOverview.${dimension}`)}</th><th scope="col">{data.right ? "A · " : `${data.left.date} · `}{currency}</th><th scope="col">{data.right ? "A · " : `${data.left.date} · `}%</th>{data.right && <><th scope="col">B · {data.right.currency}</th><th scope="col">B · %</th></>}</tr></thead><tbody>{rows.map((row, index) => <tr key={items[index]} className="border-t border-border">{row.map((value, cell) => <td key={cell} className="num p-2" title={value}>{value}</td>)}</tr>)}</tbody></table></div>
    </details>
  </section>;
}

export function HistoricalAccountDeltaView({ data, onOpen }: { data: HistoricalOverviewResult; onOpen: (key: string, trigger: HTMLButtonElement) => void }) {
  const { t } = useTranslation();
  const rowButtons = useRef(new Map<string, HTMLButtonElement>());
  useThemeVersion();
  const theme = chartTheme();
  const [role, setRole] = useState<"asset" | "liability">("asset");
  const [all, setAll] = useState(false);
  const mismatch = data.left.currency !== data.right?.currency;
  const classified = accountChanges(data.rows ?? [], role);
  const comparable = mismatch ? [] : classified.comparable;
  const separate = mismatch ? [...classified.comparable, ...classified.separate] : classified.separate;
  const visible = all ? comparable : comparable.slice(0, 10);
  const safe = !mismatch && visible.every(row => geometry(historicalRowChange(row)) !== null);
  const option: EChartsOption = {
    grid: { left: 12, right: 24, top: 15, bottom: 28, containLabel: true },
    xAxis: { type: "value", scale: false, axisLine: { onZero: true, lineStyle: { color: theme.border } }, axisLabel: { color: theme.muted }, splitLine: { lineStyle: { color: theme.border } } },
    yAxis: { type: "category", data: visible.map(row => row.name), inverse: true, axisLabel: { color: theme.foreground, width: 110, overflow: "truncate" } },
    series: [{ id: "historical-account-delta", type: "bar", barMaxWidth: 23, itemStyle: { color: theme.primary }, data: visible.map(row => ({ name: row.key, value: geometry(historicalRowChange(row))! })) }],
  };
  const rowView = (row: HistoricalOverviewRow) => <li key={row.key} className="min-w-0 rounded-lg border border-border p-3">
    {row.estimatedBaseChange != null && (row.left?.displayValuation?.quality?.coverage !== "verified" || row.right?.displayValuation?.quality?.coverage !== "verified") && <p className="text-xs">{t("valuation.estimatedChange")}</p>}
 <Button ref={button => { if (button) rowButtons.current.set(row.key, button); else rowButtons.current.delete(row.key); }} variant="link" className="h-auto min-w-0 whitespace-normal p-0 text-left" onClick={event => onOpen(row.key, event.currentTarget)}>{row.name}</Button>
    <dl className="mt-1 grid grid-cols-[repeat(auto-fit,minmax(min(100%,9rem),1fr))] gap-1 text-sm">
      <div><dt className="text-xs text-muted-foreground">A</dt><dd className="num overflow-x-auto whitespace-nowrap">{row.left ? historicalDisplayAmount(t,row.left.displayValuation,row.left.baseAmount==null ? null : {amount:row.left.baseAmount,currency:data.left.currency}) ?? t("historicalOverview.unknown") : t("historicalOverview.notPresent")}</dd></div>
      <div><dt className="text-xs text-muted-foreground">B</dt><dd className="num overflow-x-auto whitespace-nowrap">{row.right ? historicalDisplayAmount(t,row.right.displayValuation,row.right.baseAmount==null ? null : {amount:row.right.baseAmount,currency:data.right?.currency ?? data.left.currency}) ?? t("historicalOverview.unknown") : t("historicalOverview.notPresent")}</dd></div>
      <div><dt className="text-xs text-muted-foreground">{t("historicalOverview.deltaBA")}</dt><dd className="num overflow-x-auto whitespace-nowrap">{mismatch ? t("historicalOverview.notComparable") : amount(historicalRowChange(row), data.left.currency, true) ?? t("historicalOverview.notComparable")}</dd></div>
    </dl>
    {!row.left || !row.right ? <p className="text-xs text-muted-foreground">{t("historicalOverview.notPresent")}</p> : null}
    {row.left && row.right && row.left.included !== row.right.included && <Badge variant="warning">{t("historicalOverview.scopeChanged")}</Badge>}
    {row.left && row.right && !comparableStatuses(row.left.status, row.right.status) && <Badge variant="warning">{t("historicalOverview.statusChanged")}</Badge>}
    {row.baseChange && isZero(row.baseChange) && row.changed && <p className="text-xs text-muted-foreground">{t("historicalOverview.offsettingChanges")}</p>}
  </li>;
  return <section aria-label={t("historicalOverview.accountDeltaTitle")} className="min-w-0 rounded-xl border border-border bg-card p-4">
    <h2 className="font-semibold">{t("historicalOverview.accountDeltaTitle")}</h2><p className="text-xs text-muted-foreground">{t("historicalOverview.deltaNote")}</p>
    <div className="my-3 flex gap-2">{(["asset", "liability"] as const).map(value => <Button key={value} size="sm" variant={role === value ? "default" : "outline"} aria-pressed={role === value} onClick={() => { setRole(value); setAll(false); }}>{t(value === "asset" ? "overview.assets" : "overview.liabilities")}</Button>)}</div>
    {safe && visible.length > 0 && <EChart key={role} option={option} height={Math.max(220, visible.length * 38 + 50)} ariaLabel={t("historicalOverview.accountDeltaTitle")} summary={t("historicalOverview.accountDeltaSummary", { currency: data.left.currency })}
      onSelectName={key => { if (!key) return; const button = rowButtons.current.get(key); if (button?.isConnected) onOpen(key, button); }} />}
    {!safe && <p className="text-xs text-muted-foreground">{t(mismatch ? "historicalOverview.currencyMismatch" : "historicalOverview.geometryUnavailable")}</p>}
    <p className="my-2 text-xs">{t("historicalOverview.displayedCount", { shown: visible.length, total: comparable.length })}</p>
    {comparable.length > 10 && <Button variant="outline" size="sm" onClick={() => setAll(!all)}>{t(all ? "historicalOverview.topTen" : "historicalOverview.expandAll")}</Button>}
    <ul className="mt-3 grid gap-2 sm:grid-cols-2">{visible.map(rowView)}</ul>
    {separate.length > 0 && <div className="mt-4"><h3 className="mb-2 text-sm font-semibold">{t("historicalOverview.separateReview")}</h3><ul className="grid gap-2 sm:grid-cols-2">{separate.map(rowView)}</ul></div>}
    <details className="mt-3"><summary className="cursor-pointer text-sm text-primary">{t("historicalOverview.fullChartData")}</summary><div className="overflow-x-auto"><table className="w-full text-left text-sm"><thead><tr><th scope="col">{t("historicalOverview.account")}</th><th scope="col">A · {data.left.currency}</th><th scope="col">B · {data.right?.currency}</th><th scope="col">{t("historicalOverview.deltaBA")}</th></tr></thead><tbody>{[...comparable, ...separate].map(row => <tr key={row.key}><th scope="row">{row.name}</th><td>{row.left ? row.left.baseAmount ?? t("historicalOverview.unknown") : t("historicalOverview.notPresent")}</td><td>{row.right ? row.right.baseAmount ?? t("historicalOverview.unknown") : t("historicalOverview.notPresent")}</td><td>{mismatch ? t("historicalOverview.notComparable") : historicalRowChange(row) ?? t("historicalOverview.notComparable")}</td></tr>)}</tbody></table></div></details>
  </section>;
}
