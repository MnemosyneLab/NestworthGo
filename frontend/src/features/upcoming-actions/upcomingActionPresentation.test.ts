import { describe, expect, it } from "vitest";
import i18n from "@/i18n";
import { upcomingItem } from "@/test/upcomingActions";
import { upcomingNavigation, upcomingTitle, supportedUpcomingAction, upcomingReadTime, upcomingReadLabel, upcomingCountLabels } from "./upcomingActionPresentation";
describe("upcoming presentation", () => {
  it("uses receipt evidence and handles unknown kinds without a default action", () => {
    const item = upcomingItem();
    expect(upcomingTitle(i18n.t.bind(i18n), item)).toContain("2 days");
    expect(supportedUpcomingAction(upcomingItem({ kind: "future_kind" }))).toBe(false);
    expect(upcomingNavigation({ kind: "unknown", productId: "unsafe", sourceRef: null, reservationId: null })).toBeNull();
  });
  it("maps typed navigation without a financial command", () => {
    expect(upcomingNavigation(upcomingItem().primaryTarget)).toEqual({ page: "available-funds", focus: { kind: "product", productId: "product-1" } });
    expect(upcomingNavigation({ kind: "reservations", productId: null, sourceRef: null, reservationId: "missing-source" })).toEqual({ page: "available-funds", focus: { kind: "reservations", reservationId: "missing-source" } });
  });
});


describe("upcoming read timestamp", () => {
  const precise = "2026-10-11T23:30:12.123456789Z";
  it("uses the reported household zone across civil midnight without changing precision", () => {
    expect(upcomingReadTime(precise, "Asia/Shanghai", "en")?.replace(/\s+/g, " ")).toBe("Oct 12, 2026, 7:30 AM");
    expect(upcomingReadTime(precise, "UTC", "en")?.replace(/\s+/g, " ")).toBe("Oct 11, 2026, 11:30 PM");
    expect(precise).toBe("2026-10-11T23:30:12.123456789Z");
  });
  it.each(["en", "zh-CN", "zh-TW"])("localizes the timestamp and visibly names its zone in %s", language => {
    const t = i18n.getFixedT(language);
    const formatted = new Intl.DateTimeFormat(language, { timeZone: "Asia/Shanghai", dateStyle: "medium", timeStyle: "short" }).format(new Date(precise));
    const label = upcomingReadLabel(t, precise, "Asia/Shanghai", language);
    expect(label).toContain(formatted);
    expect(label).toContain("Asia/Shanghai");
    expect(label).not.toContain(precise);
  });
  it.each([
    ["", "UTC"], ["not-a-time", "UTC"], ["2026-10-11", "UTC"],
    ["2026-02-30T04:00:00Z", "UTC"], ["2026-10-11T24:00:00Z", "UTC"],
    ["2026-10-11T04:00:00+25:00", "UTC"], [precise, "invalid/zone"],
    [precise, ""], [precise, "system"],
  ])("shows unavailable instead of inventing a time for %s / %s", (value, zone) => {
    expect(upcomingReadTime(value, zone, "en")).toBeNull();
    expect(upcomingReadLabel(i18n.getFixedT("en"), value, zone, "en")).toBe("Read time unavailable");
  });
  it("uses the household's DST rules rather than a fixed offset", () => {
    expect(upcomingReadTime("2026-03-08T06:30:00Z", "America/New_York", "en")).toContain("1:30 AM");
    expect(upcomingReadTime("2026-03-08T07:30:00Z", "America/New_York", "en")).toContain("3:30 AM");
  });
});

describe("upcoming plural forms", () => {
  it.each([0, 1, 2])("formats each independent English count for %s", count => {
    const t = i18n.getFixedT("en");
    const labels = upcomingCountLabels(t, { total: count, receiptDue: count, reservationIssues: count, scheduled: count, eligible: count, dataChecks: count, scopeNotes: count });
    expect(t("upcomingActions.counts", labels)).toBe(`${count} ${count === 1 ? "receipt" : "receipts"} to verify · ${count} reservation ${count === 1 ? "issue" : "issues"} · ${count} ${count === 1 ? "schedule" : "schedules"}`);
    expect(t("upcomingActions.checksTitle", labels)).toBe(`Information affecting assessment (${count} data ${count === 1 ? "issue" : "issues"}, ${count} ${count === 1 ? "note" : "notes"})`);
    expect(t("upcomingActions.viewAll", { count })).toBe(`View all ${count} ${count === 1 ? "action" : "actions"}`);
    expect(t("upcomingActions.reservations", { count })).toBe(`${count} ${count === 1 ? "reservation" : "reservations"}`);
  });
  it.each(["en", "zh-CN", "zh-TW"])("resolves singular and plural copy in %s", language => {
    const t = i18n.getFixedT(language);
    for (const count of [0, 1, 2]) {
      for (const key of ["receiptCount", "reservationIssueCount", "scheduleCount", "dataIssueCount", "noteCount", "viewAll", "reservations", "inDays", "overdue"]) {
        const text = t(`upcomingActions.${key}`, { count });
        expect(text).toContain(String(count));
        expect(text).not.toMatch(/upcomingActions\.|{{/);
      }
    }
  });
});
