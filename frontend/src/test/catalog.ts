import type { CatalogDTO } from "@/queries/catalog";

/** TEST_CATALOG mirrors the Wails catalog DTO with a distinctive subset of
 * production values. Page tests assert that forms render exactly these
 * options so a leftover hardcoded array cannot hide behind the mock. */
export const TEST_CATALOG: CatalogDTO = {
  currencies: ["USD", "SGD", "CNY"],
  instrumentTypes: ["stock", "etf"],
  institutionTypes: ["bank", "brokerage", "insurer", "exchange", "employer", "government", "other"],
  quoteSources: ["manual", "provider"],
  instrumentProviders: ["yahoo_finance"],
  accountTypes: ["cash_on_hand", "bank_account", "brokerage", "credit_card", "other"],
  balanceSheetRoles: ["asset", "liability"],
  trackingModes: ["balance", "manual_value", "holdings"],
  accountCombinations: [
    { accountType: "cash_on_hand", balanceSheetRole: "asset", trackingMode: "balance", roleLocked: true, includeInNetWorth: true, includeInPortfolio: false, includeInLiquidAssets: true, wholeAccountWarning: false },
    { accountType: "bank_account", balanceSheetRole: "asset", trackingMode: "balance", roleLocked: true, includeInNetWorth: true, includeInPortfolio: false, includeInLiquidAssets: true, wholeAccountWarning: false },
    { accountType: "bank_account", balanceSheetRole: "asset", trackingMode: "holdings", roleLocked: true, includeInNetWorth: true, includeInPortfolio: false, includeInLiquidAssets: false, wholeAccountWarning: true },
    { accountType: "brokerage", balanceSheetRole: "asset", trackingMode: "holdings", roleLocked: true, includeInNetWorth: true, includeInPortfolio: true, includeInLiquidAssets: false, wholeAccountWarning: true },
    { accountType: "brokerage", balanceSheetRole: "asset", trackingMode: "manual_value", roleLocked: true, includeInNetWorth: true, includeInPortfolio: true, includeInLiquidAssets: false, wholeAccountWarning: false },
    { accountType: "credit_card", balanceSheetRole: "liability", trackingMode: "balance", roleLocked: true, includeInNetWorth: true, includeInPortfolio: false, includeInLiquidAssets: false, wholeAccountWarning: false },
    { accountType: "other", balanceSheetRole: "asset", trackingMode: "balance", roleLocked: false, includeInNetWorth: true, includeInPortfolio: false, includeInLiquidAssets: false, wholeAccountWarning: false },
    { accountType: "other", balanceSheetRole: "liability", trackingMode: "balance", roleLocked: false, includeInNetWorth: true, includeInPortfolio: false, includeInLiquidAssets: false, wholeAccountWarning: false },
  ],
  trackingModesByAccountType: {
    cash_on_hand: ["balance"],
    bank_account: ["balance", "holdings"],
    brokerage: ["holdings", "manual_value"],
    credit_card: ["balance"],
    other: ["balance"],
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
