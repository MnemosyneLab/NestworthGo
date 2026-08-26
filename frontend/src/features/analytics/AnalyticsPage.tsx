import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { EChart, type EChartsOption } from "@/components/charts/EChart";
import { useRealizedGain, useNetWorthTrend } from "@/queries/analytics";
import { formatAmount } from "@/lib/money";
import { cn } from "@/lib/utils";

const RANGES = [
  { id: "30d", labelKey: "analytics.range30" },
  { id: "1y", labelKey: "analytics.range1year" },
  { id: "all", labelKey: "analytics.rangeAll" },
];

/**
 * AnalyticsPage implements realized gain by period and a net worth trend
 * chart (implementation plan Phase 5) through AnalyticsService/
 * PortfolioService, reusing the range selector pattern from
 * domain.TrendRange ("30d"/"1y"/"all"), the same values History's
 * eventual date-range filter and the current Fyne Analytics page already
 * use.
 */
export function AnalyticsPage() {
  const { t } = useTranslation();
  const [range, setRange] = useState("30d");
  const realizedGain = useRealizedGain(range);
  const netWorthTrend = useNetWorthTrend(range);

  const trendOption: EChartsOption = {
    xAxis: { type: "category" as const, data: (netWorthTrend.data?.points ?? []).map((point) => point.localDate) },
    yAxis: { type: "value" as const },
    tooltip: { trigger: "axis" as const },
    series: [
      {
        type: "line" as const,
        name: t("analytics.trend"),
        data: (netWorthTrend.data?.points ?? []).map((point) => (point.value ? Number(point.value.amount) : null)),
        connectNulls: false,
      },
    ],
  };

  return (
    <div className="flex flex-col gap-6">
      <div className="flex gap-2">
        {RANGES.map((option) => (
          <Button
            key={option.id}
            variant={range === option.id ? "default" : "outline"}
            size="sm"
            onClick={() => setRange(option.id)}
          >
            {t(option.labelKey)}
          </Button>
        ))}
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("analytics.trend")}</CardTitle>
        </CardHeader>
        <CardContent>
          {netWorthTrend.data && (netWorthTrend.data.points?.length ?? 0) > 0 ? (
            <div style={{ height: 280 }}>
              <EChart option={trendOption} />
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">{t("analytics.empty")}</p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className={cn(!realizedGain.data?.available && "text-warning")}>
            Realized gain {realizedGain.data && !realizedGain.data.available && `(${t("analytics.statusPartial")})`}
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {realizedGain.isError && (
            <p role="alert" className="text-sm text-destructive">
              {(realizedGain.error as Error).message}
            </p>
          )}
          <div>
            <p className="mb-2 text-sm font-medium">By instrument</p>
            {(realizedGain.data?.byInstrument ?? []).length === 0 && <p className="text-sm text-muted-foreground">{t("analytics.empty")}</p>}
            <ul className="flex flex-col gap-1">
              {(realizedGain.data?.byInstrument ?? []).map((group) => (
                <li key={group.key} className="flex items-center justify-between text-sm">
                  <span>{group.label}</span>
                  <span className={Number(group.gain.amount) >= 0 ? "text-gain-positive" : "text-gain-negative"}>
                    {formatAmount(group.gain.amount, group.gain.currency)}
                  </span>
                </li>
              ))}
            </ul>
          </div>
          <div>
            <p className="mb-2 text-sm font-medium">By account</p>
            {(realizedGain.data?.byAccount ?? []).length === 0 && <p className="text-sm text-muted-foreground">{t("analytics.empty")}</p>}
            <ul className="flex flex-col gap-1">
              {(realizedGain.data?.byAccount ?? []).map((group) => (
                <li key={group.key} className="flex items-center justify-between text-sm">
                  <span>{group.label}</span>
                  <span className={Number(group.gain.amount) >= 0 ? "text-gain-positive" : "text-gain-negative"}>
                    {formatAmount(group.gain.amount, group.gain.currency)}
                  </span>
                </li>
              ))}
            </ul>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
