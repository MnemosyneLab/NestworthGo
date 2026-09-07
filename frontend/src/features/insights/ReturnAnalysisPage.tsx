import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import type { AnalysisNavigationContext } from "@/app/navigation";
import { PageChrome } from "@/components/layout/PageChrome";
import { PageIntro } from "@/components/layout/PageHeader";
import { EmptyState } from "@/components/layout/PageState";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AnalysisFilterBar } from "@/features/insights/AnalysisFilterBar";
import { ReturnCalendarTab } from "@/features/insights/ReturnCalendarTab";
import { currentMonth } from "@/features/insights/calendar";
import { useAnalysisStore } from "@/stores/analysis";
import { useHistoryOrigin } from "@/queries/history";

export function ReturnAnalysisPage({ onOpenAssetChanges }: { onOpenAssetChanges?: (analysis: AnalysisNavigationContext) => void }) {
  const { t } = useTranslation();
  const session = useAnalysisStore();
  const setFilters = useAnalysisStore((state) => state.setFilters);
  const setReturnView = useAnalysisStore((state) => state.setReturnView);
  const reset = useAnalysisStore((state) => state.reset);
  const origin = useHistoryOrigin();

  useEffect(() => {
    if (!session.returnCursor && !origin.isLoading) setReturnView({ cursor: currentMonth(origin.data?.timezone) });
  }, [origin.data?.timezone, origin.isLoading, session.returnCursor, setReturnView]);

  return (
    <div className="flex flex-col gap-6">
      <PageChrome pageId="return-analysis" title={t("insights.returnAnalysis")} />
      <PageIntro description={t("insights.description")} />
      <Tabs value={session.returnTab} onValueChange={(value) => setReturnView({ tab: value as typeof session.returnTab })}>
        <TabsList>
          <TabsTrigger value="calendar">{t("insights.calendar")}</TabsTrigger>
          <TabsTrigger value="trend">{t("insights.trend")}</TabsTrigger>
          <TabsTrigger value="contribution">{t("insights.contribution")}</TabsTrigger>
        </TabsList>
        <AnalysisFilterBar session={session} onChange={setFilters} onReset={reset} />
        <TabsContent value="calendar">
          <ReturnCalendarTab session={session} onCursorChange={(cursor) => setReturnView({ cursor })} onOpenAssetChanges={onOpenAssetChanges} />
        </TabsContent>
        <TabsContent value="trend"><EmptyState title={t("insights.trend")} description={t("insights.notAvailableYet")} /></TabsContent>
        <TabsContent value="contribution"><EmptyState title={t("insights.contribution")} description={t("insights.notAvailableYet")} /></TabsContent>
      </Tabs>
    </div>
  );
}
