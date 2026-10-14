import type { TFunction } from "i18next";
import { hasDisplay, type DisplayValuation, type DisplayMoney } from "@/components/valuation/valuationQuality";
import { formatMoney, compareCanonical } from "@/lib/money";

/** Historical money shares the ordinary display policy; canonical values stay untouched. */
export function formatHistoricalAmount(value: string | null | undefined, currency: string, signed = false): string | null {
  if (value == null) return null;
  const display = formatMoney(value, currency);
  const plus = signed && compareCanonical(value, "0.005") > 0 ? "+" : "";
  return plus + display;
}
export function historicalDisplayAmount(t: TFunction, value: DisplayValuation | null | undefined, legacy: DisplayMoney | null | undefined, net = false): string {
  const money = hasDisplay(value) ? value.total ?? value.knownSubtotal : legacy;
  if (!money) return t("historicalOverview.unknown");
  const partial = hasDisplay(value) && !value.total;
  return `${formatMoney(money.amount, money.currency)}${partial ? ` · ${t(net ? "valuation.knownNetContribution" : "valuation.knownSubtotal")}` : ""}`;
}
