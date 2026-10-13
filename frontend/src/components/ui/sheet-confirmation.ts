import { createContext, useContext } from "react";

export type SheetConfirmation = {
  identity: string | null;
  recovery: unknown;
  begin: (identity: string, recovery: unknown) => boolean;
  read: () => { identity: string; recovery: unknown } | null;
  update: (identity: string, recovery: unknown) => boolean;
  resolve: (identity?: string) => boolean;
};

export const SheetConfirmationContext = createContext<SheetConfirmation | null>(null);

// The Sheet owns only transient confirmation identity and review state.
// Authoritative financial facts remain in server queries.
// Synchronous registration closes the gap before React renders pending state.
const unresolved = new Set<object>();
export function registerSheetConfirmation(owner: object) { unresolved.add(owner); }
export function resolveSheetConfirmation(owner: object) { unresolved.delete(owner); }
export function hasUnresolvedSheetConfirmation() { return unresolved.size > 0; }
export function useSheetConfirmation() { return useContext(SheetConfirmationContext); }
