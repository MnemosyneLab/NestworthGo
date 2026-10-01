import { useMemo, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { useReturnCalendar, useReturnDay } from "@/queries/returnAnalysis";
import type { AnalysisQueryRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analysis/models";
import type { ReturnCalendarDTO, ReturnComponentAmountDTO, ReturnContributorDTO, ReturnDayDTO, SignedMoneyView } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import type { AnalysisNavigationContext } from "@/app/navigation";
import type { AnalysisSessionState } from "@/stores/analysis";
import { CompletenessBanner } from "@/features/insights/CompletenessBanner";
import { analysisRequest, currentMonthAwaitingClose, effectiveRange, periodRange } from "@/features/insights/analysisRequest";
import { addMonths, currentMonth, isFuture, isToday, lastClosedDate, monthDays, monthLabel, yearLabel, yearMonths } from "@/features/insights/calendar";
import { addCanonical, formatAmount, multiplyCanonical } from "@/lib/money";
import { cn } from "@/lib/utils";
import { signToneClass } from "@/components/ui/money-text";
import { useHistoryOrigin } from "@/queries/history";
import { useSettings } from "@/queries/settings";
import { useInstruments } from "@/queries/investments";
import { analysisReason } from "@/features/insights/analysisText";
import { instrumentDisplayLabels } from "@/lib/instrumentDisplay";

function amountText(value?: { amount: string; currency: string } | null): string {
  if (!value) return "—";
  const formatted = formatAmount(value.amount, value.currency);
  return value.amount.startsWith("-") || value.amount === "0" ? formatted : `+${formatted}`;
}

function rateText(value: string | null | undefined): string {
  if (value == null) return "—";
  const numeric = Number(value) * 100;
  if (!Number.isFinite(numeric)) return "—";
  const rounded = numeric.toFixed(2).replace(/\.00$/, "").replace(/(\.\d)0$/, "$1");
  return numeric > 0 ? `+${rounded}%` : `${rounded}%`;
}

function amountNumber(value?: { amount: string } | null): number {
  const numeric = Number(value?.amount ?? "0");
  return Number.isFinite(numeric) ? numeric : 0;
}

function coverageLabel(ratedDays: number, totalDays: number): string {
  return `${ratedDays}/${totalDays}`;
}

interface MonthReturnSummary {
  returnAmount: SignedMoneyView | null;
  amountStatus: "complete" | "partial" | "unavailable";
  returnRate: string | null;
  ratedDays: number;
  totalDays: number;
  available: boolean;
}

function monthAmountStatus(cells: ReturnDayDTO[]): MonthReturnSummary["amountStatus"] {
  let hasKnownAmount = false;
  let hasIncompleteAmount = false;
  for (const cell of cells) {
    if (cell.amountStatus === "complete") {
      hasKnownAmount = true;
    } else if (cell.amountStatus === "partial") {
      hasKnownAmount = true;
      hasIncompleteAmount = true;
    } else {
      hasIncompleteAmount = true;
    }
  }
  if (!hasKnownAmount) return "unavailable";
  return hasIncompleteAmount ? "partial" : "complete";
}

function aggregateMonth(cells: ReturnDayDTO[]): MonthReturnSummary {
  let amount = "0";
  let currency = "";
  let hasAmount = false;
  let factor = "1";
  let hasRate = false;
  let ratedDays = 0;
  let totalDays = 0;
  let available = false;
  for (const cell of cells) {
    ratedDays += cell.ratedDays;
    totalDays += cell.totalDays;
    available ||= cell.available;
    if (cell.returnAmount) {
      amount = addCanonical(amount, cell.returnAmount.amount);
      currency ||= cell.returnAmount.currency;
      hasAmount = true;
    }
    if (cell.returnRate != null) {
      const dailyFactor = addCanonical("1", cell.returnRate);
      const linkedFactor = dailyFactor ? multiplyCanonical(factor, dailyFactor) : "";
      if (linkedFactor) {
        factor = linkedFactor;
        hasRate = true;
      }
    }
  }
  return {
    returnAmount: hasAmount ? { amount, currency } : null,
    amountStatus: monthAmountStatus(cells),
    returnRate: hasRate && ratedDays > 0 && ratedDays * 2 >= totalDays ? addCanonical(factor, "-1") : null,
    ratedDays,
    totalDays,
    available,
  };
}

function aggregateYearMonths(data: ReturnCalendarDTO | undefined, year: number): Map<string, MonthReturnSummary> {
  const cellsByMonth = new Map<string, ReturnDayDTO[]>();
  for (const cell of data?.cells ?? []) {
    const month = cell.date.slice(0, 7);
    const cells = cellsByMonth.get(month) ?? [];
    cells.push(cell);
    cellsByMonth.set(month, cells);
  }
  return new Map(yearMonths(year).map((month) => [month, aggregateMonth(cellsByMonth.get(month) ?? [])]));
}

function CalendarEmptyState({ data }: { data?: ReturnCalendarDTO }) {
  const { t } = useTranslation();
  const insufficientHistory = Boolean(data && (data.cells?.length ?? 0) > 0);
  return <EmptyState title={t(insufficientHistory ? "insights.historyInsufficient" : "insights.noInvestmentAssets")} description={t(insufficientHistory ? "insights.historyInsufficientHint" : "insights.noInvestmentAssetsHint")} />;
}

function Summary({ data }: { data: ReturnCalendarDTO }) {
  const { t } = useTranslation();
  const summary = data.summary;
  const partial = summary.ratedDays < summary.totalDays;
  const amountPartial = summary.amountStatus === "partial";
  const amountUnavailable = summary.amountStatus === "unavailable";
  return (
    <Card>
      <CardHeader className="px-4 py-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <CardTitle>{t("insights.summary")}</CardTitle>
          <div className="flex flex-wrap items-center gap-2">
            {data.valuationForced && <Badge variant="warning">{t("insights.valuationForced")}</Badge>}
            {partial && <Badge variant={data.status === "partial" ? "warning" : "secondary"} title={t("insights.rateCoverageHint")}>{t("insights.rateCoverage")} {coverageLabel(summary.ratedDays, summary.totalDays)}</Badge>}
          </div>
        </div>
      </CardHeader>
      <CardContent className="grid grid-cols-2 gap-3 px-4 pb-3 sm:grid-cols-4">
        <div>
          <p className="text-sm text-muted-foreground">{t("insights.returnAmount")}</p>
          <p className={cn("num text-lg font-semibold", signToneClass(summary.returnAmount?.amount))}>{amountText(summary.returnAmount)}{(amountPartial || amountUnavailable) && <><sup className="ml-1 text-warning-foreground" title={t(amountPartial ? "insights.amountPartial" : "insights.amountUnavailable")}>◇</sup><span className="sr-only">{t(amountPartial ? "insights.amountPartial" : "insights.amountUnavailable")}</span></>}</p>
        </div>
        <div>
          <p className="text-sm text-muted-foreground">{t("insights.returnRate")}</p>
          <p className={cn("num text-lg font-semibold", signToneClass(summary.returnRate))} title={partial ? t("insights.rateCoverageHint") : undefined}>{rateText(summary.returnRate)}{partial && <sup className="ml-1 text-warning-foreground">◇</sup>}</p>
        </div>

        <SummaryValue label={t("insights.beginning")} value={balanceText(summary.beginningInvestedValue)} />
        <SummaryValue label={t("insights.ending")} value={balanceText(summary.endingInvestedValue)} />
      </CardContent>
    </Card>
  );
}

function SummaryValue({ label, value }: { label: string; value: string }) {
  return <div><p className="text-sm text-muted-foreground">{label}</p><p className="num text-lg font-semibold">{value}</p></div>;
}

function balanceText(value?: { amount: string; currency: string } | null): string {
  return value ? formatAmount(value.amount, value.currency) : "—";
}

function CompositionList({ values, title }: { values: ReturnComponentAmountDTO[] | null | undefined; title: string }) {
  const { t } = useTranslation();
  if (!values || values.length === 0) return null;
  return (
    <div className="mt-2 border-t border-border pt-2">
      <p className="mb-1 text-xs font-medium text-muted-foreground">{title}</p>
      <ul className="flex flex-col gap-1">
        {values.map((item) => <li key={item.component} className="flex justify-between gap-3"><span>{componentLabel(t, item.component)}</span><span>{amountText(item.amount)}</span></li>)}
      </ul>
    </div>
  );
}

function componentLabel(t: (key: string) => string, component: string): string {
  const key = ({
    price_change: "priceChange",
    fx_impact: "fxImpact",
    fx_conversion_spread: "fxConversionSpread",
    dividend_interest: "dividendInterest",
    investment_fee: "investmentFee",
    cash_fx_impact: "cashFxImpact",
    residual: "residual",
  } as Record<string, string>)[component] ?? component;
  return t(`insights.components.${key}`);
}

function contributorLabel(t: (key: string) => string, item: ReturnContributorDTO, instrumentNames: Map<string, string>): string {
  if (item.key === "cash") return t("insights.cash");
  return instrumentNames.get(item.key) ?? item.label;
}

function ContributorList({ values, title, instrumentNames = new Map() }: { values: ReturnContributorDTO[] | null | undefined; title: string; instrumentNames?: Map<string, string> }) {
  const { t } = useTranslation();
  if (!values || values.length === 0) return null;
  const visible = values.slice(0, 6);
  const remaining = values.slice(6);
  const currency = remaining[0]?.amount?.currency;
  const otherAmount = currency && remaining.every(item => item.amount?.currency === currency)
    ? { amount: remaining.reduce((sum, item) => addCanonical(sum, item.amount!.amount), "0"), currency }
    : null;
  return <div className="mt-3 border-t border-border pt-3"><p className="mb-2 text-sm font-medium">{title}</p><ul className="flex flex-col gap-2 text-sm">{visible.map((item) => <li key={item.key} className="flex justify-between gap-3"><span className="truncate">{contributorLabel(t, item, instrumentNames)}</span><span className="shrink-0">{amountText(item.amount)}</span></li>)}{remaining.length > 0 && <li className="flex justify-between gap-3"><span className="truncate">{t("insights.other")}</span><span className="shrink-0">{amountText(otherAmount)}</span></li>}</ul></div>;
}

function DayCell({ day, date, inMonth, timeZone, onOpen, selected = false, column = 0, row = 0 }: { column?: number; row?: number; selected?: boolean; day?: ReturnDayDTO; date: string; inMonth: boolean; timeZone?: string; onOpen: (date: string) => void }) {
  const { t } = useTranslation();
  const future = isFuture(date, timeZone);
  const today = isToday(date, timeZone);
  const partial = Boolean(day && (day.status === "partial"));
  const amount = amountNumber(day?.returnAmount);
  const hasData = Boolean(day?.available);
  const showData = hasData && !today;
  const amountPartial = day?.amountStatus === "partial";
  const amountUnavailable = day?.amountStatus === "unavailable";
  return (
    <div data-testid={`return-day-${date}`} data-selected={selected || undefined} className={cn("data-[selected=true]:ring-2 data-[selected=true]:ring-primary/40 group relative min-h-28 rounded-xl border border-border p-2 transition-colors hover:border-primary/40", !inMonth && "border-transparent bg-muted/20 opacity-45", inMonth && hasData && !today && amount >= 0.005 && "bg-gain-positive/14", inMonth && hasData && !today && amount <= -0.005 && "bg-gain-negative/14", (future || today) && "opacity-60")}>
      <button type="button" disabled={!inMonth || future || today || !day} onClick={() => onOpen(date)} className="flex min-h-24 w-full flex-col items-start gap-1 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50">
        <span className="flex w-full items-center justify-between text-xs font-medium"><span>{Number(date.slice(-2))}</span>{today && <span className="text-[0.65rem] text-muted-foreground">{t("insights.today")}</span>}</span>
        {day && showData ? <><span className={cn("num mt-2 text-sm font-semibold", (day.returnRate ?? "").startsWith("-") ? "text-gain-negative" : amount > 0 && "text-gain-positive")} title={partial ? `${t("insights.partial")} ${coverageLabel(day.ratedDays, day.totalDays)}` : undefined}>{rateText(day.returnRate)}{partial && <sup className="ml-0.5 text-warning-foreground">◇</sup>}</span><span className="text-xs text-muted-foreground">{amountText(day.returnAmount)}{(amountPartial || amountUnavailable) && <><sup className="ml-1 text-warning-foreground" title={t(amountPartial ? "insights.amountPartial" : "insights.amountUnavailable")}>◇</sup><span className="sr-only">{t(amountPartial ? "insights.amountPartial" : "insights.amountUnavailable")}</span></>}</span></> : <span className="mt-2 text-xs text-muted-foreground">{today ? t("insights.todayMuted") : future ? "—" : t("insights.noData")}</span>}
      </button>
      {day && showData && day.composition && day.composition.length > 0 && (
        <div className={cn("pointer-events-none invisible absolute z-20 w-64 max-w-[calc(100vw-3rem)] rounded-xl border border-border bg-card p-3 text-xs shadow-lg group-hover:visible group-focus-within:visible", column >= 4 ? "right-2" : "left-2", row >= 3 ? "bottom-full mb-1" : "top-full mt-1")}>
          <p className="font-medium">{date}</p>
          <p className="mt-1 text-muted-foreground">{t("insights.returnAmount")}: {amountText(day.returnAmount)}</p>
          <CompositionList values={day.composition} title={t("insights.composition")} />
        </div>
      )}
    </div>
  );
}

function MonthGrid({ data, month, timeZone, weekStartsOn, onOpen, selection }: { selection?: { from: string; to: string }; data?: ReturnCalendarDTO; month: string; timeZone?: string; weekStartsOn: 0 | 1; onOpen: (date: string) => void }) {
  const { i18n } = useTranslation();
  const cells = useMemo(() => new Map((data?.cells ?? []).map((day) => [day.date, day])), [data?.cells]);
  const firstWeekday = new Date(2024, 0, weekStartsOn === 0 ? 7 : 1);
  const weekdays = Array.from({ length: 7 }, (_, index) => { const date = new Date(firstWeekday); date.setDate(firstWeekday.getDate() + index); return date.toLocaleDateString(i18n.language, { weekday: "short" }); });
  return <div className="grid grid-cols-7 gap-1.5">{weekdays.map((day) => <div key={day} className="px-2 py-1 text-center text-xs font-medium text-muted-foreground">{day}</div>)}{monthDays(month, weekStartsOn).map((cell, index) => <DayCell key={cell.date} column={index % 7} row={Math.floor(index / 7)} selected={Boolean(selection?.from && selection?.to && cell.date >= selection.from && cell.date <= selection.to)} {...cell} day={cells.get(cell.date)} timeZone={timeZone} onOpen={onOpen} />)}</div>;
}

function YearGrid({ dataByMonth, year, onOpenMonth }: { dataByMonth: Map<string, MonthReturnSummary>; year: number; onOpenMonth: (month: string) => void }) {
  const { i18n } = useTranslation();
  const { t } = useTranslation();
  return <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">{yearMonths(year).map((month) => { const data = dataByMonth.get(month); const amountStatusLabel = data?.amountStatus === "partial" ? t("insights.amountPartial") : t("insights.amountUnavailable"); const showAmountStatus = Boolean(data && data.totalDays > 0 && data.amountStatus !== "complete"); return <button key={month} type="button" className="rounded-xl border border-border bg-card p-4 text-left transition-colors hover:border-primary/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50" onClick={() => onOpenMonth(month)}><div className="flex items-center justify-between"><span className="font-medium">{monthLabel(month, i18n.language, false)}</span><span className="text-sm font-semibold">{rateText(data?.returnRate)}</span></div><p className="mt-3 text-lg font-semibold">{amountText(data?.returnAmount)}{showAmountStatus && <><sup className="ml-1 text-warning-foreground" title={amountStatusLabel}>◇</sup><span className="sr-only">{amountStatusLabel}</span></>}</p><p className="mt-1 text-xs text-muted-foreground">{data ? coverageLabel(data.ratedDays, data.totalDays) : "—"}</p></button>; })}</div>;
}

function DaySheet({ request, date, session, onClose, onAssetChanges }: { request: AnalysisQueryRequest; date: string | null; session: AnalysisSessionState; onClose: () => void; onAssetChanges?: (analysis: AnalysisNavigationContext) => void }) {
  const { t } = useTranslation();
  const day = useReturnDay(request, date ?? "", Boolean(date));
  return <Sheet open={Boolean(date)} onOpenChange={(open) => { if (!open) onClose(); }}><SheetContent side="right"><SheetHeader><SheetTitle>{date ?? t("insights.daySheet")}</SheetTitle></SheetHeader>{day.isLoading ? <LoadingState label={t("insights.loading")} /> : day.isError ? <ErrorState title={t("insights.error")} description={t("ui.state.errorDescription")} onRetry={() => day.refetch()} retryLabel={t("common.retryAction")} /> : day.data ? <DayDetails date={date ?? ""} data={day.data} session={session} onAssetChanges={onAssetChanges} /> : <EmptyState title={t("insights.noData")} />}</SheetContent></Sheet>;
}

function DayDetails({ data, date, session, onAssetChanges }: { data: ReturnDayDTO; date: string; session: AnalysisSessionState; onAssetChanges?: (analysis: AnalysisNavigationContext) => void }) {
  const { t } = useTranslation();
  const instruments = useInstruments();
  const instrumentNames = instrumentDisplayLabels(instruments.data ?? []);
  const amountPartial = data.amountStatus === "partial";
  const amountUnavailable = data.amountStatus === "unavailable";
  const openAssetChanges = () => onAssetChanges?.({ scope: session.scope, scopeId: session.scopeId, valuation: session.valuation, includeCash: session.includeCash, from: date, to: date, moreFilters: session.moreFilters });
  return <div className="flex flex-col gap-4 overflow-y-auto"><div><p className="text-sm text-muted-foreground">{t("insights.returnAmount")}</p><p className="mt-1 text-2xl font-semibold">{amountText(data.returnAmount)}{(amountPartial || amountUnavailable) && <><sup className="ml-1 text-warning-foreground" title={t(amountPartial ? "insights.amountPartial" : "insights.amountUnavailable")}>◇</sup><span className="sr-only">{t(amountPartial ? "insights.amountPartial" : "insights.amountUnavailable")}</span></>}</p><p className="text-sm text-muted-foreground">{rateText(data.returnRate)}{data.ratedDays < data.totalDays && ` ◇ ${coverageLabel(data.ratedDays, data.totalDays)}`}</p></div><CompositionList values={data.composition} title={t("insights.composition")} /><ContributorList values={data.contributors} title={t("insights.contributors")} instrumentNames={instrumentNames} />{data.issues?.length ? <div className="rounded-md border border-warning/40 bg-warning/10 p-3 text-sm">{data.issues.map((issue) => <p key={`${issue.date}-${issue.status}`}>{analysisReason(t, issue.missingReason) ?? issue.status}</p>)}</div> : null}<Button type="button" variant="outline" disabled={!onAssetChanges} onClick={openAssetChanges}>{t("insights.viewAssetChanges")}</Button></div>;
}

export { aggregateMonth, aggregateYearMonths };

export function ReturnCalendarTab({ session, onCursorChange, onOpenAssetChanges }: { session: AnalysisSessionState; onCursorChange: (cursor: string) => void; onOpenAssetChanges?: (analysis: AnalysisNavigationContext) => void }) {
  const { t, i18n } = useTranslation();
  const origin = useHistoryOrigin();
  const settings = useSettings();
  const instruments = useInstruments();
  const instrumentNames = instrumentDisplayLabels(instruments.data ?? []);
  const timeZone = origin.data?.timezone;
  const weekStartsOn = settings.data?.weekStart === "sunday" ? 0 : 1;
  const [view, setView] = useState<"month" | "year">("month");
  const [selectedDate, setSelectedDate] = useState<string | null>(null);
  const visibleMonth = session.returnCursor || currentMonth(timeZone);
  const year = Number(visibleMonth.slice(0, 4));
  const range = view === "year" ? periodRange({ from: "", to: "" }, `${year}-01-01`, `${year}-12-31`, timeZone, origin.data?.startedAt) : effectiveRange({ from: "", to: "" }, visibleMonth, timeZone, origin.data?.startedAt);
  const request = analysisRequest(session, range.from, range.to);
  const visibleMonthIsFuture = isFuture(`${visibleMonth}-01`, timeZone);
  const originReady = !origin.isLoading && !origin.isError && Boolean(origin.data?.timezone);
  const scopeReady = session.scope === "portfolio" || Boolean(session.scopeId);
  const monthRangeAvailable = range.from <= range.to;
  const monthQuery = useReturnCalendar(request, visibleMonth, originReady && scopeReady && monthRangeAvailable && view === "month" && (!visibleMonthIsFuture || Boolean(session.from && session.from <= lastClosedDate(timeZone))));
  const yearSummaryQuery = useReturnCalendar(request, "", originReady && scopeReady && monthRangeAvailable && view === "year" && !isFuture(`${year}-01`, timeZone));
  const loading = origin.isLoading || (view === "year" ? yearSummaryQuery.isLoading : monthQuery.isLoading);
  const error = view === "year" ? yearSummaryQuery.isError : monthQuery.isError;
  const data = view === "year" ? yearSummaryQuery.data : monthQuery.data;
  const selectedRange = effectiveRange(session, visibleMonth, timeZone, origin.data?.startedAt);
  const hasSelection = Boolean(session.from && session.to);
  const selectedQuery = useReturnCalendar(analysisRequest(session, selectedRange.from, selectedRange.to), "", originReady && scopeReady && hasSelection && selectedRange.from <= selectedRange.to);
  const summaryData = hasSelection ? selectedQuery.data : data;
  const summary = hasSelection && selectedQuery.isLoading ? <LoadingState label={t("insights.loading")} /> : hasSelection && selectedQuery.isError ? <ErrorState title={t("insights.error")} description={t("ui.state.errorDescription")} onRetry={() => selectedQuery.refetch()} retryLabel={t("common.retryAction")} /> : summaryData ? <>
    <p className="text-xs text-muted-foreground">{hasSelection ? `${selectedRange.from} – ${selectedRange.to}` : `${range.from} – ${range.to}`}</p>
    <CompletenessBanner issues={summaryData.issues ?? []} ratedDays={summaryData.summary.ratedDays} totalDays={summaryData.summary.totalDays} />
    <Summary data={summaryData} />
  </> : null;
  const yearData = useMemo(() => aggregateYearMonths(data, year), [data, year]);

  const move = (amount: number) => onCursorChange(view === "year" ? `${year + amount}-01` : addMonths(visibleMonth, amount));
  const header = <div className="flex flex-wrap items-center justify-between gap-3"><div className="flex items-center gap-2"><Button type="button" variant="outline" size="icon" aria-label={t("insights.previous")} onClick={() => move(-1)}><ChevronLeft aria-hidden="true" /></Button><h2 className="min-w-44 text-center text-lg font-semibold">{view === "year" ? yearLabel(year, i18n.language) : monthLabel(visibleMonth, i18n.language)}</h2><Button type="button" variant="outline" size="icon" aria-label={t("insights.next")} onClick={() => move(1)}><ChevronRight aria-hidden="true" /></Button><Button type="button" variant="ghost" onClick={() => onCursorChange(currentMonth(timeZone))}>{t("insights.today")}</Button></div><div className="flex items-center gap-1 rounded-full bg-muted p-1"><Button type="button" size="sm" variant={view === "month" ? "default" : "ghost"} aria-pressed={view === "month"} onClick={() => setView("month")}>{t("insights.month")}</Button><Button type="button" size="sm" variant={view === "year" ? "default" : "ghost"} aria-pressed={view === "year"} onClick={() => setView("year")}>{t("insights.year")}</Button></div></div>;
  let content: ReactNode;
  if (loading) {
    content = <LoadingState label={t("insights.loading")} />;
  } else if (origin.isError) {
    content = <ErrorState title={t("insights.error")} description={t("ui.state.errorDescription")} onRetry={() => origin.refetch()} retryLabel={t("common.retryAction")} />;
  } else if (!origin.data) {
    content = <EmptyState title={t("insights.noOrigin")} description={t("insights.noOriginHint")} />;
  } else if (!scopeReady) {
    content = <EmptyState title={t("insights.scopeRequired")} description={t("insights.scopeRequiredHint")} />;
  } else if (error) {
    content = <ErrorState title={t("insights.error")} description={t("ui.state.errorDescription")} onRetry={() => { void (view === "year" ? yearSummaryQuery.refetch() : monthQuery.refetch()); }} retryLabel={t("common.retryAction")} />;
  } else if (view === "year") {
    content = data?.available ? <>{summary}<YearGrid year={year} dataByMonth={yearData} onOpenMonth={(month) => { onCursorChange(month); setView("month"); }} /></> : <CalendarEmptyState data={data} />;
  } else if (!monthRangeAvailable || visibleMonthIsFuture) {
    content = <>
      {currentMonthAwaitingClose(session, visibleMonth, timeZone, origin.data?.startedAt) && <EmptyState title={t("insights.currentMonthUnclosed")} description={t("insights.currentMonthUnclosedHint")} />}
      <MonthGrid selection={hasSelection ? selectedRange : undefined} data={data} month={visibleMonth} timeZone={timeZone} weekStartsOn={weekStartsOn} onOpen={setSelectedDate} />
    </>;
  } else if (data?.available) {
    content = <>{summary}<MonthGrid selection={hasSelection ? selectedRange : undefined} data={data} month={visibleMonth} timeZone={timeZone} weekStartsOn={weekStartsOn} onOpen={setSelectedDate} /><ContributorList values={summaryData?.topContributors} title={t("insights.contributors")} instrumentNames={instrumentNames} /></>;
  } else {
    content = <CalendarEmptyState data={data} />;
  }
  return <div className="flex flex-col gap-4">{header}{content}{view === "month" && <DaySheet request={request} date={selectedDate} session={session} onClose={() => setSelectedDate(null)} onAssetChanges={onOpenAssetChanges ? analysis => { setSelectedDate(null); onOpenAssetChanges(analysis); } : undefined} />}</div>;
}
