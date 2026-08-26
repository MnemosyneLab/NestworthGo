/**
 * Client-side mirror of domain.PrimaryCategory/SecondaryCategory/
 * TrackingMode's allowed combinations (internal/domain/model.go), used
 * only to drive form UX (disable invalid choices, pick sane defaults).
 * The Go backend remains the sole authority: every combination is
 * re-validated there regardless of what this module allows through. This
 * mirror is validation guidance only; the Go backend remains authoritative.
 */
export type PrimaryCategory = "cash_equivalent" | "investment" | "property" | "receivable" | "liability";
export type TrackingMode = "balance" | "manual_value" | "holdings";

export const PRIMARY_CATEGORIES: PrimaryCategory[] = ["cash_equivalent", "investment", "property", "receivable", "liability"];

export const SECONDARY_CATEGORIES_BY_PRIMARY: Record<PrimaryCategory, string[]> = {
  cash_equivalent: ["cash", "bank_account", "digital_wallet", "broker_cash", "other_cash_equivalent"],
  investment: ["brokerage_account", "investment_fund_account", "bank_investment_product", "insurance", "manual_investment", "other_investment"],
  property: ["real_estate", "vehicle", "collectible", "other_property"],
  receivable: ["loan_receivable", "other_receivable"],
  liability: ["credit_card", "mortgage", "auto_loan", "consumer_loan", "personal_debt", "other_liability"],
};

export const TRACKING_MODES_BY_PRIMARY: Record<PrimaryCategory, TrackingMode[]> = {
  cash_equivalent: ["balance"],
  investment: ["holdings", "manual_value"],
  property: ["manual_value"],
  receivable: ["manual_value"],
  liability: ["balance"],
};

export function defaultTrackingMode(primary: PrimaryCategory): TrackingMode {
  return TRACKING_MODES_BY_PRIMARY[primary][0];
}

export function defaultSecondaryCategory(primary: PrimaryCategory): string {
  return SECONDARY_CATEGORIES_BY_PRIMARY[primary][0];
}
