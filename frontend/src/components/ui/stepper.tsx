import { Check } from "lucide-react";
import { cn } from "@/lib/utils";

/** Compact numbered progress indicator for multi-step flows. `current` is zero-based. */
export function Stepper({ steps, current, label, showLabels = true }: { steps: readonly string[]; current: number; label: string; showLabels?: boolean }) {
  return (
    <ol className="flex items-center gap-2" aria-label={label}>
      {steps.map((step, index) => {
        const done = index < current;
        const active = index === current;
        return (
          <li key={step} aria-label={showLabels ? undefined : step} className={cn("flex min-w-0 items-center gap-2", index < steps.length - 1 && "flex-1")} aria-current={active ? "step" : undefined}>
            <span
              className={cn(
                "flex size-6 shrink-0 items-center justify-center rounded-full text-xs font-bold transition-colors",
                done && "bg-primary text-primary-foreground",
                active && "bg-primary/15 text-primary ring-2 ring-primary",
                !done && !active && "bg-muted text-muted-foreground",
              )}
            >
              {done ? <Check className="size-3.5" aria-hidden="true" /> : index + 1}
            </span>
            {showLabels && <span className={cn("hidden truncate text-xs font-medium sm:block", active ? "text-foreground" : "text-muted-foreground")}>{step}</span>}
            {index < steps.length - 1 && <span aria-hidden="true" className={cn("h-0.5 min-w-3 flex-1 rounded-full", done ? "bg-primary" : "bg-border")} />}
          </li>
        );
      })}
    </ol>
  );
}
