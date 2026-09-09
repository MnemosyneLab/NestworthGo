import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { NativeSelect } from "@/components/ui/select";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { TrendChart } from "@/components/charts/TrendChart";
import { useReturnTrend } from "@/queries/returnAnalysis";
import type { AnalysisSessionState } from "@/stores/analysis";
import type { ReturnTrendDTO, SignedMoneyView } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { useAnalysisProjectionContext } from "@/features/insights/analysisProjectionContext";
import { formatAmount } from "@/lib/money";
import { activeReturnTrendRange, returnTrendRange, type ReturnTrendRange } from "@/features/insights/analysisRequest";
import { useAnalysisStore } from "@/stores/analysis";

function amountText(value?: SignedMoneyView | null): string {
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

function rateAxisText(value: string | number): string {
  const numeric = Number(value) * 100;
  if (!Number.isFinite(numeric)) return "";
  const precision = Math.abs(numeric) < 0.1 ? 4 : Math.abs(numeric) < 1 ? 3 : 2;
  const rounded = numeric.toFixed(precision).replace(/\.0+$/, "").replace(/(\.\d*?)0+$/, "$1");
  return `${numeric > 0 ? "+" : ""}${rounded}%`;
}

function shareText(value: string | null | undefined): string {
  if (value == null) return "—";
  const numeric = Number(value) * 100;
  if (!Number.isFinite(numeric)) return "—";
  const rounded = numeric.toFixed(1).replace(/\.0$/, "");
  return `${rounded}%`;
}

function availabilityEmpty(data: ReturnTrendDTO | undefined, t: (key: string) => string) {
  const hasHistory = Boolean(data && (data.points?.length ?? 0) > 0);
  return <EmptyState title={t(hasHistory ? "insights.historyInsufficient" : "insights.noInvestmentAssets")} description={t(hasHistory ? "insights.historyInsufficientHint" : "insights.noInvestmentAssetsHint")} />;
}

function TrendSummary({ data }: { data: ReturnTrendDTO }) {
  const { t } = useTranslation();
  const partial = data.ratedDays < data.totalDays;
  return (
    <Card>
      <CardHeader className="gap-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <CardTitle>{t("insights.summary")}</CardTitle>
          <div className="flex flex-wrap items-center gap-2">
            {data.valuationForced && <Badge variant="warning">{t("insights.valuationForced")}</Badge>}
            {partial && <Badge variant="warning">{t("insights.partial")} {data.ratedDays}/{data.totalDays}</Badge>}
          </div>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div><p className="text-xs uppercase tracking-wide text-muted-foreground">{t("insights.returnAmount")}</p><p className="mt-1 text-2xl font-semibold">{amountText(data.amount)}</p></div>
          <div><p className="text-xs uppercase tracking-wide text-muted-foreground">{t("insights.returnRate")}</p><p className="mt-1 text-2xl font-semibold">{rateText(data.rate)}{partial && <sup className="ml-1 text-warning-foreground">◇</sup>}</p></div>
        </div>
      </CardHeader>
    </Card>
  );
}

function displayLabel(t: (key: string) => string, display: string): string {
  const key = display === "cumulative_amount" ? "returnTrendCumulative" : display === "linked_rate" ? "returnTrendLinked" : "returnTrendPeriod";
  return t(`insights.${key}`);
}

function sourceLabel(t: (key: string) => string, key: string): string {
  const labels: Record<string, string> = {
    price_change: "components.priceChange",
    fx_impact: "components.fxImpact",
    dividend_interest: "components.dividendInterest",
    investment_fee: "components.investmentFee",
  };
  return labels[key] ? t(`insights.${labels[key]}`) : key;
}

export function ReturnTrendTab({ session }: { session: AnalysisSessionState }) {
  const { t } = useTranslation();
  const [display, setDisplay] = useState("cumulative_amount");
  const context = useAnalysisProjectionContext(session);
  const setFilters = useAnalysisStore((state) => state.setFilters);
  const trend = useReturnTrend(context.request, display, context.enabled);
  const data = trend.data;
  const originData = context.origin.data;
  const chartPoints = useMemo(() => (data?.points ?? []).filter((point) => display === "linked_rate" ? point.rate != null : point.value != null), [data?.points, display]);
  const currency = data?.amount?.currency ?? chartPoints.find((point) => point.value?.currency)?.value?.currency ?? "";
  const chartAmountText = (value: string | null | undefined) => amountText(value == null ? null : { amount: value, currency });
  const chartValues = useMemo(() => (data?.points ?? []).map((point) => display === "linked_rate" ? point.rate : point.value?.amount ?? null), [data?.points, display]);
  const dates = useMemo(() => (data?.points ?? []).map((point) => point.date), [data?.points]);

  if (context.origin.isLoading) return <LoadingState label={t("insights.loading")} />;
  if (context.origin.isError) return <ErrorState title={t("insights.error")} description={t("ui.state.errorDescription")} onRetry={() => context.origin.refetch()} retryLabel={t("common.retryAction")} />;
  if (!originData) return <EmptyState title={t("insights.noOrigin")} description={t("insights.noOriginHint")} />;
  if (!context.scopeReady) return <EmptyState title={t("insights.scopeRequired")} description={t("insights.scopeRequiredHint")} />;
  if (!context.rangeAvailable) return <EmptyState title={t("insights.historyInsufficient")} description={t("insights.historyInsufficientHint")} />;

  const selectedRange = activeReturnTrendRange(session, originData.startedAt, originData.timezone);
  const applyRange = (range: Exclude<ReturnTrendRange, "custom">) => {
    setFilters(returnTrendRange(range, originData.startedAt, originData.timezone));
  };
  const focusCustomRange = () => {
    document.getElementById("analysis-from")?.focus();
  };

  let results;
  if (trend.isLoading) results = <LoadingState label={t("insights.loading")} />;
  else if (trend.isError) results = <ErrorState title={t("insights.error")} description={t("ui.state.errorDescription")} onRetry={() => trend.refetch()} retryLabel={t("common.retryAction")} />;
  else if (!data?.available) results = availabilityEmpty(data, t);
  else results = (
    <>
      <TrendSummary data={data} />
      {chartPoints.length === 0 ? <EmptyState title={t("charts.insufficientHistory")} description={t("insights.dailyRateHint")} /> : <Card><CardHeader><CardTitle>{displayLabel(t, display)}</CardTitle></CardHeader><CardContent><TrendChart ariaLabel={displayLabel(t, display)} summary={t("insights.returnTrendChartSummary")} dates={dates} series={[{ key: display, name: displayLabel(t, display), color: "hsl(var(--chart-1))", values: chartValues }]} currency={currency} valueFormatter={display === "linked_rate" ? rateText : chartAmountText} axisValueFormatter={display === "linked_rate" ? rateAxisText : undefined} emptyTitle={t("charts.insufficientHistory")} extraTableColumns={[t("charts.date"), displayLabel(t, display)]} extraTableRows={dates.map((date, index) => [date, display === "linked_rate" ? rateText(chartValues[index]) : chartAmountText(chartValues[index])])} /></CardContent></Card>}
      {(data.sources ?? []).length > 0 && <Card><CardHeader><CardTitle>{t("insights.returnSources")}</CardTitle></CardHeader><CardContent><ul className="flex flex-col gap-2 text-sm">{(data.sources ?? []).map((source) => <li key={source.key} className="flex items-center gap-3"><span className="min-w-0 flex-1 truncate">{sourceLabel(t, source.key)}</span><span className="shrink-0">{amountText(source.amount)}</span><span className="w-16 shrink-0 text-right text-muted-foreground">{source.share == null ? "—" : shareText(source.share)}</span></li>)}</ul></CardContent></Card>}
      <Button type="button" variant="ghost" className="self-start" onClick={() => setDisplay("cumulative_amount")} hidden={display === "cumulative_amount"}>{t("insights.resetDisplay")}</Button>
    </>
  );

  return (
    <div className="flex flex-col gap-4" data-testid="return-trend">
      <div className="flex flex-wrap items-end justify-between gap-3 rounded-xl border border-border bg-card/60 p-4">
        <div className="flex flex-col gap-1.5"><span className="text-sm font-medium">{t("insights.range")}</span><div className="flex flex-wrap gap-1" role="group" aria-label={t("insights.range")}>
          {(["30d", "ytd", "1y", "3y", "all"] as const).map((range) => <Button key={range} type="button" size="sm" variant={selectedRange === range ? "default" : "outline"} aria-pressed={selectedRange === range} onClick={() => applyRange(range)}>{t(`insights.range${range === "30d" ? "30d" : range === "ytd" ? "Ytd" : range === "1y" ? "1y" : range === "3y" ? "3y" : "All"}`)}</Button>)}
          <Button key="custom" type="button" size="sm" variant={selectedRange === "custom" ? "default" : "outline"} aria-pressed={selectedRange === "custom"} disabled={selectedRange === "custom"} onClick={focusCustomRange}>{t("insights.rangeCustom")}</Button>
        </div></div>
        <div className="flex flex-col gap-1.5"><label htmlFor="return-trend-display" className="text-sm font-medium">{t("insights.display")}</label><NativeSelect id="return-trend-display" value={display} onChange={(event) => setDisplay(event.target.value)}><option value="cumulative_amount">{t("insights.returnTrendCumulative")}</option><option value="linked_rate">{t("insights.returnTrendLinked")}</option><option value="period_return_amount">{t("insights.returnTrendPeriod")}</option></NativeSelect></div>
        <p className="max-w-md text-sm text-muted-foreground">{display === "linked_rate" ? t("insights.dailyRateHint") : t("insights.returnTrendHint")}</p>
      </div>
      {results}
    </div>
  );
}
