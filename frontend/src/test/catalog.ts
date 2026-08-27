import type { CatalogDTO } from "@/queries/catalog";

/** TEST_CATALOG mirrors the Wails catalog DTO with a distinctive subset of
 * production values. Page tests assert that forms render exactly these
 * options so a leftover hardcoded array cannot hide behind the mock. */
export const TEST_CATALOG: CatalogDTO = {
  currencies: ["USD", "SGD", "CNY"],
  instrumentTypes: ["stock", "etf"],
  quoteSources: ["manual", "provider"],
  primaryCategories: ["cash_equivalent", "investment", "property", "receivable", "liability"],
  secondaryCategoriesByPrimary: {
    cash_equivalent: ["cash", "bank_account", "digital_wallet", "broker_cash", "other_cash_equivalent"],
    investment: ["brokerage_account", "investment_fund_account", "bank_investment_product", "insurance", "manual_investment", "other_investment"],
    property: ["real_estate", "vehicle", "collectible", "other_property"],
    receivable: ["loan_receivable", "other_receivable"],
    liability: ["credit_card", "mortgage", "auto_loan", "consumer_loan", "personal_debt", "other_liability"],
  },
  trackingModesByPrimary: {
    cash_equivalent: ["balance"],
    investment: ["manual_value", "holdings"],
    property: ["manual_value"],
    receivable: ["manual_value"],
    liability: ["balance"],
  },
  trendRanges: ["30d", "1y"],
  appearances: ["system", "light", "dark"],
  languages: ["system", "en", "zh-CN", "zh-TW"],
  accents: ["nestworth", "ocean", "forest", "amber", "rose"],
  moneyInReasons: ["income", "contribution", "gift", "other", "reconciliation"],
  moneyOutReasons: ["expense", "fee", "tax", "other", "reconciliation"],
  valueUpdateReasons: ["reconciliation", "other"],
  tradeSides: ["buy", "sell"],
};

export function selectValues(element: HTMLElement): string[] {
  return Array.from(element.querySelectorAll("option")).map((option) => (option as HTMLOptionElement).value);
}
