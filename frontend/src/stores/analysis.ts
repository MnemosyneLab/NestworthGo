import { create } from "zustand";
import type { AnalysisNavigationContext, AnalysisTab, AssetChangesTab } from "@/app/navigation";

export interface AnalysisSessionState {
  scope: AnalysisNavigationContext["scope"];
  scopeId: string;
  valuation: NonNullable<AnalysisNavigationContext["valuation"]>;
  includeCash: boolean;
  from: string;
  to: string;
  moreFilters: Record<string, string | boolean | undefined>;
  returnTab: AnalysisTab;
  returnCursor: string;
  assetTab: AssetChangesTab;
  assetCursor: string;
  setFilters: (filters: AnalysisNavigationContext) => void;
  setReturnView: (view: { tab?: AnalysisTab; cursor?: string }) => void;
  setAssetView: (view: { tab?: AssetChangesTab; cursor?: string }) => void;
  reset: () => void;
}

const initialSession = {
  scope: "portfolio" as const,
  scopeId: "",
  valuation: "base" as const,
  includeCash: true,
  from: "",
  to: "",
  moreFilters: {},
  returnTab: "calendar" as const,
  returnCursor: "",
  assetTab: "drivers" as const,
  assetCursor: "",
};

export const useAnalysisStore = create<AnalysisSessionState>((set) => ({
  ...initialSession,
  setFilters: (filters) => set((state) => ({
    scope: filters.scope ?? state.scope,
    scopeId: filters.scopeId ?? state.scopeId,
    valuation: filters.valuation ?? state.valuation,
    includeCash: filters.includeCash ?? state.includeCash,
    from: filters.from ?? state.from,
    to: filters.to ?? state.to,
    moreFilters: filters.moreFilters ? { ...state.moreFilters, ...filters.moreFilters } : state.moreFilters,
  })),
  setReturnView: (view) => set((state) => ({
    returnTab: view.tab ?? state.returnTab,
    returnCursor: view.cursor ?? state.returnCursor,
  })),
  setAssetView: (view) => set((state) => ({
    assetTab: view.tab ?? state.assetTab,
    assetCursor: view.cursor ?? state.assetCursor,
  })),
  reset: () => set(initialSession),
}));

export const analysisSessionStore = useAnalysisStore;
