import { create } from "zustand";

export type Appearance = "system" | "light" | "dark";

interface UiState {
  sidebarCollapsed: boolean;
  toggleSidebar: () => void;
  setSidebarCollapsed: (collapsed: boolean) => void;

  /** Local UI-only view of the appearance preference; SettingsService
   * (Phase 5) owns the persisted value. Kept here per the frontend stack
   * decision Sec5.2's rule that Zustand owns frontend-only UI state, never
   * a copy of backend data. */
  appearance: Appearance;
  setAppearance: (appearance: Appearance) => void;
}

/**
 * useUiStore holds only frontend-only UI state (sidebar, appearance
 * toggle, temporary selections) per the frontend stack decision Sec5.2:
 * "不建议把 Accounts、Holdings、History 等后端数据复制进 Zustand." Backend
 * data always flows through TanStack Query (queries/), never through this
 * store.
 */
export const useUiStore = create<UiState>((set) => ({
  sidebarCollapsed: false,
  toggleSidebar: () => set((state) => ({ sidebarCollapsed: !state.sidebarCollapsed })),
  setSidebarCollapsed: (collapsed) => set({ sidebarCollapsed: collapsed }),

  appearance: "system",
  setAppearance: (appearance) => set({ appearance }),
}));
