import { useTranslation } from "react-i18next";
import { useObjectNavigation, type ObjectNavigation } from "@/app/NavigationContext";
import { useAnalysisStore } from "@/stores/analysis";
import { useHistoryOrigin } from "@/queries/history";
import { returnTrendRange } from "./analysisRequest";
import { ActionMenu, type ActionMenuItem } from "@/components/ui/action-menu";
import { Button } from "@/components/ui/button";

/** An object entry replaces the entire filter set; only the chosen dates carry over. */
type AnalyzeMenuProps = { accountId?: string; instrumentId?: string; label: string; inline?: boolean; onDividend?: () => void };
export function AnalyzeMenu(props: AnalyzeMenuProps) {
  const navigation = useObjectNavigation();
  return navigation || props.onDividend ? <AnalyzeMenuReady {...props} navigation={navigation} /> : null;
}
function AnalyzeMenuReady({ accountId, instrumentId, label, navigation, inline, onDividend }: AnalyzeMenuProps & { navigation: ObjectNavigation | null }) {
  const { t } = useTranslation();
  const origin = useHistoryOrigin();
  function navigate(page: "return-analysis" | "asset-changes") {
    if (!navigation) return;
    const session = useAnalysisStore.getState();
    const range = session.from && session.to ? { from: session.from, to: session.to }
      : origin.data?.startedAt ? returnTrendRange("30d", origin.data.startedAt, origin.data.timezone) : { from: "", to: "" };
    navigation.open({ page, tab: page === "return-analysis" ? "trend" : "drivers", analysis: {
      scope: instrumentId ? "instrument" : "account", scopeId: instrumentId ?? accountId,
      includeCash: !instrumentId, valuation: "base", ...range,
      moreFilters: instrumentId && accountId ? { accountId } : {}, returnType: "total_return",
    } } as import("@/app/navigation").NavigationTarget);
  }
  const items: ActionMenuItem[] = navigation ? [
    { id: "return", label: t("connections.viewReturn"), onSelect: () => navigate("return-analysis") },
    { id: "changes", label: t("connections.viewChanges"), onSelect: () => navigate("asset-changes") },
  ] : [];
  if (onDividend) items.push({ id: "dividend", label: t("accounts.cashDividend"), onSelect: onDividend });
  if (inline) return <div className="flex flex-wrap gap-1">
    {items.map((item) => <Button key={item.id} type="button" variant="ghost" size="sm" onClick={item.onSelect}>
      {t(item.id === "return" ? "connections.accountReturns" : "connections.accountChanges")}
    </Button>)}
  </div>;
  return <ActionMenu label={t("connections.actionsFor", { name: label })} items={items} />;
}
