import { describe, expect, it } from "vitest";
import type { ReturnCalendarDTO, ReturnDayDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { aggregateMonth, aggregateYearMonths } from "./ReturnCalendarTab";

function cell(overrides: Partial<ReturnDayDTO>): ReturnDayDTO {
  return {
    date: "2026-01-01",
    returnAmount: null,
    amountStatus: "unavailable",
    returnRate: null,
    composition: null,
    contributors: null,
    ratedDays: 0,
    totalDays: 1,
    issues: null,
    available: false,
    status: "unavailable",
    valuationForced: null,
    ...overrides,
  };
}

describe("aggregateMonth", () => {
  it("omits the month rate below half coverage and keeps defined amounts", () => {
    const none = aggregateMonth(Array.from({ length: 4 }, (_, index) => cell({ date: `2026-01-0${index + 1}` })));
    expect(none.returnRate).toBeNull();
    expect(none.ratedDays).toBe(0);
    expect(none.totalDays).toBe(4);

    const oneRated = aggregateMonth([
      cell({ date: "2026-01-01", returnRate: "0.1", ratedDays: 1, available: true, status: "ok" }),
      cell({ date: "2026-01-02" }),
      cell({ date: "2026-01-03" }),
      cell({ date: "2026-01-04" }),
    ]);
    expect(oneRated.returnRate).toBeNull();
    expect(oneRated.ratedDays).toBe(1);

    const half = aggregateMonth([
      cell({ date: "2026-01-01", returnAmount: { amount: "1", currency: "USD" }, amountStatus: "complete", returnRate: "0.1", ratedDays: 1, available: true, status: "ok" }),
      cell({ date: "2026-01-02", returnAmount: { amount: "2", currency: "USD" }, amountStatus: "complete", returnRate: "0.1", ratedDays: 1, available: true, status: "ok" }),
      cell({ date: "2026-01-03" }),
      cell({ date: "2026-01-04" }),
    ]);
    expect(half.returnRate).not.toBeNull();
    expect(half.returnAmount).toEqual({ amount: "3", currency: "USD" });
    expect(half.amountStatus).toBe("partial");
    expect(half.ratedDays).toBe(2);

    const full = aggregateMonth([
      cell({ date: "2026-01-01", returnAmount: { amount: "0", currency: "USD" }, amountStatus: "complete", returnRate: "0", ratedDays: 1, available: true, status: "ok" }),
      cell({ date: "2026-01-02", returnAmount: { amount: "0", currency: "USD" }, amountStatus: "complete", returnRate: "0", ratedDays: 1, available: true, status: "ok" }),
      cell({ date: "2026-01-03", returnAmount: { amount: "0", currency: "USD" }, amountStatus: "complete", returnRate: "0", ratedDays: 1, available: true, status: "ok" }),
      cell({ date: "2026-01-04", returnAmount: { amount: "0", currency: "USD" }, amountStatus: "complete", returnRate: "0", ratedDays: 1, available: true, status: "ok" }),
    ]);
    expect(full.returnRate).toBe("0");
    expect(full.amountStatus).toBe("complete");
    expect(full.ratedDays).toBe(4);
  });

  it("keeps partial known zero/non-zero and fully unavailable amounts distinct", () => {
    const partial = aggregateMonth([
      cell({ date: "2026-01-01", returnAmount: { amount: "0", currency: "USD" }, amountStatus: "partial", available: true, status: "partial" }),
      cell({ date: "2026-01-02", returnAmount: { amount: "2", currency: "USD" }, amountStatus: "partial", available: true, status: "partial" }),
    ]);
    expect(partial.returnAmount).toEqual({ amount: "2", currency: "USD" });
    expect(partial.amountStatus).toBe("partial");

    const unavailable = aggregateMonth([
      cell({ date: "2026-01-01", amountStatus: "unavailable" }),
      cell({ date: "2026-01-02", amountStatus: "unavailable" }),
    ]);
    expect(unavailable.returnAmount).toBeNull();
    expect(unavailable.amountStatus).toBe("unavailable");
  });

  it("carries amount status into year-month summaries", () => {
    const data = { cells: [
      cell({ date: "2026-01-01", returnAmount: { amount: "0", currency: "USD" }, amountStatus: "partial", available: true, status: "partial" }),
      cell({ date: "2026-02-01", amountStatus: "unavailable" }),
    ] } as ReturnCalendarDTO;
    const summaries = aggregateYearMonths(data, 2026);
    expect(summaries.get("2026-01")?.amountStatus).toBe("partial");
    expect(summaries.get("2026-02")?.amountStatus).toBe("unavailable");
  });
});
