import { cn } from "@/lib/utils";
import { TONE_CLASSES, type Tone } from "@/lib/tone";

/** Thin proportion bar. `shareBps` is basis points (10000 = 100%). Decorative: the value is always printed elsewhere. */
export function ShareBar({ shareBps, tone = 1, className }: { shareBps: number; tone?: Tone; className?: string }) {
  const width = Math.max(0, Math.min(100, shareBps / 100));
  return (
    <span aria-hidden="true" className={cn("block h-1.5 w-full overflow-hidden rounded-full bg-muted", className)}>
      <span className={cn("block h-full rounded-full", TONE_CLASSES[tone].solid)} style={{ width: `${width}%` }} />
    </span>
  );
}
