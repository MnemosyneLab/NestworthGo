import type { TFunction } from "i18next";
import { formatAmount } from "@/lib/money";

export interface DisplayMoney { amount: string; currency: string }
export interface DisplayQuality {
  availability: string; coverage: string; includedCount: number; valuedCount: number;
  unpricedCount: number; provisionalCount: number; carriedCount: number; staleCount: number;
  oldestMarketDate?: string | null; newestMarketDate?: string | null; reasons?: string[] | null;
}
export interface DisplayValuation { total?: DisplayMoney | null; knownSubtotal?: DisplayMoney | null; quality: DisplayQuality }
export function hasDisplay(value?: DisplayValuation | null): value is DisplayValuation {
  return Boolean(value && ["full", "partial", "none"].includes(value.quality?.availability));
}
// Coverage is verified, provisional, or unavailable. Carried prices do not
// change coverage, so a coverage check alone cannot decide an estimate.
export function displayQualityEstimated(quality?: Pick<DisplayQuality, "coverage" | "carriedCount"> | null): boolean {
  if (!quality) return false;
  return (quality.carriedCount ?? 0) > 0 || quality.coverage === "provisional" || quality.coverage === "unavailable";
}
export function fullDisplayAmount(value: DisplayValuation | null | undefined, legacy?: string | null): string | null {
  return hasDisplay(value) ? value.total?.amount ?? null : legacy ?? null;
}
export function valuationLabel(t: TFunction, value?: DisplayValuation | null, current = false): string {
  if (!hasDisplay(value)) return "";
  if (value.quality.availability !== "full") return t("valuation.knownSubtotal");
  if (current) return t("valuation.currentEstimate");
  return t(displayQualityEstimated(value.quality) ? "valuation.estimated" : "valuation.verified");
}
export function valuationDetails(t: TFunction, value?: DisplayValuation | null, current = false): string {
  if (!hasDisplay(value)) return "";
  const q = value.quality;
  return [valuationLabel(t, value, current), q.carriedCount > 0 ? `${t(q.reasons?.includes("carried_nav") ? "valuation.latestNAV" : "valuation.latestMarketValue")} · ${t("valuation.awaitingUpdate")} (${q.carriedCount})` : "",
    q.unpricedCount > 0 ? t("valuation.unpriced", { count: q.unpricedCount }) : "",
    q.oldestMarketDate ? `${t("quoteDetails.effectiveDate")}: ${q.oldestMarketDate}${q.newestMarketDate && q.newestMarketDate !== q.oldestMarketDate ? ` – ${q.newestMarketDate}` : ""}` : ""].filter(Boolean).join(" · ");
}
export function valuationTableAmount(t: TFunction, value: DisplayValuation | null | undefined, legacy?: DisplayMoney | null, net = false, includeSubtotalLabel = true): string {
  if (!hasDisplay(value)) return legacy ? formatAmount(legacy.amount, legacy.currency) : t("accounts.noValue");
  const money = value.total ?? value.knownSubtotal;
  if (!money) return t("accounts.noValue");
  return `${formatAmount(money.amount, money.currency)}${value.total || !includeSubtotalLabel ? "" : ` · ${t(net ? "valuation.knownNetContribution" : "valuation.knownSubtotal")}`}`;
}
