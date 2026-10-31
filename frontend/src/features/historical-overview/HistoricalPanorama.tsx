import { signedAccountAmount, signedAccountValuation, negateDisplayAmount } from "@/lib/liabilityDisplay";
import { ValuationQualitySummary } from "@/components/valuation/ValuationQualitySummary";
import { displayQualityEstimated, fullDisplayAmount, hasDisplay } from "@/components/valuation/valuationQuality";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { EChart, type EChartsOption } from "@/components/charts/EChart";
import { chartTheme, joinTooltipLines, useThemeVersion } from "@/components/charts/chartTheme";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { InfoPopover } from "@/components/ui/info-popover";
import { cn } from "@/lib/utils";
import { CheckCircle2, TriangleAlert } from "lucide-react";
import { SignedChange } from "./SignedChange";
import { displayEnum } from "@/lib/display";
import type { HistoricalOverviewResult, HistoricalOverviewRow, HistoricalOverviewState, HistoricalSeriesPoint } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
import { historicalRowChange, accountChanges, alignedAllocations, allocationMode, comparableStatuses, geometry, isZero, missingAssetLeaves } from "./historicalChartModel";
import { formatCivilDate, formatTimestamp } from "@/lib/time";
import { historicalDisplayAmount, formatHistoricalAmount as amount } from "./historicalAmount";

type Dimension = "byClass" | "byCurrency";
type Side = "left" | "right";

function stateLabel(state: HistoricalOverviewState, side: Side, current: string, compare = true) {
  const when = state.current ? current : formatCivilDate(state.date);
  return compare ? `${side === "left" ? "A" : "B"} · ${when}` : when;
}

function valuationEstimated(value?: Parameters<typeof hasDisplay>[0]) {
  return hasDisplay(value) && displayQualityEstimated(value.quality);
}

