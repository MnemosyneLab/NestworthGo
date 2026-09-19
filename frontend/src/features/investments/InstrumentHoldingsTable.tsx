import { Fragment, useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronDown, ChevronRight, ArrowDown, ArrowUp } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { InstrumentLabel } from "@/components/forms/InstrumentLabel";
import { compareCanonical, formatAmount } from "@/lib/money";
import { displayEnum } from "@/lib/display";
import { metalUnitLabel } from "@/lib/preciousMetals";
import type { HoldingAmountsDTO, InstrumentHoldingsDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics/models";

type SortKey = "name" | "account" | "quantity" | "totalCost" | "currentValue" | "unrealizedGain";
type Row = { id: string; group: InstrumentHoldingsDTO; amounts: HoldingAmountsDTO; accountId?: string; accountName?: string };

export function InstrumentHoldingsTable({ groups, onOpenAccount }: { groups: InstrumentHoldingsDTO[]; onOpenAccount?: (id: string) => void }) {
  const { t, i18n } = useTranslation();
  const [view, setView] = useState("instrument");
  const [showZero, setShowZero] = useState(false);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [sort, setSort] = useState<{ key: SortKey; desc: boolean }>({ key: "name", desc: false });
  const visible = groups.map(group => ({ ...group, holdings: (group.holdings ?? []).filter(member => showZero || compareCanonical(member.amounts.quantity, "0") !== 0) })).filter(group => group.holdings.length > 0);
  const rows: Row[] = view === "instrument"
    ? visible.map(group => ({ id: group.instrumentId, group, amounts: group.amounts }))
    : visible.flatMap(group => group.holdings.map(member => ({ id: member.holdingId, group, amounts: member.amounts, accountId: member.accountId, accountName: member.accountName })));
  rows.sort((a, b) => {
    let cmp = 0;
    if (sort.key === "name") cmp = a.group.name.localeCompare(b.group.name, i18n.language);
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
    return (sort.desc ? -cmp : cmp) || a.group.name.localeCompare(b.group.name, i18n.language) || a.id.localeCompare(b.id);
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
        {view === "instrument" && !child && <button type="button" aria-label={t("holdingsSummary.toggleAccounts", { name: row.group.name })} aria-expanded={expanded.has(row.id)} onClick={() => setExpanded(previous => { const next = new Set(previous); if (next.has(row.id)) next.delete(row.id); else next.add(row.id); return next; })} className="rounded p-1 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary">
          {expanded.has(row.id) ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
        </button>}
        <InstrumentLabel name={row.group.name} symbol={row.group.symbol} fallback={row.group.name} />
        {row.group.archived && <Badge variant="secondary">{t("common.archived")}</Badge>}
      </div>
    </td>
    <td className="px-3 py-3">{row.accountId ? <button type="button" className="text-primary underline-offset-4 hover:underline focus-visible:underline" onClick={() => onOpenAccount?.(row.accountId!)}>{row.accountName}</button> : t("holdingsSummary.accountCount", { count: new Set((row.group.holdings ?? []).map(member => member.accountId)).size })}</td>
    <td className="px-3 py-3 whitespace-nowrap">{formatAmount(row.amounts.quantity)} {metalUnitLabel(row.group.quantityUnit, t) || row.group.quantityUnit}</td>
    <td className="px-3 py-3 whitespace-nowrap">{money(row.amounts, "totalCost")}</td>
    <td className="px-3 py-3 whitespace-nowrap">{money(row.amounts, "currentValue")}</td>
    <td className="px-3 py-3 whitespace-nowrap">{money(row.amounts, "unrealizedGain")}</td>
  </>;
  const columns: { key: SortKey; label: string }[] = [
    { key: "name", label: t("history.instrument") }, { key: "account", label: t("history.accountSelect") },
    { key: "quantity", label: t("portfolio.quantity") }, { key: "totalCost", label: t("portfolio.cost") },
    { key: "currentValue", label: t("portfolio.currentValue") }, { key: "unrealizedGain", label: t("portfolio.unrealizedGain") },
  ];
  return <div className="flex flex-col gap-3">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div role="group" aria-label={t("holdingsSummary.view")} className="flex gap-2">
        <Button size="sm" variant={view === "instrument" ? "default" : "outline"} aria-pressed={view === "instrument"} onClick={() => setView("instrument")}>{t("holdingsSummary.byInstrument")}</Button>
        <Button size="sm" variant={view === "account" ? "default" : "outline"} aria-pressed={view === "account"} onClick={() => setView("account")}>{t("holdingsSummary.byAccount")}</Button>
      </div>
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={showZero} onChange={e => setShowZero(e.target.checked)} />{t("holdingsSummary.showZero")}</label>
    </div>
    {rows.length === 0 ? <p className="py-8 text-center text-sm text-muted-foreground">{t("holdingsSummary.noVisibleHoldings")}</p> : <div className="overflow-x-auto rounded-lg border border-border">
      <table className="w-full min-w-[48rem] text-left text-sm" data-testid="holdings-table">
        <caption className="sr-only">{t("portfolio.holdingsTableLabel")}</caption>
        <thead><tr className="border-b border-border bg-muted/40">{columns.map(column => <th key={column.key} className="px-3 py-2 font-medium text-muted-foreground" aria-sort={sort.key === column.key ? sort.desc ? "descending" : "ascending" : "none"}>
          {column.key === "account" && view === "instrument" ? column.label : <button type="button" className="inline-flex items-center gap-1" onClick={() => setSort(previous => ({ key: column.key, desc: previous.key === column.key ? !previous.desc : false }))}>{column.label}{sort.key === column.key && (sort.desc ? <ArrowDown className="size-3" /> : <ArrowUp className="size-3" />)}</button>}
        </th>)}</tr></thead>
        <tbody>{rows.map(row => <Fragment key={row.id}>
          <tr className="border-b border-border last:border-0">{cells(row)}</tr>
          {view === "instrument" && expanded.has(row.id) && (row.group.holdings ?? []).map(member => <tr key={member.holdingId} className="border-b border-border bg-muted/20 last:border-0">{cells({ id: member.holdingId, group: row.group, amounts: member.amounts, accountId: member.accountId, accountName: member.accountName }, true)}</tr>)}
        </Fragment>)}</tbody>
      </table>
    </div>}
  </div>;
}
