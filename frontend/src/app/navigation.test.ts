import { describe, expect, it } from "vitest";
import { CalendarClock, ClipboardCheck, ReceiptText } from "lucide-react";
import { NAV_GROUPS, reconciliationFocusFromTarget, resolveLegacyTarget, targetForPage, type NavigationTarget } from "./navigation";
import i18n from "../i18n";

describe("analysis navigation", () => {
  it("exposes typed Return and Asset Changes destinations alongside Wealth history", () => {
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


it("places Tasks & checks before Ledger activity and redirects legacy activity targets", () => {
  expect(targetForPage("tasks")).toEqual({ page: "tasks" });
  const activity = NAV_GROUPS.find(group => group.id === "activity");
  expect(activity?.items.map(item => item.id)).toEqual(["tasks", "history"]);
  expect(activity?.items[0]?.icon).toBe(ClipboardCheck);
  expect(activity?.items[1]?.icon).toBe(ReceiptText);
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

it("labels the editable ledger timeline and dated asset view distinctly", () => {
  const activity = NAV_GROUPS.find(group => group.id === "activity");
  const insights = NAV_GROUPS.find(group => group.id === "insights");
  expect(activity?.items.find(item => item.id === "history")?.translationKey).toBe("nav.history");
  expect(insights?.items.find(item => item.id === "historical-overview")?.icon).toBe(CalendarClock);
  expect(i18n.t("nav.history", { lng: "en" })).toBe("Ledger activity");
  expect(i18n.t("nav.historicalOverview", { lng: "en" })).toBe("Wealth history");
  expect(i18n.t("historicalOverview.title", { lng: "en" })).toBe("Wealth history");
  expect(i18n.t("insights.viewInHistory", { lng: "en" })).toBe("View in ledger activity");
  expect(i18n.t("historicalOverview.openHistory", { lng: "en" })).toBe("Leave wealth history · View ledger activity");
  expect(i18n.t("nav.history", { lng: "zh-CN" })).toBe("账务记录");
  expect(i18n.t("nav.historicalOverview", { lng: "zh-CN" })).toBe("资产回溯");
  expect(i18n.t("historicalOverview.title", { lng: "zh-CN" })).toBe("资产回溯");
  expect(i18n.t("insights.viewInHistory", { lng: "zh-CN" })).toBe("在账务记录中查看");
  expect(i18n.t("nav.history", { lng: "zh-TW" })).toBe("帳務紀錄");
  expect(i18n.t("nav.historicalOverview", { lng: "zh-TW" })).toBe("資產回溯");
  expect(i18n.t("historicalOverview.title", { lng: "zh-TW" })).toBe("資產回溯");
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
