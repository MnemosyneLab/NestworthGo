import { useState } from "react";
import { useTranslation } from "react-i18next";
import { PageChrome } from "@/components/layout/PageChrome";
import { Button } from "@/components/ui/button";
import { useUpcomingActions } from "@/queries/liquidity";
import { useObjectNavigation } from "@/app/NavigationContext";
import { UpcomingActionList } from "./UpcomingActionList";
import { upcomingNavigation, upcomingButton, upcomingCountLabels, upcomingReadLabel } from "./upcomingActionPresentation";

export function UpcomingActionsPage({ isActive = true }: { isActive?: boolean }) {
  const { t, i18n } = useTranslation();
  const query = useUpcomingActions(isActive);
  const navigation = useObjectNavigation();
  const [filter, setFilter] = useState("all");
  const [limit, setLimit] = useState(50);
  const data = query.data;
  const items = (data?.items ?? []).filter(item => {
    if (item.group === "eligible") return false;
    if (filter === "all") return true;
    if (filter === "receipt") return item.phase === "receipt_due";
    if (filter === "reservations") return item.kind.startsWith("reservation_");
    return item.group === "scheduled";
  });
  return <div className="space-y-5" aria-busy={query.isFetching}>
    <PageChrome pageId="upcoming-actions" title={t("upcomingActions.title")} actions={<Button variant="outline" disabled={query.isFetching} onClick={() => { void query.refetch(); }}>{t("upcomingActions.refresh")}</Button>} />
    <h2 id="upcoming-actions-heading" tabIndex={-1} className="text-xl font-semibold">{t("upcomingActions.title")}</h2>
    {!data && query.isPending && <p role="status">{t("ui.state.loadingPage")}</p>}
    {query.isError && <p role="alert">{t(data ? "upcomingActions.stale" : "upcomingActions.loadError")}</p>}
    {data && <>
      <p className="text-sm text-muted-foreground">{t("upcomingActions.window", { date: data.localDate, timezone: data.timezone })}</p>
      <p className="text-sm text-muted-foreground">{upcomingReadLabel(t, data.asOf, data.timezone, i18n.language)}</p>
      <p aria-live="polite">{t("upcomingActions.counts", upcomingCountLabels(t, data.counts))}</p>
      {data.completeness === "partial" && <p className="text-warning-foreground">{t("upcomingActions.partial")}</p>}
      <div role="group" aria-label={t("upcomingActions.filter")} className="flex flex-wrap gap-2">
        {["all", "receipt", "reservations", "scheduled"].map(value => <Button key={value} variant={filter === value ? "default" : "outline"} aria-pressed={filter === value} onClick={() => { setFilter(value); setLimit(50); }}>{t(`upcomingActions.filters.${value}`)}</Button>)}
      </div>
      {filter === "all" ? <>
        <section aria-label={t("upcomingActions.attentionTitle")}><h3 className="text-lg font-semibold">{t("upcomingActions.attentionTitle")} · {data.counts.attention}</h3><UpcomingActionList context="upcoming-attention" items={items.filter(item => item.group === "attention").slice(0, limit)} /></section>
        <section aria-label={t("upcomingActions.scheduledTitle")}><h3 className="text-lg font-semibold">{t("upcomingActions.scheduledTitle")} · {data.counts.scheduled}</h3><UpcomingActionList context="upcoming-scheduled" items={items.filter(item => item.group === "scheduled").slice(0, Math.max(0, limit - items.filter(item => item.group === "attention").length))} /></section>
      </> : <UpcomingActionList context="upcoming-list" items={items.slice(0, limit)} />}
      {items.length === 0 && <p>{t("upcomingActions.empty")}</p>}
      {items.length > limit && <Button variant="outline" onClick={() => setLimit(value => value + 50)}>{t("upcomingActions.showMore")}</Button>}
      <details className="rounded-lg border p-4"><summary className="cursor-pointer">{t("upcomingActions.eligibleTitle", { count: data.counts.eligible })}</summary><UpcomingActionList context="upcoming-eligible" items={(data.items ?? []).filter(item => item.group === "eligible")} /></details>
      <details className="space-y-3 rounded-lg border p-4"><summary className="cursor-pointer">{t("upcomingActions.checksTitle", upcomingCountLabels(t, data.counts))}</summary>
        <ul className="space-y-4">{(data.dataChecks ?? []).map(check => {
          const targets = [check.target, ...(check.targets ?? [])].filter((target, index, all) => all.findIndex(other => JSON.stringify(other) === JSON.stringify(target)) === index);
          return <li key={check.id} className="space-y-1 break-words"><p className="font-medium">{check.displayName}{check.anchorOn && <> · {check.anchorOn}</>}</p>{check.referenceOn && <p className="text-sm text-muted-foreground">{t("upcomingActions.reference", { date: check.referenceOn })}</p>}{(check.reasonCodes ?? []).map(reason => <p key={reason} className="text-sm">{t(`upcomingActions.reasons.${reason}`, { defaultValue: t("upcomingActions.reasons.unsupported_source") })}</p>)}{navigation && <div className="flex flex-wrap gap-2">{targets.map((target, index) => { const destination = upcomingNavigation(target); return destination && <Button key={`${target.kind}-${index}`} variant="outline" size="sm" onClick={() => navigation.open(destination)}>{upcomingButton(t, target)}</Button>; })}</div>}</li>;
        })}</ul>
        {navigation && <Button variant="link" onClick={() => navigation.open({ page: "data-health" })}>{t("nav.dataHealth")}</Button>}
      </details>
      <p className="text-sm text-muted-foreground">{t("upcomingActions.normalOnly")}</p>
    </>}
  </div>;
}
