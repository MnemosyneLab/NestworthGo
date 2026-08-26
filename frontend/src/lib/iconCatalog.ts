/**
 * ICON_CATALOG is the personal-finance icon set keyed by the stored icon_key
 * values so existing Household data keeps rendering consistently.
 * labelKey looks up the already-ported `icons.*` i18next catalog.
 */
export interface IconChoice {
  key: string;
  labelKey: string;
  categoryKey: string;
}

export const ICON_CATALOG: IconChoice[] = [
  { key: "bank", labelKey: "icons.bank", categoryKey: "icons.category.banking" },
  { key: "bank-branch", labelKey: "icons.bankBranch", categoryKey: "icons.category.banking" },
  { key: "building", labelKey: "icons.building", categoryKey: "icons.category.banking" },
  { key: "vault", labelKey: "icons.vault", categoryKey: "icons.category.banking" },
  { key: "storage", labelKey: "icons.storage", categoryKey: "icons.category.banking" },
  { key: "account", labelKey: "icons.account", categoryKey: "icons.category.accounts" },
  { key: "wallet", labelKey: "icons.wallet", categoryKey: "icons.category.accounts" },
  { key: "wallet-cards", labelKey: "icons.walletCards", categoryKey: "icons.category.accounts" },
  { key: "cash", labelKey: "icons.cash", categoryKey: "icons.category.accounts" },
  { key: "coins", labelKey: "icons.coins", categoryKey: "icons.category.accounts" },
  { key: "money", labelKey: "icons.money", categoryKey: "icons.category.accounts" },
  { key: "card", labelKey: "icons.card", categoryKey: "icons.category.accounts" },
  { key: "credit-card", labelKey: "icons.creditCard", categoryKey: "icons.category.accounts" },
  { key: "receipt", labelKey: "icons.receipt", categoryKey: "icons.category.accounts" },
  { key: "banknote-up", labelKey: "icons.banknoteUp", categoryKey: "icons.category.accounts" },
  { key: "banknote-down", labelKey: "icons.banknoteDown", categoryKey: "icons.category.accounts" },
  { key: "brokerage", labelKey: "icons.brokerage", categoryKey: "icons.category.investment" },
  { key: "brokerage-cash", labelKey: "icons.brokerageCash", categoryKey: "icons.category.investment" },
  { key: "investment", labelKey: "icons.investment", categoryKey: "icons.category.investment" },
  { key: "stock", labelKey: "icons.stock", categoryKey: "icons.category.investment" },
  { key: "market", labelKey: "icons.market", categoryKey: "icons.category.investment" },
  { key: "chart", labelKey: "icons.chart", categoryKey: "icons.category.investment" },
  { key: "trending-up", labelKey: "icons.trendingUp", categoryKey: "icons.category.investment" },
  { key: "percent", labelKey: "icons.percent", categoryKey: "icons.category.investment" },
  { key: "badge-percent", labelKey: "icons.badgePercent", categoryKey: "icons.category.investment" },
  { key: "circle-percent", labelKey: "icons.circlePercent", categoryKey: "icons.category.investment" },
  { key: "currency", labelKey: "icons.currency", categoryKey: "icons.category.currency" },
  { key: "dollar", labelKey: "icons.dollar", categoryKey: "icons.category.currency" },
  { key: "badge-dollar", labelKey: "icons.badgeDollar", categoryKey: "icons.category.currency" },
  { key: "circle-dollar", labelKey: "icons.circleDollar", categoryKey: "icons.category.currency" },
  { key: "euro", labelKey: "icons.euro", categoryKey: "icons.category.currency" },
  { key: "badge-euro", labelKey: "icons.badgeEuro", categoryKey: "icons.category.currency" },
  { key: "yen", labelKey: "icons.yen", categoryKey: "icons.category.currency" },
  { key: "badge-yen", labelKey: "icons.badgeYen", categoryKey: "icons.category.currency" },
  { key: "pound", labelKey: "icons.pound", categoryKey: "icons.category.currency" },
  { key: "badge-pound", labelKey: "icons.badgePound", categoryKey: "icons.category.currency" },
  { key: "badge-rupee", labelKey: "icons.badgeRupee", categoryKey: "icons.category.currency" },
  { key: "badge-ruble", labelKey: "icons.badgeRuble", categoryKey: "icons.category.currency" },
  { key: "badge-franc", labelKey: "icons.badgeFranc", categoryKey: "icons.category.currency" },
  { key: "bitcoin", labelKey: "icons.bitcoin", categoryKey: "icons.category.currency" },
  { key: "retirement", labelKey: "icons.retirement", categoryKey: "icons.category.protection" },
  { key: "pension", labelKey: "icons.pension", categoryKey: "icons.category.protection" },
  { key: "savings", labelKey: "icons.savings", categoryKey: "icons.category.protection" },
  { key: "insurance", labelKey: "icons.insurance", categoryKey: "icons.category.protection" },
  { key: "shield", labelKey: "icons.shield", categoryKey: "icons.category.protection" },
  { key: "shield-plus", labelKey: "icons.shieldPlus", categoryKey: "icons.category.protection" },
  { key: "goal", labelKey: "icons.goal", categoryKey: "icons.category.protection" },
  { key: "armchair", labelKey: "icons.armchair", categoryKey: "icons.category.protection" },
  { key: "circle-check", labelKey: "icons.circleCheck", categoryKey: "icons.category.protection" },
  { key: "calendar", labelKey: "icons.calendar", categoryKey: "icons.category.general" },
  { key: "calendar-clock", labelKey: "icons.calendarClock", categoryKey: "icons.category.general" },
  { key: "clock", labelKey: "icons.clock", categoryKey: "icons.category.general" },
  { key: "property", labelKey: "icons.property", categoryKey: "icons.category.general" },
  { key: "liability", labelKey: "icons.liability", categoryKey: "icons.category.general" },
  { key: "receivable", labelKey: "icons.receivable", categoryKey: "icons.category.general" },
  { key: "home", labelKey: "icons.home", categoryKey: "icons.category.general" },
  { key: "folder", labelKey: "icons.folder", categoryKey: "icons.category.general" },
  { key: "document", labelKey: "icons.document", categoryKey: "icons.category.general" },
  { key: "file", labelKey: "icons.file", categoryKey: "icons.category.general" },
  { key: "search", labelKey: "icons.search", categoryKey: "icons.category.general" },
  { key: "settings", labelKey: "icons.settings", categoryKey: "icons.category.general" },
  { key: "warning", labelKey: "icons.warning", categoryKey: "icons.category.general" },
  { key: "info", labelKey: "icons.info", categoryKey: "icons.category.general" },
  { key: "download", labelKey: "icons.download", categoryKey: "icons.category.general" },
  { key: "upload", labelKey: "icons.upload", categoryKey: "icons.category.general" },
  { key: "visibility", labelKey: "icons.visibility", categoryKey: "icons.category.general" },
  { key: "mail", labelKey: "icons.mail", categoryKey: "icons.category.general" },
  { key: "media", labelKey: "icons.media", categoryKey: "icons.category.general" },
  { key: "camera", labelKey: "icons.camera", categoryKey: "icons.category.general" },
  { key: "computer", labelKey: "icons.computer", categoryKey: "icons.category.general" },
  { key: "grid", labelKey: "icons.grid", categoryKey: "icons.category.general" },
  { key: "list", labelKey: "icons.list", categoryKey: "icons.category.general" },
  { key: "history", labelKey: "icons.history", categoryKey: "icons.category.general" },
];

export function iconChoicesByCategory(): { categoryKey: string; choices: IconChoice[] }[] {
  const groups: { categoryKey: string; choices: IconChoice[] }[] = [];
  for (const choice of ICON_CATALOG) {
    const last = groups[groups.length - 1];
    if (last && last.categoryKey === choice.categoryKey) {
      last.choices.push(choice);
    } else {
      groups.push({ categoryKey: choice.categoryKey, choices: [choice] });
    }
  }
  return groups;
}
