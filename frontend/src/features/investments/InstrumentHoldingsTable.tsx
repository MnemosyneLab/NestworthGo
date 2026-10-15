import { HoldingDetailPanel } from "./LazyHoldingDetailPanel";
import { AnalyzeMenu } from "@/features/insights/AnalyzeMenu";
import { Fragment, useState } from "react";
import { useTranslation } from "react-i18next";
import { ArrowDown, ArrowUp } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { instrumentDisplayLabel } from "@/lib/instrumentDisplay";
import { InstrumentLabel } from "@/components/forms/InstrumentLabel";
import { HoldingDisclosure } from "./HoldingDisclosure";
import { compareCanonical, formatAmount } from "@/lib/money";
import { displayEnum } from "@/lib/display";
import { metalUnitLabel } from "@/lib/preciousMetals";
import type { HoldingAmountsDTO, InstrumentHoldingsDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics/models";

type SortKey = "name" | "account" | "quantity" | "totalCost" | "currentValue" | "unrealizedGain";
type Row = { id: string; group: InstrumentHoldingsDTO; amounts: HoldingAmountsDTO; accountId?: string; accountName?: string };

export function InstrumentHoldingsTable({ groups, onOpenAccount, managedInstrumentIds, active = true }: { groups: InstrumentHoldingsDTO[]; active?: boolean; onOpenAccount?: (id: string) => void; managedInstrumentIds?: ReadonlySet<string> }) {
  const { t, i18n } = useTranslation();
  const [view, setView] = useState("instrument");
  const [showZero, setShowZero] = useState(false);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [sort, setSort] = useState<{ key: SortKey; desc: boolean }>({ key: "name", desc: false });
  const visible = groups.map(group => ({ ...group, holdings: (group.holdings ?? []).filter(member => showZero || compareCanonical(member.amounts.quantity, "0") !== 0) })).filter(group => group.holdings.length > 0);
  const rows: Row[] = view === "instrument"
    ? visible.map(group => ({ id: `instrument:${group.instrumentId}`, group, amounts: group.amounts }))
    : visible.flatMap(group => group.holdings.map(member => ({ id: `holding:${member.holdingId}`, group, amounts: member.amounts, accountId: member.accountId, accountName: member.accountName })));
  rows.sort((a, b) => {
    let cmp = 0;
    if (sort.key === "name") cmp = instrumentDisplayLabel(a.group).localeCompare(instrumentDisplayLabel(b.group), i18n.language);
    else if (sort.key === "account") cmp = (a.accountName ?? "").localeCompare(b.accountName ?? "", i18n.language);
    else if (sort.key === "quantity") {
      const unit = a.group.quantityUnit.localeCompare(b.group.quantityUnit);
      if (unit) return unit;
      cmp = compareCanonical(a.amounts.quantity, b.amounts.quantity);
    } else {
      const currency = a.group.quoteCurrency.localeCompare(b.group.quoteCurrency);
      if (currency) return currency;
      const left = a.amounts[sort.key]?.amount, right = b.amounts[sort.key]?.amount;
      if (left == null || right == null) {
        if (left == null && right != null) return 1;
        if (right == null && left != null) return -1;
      } else cmp = compareCanonical(left, right);
    }
    return (sort.desc ? -cmp : cmp) || instrumentDisplayLabel(a.group).localeCompare(instrumentDisplayLabel(b.group), i18n.language) || a.group.name.localeCompare(b.group.name, i18n.language) || a.id.localeCompare(b.id);
  });
  const money = (a: HoldingAmountsDTO, key: "totalCost" | "currentValue" | "unrealizedGain") => {
    const value = a[key];
    const reason = key === "totalCost" ? a.costMissingReason : key === "currentValue" ? a.valueMissingReason : a.gainMissingReason;
    if (!value) return <span className="text-muted-foreground">{t("accounts.noValue")}{reason && <span className="block text-xs">{displayEnum(t, "portfolio.missingReason", reason)}</span>}</span>;
    return <span className={key === "unrealizedGain" ? value.amount.startsWith("-") ? "text-gain-negative" : "text-gain-positive" : undefined}>{formatAmount(value.amount, value.currency)}</span>;
  };
  const cells = (row: Row, child = false) => <>
    <td className={`px-3 py-3 ${child ? "pl-10" : ""}`}>
      <div className="flex items-center gap-2">
        {child ? <InstrumentLabel shortName={row.group.shortName} name={row.group.name} symbol={row.group.symbol} fallback={row.group.name} className="flex-1" /> :
          <HoldingDisclosure name={row.group.name} shortName={row.group.shortName} symbol={row.group.symbol}
            expanded={expanded.has(row.id)} controls={`holding-details-${view}-${row.id}`}
            onToggle={() => setExpanded(previous => { const next = new Set(previous); if (next.has(row.id)) next.delete(row.id); else next.add(row.id); return next; })} />}
        <AnalyzeMenu instrumentId={row.group.instrumentId} accountId={row.accountId} label={instrumentDisplayLabel(row.group)} />
        {row.group.archived && <Badge variant="secondary">{t("common.archived")}</Badge>}
      </div>
    </td>
    <td className="px-3 py-3">{row.accountId ? <button type="button" className="text-primary underline-offset-4 hover:underline focus-visible:underline" onClick={() => onOpenAccount?.(row.accountId!)}>{row.accountName}</button> : t("holdingsSummary.accountCount", { count: new Set((row.group.holdings ?? []).map(member => member.accountId)).size })}</td>
    <td className="num px-3 py-3 text-right whitespace-nowrap">{managedInstrumentIds?.has(row.group.instrumentId) ? t("availableFunds.contractQuantity", { quantity: formatAmount(row.amounts.quantity) }) : `${formatAmount(row.amounts.quantity)} ${metalUnitLabel(row.group.quantityUnit, t) || row.group.quantityUnit}`}</td>
    <td className="num px-3 py-3 text-right whitespace-nowrap">{money(row.amounts, "totalCost")}</td>
    <td className="num px-3 py-3 text-right whitespace-nowrap">{money(row.amounts, "currentValue")}</td>
    <td className="num px-3 py-3 text-right whitespace-nowrap">{money(row.amounts, "unrealizedGain")}</td>
  </>;
  const columns: { key: SortKey; label: string }[] = [
    { key: "name", label: t("history.instrument") }, { key: "account", label: t("history.accountSelect") },
    { key: "quantity", label: t("portfolio.quantity") }, { key: "totalCost", label: t("portfolio.cost") },
    { key: "currentValue", label: t("portfolio.currentValue") }, { key: "unrealizedGain", label: t("portfolio.unrealizedGain") },
  ];
  return <div className="flex min-w-0 flex-col gap-3">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div role="group" aria-label={t("holdingsSummary.view")} className="flex gap-2">
        <Button size="sm" variant={view === "instrument" ? "default" : "outline"} aria-pressed={view === "instrument"} onClick={() => setView("instrument")}>{t("holdingsSummary.byInstrument")}</Button>
        <Button size="sm" variant={view === "account" ? "default" : "outline"} aria-pressed={view === "account"} onClick={() => setView("account")}>{t("holdingsSummary.byAccount")}</Button>
      </div>
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" className="size-4 rounded accent-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50" checked={showZero} onChange={e => setShowZero(e.target.checked)} />{t("holdingsSummary.showZero")}</label>
    </div>
    {rows.length === 0 ? <p className="py-8 text-center text-sm text-muted-foreground">{t("holdingsSummary.noVisibleHoldings")}</p> : <div className="overflow-x-auto rounded-2xl border border-border">
      <table className="w-full min-w-[48rem] text-left text-sm" data-testid="holdings-table">
        <caption className="sr-only">{t("portfolio.holdingsTableLabel")}</caption>
        <thead><tr className="border-b border-border bg-muted/50">{columns.map(column => <th key={column.key} className={`px-3 py-2.5 font-semibold text-muted-foreground ${["quantity", "totalCost", "currentValue", "unrealizedGain"].includes(column.key) ? "text-right" : ""}`} aria-sort={sort.key === column.key ? sort.desc ? "descending" : "ascending" : "none"}>
          {column.key === "account" && view === "instrument" ? column.label : <button type="button" className="inline-flex items-center gap-1 rounded-md hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50" onClick={() => setSort(previous => ({ key: column.key, desc: previous.key === column.key ? !previous.desc : false }))}>{column.label}{sort.key === column.key && (sort.desc ? <ArrowDown className="size-3" /> : <ArrowUp className="size-3" />)}</button>}
        </th>)}</tr></thead>
        <tbody>{rows.map(row => <Fragment key={row.id}>
          <tr className="border-b border-border transition-colors last:border-0 hover:bg-muted/40">{cells(row)}</tr>
          {active && expanded.has(row.id) && <tr id={`holding-details-${view}-${row.id}`}><td colSpan={6}><HoldingDetailPanel instrumentId={row.group.instrumentId} amounts={row.amounts} scope={row.accountName ?? t("holdingDetail.allAccounts")} /></td></tr>}
          {view === "instrument" && expanded.has(row.id) && (row.group.holdings ?? []).map(member => <tr key={member.holdingId} className="border-b border-border bg-muted/20 last:border-0">{cells({ id: `holding:${member.holdingId}`, group: row.group, amounts: member.amounts, accountId: member.accountId, accountName: member.accountName }, true)}</tr>)}
        </Fragment>)}</tbody>
      </table>
    </div>}
  </div>;
}
