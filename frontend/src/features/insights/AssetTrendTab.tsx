import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { NativeSelect } from "@/components/ui/select";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { TrendChart } from "@/components/charts/TrendChart";
import { useAssetTrend } from "@/queries/returnAnalysis";
import type { AssetTrendDTO, SignedMoneyView } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import type { AnalysisSessionState } from "@/stores/analysis";
import { AvailabilityMarks } from "@/features/insights/CompletenessBanner";
import { useAnalysisProjectionContext } from "@/features/insights/analysisProjectionContext";
import { useSettings } from "@/queries/settings";
import { formatAmount } from "@/lib/money";

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

function metricLabel(t: (key: string) => string, value: string): string {
  const key: Record<string, string> = { net_worth: "metricNetWorth", assets: "metricAssets", liabilities: "metricLiabilities", income: "income", spending: "spending", fees: "fees", dividend_interest: "components.dividendInterest", external_flow: "externalFlows", price_change: "components.priceChange", fx_impact: "components.fxImpact", investment_return: "metricInvestmentReturn", return_rate: "metricReturnRate", net_change: "metricNetChange", residual: "components.residual" };
  return key[value] ? t(`insights.${key[value]}`) : value;
}

function trendEmpty(data: AssetTrendDTO | undefined, t: (key: string) => string) {
  const hasHistory = Boolean(data && (data.points?.length ?? 0) > 0);
  return <EmptyState title={t(hasHistory ? "insights.historyInsufficient" : "insights.noInvestmentAssets")} description={t(hasHistory ? "insights.historyInsufficientHint" : "insights.noInvestmentAssetsHint")} />;
}

export function AssetTrendTab({ session }: { session: AnalysisSessionState }) {
  const { t } = useTranslation();
  const settings = useSettings();
  const [granularity, setGranularity] = useState("month");
  const [metric, setMetric] = useState("net_worth");
  const context = useAnalysisProjectionContext(session);
  const trend = useAssetTrend(context.request, granularity, metric, context.enabled);
  const data = trend.data;
  const dates = useMemo(() => (data?.points ?? []).map((point) => point.period), [data?.points]);
  const values = useMemo(() => (data?.points ?? []).map((point) => metric === "return_rate" ? point.rate : point.value?.amount ?? null), [data?.points, metric]);
  const currency = data?.summary?.currency ?? data?.points?.find((point) => point.value?.currency)?.value?.currency ?? settings.data?.currency ?? "";

  if (context.origin.isLoading) return <LoadingState label={t("insights.loading")} />;
  if (context.origin.isError) return <ErrorState title={t("insights.error")} description={t("ui.state.errorDescription")} onRetry={() => context.origin.refetch()} retryLabel={t("common.retryAction")} />;
  if (!context.origin.data) return <EmptyState title={t("insights.noOrigin")} description={t("insights.noOriginHint")} />;
  if (!context.scopeReady) return <EmptyState title={t("insights.scopeRequired")} description={t("insights.scopeRequiredHint")} />;
  if (!context.rangeAvailable) return <EmptyState title={t("insights.historyInsufficient")} description={t("insights.historyInsufficientHint")} />;

  const chartValueText = (value: string | null | undefined) => metric === "return_rate" ? rateText(value) : amountText(value == null ? null : { amount: value, currency });
  const toolbar = (
    <div className="flex flex-wrap items-end gap-3 rounded-xl border border-border bg-card/60 p-4">
      <div className="flex flex-col gap-1.5"><label htmlFor="asset-trend-granularity" className="text-sm font-medium">{t("insights.granularity")}</label><NativeSelect id="asset-trend-granularity" value={granularity} onChange={(event) => setGranularity(event.target.value)}><option value="day">{t("insights.day")}</option><option value="week">{t("insights.week")}</option><option value="month">{t("insights.month")}</option></NativeSelect></div>
      <div className="flex min-w-52 flex-col gap-1.5"><label htmlFor="asset-trend-metric" className="text-sm font-medium">{t("insights.metric")}</label><NativeSelect id="asset-trend-metric" value={metric} onChange={(event) => setMetric(event.target.value)}><optgroup label={t("insights.metricAssetsGroup")}><option value="net_worth">{metricLabel(t, "net_worth")}</option><option value="assets">{metricLabel(t, "assets")}</option><option value="liabilities">{metricLabel(t, "liabilities")}</option></optgroup><optgroup label={t("insights.metricCashFlowGroup")}><option value="external_flow">{metricLabel(t, "external_flow")}</option><option value="income">{metricLabel(t, "income")}</option><option value="spending">{metricLabel(t, "spending")}</option><option value="fees">{metricLabel(t, "fees")}</option></optgroup><optgroup label={t("insights.metricPerformanceGroup")}><option value="investment_return">{metricLabel(t, "investment_return")}</option><option value="return_rate">{metricLabel(t, "return_rate")}</option><option value="dividend_interest">{metricLabel(t, "dividend_interest")}</option><option value="price_change">{metricLabel(t, "price_change")}</option><option value="fx_impact">{metricLabel(t, "fx_impact")}</option><option value="net_change">{metricLabel(t, "net_change")}</option><option value="residual">{metricLabel(t, "residual")}</option></optgroup></NativeSelect></div>
    </div>
  );
  let results;
  if (trend.isLoading) results = <LoadingState label={t("insights.loading")} />;
  else if (trend.isError) results = <ErrorState title={t("insights.error")} description={t("ui.state.errorDescription")} onRetry={() => trend.refetch()} retryLabel={t("common.retryAction")} />;
  else if (!data?.available) results = trendEmpty(data, t);
  else results = (
    <>
      <Card>
        <CardHeader className="flex-row items-center justify-between gap-3">
          <CardTitle>{metricLabel(t, metric)}</CardTitle>
          <AvailabilityMarks status={data.status} missingReason={data.missingReason} valuationForced={data.valuationForced} ratedDays={data.ratedDays} totalDays={data.totalDays} />
        </CardHeader>
        <CardContent>{dates.length === 0 || values.every((value) => value == null) ? <EmptyState title={t("charts.insufficientHistory")} /> : <TrendChart ariaLabel={metricLabel(t, metric)} summary={t("insights.assetTrendChartSummary")} dates={dates} series={[{ key: metric, name: metricLabel(t, metric), color: "hsl(var(--chart-2))", values }]} currency={currency} valueFormatter={chartValueText} emptyTitle={t("charts.insufficientHistory")} />}</CardContent>
      </Card>
      <Card>
        <CardContent className="pt-6">
          <div>
            <p className="text-xs uppercase tracking-wide text-muted-foreground">{t("insights.periodValue")}</p>
            <p className="mt-1 text-2xl font-semibold">{metric === "return_rate" ? rateText(data.rate) : amountText(data.summary)}</p>
            {metric === "return_rate" && <p className="mt-1 text-sm text-muted-foreground">{t("insights.coverage")}: {data.ratedDays}/{data.totalDays}</p>}
          </div>
        </CardContent>
      </Card>
    </>
  );
  return <div className="flex flex-col gap-4" data-testid="asset-trend">{toolbar}{results}</div>;
}
