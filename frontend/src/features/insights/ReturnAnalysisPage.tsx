import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import type { AnalysisNavigationContext } from "@/app/navigation";
import { PageChrome } from "@/components/layout/PageChrome";
import { effectiveRange } from "./analysisRequest";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AnalysisFilterBar } from "@/features/insights/AnalysisFilterBar";
import { ReturnCalendarTab } from "@/features/insights/ReturnCalendarTab";
import { ReturnTrendTab } from "@/features/insights/ReturnTrendTab";
import { ContributionTab } from "@/features/insights/ContributionTab";
import { currentMonth } from "@/features/insights/calendar";
import { useAnalysisStore } from "@/stores/analysis";
import { useHistoryOrigin } from "@/queries/history";
import type { HistoryNavigationFilters } from "@/app/navigation";

export function ReturnAnalysisPage({ onOpenAssetChanges, onOpenHistory }: { onOpenAssetChanges?: (analysis: AnalysisNavigationContext) => void; onOpenHistory?: (filters: HistoryNavigationFilters) => void }) {
  const { t } = useTranslation();
  const session = useAnalysisStore();
  const setFilters = useAnalysisStore((state) => state.setFilters);
  const setReturnView = useAnalysisStore((state) => state.setReturnView);
  const reset = useAnalysisStore((state) => state.resetFilters);
  const origin = useHistoryOrigin();

  useEffect(() => {
    if (!session.returnCursor && !origin.isLoading) setReturnView({ cursor: currentMonth(origin.data?.timezone) });
  }, [origin.data?.timezone, origin.isLoading, session.returnCursor, setReturnView]);

  return (
    <div className="flex flex-col gap-4">
      <PageChrome pageId="return-analysis" title={t("insights.returnAnalysis")} />
      <Tabs className="flex flex-col gap-4" value={session.returnTab} onValueChange={(value) => setReturnView({ tab: value as typeof session.returnTab })}>
        <TabsList className="self-start">
          <TabsTrigger value="calendar">{t("insights.calendar")}</TabsTrigger>
          <TabsTrigger value="trend">{t("insights.trend")}</TabsTrigger>
          <TabsTrigger value="contribution">{t("insights.contribution")}</TabsTrigger>
        </TabsList>
        <AnalysisFilterBar session={session} resolvedRange={effectiveRange(session, session.returnTab === "calendar" ? session.returnCursor || currentMonth(origin.data?.timezone) : currentMonth(origin.data?.timezone), origin.data?.timezone, origin.data?.startedAt)} onChange={(filters) => { setFilters(filters); if (filters.to) setReturnView({ cursor: filters.to.slice(0, 7) }); }} onReset={reset} />
        <TabsContent value="calendar">
          <ReturnCalendarTab session={session} onCursorChange={(cursor) => setReturnView({ cursor })} onOpenAssetChanges={onOpenAssetChanges} />
        </TabsContent>
        <TabsContent value="trend"><ReturnTrendTab session={session} /></TabsContent>
        <TabsContent value="contribution"><ContributionTab session={session} onOpenHistory={onOpenHistory} /></TabsContent>
      </Tabs>
    </div>
  );
}
