import { Fragment, useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { EChart, type EChartsOption } from "@/components/charts/EChart";
import { chartTheme, joinTooltipLines, useThemeVersion } from "@/components/charts/chartTheme";
import { displayEnum } from "@/lib/display";
import { formatAmount, compareCanonical } from "@/lib/money";
import { geometry, isZero, missingAssetLeaves } from "./historicalChartModel";
import type { HistoricalCategoryCell, HistoricalCategoryComparison, HistoricalOverviewResult } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
import type { HistoricalCategoryFilter } from "@/app/navigation";

export function HistoricalCategoryComparisonView({ data, role, dimension, onRole, onDimension, onFilter }: {
  data: HistoricalOverviewResult; role: "asset" | "liability"; dimension: "byClass" | "byCurrency";
  onRole: (role: "asset" | "liability") => void; onDimension: (dimension: "byClass" | "byCurrency") => void;
  onFilter: (filter: HistoricalCategoryFilter, trigger: HTMLButtonElement) => void;
}) {
  const { t } = useTranslation();
  const rowsByKey = useMemo(() => new Map((data.rows ?? []).map(row => [row.key, row])), [data.rows]);
  const buttons = useRef(new Map<string, HTMLButtonElement>());
  useThemeVersion();
  const theme = chartTheme();
  const items = (data.categories ?? []).filter(item => item.role === role && item.dimension === dimension).sort((a, b) => {
    const x = a.right.amount; const y = b.right.amount;
    return x != null && y != null ? compareCanonical(y, x) || a.key.localeCompare(b.key) : x != null ? -1 : y != null ? 1 : a.key.localeCompare(b.key);
  });
  const label = (key: string) => key ? dimension === "byClass" ? displayEnum(t, "enum", key) || key : key : t("historicalOverview.unclassified");
  const currency = data.left.currency;
  const mismatch = currency !== data.right?.currency;
  const sideLabel = (side: "left" | "right") => `${side === "left" ? "A" : "B"} · ${data[side]?.current ? `${t("historicalOverview.capturedCurrent")} · ${data.capturedAt}` : data[side]?.date}`;
  const knownValues = items.flatMap(item => [item.left.amount, item.right.amount]).filter((value): value is string => value != null);
  const safe = !mismatch && knownValues.every(value => geometry(value) !== null);
  const allKnownZero = knownValues.length > 0 && knownValues.every(isZero);
  const zero = allKnownZero && items.every(item => item.left.amount != null && item.right.amount != null);
  const money = (value: string | null, signed = false, unit = currency) => value == null ? t("historicalOverview.notComparable") : `${signed && compareCanonical(value, "0.005") > 0 ? "+" : ""}${formatAmount(value, unit)}`;
  const points = (value: string | null) => value == null ? t("historicalOverview.notComparable") : t("historicalOverview.percentagePoints", { value: `${compareCanonical(value, "0") > 0 ? "+" : ""}${value}` });
  const reason = (value: string) => t(`historicalOverview.categoryReason.${value}`);
  const cellText = (cell: HistoricalCategoryCell, side: "left" | "right") => cell.amount == null ? reason(cell.status) : money(cell.amount, false, data[side]?.currency ?? currency);
  const knownSubtotal = (cell: HistoricalCategoryCell, side: "left" | "right") => cell.knownAmount == null ? t("historicalOverview.unknown") : money(cell.knownAmount, false, data[side]?.currency ?? currency);
  const missingRows = (cell: HistoricalCategoryCell) => (cell.missingRowKeys ?? []).map(key => {
    const row = rowsByKey.get(key);
    const parent = rowsByKey.get(row?.parentKey ?? "");
    const name = row?.kind === "cash" ? t("historicalOverview.cashCurrency", { currency: row.name }) : row?.name;
    return <li key={key} data-row-key={key}>{parent?.name && parent.name !== name ? `${parent.name} · ` : ""}{name || t("portfolio.unknownInstrument")}</li>;
  });
  const shareText = (cell: HistoricalCategoryCell) => cell.share == null ? "—" : `${cell.share}% · ${t(`historicalOverview.shareBasis.${cell.shareBasis}`)}`;
  const openCategory = (item: HistoricalCategoryComparison, trigger: HTMLButtonElement) => onFilter({ role, dimension, key: item.key }, trigger);
  const option: EChartsOption = {
    grid: { left: 12, right: 24, top: 15, bottom: 28, containLabel: true },
    tooltip: { trigger: "item", formatter: params => {
      const point = Array.isArray(params) ? params[0] : params;
      const item = items.find(item => item.key === point?.name);
      const side = point?.seriesName === "B" ? "right" : "left";
      return item ? joinTooltipLines([`${label(item.key)} · ${sideLabel(side)}`, cellText(item[side], side), shareText(item[side]), `${t("historicalOverview.deltaBA")}: ${money(item.amountChange, true)}`, `${t("historicalOverview.shareDelta")}: ${points(item.shareChangePp)}`]) : "";
    } },
    xAxis: { type: "value", scale: false, axisLine: { onZero: true, lineStyle: { color: theme.border } }, axisLabel: { color: theme.muted }, splitLine: { lineStyle: { color: theme.border } } },
    yAxis: { type: "category", inverse: true, data: items.map(item => label(item.key)), axisLabel: { color: theme.foreground, width: 110, overflow: "truncate" } },
    series: (["left", "right"] as const).map(side => ({ id: `historical-category-${side}`, name: side === "left" ? "A" : "B", type: "bar", barMaxWidth: 17,
      itemStyle: side === "left" ? { color: theme.border, borderColor: theme.primary, borderWidth: 1 } : { color: theme.primary },
      data: items.map(item => item[side].amount == null ? null : { name: item.key, value: geometry(item[side].amount)! }),
    })),
  };
  return <section aria-label={t("historicalOverview.categoryCompare")} className="min-w-0 rounded-xl border border-border bg-card p-4">
    <h2 className="font-semibold">{t("historicalOverview.categoryCompare")}</h2>
    <p className="mt-1 text-xs text-muted-foreground">{t("historicalOverview.categoryComparisonBasis")} · {currency} · {t("historicalOverview.deltaNote")}</p>
    <div className="my-3 flex flex-wrap gap-2" role="group" aria-label={t("historicalOverview.categoryRole")}>{(["asset", "liability"] as const).map(value => <Button key={value} size="sm" variant={role === value ? "default" : "outline"} aria-pressed={role === value} onClick={() => onRole(value)}>{t(value === "asset" ? "overview.assets" : "overview.liabilities")}</Button>)}</div>
    <div className="my-3 flex flex-wrap gap-2">{(["byClass", "byCurrency"] as const).map(value => <Button key={value} size="sm" variant={dimension === value ? "default" : "outline"} aria-pressed={dimension === value} onClick={() => onDimension(value)}>{t(`historicalOverview.${value}`)}</Button>)}</div>
    {role === "liability" && <p className="text-xs text-muted-foreground">{t("historicalOverview.debtCategoryNote")}</p>}
    <div className="my-3 flex flex-wrap gap-4 text-sm" aria-label={t("historicalOverview.comparisonLegend")}>
      <span className="flex items-center gap-2"><span aria-hidden className="inline-block size-4 rounded border" style={{ backgroundColor: theme.border, borderColor: theme.primary }} />{sideLabel("left")}</span>
      <span className="flex items-center gap-2"><span aria-hidden className="inline-block size-4 rounded" style={{ backgroundColor: theme.primary }} />{sideLabel("right")}</span>
    </div>
    {role === "asset" && data.left.assets == null && <p className="text-sm text-warning-foreground">{t("historicalOverview.valuedOnly", { count: missingAssetLeaves(data, "left") })}</p>}
    {role === "asset" && data.right && data.right.assets == null && <p className="text-sm text-warning-foreground">{t("historicalOverview.valuedOnlyB", { count: missingAssetLeaves(data, "right") })}</p>}
    {mismatch && <p className="text-sm text-warning-foreground">{t("historicalOverview.currencyMismatch")}</p>}
    {items.length === 0 && <p className="text-sm">{t("historicalOverview.noCategories")}</p>}
    {allKnownZero && <p className="text-sm">{t(zero ? "historicalOverview.zeroBothAssets" : "historicalOverview.zeroAvailableCategories")}</p>}
    {!safe && !mismatch && <p className="text-sm">{t("historicalOverview.geometryUnavailable")}</p>}
    {items.length > 0 && knownValues.length === 0 && <p className="text-sm">{t("historicalOverview.noComparableCategories")}</p>}
    {safe && !allKnownZero && knownValues.length > 0 && <EChart key={`${role}-${dimension}`} option={option} height={Math.max(220, items.length * 55 + 60)} ariaLabel={t("historicalOverview.categoryCompare")} summary={`${sideLabel("left")} → ${sideLabel("right")} · ${currency}`}
      onSelectName={key => { const item = items.find(item => item.key === key); const button = buttons.current.get(key ?? ""); if (item && button?.isConnected) openCategory(item, button); }} />}
    <ul className="mt-3 grid gap-3 sm:grid-cols-2">{items.map(item => <li key={item.key} className="min-w-0 rounded-lg border border-border p-3">
      <Button id={`historical-category-${role}-${dimension}-${encodeURIComponent(item.key)}`} ref={node => { if (node) buttons.current.set(item.key, node); else buttons.current.delete(item.key); }} variant="link" className="h-auto whitespace-normal p-0 text-left" onClick={event => openCategory(item, event.currentTarget)}>{label(item.key)}</Button>
      <dl className="mt-2 grid gap-2 text-sm"><div><dt>A</dt><dd className="num overflow-x-auto whitespace-nowrap">{cellText(item.left, "left")}</dd><dd>{shareText(item.left)}</dd></div><div><dt>B</dt><dd className="num overflow-x-auto whitespace-nowrap">{cellText(item.right, "right")}</dd><dd>{shareText(item.right)}</dd></div>
        <div><dt>{t("historicalOverview.deltaBA")}</dt><dd className="num overflow-x-auto whitespace-nowrap">{mismatch ? t("historicalOverview.notComparable") : money(item.amountChange, true)}</dd></div>
        <div><dt>{t("historicalOverview.shareDelta")}</dt><dd>{mismatch ? t("historicalOverview.notComparable") : points(item.shareChangePp)}</dd></div>
      </dl>
      {(["left", "right"] as const).map(side => item[side].status === "partial" && <p key={side} className="mt-2 text-xs text-warning-foreground">{side === "left" ? "A" : "B"} · {t("historicalOverview.knownSubtotal")}: {knownSubtotal(item[side], side)} · {t("historicalOverview.categoryMissing", { count: (item[side].missingRowKeys ?? []).length })}</p>)}
      <div className="mt-2 flex flex-wrap gap-1">{[...new Set([...(item.amountReasons ?? []), ...(item.shareReasons ?? [])])].map(code => <Badge key={code} variant="warning">{reason(code)}</Badge>)}</div>
    </li>)}</ul>
    <details className="mt-3" open={!safe || allKnownZero || knownValues.length === 0}><summary className="cursor-pointer text-sm text-primary">{t("historicalOverview.fullChartData")}</summary>
      <div className="mt-2 overflow-x-auto"><table className="w-full text-left text-sm"><thead><tr><th scope="col">{t(`historicalOverview.${dimension}`)}</th><th scope="col">{sideLabel("left")} · {currency}</th><th scope="col">A · %</th><th scope="col">{sideLabel("right")} · {data.right?.currency}</th><th scope="col">B · %</th><th scope="col">{t("historicalOverview.deltaBA")}</th><th scope="col">{t("historicalOverview.shareDelta")}</th><th scope="col">{t("historicalOverview.comparability")}</th></tr></thead><tbody>{items.map(item => <tr key={item.key} className="border-t border-border"><th scope="row">{label(item.key)}</th>{(["left", "right"] as const).map(side => <Fragment key={side}><td className="p-2">{cellText(item[side], side)}{item[side].status === "partial" && <> · {t("historicalOverview.knownSubtotal")}: {knownSubtotal(item[side], side)}<ul className="mt-1">{missingRows(item[side])}</ul></>}</td><td className="p-2">{shareText(item[side])}</td></Fragment>)}<td className="num p-2">{mismatch ? t("historicalOverview.notComparable") : money(item.amountChange, true)}</td><td className="p-2">{mismatch ? t("historicalOverview.notComparable") : points(item.shareChangePp)}</td><td className="p-2">{mismatch ? t("historicalOverview.currencyMismatch") : [...new Set([...(item.amountReasons ?? []), ...(item.shareReasons ?? [])])].map(reason).join(" · ") || t("historicalOverview.comparable")}</td></tr>)}</tbody></table></div>
    </details>
  </section>;
}
