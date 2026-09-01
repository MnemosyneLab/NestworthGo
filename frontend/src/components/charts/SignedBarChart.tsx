import { useTranslation } from "react-i18next";
import { EChart, type EChartsOption } from "@/components/charts/EChart";
import { chartNumber, chartTheme, joinTooltipLines } from "@/components/charts/chartTheme";
import { EmptyState } from "@/components/layout/PageState";
import { compareCanonical, formatAmount } from "@/lib/money";

export interface BarChartItem {
  key: string;
  label: string;
  amount: string;
  currency: string;
  available?: boolean;
}

export interface SignedBarChartProps {
  ariaLabel: string;
  summary: string;
  items: BarChartItem[];
  height?: number;
  emptyLabel: string;
  allowNegative?: boolean;
}

export function SignedBarChart({
  ariaLabel,
  summary,
  items,
  height = 320,
  emptyLabel,
  allowNegative = true,
}: SignedBarChartProps) {
  const { t } = useTranslation();
  const theme = chartTheme();
  const visible = items.filter((item) => item.available !== false);

  if (items.length === 0) {
    return <EmptyState title={emptyLabel} />;
  }

  const option: EChartsOption = {
    tooltip: {
      trigger: "item",
      confine: true,
      transitionDuration: 0,
      formatter: (params) => {
        const point = params as { dataIndex?: number };
        const item = visible[point.dataIndex ?? -1];
        if (!item) {
          return "";
        }
        return joinTooltipLines([item.label, formatAmount(item.amount, item.currency)]);
      },
    },
    grid: { left: 8, right: 16, top: 8, bottom: 8, containLabel: true },
    xAxis: { type: "value", axisLabel: { color: theme.muted }, splitLine: { lineStyle: { color: theme.border } } },
    yAxis: {
      type: "category",
      data: visible.map((item) => item.label),
      axisLabel: { color: theme.foreground },
      inverse: true,
    },
    series: [
      {
        type: "bar",
        emphasis: { disabled: true },
        data: visible.map((item) => {
          const positive = compareCanonical(item.amount, "0") >= 0;
          return {
            value: chartNumber(item.amount) ?? 0,
            itemStyle: { color: allowNegative && !positive ? theme.gainNegative : theme.gainPositive },
          };
        }),
      },
    ],
  };

  return (
    <div className="flex flex-col gap-3">
      {visible.length > 0 ? (
        <EChart
          option={option}
          height={height}
          ariaLabel={ariaLabel}
          summary={summary}
          dataTableLabel={t("charts.viewDataTable")}
          dataTableColumns={[t("charts.category"), t("charts.amount")]}
          dataTableRows={items.map((item) => [
            item.label,
            item.available === false ? t("analytics.statusPartial") : formatAmount(item.amount, item.currency),
          ])}
        />
      ) : null}
      <ul className="flex flex-col gap-1">
        {items.map((item) => (
          <li key={item.key} className="flex items-center justify-between gap-4 text-sm">
            <span>{item.label}</span>
            <span className={compareCanonical(item.amount, "0") >= 0 ? "text-gain-positive" : "text-gain-negative"}>
              {item.available === false ? t("analytics.statusPartial") : formatAmount(item.amount, item.currency)}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}
