import { upcomingCountLabels, upcomingReadLabel } from "./upcomingActionPresentation";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { useUpcomingActions } from "@/queries/liquidity";
import { useObjectNavigation } from "@/app/NavigationContext";
import { UpcomingActionList } from "./UpcomingActionList";

export function OverviewUpcomingActionsCard({ isActive = true, context = "overview-upcoming-actions" }: { isActive?: boolean; context?: string }) {
  const { t, i18n } = useTranslation();
  const query = useUpcomingActions(isActive);
  const navigation = useObjectNavigation();
  const data = query.data;
  return <Card className="min-w-0" data-testid="overview-upcoming-actions-card" aria-busy={query.isFetching}>
    <CardHeader><CardTitle id={`${context}-heading`} tabIndex={-1}>{t("homepage.next30Days")}</CardTitle></CardHeader>
    <CardContent className="space-y-3">
      {!data && query.isPending && <p role="status">{t("ui.state.loadingPage")}</p>}
      {query.isError && <div role="alert"><p>{t(data ? "upcomingActions.stale" : "upcomingActions.loadError")}</p><Button variant="outline" onClick={() => { void query.refetch(); }}>{t("common.retry")}</Button></div>}
      {data && <>
        <p className="break-words text-xs text-muted-foreground">
          {t("homepage.actionWindow", { start: data.windowStartOn, end: data.windowEndOn })}
          <span className="block">{upcomingReadLabel(t, data.asOf, data.timezone, i18n.language)}</span>
        </p>
        {data.counts.total > 0 && <p className="text-sm" aria-live="polite">{t("upcomingActions.overviewCounts", upcomingCountLabels(t, data.counts))}</p>}
        <UpcomingActionList context={context} items={(data.items ?? []).filter(item => item.group === "attention" || item.group === "scheduled").slice(0, 3)} />
        {data.counts.total === 0 && <p className="text-sm">{t("homepage.noScheduledActions")}</p>}
        {(data.counts.eligible > 0 || data.counts.dataChecks > 0 || data.counts.scopeNotes > 0) && <p className="text-sm">{t("upcomingActions.supplement", upcomingCountLabels(t, data.counts))}</p>}
        {data.completeness === "partial" && <p className="text-sm text-warning-foreground">{t("upcomingActions.partial")}</p>}
      </>}
      {navigation && data && data.counts.total > 0 && <Button className="h-auto max-w-full whitespace-normal p-0 text-left" variant="link" onClick={() => navigation.open({ page: "upcoming-actions" })}>{t("upcomingActions.viewAll", { count: data.counts.total })}</Button>}
      {navigation && data && data.counts.total === 0 && (data.counts.eligible > 0 || data.counts.dataChecks > 0 || data.counts.scopeNotes > 0) && <Button className="h-auto whitespace-normal p-0 text-left" variant="link" onClick={() => navigation.open({ page: "upcoming-actions" })}>{t("homepage.reviewActionDetails")}</Button>}
    </CardContent>
  </Card>;
}
