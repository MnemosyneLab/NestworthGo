import { useTranslation } from "react-i18next";
import { useObjectNavigation, type ObjectNavigation } from "@/app/NavigationContext";
import { useAnalysisStore } from "@/stores/analysis";
import { useHistoryOrigin } from "@/queries/history";
import { returnTrendRange } from "./analysisRequest";
import { NativeSelect } from "@/components/ui/select";

/** An object entry replaces the entire filter set; only the chosen dates carry over. */
type AnalyzeMenuProps = { accountId?: string; instrumentId?: string; label: string };
export function AnalyzeMenu(props: AnalyzeMenuProps) {
  const navigation = useObjectNavigation();
  return navigation ? <AnalyzeMenuReady {...props} navigation={navigation} /> : null;
}
function AnalyzeMenuReady({ accountId, instrumentId, label, navigation }: AnalyzeMenuProps & { navigation: ObjectNavigation }) {
  const { t } = useTranslation();
  const origin = useHistoryOrigin();
  return <NativeSelect aria-label={t("connections.analyzeObject", { name: label })} value="" className="w-auto" onChange={event => {
    const page = event.target.value;
    if (page !== "return-analysis" && page !== "asset-changes") return;
    const session = useAnalysisStore.getState();
    const range = session.from && session.to ? { from: session.from, to: session.to }
      : origin.data?.startedAt ? returnTrendRange("30d", origin.data.startedAt, origin.data.timezone) : { from: "", to: "" };
    navigation.open({ page, tab: page === "return-analysis" ? "trend" : "drivers", analysis: {
      scope: instrumentId ? "instrument" : "account", scopeId: instrumentId ?? accountId,
      includeCash: !instrumentId, valuation: "base", ...range,
      moreFilters: instrumentId && accountId ? { accountId } : {}, returnType: "total_return",
    } } as import("@/app/navigation").NavigationTarget);
  }}>
    <option value="" disabled>{t("connections.analyze")}</option>
    <option value="return-analysis">{t("connections.viewReturn")}</option>
    <option value="asset-changes">{t("connections.viewChanges")}</option>
  </NativeSelect>;
}
