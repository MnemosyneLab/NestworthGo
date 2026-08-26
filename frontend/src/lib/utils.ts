import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

/** Merges Tailwind class lists, resolving conflicting utility classes the
 * way shadcn/ui's own `cn()` helper does. Every UI component in
 * components/ui uses this instead of string concatenation. */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
