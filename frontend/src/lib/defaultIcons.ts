export const DEFAULT_ICONS = {
  member: "user",
  group: "folder",
  institution: "bank",
  account: "account",
  instrument: "investment",
} as const;

export const ACCOUNT_TYPE_ICONS: Record<string, string> = {
  cash_on_hand: "cash", bank_account: "bank", brokerage: "brokerage", investment_account: "investment",
  crypto_exchange: "bitcoin", digital_wallet: "wallet-cards", pension: "pension",
  insurance_policy: "shield-plus", property: "home", vehicle: "car", collectible: "gem",
  receivable: "receivable", credit_card: "credit-card", loan: "banknote-down", other: "account",
};

export const INSTRUMENT_TYPE_ICONS: Record<string, string> = {
  stock: "stock", etf: "pie-chart", mutual_fund: "chart", crypto: "bitcoin", bond: "document",
  precious_metal: "gem", bank_investment_product: "bank", other: "investment",
};

export const INSTITUTION_TYPE_ICONS: Record<string, string> = {
  bank: "bank", brokerage: "brokerage", insurer: "shield-plus", exchange: "market",
  employer: "building", government: "landmark", other: "building",
};
