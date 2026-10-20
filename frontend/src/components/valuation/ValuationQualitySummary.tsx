import { useTranslation } from "react-i18next";
import { formatCivilDate } from "@/lib/time";
import { hasDisplay, valuationDetails, valuationLabel, type DisplayValuation } from "./valuationQuality";

export function ValuationQualitySummary({ value, current = false, compact = false, net = false }: { value?: DisplayValuation | null; current?: boolean; compact?: boolean; net?: boolean }) {
  const { t } = useTranslation();
  if (!hasDisplay(value)) return null;
  if (compact) {const label=net && value.quality.availability!=="full" ? t("valuation.knownNetContribution") : valuationLabel(t,value,current);return <span className="text-xs font-normal text-muted-foreground" role="status" title={valuationDetails(t,value,current)} aria-label={valuationDetails(t,value,current)}>{label}{value.quality.carriedCount>0 && value.quality.oldestMarketDate ? ` · ${formatCivilDate(value.quality.oldestMarketDate)}` : ""}</span>;}
  return <span className="block text-xs text-muted-foreground" role="status">{valuationDetails(t, value, current)}</span>;
}
