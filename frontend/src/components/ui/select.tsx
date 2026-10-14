import * as React from "react";
import { cn } from "@/lib/utils";

export const selectFieldClass = "h-9 min-h-9 min-w-0 rounded-xl border border-border bg-card px-3 text-sm text-foreground shadow-xs transition-[border-color,box-shadow] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50 focus-visible:border-primary/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive";

export type NativeSelectProps = React.SelectHTMLAttributes<HTMLSelectElement>;

const NativeSelect = React.forwardRef<HTMLSelectElement, NativeSelectProps>(({ className, children, ...props }, ref) => (
  <select
    ref={ref}
    className={cn(
      selectFieldClass, "shrink-0",
      className,
    )}
    {...props}
  >
    {children}
  </select>
));
NativeSelect.displayName = "NativeSelect";

export { NativeSelect };
