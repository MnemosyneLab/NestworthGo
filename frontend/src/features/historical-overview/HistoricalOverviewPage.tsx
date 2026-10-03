import { Fragment, useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronDown, ChevronRight, History } from "lucide-react";
import { useHistoryOrigin } from "@/queries/history";
import { useHistoricalOverview } from "@/queries/historicalOverview";
import { lastClosedDate, parseYmd, ymd, ymdInTimeZone } from "@/features/insights/calendar";
import { PageChrome } from "@/components/layout/PageChrome";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/select";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription } from "@/components/ui/sheet";
import { formatAmount } from "@/lib/money";
import { displayEnum } from "@/lib/display";
import type { HistoricalOverviewCell, HistoricalOverviewEvidence, HistoricalOverviewState } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";

function previousMonthEnd(date: string) {
  const value = parseYmd(date);
  return ymd(new Date(value.getFullYear(), value.getMonth(), 0));
}

function shifted(date: string, days: number) {
  const value = parseYmd(date);
  value.setDate(value.getDate() + days);
  return ymd(value);
}

function Amount({ value, currency }: { value?: string | null; currency: string }) {
  const { t } = useTranslation();
  return <span className="num">{value == null ? t("historicalOverview.unknown") : formatAmount(value, currency)}</span>;
}

function CellValue({ cell, currency }: { cell?: HistoricalOverviewCell | null; currency: string }) {
  const { t } = useTranslation();
  if (!cell) return <span className="text-sm text-muted-foreground">{t("historicalOverview.notPresent")}</span>;
  return <div className="flex flex-col items-end gap-1">
    <Amount value={cell.baseAmount} currency={currency} />
    {cell.nativeAmount != null && cell.currency && cell.currency !== currency && <span className="text-xs text-muted-foreground"><Amount value={cell.nativeAmount} currency={cell.currency} /></span>}
    {cell.quantity != null && <span className="num text-xs">{t("historicalOverview.quantityValue", { value: cell.quantity })}</span>}
    <div className="flex flex-wrap justify-end gap-1">
      {cell.status !== "active" && <Badge variant={cell.status === "unknown" ? "warning" : "outline"}>{t(`historicalOverview.status.${cell.status}`)}</Badge>}
      {!cell.included && <Badge variant="outline">{t("historicalOverview.excluded")}</Badge>}
      {!cell.complete && <Badge variant="warning">{t("historicalOverview.incomplete")}</Badge>}
      {cell.manual && <Badge variant="outline">{t("historicalOverview.manual")}</Badge>}
      {[cell.price, cell.fx].some(e => e?.freshness === "stale") && <Badge variant="warning">{t("historicalOverview.stale")}</Badge>}
    </div>
  </div>;
}

function Summary({ state }: { state: HistoricalOverviewState }) {
  const { t } = useTranslation();
  return <Card>
    <CardHeader><CardTitle>{state.current ? t("historicalOverview.current") : state.date}</CardTitle><p className="text-xs text-muted-foreground">{t(state.current ? "historicalOverview.capturedCurrent" : "historicalOverview.closedSummary")}</p></CardHeader>
    <CardContent className="flex flex-col gap-4">
      <dl className="grid grid-cols-3 gap-3">{(["assets", "liabilities", "netWorth"] as const).map(key => <div key={key}><dt className="text-xs text-muted-foreground">{t(`overview.${key}`)}</dt><dd className="mt-1 break-all text-lg font-semibold"><Amount value={state[key]} currency={state.currency} /></dd></div>)}</dl>
      {!state.complete && <p role="status" className="text-sm text-warning-foreground">{t("historicalOverview.partialTotals", { assets: formatAmount(state.knownAssets, state.currency), liabilities: formatAmount(state.knownLiabilities, state.currency) })}</p>}
      <details><summary className="cursor-pointer text-sm font-medium">{t("historicalOverview.allocation")}</summary><p className="mt-2 text-xs text-muted-foreground">{t("historicalOverview.allocationBasis")}</p><div className="mt-3 grid gap-4 sm:grid-cols-2">{(["byClass", "byCurrency"] as const).map(dimension => <div key={dimension}><h3 className="mb-2 text-sm font-medium">{t(`historicalOverview.${dimension}`)}</h3><ul className="flex flex-col gap-2 text-sm">{(state[dimension] ?? []).map(item => <li key={item.key} className="flex justify-between gap-2"><span>{dimension === "byClass" ? (displayEnum(t, "enum", item.key) || item.key) : item.key}</span><span className="num text-right">{formatAmount(item.amount, state.currency)} · {item.share}%</span></li>)}</ul></div>)}</div></details>
    </CardContent>
  </Card>;
}

