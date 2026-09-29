import * as React from "react";
import { cn } from "@/lib/utils";

/** Inline validation message shown directly under the field it belongs to. */
export function FieldError({ id, children, className }: { id?: string; children?: React.ReactNode; className?: string }) {
  if (!children) return null;
  return (
    <p id={id} role="alert" className={cn("text-xs font-medium text-destructive", className)}>
      {children}
    </p>
  );
}
