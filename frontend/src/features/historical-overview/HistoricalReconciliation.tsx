import { useId, useState } from "react";
import { Button } from "@/components/ui/button";
import { useTranslation } from "react-i18next";
import type { HistoricalOverviewResult } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
import { formatHistoricalAmount } from "./historicalAmount";

export function HistoricalReconciliationView({ data }: { data: HistoricalOverviewResult }) {
  const { t } = useTranslation();
  const explanationId = useId();
  const [expanded, setExpanded] = useState(false);
  const amount = (value: string | null, currency: string, signed: boolean) => formatHistoricalAmount(value, currency, signed, true);
  const reconciliation = data.reconciliation;
  if (!data.right || !reconciliation) return null;
  const { currency, total, supported, residual, contributions, reasons, status } = reconciliation;
  return <section aria-label={t("historicalReconciliation.title")} className="min-w-0 rounded-xl border border-border bg-card p-4">
    <h2 className="font-semibold">{t("historicalReconciliation.title")}</h2>
    <p className="mt-1 text-xs text-muted-foreground">{t("historicalReconciliation.identity")}</p>
    <dl className="mt-3 grid grid-cols-[repeat(auto-fit,minmax(min(100%,12rem),1fr))] gap-3">
      {(["total", "supported", "residual"] as const).map(key => <div key={key} className="min-w-0">
        <dt className="text-sm">{t(`historicalReconciliation.${key}`)}</dt>
        <dd tabIndex={0} className="num mt-1 overflow-x-auto whitespace-nowrap font-semibold focus-visible:outline-primary">{amount(({ total, supported, residual })[key], currency, true) ?? t("historicalOverview.unknown")}</dd>
      </div>)}
    </dl>
    <p role="status" className="mt-2 text-sm text-muted-foreground">{t(`historicalReconciliation.status.${status}`)}</p>
    <Button variant="link" className="mt-3 h-auto max-w-full whitespace-normal p-0 text-left" aria-expanded={expanded} aria-controls={explanationId} onClick={() => setExpanded(!expanded)}>{t("historicalReconciliation.explanation")}</Button>
    <div id={explanationId} hidden={!expanded}>
      <p className="mt-2 text-sm text-muted-foreground">{t("historicalReconciliation.basis")}</p>
      <p className="mt-2 text-sm text-muted-foreground">{t("historicalReconciliation.marketBasis")}</p>
      <p className="mt-2 text-sm text-muted-foreground">{t("historicalReconciliation.internal")}</p>
      <div className="mt-3 overflow-x-auto"><table className="w-full text-left text-sm">
        <caption className="text-left text-xs text-muted-foreground">{t("historicalReconciliation.categories")}</caption>
        <thead><tr><th scope="col">{t("historicalReconciliation.contribution")}</th><th scope="col">{currency}</th></tr></thead>
        <tbody>{(contributions ?? []).map(item => <tr key={item.key} className="border-t border-border"><th scope="row" className="p-2 font-normal">{t(`historicalReconciliation.category.${item.key}`)}</th><td className="num whitespace-nowrap p-2">{amount(item.amount, currency, true)}</td></tr>)}
          <tr className="border-t border-border"><th scope="row" className="p-2">{t("historicalReconciliation.residual")}</th><td className="num whitespace-nowrap p-2">{amount(residual, currency, true) ?? t("historicalOverview.unknown")}</td></tr>
        </tbody>
      </table></div>
      {(reasons ?? []).length > 0 && <ul className="mt-3 list-disc space-y-1 pl-5 text-sm">{(reasons ?? []).map(reason => <li key={reason}>{t(`historicalReconciliation.reason.${reason}`)}</li>)}</ul>}
    </div>
  </section>;
}
