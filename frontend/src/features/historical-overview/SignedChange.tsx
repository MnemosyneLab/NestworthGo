import { ChangeText, type ChangeDirection } from "@/components/ui/change-text";
import { compareCanonical } from "@/lib/money";
import { cn } from "@/lib/utils";
import { formatHistoricalAmount } from "./historicalAmount";

const DECIMAL = /^-?(?:0|[1-9]\d*)(?:\.\d+)?$/;

/** Direction of an exact signed change. Amounts that display as zero stay flat. */
export function changeDirection(value: string | null | undefined): ChangeDirection | null {
  if (value == null || !DECIMAL.test(value)) return null;
  if (compareCanonical(value, "0.005") > 0) return "up";
  if (compareCanonical(value, "-0.005") < 0) return "down";
  return "flat";
}

/**
 * Signed balance change with the shared gain/loss cue. Debt rows pass `neutral`:
 * a higher debt balance is a balance change, not a return, so it gets no gain/loss color.
 */
export function SignedChange({ value, currency, neutral = false, className }: {
  value: string | null | undefined; currency: string; neutral?: boolean; className?: string;
}) {
  const text = formatHistoricalAmount(value, currency, true);
  const direction = changeDirection(value);
  if (text == null || direction == null) return null;
  if (neutral) return <span className={cn("num whitespace-nowrap", className)} title={text}>{text}</span>;
  return <ChangeText direction={direction} className={cn("num whitespace-nowrap", className)}><span className="whitespace-nowrap" title={text}>{text}</span></ChangeText>;
}
