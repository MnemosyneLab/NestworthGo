import { useTranslation } from "react-i18next";
import { useObjectNavigation } from "@/app/NavigationContext";
import { Button } from "@/components/ui/button";

/** Projection issues do not always identify an object; show the dated household scan. */
export function AnalysisHealthLink({ from, to }: { from?: string; to?: string }) {
  const { t } = useTranslation();
  const navigation = useObjectNavigation();
  if (!navigation || !from || !to) return null;
  return <Button variant="outline" size="sm" className="self-start" onClick={() => navigation.open({ page: "data-health", focus: { rangeStart: from, rangeEnd: to } })}>{t("connections.viewPeriodGaps")}</Button>;
}
