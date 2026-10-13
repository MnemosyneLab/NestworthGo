import { InvestmentReviewSummary } from "@/features/investment-review/InvestmentReviewPanel";
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
  return <Card data-testid="overview-upcoming-actions-card" aria-busy={query.isFetching}>
    <CardHeader><CardTitle id={`${context}-heading`} tabIndex={-1}>{t("upcomingActions.title")}</CardTitle></CardHeader>
    <CardContent className="space-y-3">
      {!data && query.isPending && <p role="status">{t("ui.state.loadingPage")}</p>}
      {query.isError && <div role="alert"><p>{t(data ? "upcomingActions.stale" : "upcomingActions.loadError")}</p><Button variant="outline" onClick={() => { void query.refetch(); }}>{t("common.retry")}</Button></div>}
      {data && <>
        <p className="text-sm text-muted-foreground">{t("upcomingActions.window", { date: data.localDate, timezone: data.timezone })}</p>
        <p className="text-xs text-muted-foreground">{upcomingReadLabel(t, data.asOf, data.timezone, i18n.language)}</p>
        <p className="text-sm" aria-live="polite">{t("upcomingActions.overviewCounts", upcomingCountLabels(t, data.counts))}</p>
        <UpcomingActionList context={context} items={(data.items ?? []).filter(item => item.group === "attention").slice(0, 3)} />
        {data.counts.total === 0 && <p className="text-sm">{t("upcomingActions.empty")}</p>}
        {(data.counts.eligible > 0 || data.counts.dataChecks > 0 || data.counts.scopeNotes > 0) && <p className="text-sm">{t("upcomingActions.supplement", upcomingCountLabels(t, data.counts))}</p>}
        {data.completeness === "partial" && <p className="text-sm text-warning-foreground">{t("upcomingActions.partial")}</p>}
      </>}
      {navigation && data && <Button variant="link" onClick={() => navigation.open({ page: "upcoming-actions" })}>{t("upcomingActions.viewAll", { count: data.counts.total })}</Button>}
      <InvestmentReviewSummary isActive={isActive} />
    </CardContent>
  </Card>;
}
