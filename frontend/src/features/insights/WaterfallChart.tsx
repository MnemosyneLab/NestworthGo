import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { chartNumber, chartTheme } from "@/components/charts/chartTheme";
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
  if (!summary.endingValue) {
    return steps;
  }
  steps.push(makeStep("ending", labels.ending, summary.endingValue, "0", false));
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
  const reconciled = waterfallReconciles(steps);
  const drivers = steps.filter((step) => step.delta);
  const limit = Math.max(...drivers.map((step) => Math.abs(chartNumber(step.amount) ?? 0)), 1);
  const endingKnown = steps.some((step) => step.key === "ending");
  return (
    <div data-testid="asset-waterfall">
      {!endingKnown && <div role="alert" className="mb-3 rounded-md border border-warning/40 bg-warning/10 p-3 text-sm text-warning-foreground">{t("insights.waterfallIncomplete")}</div>}
      {endingKnown && !reconciled && <div role="alert" className="mb-3 rounded-md border border-warning/40 bg-warning/10 p-3 text-sm text-warning-foreground">{t("insights.waterfallMismatch")}</div>}
      <p className="mb-2 text-sm text-muted-foreground">{t("changeContributions.help")}</p>
      <div className="divide-y divide-border rounded-xl border border-border bg-card px-4">
        {drivers.length === 0 && <p className="py-5 text-sm text-muted-foreground">{t("changeContributions.noChanges")}</p>}
        {drivers.map((step) => {
          const negative = compareCanonical(step.amount, "0") < 0;
          const width = Math.abs(chartNumber(step.amount) ?? 0) / limit * 50;
          const color = step.key === "residual" ? theme.warning : negative ? theme.gainNegative : theme.gainPositive;
          return (
            <button key={step.key} type="button" disabled={!onSelect} onClick={() => onSelect?.(step.key)} className="grid w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-2 rounded-sm py-4 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring enabled:hover:bg-muted/40 sm:grid-cols-[minmax(7rem,1fr)_minmax(0,2fr)_minmax(8rem,1fr)]">
              <span className="text-sm font-medium">{step.label}</span>
              <span className="relative col-span-2 row-start-2 h-6 sm:col-span-1 sm:row-start-auto" aria-hidden="true">
                <span className="absolute inset-y-0 left-1/2 w-px bg-border" />
                <span className="absolute top-1 h-4 rounded-sm" style={{ backgroundColor: color, width: `${width}%`, left: negative ? `${50 - width}%` : "50%" }} />
              </span>
              <span className="col-start-2 row-start-1 whitespace-nowrap text-right text-sm font-semibold tabular-nums sm:col-start-3" style={{ color }}>{signedAmount(step.amount, step.currency)}</span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

export { stepsFromData, waterfallReconciles };
