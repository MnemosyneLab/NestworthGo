import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { EChart, type EChartsOption } from "@/components/charts/EChart";
import { ErrorState, EmptyState, LoadingState } from "@/components/layout/PageState";
import { PageHeader } from "@/components/layout/PageHeader";
import { useRealizedGain, useNetWorthTrend } from "@/queries/analytics";
import { formatAmount } from "@/lib/money";
import { cn } from "@/lib/utils";

const RANGES = [
  { id: "30d", labelKey: "analytics.range30" },
  { id: "1y", labelKey: "analytics.range1year" },
  { id: "all", labelKey: "analytics.rangeAll" },
];

/** Analytics separates trend context from realized-gain detail and exposes
 * the same chart data in an accessible table disclosure. */
export function AnalyticsPage() {
  const { t } = useTranslation();
  const [range, setRange] = useState("30d");
  const realizedGain = useRealizedGain(range);
  const netWorthTrend = useNetWorthTrend(range);

  if (realizedGain.isLoading || netWorthTrend.isLoading) {
    return <LoadingState label={t("analytics.loading")} />;
  }

  if (realizedGain.isError || netWorthTrend.isError) {
    return (
      <ErrorState
        title={t("analytics.loadError")}
        description={t("ui.state.errorDescription")}
        onRetry={() => {
          void realizedGain.refetch();
          void netWorthTrend.refetch();
        }}
        retryLabel={t("common.retryAction")}
      />
    );
  }

  const points = netWorthTrend.data?.points ?? [];
  const trendOption: EChartsOption = {
    xAxis: { type: "category" as const, data: points.map((point) => point.localDate) },
    yAxis: { type: "value" as const },
    tooltip: { trigger: "axis" as const },
    series: [
      {
        type: "line" as const,
        name: t("analytics.trend"),
        data: points.map((point) => (point.value ? Number(point.value.amount) : null)),
        connectNulls: false,
      },
    ],
  };
  const gainData = realizedGain.data;

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t("nav.analytics")} description={t("analytics.description")} />

      <div className="flex flex-wrap items-center gap-2" role="group" aria-label={t("analytics.range")}>
        {RANGES.map((option) => (
          <Button key={option.id} variant={range === option.id ? "default" : "outline"} size="sm" onClick={() => setRange(option.id)}>
            {t(option.labelKey)}
          </Button>
        ))}
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("analytics.trend")}</CardTitle>
        </CardHeader>
        <CardContent>
          {points.length > 0 ? (
            <EChart
              option={trendOption}
              style={{ height: 320 }}
              ariaLabel={t("analytics.trend")}
              summary={t("analytics.chartSummary")}
              dataTableLabel={t("analytics.dataTable")}
              dataTableColumns={[t("analytics.dataTableDate"), t("analytics.dataTableValue")]}
              dataTableRows={points.map((point) => [
                point.localDate,
                point.value ? formatAmount(point.value.amount, point.value.currency) : t("accounts.noValue"),
              ])}
            />
          ) : (
            <EmptyState title={t("analytics.empty")} description={t("analytics.chartSummary")} />
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className={cn(gainData && !gainData.available && "text-warning")}>
            {t("analytics.realizedGain")} {gainData && !gainData.available && `(${t("analytics.statusPartial")})`}
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-5">
          <div>
            <p className="mb-2 text-sm font-medium">{t("analytics.byInstrument")}</p>
            {(gainData?.byInstrument ?? []).length === 0 ? (
              <p className="text-sm text-muted-foreground">{t("analytics.empty")}</p>
            ) : (
              <ul className="flex flex-col gap-1">
                {(gainData?.byInstrument ?? []).map((group) => (
                  <li key={group.key} className="flex items-center justify-between gap-4 text-sm">
                    <span>{group.label}</span>
                    <span className={Number(group.gain.amount) >= 0 ? "text-gain-positive" : "text-gain-negative"}>
                      {formatAmount(group.gain.amount, group.gain.currency)}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </div>
          <div>
            <p className="mb-2 text-sm font-medium">{t("analytics.byAccount")}</p>
            {(gainData?.byAccount ?? []).length === 0 ? (
              <p className="text-sm text-muted-foreground">{t("analytics.empty")}</p>
            ) : (
              <ul className="flex flex-col gap-1">
                {(gainData?.byAccount ?? []).map((group) => (
                  <li key={group.key} className="flex items-center justify-between gap-4 text-sm">
                    <span>{group.label}</span>
                    <span className={Number(group.gain.amount) >= 0 ? "text-gain-positive" : "text-gain-negative"}>
                      {formatAmount(group.gain.amount, group.gain.currency)}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