export function HistoricalMetricCards({ data, seriesPoints }: { data: HistoricalOverviewResult; seriesPoints?: HistoricalSeriesPoint[] | null }) {
  const { t } = useTranslation();
  const { left, right } = data;
  const change = data.estimatedChange ?? data.change;
  const mismatch = Boolean(right && left.currency !== right.currency);
  const estimated = (key: "netWorth" | "assets" | "liabilities") => [left, right].some(state => valuationEstimated(state?.displayValuation?.[key]));
  return <div className="grid min-w-0 gap-3 min-[700px]:grid-cols-2 min-[1100px]:grid-cols-3">
    {(["netWorth", "assets", "liabilities"] as const).map(key => {
      const subtotalKey = key === "assets" ? "knownAssets" : "knownLiabilities";
      const side = (which: Side) => {
        const state = which === "left" ? left : right!;
        const point = seriesPoints?.find(point => point.date === state.date);
        const known = point ? point[subtotalKey] : state[subtotalKey];
        return { state, known };
      };
      const headline = side(right ? "right" : "left");
      const baseline = right ? side("left") : null;
      const rawChange = change?.[key];
      const changeValue = key === "liabilities" && rawChange != null ? negateDisplayAmount(rawChange) : rawChange;
      const displayAmount = (state: HistoricalOverviewState) => historicalDisplayAmount(t, signedAccountValuation(state.displayValuation?.[key], key === "liabilities"), state[key] == null ? null : { amount: signedAccountAmount(state[key], key === "liabilities"), currency: state.currency }, key === "netWorth", false);
      const showKnown = (which: { state: HistoricalOverviewState; known: string | null }) => fullDisplayAmount(which.state.displayValuation?.[key], which.state[key]) == null && !which.state.displayValuation && key !== "netWorth";
      return <Card key={key} className="min-w-0">
        <CardHeader className="flex-row items-center justify-between gap-2 p-4 pb-1">
          <CardTitle className="text-sm text-muted-foreground">{t(`overview.${key}`)}</CardTitle>
          {mismatch && <Badge variant="warning">{t("historicalOverview.currencyMismatch")}</Badge>}
        </CardHeader>
        <CardContent className="p-4 pt-0">
          <dl className="grid grid-cols-[repeat(auto-fit,minmax(min(100%,12rem),1fr))] gap-1">
            <div className="min-w-0">
              <dt className={right ? "text-xs text-muted-foreground" : "sr-only"}>{stateLabel(headline.state, right ? "right" : "left", t("historicalOverview.current"), Boolean(right))}</dt>
              <dd tabIndex={0} aria-label={t(`overview.${key}`)} className="mt-0.5 overflow-x-auto text-2xl font-bold tracking-tight focus-visible:outline-primary">
                <span className="num whitespace-nowrap" title={displayAmount(headline.state)}>{displayAmount(headline.state)}</span>
              </dd>
              <ValuationQualitySummary value={headline.state.displayValuation?.[key]} current={headline.state.current} net={key === "netWorth"} compact />
              {showKnown(headline) && <p className="text-xs text-muted-foreground">{t("historicalOverview.knownSubtotal")}: {amount(headline.known == null ? null : signedAccountAmount(headline.known, key === "liabilities"), headline.state.currency) ?? t("historicalOverview.unknown")}</p>}
            </div>
            {right && !mismatch && <div className="min-w-0">
              <dt className="sr-only">{estimated(key) ? t("valuation.estimatedMetricChange") : t("historicalOverview.deltaBA")}</dt>
              <dd tabIndex={0} className="flex items-center gap-2 overflow-x-auto text-base font-semibold focus-visible:outline-primary">
                {changeValue == null ? <span className="text-sm font-normal text-muted-foreground">{t("historicalOverview.unknown")}</span>
                  : <SignedChange value={changeValue} currency={left.currency} neutral={key === "liabilities"} />}
                {changeValue != null && estimated(key) && <Badge variant="warning" title={t("valuation.estimatedMetricChange")}>{t("historicalOverview.estimatedShort")}</Badge>}
              </dd>
            </div>}
            {baseline && <div className="min-w-0 border-t border-border pt-1.5">
              <dt className="text-xs text-muted-foreground">{stateLabel(baseline.state, "left", t("historicalOverview.current"))}</dt>
              <dd tabIndex={0} aria-label={t(`overview.${key}`)} className="overflow-x-auto text-sm font-medium focus-visible:outline-primary">
                <span className="num whitespace-nowrap" title={displayAmount(baseline.state)}>{displayAmount(baseline.state)}</span>
              </dd>
              <ValuationQualitySummary value={baseline.state.displayValuation?.[key]} current={baseline.state.current} net={key === "netWorth"} compact />
              {showKnown(baseline) && <p className="text-xs text-muted-foreground">{t("historicalOverview.knownSubtotal")}: {amount(baseline.known == null ? null : signedAccountAmount(baseline.known, key === "liabilities"), baseline.state.currency) ?? t("historicalOverview.unknown")}</p>}
            </div>}
          </dl>
          {key === "liabilities" && right && <p className="mt-2 flex items-center gap-1 text-xs text-muted-foreground">{t("historicalOverview.debtShort")}<InfoPopover label={t("historicalOverview.aboutChange")}>{t("historicalOverview.debtBalanceNote")}</InfoPopover></p>}
        </CardContent>
      </Card>;
    })}
  </div>;
}

