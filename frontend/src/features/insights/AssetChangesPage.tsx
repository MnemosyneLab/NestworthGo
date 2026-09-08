import { useTranslation } from "react-i18next";
import { PageChrome } from "@/components/layout/PageChrome";
import { PageIntro } from "@/components/layout/PageHeader";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AnalysisFilterBar } from "@/features/insights/AnalysisFilterBar";
import { ChangeDriversTab } from "@/features/insights/ChangeDriversTab";
import { AssetTrendTab } from "@/features/insights/AssetTrendTab";
import { CategoriesTab } from "@/features/insights/CategoriesTab";
import { analysisRequest, effectiveRange } from "@/features/insights/analysisRequest";
import { currentMonth } from "@/features/insights/calendar";
import { useAnalysisStore } from "@/stores/analysis";
import { useHistoryOrigin } from "@/queries/history";
import type { HistoryNavigationFilters } from "@/app/navigation";

export function AssetChangesPage({ onOpenHistory }: { onOpenHistory?: (filters: HistoryNavigationFilters) => void }) {
  const { t } = useTranslation();
  const session = useAnalysisStore();
  const setFilters = useAnalysisStore((state) => state.setFilters);
  const setAssetView = useAnalysisStore((state) => state.setAssetView);
  const reset = useAnalysisStore((state) => state.reset);
  const origin = useHistoryOrigin();
  const scope = session.scope ?? "portfolio";
  const visibleMonth = currentMonth(origin.data?.timezone);
  const range = effectiveRange(session, visibleMonth, origin.data?.timezone);
  const request = analysisRequest(session, range.from, range.to);
  const scopeReady = scope === "portfolio" || Boolean(session.scopeId);
  const rangeAvailable = range.from <= range.to;

  const driverContent = origin.isLoading ? (
    <LoadingState label={t("insights.loading")} />
  ) : origin.isError ? (
    <ErrorState title={t("insights.error")} description={t("ui.state.errorDescription")} onRetry={() => origin.refetch()} retryLabel={t("common.retryAction")} />
  ) : !origin.data ? (
    <EmptyState title={t("insights.noOrigin")} description={t("insights.noOriginHint")} />
  ) : !scopeReady ? (
    <EmptyState title={t("insights.scopeRequired")} description={t("insights.scopeRequiredHint")} />
  ) : !rangeAvailable ? (
    <EmptyState title={t("insights.noAssetChangeData")} description={t("insights.noAssetChangeDataHint")} />
  ) : (
    <ChangeDriversTab request={request} scope={scope} onOpenHistory={onOpenHistory} />
  );

  return (
    <div className="flex flex-col gap-6">
      <PageChrome pageId="asset-changes" title={t("insights.assetChanges")} />
      <PageIntro description={t("insights.assetChangesDescription")} />
      <Tabs value={session.assetTab} onValueChange={(value) => setAssetView({ tab: value as typeof session.assetTab })}>
        <TabsList>
          <TabsTrigger value="drivers">{t("insights.drivers")}</TabsTrigger>
          <TabsTrigger value="trend">{t("insights.assetTrend")}</TabsTrigger>
          <TabsTrigger value="categories">{t("insights.categories")}</TabsTrigger>
        </TabsList>
        <AnalysisFilterBar session={session} onChange={setFilters} onReset={reset} />
        <TabsContent value="drivers">{driverContent}</TabsContent>
        <TabsContent value="trend"><AssetTrendTab session={session} /></TabsContent>
        <TabsContent value="categories"><CategoriesTab session={session} onOpenHistory={onOpenHistory} /></TabsContent>
      </Tabs>
    </div>
  );
}
