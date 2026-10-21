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
export function historicalDisplayAmount(t: TFunction, value: DisplayValuation | null | undefined, legacy: DisplayMoney | null | undefined, net = false, includeSubtotalLabel = true): string {
  // Callers supply the exact complete amount for this same metric/row as
  // legacy, never a known-only subtotal. Prefer it for a full valuation so
  // Money4 projection followed by currency rounding cannot round twice.
  const display = hasDisplay(value);
  const money = display
    ? value.total && value.quality.availability === "full" && legacy?.currency === value.total.currency
      ? legacy : value.total ?? value.knownSubtotal
    : legacy;
  if (!money) return t("historicalOverview.unknown");
  const partial = includeSubtotalLabel && hasDisplay(value) && !value.total;
  return `${formatMoney(money.amount, money.currency)}${partial ? ` · ${t(net ? "valuation.knownNetContribution" : "valuation.knownSubtotal")}` : ""}`;
}
