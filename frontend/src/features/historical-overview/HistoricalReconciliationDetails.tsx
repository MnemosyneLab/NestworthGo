import { useId, useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { formatTimestamp } from "@/lib/time";
import { instrumentSelectionLabels } from "@/lib/instrumentDisplay";
import type { HistoricalOverviewResult, HistoricalReconciliationDetail } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";

/** Audit detail retains canonical decimals. Ordinary overview money keeps its
 * shared rounded display; this disclosure is the exact reconciliation. */
function ExactAmount({ value, currency }: { value: string | null; currency?: string }) {
  const { t } = useTranslation();
  return <span className="num break-all">{value == null ? t("historicalOverview.unknown") : `${value}${currency ? ` ${currency}` : ""}`}</span>;
}

function ComponentDetail({ detail, name, currency, timezone }: { detail: HistoricalReconciliationDetail; name: string; currency: string; timezone: string }) {
  const { t } = useTranslation();
  const id = useId();
  const [expanded, setExpanded] = useState(false);
  return <div className="min-w-0 border-t border-border py-2">
    <Button type="button" variant="ghost" className="h-auto w-full justify-start whitespace-normal px-2 text-left" aria-expanded={expanded} aria-controls={id} onClick={() => setExpanded(!expanded)}>
      {expanded ? <ChevronDown aria-hidden="true" /> : <ChevronRight aria-hidden="true" />}
      <span className="min-w-0 flex-1 break-words">{name}</span>
      <span className="min-w-0 text-xs">{t("historicalReconciliation.residual")}: <ExactAmount value={detail.residual} currency={currency} /></span>
    </Button>
    <div id={id} hidden={!expanded} className="px-2 pb-2">
      {expanded && <>
        <p className="mt-2 text-xs text-muted-foreground">{t("historicalReconciliation.detailIdentity")}</p>
        <dl className="mt-2 grid grid-cols-[repeat(auto-fit,minmax(min(100%,12rem),1fr))] gap-2 text-sm">
          {(["total", "supported", "principal", "residual"] as const).map(key => <div key={key} className="min-w-0"><dt className="text-xs text-muted-foreground">{t(`historicalReconciliation.${key}`)}</dt><dd><ExactAmount value={detail[key]} currency={currency} /></dd></div>)}
        </dl>
        {detail.openingQuantity != null && <div className="mt-3">
          <p className="text-xs text-muted-foreground">{t("historicalReconciliation.quantityBasis")}</p>
          <dl className="mt-1 grid grid-cols-2 gap-2 text-sm sm:grid-cols-4">{(["openingQuantity", "activityQuantity", "closingQuantity", "quantityResidual"] as const).map(key => <div key={key} className="min-w-0"><dt className="text-xs text-muted-foreground">{t(`historicalReconciliation.${key}`)}</dt><dd><ExactAmount value={detail[key]} /></dd></div>)}</dl>
        </div>}
        {(detail.contributions ?? []).length > 0 && <ul className="mt-3 space-y-1 text-sm">{(detail.contributions ?? []).map(item => <li key={item.key} className="flex flex-wrap justify-between gap-x-4"><span>{t(`historicalReconciliation.category.${item.key}`)}</span><ExactAmount value={item.amount} currency={currency} /></li>)}</ul>}
        {(detail.reasons ?? []).length > 0 && <ul className="mt-3 list-disc space-y-1 pl-5 text-sm">{(detail.reasons ?? []).map(reason => <li key={reason}>{t(`historicalReconciliation.reason.${reason}`)}</li>)}</ul>}
        <h4 className="mt-3 text-sm font-medium">{t("historicalReconciliation.records")}</h4>
        {(detail.records ?? []).length === 0 ? <p className="text-xs text-muted-foreground">{t("historicalReconciliation.noRecords")}</p> : <ol className="mt-1 space-y-3 text-sm">{(detail.records ?? []).map((record, index) => <li key={`${record.id}:${index}`} className="min-w-0">
          <p><time dateTime={record.effectiveAt}>{formatTimestamp(record.effectiveAt, timezone)}</time> · {(record.effects ?? []).some(effect => effect.classification === "remeasurement" && effect.quantity != null) ? t("history.detailPositionAdjustment") : t(`history.kind.${record.kind}`, { defaultValue: t("historicalOverview.unknown") })}</p>
          {record.reason && <p className="text-xs text-muted-foreground">{t(`history.reason.${record.reason}`, { defaultValue: t("historicalOverview.unknown") })}</p>}
          {record.note && <p className="break-words text-xs text-muted-foreground">{record.note}</p>}
          <ul className="space-y-1">{(record.effects ?? []).map((effect, effectIndex) => <li key={effectIndex} className="flex flex-wrap justify-between gap-x-4">
            <span>{t(`historicalReconciliation.effect.${effect.classification}`, { defaultValue: t("historicalReconciliation.effect.unknown") })}</span>
            {effect.amount != null && <ExactAmount value={effect.amount} currency={effect.currency} />}
            {effect.quantity != null && <span>{t("historicalReconciliation.quantity")}: <ExactAmount value={effect.quantity} /></span>}
          </li>)}</ul>
        </li>)}</ol>}
      </>}
    </div>
  </div>;
}

export function HistoricalReconciliationDetails({ data }: { data: HistoricalOverviewResult }) {
  const { t } = useTranslation();
  const rows = new Map((data.rows ?? []).map(row => [row.key, row]));
  const accounts = new Map<string, HistoricalReconciliationDetail[]>();
  for (const detail of data.reconciliation?.details ?? []) {
    const group = accounts.get(detail.accountId) ?? [];
    group.push(detail); accounts.set(detail.accountId, group);
  }
  if (!accounts.size) return null;
  const businessName = (key: string) => {
    const row = rows.get(key);
    return row?.name && ![row.key, row.accountId, row.holdingId, row.instrumentId].includes(row.name) ? row.name : null;
  };
  const accountNames = instrumentSelectionLabels([...accounts.keys()].map(id => ({ id, name: businessName(id) })), t("historicalOverview.unknown"));
  return <div className="mt-4 min-w-0">
    <h3 className="text-sm font-semibold">{t("historicalReconciliation.details")}</h3>
    <p className="mt-1 text-xs text-muted-foreground">{t("historicalReconciliation.detailsHint")}</p>
    {[...accounts].map(([accountId, details]) => {
      const names = instrumentSelectionLabels(details.map(detail => ({ id: detail.key, name: businessName(detail.key), quoteCurrency: rows.get(detail.key)?.right?.currency || rows.get(detail.key)?.left?.currency })), t("historicalOverview.unknown"));
      return <section key={accountId} className="mt-3 min-w-0" aria-label={accountNames.get(accountId)}>
        <h4 className="mb-1 break-words text-sm font-medium">{accountNames.get(accountId)}</h4>
        {details.map(detail => <ComponentDetail key={detail.key} detail={detail} name={names.get(detail.key)!} currency={data.reconciliation!.currency} timezone={data.timezone} />)}
      </section>;
    })}
  </div>;
}
