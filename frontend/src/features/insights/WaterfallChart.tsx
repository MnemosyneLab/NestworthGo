import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { EChart, type EChartsOption } from "@/components/charts/EChart";
import { chartNumber, chartTheme, joinTooltipLines } from "@/components/charts/chartTheme";
import { EmptyState } from "@/components/layout/PageState";
import type { AssetChangeRowDTO, AssetChangeSummaryDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { addCanonical, compareCanonical, formatAmount } from "@/lib/money";

interface WaterfallStep {
  key: string;
  label: string;
  amount: string;
  currency: string;
  start: string;
  end: string;
  base: string;
  span: string;
  delta: boolean;
}

function signedAmount(amount: string, currency: string): string {
  const formatted = formatAmount(amount, currency);
  return amount.startsWith("-") || amount === "0" ? formatted : `+${formatted}`;
}

function absoluteCanonical(value: string): string {
  return value.startsWith("-") ? value.slice(1) : value;
}

function makeStep(key: string, label: string, amount: { amount: string; currency: string }, start: string, delta: boolean): WaterfallStep {
  const end = delta ? addCanonical(start, amount.amount) : amount.amount;
  const positive = compareCanonical(end, start) >= 0;
  return {
    key,
    label,
    amount: delta ? amount.amount : end,
    currency: amount.currency,
    start,
    end,
    base: delta ? (positive ? start : end) : (compareCanonical(end, "0") >= 0 ? "0" : end),
    span: delta ? absoluteCanonical(amount.amount) : absoluteCanonical(end),
    delta,
  };
}

function stepsFromData(summary: AssetChangeSummaryDTO, rows: AssetChangeRowDTO[], labels: { beginning: string; ending: string }): WaterfallStep[] {
  const beginning = summary.beginningValue;
  if (!beginning) {
    return [];
  }
  const steps: WaterfallStep[] = [makeStep("beginning", labels.beginning, beginning, "0", false)];
  let running = beginning.amount;
  for (const row of rows) {
    if (!row.amount || row.amount.amount === "0") {
      continue;
    }
    steps.push(makeStep(row.key, row.label, row.amount, running, true));
    running = steps[steps.length - 1]?.end ?? running;
  }
  const ending = summary.endingValue ?? { amount: running, currency: beginning.currency };
  steps.push(makeStep("ending", labels.ending, ending, "0", false));
  return steps;
}

function waterfallReconciles(steps: WaterfallStep[]): boolean {
  const beginning = steps.find((step) => step.key === "beginning");
  const ending = steps.find((step) => step.key === "ending");
  if (!beginning || !ending) return false;
  const lastDriver = [...steps].reverse().find((step) => step.delta);
  const running = lastDriver?.end ?? beginning.end;
  return ending.currency === (lastDriver?.currency ?? beginning.currency) && compareCanonical(running, ending.end) === 0;
}

export function WaterfallChart({ summary, rows, labels, onSelect }: { summary: AssetChangeSummaryDTO; rows: AssetChangeRowDTO[]; labels: { beginning: string; ending: string; amount: string; empty: string }; onSelect?: (key: string) => void }) {
  const { t } = useTranslation();
  const theme = chartTheme();
  const steps = useMemo(() => stepsFromData(summary, rows, labels), [labels, rows, summary]);
  if (steps.length === 0) {
    return <EmptyState title={labels.empty} />;
  }
  const currency = steps.find((step) => step.currency)?.currency ?? "";
  const reconciled = waterfallReconciles(steps);
  const option: EChartsOption = {
    tooltip: {
      trigger: "axis",
      confine: true,
      transitionDuration: 0,
      formatter: (params) => {
        const first = (Array.isArray(params) ? params[0] : params) as { dataIndex?: number };
        const step = steps[first.dataIndex ?? -1];
        return step ? joinTooltipLines([step.label, signedAmount(step.amount, step.currency)]) : "";
      },
    },
    grid: { left: 8, right: 16, top: 16, bottom: 8, containLabel: true },
    xAxis: { type: "category", data: steps.map((step) => step.label), axisLabel: { color: theme.muted, interval: 0, rotate: steps.length > 7 ? 28 : 0 } },
    yAxis: { type: "value", axisLabel: { color: theme.muted }, splitLine: { lineStyle: { color: theme.border } } },
    series: [
      {
        type: "bar",
        stack: "waterfall",
        silent: true,
        itemStyle: { color: "transparent" },
        emphasis: { disabled: true },
        data: steps.map((step) => chartNumber(step.base) ?? 0),
      },
      {
        type: "bar",
        stack: "waterfall",
        emphasis: { disabled: true },
        data: steps.map((step) => ({
          value: chartNumber(step.span) ?? 0,
          itemStyle: { color: step.key === "residual" ? theme.warning : (compareCanonical(step.amount, "0") < 0 ? theme.gainNegative : theme.gainPositive) },
        })),
      },
    ],
  };
  return (
    <div data-testid="asset-waterfall">
      {!reconciled && <div role="alert" className="mb-3 rounded-md border border-warning/40 bg-warning/10 p-3 text-sm text-warning-foreground">{t("insights.waterfallMismatch")}</div>}
      <EChart
        option={option}
        height={340}
        ariaLabel={labels.amount}
        summary={labels.amount}
        dataTableLabel={t("charts.viewDataTable")}
        dataTableColumns={[t("charts.category"), labels.amount]}
        dataTableRows={steps.map((step) => [step.label, signedAmount(step.amount, step.currency || currency)])}
        onSelectName={(name) => {
          const step = steps.find((candidate) => candidate.label === name && candidate.delta);
          if (step) onSelect?.(step.key);
        }}
      />
    </div>
  );
}

export { stepsFromData, waterfallReconciles };