function Evidence({ title, value }: { title: string; value?: HistoricalOverviewEvidence | null }) {
  const { t } = useTranslation();
  if (!value) return null;
  return <div className="rounded-lg border border-border p-3"><h4 className="text-sm font-medium">{title}</h4><dl className="mt-2 space-y-1 text-xs"><div><dt className="text-muted-foreground">{t("historicalOverview.source")}</dt><dd>{t(`historicalOverview.sourceKind.${value.source}`, { defaultValue: value.source })} · {t(`historicalOverview.freshness.${value.freshness}`, { defaultValue: value.freshness })}</dd></div><div><dt className="text-muted-foreground">{t("historicalOverview.effectiveAt")}</dt><dd>{value.effectiveAt}</dd></div>{value.marketDate && <div><dt className="text-muted-foreground">{t("historicalOverview.marketDate")}</dt><dd>{value.marketDate}</dd></div>}</dl></div>;
}

function CellDetail({ cell, state }: { cell?: HistoricalOverviewCell | null; state: HistoricalOverviewState }) {
  const { t } = useTranslation();
  return <section className="flex flex-col gap-3"><h3 className="font-semibold">{state.current ? t("historicalOverview.current") : state.date}</h3><CellValue cell={cell} currency={state.currency} />{cell && <>
    {cell.nativeAmount != null && cell.currency && <p className="text-sm">{t("historicalOverview.original")}: <span className="num">{formatAmount(cell.nativeAmount)} {cell.currency}</span></p>}
    {cell.baseAmount != null && <p className="text-sm">{t("historicalOverview.exactBase")}: <span className="num">{formatAmount(cell.baseAmount)} {state.currency}</span></p>}
    {cell.valueSourceAt && <p className="text-sm">{t("historicalOverview.balanceSource")}: <time>{cell.valueSourceAt}</time></p>}
    {cell.manual && <p className="text-xs text-muted-foreground">{t("historicalOverview.manualNote")}</p>}
    {(cell.missing ?? []).length > 0 && <ul className="text-sm text-warning-foreground">{[...new Set(cell.missing ?? [])].map(reason => <li key={reason}>{t(`historicalOverview.missing.${reason}`)}</li>)}</ul>}
    <Evidence title={t("historicalOverview.price")} value={cell.price} /><Evidence title={t("historicalOverview.fx")} value={cell.fx} />
  </>}</section>;
}

