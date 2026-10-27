import { useTranslation } from "react-i18next";
import { ChangeText } from "@/components/ui/change-text";
import { useQuoteReturns } from "@/queries/investments";
import { formatDecimalPercent } from "@/lib/money";
import { formatCivilDateRange } from "@/lib/time";
import { amountDirection } from "./holdingGroups";

/**
 * Returns over the fixed 7-, 30-, 90-, 180- and 365-day windows. The backend
 * owns every figure and omits a window without enough comparable history, so a
 * missing window is simply not rendered. All-history is a chart period only.
 */
export function PerformanceReturns({ instrumentId }: { instrumentId: string }) {
  const { t, i18n } = useTranslation();
  const returns = useQuoteReturns(instrumentId);
  const items = (returns.data?.items ?? []).filter((item) => item.windowDays > 0);
  if (returns.isLoading) return null;
  if (returns.isError) {
    return <section aria-label={t("holdingPerformance.returns.label")} className="flex items-center gap-2 text-xs text-muted-foreground">
      <p>{t("holdingPerformance.returns.loadError")}</p>
      <button type="button" className="rounded underline underline-offset-4 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50" onClick={() => void returns.refetch()}>{t("common.retryAction")}</button>
    </section>;
  }
  if (items.length === 0) return null;
  return (
    <section aria-label={t("holdingPerformance.returns.label")} className="flex min-w-0 flex-col gap-2 rounded-xl border border-border bg-card p-4">
      <h4 className="text-sm font-medium">{t("holdingPerformance.returns.title")}</h4>
      {items.some(item => item.annualizedPercent != null) && <p className="text-xs text-muted-foreground">{t("holdingPerformance.annualizedNote")}</p>}
      <ul className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
        {items.map((item) => {
          const direction = amountDirection(item.periodPercent) ?? "flat";
          return (
            <li key={item.windowDays} className="flex min-w-0 flex-col gap-0.5 rounded-lg bg-muted/40 px-3 py-2">
              <span className="text-xs text-muted-foreground">{t("holdingPerformance.returns.window", { count: item.windowDays })}</span>
              <span className="num text-lg font-semibold">
                <ChangeText direction={direction}>{`${direction === "up" ? "+" : ""}${formatDecimalPercent(item.periodPercent)}`}</ChangeText>
              </span>
              <span className="num text-xs text-muted-foreground">{formatCivilDateRange(item.baselineOn, item.latestOn, i18n.language)}</span>
              {item.annualizedPercent != null && <span className="num text-xs text-muted-foreground">{t("holdingPerformance.annualized", { percent: formatDecimalPercent(item.annualizedPercent) })}</span>}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
