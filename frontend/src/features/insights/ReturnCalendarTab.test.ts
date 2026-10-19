import { describe, expect, it, vi } from "vitest";

// The generated Wails service imports the runtime transitively. Keep this
// projection test isolated from the runtime's browser drag polling interval.
vi.mock("@wailsio/runtime", () => ({
  Call: {
    ByID: vi.fn(() => Promise.resolve(undefined)),
  },
}));

import type { ReturnCalendarDTO, ReturnCalendarMonthDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { aggregateYearMonths } from "./ReturnCalendarTab";

function month(overrides: Partial<ReturnCalendarMonthDTO>): ReturnCalendarMonthDTO {
  return {
    month: "2026-01",
    returnAmount: null,
    amountStatus: "unavailable",
    returnRate: null,
    ratedDays: 0,
    totalDays: 0,
    ...overrides,
  };
}

describe("aggregateYearMonths", () => {
  it("uses backend month summaries instead of folding daily cells in the UI", () => {
    const data = {
      months: [
        month({
          month: "2026-01",
          returnAmount: { amount: "0.004", currency: "USD" },
          amountStatus: "partial",
          returnRate: "0.123456789",
          ratedDays: 2,
          totalDays: 4,
        }),
        month({ month: "2026-02", amountStatus: "unavailable", ratedDays: 0, totalDays: 8 }),
      ],
      // Conflicting cells ensure the frontend ignores daily values for month
      // amount/rate summaries and uses the backend's one monthly contract.
      cells: [{ date: "2026-01-01", returnAmount: { amount: "99", currency: "USD" }, returnRate: "9" }],
    } as ReturnCalendarDTO;

    const summaries = aggregateYearMonths(data, 2026);
    expect(summaries.get("2026-01")).toEqual({
      returnAmount: { amount: "0.004", currency: "USD" },
      amountStatus: "partial",
      returnRate: "0.123456789",
      ratedDays: 2,
      totalDays: 4,
    });
    expect(summaries.get("2026-02")).toEqual({
      returnAmount: null,
      amountStatus: "unavailable",
      returnRate: null,
      ratedDays: 0,
      totalDays: 8,
    });
    expect(summaries.get("2026-03")).toEqual({
      returnAmount: null,
      amountStatus: "unavailable",
      returnRate: null,
      ratedDays: 0,
      totalDays: 0,
    });
  });
});
