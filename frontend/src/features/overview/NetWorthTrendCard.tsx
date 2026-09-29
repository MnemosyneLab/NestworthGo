import { useTranslation } from "react-i18next";
import { useNetWorthTrend } from "@/queries/analytics";
import { useObjectNavigation } from "@/app/NavigationContext";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { DateRangeControl } from "@/components/charts/DateRangeControl";
import { useTrendDateRange } from "@/components/charts/useTrendDateRange";
import { TrendChart } from "@/components/charts/TrendChart";
import { ErrorState, LoadingState } from "@/components/layout/PageState";
import { Button } from "@/components/ui/button";
import { formatAmount } from "@/lib/money";

export function NetWorthTrendCard() {
  const { t } = useTranslation();
  const range = useTrendDateRange();
  const trend = useNetWorthTrend({ kind: "trend", value: range.queryRange });
  const navigation = useObjectNavigation();
  const data = trend.data;
  const points = data?.points ?? [];
  const gaps = points.filter(point => !point.complete);
  return <Card><CardHeader><CardTitle>{t("connections.netWorthTrend")}</CardTitle></CardHeader><CardContent className="flex flex-col gap-4">
    <DateRangeControl {...range} />
    <p className="text-sm text-muted-foreground">{t("connections.netWorthNote")}</p>
    {trend.isLoading ? <LoadingState label={t("ui.state.loadingPage")} /> : trend.isError ? <ErrorState title={t("overview.loadError")} description={t("ui.state.errorDescription")} onRetry={() => void trend.refetch()} retryLabel={t("common.retryAction")} /> : <>
      <p className="text-sm">{data?.startDate} {data?.endDate && `– ${data.endDate}`}</p>
      <p className="num text-lg font-bold">{t("connections.periodChange")}: {data?.change ? formatAmount(data.change.amount, data.change.currency) : t("connections.changeUnavailable")}</p>
      {!data?.change && <p className="text-sm text-muted-foreground">{t(data?.summaryReason === "missing_boundary" ? "connections.missingBoundary" : "charts.insufficientHistory")}</p>}
      <TrendChart ariaLabel={t("connections.netWorthTrend")} summary={t("connections.netWorthNote")} dates={points.map(point => point.localDate)}
        series={[{ key: "netWorth", name: t("overview.netWorth"), color: "var(--color-primary)", values: points.map(point => point.complete ? point.netWorth?.amount : null) }]}
        currency={data?.currency ?? ""} height={280} emptyTitle={t("charts.insufficientHistory")}
        extraTableColumns={[t("charts.date"), t("overview.netWorth"), t("overview.assets"), t("overview.liabilities"), t("connections.coverage")]}
        extraTableRows={points.map(point => [
          point.localDate,
          ...[point.netWorth, point.assets, point.liabilities].map(value => point.complete && value ? formatAmount(value.amount, value.currency) : t("accounts.noValue")),
          t(point.current ? "connections.currentPoint" : point.complete ? "overview.healthy" : "overview.needsAttention"),
        ])} />
      {points.length > 0 && <p className="text-xs text-muted-foreground">{t("connections.currentPointNote")}</p>}
      {gaps.length > 0 && <details><summary className="cursor-pointer text-sm">{t("connections.gapDays", { count: gaps.length })}</summary><ul className="mt-2 flex max-h-48 flex-col gap-1 overflow-y-auto">{gaps.map(point => <li key={point.localDate}><Button variant="link" size="sm" disabled={!navigation} onClick={() => navigation?.open({ page: "data-health", focus: { rangeStart: point.localDate, rangeEnd: point.localDate } })}>{point.localDate} · {t("connections.viewGap")}</Button></li>)}</ul></details>}
    </>}
  </CardContent></Card>;
}
