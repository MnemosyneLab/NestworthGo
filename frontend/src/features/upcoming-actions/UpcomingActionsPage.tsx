import { createPortal } from "react-dom";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Notice } from "@/components/ui/notice";
import { LoadingState, ErrorState, EmptyState } from "@/components/layout/PageState";
import { cn } from "@/lib/utils";
import { useUpcomingActions } from "@/queries/liquidity";
import { useObjectNavigation } from "@/app/NavigationContext";
import { UpcomingActionList } from "./UpcomingActionList";
import { formatCivilDate } from "@/lib/time";
import { upcomingCountLabels, upcomingReadLabel } from "./upcomingActionPresentation";

const sectionClass = "min-w-0 overflow-hidden rounded-2xl border border-border bg-card shadow-sm";
const sectionHeadingClass = "border-b border-border bg-muted/30 px-5 py-3.5 text-base font-semibold";
const detailsClass = "rounded-2xl border border-border bg-card shadow-sm";
const summaryClass = "cursor-pointer select-none rounded-2xl px-5 py-3.5 text-sm font-medium text-foreground marker:text-muted-foreground hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50";

export function UpcomingActionsPage({ isActive = true, actionsHost }: { isActive?: boolean; actionsHost?: HTMLElement | null }) {
  const { t, i18n } = useTranslation();
  const query = useUpcomingActions(isActive);
  const navigation = useObjectNavigation();
  const [filter, setFilter] = useState("all");
  const [limit, setLimit] = useState(50);
  const [eligibleOpen, setEligibleOpen] = useState(false);
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
  const refresh = <Button variant="outline" disabled={query.isFetching} onClick={() => { void query.refetch(); }}>{t("upcomingActions.refresh")}</Button>;
  return <div className="flex min-w-0 flex-col gap-5" aria-busy={query.isFetching}>
    {actionsHost ? createPortal(refresh, actionsHost) : actionsHost === undefined ? refresh : null}
    {!data && query.isPending && <LoadingState label={t("ui.state.loadingPage")} variant="list" />}
    {query.isError && !data && <ErrorState title={t("upcomingActions.loadError")} description={t("ui.state.errorDescription")} retryLabel={t("common.retry")} onRetry={() => query.refetch()} />}
    {query.isError && data && <Notice tone="warning" role="alert">{t("upcomingActions.stale")}</Notice>}
    {data && <>
      <Card className="flex flex-col gap-3 p-5">
        <p aria-live="polite" className="num text-base font-semibold">{t("upcomingActions.counts", upcomingCountLabels(t, data.counts))}</p>
        <div className="flex flex-col gap-0.5 text-xs text-muted-foreground">
          <p className="num">{t("upcomingActions.window", { date: formatCivilDate(data.localDate), timezone: data.timezone })}</p>
          <p className="num">{upcomingReadLabel(t, data.asOf, data.timezone, i18n.language)}</p>
        </div>
        {data.completeness === "partial" && <Notice tone="warning">{t("upcomingActions.partial")}</Notice>}
      </Card>
      <div role="group" aria-label={t("upcomingActions.filter")} className="flex flex-wrap gap-2">
        {["all", "receipt", "reservations", "scheduled"].map(value => <Button key={value} size="sm" variant={filter === value ? "default" : "outline"} aria-pressed={filter === value} onClick={() => { setFilter(value); setLimit(50); }}>{t(`upcomingActions.filters.${value}`)}</Button>)}
      </div>
      {filter === "all" ? <>
        {attentionItems.length > 0 && <section aria-label={t("upcomingActions.attentionTitle")} className={sectionClass}><h3 className={sectionHeadingClass}>{t("upcomingActions.attentionTitle")} · {data.counts.attention}</h3><div className="px-5"><UpcomingActionList context="upcoming-attention" items={attentionItems.slice(0, limit)} /></div></section>}
        {scheduledItems.length > 0 && <section aria-label={t("upcomingActions.scheduledTitle")} className={sectionClass}><h3 className={sectionHeadingClass}>{t("upcomingActions.scheduledTitle")} · {data.counts.scheduled}</h3><div className="px-5"><UpcomingActionList context="upcoming-scheduled" items={scheduledItems.slice(0, limit)} /></div></section>}
      </> : items.length > 0 && <div className={cn(sectionClass, "px-5")}><UpcomingActionList context="upcoming-list" items={items.slice(0, limit)} /></div>}
      {items.length === 0 && <EmptyState title={t("upcomingActions.empty")} variant="success" />}
      {hasMore && <Button variant="outline" className="self-center" onClick={() => setLimit(value => value + 50)}>{t("upcomingActions.showMore")}</Button>}
      <details className={detailsClass} onToggle={event => setEligibleOpen(event.currentTarget.open)}><summary className={summaryClass}>{t("upcomingActions.eligibleTitle", { count: data.counts.eligible })}</summary>{eligibleOpen && <div className="border-t border-border px-5"><UpcomingActionList context="upcoming-eligible" items={(data.items ?? []).filter(item => item.group === "eligible")} /></div>}</details>
      {navigation ? <Button variant="link" className="self-start px-0" onClick={() => navigation.open({ page: "data-health" })}>{t("upcomingActions.checksEntry", { count: data.counts.dataChecks })}</Button> : <p className="text-sm text-muted-foreground">{t("upcomingActions.checksEntry", { count: data.counts.dataChecks })}</p>}
      <Notice>{t("upcomingActions.normalOnly")}</Notice>
    </>}
  </div>;
}
