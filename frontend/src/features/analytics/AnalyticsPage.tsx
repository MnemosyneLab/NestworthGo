import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { EChart, type EChartsOption } from "@/components/charts/EChart";
import { ErrorState, EmptyState, LoadingState } from "@/components/layout/PageState";
import { PageHeader } from "@/components/layout/PageHeader";
import { useRealizedGain, useDividendIncome, useNetWorthTrend } from "@/queries/analytics";
import { useHistoryOrigin, useRebuildHistoricalSnapshots } from "@/queries/history";
import { useCatalog } from "@/queries/catalog";
import { formatAmount } from "@/lib/money";
import { localDateTimeInTimeZone } from "@/lib/time";
import { cn } from "@/lib/utils";

const TREND_RANGE_LABELS: Record<string, string> = {
  "30d": "analytics.range30",
  "1y": "analytics.range1year",
  all: "analytics.rangeAll",
};

type PeriodGroup = {
  key: string;
  label: string;
  gain: { amount: string; currency: string };
  available?: boolean;
};

function PeriodGroupList({
  title,
  groups,
  emptyLabel,
  incompleteLabel,
}: {
  title: string;
  groups: PeriodGroup[];
  emptyLabel: string;
  incompleteLabel: string;
}) {
  return (
    <div>
      <p className="mb-2 text-sm font-medium">{title}</p>
      {groups.length === 0 ? (
        <p className="text-sm text-muted-foreground">{emptyLabel}</p>
      ) : (
        <ul className="flex flex-col gap-1">
          {groups.map((group) => (
            <li key={group.key} className="flex items-center justify-between gap-4 text-sm">
              <span>{group.label}</span>
              <span className={Number(group.gain.amount) >= 0 ? "text-gain-positive" : "text-gain-negative"}>
                {group.available === false ? incompleteLabel : formatAmount(group.gain.amount, group.gain.currency)}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function shiftYmd(ymd: string, days: number): string {
  const [year, month, day] = ymd.split("-").map(Number);
  const date = new Date(year, (month ?? 1) - 1, (day ?? 1) + days);
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
}

/** Analytics separates trend context from realized-gain detail and exposes
 * the same chart data in an accessible table disclosure. */
export function AnalyticsPage() {
  const { t } = useTranslation();
  const catalog = useCatalog();
  const origin = useHistoryOrigin();
  const rebuild = useRebuildHistoricalSnapshots();
  const ranges = catalog.data?.trendRanges ?? [];
  const [range, setRange] = useState("30d");
  const realizedGain = useRealizedGain(range);
  const dividendIncome = useDividendIncome(range);
  const netWorthTrend = useNetWorthTrend(range);

  useEffect(() => {
    if (!origin.data) {
      return;
    }
    const today = localDateTimeInTimeZone(origin.data.timezone)?.date;
    const start = localDateTimeInTimeZone(origin.data.timezone, new Date(origin.data.startedAt))?.date;
    if (!today || !start) {
      return;
    }
    const yesterday = shiftYmd(today, -1);
    if (start <= yesterday) {
      rebuild.mutate({ startDate: start, endDate: yesterday });
    }
    // Rebuild once per origin load; mutate identity is stable enough for this page.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [origin.data?.id, origin.data?.startedAt, origin.data?.timezone]);

  if (realizedGain.isLoading || dividendIncome.isLoading || netWorthTrend.isLoading) {
    return <LoadingState label={t("analytics.loading")} />;
  }

  if (realizedGain.isError || dividendIncome.isError) {
    return (
      <ErrorState
        title={t("analytics.loadError")}
        description={t("ui.state.errorDescription")}
        onRetry={() => {
          void realizedGain.refetch();
          void dividendIncome.refetch();
          void netWorthTrend.refetch();
        }}
        retryLabel={t("common.retryAction")}
      />
    );
  }

  const points = netWorthTrend.data?.points ?? [];
  const today = origin.data ? localDateTimeInTimeZone(origin.data.timezone)?.date : undefined;
  const todayOnlyTrend = points.length <= 1 && (!points[0] || points[0].localDate === today);
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
  const incomeData = dividendIncome.data;

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t("nav.analytics")} description={t("analytics.description")} />

      <div className="flex flex-wrap items-center gap-2" role="group" aria-label={t("analytics.range")}>
        {ranges.map((id) => (
          <Button key={id} variant={range === id ? "default" : "outline" } size="sm" onClick={() => setRange(id)}>
            {t(TREND_RANGE_LABELS[id] ?? id)}
          </Button>
        ))}
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("analytics.trend")}</CardTitle>
        </CardHeader>
        <CardContent>
          {netWorthTrend.isError || rebuild.isError ? (
            <ErrorState
              title={t("analytics.loadError")}
              description={t("ui.state.errorDescription")}
              onRetry={() => {
                void netWorthTrend.refetch();
                if (origin.data) {
                  const todayDate = localDateTimeInTimeZone(origin.data.timezone)?.date;
                  const start = localDateTimeInTimeZone(origin.data.timezone, new Date(origin.data.startedAt))?.date;
                  if (todayDate && start) {
                    rebuild.mutate({ startDate: start, endDate: shiftYmd(todayDate, -1) });
                  }
                }
              }}
              retryLabel={t("common.retryAction")}
            />
          ) : todayOnlyTrend ? (
            <EmptyState title={t("analytics.trendEmpty")} description={t("analytics.chartSummary")} />
          ) : (
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
          <PeriodGroupList
            title={t("analytics.byInstrument")}
            groups={gainData?.byInstrument ?? []}
            emptyLabel={t("analytics.empty")}
            incompleteLabel={t("analytics.statusPartial")}
          />
          <PeriodGroupList
            title={t("analytics.byAccount")}
            groups={gainData?.byAccount ?? []}
            emptyLabel={t("analytics.empty")}
            incompleteLabel={t("analytics.statusPartial")}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className={cn(incomeData && !incomeData.available && "text-warning")}>
            {t("analytics.dividendIncome")} {incomeData && !incomeData.available && `(${t("analytics.statusPartial")})`}
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-5">
          <PeriodGroupList
            title={t("analytics.byInstrument")}
            groups={incomeData?.byInstrument ?? []}
            emptyLabel={t("analytics.empty")}
            incompleteLabel={t("analytics.statusPartial")}
          />
          <PeriodGroupList
            title={t("analytics.byAccount")}
            groups={incomeData?.byAccount ?? []}
            emptyLabel={t("analytics.empty")}
            incompleteLabel={t("analytics.statusPartial")}
          />
        </CardContent>
      </Card>
    </div>
  );
}
