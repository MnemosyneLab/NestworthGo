import type { TFunction } from "i18next";
import { hasDisplay, type DisplayValuation, type DisplayMoney } from "@/components/valuation/valuationQuality";
import { formatAmount } from "@/lib/money";
import { isZero } from "./historicalChartModel";

/** Displays backend decimals without rounding a nonzero value into signed zero. */
export function formatHistoricalAmount(value: string | null | undefined, currency: string, signed = false, exact = false): string | null {
  if (value == null) return null;
  const plus = signed && !isZero(value) && !value.startsWith("-") ? "+" : "";
  const rounded = formatAmount(value, exact ? undefined : currency);
  const signedZero = formatAmount(value.startsWith("-") ? "-0" : "0", currency);
  const display = !exact && !isZero(value) && rounded === signedZero ? `${value} ${currency}` : `${rounded}${exact ? ` ${currency}` : ""}`;
  return `${plus}${display}`;
}

/** Preserve the historical surface's exact/nonzero fallback and unknown copy. */
export function historicalDisplayAmount(t: TFunction, value: DisplayValuation | null | undefined, legacy: DisplayMoney | null | undefined, net = false, exact = false): string {
  const money = hasDisplay(value) ? value.total ?? value.knownSubtotal : legacy;
  if (!money) return t("historicalOverview.unknown");
  const display = exact ? `${money.amount} ${money.currency}` : formatHistoricalAmount(money.amount,money.currency)!;
  const partial = hasDisplay(value) && !value.total;
  return `${display}${partial ? ` · ${t(net ? "valuation.knownNetContribution" : "valuation.knownSubtotal")}` : ""}`;
}
