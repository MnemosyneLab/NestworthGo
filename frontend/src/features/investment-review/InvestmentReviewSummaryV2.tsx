import { Button } from "@/components/ui/button";
import { useObjectNavigation } from "@/app/NavigationContext";
import { useReviewAlertPage } from "@/queries/investmentReviewV2";
import { useTranslation } from "react-i18next";

export function InvestmentReviewSummaryV2({ isActive = true, compact = false }: { isActive?: boolean; compact?: boolean }) {
  const { t } = useTranslation();
  const navigation = useObjectNavigation();
  const query = useReviewAlertPage({ status: "unignored", instrumentType: "", query: "" }, "", isActive);
  const summary = query.data?.summary;
  const actionable = (query.data?.items ?? []).filter(group => (group.rules ?? []).some(rule => rule.actionable)).slice(0, 3);

  return <section aria-label={t("reviewV2.title")} className={`min-w-0 space-y-2 ${compact ? "border-t border-border pt-4" : "border-t pt-3"}`} aria-busy={query.isFetching}>
    <h3 className="font-medium">{t("reviewV2.title")}</h3>
    {!query.data && query.isPending && <p role="status" className="text-sm text-muted-foreground">{t("ui.state.loadingPage")}</p>}
    {query.isError && <div role="alert" className="space-y-2 text-sm"><p>{t(query.data ? "reviewV2.staleRead" : "reviewV2.loadError")}</p><Button size="sm" variant="outline" onClick={() => void query.refetch()}>{t("common.retryAction")}</Button></div>}
    {summary && <>
      <p className="text-sm" role="status">{t("reviewV2.summaryCounts", { total: summary.totalGroups, matched: summary.matchedGroups, pending: summary.pendingGroups, actionable: summary.actionableRules, unknown: summary.unknownGroups })}</p>
      {actionable.length > 0 && <ul className="space-y-1 text-sm">{actionable.map(group => <li key={group.instrumentId} className="break-words font-medium">{group.displayName}</li>)}</ul>}
      {!query.isFetching && summary.actionableRules === 0 && <p className="text-sm text-muted-foreground">{t("reviewV2.noAlerts")}</p>}
    </>}
    {navigation && <Button className="h-auto max-w-full whitespace-normal p-0 text-left" variant="link" onClick={() => navigation.open({ page: "upcoming-actions", tab: "review" })}>{t("reviewV2.openReview")}</Button>}
  </section>;
}
