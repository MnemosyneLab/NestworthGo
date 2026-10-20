import type { TFunction } from "i18next";
import { formatDecimalPercent, formatMoney } from "@/lib/money";

type EvidenceLeaf = { metric: string; actual?: string; currency?: string };

/** Machine reason codes from the review engine, with a readable fallback for unknown codes. */
export function reviewReasonLabel(t: TFunction, code: string): string {
  return t(`reviewV2.reasons.${code}`, { defaultValue: code });
}

export function reviewReasonList(t: TFunction, codes: readonly string[]): string {
  return codes.map(code => reviewReasonLabel(t, code)).join(t("reviewV2.reasonSeparator"));
}

/**
 * Presents the engine's canonical `actual` string for one evaluated condition.
 * The value is only formatted, never recomputed: money uses the shared money
 * formatter, performance metrics are already percentages.
 */
export function reviewLeafActual(t: TFunction, leaf: EvidenceLeaf): string {
  const actual = leaf.actual;
  if (actual === undefined || actual === "") return t("reviewV2.unknown");
  switch (leaf.metric) {
    case "amount":
    case "cost":
      return formatMoney(actual, leaf.currency || undefined);
    case "price_change":
    case "annualized":
      return formatDecimalPercent(actual);
    case "type":
      return t(`reviewV2.types.${actual}`, { defaultValue: actual });
    default:
      return actual;
  }
}
