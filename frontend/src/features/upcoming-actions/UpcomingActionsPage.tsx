import { useState } from "react";
import { useTranslation } from "react-i18next";
import { PageChrome } from "@/components/layout/PageChrome";
import { Button } from "@/components/ui/button";
import { useUpcomingActions } from "@/queries/liquidity";
import { useObjectNavigation } from "@/app/NavigationContext";
import { UpcomingActionList } from "./UpcomingActionList";
import { upcomingNavigation, upcomingButton } from "./upcomingActionPresentation";

export function UpcomingActionsPage({ isActive = true }: { isActive?: boolean }) {
  const { t } = useTranslation();
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
    return (item.kind === "product_schedule" || item.kind === "source_access") && item.phase !== "receipt_due";
  });
  return <div className="space-y-5" aria-busy={query.isFetching}>
    <PageChrome pageId="upcoming-actions" title={t("upcomingActions.title")} actions={<Button variant="outline" disabled={query.isFetching} onClick={() => { void query.refetch(); }}>{t("upcomingActions.refresh")}</Button>} />
    <h2 id="upcoming-actions-heading" tabIndex={-1} className="text-xl font-semibold">{t("upcomingActions.title")}</h2>
    {!data && query.isPending && <p role="status">{t("ui.state.loadingPage")}</p>}
    {query.isError && <p role="alert">{t(data ? "upcomingActions.stale" : "upcomingActions.loadError")}</p>}
    {data && <>
      <p className="text-sm text-muted-foreground">{t("upcomingActions.window", { date: data.localDate, timezone: data.timezone })}</p>
      <p className="text-sm text-muted-foreground">{t("upcomingActions.asOf", { asOf: data.asOf })}</p>
      <p aria-live="polite">{t("upcomingActions.counts", { ...data.counts })}</p>
      {data.completeness === "partial" && <p className="text-warning-foreground">{t("upcomingActions.partial")}</p>}
      <div role="group" aria-label={t("upcomingActions.filter")} className="flex flex-wrap gap-2">
        {["all", "receipt", "reservations", "scheduled"].map(value => <Button key={value} variant={filter === value ? "default" : "outline"} aria-pressed={filter === value} onClick={() => { setFilter(value); setLimit(50); }}>{t(`upcomingActions.filters.${value}`)}</Button>)}
      </div>
      <UpcomingActionList context="upcoming-list" items={items.slice(0, limit)} />
      {items.length === 0 && <p>{t("upcomingActions.empty")}</p>}
      {items.length > limit && <Button variant="outline" onClick={() => setLimit(value => value + 50)}>{t("upcomingActions.showMore")}</Button>}
      <details className="rounded-lg border p-4"><summary className="cursor-pointer">{t("upcomingActions.eligibleTitle", { count: data.counts.eligible })}</summary><UpcomingActionList context="upcoming-eligible" items={(data.items ?? []).filter(item => item.group === "eligible")} /></details>
      <details className="space-y-3 rounded-lg border p-4"><summary className="cursor-pointer">{t("upcomingActions.checksTitle", { count: data.counts.dataChecks, scopeNotes: data.counts.scopeNotes })}</summary>
        <ul className="space-y-4">{(data.dataChecks ?? []).map(check => {
          const target = upcomingNavigation(check.target);
          return <li key={check.id} className="space-y-1 break-words"><p className="font-medium">{check.displayName}{check.anchorOn && <> · {check.anchorOn}</>}</p>{check.referenceOn && <p className="text-sm text-muted-foreground">{t("upcomingActions.reference", { date: check.referenceOn })}</p>}{(check.reasonCodes ?? []).map(reason => <p key={reason} className="text-sm">{t(`upcomingActions.reasons.${reason}`, { defaultValue: t("upcomingActions.reasons.unsupported_source") })}</p>)}{target && navigation && <Button variant="outline" size="sm" onClick={() => navigation.open(target)}>{upcomingButton(t, check.target)}</Button>}</li>;
        })}</ul>
        {navigation && <Button variant="link" onClick={() => navigation.open({ page: "data-health" })}>{t("nav.dataHealth")}</Button>}
      </details>
      <p className="text-sm text-muted-foreground">{t("upcomingActions.normalOnly")}</p>
    </>}
  </div>;
}
