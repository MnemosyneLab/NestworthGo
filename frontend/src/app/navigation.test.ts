import { describe, expect, it } from "vitest";
import { ClipboardCheck } from "lucide-react";
import { NAV_GROUPS, reconciliationFocusFromTarget, resolveLegacyTarget, targetForPage, type NavigationTarget } from "./navigation";

describe("analysis navigation", () => {
  it("exposes typed Return and Asset Changes destinations alongside Historical overview", () => {
    const insights = NAV_GROUPS.find((group) => group.id === "insights");
    expect(insights?.items.map((item) => item.id)).toEqual(["historical-overview", "return-analysis", "asset-changes"]);
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


it("places Tasks & checks before History and redirects legacy activity targets", () => {
  expect(targetForPage("tasks")).toEqual({ page: "tasks" });
  const activity = NAV_GROUPS.find(group => group.id === "activity");
  expect(activity?.items.map(item => item.id)).toEqual(["tasks", "history"]);
  expect(activity?.items[0]?.icon).toBe(ClipboardCheck);
  expect(resolveLegacyTarget({ page: "upcoming-actions" })).toEqual({ page: "tasks", tab: "upcoming" });
  expect(resolveLegacyTarget({ page: "upcoming-actions", tab: "recent" })).toEqual({ page: "tasks", tab: "upcoming" });
  expect(resolveLegacyTarget({ page: "upcoming-actions", tab: "review" })).toEqual({ page: "tasks", tab: "review" });
  expect(resolveLegacyTarget({ page: "reconciliation", accountId: "bank", sessionId: "session-1" })).toEqual({
    page: "tasks", tab: "reconciliation", accountId: "bank", sessionId: "session-1",
  });
  expect(reconciliationFocusFromTarget(resolveLegacyTarget({ page: "reconciliation", accountId: "bank", sessionId: "session-1" }))).toEqual({ accountId: "bank", sessionId: "session-1" });
  expect(reconciliationFocusFromTarget(resolveLegacyTarget({ page: "reconciliation" }))).toEqual({});
  expect(reconciliationFocusFromTarget(resolveLegacyTarget({ page: "upcoming-actions", tab: "recent" }))).toEqual({});
  expect(reconciliationFocusFromTarget(targetForPage("tasks"))).toEqual({});
  expect(reconciliationFocusFromTarget({ page: "tasks", tab: "reconciliation", accountId: "bank" })).toEqual({ accountId: "bank", sessionId: undefined });
  const legacy: NavigationTarget = { page: "available-funds", productId: "synthetic", accountId: "bank" };
  const focus: NavigationTarget = { page: "available-funds", focus: { kind: "reservations", reservationId: "released" } };
  expect(legacy.productId).toBe("synthetic");
  expect(focus.focus?.kind).toBe("reservations");
});

it("keeps old destinations while routing plain instrument links to the shared data record", () => {
  expect(targetForPage("portfolio")).toEqual({ page: "portfolio", view: "holdings" });
  expect(NAV_GROUPS.find((group) => group.id === "settings")?.items.map((item) => item.id)).toEqual(["market-data", "settings"]);
  expect(resolveLegacyTarget({ page: "market-data", focus: { instrumentId: "fund-1", accountId: "broker-1", rangeStart: "2026-01-01", rangeEnd: "2026-09-30" } })).toEqual({
    page: "instrument-detail", instrumentId: "fund-1", accountId: "broker-1", section: "data", from: "2026-01-01", to: "2026-09-30",
    healthFocus: { instrumentId: "fund-1", accountId: "broker-1", rangeStart: "2026-01-01", rangeEnd: "2026-09-30" },
  });
  const repair: NavigationTarget = { page: "market-data", focus: { instrumentId: "fund-1", action: "repair", rangeStart: "2026-01-01" } };
  expect(resolveLegacyTarget(repair)).toBe(repair);
  expect(resolveLegacyTarget({ page: "data-health", focus: { id: "gap", instrumentId: "fund-1" } })).toEqual({ page: "data-health", focus: { id: "gap", instrumentId: "fund-1" } });
});
