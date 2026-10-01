import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { currentMonth } from "./calendar";
import { activeReturnTrendRange, currentMonthAwaitingClose, effectiveRange, periodRange, returnTrendRange } from "./analysisRequest";

describe("analysis request ranges", () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-09-07T12:00:00Z"));
  });
  afterEach(() => vi.useRealTimers());

  it.each([
    ["2026-09-30T12:00:00Z", "UTC", "2026-09-01", "2026-09-29", false],
    ["2026-10-01T12:00:00Z", "UTC", "2026-10-01", "2026-09-30", true],
    ["2026-10-02T12:00:00Z", "UTC", "2026-10-01", "2026-10-01", false],
    ["2026-11-01T12:00:00Z", "UTC", "2026-11-01", "2026-10-31", true],
    ["2027-01-01T00:00:00Z", "UTC", "2027-01-01", "2026-12-31", true],
    ["2026-09-30T16:00:00Z", "Asia/Singapore", "2026-10-01", "2026-09-30", true],
    ["2026-10-01T00:00:00Z", "America/Los_Angeles", "2026-09-01", "2026-09-29", false],
    ["2027-01-01T00:00:00Z", "America/Los_Angeles", "2026-12-01", "2026-12-30", false],
    ["2026-12-31T16:00:00Z", "Asia/Singapore", "2027-01-01", "2026-12-31", true],
  ])("uses closed local dates at %s in %s", (instant, timeZone, from, to, awaiting) => {
    vi.setSystemTime(new Date(instant));
    const state = { from: "", to: "" };
    const month = currentMonth(timeZone);
    expect(effectiveRange(state, month, timeZone, "2026-01-01T00:00:00Z")).toEqual({ from, to });
    expect(currentMonthAwaitingClose(state, month, timeZone, "2026-01-01T00:00:00Z")).toBe(awaiting);
  });

  it("does not label explicit filters, other months, or future Origin as an unclosed current month", () => {
    vi.setSystemTime(new Date("2026-10-01T12:00:00Z"));
    expect(currentMonthAwaitingClose({ from: "2026-10-01", to: "2026-10-01" }, "2026-10", "UTC")).toBe(false);
    expect(currentMonthAwaitingClose({ from: "", to: "" }, "2026-09", "UTC")).toBe(false);
    expect(currentMonthAwaitingClose({ from: "", to: "" }, "2026-10", "UTC", "2026-10-02T00:00:00Z")).toBe(false);
  });
  it("intersects year and month cards with an explicit session date range", () => {
    const state = { from: "2025-03-15", to: "2025-10-20" };

    expect(periodRange(state, "2025-01-01", "2025-12-31", "UTC")).toEqual({ from: "2025-03-15", to: "2025-10-20" });
    expect(periodRange(state, "2025-04-01", "2025-04-30", "UTC")).toEqual({ from: "2025-04-01", to: "2025-04-30" });
    expect(periodRange(state, "2025-01-01", "2025-02-28", "UTC")).toEqual({ from: "2025-03-15", to: "2025-02-28" });
  });

  it("keeps the visible-month query as the full selected period", () => {
    expect(effectiveRange({ from: "2025-03-15", to: "2025-10-20" }, "2025-04", "UTC")).toEqual({ from: "2025-03-15", to: "2025-10-20" });
    expect(effectiveRange({ from: "2025-03-15", to: "" }, "2025-04", "UTC")).toEqual({ from: "2025-03-15", to: "2025-04-30" });
    expect(effectiveRange({ from: "", to: "2025-04-20" }, "2025-04", "UTC")).toEqual({ from: "2025-04-01", to: "2025-04-20" });
    expect(periodRange({ from: "", to: "" }, "2025-01-01", "2025-12-31", "UTC")).toEqual({ from: "2025-01-01", to: "2025-12-31" });
  });

  it("derives named Return Trend ranges from the closed Origin-local day", () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-09-08T12:00:00Z"));
    expect(returnTrendRange("30d", "2026-01-01T00:00:00Z", "UTC")).toEqual({ from: "2026-08-09", to: "2026-09-07" });
    expect(returnTrendRange("ytd", "2026-01-01T00:00:00Z", "UTC")).toEqual({ from: "2026-01-01", to: "2026-09-07" });
    expect(returnTrendRange("all", "2026-06-15T00:00:00Z", "UTC")).toEqual({ from: "2026-06-15", to: "2026-09-07" });
    expect(returnTrendRange("all", "2025-12-31T16:00:00Z", "Asia/Singapore")).toEqual({ from: "2026-01-01", to: "2026-09-07" });
    vi.useRealTimers();
  });

  it("recognizes a custom Return Trend range without rewriting it", () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-09-08T12:00:00Z"));
    expect(activeReturnTrendRange({ from: "2026-08-10", to: "2026-09-07" }, "2026-01-01T00:00:00Z", "UTC")).toBe("custom");
    expect(activeReturnTrendRange({ from: "", to: "" }, "2026-01-01T00:00:00Z", "UTC")).toBeNull();
    expect(activeReturnTrendRange({ from: "2026-08-09", to: "" }, "2026-01-01T00:00:00Z", "UTC")).toBeNull();
    vi.useRealTimers();
  });

  it("clamps default month and year ranges to History Origin", () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-09-08T12:00:00Z"));
    expect(effectiveRange({ from: "", to: "" }, "2026-09", "UTC", "2026-09-03T00:00:00Z")).toEqual({ from: "2026-09-03", to: "2026-09-07" });
    expect(periodRange({ from: "", to: "" }, "2026-01-01", "2026-12-31", "UTC", "2026-09-03T00:00:00Z")).toEqual({ from: "2026-09-03", to: "2026-09-07" });
    expect(effectiveRange({ from: "", to: "" }, "2026-08", "UTC", "2026-09-03T00:00:00Z")).toEqual({ from: "2026-09-03", to: "2026-08-31" });
    vi.useRealTimers();
  });
});
