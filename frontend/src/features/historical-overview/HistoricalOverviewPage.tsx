import { Fragment, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { ChevronDown, ChevronRight, History } from "lucide-react";
import { useHistoryOrigin } from "@/queries/history";
import { useHistoricalOverview } from "@/queries/historicalOverview";
import { ymdInTimeZone } from "@/features/insights/calendar";
import { isClosedDate, previousMonthEnd, shiftCivilDate } from "./dates";
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
import type {
  HistoricalOverviewCell, HistoricalOverviewEvidence, HistoricalOverviewRow,
  HistoricalOverviewState, HistoricalOverviewTotalsChange,
} from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";

function Amount({ value, currency, signed = false, exact = false }: {
  value?: string | null; currency: string; signed?: boolean; exact?: boolean;
}) {
  const { t } = useTranslation();
  if (value == null) return <span>{t("historicalOverview.unknown")}</span>;
  const sign = signed && value !== "0" && !value.startsWith("-") ? "+" : "";
  return <span className="num" title={`${value} ${currency}`}>
    {sign}{formatAmount(value, exact ? undefined : currency)}{exact ? ` ${currency}` : ""}
  </span>;
}

function ObservationTime({ value, timezone }: { value: string; timezone: string }) {
  const { i18n } = useTranslation();
  const parsed = new Date(value);
  const text = Number.isFinite(parsed.getTime())
    ? new Intl.DateTimeFormat(i18n.language, {
      timeZone: timezone, year: "numeric", month: "short", day: "numeric",
      hour: "2-digit", minute: "2-digit", second: "2-digit", timeZoneName: "short",
    }).format(parsed)
    : value;
  return <time dateTime={value} title={value}>{text}</time>;
}

function rowLabel(row: HistoricalOverviewRow, t: TFunction): string {
  if (row.kind === "balance") return t("historicalOverview.balance");
  if (row.kind === "cash") return t("historicalOverview.cashCurrency", { currency: row.name });
  return row.name;
}

function CellValue({ cell, currency }: { cell?: HistoricalOverviewCell | null; currency: string }) {
  const { t } = useTranslation();
  if (!cell) return <span className="text-sm text-muted-foreground">{t("historicalOverview.notPresent")}</span>;
  return <div className="flex flex-col items-end gap-1">
    <span className="whitespace-nowrap"><Amount value={cell.baseAmount} currency={currency} /></span>
    {cell.nativeAmount != null && cell.currency && cell.currency !== currency && (
      <span className="whitespace-nowrap text-xs text-muted-foreground"><Amount value={cell.nativeAmount} currency={cell.currency} /></span>
    )}
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
    <CardHeader>
      <CardTitle>{state.current ? t("historicalOverview.current") : state.date}</CardTitle>
      <p className="text-xs text-muted-foreground">{t(state.current ? "historicalOverview.capturedCurrent" : "historicalOverview.closedSummary")}</p>
    </CardHeader>
    <CardContent className="flex flex-col gap-4">
      <dl className="grid gap-3 sm:grid-cols-3">
        {(["assets", "liabilities", "netWorth"] as const).map(key => <div key={key}>
          <dt className="text-xs text-muted-foreground">{t(`overview.${key}`)}</dt>
          <dd className="mt-1 break-words text-lg font-semibold"><Amount value={state[key]} currency={state.currency} /></dd>
        </div>)}
      </dl>
      {!state.complete && <p role="status" className="text-sm text-warning-foreground">
        {t("historicalOverview.partialTotals", { assets: formatAmount(state.knownAssets, state.currency), liabilities: formatAmount(state.knownLiabilities, state.currency) })}
      </p>}
      <details>
        <summary className="cursor-pointer text-sm font-medium">{t("historicalOverview.allocation")}</summary>
        <p className="mt-2 text-xs text-muted-foreground">{t("historicalOverview.allocationBasis")}</p>
        <div className="mt-3 grid gap-4 sm:grid-cols-2">
          {(["byClass", "byCurrency"] as const).map(dimension => <div key={dimension}>
            <h3 className="mb-2 text-sm font-medium">{t(`historicalOverview.${dimension}`)}</h3>
            {(state[dimension] ?? []).length === 0
              ? <p className="text-sm text-muted-foreground">{t("historicalOverview.noValuedAssets")}</p>
              : <ul className="flex flex-col gap-2 text-sm">{(state[dimension] ?? []).map(item => <li key={item.key} className="flex justify-between gap-2">
                <span>{dimension === "byClass" ? displayEnum(t, "enum", item.key) || item.key : item.key}</span>
                <span className="num text-right">{formatAmount(item.amount, state.currency)} · {item.share}%</span>
              </li>)}</ul>}
          </div>)}
        </div>
      </details>
    </CardContent>
  </Card>;
}

function TotalsChange({ change, currency }: { change: HistoricalOverviewTotalsChange; currency: string }) {
  const { t } = useTranslation();
  return <section aria-label={t("historicalOverview.totalChange")} className="rounded-xl border border-border bg-card p-4">
    <h2 className="text-sm font-semibold">{t("historicalOverview.totalChange")}</h2>
    <p className="mt-1 text-xs text-muted-foreground">{t("historicalOverview.deltaNote")}</p>
    <dl className="mt-3 grid gap-3 sm:grid-cols-3">
      {(["assets", "liabilities", "netWorth"] as const).map(key => <div key={key}>
        <dt className="text-xs text-muted-foreground">{t(`overview.${key}`)}</dt>
        <dd className="mt-1 break-words font-semibold"><Amount value={change[key]} currency={currency} signed /></dd>
      </div>)}
    </dl>
  </section>;
}

function RowChanges({ row, currency, exact = false }: { row: HistoricalOverviewRow; currency: string; exact?: boolean }) {
  const { t } = useTranslation();
  if (!row.left || !row.right) return <span className="text-xs text-muted-foreground">{t("historicalOverview.notComparable")}</span>;
  return <div className="flex flex-col gap-1 text-right">
    <span className="whitespace-nowrap"><Amount value={row.baseChange} currency={currency} signed exact={exact} /></span>
    {row.nativeChange != null && row.left.currency && row.left.currency !== currency && (
      <span className="whitespace-nowrap text-xs text-muted-foreground"><Amount value={row.nativeChange} currency={row.left.currency} signed exact={exact} /></span>
    )}
    {row.quantityChange != null && <span className="num text-xs">{t("historicalOverview.quantityChange", { value: row.quantityChange })}</span>}
  </div>;
}

function Evidence({ title, value, timezone }: { title: string; value?: HistoricalOverviewEvidence | null; timezone: string }) {
  const { t } = useTranslation();
  if (!value) return null;
  return <div className="rounded-lg border border-border p-3">
    <h4 className="text-sm font-medium">{title}</h4>
    <dl className="mt-2 space-y-2 break-words text-xs">
      <div><dt className="text-muted-foreground">{t("historicalOverview.source")}</dt>
        <dd>{t(`historicalOverview.sourceKind.${value.source}`, { defaultValue: value.source })} · {t(`historicalOverview.freshness.${value.freshness}`, { defaultValue: value.freshness })}</dd>
      </div>
      <div><dt className="text-muted-foreground">{t("historicalOverview.effectiveAt")}</dt>
        <dd><ObservationTime value={value.effectiveAt} timezone={timezone} /></dd>
      </div>
      {value.marketDate && <div><dt className="text-muted-foreground">{t("historicalOverview.marketDate")}</dt><dd>{value.marketDate}</dd></div>}
    </dl>
  </div>;
}

function CellDetail({ cell, state, timezone }: { cell?: HistoricalOverviewCell | null; state: HistoricalOverviewState; timezone: string }) {
  const { t } = useTranslation();
  return <section className="flex flex-col gap-3 border-t border-border pt-4">
    <h3 className="font-semibold">{state.current ? t("historicalOverview.current") : state.date}</h3>
    <p className="text-xs text-muted-foreground">
      {t(state.current ? "historicalOverview.captureTime" : "historicalOverview.dayCutoff")}: <ObservationTime value={state.cutoffAt} timezone={timezone} />
    </p>
    <CellValue cell={cell} currency={state.currency} />
    {cell && <>
      {cell.nativeAmount != null && cell.currency && <p className="text-sm">{t("historicalOverview.original")}: <Amount value={cell.nativeAmount} currency={cell.currency} exact /></p>}
      {cell.baseAmount != null && <p className="text-sm">{t("historicalOverview.exactBase")}: <Amount value={cell.baseAmount} currency={state.currency} exact /></p>}
      {cell.valueSourceAt && <p className="text-sm">{t("historicalOverview.balanceSource")}: <ObservationTime value={cell.valueSourceAt} timezone={timezone} /></p>}
      {cell.manual && cell.nativeAmount != null && <p className="text-xs text-muted-foreground">{t("historicalOverview.manualNote")}</p>}
      {(cell.missing ?? []).length > 0 && <ul className="text-sm text-warning-foreground">
        {[...new Set(cell.missing ?? [])].map(reason => <li key={reason}>{t(`historicalOverview.missing.${reason}`)}</li>)}
      </ul>}
      <Evidence title={t("historicalOverview.price")} value={cell.price} timezone={timezone} />
      <Evidence title={t("historicalOverview.fx")} value={cell.fx} timezone={timezone} />
    </>}
  </section>;
}

export function HistoricalOverviewPage({ initialDate, onExit }: { initialDate?: string; onExit: () => void }) {
  const { t } = useTranslation();
  const origin = useHistoryOrigin();
  // null means not chosen yet; an explicitly cleared date must remain empty.
  const [chosenDate, setChosenDate] = useState<string | null>(initialDate || null);
  const [comparison, setComparison] = useState("none");
  const [comparisonDate, setComparisonDate] = useState<string | null>(null);
  const [changesOnly, setChangesOnly] = useState(false);
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const dateInput = useRef<HTMLInputElement>(null);
  const detailTrigger = useRef<HTMLButtonElement | null>(null);
  const timezone = origin.data?.timezone ?? "UTC";
  const firstDate = origin.data ? ymdInTimeZone(new Date(origin.data.startedAt), timezone) : "";
  const today = ymdInTimeZone(new Date(), timezone);
  const lastDate = shiftCivilDate(today, -1);
  const defaultDate = previousMonthEnd(today);
  const initialDefault = defaultDate < firstDate ? firstDate : defaultDate;
  const date = chosenDate ?? initialDefault;
  const compareDate = comparisonDate ?? lastDate;
  const compareTo = comparison === "none" ? "" : comparison === "current" ? "current" : compareDate;
  const validDate = isClosedDate(date, firstDate, lastDate);
  const validComparison = comparison !== "date" || isClosedDate(compareDate, firstDate, lastDate);
  const valid = Boolean(origin.data && validDate && validComparison);
  const query = useHistoricalOverview(date, compareTo, valid);
  const data = valid && !query.isFetching && !query.isError ? query.data : undefined;
  const selected = data?.rows?.find(row => row.key === selectedKey);
  const parent = data?.rows?.find(row => row.key === selected?.parentKey);
  const accounts = data?.rows?.filter(row => row.kind === "account" && (!changesOnly || !data.right || row.changed)) ?? [];
  const chooseDate = (value: string) => { setSelectedKey(null); setChosenDate(value); };
  const openDetail = (key: string, trigger: HTMLButtonElement) => { detailTrigger.current = trigger; setSelectedKey(key); };
  const toggleAccount = (key: string) => setExpanded(previous => {
    const next = new Set(previous);
    if (next.has(key)) next.delete(key); else next.add(key);
    return next;
  });
  const chrome = <PageChrome pageId="historical-overview" title={t("historicalOverview.title")} actions={
    <Button variant="outline" onClick={onExit}>{t("historicalOverview.exit")}</Button>
  } />;
  if (origin.isLoading) return <>{chrome}<LoadingState label={t("historicalOverview.loading")} /></>;
  if (origin.isError) return <>{chrome}<ErrorState title={t("historicalOverview.error")} description={t("historicalOverview.errorHint")} onRetry={() => void origin.refetch()} retryLabel={t("common.retryAction")} /></>;
  if (!origin.data) return <>{chrome}<EmptyState title={t("insights.noOrigin")} description={t("insights.noOriginHint")} /></>;
  if (firstDate > lastDate) return <>{chrome}<EmptyState title={t("historicalOverview.noClosedDays")} description={t("historicalOverview.noClosedDaysHint")} /></>;

  return <div className="flex min-w-0 flex-col gap-4">
    {chrome}
    <div className="rounded-xl border border-primary/25 bg-primary/5 p-4">
      <div className="flex flex-wrap items-center gap-2">
        <History className="size-4" aria-hidden="true" /><strong>{date || t("historicalOverview.date")}</strong>
        <Badge variant="outline">{t("historicalOverview.readOnly")}</Badge><span className="text-sm">{timezone}</span>
      </div>
      <p className="mt-2 text-sm">{t("historicalOverview.reconstructed")}</p>
      <p className="mt-1 text-xs text-muted-foreground">{t("historicalOverview.metadataNote")}</p>
    </div>
    <div className="flex flex-wrap items-end gap-3">
      <label className="flex min-w-0 flex-col gap-1 text-sm">
        {t("historicalOverview.date")}
        <Input ref={dateInput} autoFocus type="date" value={date} min={firstDate} max={lastDate} aria-invalid={!validDate} aria-describedby="historical-date-range" onChange={event => chooseDate(event.target.value)} />
      </label>
      <Button variant="outline" disabled={!validDate || date <= firstDate} onClick={() => chooseDate(shiftCivilDate(date, -1))}>{t("historicalOverview.previous")}</Button>
      <Button variant="outline" disabled={!validDate || date >= lastDate} onClick={() => chooseDate(shiftCivilDate(date, 1))}>{t("historicalOverview.next")}</Button>
      <label className="flex min-w-0 flex-col gap-1 text-sm">
        {t("historicalOverview.compare")}
        <NativeSelect value={comparison} onChange={event => {
          setSelectedKey(null);
          if (event.target.value === "date" && comparisonDate === null) {
            setComparisonDate(date === lastDate && date > firstDate ? shiftCivilDate(date, -1) : lastDate);
          }
          setComparison(event.target.value);
        }}>
          {["none", "current", "date"].map(value => <option key={value} value={value}>{t(`historicalOverview.compareOptions.${value}`)}</option>)}
        </NativeSelect>
      </label>
      {comparison === "date" && <label className="flex min-w-0 flex-col gap-1 text-sm">
        {t("historicalOverview.compareDate")}
        <Input type="date" value={compareDate} min={firstDate} max={lastDate} aria-invalid={!validComparison} aria-describedby="historical-date-range" onChange={event => { setSelectedKey(null); setComparisonDate(event.target.value); }} />
      </label>}
      <Button variant="outline" disabled={!valid || query.isFetching} onClick={() => { setSelectedKey(null); void query.refetch(); }}>
        {t(comparison === "none" ? "historicalOverview.refreshDate" : "historicalOverview.refresh")}
      </Button>
    </div>
    <div className="flex flex-wrap gap-2">
      <Button variant="ghost" size="sm" disabled={!isClosedDate(defaultDate, firstDate, lastDate) || date === defaultDate} onClick={() => chooseDate(defaultDate)}>{t("historicalOverview.lastMonth")}</Button>
      <Button variant="ghost" size="sm" disabled={date === lastDate} onClick={() => chooseDate(lastDate)}>{t("historicalOverview.latestClosed")}</Button>
      <Button variant="ghost" size="sm" disabled={date === firstDate} onClick={() => chooseDate(firstDate)}>{t("historicalOverview.startingPoint")}</Button>
    </div>
    <p id="historical-date-range" className="text-xs text-muted-foreground">{t("historicalOverview.dateRange", { first: firstDate, last: lastDate, timezone })}</p>
    <p className="text-xs text-muted-foreground">{t("historicalOverview.cutoffNote")}</p>
    {!valid ? <EmptyState title={t("historicalOverview.invalidDate")} description={t("historicalOverview.invalidDateHint")} />
      : query.isFetching ? <LoadingState label={t("historicalOverview.loading")} variant="table" />
      : query.isError ? <ErrorState title={t("historicalOverview.error")} description={t("historicalOverview.errorHint")} onRetry={() => void query.refetch()} retryLabel={t("common.retryAction")} />
      : data && <>
        <p className="text-xs text-muted-foreground">{t("historicalOverview.captureTime")}: <ObservationTime value={data.capturedAt} timezone={data.timezone} /> · {t("historicalOverview.refreshHint")}</p>
        <div className={`grid gap-4 ${data.right ? "xl:grid-cols-2" : ""}`}>
          <Summary state={data.left} />{data.right && <Summary state={data.right} />}
        </div>
        {data.right && <>
          {data.change && <TotalsChange change={data.change} currency={data.left.currency} />}
          {data.right.date === data.left.date && !data.right.current && <p role="status" className="text-sm text-muted-foreground">{t("historicalOverview.sameDate")}</p>}
          <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={changesOnly} onChange={event => setChangesOnly(event.target.checked)} />{t("historicalOverview.changesOnly")}</label>
          <p className="text-xs text-muted-foreground">{t("historicalOverview.changesOnlyHint")}</p>
        </>}
        {accounts.length === 0
          ? <EmptyState title={t(changesOnly && data.right ? "historicalOverview.noChanges" : "historicalOverview.noAccounts")} />
          : <>
            <p className="text-xs text-muted-foreground md:hidden">{t("historicalOverview.scrollHint")}</p>
            <div role="region" aria-label={t("historicalOverview.tableRegion")} tabIndex={0} className="min-w-0 overflow-x-auto rounded-xl border border-border focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50">
              <table className={`w-full text-sm ${data.right ? "min-w-[48rem]" : "min-w-[32rem]"}`}>
                <caption className="sr-only">{t("historicalOverview.table")}</caption>
                <thead className="bg-muted/60"><tr>
                  <th scope="col" className="sticky left-0 z-10 w-52 max-w-52 bg-card p-3 text-left">{t("historicalOverview.account")}</th>
                  <th scope="col" className="p-3 text-right">{data.left.date}</th>
                  {data.right && <><th scope="col" className="p-3 text-right">{data.right.current ? t("historicalOverview.current") : data.right.date}</th><th scope="col" className="p-3 text-right">{t("historicalOverview.change")}</th></>}
                </tr></thead>
                <tbody>{accounts.map(account => <Fragment key={account.key}>
                  <tr className="border-t border-border bg-card">
                    <th scope="row" className="sticky left-0 z-10 max-w-52 bg-card p-3 text-left">
                      <div className="flex items-center gap-2">
                        <Button variant="ghost" size="icon" aria-label={t(expanded.has(account.key) ? "historicalOverview.collapse" : "historicalOverview.expand", { name: account.name })} aria-expanded={expanded.has(account.key)} onClick={() => toggleAccount(account.key)}>
                          {expanded.has(account.key) ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
                        </Button>
                        <Button variant="link" className="h-auto min-w-0 whitespace-normal [overflow-wrap:anywhere] p-0 text-left" onClick={event => openDetail(account.key, event.currentTarget)}>{account.name}</Button>
                        {account.role === "liability" && <Badge variant="outline">{t("historicalOverview.debt")}</Badge>}
                      </div>
                    </th>
                    <td className="p-3"><CellValue cell={account.left} currency={data.left.currency} /></td>
                    {data.right && <><td className="p-3"><CellValue cell={account.right} currency={data.right.currency} /></td><td className="p-3"><RowChanges row={account} currency={data.left.currency} /></td></>}
                  </tr>
                  {expanded.has(account.key) && (data.rows ?? []).filter(row => row.parentKey === account.key && (!changesOnly || !data.right || row.changed)).map(row => <tr key={row.key} className="border-t border-border/60">
                    <th scope="row" className="sticky left-0 z-10 max-w-52 bg-card py-3 pl-12 pr-3 text-left font-normal">
                      <Button variant="link" className="h-auto whitespace-normal [overflow-wrap:anywhere] p-0 text-left" onClick={event => openDetail(row.key, event.currentTarget)}>{rowLabel(row, t)}</Button>
                    </th>
                    <td className="p-3"><CellValue cell={row.left} currency={data.left.currency} /></td>
                    {data.right && <><td className="p-3"><CellValue cell={row.right} currency={data.right.currency} /></td><td className="p-3"><RowChanges row={row} currency={data.left.currency} /></td></>}
                  </tr>)}
                </Fragment>)}</tbody>
              </table>
            </div>
          </>}
        <Sheet open={Boolean(selected)} onOpenChange={open => { if (!open) setSelectedKey(null); }}>
          <SheetContent finalFocus={() => detailTrigger.current?.isConnected ? detailTrigger.current : dateInput.current}>
            <SheetHeader>
              <SheetTitle>{selected ? rowLabel(selected, t) : ""}</SheetTitle>
              <SheetDescription>{t("historicalOverview.readOnly")} · {data.timezone}</SheetDescription>
              {parent && <p className="text-sm">{t("historicalOverview.accountContext", { name: parent.name })}</p>}
              <p className="text-xs text-muted-foreground">{t("historicalOverview.metadataNote")}</p>
            </SheetHeader>
            {selected && <>
              <CellDetail cell={selected.left} state={data.left} timezone={data.timezone} />
              {data.right && <>
                <CellDetail cell={selected.right} state={data.right} timezone={data.timezone} />
                <section className="border-t border-border pt-4">
                  <h3 className="mb-2 text-sm font-semibold">{t("historicalOverview.change")}</h3>
                  <p className="mb-2 text-xs text-muted-foreground">{t("historicalOverview.deltaNote")}</p>
                  <RowChanges row={selected} currency={data.left.currency} exact />
                </section>
              </>}
            </>}
          </SheetContent>
        </Sheet>
      </>}
  </div>;
}
