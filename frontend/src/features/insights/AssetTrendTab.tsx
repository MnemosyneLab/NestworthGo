import { negateDisplayAmount, signedAccountAmount, signedAccountMoney, signedAccountValuation } from "@/lib/liabilityDisplay";
import { compareCanonical } from "@/lib/money";
import { fullDisplayAmount, valuationDetails, valuationTableAmount } from "@/components/valuation/valuationQuality";
import { AssetValueSummary } from "./AssetValueSummary";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { NativeSelect } from "@/components/ui/select";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { TrendChart } from "@/components/charts/TrendChart";
import { useAssetTrend } from "@/queries/returnAnalysis";
import type { AssetTrendDTO, SignedMoneyView } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import type { AnalysisSessionState } from "@/stores/analysis";
import type { TFunction } from "i18next";
import { analysisReason } from "@/features/insights/analysisText";
import { AvailabilityMarks } from "@/features/insights/CompletenessBanner";
import { useAnalysisProjectionContext } from "@/features/insights/analysisProjectionContext";
import { useSettings } from "@/queries/settings";
import { formatAmount, formatDecimalRatioPercent } from "@/lib/money";
import { formatCivilDateRange } from "@/lib/time";
import { assetTrendGaps, assetTrendPeriods } from "./assetTrendPeriods";
import { AnalysisHealthLink } from "./AnalysisHealthLink";
import { ReturnCoverage } from "./ReturnCoverage";

function trendCoverage(t: TFunction, point: { missingReason?: string | null; status?: string | null; displayValuation?: Parameters<typeof valuationDetails>[1] }, isLevel: boolean): string {
  const details = isLevel ? valuationDetails(t, point.displayValuation) : "";
  if (details) return details;
  const reason = analysisReason(t, point.missingReason);
  if (reason) return reason;
  if (!point.status) return "";
  return t(`insights.status.${point.status}`, { defaultValue: t("common.unknownValue") });
}

function amountText(value?: SignedMoneyView | null): string {
  if (!value) return "—";
  const formatted = formatAmount(value.amount, value.currency);
  return compareCanonical(value.amount, "0.005") <= 0 ? formatted : `+${formatted}`;
}

function rateText(value: string | null | undefined): string {
  return formatDecimalRatioPercent(value, 2, true);
}

function metricLabel(t: (key: string) => string, value: string): string {
  const key: Record<string, string> = { net_worth: "metricNetWorth", assets: "metricAssets", liabilities: "metricLiabilities", income: "income", spending: "spending", fees: "fees", dividend_interest: "components.dividendInterest", external_flow: "externalFlows", price_change: "components.priceChange", fx_impact: "components.fxImpact", fx_conversion_spread: "components.fxConversionSpread", investment_return: "metricInvestmentReturn", return_rate: "metricReturnRate", net_change: "metricNetChange", residual: "components.residual" };
  return key[value] ? t(`insights.${key[value]}`) : value;
}

function trendEmpty(data: AssetTrendDTO | undefined, t: (key: string) => string) {
  const hasHistory = Boolean(data && (data.points?.length ?? 0) > 0);
  return <EmptyState title={t(hasHistory ? "insights.historyInsufficient" : "insights.noInvestmentAssets")} description={t(hasHistory ? "insights.historyInsufficientHint" : "insights.noInvestmentAssetsHint")} />;
}

