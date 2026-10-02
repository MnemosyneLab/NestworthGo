import type { LucideIcon } from "lucide-react";
import {
  Amphora, Archive, Armchair, BadgeDollarSign, BadgeEuro, BadgeIndianRupee, BadgeJapaneseYen,
  BadgePercent, BadgePoundSterling, BadgeRussianRuble, BadgeSwissFranc, Banknote, BanknoteArrowDown,
  BanknoteArrowUp, Bitcoin, Blocks, Building2, Calendar, CalendarClock, Camera, Car,
  ChartNoAxesCombined, CircleCheck, CircleDollarSign, CirclePercent, Coins, CreditCard, Download,
  Euro, Eye, FileText, Folder, Gem, HandCoins, History, House, Info, JapaneseYen, LandPlot,
  Landmark, Layers, LayoutGrid, List, Mail, Monitor, Percent, PieChart, ReceiptText, ScrollText,
  Search, Settings, Shield, ShieldPlus, Store, Target, Timer, TrendingUp, TriangleAlert, Upload,
  UserRound, Vault, Wallet, WalletCards,
} from "lucide-react";

type IconMetadata = { key: string; labelKey: string; categoryKey: string };
export type IconChoice = IconMetadata & (
  { icon: LucideIcon; file?: never } | { file: string; icon?: never }
);

/** One registry owns choices and renderers. Stored IDs never depend on labels,
 * categories, symbols or filenames. Brand assets require explicit selection. */
