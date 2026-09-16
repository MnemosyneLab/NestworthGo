import { cn } from "@/lib/utils";
import { instrumentDisplayLabel, instrumentSecondaryName, type InstrumentIdentityFields } from "@/lib/instrumentDisplay";

export function InstrumentLabel({
  name,
  symbol,
  fallback = "",
  className,
}: InstrumentIdentityFields & { fallback?: string; className?: string }) {
  const ticker = instrumentDisplayLabel({ name, symbol }, fallback);
  const subtitle = instrumentSecondaryName({ name, symbol });
  if (!ticker) {
    return null;
  }
  return (
    <span className={cn("flex min-w-0 flex-col", className)}>
      <span className="truncate font-medium">{ticker}</span>
      {subtitle ? <span className="truncate text-xs text-muted-foreground">{subtitle}</span> : null}
    </span>
  );
}
