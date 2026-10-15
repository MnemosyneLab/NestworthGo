import { create } from "zustand";

/** Where a grouped holdings list is shown. Each scope remembers its own collapsed groups. */
export type HoldingGroupScope = "portfolio" | "account";

interface HoldingGroupState {
  /** Group keys the household collapsed, per scope. Groups default to expanded. */
  collapsed: Record<HoldingGroupScope, string[]>;
  toggleGroup: (scope: HoldingGroupScope, key: string) => void;
  setCollapsed: (scope: HoldingGroupScope, keys: string[]) => void;
  reset: () => void;
}

const empty = (): HoldingGroupState["collapsed"] => ({ portfolio: [], account: [] });

/**
 * Session-scoped UI memory for the holdings list. It only stores which groups
 * are collapsed so the choice survives tab and page changes; it never holds
 * backend data (see AGENTS.md: TanStack Query owns that).
 */
export const useHoldingGroupStore = create<HoldingGroupState>((set) => ({
  collapsed: empty(),
  toggleGroup: (scope, key) => set((state) => {
    const current = state.collapsed[scope];
    const next = current.includes(key) ? current.filter((item) => item !== key) : [...current, key];
    return { collapsed: { ...state.collapsed, [scope]: next } };
  }),
  setCollapsed: (scope, keys) => set((state) => ({ collapsed: { ...state.collapsed, [scope]: [...keys] } })),
  reset: () => set({ collapsed: empty() }),
}));
