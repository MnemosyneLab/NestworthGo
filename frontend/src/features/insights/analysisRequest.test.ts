import { describe, expect, it, vi } from "vitest";
import { activeReturnTrendRange, effectiveRange, periodRange, returnTrendRange, yearRange } from "./analysisRequest";

describe("analysis request ranges", () => {
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
    expect(yearRange(2025, "UTC")).toEqual({ from: "2025-01-01", to: "2025-12-31" });
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
});
