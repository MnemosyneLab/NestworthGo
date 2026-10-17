import { Fragment, useId, useState } from "react";
import { useTranslation } from "react-i18next";
import { ArrowDown, ArrowUp, ArrowUpRight, ChevronsDownUp, ChevronsUpDown } from "lucide-react";
import { useObjectNavigation } from "@/app/NavigationContext";
import { AnalyzeMenu } from "@/features/insights/AnalyzeMenu";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ChangeText } from "@/components/ui/change-text";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { instrumentDisplayLabel } from "@/lib/instrumentDisplay";
import { displayEnum } from "@/lib/display";
import { compareCanonical, formatAmount, formatSignedMoney } from "@/lib/money";
import { metalUnitLabel } from "@/lib/preciousMetals";
import { cn } from "@/lib/utils";
import { useHoldingGroupStore, type HoldingGroupScope } from "@/stores/holdings";
import { HoldingDisclosure } from "./HoldingDisclosure";
import { HoldingGroupHeader } from "./HoldingGroupHeader";
import { HoldingDetailPanel } from "./LazyHoldingDetailPanel";
import { amountDirection, visibleHoldingGroups, type HoldingQuantityFilter, type HoldingRow, type HoldingSort, type HoldingSortKey, type HoldingView } from "./holdingGroups";
import type { HoldingAmountsDTO, InstrumentHoldingGroupDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics/models";

type Props = {
  /** "portfolio" lists every account; "account" lists one account's holdings. */
  scope: HoldingGroupScope;
  groups: readonly InstrumentHoldingGroupDTO[];
  active?: boolean;
  /** Portfolio: opens the account behind a holding. */
  onOpenAccount?: (id: string) => void;
  /** Account scope: the account whose holdings are shown. */
  accountId?: string;
  accountName?: string;
  /** Account scope: records a dividend for one holding; omit when the account is read-only. */
  onDividend?: (holdingId: string) => void;
};

const NUMERIC: HoldingSortKey[] = ["quantity", "totalCost", "currentValue", "unrealizedGain"];

/**
 * Shared holdings list for Portfolio and Account detail. Instruments are
 * grouped by type in collapsible sections with per-currency subtotals; every
 * figure is read from the backend response and only formatted here.
 */
export function HoldingGroupsList({ scope, groups, active = true, onOpenAccount, accountId, accountName, onDividend }: Props) {
  const { t, i18n } = useTranslation();
  const navigation = useObjectNavigation();
  const idPrefix = useId();
  const portfolio = scope === "portfolio";
  const [view, setView] = useState<HoldingView>("instrument");
  const [showZero, setShowZero] = useState(false);
  const [accountPositions, setAccountPositions] = useState<HoldingQuantityFilter>("current");
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [sort, setSort] = useState<HoldingSort>({ key: "name", desc: false });
  const collapsed = useHoldingGroupStore((state) => state.collapsed[scope]);
  const toggleGroup = useHoldingGroupStore((state) => state.toggleGroup);
  const setCollapsed = useHoldingGroupStore((state) => state.setCollapsed);

  const effectiveView: HoldingView = portfolio ? view : "account";
  const visible = visibleHoldingGroups(groups, { view: effectiveView, showZero: portfolio && showZero, quantityFilter: portfolio ? undefined : accountPositions, sort, language: i18n.language })
    // Contract-valued holdings have their own page; the account list shows market holdings only.
    .filter((group) => portfolio || group.key !== "cash_like");
  const typeLabel = (key: string) => t(`holdingGroups.types.${key}`);
  const allCollapsed = visible.length > 0 && visible.every((group) => collapsed.includes(group.key));

  const money = (amounts: HoldingAmountsDTO, key: "totalCost" | "currentValue" | "unrealizedGain") => {
    const value = amounts[key];
    const reason = key === "totalCost" ? amounts.costMissingReason : key === "currentValue" ? amounts.valueMissingReason : amounts.gainMissingReason;
    if (!value) return <span className="text-muted-foreground">{t("accounts.noValue")}{reason && <span className="block text-xs">{displayEnum(t, "portfolio.missingReason", reason)}</span>}</span>;
    if (key === "unrealizedGain") return <ChangeText direction={amountDirection(value.amount) ?? "flat"}>{formatSignedMoney(value.amount, value.currency)}</ChangeText>;
    return <span>{formatAmount(value.amount, value.currency)}</span>;
  };
  const quantity = (row: HoldingRow, contract: boolean) => contract
    ? t("availableFunds.contractQuantity", { quantity: formatAmount(row.amounts.quantity) })
    : `${formatAmount(row.amounts.quantity)} ${metalUnitLabel(row.instrument.quantityUnit, t) || row.instrument.quantityUnit}`;
  const toggleRow = (id: string) => setExpanded((previous) => { const next = new Set(previous); if (next.has(id)) next.delete(id); else next.add(id); return next; });

  const cells = (row: HoldingRow, contract: boolean, child = false) => {
    const label = instrumentDisplayLabel(row.instrument);
    const accountCount = new Set((row.instrument.holdings ?? []).map((member) => member.accountId)).size;
    return <>
      <td className={cn("px-3 py-3 align-top", child && "pl-12")}>
        {child ? <span className="flex min-w-0 flex-col">
          <Button type="button" variant="link" size="sm" className="h-auto w-fit max-w-full justify-start truncate whitespace-normal p-0 text-left" onClick={() => onOpenAccount?.(row.accountId!)}>{row.accountName}</Button>
        </span> : <>
          <div className="flex items-center gap-2">
            <HoldingDisclosure name={row.instrument.name} shortName={row.instrument.shortName} symbol={row.instrument.symbol}
              expanded={expanded.has(row.id)} controls={`${idPrefix}-details-${effectiveView}-${row.id}`} onToggle={() => toggleRow(row.id)} />
            {row.instrument.archived && <Badge variant="secondary">{t("common.archived")}</Badge>}
          </div>
          {portfolio && <p className="mt-0.5 pl-11 text-xs text-muted-foreground">
            {row.accountId
              ? <Button type="button" variant="link" size="sm" className="h-auto justify-start p-0 text-left" onClick={() => onOpenAccount?.(row.accountId!)}>{row.accountName}</Button>
              : t("holdingsSummary.accountCount", { count: accountCount })}
          </p>}
        </>}
      </td>
      <td className="num whitespace-nowrap px-3 py-3 text-right align-top text-muted-foreground">{quantity(row, contract)}</td>
      <td className="num whitespace-nowrap px-3 py-3 text-right align-top">{money(row.amounts, "totalCost")}</td>
      <td className="num whitespace-nowrap px-3 py-3 text-right align-top font-medium">{money(row.amounts, "currentValue")}</td>
      <td className="num whitespace-nowrap px-3 py-3 text-right align-top font-medium">{money(row.amounts, "unrealizedGain")}</td>
      <td className="px-2 py-2.5 text-right align-top">
        <span className="inline-flex items-center gap-1">
          {navigation && <Button type="button" size="icon" variant="ghost"
            aria-label={t("investmentAssets.openInstrument", { name: label })}
            onClick={() => navigation.open({ page: "instrument-detail", instrumentId: row.instrument.instrumentId, accountId: row.accountId ?? accountId, section: "holdings" })}>
            <ArrowUpRight aria-hidden="true" />
          </Button>}
          <AnalyzeMenu instrumentId={row.instrument.instrumentId} accountId={row.accountId ?? accountId} label={label}
            onDividend={onDividend && row.holdingId && !row.instrument.archived && compareCanonical(row.amounts.quantity, "0") !== 0 ? () => onDividend(row.holdingId!) : undefined} />
        </span>
      </td>
    </>;
  };

  const columns: { key: HoldingSortKey; label: string }[] = [
    { key: "name", label: t("history.instrument") }, { key: "quantity", label: t("portfolio.quantity") },
    { key: "totalCost", label: t("portfolio.cost") }, { key: "currentValue", label: t("portfolio.currentValue") },
    { key: "unrealizedGain", label: t("portfolio.unrealizedGain") },
  ];

  return <div className="flex min-w-0 flex-col gap-4">
    {!portfolio && <SegmentedControl
      label={t("investmentAssets.accountPositions")}
      value={accountPositions}
      onChange={value => setAccountPositions(value as HoldingQuantityFilter)}
      options={(["current", "closed", "all"] as const).map(choice => ({ value: choice, label: t(`investmentAssets.positions.${choice}`) }))}
    />}
    {!portfolio && accountPositions === "all" && <p className="text-xs text-muted-foreground">{t("investmentAssets.positions.allSubtotalHelp")}</p>}
    {portfolio && <div className="flex flex-wrap items-center justify-between gap-3">
      <div role="group" aria-label={t("holdingsSummary.view")} className="flex gap-2">
        <Button size="sm" variant={view === "instrument" ? "default" : "outline"} aria-pressed={view === "instrument"} onClick={() => setView("instrument")}>{t("holdingsSummary.byInstrument")}</Button>
        <Button size="sm" variant={view === "account" ? "default" : "outline"} aria-pressed={view === "account"} onClick={() => setView("account")}>{t("holdingsSummary.byAccount")}</Button>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" className="size-4 rounded accent-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50" checked={showZero} onChange={(event) => setShowZero(event.target.checked)} />{t("holdingsSummary.showZero")}</label>
        {visible.length > 1 && <Button size="sm" variant="ghost" onClick={() => setCollapsed(scope, allCollapsed ? [] : visible.map((group) => group.key))}>
          {allCollapsed ? <ChevronsUpDown aria-hidden="true" /> : <ChevronsDownUp aria-hidden="true" />}
          {t(allCollapsed ? "holdingGroups.expandAll" : "holdingGroups.collapseAll")}
        </Button>}
      </div>
    </div>}
    {visible.length === 0 ? <p className="py-8 text-center text-sm text-muted-foreground">{t("holdingsSummary.noVisibleHoldings")}</p> : <div role="list" aria-label={t("holdingGroups.label")} className="flex flex-col gap-3">
      {visible.map((group) => {
        const open = !collapsed.includes(group.key);
        const bodyId = `${idPrefix}-group-${group.key}`;
        const contract = group.key === "cash_like";
        return <section key={group.key} role="listitem" aria-label={typeLabel(group.key)} data-group={group.key} className="min-w-0 overflow-hidden rounded-2xl border border-border bg-card shadow-sm">
          <HoldingGroupHeader groupKey={group.key} label={typeLabel(group.key)} count={group.instrumentCount} subtotals={group.subtotals} expanded={open} controls={bodyId} onToggle={() => toggleGroup(scope, group.key)} />
          <div id={bodyId} hidden={!open}>
            {open && <div className="overflow-x-auto border-t border-border">
              <table className="w-full min-w-[46rem] table-fixed text-left text-sm" data-testid="holdings-table">
                <caption className="sr-only">{t("portfolio.holdingsTableLabel")} · {typeLabel(group.key)}</caption>
                <colgroup><col className="w-[32%]" /><col className="w-[13%]" /><col className="w-[16%]" /><col className="w-[16%]" /><col className="w-[16%]" /><col className="w-[7%]" /></colgroup>
                <thead><tr className="text-xs text-muted-foreground">{columns.map((column) => <th key={column.key} scope="col" className={cn("px-3 py-2 font-medium", NUMERIC.includes(column.key) && "text-right")} aria-sort={sort.key === column.key ? sort.desc ? "descending" : "ascending" : "none"}>
                  <button type="button" className="inline-flex items-center gap-1 rounded-md hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50" onClick={() => setSort((previous) => ({ key: column.key, desc: previous.key === column.key ? !previous.desc : false }))}>
                    {column.label}{sort.key === column.key && (sort.desc ? <ArrowDown className="size-3" aria-hidden="true" /> : <ArrowUp className="size-3" aria-hidden="true" />)}
                  </button>
                </th>)}<th scope="col" className="px-2 py-2"><span className="sr-only">{t("connections.actions")}</span></th></tr></thead>
                <tbody>{group.rows.map((row) => <Fragment key={row.id}>
                  <tr className="border-t border-border transition-colors hover:bg-muted/40">{cells(row, contract)}</tr>
                  {active && expanded.has(row.id) && <tr id={`${idPrefix}-details-${effectiveView}-${row.id}`}><td colSpan={6} className="px-3 pb-3"><HoldingDetailPanel instrumentId={row.instrument.instrumentId} scope={row.accountName ?? accountName ?? t("holdingDetail.allAccounts")} /></td></tr>}
                  {effectiveView === "instrument" && expanded.has(row.id) && (row.instrument.holdings ?? []).map((member) => <tr key={member.holdingId} className="border-t border-border bg-muted/20">{cells({ id: `holding:${member.holdingId}`, instrument: row.instrument, amounts: member.amounts, accountId: member.accountId, accountName: member.accountName, holdingId: member.holdingId }, contract, true)}</tr>)}
                </Fragment>)}</tbody>
              </table>
            </div>}
          </div>
        </section>;
      })}
    </div>}
  </div>;
}
