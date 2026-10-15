import { useId, useState } from "react";
import { Button } from "@/components/ui/button";
import { useTranslation } from "react-i18next";
import type { HistoricalOverviewResult } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
import { Badge } from "@/components/ui/badge";
import { InfoPopover } from "@/components/ui/info-popover";
import { formatHistoricalAmount } from "./historicalAmount";
import { SignedChange } from "./SignedChange";

export function HistoricalReconciliationView({ data }: { data: HistoricalOverviewResult }) {
  const { t } = useTranslation();
  const explanationId = useId();
  const [expanded, setExpanded] = useState(false);
  const amount = (value: string | null, currency: string, signed: boolean) => formatHistoricalAmount(value, currency, signed);
  const reconciliation = data.reconciliation;
  if (!data.right || !reconciliation) return null;
  const { currency, total, supported, residual, contributions, reasons, status } = reconciliation;
  const known = (["total", "supported", "residual"] as const).filter(key => ({ total, supported, residual })[key] != null);
  const tone = status === "reconciled" ? "success" : status === "partial" ? "warning" : "secondary";
  return <section aria-label={t("historicalReconciliation.title")} className="min-w-0 rounded-xl border border-border bg-card p-4">
    <div className="flex flex-wrap items-center gap-2">
      <h2 className="font-semibold">{t("historicalReconciliation.title")}</h2>
      <InfoPopover label={t("historicalOverview.aboutChange")}>{t("historicalReconciliation.identity")}</InfoPopover>
    </div>
    {known.length > 0 && <dl className="mt-3 grid grid-cols-[repeat(auto-fit,minmax(min(100%,12rem),1fr))] gap-3">
      {known.map(key => <div key={key} className="min-w-0">
        <dt className="text-xs text-muted-foreground">{t(`historicalReconciliation.${key}`)}</dt>
        <dd tabIndex={0} className="mt-1 overflow-x-auto whitespace-nowrap text-lg font-semibold focus-visible:outline-primary">
          {key === "total" ? <SignedChange value={total} currency={currency} className="text-xl" /> : <span className="num">{amount(({ total, supported, residual })[key], currency, true)}</span>}
        </dd>
      </div>)}
    </dl>}
    <p role="status" className={`mt-2 flex items-start gap-2 text-sm ${known.length > 0 ? "text-muted-foreground" : ""}`}>
      <Badge variant={tone} className="shrink-0">{t(`historicalReconciliation.badge.${status}`)}</Badge>
      <span>{t(`historicalReconciliation.status.${status}`)}</span>
    </p>
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
