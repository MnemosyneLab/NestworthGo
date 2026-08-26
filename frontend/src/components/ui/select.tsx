import * as React from "react";
import { cn } from "@/lib/utils";

export type NativeSelectProps = React.SelectHTMLAttributes<HTMLSelectElement>;

const NativeSelect = React.forwardRef<HTMLSelectElement, NativeSelectProps>(({ className, children, ...props }, ref) => (
  <select
    ref={ref}
    className={cn("h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground", className)}
    {...props}
  >
    {children}
  </select>
));
NativeSelect.displayName = "NativeSelect";

export { NativeSelect };
