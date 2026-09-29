import type { LucideIcon } from "lucide-react";
import {
  Landmark,
  ChartCandlestick,
  Bitcoin,
  Wallet,
  House,
  Car,
  CreditCard,
  HandCoins,
  PiggyBank,
  ShieldCheck,
  Gem,
  Receipt,
  Banknote,
  CircleDollarSign,
} from "lucide-react";

/** The 8 category hues defined as --color-cat-1..8 in index.css. */
export type Tone = 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8;

/** Tailwind needs literal class names, so every tone spells them out. */
export const TONE_CLASSES: Record<Tone, { soft: string; text: string; solid: string; dot: string }> = {
  1: { soft: "bg-cat-1-soft", text: "text-cat-1", solid: "bg-cat-1", dot: "bg-cat-1" },
  2: { soft: "bg-cat-2-soft", text: "text-cat-2", solid: "bg-cat-2", dot: "bg-cat-2" },
  3: { soft: "bg-cat-3-soft", text: "text-cat-3", solid: "bg-cat-3", dot: "bg-cat-3" },
  4: { soft: "bg-cat-4-soft", text: "text-cat-4", solid: "bg-cat-4", dot: "bg-cat-4" },
  5: { soft: "bg-cat-5-soft", text: "text-cat-5", solid: "bg-cat-5", dot: "bg-cat-5" },
  6: { soft: "bg-cat-6-soft", text: "text-cat-6", solid: "bg-cat-6", dot: "bg-cat-6" },
  7: { soft: "bg-cat-7-soft", text: "text-cat-7", solid: "bg-cat-7", dot: "bg-cat-7" },
  8: { soft: "bg-cat-8-soft", text: "text-cat-8", solid: "bg-cat-8", dot: "bg-cat-8" },
};

/** Stable tone for an arbitrary key (institution id, member id, ...). */
export function toneForKey(key: string): Tone {
  let hash = 0;
  for (let index = 0; index < key.length; index += 1) {
    hash = (hash * 31 + key.charCodeAt(index)) >>> 0;
  }
  return ((hash % 8) + 1) as Tone;
}

const ACCOUNT_TYPE_STYLE: Record<string, { tone: Tone; icon: LucideIcon }> = {
  bank_account: { tone: 1, icon: Landmark },
  cash_on_hand: { tone: 2, icon: Banknote },
  brokerage: { tone: 5, icon: ChartCandlestick },
  investment_account: { tone: 5, icon: ChartCandlestick },
  crypto_exchange: { tone: 6, icon: Bitcoin },
  digital_wallet: { tone: 8, icon: Wallet },
  property: { tone: 2, icon: House },
  vehicle: { tone: 4, icon: Car },
  credit_card: { tone: 3, icon: CreditCard },
  loan: { tone: 7, icon: HandCoins },
  pension: { tone: 2, icon: PiggyBank },
  insurance_policy: { tone: 8, icon: ShieldCheck },
  collectible: { tone: 7, icon: Gem },
  receivable: { tone: 4, icon: Receipt },
  other: { tone: 8, icon: CircleDollarSign },
};

export function accountTypeStyle(accountType: string | undefined): { tone: Tone; icon: LucideIcon } {
  return (accountType && ACCOUNT_TYPE_STYLE[accountType]) || ACCOUNT_TYPE_STYLE.other;
}