export const ICON_CATALOG: IconChoice[] = [
  { key: "account", labelKey: "icons.account", categoryKey: "icons.category.accounts", icon: Wallet },
  { key: "wallet", labelKey: "icons.wallet", categoryKey: "icons.category.accounts", icon: Wallet },
  { key: "wallet-cards", labelKey: "icons.walletCards", categoryKey: "icons.category.accounts", icon: WalletCards },
  { key: "cash", labelKey: "icons.cash", categoryKey: "icons.category.accounts", icon: Banknote },
  { key: "coins", labelKey: "icons.coins", categoryKey: "icons.category.accounts", icon: Coins },
  { key: "money", labelKey: "icons.money", categoryKey: "icons.category.accounts", icon: Banknote },
  { key: "card", labelKey: "icons.card", categoryKey: "icons.category.accounts", icon: CreditCard },
  { key: "receipt", labelKey: "icons.receipt", categoryKey: "icons.category.accounts", icon: ReceiptText },
  { key: "banknote-up", labelKey: "icons.banknoteUp", categoryKey: "icons.category.accounts", icon: BanknoteArrowUp },
  { key: "term-deposit", labelKey: "icons.termDeposit", categoryKey: "icons.category.accounts", icon: Timer },
  { key: "bank", labelKey: "icons.bank", categoryKey: "icons.category.banking", icon: Landmark },
  { key: "bank-branch", labelKey: "icons.bankBranch", categoryKey: "icons.category.banking", icon: Landmark },
  { key: "building", labelKey: "icons.building", categoryKey: "icons.category.banking", icon: Building2 },
  { key: "vault", labelKey: "icons.vault", categoryKey: "icons.category.banking", icon: Vault },
  { key: "storage", labelKey: "icons.storage", categoryKey: "icons.category.banking", icon: Archive },
  { key: "brokerage", labelKey: "icons.brokerage", categoryKey: "icons.category.investment", icon: ChartNoAxesCombined },
  { key: "brokerage-cash", labelKey: "icons.brokerageCash", categoryKey: "icons.category.investment", icon: Banknote },
  { key: "investment", labelKey: "icons.investment", categoryKey: "icons.category.investment", icon: TrendingUp },
  { key: "stock", labelKey: "icons.stock", categoryKey: "icons.category.investment", icon: TrendingUp },
  { key: "market", labelKey: "icons.market", categoryKey: "icons.category.investment", icon: ChartNoAxesCombined },
  { key: "chart", labelKey: "icons.chart", categoryKey: "icons.category.investment", icon: ChartNoAxesCombined },
  { key: "trending-up", labelKey: "icons.trendingUp", categoryKey: "icons.category.investment", icon: TrendingUp },
  { key: "percent", labelKey: "icons.percent", categoryKey: "icons.category.investment", icon: Percent },
  { key: "badge-percent", labelKey: "icons.badgePercent", categoryKey: "icons.category.investment", icon: BadgePercent },
  { key: "circle-percent", labelKey: "icons.circlePercent", categoryKey: "icons.category.investment", icon: CirclePercent },
  { key: "fund", labelKey: "icons.fund", categoryKey: "icons.category.investment", icon: Layers },
  { key: "bond", labelKey: "icons.bond", categoryKey: "icons.category.investment", icon: ScrollText },
  { key: "car", labelKey: "icons.car", categoryKey: "icons.category.assets", icon: Car },
  { key: "gem", labelKey: "icons.gem", categoryKey: "icons.category.assets", icon: Gem },
  { key: "property", labelKey: "icons.property", categoryKey: "icons.category.assets", icon: House },
  { key: "home", labelKey: "icons.home", categoryKey: "icons.category.assets", icon: House },
  { key: "land", labelKey: "icons.land", categoryKey: "icons.category.assets", icon: LandPlot },
  { key: "commercial-property", labelKey: "icons.commercialProperty", categoryKey: "icons.category.assets", icon: Store },
  { key: "collectible", labelKey: "icons.collectible", categoryKey: "icons.category.assets", icon: Amphora },
  { key: "credit-card", labelKey: "icons.creditCard", categoryKey: "icons.category.liabilities", icon: CreditCard },
  { key: "banknote-down", labelKey: "icons.banknoteDown", categoryKey: "icons.category.liabilities", icon: BanknoteArrowDown },
  { key: "liability", labelKey: "icons.liability", categoryKey: "icons.category.liabilities", icon: Banknote },
  { key: "receivable", labelKey: "icons.receivable", categoryKey: "icons.category.liabilities", icon: ReceiptText },
  { key: "lending", labelKey: "icons.lending", categoryKey: "icons.category.liabilities", icon: HandCoins },
  { key: "retirement", labelKey: "icons.retirement", categoryKey: "icons.category.protection", icon: Armchair },
  { key: "pension", labelKey: "icons.pension", categoryKey: "icons.category.protection", icon: Armchair },
  { key: "savings", labelKey: "icons.savings", categoryKey: "icons.category.protection", icon: Coins },
  { key: "insurance", labelKey: "icons.insurance", categoryKey: "icons.category.protection", icon: Shield },
  { key: "shield", labelKey: "icons.shield", categoryKey: "icons.category.protection", icon: Shield },
  { key: "shield-plus", labelKey: "icons.shieldPlus", categoryKey: "icons.category.protection", icon: ShieldPlus },
  { key: "goal", labelKey: "icons.goal", categoryKey: "icons.category.protection", icon: Target },
  { key: "armchair", labelKey: "icons.armchair", categoryKey: "icons.category.protection", icon: Armchair },
  { key: "circle-check", labelKey: "icons.circleCheck", categoryKey: "icons.category.protection", icon: CircleCheck },
  { key: "bitcoin", labelKey: "icons.bitcoin", categoryKey: "icons.category.digital", icon: Bitcoin },
  { key: "digital-asset", labelKey: "icons.digitalAsset", categoryKey: "icons.category.digital", icon: Blocks },
  { key: "crypto-logo:btc", labelKey: "icons.cryptoBTC", categoryKey: "icons.category.digital", file: "crypto-logos/btc.svg" },
  { key: "crypto-logo:eth", labelKey: "icons.cryptoETH", categoryKey: "icons.category.digital", file: "crypto-logos/eth.svg" },
  { key: "crypto-logo:sol", labelKey: "icons.cryptoSOL", categoryKey: "icons.category.digital", file: "crypto-logos/sol.svg" },
  { key: "crypto-logo:usdc", labelKey: "icons.cryptoUSDC", categoryKey: "icons.category.digital", file: "crypto-logos/usdc.svg" },
  { key: "currency", labelKey: "icons.currency", categoryKey: "icons.category.currency", icon: CircleDollarSign },
  { key: "dollar", labelKey: "icons.dollar", categoryKey: "icons.category.currency", icon: CircleDollarSign },
  { key: "badge-dollar", labelKey: "icons.badgeDollar", categoryKey: "icons.category.currency", icon: BadgeDollarSign },
  { key: "circle-dollar", labelKey: "icons.circleDollar", categoryKey: "icons.category.currency", icon: CircleDollarSign },
  { key: "euro", labelKey: "icons.euro", categoryKey: "icons.category.currency", icon: Euro },
  { key: "badge-euro", labelKey: "icons.badgeEuro", categoryKey: "icons.category.currency", icon: BadgeEuro },
  { key: "yen", labelKey: "icons.yen", categoryKey: "icons.category.currency", icon: JapaneseYen },
  { key: "badge-yen", labelKey: "icons.badgeYen", categoryKey: "icons.category.currency", icon: BadgeJapaneseYen },
  { key: "badge-pound", labelKey: "icons.badgePound", categoryKey: "icons.category.currency", icon: BadgePoundSterling },
  { key: "badge-rupee", labelKey: "icons.badgeRupee", categoryKey: "icons.category.currency", icon: BadgeIndianRupee },
  { key: "badge-ruble", labelKey: "icons.badgeRuble", categoryKey: "icons.category.currency", icon: BadgeRussianRuble },
  { key: "badge-franc", labelKey: "icons.badgeFranc", categoryKey: "icons.category.currency", icon: BadgeSwissFranc },
  { key: "calendar", labelKey: "icons.calendar", categoryKey: "icons.category.general", icon: Calendar },
  { key: "calendar-clock", labelKey: "icons.calendarClock", categoryKey: "icons.category.general", icon: CalendarClock },
  { key: "user", labelKey: "icons.user", categoryKey: "icons.category.general", icon: UserRound },
  { key: "landmark", labelKey: "icons.landmark", categoryKey: "icons.category.general", icon: Landmark },
  { key: "pie-chart", labelKey: "icons.pieChart", categoryKey: "icons.category.general", icon: PieChart },
  { key: "folder", labelKey: "icons.folder", categoryKey: "icons.category.general", icon: Folder },
  { key: "document", labelKey: "icons.document", categoryKey: "icons.category.general", icon: FileText },
  { key: "file", labelKey: "icons.file", categoryKey: "icons.category.general", icon: FileText },
  { key: "search", labelKey: "icons.search", categoryKey: "icons.category.general", icon: Search },
  { key: "settings", labelKey: "icons.settings", categoryKey: "icons.category.general", icon: Settings },
  { key: "warning", labelKey: "icons.warning", categoryKey: "icons.category.general", icon: TriangleAlert },
  { key: "info", labelKey: "icons.info", categoryKey: "icons.category.general", icon: Info },
  { key: "download", labelKey: "icons.download", categoryKey: "icons.category.general", icon: Download },
  { key: "upload", labelKey: "icons.upload", categoryKey: "icons.category.general", icon: Upload },
  { key: "visibility", labelKey: "icons.visibility", categoryKey: "icons.category.general", icon: Eye },
  { key: "mail", labelKey: "icons.mail", categoryKey: "icons.category.general", icon: Mail },
  { key: "camera", labelKey: "icons.camera", categoryKey: "icons.category.general", icon: Camera },
  { key: "computer", labelKey: "icons.computer", categoryKey: "icons.category.general", icon: Monitor },
  { key: "grid", labelKey: "icons.grid", categoryKey: "icons.category.general", icon: LayoutGrid },
  { key: "list", labelKey: "icons.list", categoryKey: "icons.category.general", icon: List },
  { key: "history", labelKey: "icons.history", categoryKey: "icons.category.general", icon: History },
];
const byKey = new Map(ICON_CATALOG.map(choice => [choice.key, choice]));
export function iconChoice(key?: string | null) { return byKey.get(key ?? ""); }

export function iconChoicesByCategory(): { categoryKey: string; choices: IconChoice[] }[] {
  const groups = new Map<string, IconChoice[]>();
  for (const choice of ICON_CATALOG) {
    const group = groups.get(choice.categoryKey) ?? [];
    group.push(choice);
    groups.set(choice.categoryKey, group);
  }
  return [...groups].map(([categoryKey, choices]) => ({ categoryKey, choices }));
}
