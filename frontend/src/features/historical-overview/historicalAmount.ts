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
