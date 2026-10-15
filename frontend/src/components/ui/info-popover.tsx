import type { ReactNode } from "react";
import { Info } from "lucide-react";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { cn } from "@/lib/utils";

/** Small info icon that reveals explanatory text on click, tap or keyboard focus. */
export function InfoPopover({ label, children, className }: { label: string; children: ReactNode; className?: string }) {
  return (
    <Popover>
      <PopoverTrigger
        type="button"
        aria-label={label}
        className={cn("inline-flex size-6 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50", className)}
      >
        <Info className="size-3.5" aria-hidden="true" />
      </PopoverTrigger>
      <PopoverContent className="max-w-xs p-3 text-xs leading-relaxed text-muted-foreground">{children}</PopoverContent>
    </Popover>
  );
}
