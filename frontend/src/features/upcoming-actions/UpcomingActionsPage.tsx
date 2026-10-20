import { InvestmentReviewPage } from "@/features/investment-review/InvestmentReviewPage";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { PageChrome } from "@/components/layout/PageChrome";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Notice } from "@/components/ui/notice";
import { LoadingState, ErrorState, EmptyState } from "@/components/layout/PageState";
import { cn } from "@/lib/utils";
import { useUpcomingActions } from "@/queries/liquidity";
import { useObjectNavigation } from "@/app/NavigationContext";
import { UpcomingActionList } from "./UpcomingActionList";
import { upcomingNavigation, upcomingButton, upcomingCountLabels, upcomingReadLabel } from "./upcomingActionPresentation";
import type { UpcomingActionsTab } from "@/app/navigation";

const sectionClass = "min-w-0 overflow-hidden rounded-2xl border border-border bg-card shadow-sm";
const sectionHeadingClass = "border-b border-border bg-muted/30 px-5 py-3.5 text-base font-semibold";
const detailsClass = "rounded-2xl border border-border bg-card shadow-sm";
const summaryClass = "cursor-pointer select-none rounded-2xl px-5 py-3.5 text-sm font-medium text-foreground marker:text-muted-foreground hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50";

export function UpcomingActionsPage({ isActive = true, tab: controlledTab, onTabChange }: { isActive?: boolean; tab?: UpcomingActionsTab; onTabChange?: (tab: UpcomingActionsTab) => void }) {
  const { t, i18n } = useTranslation();
  const [localTab, setLocalTab] = useState<UpcomingActionsTab>("recent");
  const tab = controlledTab ?? localTab;
  const setTab = (value: string) => {
    const next = value === "review" ? "review" : "recent";
    setLocalTab(next);
    onTabChange?.(next);
  };
  const query = useUpcomingActions(isActive && tab === "recent");
  const navigation = useObjectNavigation();
  const [filter, setFilter] = useState("all");
  const [limit, setLimit] = useState(50);
  const [eligibleOpen, setEligibleOpen] = useState(false);
  const [checksOpen, setChecksOpen] = useState(false);
  const data = query.data;
  const items = (data?.items ?? []).filter(item => {
    if (item.group === "eligible") return false;
    if (filter === "all") return true;
    if (filter === "receipt") return item.phase === "receipt_due";
    if (filter === "reservations") return item.kind.startsWith("reservation_");
    return item.group === "scheduled";
  });
  const attentionItems = items.filter(item => item.group === "attention");
  const scheduledItems = items.filter(item => item.group === "scheduled");
  const hasMore = filter === "all" ? attentionItems.length > limit || scheduledItems.length > limit : items.length > limit;
  return <div className="flex min-w-0 flex-col gap-6" aria-busy={query.isFetching}>
    <PageChrome pageId="upcoming-actions" title={t("upcomingActions.title")} actions={tab === "recent" ? <Button variant="outline" disabled={query.isFetching} onClick={() => { void query.refetch(); }}>{t("upcomingActions.refresh")}</Button> : undefined} />
    <div className="flex flex-col gap-1.5">
      <h2 id="upcoming-actions-heading" tabIndex={-1} className="text-xl font-semibold tracking-tight">{t("upcomingActions.title")}</h2>
      <p className="max-w-3xl text-sm leading-6 text-muted-foreground">{t("upcomingActions.description")}</p>
    </div>
    <Tabs value={tab} onValueChange={value => setTab(String(value))}>
      <TabsList aria-label={t("upcomingActions.description")}>
        <TabsTrigger value="recent">{t("upcomingActions.title")}</TabsTrigger>
        <TabsTrigger value="review">{t("reviewV2.title")}</TabsTrigger>
      </TabsList>
      <TabsContent value="review" keepMounted><InvestmentReviewPage embedded isActive={isActive && tab === "review"} /></TabsContent>
      <TabsContent value="recent" keepMounted className="flex flex-col gap-5">
    {!data && query.isPending && <LoadingState label={t("ui.state.loadingPage")} variant="list" />}
    {query.isError && !data && <ErrorState title={t("upcomingActions.loadError")} description={t("ui.state.errorDescription")} retryLabel={t("common.retry")} onRetry={() => query.refetch()} />}
    {query.isError && data && <Notice tone="warning" role="alert">{t("upcomingActions.stale")}</Notice>}
    {data && <>
      <Card className="flex flex-col gap-3 p-5">
        <p aria-live="polite" className="num text-base font-semibold">{t("upcomingActions.counts", upcomingCountLabels(t, data.counts))}</p>
        <div className="flex flex-col gap-0.5 text-xs text-muted-foreground">
          <p className="num">{t("upcomingActions.window", { date: data.localDate, timezone: data.timezone })}</p>
          <p className="num">{upcomingReadLabel(t, data.asOf, data.timezone, i18n.language)}</p>
        </div>
        {data.completeness === "partial" && <Notice tone="warning">{t("upcomingActions.partial")}</Notice>}
      </Card>
      <div role="group" aria-label={t("upcomingActions.filter")} className="flex flex-wrap gap-2">
        {["all", "receipt", "reservations", "scheduled"].map(value => <Button key={value} size="sm" variant={filter === value ? "default" : "outline"} aria-pressed={filter === value} onClick={() => { setFilter(value); setLimit(50); }}>{t(`upcomingActions.filters.${value}`)}</Button>)}
      </div>
      {filter === "all" ? <>
        <section aria-label={t("upcomingActions.attentionTitle")} className={sectionClass}><h3 className={sectionHeadingClass}>{t("upcomingActions.attentionTitle")} · {data.counts.attention}</h3><div className="px-5"><UpcomingActionList context="upcoming-attention" items={attentionItems.slice(0, limit)} /></div></section>
        <section aria-label={t("upcomingActions.scheduledTitle")} className={sectionClass}><h3 className={sectionHeadingClass}>{t("upcomingActions.scheduledTitle")} · {data.counts.scheduled}</h3><div className="px-5"><UpcomingActionList context="upcoming-scheduled" items={scheduledItems.slice(0, limit)} /></div></section>
      </> : items.length > 0 && <div className={cn(sectionClass, "px-5")}><UpcomingActionList context="upcoming-list" items={items.slice(0, limit)} /></div>}
      {items.length === 0 && <EmptyState title={t("upcomingActions.empty")} variant="success" />}
      {hasMore && <Button variant="outline" className="self-center" onClick={() => setLimit(value => value + 50)}>{t("upcomingActions.showMore")}</Button>}
      <details className={detailsClass} onToggle={event => setEligibleOpen(event.currentTarget.open)}><summary className={summaryClass}>{t("upcomingActions.eligibleTitle", { count: data.counts.eligible })}</summary>{eligibleOpen && <div className="border-t border-border px-5"><UpcomingActionList context="upcoming-eligible" items={(data.items ?? []).filter(item => item.group === "eligible")} /></div>}</details>
      <details className={detailsClass} onToggle={event => setChecksOpen(event.currentTarget.open)}><summary className={summaryClass}>{t("upcomingActions.checksTitle", upcomingCountLabels(t, data.counts))}</summary>
        {checksOpen && <div className="flex flex-col gap-3 border-t border-border px-5 py-4">
        <ul className="divide-y divide-border">{(data.dataChecks ?? []).map(check => {
          const targets = [check.target, ...(check.targets ?? [])].filter((target, index, all) => all.findIndex(other => JSON.stringify(other) === JSON.stringify(target)) === index);
          return <li key={check.id} className="flex flex-col gap-1.5 break-words py-3 first:pt-0 last:pb-0"><p className="num font-semibold">{check.displayName}{check.anchorOn && <> · {check.anchorOn}</>}</p>{check.referenceOn && <p className="num text-xs text-muted-foreground">{t("upcomingActions.reference", { date: check.referenceOn })}</p>}{(check.reasonCodes ?? []).map(reason => <Notice key={reason} tone="warning" className="py-1.5 text-xs leading-5">{t(`upcomingActions.reasons.${reason}`, { defaultValue: t("upcomingActions.reasons.unsupported_source") })}</Notice>)}{navigation && <div className="flex flex-wrap gap-2">{targets.map((target, index) => { const destination = upcomingNavigation(target); return destination && <Button key={`${target.kind}-${index}`} variant="outline" size="sm" onClick={() => navigation.open(destination)}>{upcomingButton(t, target)}</Button>; })}</div>}</li>;
        })}</ul>
        {navigation && <Button variant="link" className="self-start px-0" onClick={() => navigation.open({ page: "data-health" })}>{t("nav.dataHealth")}</Button>}
        </div>}
      </details>
      <Notice>{t("upcomingActions.normalOnly")}</Notice>
    </>}
      </TabsContent>
    </Tabs>
  </div>;
}
