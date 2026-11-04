import { ArrowLeft, ArrowRight, History } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { ActionMenu } from "@/components/ui/action-menu";
import { NAV_GROUPS, type NavigationTarget } from "./navigation";
import { useAccounts } from "@/queries/accounts";
import { useInstruments } from "@/queries/investments";

export interface NavigationHistoryControlsProps {
  canBack: boolean;
  canForward: boolean;
  backTarget?: NavigationTarget;
  forwardTarget?: NavigationTarget;
  back: () => void;
  forward: () => void;
  sessions?: Array<{ id: string; ordinal: number; target: NavigationTarget; onSelect: () => void }>;
  recent: Array<{ id: string; target: NavigationTarget; onSelect: () => void }>;
}

export function NavigationHistoryControls({ canBack, canForward, backTarget, forwardTarget, back, forward, recent, sessions = [] }: NavigationHistoryControlsProps) {
  const { t } = useTranslation();
  const accounts = useAccounts({ includeArchived: true });
  const instruments = useInstruments(true);
  const label = (target: NavigationTarget) => {
    const page = NAV_GROUPS.flatMap(group => group.items).find(item => item.id === target.page);
    const title = page ? t(page.translationKey) : target.page === "instrument-detail" ? t("instrumentDetail.title") : t("nav.dataHealth");
    const context: string[] = [];
    if ("accountId" in target && target.accountId) context.push(accounts.data?.find(record => record.account.id === target.accountId)?.account.name ?? t("navigationHistory.unavailableAccount"));
    if ("instrumentId" in target && target.instrumentId) context.push(instruments.data?.find(record => record.id === target.instrumentId)?.name ?? t("navigationHistory.unavailableInstrument"));
    if (target.page === "tasks" && target.tab) context.push(t(`tasks.tabs.${target.tab}`));
    if (target.page === "market-data" && target.search) context.push(t("navigationHistory.search", { query: target.search.slice(0, 80) }));
    if (target.page === "accounts" && target.filter) context.push(target.filter.label);
    if (target.page === "historical-overview" && target.date) context.push(target.date);
    if (target.page === "portfolio" && target.view) context.push(t(`investmentAssets.${target.view}`));
    const filters = target.page === "history" ? target.filters : target.page === "return-analysis" || target.page === "asset-changes" ? target.analysis : undefined;
    if (filters && "accountId" in filters && filters.accountId) context.push(accounts.data?.find(record => record.account.id === filters.accountId)?.account.name ?? t("navigationHistory.unavailableAccount"));
    if (filters && "instrumentId" in filters && filters.instrumentId) context.push(instruments.data?.find(record => record.id === filters.instrumentId)?.name ?? t("navigationHistory.unavailableInstrument"));
    if (filters && "scopeId" in filters && filters.scopeId) {
      if (filters.scope === "account") context.push(accounts.data?.find(record => record.account.id === filters.scopeId)?.account.name ?? t("navigationHistory.unavailableAccount"));
      if (filters.scope === "instrument") context.push(instruments.data?.find(record => record.id === filters.scopeId)?.name ?? t("navigationHistory.unavailableInstrument"));
    }
    if (filters?.from) context.push(filters.from);
    if (filters?.to) context.push(filters.to);
    if (filters && Object.values(filters).some(value => value !== undefined && value !== "")) context.push(t("navigationHistory.filtered"));
    return [title, ...context].join(" · ");
  };
  const backLabel = canBack && backTarget ? t("navigationHistory.backTo", { destination: label(backTarget) }) : t("navigationHistory.back");
  const forwardLabel = canForward && forwardTarget ? t("navigationHistory.forwardTo", { destination: label(forwardTarget) }) : t("navigationHistory.forward");
  return <div className="flex shrink-0 items-center gap-0.5" role="group" aria-label={t("navigationHistory.label")}>
    <Button variant="ghost" size="icon" disabled={!canBack} onClick={back} aria-label={backLabel} title={backLabel}><ArrowLeft aria-hidden="true" className="size-4" /></Button>
    <Button variant="ghost" size="icon" disabled={!canForward} onClick={forward} aria-label={forwardLabel} title={forwardLabel}><ArrowRight aria-hidden="true" className="size-4" /></Button>
    <ActionMenu label={t("navigationHistory.recent")} icon={History} disabled={recent.length === 0 && sessions.length === 0} sections={[{ label: t("navigationHistory.retainedSessions"), items: sessions.map(entry => ({ id: entry.id, label: `${label(entry.target)} · ${t("navigationHistory.workspace", { number: entry.ordinal })}`, onSelect: entry.onSelect })) }]} items={recent.map(entry => ({ id: entry.id, label: label(entry.target), onSelect: entry.onSelect }))} />
  </div>;
}
