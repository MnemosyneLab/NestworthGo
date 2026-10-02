import type { LucideIcon } from "lucide-react";
import {
  Archive, Armchair, Banknote, Bitcoin, Building2, Calendar, Camera, Car, ChartNoAxesCombined,
  CircleCheck, CircleDollarSign, Coins, CreditCard, Download, Euro, Eye, FileText, Folder,
  Gem, History, House, Info, JapaneseYen, Landmark, LayoutGrid, List, Mail, Monitor,
  PieChart, PoundSterling, ReceiptText, Search, Settings, Shield, ShieldPlus, Target, TrendingUp,
  TriangleAlert, Upload, UserRound, Wallet,
} from "lucide-react";
import { bankLogo, bankLogoUrl } from "@/lib/bankLogos";
import { cn } from "@/lib/utils";

const ICONS: Record<string, LucideIcon> = {
  account: Wallet, armchair: Armchair, "badge-dollar": CircleDollarSign, "badge-euro": Euro,
  "badge-franc": CircleDollarSign, "badge-rupee": CircleDollarSign, "badge-pound": PoundSterling,
  "badge-percent": TrendingUp, "badge-ruble": CircleDollarSign, "badge-yen": JapaneseYen,
  bank: Landmark, "bank-branch": Landmark, "banknote-down": Banknote, "banknote-up": Banknote,
  bitcoin: Bitcoin, brokerage: ChartNoAxesCombined, "brokerage-cash": Banknote, building: Building2,
  calendar: Calendar, "calendar-clock": Calendar, camera: Camera, card: CreditCard, cash: Banknote,
  chart: ChartNoAxesCombined, "circle-check": CircleCheck, "circle-dollar": CircleDollarSign,
  "circle-percent": TrendingUp, coins: Coins, computer: Monitor, "credit-card": CreditCard,
  currency: CircleDollarSign, document: FileText, dollar: CircleDollarSign, download: Download,
  euro: Euro, file: FileText, folder: Folder, goal: Target, grid: LayoutGrid, history: History,
  home: House, info: Info, insurance: Shield, investment: TrendingUp, liability: Banknote,
  list: List, mail: Mail, market: ChartNoAxesCombined, money: Banknote,
  percent: TrendingUp, pension: Armchair, property: House, receivable: ReceiptText, receipt: ReceiptText,
  retirement: Armchair, savings: Coins, search: Search, settings: Settings, shield: Shield,
  "shield-plus": ShieldPlus, stock: TrendingUp, storage: Archive, "trending-up": TrendingUp,
  upload: Upload, vault: Archive, visibility: Eye, wallet: Wallet, "wallet-cards": CreditCard,
  warning: TriangleAlert, yen: JapaneseYen, user: UserRound, landmark: Landmark, car: Car, gem: Gem,
  "pie-chart": PieChart,
};

export type EntityIconKind = "member" | "institution" | "group" | "account" | "instrument";

const KIND_FALLBACKS: Record<EntityIconKind, string> = {
  member: "user", institution: "bank", group: "folder", account: "account", instrument: "investment",
};

export function EntityIcon({ iconKey, kind, className, label }: { iconKey?: string | null; kind: EntityIconKind; className?: string; label?: string }) {
  const logo = bankLogo(iconKey);
  if (logo) return <img src={bankLogoUrl(logo.file)} alt={label ?? ""} aria-hidden={label ? undefined : true}
    className={cn("size-4 shrink-0 rounded-sm bg-white object-contain p-0.5", className)} loading="lazy" draggable={false} />;
  const key = iconKey ?? "";
  const Icon = Object.prototype.hasOwnProperty.call(ICONS, key) ? ICONS[key] : ICONS[KIND_FALLBACKS[kind]];
  return <Icon className={cn("size-4 shrink-0", className)} aria-hidden={label ? undefined : "true"} aria-label={label} />;
}

export function hasIcon(key: string): boolean { return Object.prototype.hasOwnProperty.call(ICONS, key) || Boolean(bankLogo(key)); }
