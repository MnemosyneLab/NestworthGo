import { InvestmentReviewSummaryV2 } from "@/features/investment-review/InvestmentReviewSummaryV2";
import { upcomingCountLabels, upcomingReadLabel } from "./upcomingActionPresentation";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Notice } from "@/components/ui/notice";
import { LoadingState } from "@/components/layout/PageState";
import { useUpcomingActions } from "@/queries/liquidity";
import { useObjectNavigation } from "@/app/NavigationContext";
import { UpcomingActionList } from "./UpcomingActionList";

export function OverviewUpcomingActionsCard({ isActive = true, context = "overview-upcoming-actions", presentation = "standard" }: { isActive?: boolean; context?: string; presentation?: "standard" | "homepage" }) {
  const { t, i18n } = useTranslation();
  const query = useUpcomingActions(isActive);
  const navigation = useObjectNavigation();
  const data = query.data;
  // Other callers retain their existing review entry; homepage composes it in Needs attention.
  const homepage = presentation === "homepage";
  return <Card className="min-w-0" data-testid="overview-upcoming-actions-card" aria-busy={query.isFetching}>
    <CardHeader><CardTitle id={`${context}-heading`} tabIndex={-1}>{t(homepage ? "homepage.next30Days" : "upcomingActions.title")}</CardTitle></CardHeader>
    <CardContent className="flex flex-col gap-3">
      {!data && query.isPending && <LoadingState label={t("ui.state.loadingPage")} variant="list" />}
      {query.isError && <Notice tone={data ? "warning" : "danger"} role="alert"><div className="flex flex-wrap items-center justify-between gap-2"><p>{t(data ? "upcomingActions.stale" : "upcomingActions.loadError")}</p><Button variant="outline" size="sm" onClick={() => { void query.refetch(); }}>{t("common.retry")}</Button></div></Notice>}
      {data && <>
        {homepage ? <p className="break-words text-xs text-muted-foreground">
          {t("homepage.actionWindow", { start: data.windowStartOn, end: data.windowEndOn })}
          <span className="block">{upcomingReadLabel(t, data.asOf, data.timezone, i18n.language)}</span>
        </p> : <>
          <p className="num text-xs text-muted-foreground">{t("upcomingActions.window", { date: data.localDate, timezone: data.timezone })}</p>
          <p className="num text-xs text-muted-foreground">{upcomingReadLabel(t, data.asOf, data.timezone, i18n.language)}</p>
        </>}
        {(!homepage || data.counts.total > 0) && <p className="num text-sm font-semibold" aria-live="polite">{t("upcomingActions.overviewCounts", upcomingCountLabels(t, data.counts))}</p>}
        <UpcomingActionList context={context} items={(data.items ?? []).filter(item => item.group === "attention" || (homepage && item.group === "scheduled")).slice(0, 3)} />
        {data.counts.total === 0 && <p className="rounded-xl border border-dashed border-border bg-muted/30 px-3 py-4 text-center text-sm text-muted-foreground">{t(homepage ? "homepage.noScheduledActions" : "upcomingActions.empty")}</p>}
        {(data.counts.eligible > 0 || data.counts.dataChecks > 0 || data.counts.scopeNotes > 0) && <p className="text-sm text-muted-foreground">{t("upcomingActions.supplement", upcomingCountLabels(t, data.counts))}</p>}
        {data.completeness === "partial" && <Notice tone="warning">{t("upcomingActions.partial")}</Notice>}
      </>}
      {navigation && data && (!homepage || data.counts.total > 0) && <Button className="h-auto max-w-full whitespace-normal p-0 text-left" variant="link" onClick={() => navigation.open({ page: "upcoming-actions" })}>{t("upcomingActions.viewAll", { count: data.counts.total })}</Button>}
      {homepage && navigation && data && data.counts.total === 0 && (data.counts.eligible > 0 || data.counts.dataChecks > 0 || data.counts.scopeNotes > 0) && <Button className="h-auto whitespace-normal p-0 text-left" variant="link" onClick={() => navigation.open({ page: "upcoming-actions" })}>{t("homepage.reviewActionDetails")}</Button>}
      {!homepage && <InvestmentReviewSummaryV2 isActive={isActive} />}
    </CardContent>
  </Card>;
}