export function AssetTrendTab({ session }: { session: AnalysisSessionState }) {
  const { t } = useTranslation();
  const settings = useSettings();
  const [granularity, setGranularity] = useState("day");
  const [metric, setMetric] = useState("net_worth");
  const context = useAnalysisProjectionContext(session);
  const trend = useAssetTrend(context.request, granularity, metric, context.enabled);
  const data = trend.data;
  const valueChange = data?.valueChange && metric === "liabilities" ? {
    ...data.valueChange,
    beginningValue: signedAccountMoney(data.valueChange.beginningValue, true),
    endingValue: signedAccountMoney(data.valueChange.endingValue, true),
    change: data.valueChange.change ? { ...data.valueChange.change, amount: negateDisplayAmount(data.valueChange.change.amount) } : data.valueChange.change,
  } : data?.valueChange;
  const isLevel = ["net_worth","assets","liabilities"].includes(metric);
  const { from, to } = context.request;
  const periods = useMemo(() => data ? assetTrendPeriods(from, to, granularity) : [], [data, from, to, granularity]);
  const entries = useMemo(() => {
    const points = data?.points ?? [];
    if (!periods.length) return points.map(point => ({ key: point.period, date: point.period, point }));
    const byPeriod = new Map(points.map(point => [point.period, point]));
    return periods.map(period => ({ ...period, point: byPeriod.get(period.key) }));
  }, [data?.points, periods]);
  const values = entries.map(({ point }) => metric === "return_rate" ? point?.rate : isLevel
    ? fullDisplayAmount(signedAccountValuation(point?.displayValuation, metric === "liabilities"), point?.value ? signedAccountAmount(point.value.amount, metric === "liabilities") : null)
    : point?.value?.amount ?? null);
  const missingIntervals = assetTrendGaps(periods, values);
  const missingCount = values.filter(value => value == null).length;
  const hasTimeline = entries.length > 0 && Boolean(data?.points?.length || data?.valueChange);
  const display = isLevel ? data?.points?.find(point => point.displayValuation?.currency || point.displayValuation?.total?.currency || point.displayValuation?.knownSubtotal?.currency)?.displayValuation : undefined;
  const currency = data?.summary?.currency || display?.currency || display?.total?.currency || display?.knownSubtotal?.currency || data?.points?.find(point => point.value?.currency)?.value?.currency || settings.data?.currency || "";

  if (context.origin.isLoading) return <LoadingState label={t("insights.loading")} />;
  if (context.origin.isError) return <ErrorState title={t("insights.error")} description={t("ui.state.errorDescription")} onRetry={() => context.origin.refetch()} retryLabel={t("common.retryAction")} />;
  if (!context.origin.data) return <EmptyState title={t("insights.noOrigin")} description={t("insights.noOriginHint")} />;
  if (!context.scopeReady) return <EmptyState title={t("insights.scopeRequired")} description={t("insights.scopeRequiredHint")} />;
  if (!context.rangeAvailable) return <EmptyState title={t(context.awaitingClose ? "insights.currentMonthUnclosed" : "insights.historyInsufficient")} description={t(context.awaitingClose ? "insights.currentMonthUnclosedHint" : "insights.historyInsufficientHint")} />;

  const chartValueText = (value: string | null | undefined) => metric === "return_rate" ? rateText(value) : value == null ? "—" : formatAmount(value, currency);
  const toolbar = (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
      <div className="flex items-center gap-2"><label htmlFor="asset-trend-granularity" className="text-xs font-medium text-muted-foreground">{t("insights.granularity")}</label><NativeSelect id="asset-trend-granularity" value={granularity} onChange={(event) => setGranularity(event.target.value)}><option value="day">{t("insights.day")}</option><option value="week">{t("insights.week")}</option><option value="month">{t("insights.month")}</option></NativeSelect></div>
      <div className="flex min-w-52 items-center gap-2"><label htmlFor="asset-trend-metric" className="text-xs font-medium text-muted-foreground">{t("insights.metric")}</label><NativeSelect id="asset-trend-metric" value={metric} onChange={(event) => setMetric(event.target.value)}><optgroup label={t("insights.metricAssetsGroup")}><option value="net_worth">{metricLabel(t, "net_worth")}</option><option value="assets">{metricLabel(t, "assets")}</option><option value="liabilities">{metricLabel(t, "liabilities")}</option></optgroup><optgroup label={t("insights.metricCashFlowGroup")}><option value="external_flow">{metricLabel(t, "external_flow")}</option><option value="income">{metricLabel(t, "income")}</option><option value="spending">{metricLabel(t, "spending")}</option><option value="fees">{metricLabel(t, "fees")}</option></optgroup><optgroup label={t("insights.metricPerformanceGroup")}><option value="investment_return">{metricLabel(t, "investment_return")}</option><option value="return_rate">{metricLabel(t, "return_rate")}</option><option value="dividend_interest">{metricLabel(t, "dividend_interest")}</option><option value="price_change">{metricLabel(t, "price_change")}</option><option value="fx_impact">{metricLabel(t, "fx_impact")}</option><option value="fx_conversion_spread">{metricLabel(t, "fx_conversion_spread")}</option><option value="net_change">{metricLabel(t, "net_change")}</option><option value="residual">{metricLabel(t, "residual")}</option></optgroup></NativeSelect></div>
    </div>
  );
  let results;
  if (trend.isLoading) results = <LoadingState label={t("insights.loading")} />;
  else if (trend.isError) results = <ErrorState title={t("insights.error")} description={t("ui.state.errorDescription")} onRetry={() => trend.refetch()} retryLabel={t("common.retryAction")} />;
  else if (!data || !hasTimeline) results = trendEmpty(data, t);
  else results = (
    <>
      {data.valueChange && <Card><CardContent className="pt-5"><AssetValueSummary summary={valueChange!} /></CardContent></Card>}
      <Card>
        <CardHeader className="flex-row flex-wrap items-center justify-between gap-3">
          <CardTitle>{metricLabel(t, metric)}{metric !== "return_rate" && currency ? <span className="font-normal text-muted-foreground"> · {currency}</span> : null}</CardTitle>
          <div className="flex flex-wrap items-center gap-3">{toolbar}</div>
          <AvailabilityMarks status={data.status} missingReason={data.missingReason} valuationForced={data.valuationForced} />
        </CardHeader>
        <CardContent className="space-y-3">
          {periods.length > 0 && <p className="text-sm text-muted-foreground">{formatCivilDateRange(from, to)}</p>}
          {missingCount > 0 && <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
            {missingIntervals.length > 0 && <span aria-hidden="true" className="size-3 rounded-sm border border-muted-foreground/30 bg-muted-foreground/15" />}
            <p>{t(granularity === "day" ? "insights.trendMissingDays" : "insights.trendMissingPeriods", { count: missingCount })}</p>
            {metric !== "return_rate" && <AnalysisHealthLink from={from} to={to} />}
          </div>}
          <TrendChart
            ariaLabel={metricLabel(t, metric)} summary={t("insights.assetTrendChartSummary")}
            dates={entries.map(entry => entry.date)} timeRange={periods.length ? { from, to } : undefined}
            missingIntervals={periods.length ? missingIntervals : undefined}
            series={[{ key: metric, name: metricLabel(t, metric), color: "var(--color-primary)", values, pointMeta: entries.map(({ point }, index) => ({
              breakBefore: index > 0 && values[index - 1] == null && values[index] != null,
              provisional: isLevel && point?.displayValuation?.quality?.coverage === "provisional",
              qualityLabel: isLevel ? valuationDetails(t, point?.displayValuation) : "",
            })) }]}
            extraTableColumns={[t("charts.date"), metricLabel(t, metric), t("connections.coverage")]}
            extraTableRows={entries.map(({ key, point }, index) => [
              key,
              metric === "return_rate" ? rateText(point?.rate) : valuationTableAmount(t, isLevel ? signedAccountValuation(point?.displayValuation, metric === "liabilities") : null, signedAccountMoney(point?.value, metric === "liabilities"), metric === "net_worth"),
              [values[index] == null ? t("insights.trendNoPlottedValue") : "", point ? trendCoverage(t, point, isLevel) : ""].filter(Boolean).join(" · "),
            ])}
            height={280} currency={currency} axisValueFormatter={metric === "return_rate" ? (value) => formatDecimalRatioPercent(value, 1, false, false) : undefined} valueFormatter={chartValueText} emptyTitle={t("charts.insufficientHistory")}
          />
        </CardContent>
      </Card>
      {!data.valueChange && <Card>
        <CardContent className="pt-6">
          <div>
            <p className="text-sm text-muted-foreground">{t("insights.periodValue")}</p>
            <p className="mt-1 text-2xl font-semibold">{metric === "return_rate" ? rateText(data.rate) : amountText(data.summary)}</p>
            {metric === "return_rate" && <ReturnCoverage data={data} rate={data.rate} from={from} to={to} />}
          </div>
        </CardContent>
      </Card>}
    </>
  );
  return <div className="flex flex-col gap-4" data-testid="asset-trend">{(!hasTimeline || trend.isError) && toolbar}{results}</div>;
}