export function HistoricalCoverage({ data, onDetails }: { data: HistoricalOverviewResult; onDetails: (trigger: HTMLButtonElement) => void }) {
  const { t } = useTranslation();
  const sides: Side[] = data.right ? ["left", "right"] : ["left"];
  const full = (state: HistoricalOverviewState) => state.displayValuation
    ? (["netWorth", "assets", "liabilities"] as const).every(key => hasDisplay(state.displayValuation?.[key]) && state.displayValuation[key].quality.availability === "full" && state.displayValuation[key].total != null)
    : state.complete;
  const estimated = (state: HistoricalOverviewState) => full(state) && (!state.complete || (["netWorth", "assets", "liabilities"] as const).some(key => valuationEstimated(state.displayValuation?.[key])));
  const complete = sides.every(side => full(data[side]!)) && !(data.right && data.left.currency !== data.right.currency);
  const verified = complete && !sides.some(side => estimated(data[side]!));
  return <div role="status" className={cn("flex flex-wrap items-center justify-between gap-2 rounded-xl border px-3 py-2 text-sm", verified ? "border-border bg-card text-muted-foreground" : "border-warning/40 bg-warning/12 text-warning-foreground")}>
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
      {verified ? <CheckCircle2 className="size-4 text-success" aria-hidden="true" /> : <TriangleAlert className="size-4" aria-hidden="true" />}
      {sides.map(side => {
        const state = data[side]!;
        const count = missingAssetLeaves(data, side);
        return <span key={side}>{stateLabel(state, side, t("historicalOverview.current"), Boolean(data.right))}: {t(!full(state) ? "historicalOverview.coveragePartial" : estimated(state) ? "historicalOverview.coverageEstimated" : "historicalOverview.coverageComplete")}
          {count > 0 && !full(state) && ` · ${t("historicalOverview.missingAssetLeaves", { count })}`}
        </span>;
      })}
      {data.right && data.left.currency !== data.right.currency && <Badge variant="warning">{t("historicalOverview.currencyMismatch")}</Badge>}
    </div>
    <Button variant={verified ? "link" : "outline"} size="sm" onClick={event => onDetails(event.currentTarget)}>{t("historicalOverview.viewEvidence")}</Button>
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
      series: [{ id: "historical-allocation", type: "pie", radius: ["62%", "84%"], padAngle: 1, itemStyle: { borderRadius: 3 }, label: { show: false }, data: left.filter(item => !isZero(item.amount)).map(item => ({ name: item.key, value: geometry(item.amount)! })) }],
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
        const sideLabel = data.right ? `${stateLabel(state, side, t("historicalOverview.current"))}${state.current ? ` · ${formatTimestamp(data.capturedAt, data.timezone)}` : ""}` : formatCivilDate(state.date);
        return joinTooltipLines([`${label(key)} · ${sideLabel}`, item ? `${amount(item.amount, state.currency)} · ${item.share}%` : t("historicalOverview.notReturned")]);
      } },
      xAxis: { type: "value", scale: false, axisLine: { onZero: true, lineStyle: { color: theme.border } }, axisLabel: { color: theme.muted }, splitLine: { lineStyle: { color: theme.border } } },
      yAxis: { type: "category", data: items.map(label), inverse: true, axisLabel: { color: theme.foreground, width: 110, overflow: "truncate" } },
      series: data.right ? [
        { id: "historical-A", name: "A", type: "bar", barMaxWidth: 17, itemStyle: { color: theme.border, borderColor: theme.primary, borderWidth: 1 }, data: paired.map(item => item.left ? { name: item.key, value: geometry(item.left.amount)! } : null) },
        { id: "historical-B", name: "B", type: "bar", barMaxWidth: 17, itemStyle: { color: theme.primary }, data: paired.map(item => item.right ? { name: item.key, value: geometry(item.right.amount)! } : null) },
      ] : [{ id: "historical-single", type: "bar", barMaxWidth: 22, itemStyle: { color: theme.primary }, data: left.map(item => ({ name: item.key, value: geometry(item.amount)! })) }],
    };
  }
  const rows = data.right ? paired.map(item => [label(item.key), (item.left ? amount(item.left.amount, data.left.currency) : null) ?? t("historicalOverview.notReturned"), item.left?.share == null ? "—" : `${item.left.share}%`, (item.right ? amount(item.right.amount, data.right!.currency) : null) ?? t("historicalOverview.notReturned"), item.right?.share == null ? "—" : `${item.right.share}%`])
    : left.map(item => [label(item.key), amount(item.amount, currency)!, `${item.share}%`]);
  return <section aria-label={t(data.right ? "historicalOverview.allocationCompare" : "historicalOverview.allocation")} className="min-w-0 rounded-xl border border-border bg-card p-4">
    <h2 className="font-semibold">{t(data.right ? "historicalOverview.allocationCompare" : "historicalOverview.allocation")}</h2>
    <p className="mt-1 text-xs text-muted-foreground">{t("historicalOverview.allocationBasis")} · {t("historicalOverview.assetOnly")} · {currency}</p>
    <SegmentedControl className="my-3" label={t("historicalOverview.dimension")} value={dimension} onChange={value => setDimension(value as Dimension)} options={(["byClass", "byCurrency"] as const).map(value => ({ value, label: t(`historicalOverview.${value}`) }))} />
    {data.right && <div className="mb-3 flex flex-wrap gap-4 text-sm" aria-label={t("historicalOverview.comparisonLegend")}>
      <span className="flex items-center gap-2"><span aria-hidden="true" className="inline-block size-4 rounded border" style={{ backgroundColor: theme.border, borderColor: theme.primary }} />{stateLabel(data.left, "left", t("historicalOverview.current"))}</span>
      <span className="flex items-center gap-2"><span aria-hidden="true" className="inline-block size-4 rounded" style={{ backgroundColor: theme.primary }} />{stateLabel(data.right, "right", t("historicalOverview.current"))}{data.right.current && <> · <time dateTime={data.capturedAt}>{formatTimestamp(data.capturedAt, data.timezone)}</time></>}</span>
    </div>}
    {data.left.assets == null && <p className="text-sm text-warning-foreground">{t("historicalOverview.valuedOnly", { count: missingAssetLeaves(data, "left") })}</p>}
    {data.right && data.right.assets == null && <p className="text-sm text-warning-foreground">{t("historicalOverview.valuedOnlyB", { count: missingAssetLeaves(data, "right") })}</p>}
    {mismatch && <p className="text-sm text-warning-foreground">{t("historicalOverview.currencyMismatch")}</p>}
    {items.length === 0 && mode !== "zero" && <p className="text-sm text-muted-foreground">{t("historicalOverview.noValuedAssets")}</p>}
    {!data.right && mode === "zero" && <p className="text-sm">{t("historicalOverview.zeroKnownAssets")}</p>}
    {data.right && allAmountsZero && <p className="text-sm">{t("historicalOverview.zeroBothAssets")}</p>}
    {!option && items.length > 0 && !mismatch && !allAmountsZero && <p className="text-sm text-muted-foreground">{t("historicalOverview.geometryUnavailable")}</p>}
    {option && <EChart key={`${dimension}-${data.right ? "paired" : mode}`} option={option} height={mode === "donut" && !data.right ? 280 : Math.max(220, items.length * (data.right ? 55 : 38) + 60)} ariaLabel={t(data.right ? "historicalOverview.allocationCompare" : "historicalOverview.allocation")} summary={t("historicalOverview.chartSummary", { currency })}
      center={mode === "donut" && !data.right ? <><span className="text-xs text-muted-foreground">{t("historicalOverview.valuedAssets")}</span><strong className="num max-w-40 text-sm font-semibold [overflow-wrap:anywhere]">{amount(data.left.knownAssets, currency)}</strong></> : undefined}
      onSelectName={name => { const item = items.find(key => label(key) === name || key === name); if (item) onFilter(dimension, item, categoryButtons.current.get(item)); }} />}
    <div className="mt-3 flex flex-wrap gap-2">{items.map(key => <Button key={key} id={`historical-category-asset-${dimension}-${encodeURIComponent(key)}`} ref={node => { if (node) categoryButtons.current.set(key, node); else categoryButtons.current.delete(key); }} size="sm" variant="outline" onClick={event => onFilter(dimension, key, event.currentTarget)}>{label(key)}</Button>)}</div>
    <details className="mt-3" open={!option}><summary className="cursor-pointer text-sm text-primary">{t("historicalOverview.fullChartData")}</summary>
      <div className="mt-2 overflow-x-auto"><table className="w-full text-left text-sm"><thead><tr><th scope="col">{t(`historicalOverview.${dimension}`)}</th><th scope="col">{data.right ? "A · " : `${formatCivilDate(data.left.date)} · `}{currency}</th><th scope="col">{data.right ? "A · " : `${formatCivilDate(data.left.date)} · `}%</th>{data.right && <><th scope="col">B · {data.right.currency}</th><th scope="col">B · %</th></>}</tr></thead><tbody>{rows.map((row, index) => <tr key={items[index]} className="border-t border-border">{row.map((value, cell) => <td key={cell} className="num p-2" title={value}>{value}</td>)}</tr>)}</tbody></table></div>
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
  const signedRowChange = (row: HistoricalOverviewRow) => {
    const value = historicalRowChange(row);
    return value == null ? null : row.role === "liability" ? negateDisplayAmount(value) : value;
  };
  const classified = accountChanges(data.rows ?? [], role);
  const comparable = mismatch ? [] : classified.comparable;
  const separate = mismatch ? [...classified.comparable, ...classified.separate] : classified.separate;
  const visible = all ? comparable : comparable.slice(0, 10);
  const safe = !mismatch && visible.every(row => geometry(signedRowChange(row)) !== null);
  const option: EChartsOption = {
    grid: { left: 12, right: 24, top: 15, bottom: 28, containLabel: true },
    xAxis: { type: "value", scale: false, axisLine: { onZero: true, lineStyle: { color: theme.border } }, axisLabel: { color: theme.muted }, splitLine: { lineStyle: { color: theme.border } } },
    yAxis: { type: "category", data: visible.map(row => row.name), inverse: true, axisLabel: { color: theme.foreground, width: 110, overflow: "truncate" } },
    series: [{ id: "historical-account-delta", type: "bar", barMaxWidth: 23, itemStyle: { color: theme.primary }, data: visible.map(row => {
      const value = geometry(signedRowChange(row))!;
      // Debt balance changes are not returns, so only asset bars carry gain/loss color.
      return { name: row.key, value, ...(role === "asset" ? { itemStyle: { color: value < 0 ? theme.gainNegative : theme.gainPositive } } : {}) };
    }) }],
  };
  const rowView = (row: HistoricalOverviewRow) => <li key={row.key} className="min-w-0 rounded-lg border border-border p-3">
    {row.estimatedBaseChange != null && (valuationEstimated(row.left?.displayValuation) || valuationEstimated(row.right?.displayValuation)) && <p className="text-xs">{t("valuation.estimatedChange")}</p>}
 <Button ref={button => { if (button) rowButtons.current.set(row.key, button); else rowButtons.current.delete(row.key); }} variant="link" className="h-auto min-w-0 whitespace-normal p-0 text-left" onClick={event => onOpen(row.key, event.currentTarget)}>{row.name}</Button>
    <dl className="mt-1 grid grid-cols-[repeat(auto-fit,minmax(min(100%,9rem),1fr))] gap-1 text-sm">
      <div><dt className="text-xs text-muted-foreground">A</dt><dd className="num overflow-x-auto whitespace-nowrap">{row.left ? historicalDisplayAmount(t,signedAccountValuation(row.left.displayValuation,row.role === "liability"),row.left.baseAmount==null ? null : {amount:signedAccountAmount(row.left.baseAmount,row.role === "liability"),currency:data.left.currency}) ?? t("historicalOverview.unknown") : t("historicalOverview.notPresent")}</dd></div>
      <div><dt className="text-xs text-muted-foreground">B</dt><dd className="num overflow-x-auto whitespace-nowrap">{row.right ? historicalDisplayAmount(t,signedAccountValuation(row.right.displayValuation,row.role === "liability"),row.right.baseAmount==null ? null : {amount:signedAccountAmount(row.right.baseAmount,row.role === "liability"),currency:data.right?.currency ?? data.left.currency}) ?? t("historicalOverview.unknown") : t("historicalOverview.notPresent")}</dd></div>
      <div><dt className="text-xs text-muted-foreground">{t("historicalOverview.deltaBA")}</dt><dd className="num overflow-x-auto whitespace-nowrap font-semibold">{mismatch || signedRowChange(row) == null ? t("historicalOverview.notComparable") : <SignedChange value={signedRowChange(row)} currency={data.left.currency} neutral={row.role === "liability"} />}</dd></div>
    </dl>
    {!row.left || !row.right ? <p className="text-xs text-muted-foreground">{t("historicalOverview.notPresent")}</p> : null}
    {row.left && row.right && row.left.included !== row.right.included && <Badge variant="warning">{t("historicalOverview.scopeChanged")}</Badge>}
    {row.left && row.right && !comparableStatuses(row.left.status, row.right.status) && <Badge variant="warning">{t("historicalOverview.statusChanged")}</Badge>}
    {row.baseChange && isZero(row.baseChange) && row.changed && <p className="text-xs text-muted-foreground">{t("historicalOverview.offsettingChanges")}</p>}
  </li>;
  return <section aria-label={t("historicalOverview.accountDeltaTitle")} className="min-w-0 rounded-xl border border-border bg-card p-4">
    <h2 className="font-semibold">{t("historicalOverview.accountDeltaTitle")}</h2><p className="text-xs text-muted-foreground">{t("historicalOverview.deltaNote")}</p>
    <SegmentedControl className="my-3" label={t("historicalOverview.categoryRole")} value={role} onChange={value => { setRole(value as typeof role); setAll(false); }} options={(["asset", "liability"] as const).map(value => ({ value, label: t(value === "asset" ? "overview.assets" : "overview.liabilities") }))} />
    {safe && visible.length > 0 && <EChart key={role} option={option} height={Math.max(220, visible.length * 38 + 50)} ariaLabel={t("historicalOverview.accountDeltaTitle")} summary={t("historicalOverview.accountDeltaSummary", { currency: data.left.currency })}
      onSelectName={key => { if (!key) return; const button = rowButtons.current.get(key); if (button?.isConnected) onOpen(key, button); }} />}
    {!safe && <p className="text-xs text-muted-foreground">{t(mismatch ? "historicalOverview.currencyMismatch" : "historicalOverview.geometryUnavailable")}</p>}
    <p className="my-2 text-xs">{t("historicalOverview.displayedCount", { shown: visible.length, total: comparable.length })}</p>
    {comparable.length > 10 && <Button variant="outline" size="sm" onClick={() => setAll(!all)}>{t(all ? "historicalOverview.topTen" : "historicalOverview.expandAll")}</Button>}
    <ul className="mt-3 grid gap-2 sm:grid-cols-2">{visible.map(rowView)}</ul>
    {separate.length > 0 && <div className="mt-4"><h3 className="mb-2 text-sm font-semibold">{t("historicalOverview.separateReview")}</h3><ul className="grid gap-2 sm:grid-cols-2">{separate.map(rowView)}</ul></div>}
    <details className="mt-3"><summary className="cursor-pointer text-sm text-primary">{t("historicalOverview.fullChartData")}</summary><div className="overflow-x-auto"><table className="w-full text-left text-sm"><thead><tr><th scope="col">{t("historicalOverview.account")}</th><th scope="col">A · {data.left.currency}</th><th scope="col">B · {data.right?.currency}</th><th scope="col">{t("historicalOverview.deltaBA")}</th></tr></thead><tbody>{[...comparable, ...separate].map(row => <tr key={row.key}><th scope="row">{row.name}</th><td>{row.left ? amount(row.left.baseAmount == null ? null : signedAccountAmount(row.left.baseAmount,row.role === "liability"), data.left.currency) ?? t("historicalOverview.unknown") : t("historicalOverview.notPresent")}</td><td>{row.right ? amount(row.right.baseAmount == null ? null : signedAccountAmount(row.right.baseAmount,row.role === "liability"), data.right!.currency) ?? t("historicalOverview.unknown") : t("historicalOverview.notPresent")}</td><td>{mismatch ? t("historicalOverview.notComparable") : amount(signedRowChange(row), data.left.currency, true) ?? t("historicalOverview.notComparable")}</td></tr>)}</tbody></table></div></details>
  </section>;
}
