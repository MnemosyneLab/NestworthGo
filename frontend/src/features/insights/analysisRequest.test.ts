import { describe, expect, it } from "vitest";
import { effectiveRange, periodRange, yearRange } from "./analysisRequest";

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
});
