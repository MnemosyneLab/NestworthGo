import { useState } from "react";
import { useTranslation } from "react-i18next";
import { EChart, type EChartsOption } from "@/components/charts/EChart";
import { chartNumber, chartTheme, joinTooltipLines } from "@/components/charts/chartTheme";
import { collapseChartCategories, type ChartCategory } from "@/components/charts/collapseCategories";
import { Badge } from "@/components/ui/badge";
import { formatAmount, formatPercent } from "@/lib/money";
import { cn } from "@/lib/utils";

function distinctNative(slice: ChartCategory, householdCurrency: string): boolean {
  return Boolean(slice.nativeAmount && slice.nativeCurrency && slice.nativeCurrency !== householdCurrency);
}

function sliceTooltipAmount(slice: ChartCategory, householdCurrency: string): string {
  const household = `${formatAmount(slice.amount, householdCurrency)} (${formatPercent(slice.shareBps)})`;
  if (!distinctNative(slice, householdCurrency)) {
    return household;
  }
  return `${formatAmount(slice.nativeAmount as string, slice.nativeCurrency as string)} · ${household}`;
}

export interface CompositionChartProps {
  title: string;
  description?: string;
  items: ChartCategory[];
  currency: string;
  centerValue: string;
  centerCaption: string;
  ariaLabel: string;
  summary: string;
  height?: number;
  missingCount?: number;
  complete?: boolean;
}

export function CompositionChart({
  title,
  description,
  items,
  currency,
  centerValue,
  centerCaption,
  ariaLabel,
  summary,
  height = 280,
  missingCount = 0,
  complete = true,
}: CompositionChartProps) {
  const { t } = useTranslation();
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [hoveredKey, setHoveredKey] = useState<string | null>(null);
  const theme = chartTheme();
  const slices = collapseChartCategories(items).map((item) => (
    item.key === "other" ? { ...item, label: t("charts.otherCategory") } : item
  ));
  const showNative = slices.some((slice) => distinctNative(slice, currency));
  const activeKey = hoveredKey ?? selectedKey;

  if (slices.length === 0) {
    return null;
  }

  const keyFromLabel = (name: string | null) => slices.find((slice) => slice.label === name)?.key ?? null;

  const option: EChartsOption = {
    tooltip: {
      trigger: "item",
      confine: true,
      transitionDuration: 0,
      formatter: (params) => {
        const point = params as { name?: string; dataIndex?: number };
        const slice = slices[point.dataIndex ?? -1];
        if (!slice) {
          return "";
        }
        return joinTooltipLines([slice.label, sliceTooltipAmount(slice, currency)]);
      },
    },
    series: [
      {
        type: "pie",
        name: title,
        radius: ["52%", "74%"],
        center: ["50%", "50%"],
        selectedMode: "single",
        data: slices.map((slice, index) => ({
          name: slice.label,
          value: chartNumber(slice.amount) ?? 0,
          itemStyle: { color: theme.palette[index % theme.palette.length] },
        })),
        label: { show: false },
        emphasis: { disabled: true },
      },
    ],
  };

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(16rem,0.9fr)]" data-testid="composition-chart">
      <EChart
          option={option}
          height={height}
          ariaLabel={ariaLabel}
          summary={summary}
          dataTableLabel={t("charts.viewDataTable")}
          dataTableColumns={showNative
            ? [t("charts.category"), t("accounts.nativeAmount"), t("accounts.householdAmount"), t("charts.share")]
            : [t("charts.category"), t("charts.amount"), t("charts.share")]}
          dataTableRows={slices.map((item) => showNative
            ? [
              item.label,
              distinctNative(item, currency) && item.nativeAmount && item.nativeCurrency
                ? formatAmount(item.nativeAmount, item.nativeCurrency)
                : formatAmount(item.amount, currency),
              formatAmount(item.amount, currency),
              formatPercent(item.shareBps),
            ]
            : [item.label, formatAmount(item.amount, currency), formatPercent(item.shareBps)])}
          onSelectName={(name) => {
            const key = keyFromLabel(name);
            setSelectedKey((current) => (key && current === key ? null : key));
          }}
          center={(
            <>
              <p className="text-lg font-semibold tracking-tight">{formatAmount(centerValue, currency)}</p>
              <p className="text-xs text-muted-foreground">{centerCaption}</p>
            </>
          )}
        />
      <div className="flex flex-col gap-2">
        {!complete && (
          <p className="text-sm text-warning-foreground">
            {t("charts.partialValued", { count: missingCount })}
          </p>
        )}
        {slices.map((slice) => (
          <button
            type="button"
            key={slice.key}
            data-testid={`composition-slice-${slice.key}`}
            aria-pressed={activeKey === slice.key}
            className={cn(
              "flex items-center justify-between rounded-md px-2 py-1.5 text-left text-sm",
              activeKey === slice.key && "bg-muted ring-2 ring-primary/40",
            )}
            onClick={() => setSelectedKey((current) => (current === slice.key ? null : slice.key))}
            onMouseEnter={() => setHoveredKey(slice.key)}
            onMouseLeave={() => setHoveredKey(null)}
            onFocus={() => setHoveredKey(slice.key)}
            onBlur={() => setHoveredKey(null)}
          >
            <span className="text-foreground">{slice.label}</span>
            <span className="flex items-center gap-2 text-muted-foreground">
              <span className="flex flex-col items-end">
                {distinctNative(slice, currency) && slice.nativeAmount && slice.nativeCurrency && (
                  <span data-testid={`composition-native-${slice.key}`}>{formatAmount(slice.nativeAmount, slice.nativeCurrency)}</span>
                )}
                <span>{formatAmount(slice.amount, currency)}</span>
              </span>
              <Badge variant="secondary">{formatPercent(slice.shareBps)}</Badge>
            </span>
          </button>
        ))}
      </div>
      {description && <p className="text-sm text-muted-foreground lg:col-span-2">{description}</p>}
    </div>
  );
}
