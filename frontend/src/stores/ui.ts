import { create } from "zustand";

export type Appearance = "system" | "light" | "dark";

interface UiState {
  sidebarCollapsed: boolean;
  toggleSidebar: () => void;
  setSidebarCollapsed: (collapsed: boolean) => void;

  /** Local UI-only view of the appearance preference; SettingsService owns
   * the persisted value. Zustand contains frontend-only UI state, never a
   * copy of backend data. */
  appearance: Appearance;
  setAppearance: (appearance: Appearance) => void;
}

/**
 * useUiStore holds only frontend-only UI state (sidebar, appearance
 * toggle, temporary selections). Backend data always flows through TanStack
 * Query (queries/), never through this
 * store.
 */
export const useUiStore = create<UiState>((set) => ({
  sidebarCollapsed: false,
  toggleSidebar: () => set((state) => ({ sidebarCollapsed: !state.sidebarCollapsed })),
  setSidebarCollapsed: (collapsed) => set({ sidebarCollapsed: collapsed }),

  appearance: "system",
  setAppearance: (appearance) => set({ appearance }),
}));
