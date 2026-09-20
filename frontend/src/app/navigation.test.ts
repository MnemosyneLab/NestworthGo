import { describe, expect, it } from "vitest";
import { NAV_GROUPS, targetForPage, type NavigationTarget } from "./navigation";

describe("analysis navigation", () => {
  it("exposes typed Return and Asset Changes destinations as the only Insights items", () => {
    const insights = NAV_GROUPS.find((group) => group.id === "insights");
    expect(insights?.items.map((item) => item.id)).toEqual(["return-analysis", "asset-changes"]);
    expect(targetForPage("return-analysis")).toEqual({ page: "return-analysis" });
    expect(targetForPage("asset-changes")).toEqual({ page: "asset-changes" });
  });

  it("keeps filters and view state in a structured target", () => {
    const target: NavigationTarget = {
      page: "asset-changes",
      tab: "categories",
      cursor: "2026-09",
      analysis: { scope: "instrument", scopeId: "qqq", valuation: "native", includeCash: false, from: "2026-01-01", to: "2026-09-30" },
    };
    expect(target).toMatchObject({ page: "asset-changes", tab: "categories", cursor: "2026-09" });
    expect(target.analysis?.scopeId).toBe("qqq");
  });
});

describe("available funds navigation", () => {
  it("places Available funds after Accounts in the workspace group", () => {
    const workspace = NAV_GROUPS.find((group) => group.id === "workspace");
    expect(workspace?.items.map((item) => item.id)).toEqual(["accounts", "available-funds", "portfolio"]);
    expect(targetForPage("available-funds")).toEqual({ page: "available-funds" });
  });
});
