import { beforeEach, describe, expect, it } from "vitest";
import { useAnalysisStore } from "./analysis";

describe("analysis session store", () => {
  beforeEach(() => {
    useAnalysisStore.getState().reset();
  });

  it("shares universe filters between Return and Asset Changes views", () => {
    const state = useAnalysisStore.getState();
    state.setFilters({ scope: "account", scopeId: "checking", valuation: "native", includeCash: false, from: "2026-01-01", to: "2026-03-31" });
    state.setReturnView({ tab: "trend", cursor: "2026-02" });
    state.setAssetView({ tab: "categories", cursor: "2026-03" });

    expect(useAnalysisStore.getState()).toMatchObject({
      scope: "account",
      scopeId: "checking",
      valuation: "native",
      includeCash: false,
      from: "2026-01-01",
      to: "2026-03-31",
      returnTab: "trend",
      returnCursor: "2026-02",
      assetTab: "categories",
      assetCursor: "2026-03",
    });
  });

  it("does not change stored valuation when a forced-base result is observed elsewhere", () => {
    useAnalysisStore.getState().setFilters({ valuation: "native" });
    useAnalysisStore.getState().setAssetView({ tab: "trend" });
    expect(useAnalysisStore.getState().valuation).toBe("native");
  });

  it("stores a navigated Contribution return type without changing valuation", () => {
    useAnalysisStore.getState().setFilters({ valuation: "native", returnType: "dividend_interest" });
    expect(useAnalysisStore.getState()).toMatchObject({
      valuation: "native",
      contributionReturnType: "dividend_interest",
    });
  });
});

it("replaces unrelated filters when entering an object, while edits remain incremental", () => {
  const store = useAnalysisStore.getState();
  store.setFilters({ scope: "account", scopeId: "B", valuation: "native", moreFilters: { accountId: "B", memberId: "old-member", currency: "EUR" } });
  store.replaceFilters({ scope: "instrument", scopeId: "NVDA", includeCash: false, moreFilters: { accountId: "A" }, from: "2026-01-01", to: "2026-01-31" });
  expect(useAnalysisStore.getState()).toMatchObject({ scope: "instrument", scopeId: "NVDA", valuation: "base", includeCash: false, moreFilters: { accountId: "A" } });
  expect(useAnalysisStore.getState().moreFilters).toEqual({ accountId: "A" });
  store.replaceFilters({ scope: "instrument", scopeId: "NVDA" });
  expect(useAnalysisStore.getState().moreFilters).toEqual({});
});
