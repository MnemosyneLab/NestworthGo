import type { TFunction } from "i18next";

export function quoteObservationLabel(t: TFunction, point: { observationKind?: string; sourceKind: string; sourceKey?: string; priceBasis?: string }) {
  if (point.priceBasis === "agent_unit_nav_v1") return t("quoteDetails.kinds.nav");
  if (point.observationKind === "close" && point.sourceKey === "coingecko") return t("portfolio.coinGeckoDailyReference");
  const kind = point.sourceKind === "manual" ? "manual" : point.observationKind || "unknown";
  return t(`quoteDetails.kinds.${kind}`, { defaultValue: t("quoteDetails.kinds.unknown") });
}
export function quoteSourceLabel(t: TFunction, kind: string, key?: string) {
  const provider = key && ["tiingo", "coingecko", "frankfurter", "goldapi"].includes(key) ? key : "";
  return [t(`portfolio.${kind}`, { defaultValue: t("quoteDetails.kinds.unknown") }), provider].filter(Boolean).join(" · ");
}
