import { create } from "zustand";

export type Appearance = "system" | "light" | "dark";
export type Accent = "indigo" | "lavender" | "mint" | "sky" | "peach";

interface UiState {
  sidebarCollapsed: boolean;
  toggleSidebar: () => void;
  assistantWidth: number;
  setAssistantWidth: (width: number) => void;

  /** Local UI-only view of the appearance/accent preference; SettingsService
   * owns the persisted value. Zustand contains frontend-only UI state, never
   * a copy of backend data. */
  appearance: Appearance;
  setAppearance: (appearance: Appearance) => void;
  accent: Accent;
  setAccent: (accent: Accent) => void;
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
  // Layout choices share the sidebar's session lifetime; no ledger data or
  // browser persistence. Temporary viewport clamps do not replace this choice.
  assistantWidth: 480,
  setAssistantWidth: (assistantWidth) => set({ assistantWidth }),

  appearance: "system",
  setAppearance: (appearance) => set({ appearance }),
  accent: "indigo",
  setAccent: (accent) => set({ accent }),
}));
