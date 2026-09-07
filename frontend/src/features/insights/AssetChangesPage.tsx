import { useTranslation } from "react-i18next";
import { PageChrome } from "@/components/layout/PageChrome";
import { PageIntro } from "@/components/layout/PageHeader";
import { EmptyState } from "@/components/layout/PageState";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AnalysisFilterBar } from "@/features/insights/AnalysisFilterBar";
import { useAnalysisStore } from "@/stores/analysis";

export function AssetChangesPage() {
  const { t } = useTranslation();
  const session = useAnalysisStore();
  const setFilters = useAnalysisStore((state) => state.setFilters);
  const setAssetView = useAnalysisStore((state) => state.setAssetView);
  const reset = useAnalysisStore((state) => state.reset);
  return <div className="flex flex-col gap-6"><PageChrome pageId="asset-changes" title={t("insights.assetChanges")} /><PageIntro description={t("insights.description")} /><Tabs value={session.assetTab} onValueChange={(value) => setAssetView({ tab: value as typeof session.assetTab })}><TabsList><TabsTrigger value="drivers">{t("insights.drivers")}</TabsTrigger><TabsTrigger value="trend">{t("insights.assetTrend")}</TabsTrigger><TabsTrigger value="categories">{t("insights.categories")}</TabsTrigger></TabsList><AnalysisFilterBar session={session} onChange={setFilters} onReset={reset} /><TabsContent value="drivers"><EmptyState title={t("insights.assetChanges")} description={t("insights.placeholder")} /></TabsContent><TabsContent value="trend"><EmptyState title={t("insights.assetTrend")} description={t("insights.notAvailableYet")} /></TabsContent><TabsContent value="categories"><EmptyState title={t("insights.categories")} description={t("insights.notAvailableYet")} /></TabsContent></Tabs></div>;
}
