import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useNetWorthTrend, type AnalyticsTrendRange } from "@/queries/analytics";
import { useObjectNavigation } from "@/app/NavigationContext";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { RangeToggle } from "@/components/charts/RangeToggle";
import { TrendChart } from "@/components/charts/TrendChart";
import { chartTheme } from "@/components/charts/chartTheme";
import { ErrorState, LoadingState } from "@/components/layout/PageState";
import { Button } from "@/components/ui/button";
import { formatAmount } from "@/lib/money";

export function NetWorthTrendCard() {
  const { t } = useTranslation();
  const [range, setRange] = useState<AnalyticsTrendRange>("30d");
  const trend = useNetWorthTrend({ kind: "trend", value: range });
  const navigation = useObjectNavigation();
  const data = trend.data;
  const points = data?.points ?? [];
  const gaps = points.filter(point => !point.complete);
  return <Card><CardHeader><CardTitle>{t("connections.netWorthTrend")}</CardTitle></CardHeader><CardContent className="flex flex-col gap-4">
    <RangeToggle ranges={["30d", "ytd", "1y", "all"]} value={range} onChange={value => setRange(value as AnalyticsTrendRange)} label={t("analytics.range")} />
    <p className="text-sm text-muted-foreground">{t("connections.netWorthNote")}</p>
    {trend.isLoading ? <LoadingState label={t("ui.state.loadingPage")} /> : trend.isError ? <ErrorState title={t("overview.loadError")} description={t("ui.state.errorDescription")} onRetry={() => void trend.refetch()} retryLabel={t("common.retryAction")} /> : <>
      <p className="text-sm">{data?.startDate} {data?.endDate && `– ${data.endDate}`}</p>
      <p className="text-lg font-semibold">{t("connections.periodChange")}: {data?.change ? formatAmount(data.change.amount, data.change.currency) : t("connections.changeUnavailable")}</p>
      {!data?.change && <p className="text-sm text-muted-foreground">{t(data?.summaryReason === "missing_boundary" ? "connections.missingBoundary" : "charts.insufficientHistory")}</p>}
      <TrendChart ariaLabel={t("connections.netWorthTrend")} summary={t("connections.netWorthNote")} dates={points.map(point => point.localDate)}
        series={[{ key: "netWorth", name: t("overview.netWorth"), color: chartTheme().primary, values: points.map(point => point.complete ? point.netWorth?.amount : null) }]}
        currency={data?.currency ?? ""} height={280} emptyTitle={t("charts.insufficientHistory")}
        extraTableColumns={[t("connections.coverage")]} extraTableRows={points.map(point => [t(point.current ? "connections.currentPoint" : point.complete ? "overview.healthy" : "overview.needsAttention")])} />
      {points.length > 0 && <p className="text-xs text-muted-foreground">{t("connections.currentPointNote")}</p>}
      {gaps.length > 0 && <details><summary className="cursor-pointer text-sm">{t("connections.gapDays", { count: gaps.length })}</summary><ul className="mt-2 flex max-h-48 flex-col gap-1 overflow-y-auto">{gaps.map(point => <li key={point.localDate}><Button variant="link" size="sm" disabled={!navigation} onClick={() => navigation?.open({ page: "data-health", focus: { rangeStart: point.localDate, rangeEnd: point.localDate } })}>{point.localDate} · {t("connections.viewGap")}</Button></li>)}</ul></details>}
    </>}
  </CardContent></Card>;
}
