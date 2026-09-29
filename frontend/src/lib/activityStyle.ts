import type { LucideIcon } from "lucide-react";
import { ArrowDownToLine, ArrowUpFromLine, Coins, ArrowLeftRight, RefreshCcw, PencilLine, TrendingUp, TrendingDown, Repeat, HandCoins, Receipt, CircleDot } from "lucide-react";
import type { Tone } from "@/lib/tone";

const ACTIVITY_STYLE: Record<string, { tone: Tone; icon: LucideIcon }> = {
  cash_in: { tone: 2, icon: ArrowDownToLine },
  cash_out: { tone: 3, icon: ArrowUpFromLine },
  cash_dividend: { tone: 4, icon: Coins },
  cash_transfer: { tone: 1, icon: ArrowLeftRight },
  fx_conversion: { tone: 8, icon: RefreshCcw },
  value_update: { tone: 5, icon: PencilLine },
  buy: { tone: 1, icon: TrendingUp },
  sell: { tone: 6, icon: TrendingDown },
  position_transfer: { tone: 5, icon: Repeat },
  debt_draw: { tone: 7, icon: HandCoins },
  debt_payment: { tone: 7, icon: Receipt },
};

/** Icon and hue used for an activity kind in timelines and recent-activity lists. */
export function activityStyle(kind: string | undefined): { tone: Tone; icon: LucideIcon } {
  return (kind && ACTIVITY_STYLE[kind]) || { tone: 8, icon: CircleDot };
}