export function HistoricalOverviewPage({ initialDate = "", onExit }: { initialDate?: string; onExit: () => void }) {
  const { t } = useTranslation();
  const origin = useHistoryOrigin();
  const [chosenDate, setChosenDate] = useState(initialDate);
  const [comparison, setComparison] = useState("none");
  const [comparisonDate, setComparisonDate] = useState("");
  const [changesOnly, setChangesOnly] = useState(false);
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const timezone = origin.data?.timezone;
  const firstDate = origin.data ? ymdInTimeZone(new Date(origin.data.startedAt), timezone) : "";
  const lastDate = lastClosedDate(timezone);
  const defaultDate = previousMonthEnd(ymdInTimeZone(new Date(), timezone));
  const date = chosenDate || (defaultDate < firstDate ? firstDate : defaultDate);
  const compareDate = comparisonDate || lastDate;
  const compareTo = comparison === "none" ? "" : comparison === "current" ? "current" : compareDate;
  const valid = Boolean(origin.data && firstDate <= lastDate && date >= firstDate && date <= lastDate && (comparison !== "date" || (compareDate >= firstDate && compareDate <= lastDate)));
  const query = useHistoricalOverview(date, compareTo, valid);
  const data = query.isFetching ? undefined : query.data;
  const selected = data?.rows?.find(row => row.key === selectedKey);
  const chooseDate = (value: string) => { setSelectedKey(null); setChosenDate(value); };
  const accounts = data?.rows?.filter(row => row.kind === "account" && (!changesOnly || !data.right || row.changed)) ?? [];
  const chrome = <PageChrome pageId="historical-overview" title={t("historicalOverview.title")} actions={<Button variant="outline" onClick={onExit}>{t("historicalOverview.exit")}</Button>} />;
  if (origin.isLoading) return <>{chrome}<LoadingState label={t("historicalOverview.loading")} /></>;
  if (origin.isError) return <>{chrome}<ErrorState title={t("historicalOverview.error")} description={t("historicalOverview.errorHint")} onRetry={() => void origin.refetch()} retryLabel={t("common.retryAction")} /></>;
  if (!origin.data) return <>{chrome}<EmptyState title={t("insights.noOrigin")} description={t("insights.noOriginHint")} /></>;
  if (firstDate > lastDate) return <>{chrome}<EmptyState title={t("historicalOverview.noClosedDays")} description={t("historicalOverview.noClosedDaysHint")} /></>;

  return <div className="flex flex-col gap-4">
    {chrome}
    <div className="rounded-xl border border-primary/25 bg-primary/5 p-4">
      <div className="flex flex-wrap items-center gap-2"><History className="size-4" aria-hidden="true" /><strong>{date}</strong><Badge variant="outline">{t("historicalOverview.readOnly")}</Badge><span className="text-sm">{timezone}</span></div>
      <p className="mt-2 text-sm">{t("historicalOverview.reconstructed")}</p>
      <p className="mt-1 text-xs text-muted-foreground">{t("historicalOverview.metadataNote")}</p>
    </div>
    <div className="flex flex-wrap items-end gap-3">
      <label className="flex flex-col gap-1 text-sm">{t("historicalOverview.date")}<Input type="date" value={date} min={firstDate} max={lastDate} onChange={event => chooseDate(event.target.value)} /></label>
      <Button variant="outline" disabled={date <= firstDate} onClick={() => chooseDate(shifted(date, -1))}>{t("historicalOverview.previous")}</Button>
      <Button variant="outline" disabled={date >= lastDate} onClick={() => chooseDate(shifted(date, 1))}>{t("historicalOverview.next")}</Button>
      <Button variant="ghost" onClick={() => chooseDate(defaultDate < firstDate ? firstDate : defaultDate)}>{t("historicalOverview.lastMonth")}</Button>
      <label className="flex flex-col gap-1 text-sm">{t("historicalOverview.compare")}<NativeSelect value={comparison} onChange={event => { setSelectedKey(null); setComparison(event.target.value); }}>{["none", "current", "date"].map(value => <option key={value} value={value}>{t(`historicalOverview.compareOptions.${value}`)}</option>)}</NativeSelect></label>
      {comparison === "date" && <label className="flex flex-col gap-1 text-sm">{t("historicalOverview.compareDate")}<Input type="date" value={compareDate} min={firstDate} max={lastDate} onChange={event => { setSelectedKey(null); setComparisonDate(event.target.value); }} /></label>}
      <Button variant="outline" disabled={!valid || query.isFetching} onClick={() => { setSelectedKey(null); void query.refetch(); }}>{t("historicalOverview.refresh")}</Button>
    </div>
    <p className="text-xs text-muted-foreground">{t("historicalOverview.cutoffNote")}</p>
    {!valid ? <EmptyState title={t("historicalOverview.invalidDate")} /> : query.isFetching ? <LoadingState label={t("historicalOverview.loading")} /> : query.isError ? <ErrorState title={t("historicalOverview.error")} description={t("historicalOverview.errorHint")} onRetry={() => void query.refetch()} retryLabel={t("common.retryAction")} /> : data && <>
      <p className="text-xs text-muted-foreground">{t("historicalOverview.captured", { at: data.capturedAt })}</p>
      <div className={`grid gap-4 ${data.right ? "xl:grid-cols-2" : ""}`}><Summary state={data.left} />{data.right && <Summary state={data.right} />}</div>
      {data.right && <div className="flex flex-wrap items-center justify-between gap-2"><label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={changesOnly} onChange={event => setChangesOnly(event.target.checked)} />{t("historicalOverview.changesOnly")}</label><p className="text-xs text-muted-foreground">{t("historicalOverview.deltaNote")}</p></div>}
      {accounts.length === 0 ? <EmptyState title={t(changesOnly && data.right ? "historicalOverview.noChanges" : "historicalOverview.noAccounts")} /> : <div className="overflow-x-auto rounded-xl border border-border"><table className="w-full text-sm"><caption className="sr-only">{t("historicalOverview.table")}</caption><thead className="bg-muted/60"><tr><th scope="col" className="p-3 text-left">{t("historicalOverview.account")}</th><th scope="col" className="p-3 text-right">{data.left.date}</th>{data.right && <><th scope="col" className="p-3 text-right">{data.right.current ? t("historicalOverview.current") : data.right.date}</th><th scope="col" className="p-3 text-right">{t("historicalOverview.change")}</th></>}</tr></thead><tbody>{accounts.map(account => <Fragment key={account.key}>
        <tr className="border-t border-border bg-card"><th scope="row" className="p-3 text-left"><div className="flex items-center gap-2"><Button variant="ghost" size="icon" aria-label={t("historicalOverview.expand", { name: account.name })} aria-expanded={expanded.has(account.key)} onClick={() => setExpanded(previous => { const next = new Set(previous); if (next.has(account.key)) next.delete(account.key); else next.add(account.key); return next; })}>{expanded.has(account.key) ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}</Button><Button variant="link" className="h-auto whitespace-normal p-0 text-left" onClick={() => setSelectedKey(account.key)}>{account.name}</Button>{account.role === "liability" && <Badge variant="outline">{t("historicalOverview.debt")}</Badge>}</div></th><td className="p-3"><CellValue cell={account.left} currency={data.left.currency} /></td>{data.right && <><td className="p-3"><CellValue cell={account.right} currency={data.right.currency} /></td><td className="p-3 text-right"><Amount value={account.baseChange} currency={data.left.currency} /></td></>}</tr>
        {expanded.has(account.key) && (data.rows ?? []).filter(row => row.parentKey === account.key && (!changesOnly || !data.right || row.changed)).map(row => <tr key={row.key} className="border-t border-border/60"><th scope="row" className="py-3 pl-12 pr-3 text-left font-normal"><Button variant="link" className="h-auto whitespace-normal p-0 text-left" onClick={() => setSelectedKey(row.key)}>{row.kind === "balance" ? t("historicalOverview.balance") : row.kind === "cash" ? t("historicalOverview.cashCurrency", { currency: row.name }) : row.name}</Button></th><td className="p-3"><CellValue cell={row.left} currency={data.left.currency} /></td>{data.right && <><td className="p-3"><CellValue cell={row.right} currency={data.right.currency} /></td><td className="p-3 text-right"><Amount value={row.baseChange} currency={data.left.currency} />{row.quantityChange != null && <p className="num text-xs">{t("historicalOverview.quantityChange", { value: row.quantityChange })}</p>}</td></>}</tr>)}
      </Fragment>)}</tbody></table></div>}
      <Sheet open={Boolean(selected)} onOpenChange={open => { if (!open) setSelectedKey(null); }}><SheetContent><SheetHeader><SheetTitle>{selected?.name}</SheetTitle><SheetDescription>{t("historicalOverview.readOnly")} · {t("historicalOverview.metadataNote")}</SheetDescription></SheetHeader>{selected && <><CellDetail cell={selected.left} state={data.left} />{data.right && <CellDetail cell={selected.right} state={data.right} />}{selected.nativeChange != null && <p className="text-sm">{t("historicalOverview.nativeChange")}: <Amount value={selected.nativeChange} currency={selected.left?.currency ?? ""} /></p>}{selected.quantityChange != null && <p className="text-sm num">{t("historicalOverview.quantityChange", { value: selected.quantityChange })}</p>}</>}</SheetContent></Sheet>
    </>}
  </div>;
}
